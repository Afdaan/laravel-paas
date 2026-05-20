package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/repositories"
	"gorm.io/gorm"
)

type NginxReloader interface {
	SyncProjectNginxFrom(project *models.Project, triggerSource string) (string, error)
}

// DomainService handles custom domain management and verification
type DomainService struct {
	cfg            *config.Config
	db             *gorm.DB
	redisService   *infrastructure.RedisService
	projectService NginxReloader
	projectRepo    repositories.ProjectRepository
}

func NewDomainService(cfg *config.Config, db *gorm.DB, redisService *infrastructure.RedisService, projectService NginxReloader, projectRepo repositories.ProjectRepository) *DomainService {
	return &DomainService{
		cfg:            cfg,
		db:             db,
		redisService:   redisService,
		projectService: projectService,
		projectRepo:    projectRepo,
	}
}

// GetDomainByID retrieves a custom domain by its ID
func (s *DomainService) GetDomainByID(domainID uint) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := s.db.First(&domain, domainID).Error; err != nil {
		return nil, err
	}
	return &domain, nil
}

// RecordEvent appends a lifecycle transition or audit event to the append-only DomainEvent table and broadcasts it via Redis Pub/Sub immediately (non-transactional).
func (s *DomainService) RecordEvent(domain *models.CustomDomain, stateFrom, stateTo models.CustomDomainStatus, eventType, payload, errMsg string) error {
	event, err := s.RecordEventTx(s.db, domain, stateFrom, stateTo, eventType, payload, errMsg)
	if err != nil {
		return err
	}
	eventBytes, _ := json.Marshal(event)
	_ = s.redisService.PublishDomainEvent(domain.ID, domain.ProjectID, string(eventBytes))
	return nil
}

// RecordEventTx executes event persistence inside an active database transaction.
// To guarantee consistency and prevent phantom events on rollback, the caller MUST publish the returned DomainEvent to Redis only after the enclosing transaction successfully commits.
func (s *DomainService) RecordEventTx(tx *gorm.DB, domain *models.CustomDomain, stateFrom, stateTo models.CustomDomainStatus, eventType, payload, errMsg string) (*models.DomainEvent, error) {
	var nextSeq int
	err := tx.Raw("UPDATE custom_domains SET current_sequence = current_sequence + 1 WHERE id = ? RETURNING current_sequence", domain.ID).Scan(&nextSeq).Error
	if err != nil {
		return nil, fmt.Errorf("failed atomic sequence increment: %w", err)
	}
	domain.CurrentSequence = nextSeq

	event := &models.DomainEvent{
		DomainID:       domain.ID,
		JobID:          fmt.Sprintf("job-%d-%d", domain.ID, time.Now().UnixNano()),
		SequenceNumber: domain.CurrentSequence,
		StateFrom:      stateFrom,
		StateTo:        stateTo,
		EventType:      eventType,
		Payload:        payload,
		Error:          errMsg,
		CreatedAt:      time.Now(),
	}

	if err := tx.Create(event).Error; err != nil {
		slog.Error("Failed to persist DomainEvent inside tx", "domainID", domain.ID, "error", err)
		return nil, err
	}

	return event, nil
}

// TransitionState deterministically mutates domain status and error codes with full transaction audit observability.
func (s *DomainService) TransitionState(domain *models.CustomDomain, nextState models.CustomDomainStatus, errCode models.DomainErrorCode, errMsg string) error {
	if domain.Status == nextState && domain.ErrorCode == errCode && domain.ErrorMessage == errMsg {
		// Suppress duplicate identical state transition to prevent database write amplification and event flood
		return nil
	}

	stateFrom := domain.Status
	var publishedEvent *models.DomainEvent

	err := s.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"status":        nextState,
			"error_code":    errCode,
			"error_message": errMsg,
			"updated_at":    time.Now(),
		}
		if err := tx.Model(domain).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to save domain state transition: %w", err)
		}
		domain.Status = nextState
		domain.ErrorCode = errCode
		domain.ErrorMessage = errMsg
		domain.UpdatedAt = time.Now()

		eventType := fmt.Sprintf("transition_%s", string(nextState))
		event, err := s.RecordEventTx(tx, domain, stateFrom, nextState, eventType, fmt.Sprintf("Transitioned from %s to %s", string(stateFrom), string(nextState)), errMsg)
		if err != nil {
			return err
		}
		publishedEvent = event
		return nil
	})

	if err == nil && publishedEvent != nil {
		eventBytes, _ := json.Marshal(publishedEvent)
		_ = s.redisService.PublishDomainEvent(domain.ID, domain.ProjectID, string(eventBytes))
	}
	return err
}

// TransitionStateCtx deterministically mutates domain status with lock-scoped context awareness.
func (s *DomainService) TransitionStateCtx(ctx context.Context, domain *models.CustomDomain, nextState models.CustomDomainStatus, errCode models.DomainErrorCode, errMsg string) error {
	if ctx != nil && ctx.Err() != nil {
		slog.Warn("Operation aborted due to context cancellation or lock lease loss", "domainID", domain.ID, "nextState", nextState, "error", ctx.Err())
		return fmt.Errorf("operation aborted due to context cancellation or lock lease loss: %w", ctx.Err())
	}
	return s.TransitionState(domain, nextState, errCode, errMsg)
}

// SubscribeEvents subscribes to domain realtime lifecycle audit events.
func (s *DomainService) SubscribeEvents(ctx context.Context, domainID uint) (<-chan string, error) {
	return s.redisService.SubscribeDomainEvents(ctx, domainID)
}

// SubscribeProjectEvents subscribes to project-wide realtime lifecycle audit events.
func (s *DomainService) SubscribeProjectEvents(ctx context.Context, projectID uint) (<-chan string, error) {
	return s.redisService.SubscribeProjectEvents(ctx, projectID)
}

// ListEventsAfterSequence fetches missed events after a specific monotonic sequence number (bounded in memory).
func (s *DomainService) ListEventsAfterSequence(domainID uint, afterSeq int) ([]models.DomainEvent, error) {
	var events []models.DomainEvent
	err := s.db.Where("domain_id = ? AND sequence_number > ?", domainID, afterSeq).Order("sequence_number ASC").Limit(100).Find(&events).Error
	return events, err
}

// ListProjectEventsAfterSequence fetches missed events for all domains within a project after a specific sequence number (bounded in memory).
func (s *DomainService) ListProjectEventsAfterSequence(projectID uint, afterSeq int) ([]models.DomainEvent, error) {
	var events []models.DomainEvent
	subQuery := s.db.Model(&models.CustomDomain{}).Select("id").Where("project_id = ?", projectID)
	err := s.db.Where("domain_id IN (?) AND sequence_number > ?", subQuery, afterSeq).Order("sequence_number ASC").Limit(200).Find(&events).Error
	return events, err
}

// StartLockHeartbeat starts a background watchdog goroutine to periodically extend domain lock lease TTL during long-running operations.
// If lock renewal fails (e.g. lease expired or token mismatch), it triggers cancel() to abort the operation immediately.
func (s *DomainService) StartLockHeartbeat(ctx context.Context, cancel context.CancelFunc, domain *models.CustomDomain, token string, ttl time.Duration) {
	ticker := time.NewTicker(ttl / 2)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.redisService.RenewDomainLock(domain.ID, token, ttl); err != nil {
					metrics.GetCollector().IncrLeaseRenewalFailures()
					metrics.GetCollector().IncrLeaseOwnershipLoss()
					slog.Error("Domain lock heartbeat renewal failed, cancelling operation context", "domainID", domain.ID, "token", token, "error", err)
					eventJSON := fmt.Sprintf(`{"event_type":"lease_loss","domain_id":%d,"error":"Worker lost lock lease ownership"}`, domain.ID)
					_ = s.redisService.PublishDomainEvent(domain.ID, domain.ProjectID, eventJSON)
					cancel()
					return
				}
			}
		}
	}()
}

// GetMetrics retrieves operational observability metrics.
func (s *DomainService) GetMetrics() (map[string]interface{}, error) {
	return s.redisService.GetDomainMetrics()
}
