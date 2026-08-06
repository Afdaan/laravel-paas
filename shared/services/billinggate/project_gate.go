package billinggate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

var (
	ErrProjectActionBlocked          = errors.New("project action blocked by overdue billing")
	ErrPaymentDueEvidenceUnavailable = errors.New("payment-due resource has no open invoice evidence")
)

// ProjectRuntimeGate prevents runtime-creating actions after the configured overdue age.
// It intentionally does not stop already-running containers; suspension is handled separately.
type ProjectRuntimeGate struct {
	db        *gorm.DB
	enabled   bool
	blockDays int
}

func NewProjectRuntimeGate(db *gorm.DB, enabled bool, blockDays int) *ProjectRuntimeGate {
	return &ProjectRuntimeGate{db: db, enabled: enabled, blockDays: blockDays}
}

func (g *ProjectRuntimeGate) Check(ctx context.Context, projectID uint, now time.Time) error {
	return g.CheckResource(ctx, models.BillableTypeProject, projectID, now)
}

func (g *ProjectRuntimeGate) CheckResource(ctx context.Context, resourceType models.BillableType, resourceID uint, now time.Time) error {
	if g == nil || !g.enabled || resourceID == 0 {
		return nil
	}
	if g.db == nil || g.blockDays <= 0 {
		return fmt.Errorf("billing runtime gate unavailable")
	}
	if resourceType != models.BillableTypeProject && resourceType != models.BillableTypeDatabase {
		return fmt.Errorf("unsupported billable resource type")
	}

	var resource models.BillableResource
	err := g.db.WithContext(ctx).
		Where("type = ? AND resource_id = ?", resourceType, resourceID).
		First(&resource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load project billing resource: %w", err)
	}
	if resource.BillingStatus == models.BillableResourceStatusSuspended {
		return ErrProjectActionBlocked
	}
	if resource.BillingStatus != models.BillableResourceStatusPaymentDue {
		return nil
	}

	dueAt, err := OldestOpenInvoiceDueAt(ctx, g.db, resource.ID)
	if err != nil {
		return err
	}
	if dueAt == nil {
		return ErrPaymentDueEvidenceUnavailable
	}
	if !now.UTC().Before(dueAt.UTC().AddDate(0, 0, g.blockDays)) {
		return ErrProjectActionBlocked
	}
	return nil
}

// OldestOpenInvoiceDueAt is the durable payment-due clock for a resource.
// next_invoice_at is deliberately excluded because it advances only after payment.
func OldestOpenInvoiceDueAt(ctx context.Context, db *gorm.DB, billableResourceID uint) (*time.Time, error) {
	if db == nil || billableResourceID == 0 {
		return nil, fmt.Errorf("billing invoice evidence unavailable")
	}
	var invoice models.Invoice
	err := db.WithContext(ctx).
		Model(&models.Invoice{}).
		Select("invoices.*").
		Joins("JOIN invoice_items ON invoice_items.invoice_id = invoices.id").
		Where("invoice_items.billable_resource_id = ? AND invoices.status = ? AND invoices.due_at IS NOT NULL", billableResourceID, models.InvoiceStatusPaymentDue).
		Order("invoices.due_at ASC, invoices.id ASC").
		First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load oldest unpaid invoice due time: %w", err)
	}
	return invoice.DueAt, nil
}
