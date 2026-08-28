package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvoiceServiceUnavailable = errors.New("invoice service unavailable")
	ErrInvalidInvoiceInput       = errors.New("invalid invoice input")
	ErrResourceAlreadyBilled     = errors.New("billable resource already exists")
	ErrInsufficientCredits       = errors.New("insufficient wallet credits")
	ErrBillableResourceNotFound  = errors.New("billable resource not found")
	ErrResourcePaymentNotDue     = errors.New("resource payment is not due")
)

// InvoiceService owns billable-resource periods, invoice rows, and their wallet debits.
type InvoiceService struct {
	db      *gorm.DB
	wallets *WalletService
}

func NewInvoiceService(db *gorm.DB, wallets *WalletService) *InvoiceService {
	if wallets == nil {
		wallets = NewWalletService(db)
	}
	return &InvoiceService{db: db, wallets: wallets}
}

// ChargeInitialResourceTx creates the resource billing link and charges its first period.
// The caller must create the underlying project or database in the same transaction.
func (s *InvoiceService) ChargeInitialResourceTx(tx *gorm.DB, userID uint, resourceType models.BillableType, resourceID, specID uint, now time.Time) error {
	if err := s.validateTransaction(tx, userID, resourceType, resourceID, specID); err != nil {
		return err
	}
	// ponytail: Phase 5 charges a full fixed month from provisioning time; add proration only with explicit pricing rules.
	periodStart := now.UTC()
	anchorDay, anchorMonthEnd := billingAnchor(periodStart)
	periodEnd := nextBillingCycleWithAnchor(periodStart, anchorDay, anchorMonthEnd)
	resource := models.BillableResource{
		UserID:                userID,
		Type:                  resourceType,
		ResourceID:            resourceID,
		SpecID:                specID,
		BillingStatus:         models.BillableResourceStatusActive,
		CurrentPeriodStart:    periodStart,
		NextInvoiceAt:         periodEnd,
		BillingAnchorDay:      anchorDay,
		BillingAnchorMonthEnd: anchorMonthEnd,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&resource)
	if result.Error != nil {
		return fmt.Errorf("create billable resource: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrResourceAlreadyBilled
	}

	_, _, err := s.chargeResourceTx(tx, &resource, periodStart, periodEnd, true, now.UTC())
	return err
}

// RefundInitialResourceTx refunds credits debited for a resource's initial billing period
// when provisioning fails or is cancelled. It idempotently credits the user's wallet and
// marks the billable resource status as deleted.
func (s *InvoiceService) RefundInitialResourceTx(tx *gorm.DB, userID uint, resourceType models.BillableType, resourceID uint, reason string) error {
	if s == nil || s.wallets == nil {
		return ErrInvoiceServiceUnavailable
	}
	if tx == nil {
		return fmt.Errorf("transaction is required for resource refund")
	}

	var resource models.BillableResource
	if err := tx.Where("user_id = ? AND type = ? AND resource_id = ?", userID, resourceType, resourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load billable resource for refund: %w", err)
	}

	var item models.InvoiceItem
	if err := tx.Where("billable_resource_id = ?", resource.ID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load invoice item for refund: %w", err)
	}

	if item.Credits <= 0 {
		return nil
	}

	refundKey := fmt.Sprintf("invoice-refund:%d:%d", item.InvoiceID, item.ID)
	var existingCount int64
	if err := tx.Model(&models.WalletLedgerEntry{}).Where("idempotency_key = ?", refundKey).Count(&existingCount).Error; err != nil {
		return fmt.Errorf("check refund ledger entry: %w", err)
	}
	if existingCount > 0 {
		return nil
	}

	_, err := s.wallets.applyInTransaction(tx, LedgerMutation{
		UserID:         userID,
		EntryType:      models.WalletLedgerEntryInvoiceRefund,
		AmountCredits:  item.Credits,
		IdempotencyKey: refundKey,
		ReferenceType:  "invoice_item",
		ReferenceID:    fmt.Sprintf("%d", item.ID),
	}, true)
	if err != nil {
		return fmt.Errorf("credit wallet refund: %w", err)
	}

	if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
		return fmt.Errorf("update resource billing status on refund: %w", err)
	}

	return nil
}

// RunMonthly charges each due resource once. Insufficient balance marks the resource and
// invoice payment-due; a later scheduler run retries the same invoice item idempotently.
func (s *InvoiceService) RunMonthly(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil || s.wallets == nil || ctx == nil {
		return ErrInvoiceServiceUnavailable
	}
	return s.runMonthlyWithRecovery(ctx, s.db, now, nil)
}

func (s *InvoiceService) runMonthlyWithRecovery(ctx context.Context, db *gorm.DB, now time.Time, userID *uint) error {
	if db == nil {
		return ErrInvoiceServiceUnavailable
	}
	var runErr error
	if userID == nil {
		if err := s.restoreCurrentPeriodResourcesForAllUsersWithDB(ctx, db, now); err != nil {
			runErr = errors.Join(runErr, err)
		}
	} else if err := s.restoreCurrentPeriodResourcesWithDB(ctx, db, *userID, now); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if err := s.runMonthlyWithDB(ctx, db, now, userID); err != nil {
		runErr = errors.Join(runErr, err)
	}
	return runErr
}

// RetryDueForUser settles a user's due resources after their wallet is credited.
// The periodic scheduler remains the durable retry path when this best-effort signal fails.
func (s *InvoiceService) RetryDueForUser(ctx context.Context, userID uint, now time.Time) error {
	if s == nil || s.db == nil || s.wallets == nil || ctx == nil {
		return ErrInvoiceServiceUnavailable
	}
	if userID == 0 {
		return ErrInvalidInvoiceInput
	}
	return s.runMonthlyWithRecovery(ctx, s.db, now, &userID)
}

// PayDueResource settles one overdue resource without changing its auto-renew preference.
func (s *InvoiceService) PayDueResource(ctx context.Context, userID uint, resourceType models.BillableType, resourceID uint, now time.Time) error {
	if s == nil || s.db == nil || s.wallets == nil || ctx == nil {
		return ErrInvoiceServiceUnavailable
	}
	if userID == 0 || resourceID == 0 || (resourceType != models.BillableTypeProject && resourceType != models.BillableTypeDatabase) {
		return ErrInvalidInvoiceInput
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND type = ? AND resource_id = ?", userID, resourceType, resourceID).
			First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBillableResourceNotFound
			}
			return fmt.Errorf("lock billable resource for manual payment: %w", err)
		}
		if resource.BillingStatus == models.BillableResourceStatusActive {
			return nil
		}
		if resource.BillingStatus != models.BillableResourceStatusPaymentDue && resource.BillingStatus != models.BillableResourceStatusSuspended {
			return ErrResourcePaymentNotDue
		}

		exists, err := billableResourceExistsTx(tx, &resource)
		if err != nil {
			return err
		}
		if !exists {
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
				return fmt.Errorf("terminate missing billable resource: %w", err)
			}
			return ErrBillableResourceNotFound
		}

		var item models.InvoiceItem
		if err := tx.Select("invoice_items.*").
			Joins("JOIN invoices ON invoices.id = invoice_items.invoice_id").
			Where("invoice_items.billable_resource_id = ? AND invoice_items.credits > ? AND invoices.user_id = ? AND invoices.status = ?", resource.ID, 0, userID, models.InvoiceStatusPaymentDue).
			Order("invoices.period_start ASC, invoices.id ASC").
			First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrResourcePaymentNotDue
			}
			return fmt.Errorf("load overdue invoice item: %w", err)
		}

		var invoice models.Invoice
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ? AND status = ?", item.InvoiceID, userID, models.InvoiceStatusPaymentDue).
			First(&invoice).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrResourcePaymentNotDue
			}
			return fmt.Errorf("lock overdue invoice: %w", err)
		}

		_, err = s.wallets.applyInTransaction(tx, LedgerMutation{
			UserID:         userID,
			EntryType:      models.WalletLedgerEntryInvoiceDebit,
			AmountCredits:  item.Credits,
			IdempotencyKey: invoiceItemLedgerKey(invoice.ID, item.ID),
			ReferenceType:  "invoice_item",
			ReferenceID:    fmt.Sprintf("%d", item.ID),
		}, false)
		if errors.Is(err, ErrInsufficientBalance) {
			return ErrInsufficientCredits
		}
		if err != nil {
			return fmt.Errorf("debit overdue invoice item: %w", err)
		}
		if err := s.syncInvoiceStatusTx(tx, &invoice, now.UTC()); err != nil {
			return err
		}

		wasSuspended := resource.BillingStatus == models.BillableResourceStatusSuspended
		if err := tx.Model(&resource).Updates(map[string]any{
			"billing_status":       models.BillableResourceStatusActive,
			"current_period_start": invoice.PeriodStart,
			"next_invoice_at":      invoice.PeriodEnd,
		}).Error; err != nil {
			return fmt.Errorf("restore manually paid resource: %w", err)
		}
		if wasSuspended && resource.Type == models.BillableTypeDatabase {
			if err := ensureDatabaseStatusOperationTaskTx(tx, resource, models.DBStatusActive); err != nil {
				return fmt.Errorf("queue manually paid database resume: %w", err)
			}
		}
		if wasSuspended && resource.Type == models.BillableTypeProject {
			if err := completeProjectSuspensionTaskTx(tx, resource); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *InvoiceService) RestoreCurrentPeriodResources(ctx context.Context, userID uint, now time.Time) error {
	return s.restoreCurrentPeriodResourcesWithDB(ctx, s.db, userID, now)
}

func (s *InvoiceService) restoreCurrentPeriodResourcesWithDB(ctx context.Context, db *gorm.DB, userID uint, now time.Time) error {
	if db == nil {
		return ErrInvoiceServiceUnavailable
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var wallet models.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&wallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("lock wallet for payment-due recovery: %w", err)
		}
		if wallet.BalanceCredits < 0 {
			return nil
		}
		var candidateResourceIDs []uint
		if err := tx.Model(&models.BillableResource{}).
			Distinct("billable_resources.id").
			Joins("LEFT JOIN invoice_items ON invoice_items.billable_resource_id = billable_resources.id").
			Joins("LEFT JOIN invoices ON invoices.id = invoice_items.invoice_id").
			Where("billable_resources.user_id = ? AND billable_resources.next_invoice_at > ? AND (billable_resources.billing_status = ? OR (billable_resources.billing_status = ? AND invoices.status = ? AND invoices.total_credits = ?))", userID, now.UTC(), models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended, models.InvoiceStatusPaymentDue, 0).
			Pluck("billable_resources.id", &candidateResourceIDs).Error; err != nil {
			return fmt.Errorf("list current-period top-up reversal resources: %w", err)
		}
		if len(candidateResourceIDs) == 0 {
			return nil
		}
		var resources []models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&models.BillableResource{}).
			Where("id IN ?", candidateResourceIDs).
			Find(&resources).Error; err != nil {
			return fmt.Errorf("lock current-period top-up reversal resources: %w", err)
		}
		restoredResourceIDs := make([]uint, 0, len(resources))
		for _, resource := range resources {
			if resource.BillingStatus != models.BillableResourceStatusPaymentDue && resource.BillingStatus != models.BillableResourceStatusSuspended {
				continue
			}
			// A resource whose underlying project or database is already gone
			// must never be reactivated by wallet recovery. This races with
			// deletion: a project delete can land between the candidate query
			// and this lock.
			exists, existsErr := billableResourceExistsTx(tx, &resource)
			if existsErr != nil {
				return existsErr
			}
			if !exists {
				if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
					return fmt.Errorf("terminate missing billable resource %d: %w", resource.ID, err)
				}
				continue
			}
			wasSuspended := resource.BillingStatus == models.BillableResourceStatusSuspended
			if wasSuspended {
				var evidence models.Invoice
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Select("invoices.*").
					Joins("JOIN invoice_items ON invoice_items.invoice_id = invoices.id").
					Where("invoice_items.billable_resource_id = ? AND invoices.status = ? AND invoices.total_credits = ?", resource.ID, models.InvoiceStatusPaymentDue, 0).
					First(&evidence).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				if err != nil {
					return fmt.Errorf("lock suspended top-up reversal evidence for resource %d: %w", resource.ID, err)
				}
			}
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
				return fmt.Errorf("restore current-period payment-due resource %d: %w", resource.ID, err)
			}
			restoredResourceIDs = append(restoredResourceIDs, resource.ID)
			if resource.Type == models.BillableTypeProject {
				if err := completeProjectSuspensionTaskTx(tx, resource); err != nil {
					return err
				}
			}
			if wasSuspended && resource.Type == models.BillableTypeDatabase {
				if err := ensureDatabaseStatusOperationTaskTx(tx, resource, models.DBStatusActive); err != nil {
					return fmt.Errorf("queue current-period paid database resume: %w", err)
				}
			}
		}
		if len(restoredResourceIDs) > 0 {
			var invoiceIDs []uint
			if err := tx.Model(&models.Invoice{}).
				Joins("JOIN invoice_items ON invoice_items.invoice_id = invoices.id").
				Where("invoices.user_id = ? AND invoices.status = ? AND invoices.total_credits = ? AND invoice_items.billable_resource_id IN ?", userID, models.InvoiceStatusPaymentDue, 0, restoredResourceIDs).
				Distinct("invoices.id").
				Pluck("invoices.id", &invoiceIDs).Error; err != nil {
				return fmt.Errorf("list top-up reversal payment-due invoices: %w", err)
			}
			if len(invoiceIDs) == 0 {
				return nil
			}
			paidAt := now.UTC()
			if err := tx.Model(&models.Invoice{}).
				Where("id IN ?", invoiceIDs).
				Updates(map[string]any{"status": models.InvoiceStatusPaid, "paid_at": &paidAt}).Error; err != nil {
				return fmt.Errorf("settle top-up reversal payment-due invoices: %w", err)
			}
		}
		return nil
	})
}

func (s *InvoiceService) RestoreCurrentPeriodResourcesForAllUsers(ctx context.Context, now time.Time) error {
	return s.restoreCurrentPeriodResourcesForAllUsersWithDB(ctx, s.db, now)
}

func (s *InvoiceService) restoreCurrentPeriodResourcesForAllUsersWithDB(ctx context.Context, db *gorm.DB, now time.Time) error {
	if db == nil {
		return ErrInvoiceServiceUnavailable
	}
	var userIDs []uint
	if err := db.WithContext(ctx).Model(&models.BillableResource{}).
		Joins("JOIN wallets ON wallets.user_id = billable_resources.user_id").
		Where("next_invoice_at > ? AND billing_status IN ?", now.UTC(), []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}).
		Distinct().
		Pluck("billable_resources.user_id", &userIDs).Error; err != nil {
		return fmt.Errorf("list current-period payment recovery users: %w", err)
	}

	var runErr error
	for _, userID := range userIDs {
		if err := s.restoreCurrentPeriodResourcesWithDB(ctx, db, userID, now); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("restore current-period resources for user %d: %w", userID, err))
		}
	}
	return runErr
}

func (s *InvoiceService) runMonthlyWithDB(ctx context.Context, db *gorm.DB, now time.Time, userID *uint) error {
	if db == nil {
		return ErrInvoiceServiceUnavailable
	}
	now = now.UTC()
	query := db.WithContext(ctx).Model(&models.BillableResource{})
	if userID == nil {
		query = query.Where("next_invoice_at <= ? AND billing_status IN ?", now, []models.BillableResourceStatus{models.BillableResourceStatusActive, models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended})
	} else {
		query = query.Where("billing_status IN ? OR (next_invoice_at <= ? AND billing_status = ?)", []models.BillableResourceStatus{models.BillableResourceStatusPaymentDue, models.BillableResourceStatusSuspended}, now, models.BillableResourceStatusActive)
	}
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	var resourceIDs []uint
	if err := query.Order("id ASC").Pluck("id", &resourceIDs).Error; err != nil {
		return fmt.Errorf("list due billable resources: %w", err)
	}
	var runErr error
	for _, resourceID := range resourceIDs {
		if err := s.chargeDueResource(ctx, db, resourceID, now); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("charge billable resource %d: %w", resourceID, err))
		}
	}
	return runErr
}

func (s *InvoiceService) chargeDueResource(ctx context.Context, db *gorm.DB, resourceID uint, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&resource, resourceID).Error; err != nil {
			return fmt.Errorf("lock due billable resource: %w", err)
		}
		if resource.NextInvoiceAt.After(now) || (resource.BillingStatus != models.BillableResourceStatusActive && resource.BillingStatus != models.BillableResourceStatusPaymentDue && resource.BillingStatus != models.BillableResourceStatusSuspended) {
			return nil
		}
		exists, err := billableResourceExistsTx(tx, &resource)
		if err != nil {
			return err
		}
		if !exists {
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
				return fmt.Errorf("terminate deleted billable resource: %w", err)
			}
			return nil
		}
		if resource.BillingAnchorDay < 1 || resource.BillingAnchorDay > 31 {
			return ErrInvalidInvoiceInput
		}
		periodStart := resource.NextInvoiceAt.UTC()
		periodEnd := nextBillingCycleWithAnchor(periodStart, resource.BillingAnchorDay, resource.BillingAnchorMonthEnd)

		if !resource.AutoRenew {
			// Auto-renew is disabled: per contract, do not debit wallet automatically.
			// Create/ensure the invoice item for this period and move the resource to payment_due.
			var spec models.BillableSpec
			if err := tx.Where("id = ? AND type = ?", resource.SpecID, resource.Type).First(&spec).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInvalidInvoiceInput
				}
				return fmt.Errorf("load billable spec for non-renewing resource: %w", err)
			}
			if spec.MonthlyCredits <= 0 {
				return ErrInvalidInvoiceInput
			}

			wallet, err := getOrCreateLockedWallet(tx, resource.UserID)
			if err != nil {
				return err
			}
			invoice, err := findOrCreateInvoice(tx, resource.UserID, wallet.ID, periodStart, periodEnd)
			if err != nil {
				return err
			}
			item, created, err := findOrCreateInvoiceItem(tx, invoice.ID, &resource, &spec)
			if err != nil {
				return err
			}
			if created {
				if invoice.TotalCredits > math.MaxInt64-item.Credits {
					return ErrInvalidInvoiceInput
				}
				invoice.TotalCredits += item.Credits
				if err := tx.Model(&invoice).Update("total_credits", invoice.TotalCredits).Error; err != nil {
					return fmt.Errorf("update invoice total: %w", err)
				}
			}

			updates := map[string]any{"status": models.InvoiceStatusPaymentDue, "paid_at": nil}
			if invoice.DueAt == nil {
				dueAt := now
				updates["due_at"] = &dueAt
			}
			if err := tx.Model(&invoice).Updates(updates).Error; err != nil {
				return fmt.Errorf("mark invoice payment due: %w", err)
			}
			if resource.BillingStatus != models.BillableResourceStatusPaymentDue {
				if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusPaymentDue).Error; err != nil {
					return fmt.Errorf("mark resource payment due: %w", err)
				}
			}
			return nil
		}

		invoice, _, err := s.chargeResourceTx(tx, &resource, periodStart, periodEnd, false, now)
		if errors.Is(err, ErrInsufficientCredits) {
			updates := map[string]any{"status": models.InvoiceStatusPaymentDue, "paid_at": nil}
			if invoice.DueAt == nil {
				dueAt := now
				updates["due_at"] = &dueAt
			}
			if err := tx.Model(&invoice).Updates(updates).Error; err != nil {
				return fmt.Errorf("mark invoice payment due: %w", err)
			}
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusPaymentDue).Error; err != nil {
				return fmt.Errorf("mark resource payment due: %w", err)
			}
			return nil
		}
		if err != nil {
			return err
		}
		wasSuspended := resource.BillingStatus == models.BillableResourceStatusSuspended
		if err := tx.Model(&resource).Updates(map[string]any{
			"billing_status":       models.BillableResourceStatusActive,
			"current_period_start": periodStart,
			"next_invoice_at":      periodEnd,
		}).Error; err != nil {
			return fmt.Errorf("advance billable resource period: %w", err)
		}
		if wasSuspended && resource.Type == models.BillableTypeDatabase {
			if err := ensureDatabaseStatusOperationTaskTx(tx, resource, models.DBStatusActive); err != nil {
				return fmt.Errorf("queue paid database resume: %w", err)
			}
		}
		if wasSuspended && resource.Type == models.BillableTypeProject {
			if err := completeProjectSuspensionTaskTx(tx, resource); err != nil {
				return err
			}
		}
		return nil
	})
}

func completeProjectSuspensionTaskTx(tx *gorm.DB, resource models.BillableResource) error {
	if tx == nil || resource.Type != models.BillableTypeProject {
		return nil
	}
	now := time.Now().UTC()
	var task models.ProjectSuspensionTask
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NULL", resource.ResourceID, resource.ID, resource.UserID).
		First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock paid project suspension task: %w", err)
	}
	updates := map[string]any{"completed_at": &now}
	if task.StopAttemptedAt != nil && (task.MainWasRunning || task.WorkerWasRunning) {
		updates["resume_requested_at"] = &now
		updates["resume_completed_at"] = nil
	}
	if err := tx.Model(&task).Updates(updates).Error; err != nil {
		return fmt.Errorf("complete paid project suspension task: %w", err)
	}
	return nil
}

func billableResourceExistsTx(tx *gorm.DB, resource *models.BillableResource) (bool, error) {
	var count int64
	switch resource.Type {
	case models.BillableTypeProject:
		if err := tx.Model(&models.Project{}).Where("id = ? AND status <> ?", resource.ResourceID, models.StatusDeleting).Count(&count).Error; err != nil {
			return false, fmt.Errorf("check project billable resource: %w", err)
		}
	case models.BillableTypeDatabase:
		if err := tx.Model(&models.DatabaseInstance{}).Where("id = ? AND status <> ?", resource.ResourceID, models.DBStatusDeleted).Count(&count).Error; err != nil {
			return false, fmt.Errorf("check database billable resource: %w", err)
		}
	default:
		return false, ErrInvalidInvoiceInput
	}
	return count == 1, nil
}

func (s *InvoiceService) chargeResourceTx(tx *gorm.DB, resource *models.BillableResource, periodStart, periodEnd time.Time, requireActiveSpec bool, now time.Time) (models.Invoice, models.InvoiceItem, error) {
	if s == nil || s.wallets == nil || tx == nil || resource == nil || resource.ID == 0 || !periodEnd.After(periodStart) {
		return models.Invoice{}, models.InvoiceItem{}, ErrInvalidInvoiceInput
	}
	var spec models.BillableSpec
	specQuery := tx.Where("id = ? AND type = ?", resource.SpecID, resource.Type)
	if requireActiveSpec {
		specQuery = specQuery.Where("is_active = ?", true)
	}
	if err := specQuery.First(&spec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Invoice{}, models.InvoiceItem{}, ErrInvalidInvoiceInput
		}
		return models.Invoice{}, models.InvoiceItem{}, fmt.Errorf("load billable spec: %w", err)
	}
	if spec.MonthlyCredits <= 0 {
		return models.Invoice{}, models.InvoiceItem{}, ErrInvalidInvoiceInput
	}

	wallet, err := getOrCreateLockedWallet(tx, resource.UserID)
	if err != nil {
		return models.Invoice{}, models.InvoiceItem{}, err
	}
	invoice, err := findOrCreateInvoice(tx, resource.UserID, wallet.ID, periodStart, periodEnd)
	if err != nil {
		return models.Invoice{}, models.InvoiceItem{}, err
	}
	item, created, err := findOrCreateInvoiceItem(tx, invoice.ID, resource, &spec)
	if err != nil {
		return models.Invoice{}, models.InvoiceItem{}, err
	}
	if created {
		if invoice.TotalCredits > math.MaxInt64-item.Credits {
			return models.Invoice{}, models.InvoiceItem{}, ErrInvalidInvoiceInput
		}
		invoice.TotalCredits += item.Credits
		if err := tx.Model(&invoice).Update("total_credits", invoice.TotalCredits).Error; err != nil {
			return models.Invoice{}, models.InvoiceItem{}, fmt.Errorf("update invoice total: %w", err)
		}
	}

	_, err = s.wallets.applyInTransaction(tx, LedgerMutation{
		UserID:         resource.UserID,
		EntryType:      models.WalletLedgerEntryInvoiceDebit,
		AmountCredits:  item.Credits,
		IdempotencyKey: invoiceItemLedgerKey(invoice.ID, item.ID),
		ReferenceType:  "invoice_item",
		ReferenceID:    fmt.Sprintf("%d", item.ID),
	}, false)
	if errors.Is(err, ErrInsufficientBalance) {
		return invoice, item, ErrInsufficientCredits
	}
	if err != nil {
		return models.Invoice{}, models.InvoiceItem{}, err
	}
	if err := s.syncInvoiceStatusTx(tx, &invoice, now); err != nil {
		return models.Invoice{}, models.InvoiceItem{}, err
	}
	return invoice, item, nil
}

func findOrCreateInvoice(tx *gorm.DB, userID, walletID uint, periodStart, periodEnd time.Time) (models.Invoice, error) {
	invoice := models.Invoice{
		UserID:         userID,
		WalletID:       walletID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		Status:         models.InvoiceStatusPending,
		IdempotencyKey: invoiceKey(userID, periodStart, periodEnd),
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&invoice)
	if result.Error != nil {
		return models.Invoice{}, fmt.Errorf("create invoice: %w", result.Error)
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND period_start = ? AND period_end = ?", userID, periodStart, periodEnd).First(&invoice).Error; err != nil {
		return models.Invoice{}, fmt.Errorf("lock invoice: %w", err)
	}
	if invoice.WalletID != walletID {
		return models.Invoice{}, ErrInvalidInvoiceInput
	}
	return invoice, nil
}

func findOrCreateInvoiceItem(tx *gorm.DB, invoiceID uint, resource *models.BillableResource, spec *models.BillableSpec) (models.InvoiceItem, bool, error) {
	resourceName := ""
	switch resource.Type {
	case models.BillableTypeProject:
		var proj models.Project
		if err := tx.Select("id, name").Where("id = ?", resource.ResourceID).First(&proj).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return models.InvoiceItem{}, false, fmt.Errorf("lookup project snapshot name: %w", err)
			}
		} else {
			resourceName = proj.Name
		}
	case models.BillableTypeDatabase:
		var dbInst models.DatabaseInstance
		if err := tx.Select("id, name").Where("id = ?", resource.ResourceID).First(&dbInst).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return models.InvoiceItem{}, false, fmt.Errorf("lookup database snapshot name: %w", err)
			}
		} else {
			resourceName = dbInst.Name
		}
	}
	specName := spec.Name

	item := models.InvoiceItem{
		InvoiceID:          invoiceID,
		BillableResourceID: resource.ID,
		SpecID:             spec.ID,
		ResourceName:       resourceName,
		SpecName:           specName,
		Description:        fmt.Sprintf("%s resource %d monthly credits", resource.Type, resource.ResourceID),
		Credits:            spec.MonthlyCredits,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
	if result.Error != nil {
		return models.InvoiceItem{}, false, fmt.Errorf("create invoice item: %w", result.Error)
	}
	created := result.RowsAffected > 0
	if err := tx.Where("invoice_id = ? AND billable_resource_id = ?", invoiceID, resource.ID).First(&item).Error; err != nil {
		return models.InvoiceItem{}, false, fmt.Errorf("load invoice item: %w", err)
	}
	if item.SpecID != spec.ID || item.Credits != spec.MonthlyCredits {
		return models.InvoiceItem{}, false, ErrInvalidInvoiceInput
	}
	return item, created, nil
}

func (s *InvoiceService) syncInvoiceStatusTx(tx *gorm.DB, invoice *models.Invoice, now time.Time) error {
	var items []models.InvoiceItem
	if err := tx.Where("invoice_id = ?", invoice.ID).Find(&items).Error; err != nil {
		return fmt.Errorf("load invoice items: %w", err)
	}
	if len(items) == 0 {
		return ErrInvalidInvoiceInput
	}
	for _, item := range items {
		var count int64
		if err := tx.Model(&models.WalletLedgerEntry{}).Where("idempotency_key = ?", invoiceItemLedgerKey(invoice.ID, item.ID)).Count(&count).Error; err != nil {
			return fmt.Errorf("check invoice ledger entry: %w", err)
		}
		if count == 0 {
			return nil
		}
	}
	paidAt := now
	if err := tx.Model(invoice).Updates(map[string]any{"status": models.InvoiceStatusPaid, "paid_at": &paidAt}).Error; err != nil {
		return fmt.Errorf("mark invoice paid: %w", err)
	}
	invoice.Status = models.InvoiceStatusPaid
	invoice.PaidAt = &paidAt
	return nil
}

func nextBillingCycle(periodStart time.Time) time.Time {
	anchorDay, anchorMonthEnd := billingAnchor(periodStart)
	return nextBillingCycleWithAnchor(periodStart, anchorDay, anchorMonthEnd)
}

func nextBillingCycleWithAnchor(periodStart time.Time, anchorDay int, anchorMonthEnd bool) time.Time {
	periodStart = periodStart.UTC()
	if anchorDay < 1 || anchorDay > 31 {
		return time.Time{}
	}
	lastDay := lastDayOfMonth(time.Date(periodStart.Year(), periodStart.Month()+1, 1, periodStart.Hour(), periodStart.Minute(), periodStart.Second(), periodStart.Nanosecond(), time.UTC))
	day := anchorDay
	if anchorMonthEnd || day > lastDay {
		day = lastDay
	}
	return time.Date(periodStart.Year(), periodStart.Month()+1, day, periodStart.Hour(), periodStart.Minute(), periodStart.Second(), periodStart.Nanosecond(), time.UTC)
}

func billingAnchor(periodStart time.Time) (int, bool) {
	periodStart = periodStart.UTC()
	return periodStart.Day(), periodStart.Day() == lastDayOfMonth(periodStart)
}

func lastDayOfMonth(value time.Time) int {
	return time.Date(value.Year(), value.Month()+1, 0, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), time.UTC).Day()
}

func (s *InvoiceService) validateTransaction(tx *gorm.DB, userID uint, resourceType models.BillableType, resourceID, specID uint) error {
	if s == nil || s.db == nil || s.wallets == nil || tx == nil {
		return ErrInvoiceServiceUnavailable
	}
	if userID == 0 || resourceID == 0 || specID == 0 || (resourceType != models.BillableTypeProject && resourceType != models.BillableTypeDatabase) {
		return ErrInvalidInvoiceInput
	}
	return nil
}

func invoiceKey(userID uint, periodStart, periodEnd time.Time) string {
	return fmt.Sprintf("invoice:%d:%s:%s", userID, periodStart.UTC().Format(time.RFC3339Nano), periodEnd.UTC().Format(time.RFC3339Nano))
}

func invoiceItemLedgerKey(invoiceID, itemID uint) string {
	return fmt.Sprintf("invoice:%d:item:%d", invoiceID, itemID)
}
