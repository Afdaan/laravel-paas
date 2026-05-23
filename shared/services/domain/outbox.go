package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repository"
	"gorm.io/gorm"
)

type OutboxService interface {
	Enqueue(ctx context.Context, repo repository.DomainRepository, eventType string, domain *models.CustomDomain, payload interface{}) error
	PublishPending(ctx context.Context) error
}

type gormOutboxService struct {
	db           *gorm.DB
	redisService *infrastructure.RedisService
}

func NewOutboxService(db *gorm.DB, redisService *infrastructure.RedisService) OutboxService {
	return &gormOutboxService{
		db:           db,
		redisService: redisService,
	}
}

func (s *gormOutboxService) Enqueue(ctx context.Context, repo repository.DomainRepository, eventType string, domain *models.CustomDomain, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Event is created inside the caller's active database transaction.
	// This guarantees that if the transaction rolls back, no dirty events are published to Redis/SSE.
	event := &models.OutboxEvent{
		EventType:  eventType,
		DomainID:   domain.ID,
		ProjectID:  domain.ProjectID,
		Sequence:   domain.CurrentSequence,
		Payload:    payloadBytes,
		Published:  false,
		RetryCount: 0,
		CreatedAt:  time.Now(),
	}
	return repo.RawDB().WithContext(ctx).Create(event).Error
}

func (s *gormOutboxService) PublishPending(ctx context.Context) error {
	var events []models.OutboxEvent
	// Order strictly by ID ascending to preserve monotonic sequence order for clients.
	// Out-of-order SSE rendering will flicker or rollback state on the user's browser.
	if err := s.db.WithContext(ctx).Where("published = ?", false).Order("id asc").Limit(100).Find(&events).Error; err != nil {
		return err
	}

	for _, e := range events {
		// Attempt delivery over Redis Pub/Sub channels to active SSE streams.
		err := s.redisService.PublishDomainEvent(e.DomainID, e.ProjectID, string(e.Payload))
		if err != nil {
			// Increment retry count so we can diagnose stuck event delivery in logs.
			// Do not block the entire publish loop for a single bad domain stream.
			s.db.WithContext(ctx).Model(&e).Updates(map[string]interface{}{
				"retry_count": gorm.Expr("retry_count + 1"),
			})
			continue
		}
		s.db.WithContext(ctx).Model(&e).Update("published", true)
	}
	return nil
}
