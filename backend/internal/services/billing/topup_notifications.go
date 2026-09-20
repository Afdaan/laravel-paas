// ===========================================
// Topup Notifications & Webhook Handlers
// ===========================================
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/laravel-paas/backend/internal/services/billing/telegram"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type topupReconcileCtxKey struct{}

// ProcessNotification applies a validated Midtrans payment notification. It is
// safe for duplicate deliveries: payment events are deduplicated by payload
// hash and wallet mutations are idempotent.
func (s *TopupService) ProcessNotification(ctx context.Context, notification MidtransNotification) error {
	return s.processNotification(ctx, notification)
}

// ProcessReconciledNotification is ProcessNotification for notifications that
// were just fetched from the Midtrans transaction status API by Reconcile. The
// status API carries the authoritative cumulative refund amount.
func (s *TopupService) ProcessReconciledNotification(ctx context.Context, notification MidtransNotification) error {
	return s.processNotification(context.WithValue(ctx, topupReconcileCtxKey{}, true), notification)
}

func (s *TopupService) processNotification(ctx context.Context, notification MidtransNotification) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	validated, err := s.validateNotification(notification)
	if err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(validated)
	if err != nil {
		return fmt.Errorf("marshal validated payment event: %w", err)
	}
	eventKey := paymentEventKey(payloadJSON)

	// The provider status endpoint carries the authoritative cumulative refund
	// amount. Cross-check every reversal webhook before touching the wallet.
	if isReversalTopupStatus(validated.Status) && ctx.Value(topupReconcileCtxKey{}) == nil {
		crosscheckCtx, cancel := context.WithTimeout(ctx, midtransRequestTimeout)
		latest, fetchErr := s.gateway.GetTransactionStatus(crosscheckCtx, validated.OrderID)
		cancel()
		if fetchErr != nil {
			return fmt.Errorf("cross-check reversal notification: %w", fetchErr)
		}
		if _, statusErr := topupStatusFromNotification(latest.StatusCode, latest.TransactionStatus, latest.FraudStatus); statusErr != nil {
			return fmt.Errorf("cross-check reversal notification: %w", statusErr)
		}
		revalidated, validationErr := s.validateNotification(latest)
		if validationErr != nil {
			return fmt.Errorf("cross-check reversal notification: %w", validationErr)
		}
		if revalidated.OrderID != validated.OrderID || revalidated.TransactionID != validated.TransactionID || !isReversalTopupStatus(revalidated.Status) {
			return ErrInvalidPaymentNotification
		}
		validated = revalidated
		payloadJSON, err = json.Marshal(validated)
		if err != nil {
			return fmt.Errorf("marshal cross-checked payment event: %w", err)
		}
		eventKey = paymentEventKey(payloadJSON)
	}

	var creditedUserID uint
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event models.PaymentEvent
		result := tx.Session(&gorm.Session{}).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_key"}},
			DoNothing: true,
		}).Create(&models.PaymentEvent{
			Provider:        models.BillingProviderMidtrans,
			EventKey:        eventKey,
			ProviderOrderID: validated.OrderID,
			PayloadJSON:     string(payloadJSON),
		})
		if result.Error != nil {
			return fmt.Errorf("record payment event: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Session(&gorm.Session{}).Where("event_key = ?", eventKey).First(&event).Error; err != nil {
			return fmt.Errorf("load recorded payment event: %w", err)
		}

		var topupRef struct {
			ID       uint
			WalletID uint
		}
		if err := tx.Session(&gorm.Session{}).Model(&models.Topup{}).Select("id, wallet_id").Where("provider = ? AND provider_order_id = ?", models.BillingProviderMidtrans, validated.OrderID).First(&topupRef).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupNotFound
			}
			return fmt.Errorf("lookup topup reference: %w", err)
		}

		// Global Lock Hierarchy: Lock Wallet first, then Topup, then BillableResource, then Invoice.
		var lockedWallet models.Wallet
		if err := tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedWallet, topupRef.WalletID).Error; err != nil {
			return fmt.Errorf("lock wallet for payment notification: %w", err)
		}

		var topup models.Topup
		if err := tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}).First(&topup, topupRef.ID).Error; err != nil {
			return fmt.Errorf("lock topup for payment notification: %w", err)
		}

		if topup.AmountMinor != validated.AmountMinor || (validated.Currency != "" && topup.Currency != validated.Currency) {
			return ErrInvalidPaymentNotification
		}
		if topup.ProviderTransactionID != nil && *topup.ProviderTransactionID != "" && validated.TransactionID != "" && *topup.ProviderTransactionID != validated.TransactionID {
			return ErrInvalidPaymentNotification
		}

		updates := map[string]any{
			"status": validated.Status,
		}
		if validated.TransactionID != "" {
			updates["provider_transaction_id"] = validated.TransactionID
		}
		switch validated.Status {
		case models.TopupStatusPaid:
			if topup.Status != models.TopupStatusPartialRefund && topup.Status != models.TopupStatusRefunded && topup.Status != models.TopupStatusChargeback {
				userID, err := loadWalletUserID(tx, topup.WalletID)
				if err != nil {
					return err
				}
				if _, err := s.wallets.applyInTransaction(tx, LedgerMutation{
					UserID:         userID,
					EntryType:      models.WalletLedgerEntryTopup,
					AmountCredits:  topup.Credits,
					IdempotencyKey: fmt.Sprintf("topup:%s:credit", topup.ProviderOrderID),
					ReferenceType:  "topup",
					ReferenceID:    strconv.FormatUint(uint64(topup.ID), 10),
				}, true); err != nil {
					return err
				}
				if topup.Status != models.TopupStatusPaid {
					creditedUserID = userID
				}
				if topup.Status != models.TopupStatusPaid {
					now := time.Now().UTC()
					updates["paid_at"] = &now
				}
			} else {
				updates["status"] = topup.Status
			}
		case models.TopupStatusPartialRefund, models.TopupStatusRefunded, models.TopupStatusPartialChargeback, models.TopupStatusChargeback:
			if err := s.applyTopupReversalTx(tx, &topup, validated); err != nil {
				return err
			}
			if (topup.Status == models.TopupStatusRefunded || topup.Status == models.TopupStatusChargeback) && (validated.Status == models.TopupStatusPartialRefund || validated.Status == models.TopupStatusPartialChargeback) {
				updates["status"] = topup.Status
			}
		case models.TopupStatusPending:
			if topup.Status != models.TopupStatusPending {
				updates["status"] = topup.Status
			}
		default:
			if topup.Status == models.TopupStatusPaid || topup.Status == models.TopupStatusPartialRefund || topup.Status == models.TopupStatusRefunded || topup.Status == models.TopupStatusChargeback {
				updates["status"] = topup.Status
			}
		}
		if isTerminalTopupStatus(updates["status"].(models.TopupStatus)) {
			updates["provider_request_state"] = providerRequestTerminal
		}
		if err := tx.Model(&topup).Updates(updates).Error; err != nil {
			return fmt.Errorf("update topup payment status: %w", err)
		}
		if err := tx.Model(&event).Update("processed_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("mark payment event processed: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if creditedUserID != 0 {
		s.retryDueInvoices(ctx, creditedUserID)
	}

	if s.telegramNotifier != nil {
		var topup models.Topup
		if err := s.db.WithContext(ctx).Where("provider = ? AND provider_order_id = ?", models.BillingProviderMidtrans, validated.OrderID).First(&topup).Error; err == nil {
			userID, _ := loadWalletUserID(s.db, topup.WalletID)
			userEmail := loadUserEmail(s.db, userID)
			currentBal, hasBal := loadWalletBalance(s.db, topup.WalletID)
			notifCredits := topup.Credits
			if isReversalTopupStatus(validated.Status) {
				if rev, revErr := desiredTopupReversalCredits(&topup, validated.Status, validated.RefundAmountMinor); revErr == nil {
					notifCredits = rev
				}
			}
			balBefore, balAfter := calculateBalanceBeforeAfter(validated.Status, currentBal, notifCredits)

			s.telegramNotifier.SendTopupNotification(telegram.NotificationMessage{
				OrderID:        validated.OrderID,
				UserEmail:      userEmail,
				UserID:         userID,
				AmountMinor:    validated.AmountMinor,
				Currency:       validated.Currency,
				Credits:        notifCredits,
				BalanceBefore:  balBefore,
				BalanceAfter:   balAfter,
				HasBalanceInfo: hasBal,
				Provider:       models.BillingProviderMidtrans,
				Status:         validated.Status,
				PaidAt:         topup.PaidAt,
			})
		}
	}

	return nil
}

func (s *TopupService) ProcessPakasirWebhook(ctx context.Context, orderID string, amountMinor int64, webhookStatus string) error {
	if s.cfg != nil && (!s.cfg.BillingTopupEnabled || !s.cfg.PakasirEnabled) {
		return ErrTopupDisabled
	}
	if orderID == "" || amountMinor <= 0 {
		return ErrInvalidPaymentNotification
	}

	// 1. Fast local verification: ensure order exists and amount matches before calling outbound provider API
	var localTopup models.Topup
	if err := s.db.WithContext(ctx).Where("provider = ? AND provider_order_id = ?", models.BillingProviderPakasir, orderID).First(&localTopup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTopupNotFound
		}
		return fmt.Errorf("lookup topup for pakasir notification: %w", err)
	}
	if localTopup.AmountMinor != amountMinor {
		return ErrInvalidPaymentNotification
	}
	if localTopup.Status == models.TopupStatusPaid {
		return nil
	}

	if s.pakasirGateway == nil {
		return ErrPaymentProvider
	}

	detail, err := s.pakasirGateway.GetTransactionDetail(ctx, orderID, amountMinor)
	if err != nil {
		return fmt.Errorf("%w: verify transaction detail with pakasir: %v", ErrPaymentProvider, err)
	}

	if detail.OrderID != orderID || detail.Amount != amountMinor {
		return ErrInvalidPaymentNotification
	}

	status, err := pakAsirStatusToTopupStatus(detail.Status)
	if err != nil {
		return ErrInvalidPaymentNotification
	}

	payloadJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal pakasir payment event: %w", err)
	}
	eventKey := fmt.Sprintf("pakasir:%s:%s", orderID, detail.Status)

	var creditedUserID uint
	var wasTransitioned bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		event := models.PaymentEvent{
			Provider:        models.BillingProviderPakasir,
			EventKey:        eventKey,
			ProviderOrderID: orderID,
			PayloadJSON:     string(payloadJSON),
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_key"}},
			DoNothing: true,
		}).Create(&event)
		if result.Error != nil {
			return fmt.Errorf("record payment event: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}

		var topupRef struct {
			ID       uint
			WalletID uint
		}
		if err := tx.Model(&models.Topup{}).Select("id, wallet_id").Where("provider = ? AND provider_order_id = ?", models.BillingProviderPakasir, orderID).First(&topupRef).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupNotFound
			}
			return fmt.Errorf("lookup topup reference: %w", err)
		}

		// Global Lock Hierarchy: Lock Wallet first, then Topup, then BillableResource, then Invoice.
		var lockedWallet models.Wallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedWallet, topupRef.WalletID).Error; err != nil {
			return fmt.Errorf("lock wallet for pakasir notification: %w", err)
		}

		var topup models.Topup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&topup, topupRef.ID).Error; err != nil {
			return fmt.Errorf("lock topup for pakasir notification: %w", err)
		}
		if topup.AmountMinor != amountMinor {
			return ErrInvalidPaymentNotification
		}

		if topup.Status == models.TopupStatusPaid {
			return nil
		}

		switch status {
		case models.TopupStatusPaid:
			topup.Status = models.TopupStatusPaid
			now := time.Now().UTC()
			topup.PaidAt = &now
			topup.ProviderRequestState = providerRequestTerminal
			if err := tx.Save(&topup).Error; err != nil {
				return fmt.Errorf("save paid topup: %w", err)
			}
			walletUserID, err := loadWalletUserID(tx, topup.WalletID)
			if err != nil {
				return err
			}
			idempotencyKey := fmt.Sprintf("topup:%s:%s", models.BillingProviderPakasir, topup.ProviderOrderID)
			topupIDStr := strconv.FormatUint(uint64(topup.ID), 10)
			if _, err := s.wallets.applyInTransaction(tx, LedgerMutation{
				UserID:         walletUserID,
				EntryType:      models.WalletLedgerEntryTopup,
				AmountCredits:  topup.Credits,
				IdempotencyKey: idempotencyKey,
				ReferenceType:  "topup",
				ReferenceID:    topupIDStr,
			}, true); err != nil {
				return fmt.Errorf("credit wallet balance: %w", err)
			}
			creditedUserID = walletUserID
			wasTransitioned = true
		case models.TopupStatusFailed, models.TopupStatusExpired:
			topup.Status = status
			topup.ProviderRequestState = providerRequestTerminal
			if err := tx.Save(&topup).Error; err != nil {
				return fmt.Errorf("save failed topup: %w", err)
			}
			wasTransitioned = true
		}
		if err := tx.Model(&event).Update("processed_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("mark payment event processed: %w", err)
		}
		return nil
	})

	if err != nil {
		return err
	}

	if creditedUserID != 0 {
		s.retryDueInvoices(ctx, creditedUserID)
	}

	if wasTransitioned && s.telegramNotifier != nil {
		var topup models.Topup
		if fetchErr := s.db.WithContext(ctx).Where("provider = ? AND provider_order_id = ?", models.BillingProviderPakasir, orderID).First(&topup).Error; fetchErr == nil {
			walletUserID, _ := loadWalletUserID(s.db, topup.WalletID)
			userEmail := loadUserEmail(s.db, walletUserID)
			currentBal, hasBal := loadWalletBalance(s.db, topup.WalletID)
			balBefore, balAfter := calculateBalanceBeforeAfter(topup.Status, currentBal, topup.Credits)

			s.telegramNotifier.SendTopupNotification(telegram.NotificationMessage{
				OrderID:        orderID,
				UserEmail:      userEmail,
				UserID:         walletUserID,
				AmountMinor:    topup.AmountMinor,
				Currency:       topup.Currency,
				Credits:        topup.Credits,
				BalanceBefore:  balBefore,
				BalanceAfter:   balAfter,
				HasBalanceInfo: hasBal,
				Provider:       models.BillingProviderPakasir,
				Status:         topup.Status,
				PaidAt:         topup.PaidAt,
			})
		}
	}

	return nil
}

func loadUserEmail(db *gorm.DB, userID uint) string {
	if db == nil || userID == 0 {
		return ""
	}
	var user models.User
	if err := db.Select("email").Where("id = ?", userID).First(&user).Error; err == nil {
		return user.Email
	}
	return ""
}

func loadWalletBalance(db *gorm.DB, walletID uint) (int64, bool) {
	if db == nil || walletID == 0 {
		return 0, false
	}
	var wallet models.Wallet
	if err := db.Select("balance_credits").Where("id = ?", walletID).First(&wallet).Error; err == nil {
		return wallet.BalanceCredits, true
	}
	return 0, false
}

func calculateBalanceBeforeAfter(status models.TopupStatus, currentBalance, credits int64) (int64, int64) {
	switch status {
	case models.TopupStatusPaid:
		return currentBalance - credits, currentBalance
	case models.TopupStatusRefunded, models.TopupStatusPartialRefund, models.TopupStatusChargeback, models.TopupStatusPartialChargeback:
		return currentBalance + credits, currentBalance
	default:
		return currentBalance, currentBalance
	}
}

func (s *TopupService) applyTopupReversalTx(tx *gorm.DB, topup *models.Topup, notification validatedPaymentNotification) error {
	if topup.Status != models.TopupStatusPaid && topup.Status != models.TopupStatusPartialRefund && topup.Status != models.TopupStatusRefunded && topup.Status != models.TopupStatusPartialChargeback && topup.Status != models.TopupStatusChargeback {
		return ErrInvalidPaymentNotification
	}
	desiredReversal, err := desiredTopupReversalCredits(topup, notification.Status, notification.RefundAmountMinor)
	if err != nil {
		return err
	}
	var existingReversal int64
	if err := tx.Session(&gorm.Session{}).Model(&models.WalletLedgerEntry{}).
		Select("COALESCE(SUM(-amount_credits), 0)").
		Where("wallet_id = ? AND type = ? AND reference_type = ? AND reference_id = ?", topup.WalletID, models.WalletLedgerEntryTopupReversal, "topup", strconv.FormatUint(uint64(topup.ID), 10)).
		Scan(&existingReversal).Error; err != nil {
		return fmt.Errorf("sum applied top-up reversals: %w", err)
	}
	if existingReversal > desiredReversal {
		// A provider event older than the reversal already applied must not
		// move money backwards. The immutable ledger remains authoritative.
		return nil
	}
	delta := desiredReversal - existingReversal
	if delta == 0 {
		return nil
	}

	userID, err := loadWalletUserID(tx, topup.WalletID)
	if err != nil {
		return err
	}
	// Global Lock Hierarchy: Apply Wallet debit (which locks Wallet) first, then lock active resources only if in debt.
	reversal, err := s.wallets.applyInTransaction(tx, LedgerMutation{
		UserID:         userID,
		EntryType:      models.WalletLedgerEntryTopupReversal,
		AmountCredits:  delta,
		IdempotencyKey: fmt.Sprintf("topup:%s:reversal:%d", topup.ProviderOrderID, desiredReversal),
		ReferenceType:  "topup",
		ReferenceID:    strconv.FormatUint(uint64(topup.ID), 10),
	}, false)
	if err != nil {
		return err
	}
	if reversal.BalanceAfter >= 0 {
		return nil
	}

	var activeResources []models.BillableResource
	if err := tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND billing_status = ?", userID, models.BillableResourceStatusActive).
		Find(&activeResources).Error; err != nil {
		return fmt.Errorf("lock active resources before top-up reversal: %w", err)
	}
	if len(activeResources) == 0 {
		return nil
	}
	now := time.Now().UTC()
	if err := recordTopupReversalPaymentDueTx(tx, topup, activeResources, now); err != nil {
		return err
	}
	resourceIDs := make([]uint, 0, len(activeResources))
	for _, resource := range activeResources {
		resourceIDs = append(resourceIDs, resource.ID)
	}
	if err := tx.Session(&gorm.Session{}).Model(&models.BillableResource{}).Where("id IN ?", resourceIDs).Update("billing_status", models.BillableResourceStatusPaymentDue).Error; err != nil {
		return fmt.Errorf("move overdue resources to payment due after top-up reversal: %w", err)
	}
	return nil
}

func desiredTopupReversalCredits(topup *models.Topup, status models.TopupStatus, refundAmountMinor int64) (int64, error) {
	if topup == nil || topup.Credits <= 0 || topup.AmountMinor <= 0 {
		return 0, ErrInvalidPaymentNotification
	}
	switch status {
	case models.TopupStatusPartialRefund, models.TopupStatusPartialChargeback:
		if refundAmountMinor <= 0 || refundAmountMinor >= topup.AmountMinor {
			return 0, ErrInvalidPaymentNotification
		}
	case models.TopupStatusRefunded, models.TopupStatusChargeback:
		refundAmountMinor = topup.AmountMinor
	default:
		return 0, ErrInvalidPaymentNotification
	}
	if topup.Credits > math.MaxInt64/refundAmountMinor {
		return 0, ErrInvalidPaymentNotification
	}
	return topup.Credits * refundAmountMinor / topup.AmountMinor, nil
}

func recordTopupReversalPaymentDueTx(tx *gorm.DB, topup *models.Topup, resources []models.BillableResource, dueAt time.Time) error {
	if tx == nil || topup == nil || topup.ID == 0 || len(resources) == 0 {
		return ErrInvalidPaymentNotification
	}
	idempotencyKey := fmt.Sprintf("topup:%s:payment-due", topup.ProviderOrderID)
	var invoice models.Invoice
	err := tx.Session(&gorm.Session{}).Where("idempotency_key = ?", idempotencyKey).First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		periodStart := topup.CreatedAt.UTC()
		invNumber := generateCanonicalInvoiceNumber(periodStart)
		profileSnapshot := snapshotBillingProfileTx(tx, resources[0].UserID)
		invoice = models.Invoice{
			UserID:                 resources[0].UserID,
			WalletID:               topup.WalletID,
			InvoiceNumber:          invNumber,
			BillingProfileSnapshot: profileSnapshot,
			PeriodStart:            periodStart,
			PeriodEnd:              periodStart.Add(time.Second),
			TotalCredits:           0,
			Status:                 models.InvoiceStatusPaymentDue,
			IdempotencyKey:         idempotencyKey,
			DueAt:                  &dueAt,
		}
		if err := tx.Session(&gorm.Session{}).Create(&invoice).Error; err != nil {
			return fmt.Errorf("create top-up reversal payment-due invoice: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("load top-up reversal payment-due invoice: %w", err)
	}

	for _, resource := range resources {
		item := models.InvoiceItem{
			InvoiceID:          invoice.ID,
			BillableResourceID: resource.ID,
			SpecID:             resource.SpecID,
			Description:        "Top-up reversal payment due",
			Credits:            0,
		}
		result := tx.Session(&gorm.Session{}).Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
		if result.Error != nil {
			return fmt.Errorf("create top-up reversal payment-due item: %w", result.Error)
		}
	}
	return nil
}

func (s *TopupService) validateNotification(notification MidtransNotification) (validatedPaymentNotification, error) {
	if notification.OrderID == "" || len(notification.OrderID) > 255 || notification.StatusCode == "" || len(notification.StatusCode) > 3 || notification.TransactionID == "" || len(notification.TransactionID) > 255 || notification.Currency != models.BillingCurrencyIDR || notification.MerchantID != s.cfg.MidtransMerchantID {
		return validatedPaymentNotification{}, ErrInvalidPaymentNotification
	}
	amount, err := parseIDRMinor(notification.GrossAmount)
	if err != nil || amount <= 0 {
		return validatedPaymentNotification{}, ErrInvalidPaymentNotification
	}
	if !validMidtransSignature(notification, s.cfg.MidtransServerKey) {
		return validatedPaymentNotification{}, ErrInvalidPaymentNotification
	}
	status, err := topupStatusFromNotification(notification.StatusCode, notification.TransactionStatus, notification.FraudStatus)
	if err != nil {
		return validatedPaymentNotification{}, err
	}
	if status == models.TopupStatusPaid && notification.StatusCode != "200" {
		return validatedPaymentNotification{}, ErrInvalidPaymentNotification
	}
	refundAmount, err := parseOptionalIDRMinor(notification.RefundAmount)
	if err != nil {
		return validatedPaymentNotification{}, ErrInvalidPaymentNotification
	}
	return validatedPaymentNotification{
		OrderID:           notification.OrderID,
		StatusCode:        notification.StatusCode,
		AmountMinor:       amount,
		Currency:          notification.Currency,
		TransactionID:     notification.TransactionID,
		TransactionStatus: notification.TransactionStatus,
		Status:            status,
		FraudStatus:       notification.FraudStatus,
		RefundAmountMinor: refundAmount,
	}, nil
}
