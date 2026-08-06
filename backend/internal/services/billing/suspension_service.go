package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/services/billinggate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const billingSuspensionInterval = time.Minute

// SuspensionService applies the day-seven billing transition without deleting data.
// Physical database suspension and project stop are separate durable processors.
type SuspensionService struct {
	db        *gorm.DB
	graceDays int
}

func NewSuspensionService(db *gorm.DB, cfg *config.Config) *SuspensionService {
	graceDays := 0
	if cfg != nil && cfg.BillingEnabled {
		graceDays = cfg.BillingGraceDays
	}
	return &SuspensionService{db: db, graceDays: graceDays}
}

func (s *SuspensionService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.graceDays <= 0 || ctx == nil {
		return
	}
	s.runOnce(ctx, time.Now().UTC())
	ticker := time.NewTicker(billingSuspensionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.runOnce(ctx, now.UTC())
		}
	}
}

func (s *SuspensionService) runOnce(ctx context.Context, now time.Time) {
	if err := s.SuspendOverdue(ctx, now); err != nil {
		slog.Error("Billing suspension scheduler failed", "error", err)
	}
}

// SuspendOverdue suspends resources whose oldest open invoice is past the grace period.
func (s *SuspensionService) SuspendOverdue(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil || s.graceDays <= 0 || ctx == nil {
		return ErrInvoiceServiceUnavailable
	}
	var resourceIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&models.BillableResource{}).
		Where("billing_status IN ?", []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}).
		Order("id ASC").
		Pluck("id", &resourceIDs).Error; err != nil {
		return fmt.Errorf("list payment-due billing resources: %w", err)
	}

	var runErr error
	for _, resourceID := range resourceIDs {
		if err := s.suspendResource(ctx, resourceID, now.UTC()); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("suspend billable resource %d: %w", resourceID, err))
		}
	}
	return runErr
}

func (s *SuspensionService) suspendResource(ctx context.Context, resourceID uint, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&resource, resourceID).Error; err != nil {
			return fmt.Errorf("lock billable resource: %w", err)
		}
		if resource.BillingStatus != models.BillableResourceStatusPaymentDue && resource.BillingStatus != models.BillableResourceStatusSuspended {
			return nil
		}

		dueAt, err := billinggate.OldestOpenInvoiceDueAt(ctx, tx, resource.ID)
		if err != nil {
			return err
		}
		if dueAt == nil {
			return billinggate.ErrPaymentDueEvidenceUnavailable
		}
		if now.Before(dueAt.UTC().AddDate(0, 0, s.graceDays)) {
			return nil
		}
		newlySuspended := resource.BillingStatus == models.BillableResourceStatusPaymentDue
		if newlySuspended {
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
				return fmt.Errorf("mark billable resource suspended: %w", err)
			}
		}
		if !newlySuspended {
			return nil
		}

		switch resource.Type {
		case models.BillableTypeProject:
			return ensureProjectSuspensionTaskTx(tx, resource, true)
		case models.BillableTypeDatabase:
			return ensureDatabaseSuspensionTaskTx(tx, resource)
		default:
			return ErrInvalidInvoiceInput
		}
	})
}

func ensureProjectSuspensionTaskTx(tx *gorm.DB, resource models.BillableResource, reopen bool) error {
	var project models.Project
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, resource.ResourceID).Error; err != nil {
		return fmt.Errorf("lock project for billing suspension: %w", err)
	}
	if project.Status == models.StatusDeleting {
		return nil
	}
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: resource.UserID}
	conflict := clause.OnConflict{DoNothing: true}
	if reopen {
		conflict = clause.OnConflict{
			Columns: []clause.Column{{Name: "project_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"billable_resource_id": resource.ID,
				"user_id":              resource.UserID,
				"main_container_id":    "",
				"worker_container_id":  "",
				"main_was_running":     false,
				"worker_was_running":   false,
				"stop_attempted_at":    nil,
				"stop_completed_at":    nil,
				"resume_requested_at":  nil,
				"resume_completed_at":  nil,
				"completed_at":         nil,
				"last_error":           "",
				"retry_count":          0,
			}),
		}
	}
	if err := tx.Clauses(conflict).Create(&task).Error; err != nil {
		return fmt.Errorf("create project suspension task: %w", err)
	}
	return nil
}

func ensureDatabaseSuspensionTaskTx(tx *gorm.DB, resource models.BillableResource) error {
	return ensureDatabaseStatusOperationTaskTx(tx, resource, models.DBStatusSuspended)
}

func ensureDatabaseStatusOperationTaskTx(tx *gorm.DB, resource models.BillableResource, desiredStatus models.DatabaseInstanceStatus) error {
	if desiredStatus != models.DBStatusActive && desiredStatus != models.DBStatusSuspended {
		return fmt.Errorf("invalid database billing status operation: %s", desiredStatus)
	}
	var instance models.DatabaseInstance
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, resource.ResourceID).Error; err != nil {
		return fmt.Errorf("lock database for billing status operation: %w", err)
	}
	if instance.Status == models.DBStatusDeleted {
		return nil
	}

	var task models.DatabaseStatusOperationTask
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("database_instance_id = ?", instance.ID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if instance.Status == desiredStatus {
			return nil
		}
		task = models.DatabaseStatusOperationTask{
			DatabaseInstanceID:  instance.ID,
			DatabaseInstanceUID: instance.UID,
			Engine:              instance.Engine,
			Name:                instance.Name,
			Username:            instance.Username,
			DesiredStatus:       desiredStatus,
		}
		if err := tx.Create(&task).Error; err != nil {
			return fmt.Errorf("create database billing status operation: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock database billing status operation: %w", err)
	}
	if task.DesiredStatus != desiredStatus || task.PhysicalApplied {
		if err := tx.Model(&task).Updates(map[string]any{
			"desired_status":   desiredStatus,
			"physical_applied": false,
			"last_error":       "",
			"retry_count":      0,
		}).Error; err != nil {
			return fmt.Errorf("replace database billing status operation: %w", err)
		}
	}
	return nil
}

// SuspensionViews returns a deliberately narrow admin view without wallet or payment internals.
func (s *SuspensionService) SuspensionViews(ctx context.Context, now time.Time) ([]SuspensionView, error) {
	return s.suspensionViews(ctx, nil, now)
}

func (s *SuspensionService) SuspensionViewsForUser(ctx context.Context, userID uint, now time.Time) ([]SuspensionView, error) {
	if userID == 0 {
		return nil, ErrInvalidInvoiceInput
	}
	return s.suspensionViews(ctx, &userID, now)
}

func (s *SuspensionService) suspensionViews(ctx context.Context, userID *uint, now time.Time) ([]SuspensionView, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvoiceServiceUnavailable
	}
	query := s.db.WithContext(ctx).
		Where("billing_status IN ?", []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}).
		Order("id ASC")
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	var resources []models.BillableResource
	if err := query.Find(&resources).Error; err != nil {
		return nil, fmt.Errorf("list billing suspension resources: %w", err)
	}
	views := make([]SuspensionView, 0, len(resources))
	for _, resource := range resources {
		dueAt, err := billinggate.OldestOpenInvoiceDueAt(ctx, s.db, resource.ID)
		if err != nil {
			return nil, err
		}
		view := SuspensionView{ResourceID: resource.ResourceID, ResourceType: resource.Type, UserID: resource.UserID, Status: resource.BillingStatus, OldestDueAt: dueAt}
		if dueAt != nil && now.After(*dueAt) {
			view.PaymentDueDays = int(now.UTC().Sub(dueAt.UTC()) / (24 * time.Hour))
		}
		views = append(views, view)
	}
	return views, nil
}

type SuspensionView struct {
	ResourceID     uint                          `json:"resource_id"`
	ResourceType   models.BillableType           `json:"resource_type"`
	UserID         uint                          `json:"user_id"`
	Status         models.BillableResourceStatus `json:"status"`
	OldestDueAt    *time.Time                    `json:"oldest_due_at,omitempty"`
	PaymentDueDays int                           `json:"payment_due_days"`
}
