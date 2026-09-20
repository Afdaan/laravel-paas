package billing

import (
	"context"
	"database/sql/driver"
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

type scannedTime struct {
	Time  time.Time
	Valid bool
}

func (value *scannedTime) Scan(source any) error {
	if source == nil {
		return nil
	}
	if parsed, ok := source.(time.Time); ok {
		value.Time, value.Valid = parsed, true
		return nil
	}
	text := ""
	switch typed := source.(type) {
	case string:
		text = typed
	case []byte:
		text = string(typed)
	default:
		return fmt.Errorf("unsupported billing due time type %T", source)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999"} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			value.Time, value.Valid = parsed, true
			return nil
		}
	}
	return fmt.Errorf("invalid billing due time %q", text)
}

func (value scannedTime) Value() (driver.Value, error) {
	if !value.Valid {
		return nil, nil
	}
	return value.Time, nil
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
	views, _, err := s.suspensionViews(ctx, nil, now, 0, 0)
	return views, err
}

func (s *SuspensionService) ListSuspensionViews(ctx context.Context, page, limit int, now time.Time) (AdminCollection[SuspensionView], error) {
	page, limit, err := normalizeAdminCollectionPage(page, limit)
	if err != nil {
		return AdminCollection[SuspensionView]{}, err
	}
	views, total, err := s.suspensionViews(ctx, nil, now, page, limit)
	if err != nil {
		return AdminCollection[SuspensionView]{}, err
	}
	return AdminCollection[SuspensionView]{Data: views, Page: page, Limit: limit, Total: total}, nil
}

func (s *SuspensionService) SuspensionViewsForUser(ctx context.Context, userID uint, now time.Time) ([]SuspensionView, error) {
	if userID == 0 {
		return nil, ErrInvalidInvoiceInput
	}
	views, _, err := s.suspensionViews(ctx, &userID, now, 0, 0)
	return views, err
}

func (s *SuspensionService) suspensionViews(ctx context.Context, userID *uint, now time.Time, page, limit int) ([]SuspensionView, int64, error) {
	if s == nil || s.db == nil {
		return nil, 0, ErrInvoiceServiceUnavailable
	}
	query := s.db.WithContext(ctx).Table("billable_resources").
		Joins("LEFT JOIN invoice_items ON invoice_items.billable_resource_id = billable_resources.id").
		Joins("LEFT JOIN invoices ON invoices.id = invoice_items.invoice_id AND invoices.status = ? AND invoices.due_at IS NOT NULL", models.InvoiceStatusPaymentDue).
		Joins("LEFT JOIN users ON users.id = billable_resources.user_id").
		Where("billable_resources.billing_status IN ?", []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}).
		Where(`
			(billable_resources.type = ? AND EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?))
			OR (billable_resources.type = ? AND EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?))
		`, models.BillableTypeProject, models.StatusDeleting, models.BillableTypeDatabase, models.DBStatusDeleted).
		Group("billable_resources.id, billable_resources.resource_id, billable_resources.type, billable_resources.user_id, billable_resources.billing_status, users.name, users.email")
	if userID != nil {
		query = query.Where("billable_resources.user_id = ?", *userID)
	}

	var total int64
	if page > 0 {
		countQuery := s.db.WithContext(ctx).Table("billable_resources").
			Joins("LEFT JOIN invoice_items ON invoice_items.billable_resource_id = billable_resources.id").
			Joins("LEFT JOIN invoices ON invoices.id = invoice_items.invoice_id AND invoices.status = ? AND invoices.due_at IS NOT NULL", models.InvoiceStatusPaymentDue).
			Where("billable_resources.billing_status IN ?", []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}).
			Where(`
				(billable_resources.type = ? AND EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?))
				OR (billable_resources.type = ? AND EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?))
			`, models.BillableTypeProject, models.StatusDeleting, models.BillableTypeDatabase, models.DBStatusDeleted)
		if userID != nil {
			countQuery = countQuery.Where("billable_resources.user_id = ?", *userID)
		}
		if err := countQuery.Distinct("billable_resources.id").Count(&total).Error; err != nil {
			return nil, 0, fmt.Errorf("count billing suspension resources: %w", err)
		}
		query = query.Offset((page - 1) * limit).Limit(limit)
	}

	type suspensionRow struct {
		ResourceID   uint
		ResourceType models.BillableType
		UserID       uint
		UserName     string
		UserEmail    string
		Status       models.BillableResourceStatus
		OldestDueAt  scannedTime
	}
	var rows []suspensionRow
	if err := query.
		Select("billable_resources.resource_id, billable_resources.type AS resource_type, billable_resources.user_id, COALESCE(users.name, '') AS user_name, COALESCE(users.email, '') AS user_email, billable_resources.billing_status AS status, MIN(invoices.due_at) AS oldest_due_at").
		Order("billable_resources.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list billing suspension resources: %w", err)
	}
	if page == 0 {
		total = int64(len(rows))
	}
	views := make([]SuspensionView, 0, len(rows))
	for _, row := range rows {
		var dueAt *time.Time
		if row.OldestDueAt.Valid {
			parsed := row.OldestDueAt.Time.UTC()
			dueAt = &parsed
		}
		view := SuspensionView{ResourceID: row.ResourceID, ResourceType: row.ResourceType, UserID: row.UserID, UserName: row.UserName, UserEmail: row.UserEmail, Status: row.Status, OldestDueAt: dueAt}
		if dueAt != nil && now.After(*dueAt) {
			view.PaymentDueDays = int(now.UTC().Sub(*dueAt) / (24 * time.Hour))
		}
		views = append(views, view)
	}
	return views, total, nil
}

type SuspensionView struct {
	ResourceID     uint                          `json:"resource_id"`
	ResourceType   models.BillableType           `json:"resource_type"`
	UserID         uint                          `json:"user_id"`
	UserName       string                        `json:"user_name"`
	UserEmail      string                        `json:"user_email"`
	Status         models.BillableResourceStatus `json:"status"`
	OldestDueAt    *time.Time                    `json:"oldest_due_at,omitempty"`
	PaymentDueDays int                           `json:"payment_due_days"`
}
