package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
)

const invoiceScheduleInterval = time.Minute

const invoiceSchedulerLockIdentity = "runara:billing-invoice-scheduler"

// InvoiceScheduler periodically retries due resource invoices.
type InvoiceScheduler struct {
	invoices *InvoiceService
	topups   *TopupService
}

func NewInvoiceScheduler(invoices *InvoiceService, topups ...*TopupService) *InvoiceScheduler {
	scheduler := &InvoiceScheduler{invoices: invoices}
	if len(topups) > 0 {
		scheduler.topups = topups[0]
	}
	return scheduler
}

func (s *InvoiceScheduler) Run(ctx context.Context) {
	if s == nil || s.invoices == nil || ctx == nil {
		return
	}
	s.runOnce(ctx)
	ticker := time.NewTicker(invoiceScheduleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *InvoiceScheduler) runOnce(ctx context.Context) {
	if s.invoices.db == nil {
		return
	}
	if s.invoices.db.Dialector.Name() != "postgres" {
		if err := s.invoices.RunMonthly(ctx, time.Now().UTC()); err != nil {
			slog.Error("Billing invoice scheduler failed", "error", err)
		}
		s.sweepStaleTopups(ctx)
		return
	}
	if err := s.invoices.db.Connection(func(connection *gorm.DB) (runErr error) {
		var acquired bool
		if err := connection.Raw("SELECT pg_try_advisory_lock(hashtextextended(?, 0))", invoiceSchedulerLockIdentity).Scan(&acquired).Error; err != nil {
			return fmt.Errorf("acquire invoice scheduler lock: %w", err)
		}
		if !acquired {
			return nil
		}
		defer func() {
			if err := connection.Exec("SELECT pg_advisory_unlock(hashtextextended(?, 0))", invoiceSchedulerLockIdentity).Error; err != nil && runErr == nil {
				runErr = fmt.Errorf("release invoice scheduler lock: %w", err)
			}
		}()
		return s.invoices.runMonthlyWithRecovery(ctx, connection, time.Now().UTC(), nil)
	}); err != nil {
		slog.Error("Billing invoice scheduler failed", "error", err)
	}
	s.sweepStaleTopups(ctx)
}

// sweepStaleTopups recovers checkouts whose creating process died mid-request.
// Failures are logged and retried on the next tick; stale top-ups are never
// lost because the reconcile endpoint remains as a user-triggered fallback.
func (s *InvoiceScheduler) sweepStaleTopups(ctx context.Context) {
	if s.topups == nil {
		return
	}
	if err := s.topups.RecoverStaleTopups(ctx, time.Now().UTC()); err != nil {
		slog.Error("Stale top-up recovery failed", "error", err)
	}
}
