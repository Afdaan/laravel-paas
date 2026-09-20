// ===========================================
// Topup Recovery, Provider Claims & Helper Utilities
// ===========================================
package billing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const staleTopupRecoveryThreshold = 15 * time.Minute

type validatedPaymentNotification struct {
	OrderID           string             `json:"order_id"`
	StatusCode        string             `json:"status_code"`
	AmountMinor       int64              `json:"amount_minor"`
	Currency          string             `json:"currency"`
	TransactionID     string             `json:"transaction_id"`
	TransactionStatus string             `json:"transaction_status"`
	Status            models.TopupStatus `json:"status"`
	FraudStatus       string             `json:"fraud_status,omitempty"`
	RefundAmountMinor int64              `json:"refund_amount_minor,omitempty"`
}

func (s *TopupService) RecoverStaleTopups(ctx context.Context, now time.Time) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	var topupIDs []uint
	if err := s.db.WithContext(ctx).Model(&models.Topup{}).
		Where("status = ? AND provider_request_state = ? AND provider_payment_token = ? AND updated_at <= ?",
			models.TopupStatusPending, providerRequestCreating, "", now.UTC().Add(-staleTopupRecoveryThreshold)).
		Order("id ASC").Limit(50).
		Pluck("id", &topupIDs).Error; err != nil {
		return fmt.Errorf("list stale topups: %w", err)
	}
	var runErr error
	for _, topupID := range topupIDs {
		if err := s.recoverStaleTopup(ctx, topupID); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("recover stale topup %d: %w", topupID, err))
		}
	}
	return runErr
}

func (s *TopupService) recoverStaleTopup(ctx context.Context, topupID uint) error {
	var topup models.Topup
	if err := s.db.WithContext(ctx).First(&topup, topupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load stale topup: %w", err)
	}
	if err := s.recoverPaymentRequest(ctx, &topup); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).First(&topup, topupID).Error; err != nil {
		return fmt.Errorf("reload recovered topup: %w", err)
	}
	if topup.Provider == models.BillingProviderPakasir {
		if s.pakasirGateway != nil {
			detail, err := s.pakasirGateway.GetTransactionDetail(ctx, topup.ProviderOrderID, topup.AmountMinor)
			if err == nil && detail.OrderID != "" {
				_ = s.ProcessPakasirWebhook(ctx, detail.OrderID, detail.Amount, detail.Status)
			}
		}
		return nil
	} else if s.gateway != nil {
		notification, err := s.gateway.GetTransactionStatus(ctx, topup.ProviderOrderID)
		if err != nil {
			return err
		}
		if notification.OrderID == "" {
			return nil
		}
		return s.ProcessReconciledNotification(ctx, notification)
	}
	return nil
}

func (s *TopupService) retryDueInvoices(ctx context.Context, userID uint) {
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), invoiceRetryTimeout)
	defer cancel()
	if err := NewInvoiceService(s.db, s.wallets).RetryDueForUser(retryCtx, userID, time.Now().UTC()); err != nil {
		slog.Error("Billing invoice retry after top-up failed", "user_id", userID, "error", err)
	}
}

func (s *TopupService) ensurePaymentRequest(ctx context.Context, topup *models.Topup) error {
	if topup.ProviderPaymentToken != "" || isTerminalTopupStatus(topup.Status) {
		return nil
	}

	for {
		claimed, err := s.claimPaymentRequest(ctx, topup)
		if err != nil {
			return err
		}
		if !claimed {
			ready, err := s.waitForPaymentRequest(ctx, topup)
			if err != nil || ready {
				return err
			}
			continue
		}
		break
	}

	var paymentToken, paymentURL string
	if topup.Provider == models.BillingProviderPakasir {
		if s.pakasirGateway == nil {
			if cleanupErr := s.resetPaymentRequestAfterProviderFailure(ctx, topup); cleanupErr != nil {
				return errors.Join(ErrPaymentProvider, cleanupErr)
			}
			return ErrPaymentProvider
		}
		res, err := s.pakasirGateway.CreateTransaction(ctx, topup.ProviderOrderID, topup.AmountMinor, "qris")
		if err != nil {
			if cleanupErr := s.resetPaymentRequestAfterProviderFailure(ctx, topup); cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("%w: reset checkout request: %v", ErrTopupRecoveryRequired, cleanupErr))
			}
			return err
		}
		paymentToken = res.PaymentNumber
		paymentURL = sanitizePakasirPaymentURL(res.PaymentNumber)
		if paymentURL == "" {
			slug := ""
			if s.cfg != nil {
				slug = s.cfg.PakasirProjectSlug
			}
			if slug == "" {
				slug = "runara"
			}
			redirectURL := ""
			redirectPath := fmt.Sprintf("/billing?payment_return=pakasir&topup_ref=%s", url.QueryEscape(topup.ProviderOrderID))
			if s.cfg != nil {
				if s.cfg.FrontendURL != "" {
					redirectURL = strings.TrimRight(s.cfg.FrontendURL, "/") + redirectPath
				} else if s.cfg.BaseDomain != "" {
					redirectURL = "https://" + strings.TrimRight(s.cfg.BaseDomain, "/") + redirectPath
				}
			}
			paymentURL = fmt.Sprintf("https://app.pakasir.com/pay/%s/%d?order_id=%s&qris_only=1", slug, topup.AmountMinor, url.QueryEscape(topup.ProviderOrderID))
			if redirectURL != "" {
				paymentURL += "&redirect=" + url.QueryEscape(redirectURL)
			}
		}
		if paymentURL == "" {
			if cleanupErr := s.resetPaymentRequestAfterProviderFailure(ctx, topup); cleanupErr != nil {
				return errors.Join(ErrPaymentProvider, cleanupErr)
			}
			return ErrPaymentProvider
		}
	} else {
		finishURL := ""
		if s.cfg != nil {
			if s.cfg.FrontendURL != "" {
				finishURL = strings.TrimRight(s.cfg.FrontendURL, "/") + "/billing"
			} else if s.cfg.BaseDomain != "" {
				finishURL = "https://" + strings.TrimRight(s.cfg.BaseDomain, "/") + "/billing"
			}
		}
		payment, err := s.gateway.CreatePayment(ctx, MidtransPaymentRequest{
			OrderID:        topup.ProviderOrderID,
			AmountMinor:    topup.AmountMinor,
			Currency:       topup.Currency,
			IdempotencyKey: topup.ProviderOrderID,
			FinishURL:      finishURL,
		})
		if err != nil {
			if cleanupErr := s.resetPaymentRequestAfterProviderFailure(ctx, topup); cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("%w: reset checkout request: %v", ErrTopupRecoveryRequired, cleanupErr))
			}
			return err
		}
		if err := validatePaymentResponse(payment); err != nil {
			if cleanupErr := s.resetPaymentRequestAfterProviderFailure(ctx, topup); cleanupErr != nil {
				return errors.Join(err, fmt.Errorf("%w: reset checkout request: %v", ErrTopupRecoveryRequired, cleanupErr))
			}
			return err
		}
		paymentToken = payment.Token
		paymentURL = payment.RedirectURL
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(topup, topup.ID).Error; err != nil {
			return fmt.Errorf("lock created payment request: %w", err)
		}
		if topup.ProviderPaymentToken != "" || isTerminalTopupStatus(topup.Status) {
			return nil
		}
		if topup.ProviderRequestState != providerRequestCreating {
			return ErrTopupRecoveryRequired
		}
		if err := tx.Model(topup).Updates(map[string]any{"provider_payment_token": paymentToken, "provider_payment_url": paymentURL, "provider_request_state": providerRequestReady}).Error; err != nil {
			return fmt.Errorf("store payment request: %w", err)
		}
		topup.ProviderPaymentToken = paymentToken
		topup.ProviderPaymentURL = paymentURL
		topup.ProviderRequestState = providerRequestReady
		return nil
	})
}

func (s *TopupService) resetPaymentRequestAfterProviderFailure(ctx context.Context, topup *models.Topup) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()

	result := s.db.WithContext(cleanupCtx).
		Model(&models.Topup{}).
		Where("id = ? AND provider_request_state = ? AND status = ?", topup.ID, providerRequestCreating, models.TopupStatusPending).
		Update("provider_request_state", providerRequestPending)
	if result.Error != nil {
		return fmt.Errorf("reset payment request after provider failure: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		topup.ProviderRequestState = providerRequestPending
	}
	return nil
}

func (s *TopupService) claimPaymentRequest(ctx context.Context, topup *models.Topup) (bool, error) {
	claimed := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(topup, topup.ID).Error; err != nil {
			return fmt.Errorf("lock topup payment request: %w", err)
		}
		if topup.ProviderPaymentToken != "" || isTerminalTopupStatus(topup.Status) {
			return nil
		}
		switch topup.ProviderRequestState {
		case providerRequestPending:
			if err := tx.Model(topup).Update("provider_request_state", providerRequestCreating).Error; err != nil {
				return fmt.Errorf("mark payment request creating: %w", err)
			}
			topup.ProviderRequestState = providerRequestCreating
			claimed = true
			return nil
		case providerRequestCreating:
			return nil
		default:
			return ErrTopupRecoveryRequired
		}
	})
	if err != nil {
		return false, err
	}
	return claimed, nil
}

func (s *TopupService) waitForPaymentRequest(ctx context.Context, topup *models.Topup) (bool, error) {
	for {
		if topup.ProviderPaymentToken != "" || isTerminalTopupStatus(topup.Status) {
			return true, nil
		}
		if topup.ProviderRequestState == providerRequestPending {
			return false, nil
		}
		if topup.ProviderRequestState != providerRequestCreating {
			return false, ErrTopupRecoveryRequired
		}
		timer := time.NewTimer(paymentRequestWaitInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false, ctx.Err()
		case <-timer.C:
		}
		if err := s.db.WithContext(ctx).First(topup, topup.ID).Error; err != nil {
			return false, fmt.Errorf("reload topup payment request: %w", err)
		}
	}
}

func withTopupRequestDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), topupRequestTimeout)
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, topupRequestTimeout)
}

func (s *TopupService) recoverPaymentRequest(ctx context.Context, topup *models.Topup) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(topup, topup.ID).Error; err != nil {
			return fmt.Errorf("lock topup payment recovery: %w", err)
		}
		if topup.ProviderPaymentToken != "" || isTerminalTopupStatus(topup.Status) {
			return nil
		}
		if topup.Status != models.TopupStatusPending || topup.ProviderRequestState != providerRequestCreating {
			return ErrTopupRecoveryRequired
		}
		if err := tx.Model(topup).Update("provider_request_state", providerRequestPending).Error; err != nil {
			return fmt.Errorf("reset payment request recovery: %w", err)
		}
		topup.ProviderRequestState = providerRequestPending
		return nil
	}); err != nil {
		return err
	}
	return s.ensurePaymentRequest(ctx, topup)
}

func isTerminalTopupStatus(status models.TopupStatus) bool {
	switch status {
	case models.TopupStatusPaid, models.TopupStatusFailed, models.TopupStatusExpired, models.TopupStatusPartialRefund, models.TopupStatusRefunded, models.TopupStatusPartialChargeback, models.TopupStatusChargeback:
		return true
	default:
		return false
	}
}

func isReversalTopupStatus(status models.TopupStatus) bool {
	switch status {
	case models.TopupStatusPartialRefund, models.TopupStatusRefunded, models.TopupStatusPartialChargeback, models.TopupStatusChargeback:
		return true
	default:
		return false
	}
}

func parseIDRMinor(value string) (int64, error) {
	whole, fraction, hasFraction := strings.Cut(value, ".")
	if whole == "" || (hasFraction && fraction != "00") {
		return 0, ErrInvalidPaymentNotification
	}
	for _, character := range whole {
		if character < '0' || character > '9' {
			return 0, ErrInvalidPaymentNotification
		}
	}
	amount, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrInvalidPaymentNotification
	}
	return amount, nil
}

func parseOptionalIDRMinor(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	return parseIDRMinor(value)
}

func validatePaymentResponse(payment MidtransPaymentResponse) error {
	if payment.Token == "" || len(payment.Token) > 255 || payment.RedirectURL == "" || len(payment.RedirectURL) > 2048 {
		return ErrPaymentProvider
	}
	parsed, err := url.Parse(payment.RedirectURL)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "app.sandbox.midtrans.com" && parsed.Host != "app.midtrans.com") {
		return ErrPaymentProvider
	}
	return nil
}

func paymentEventKey(payload []byte) string {
	sum := sha256.Sum256(payload)
	return models.BillingProviderMidtrans + ":" + hex.EncodeToString(sum[:])
}

func majorUnits(amountMinor int64, currency string) (int64, error) {
	exponent, supported := models.CurrencyMinorUnits(currency)
	if !supported {
		return 0, ErrPaymentProvider
	}
	divisor := int64(1)
	for range exponent {
		divisor *= 10
	}
	if amountMinor%divisor != 0 {
		return 0, ErrPaymentProvider
	}
	return amountMinor / divisor, nil
}

func topupIdempotencyMatch(topup models.Topup, input TopupInput) bool {
	if input.PackageID > 0 {
		return topup.TopupPackageID != nil && *topup.TopupPackageID == input.PackageID
	}
	return topup.TopupPackageID == nil && topup.AmountMinor == input.AmountMinor
}

var topupRandReader io.Reader = rand.Reader

func generateTopupProviderOrderID() (string, error) {
	var buf [16]byte
	if _, err := io.ReadFull(topupRandReader, buf[:]); err != nil {
		return "", fmt.Errorf("generate provider order ID: %w", err)
	}
	return fmt.Sprintf("topup-%x", buf[:]), nil
}

func topupView(topup models.Topup) TopupView {
	view := TopupView{
		ID:          topup.ID,
		Credits:     topup.Credits,
		AmountMinor: topup.AmountMinor,
		Currency:    topup.Currency,
		Status:      topup.Status,
	}
	if topup.Status == models.TopupStatusPending {
		view.PaymentToken = topup.ProviderPaymentToken
		view.PaymentURL = topup.ProviderPaymentURL
	}
	return view
}

func loadWalletUserID(tx *gorm.DB, walletID uint) (uint, error) {
	var wallet models.Wallet
	if err := tx.Session(&gorm.Session{}).Where("id = ?", walletID).First(&wallet).Error; err != nil {
		return 0, fmt.Errorf("load topup wallet: %w", err)
	}
	return wallet.UserID, nil
}

func sanitizePakasirPaymentURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if parsed.Scheme != "https" {
		return ""
	}
	if parsed.User != nil {
		return ""
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "app.pakasir.com" && host != "pakasir.com" {
		return ""
	}
	return parsed.String()
}
