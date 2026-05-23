package domain

import (
	"context"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

type AuditService interface {
	Log(ctx context.Context, domainID uint, operation string, fromState, toState string, status string, err error, duration time.Duration) error
}

type gormAuditService struct {
	db *gorm.DB
}

func NewAuditService(db *gorm.DB) AuditService {
	return &gormAuditService{db: db}
}

func (s *gormAuditService) Log(ctx context.Context, domainID uint, operation string, fromState, toState string, status string, err error, duration time.Duration) error {
	tc := GetTraceContext(ctx)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	log := &models.AuditLog{
		DomainID:     domainID,
		Operation:    operation,
		StateFrom:    fromState,
		StateTo:      toState,
		Status:       status,
		ErrorMessage: errMsg,
		DurationMs:   duration.Milliseconds(),
		TraceID:      tc.TraceID,
		CreatedAt:    time.Now(),
	}

	return s.db.WithContext(ctx).Create(log).Error
}
