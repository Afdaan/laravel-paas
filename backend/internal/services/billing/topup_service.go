package billing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTopupDisabled              = errors.New("topups are disabled")
	ErrInvalidTopupInput          = errors.New("invalid topup input")
	ErrTopupIdempotencyConflict   = errors.New("topup idempotency key conflict")
	ErrTopupNotFound              = errors.New("topup not found")
	ErrTopupRecoveryRequired      = errors.New("topup checkout recovery required")
	ErrInvalidPaymentNotification = errors.New("invalid payment notification")
	ErrPaymentProvider            = errors.New("payment provider unavailable")
)

const (
	midtransSandboxSnapURL     = "https://app.sandbox.midtrans.com/snap/v1/transactions"
	midtransProductionSnapURL  = "https://app.midtrans.com/snap/v1/transactions"
	midtransSandboxAPIURL      = "https://api.sandbox.midtrans.com"
	midtransProductionAPIURL   = "https://api.midtrans.com"
	midtransRequestTimeout     = 10 * time.Second
	topupRequestTimeout        = midtransRequestTimeout + 2*time.Second
	paymentRequestWaitInterval = 25 * time.Millisecond
	invoiceRetryTimeout        = time.Second

	providerRequestPending  = "pending"
	providerRequestCreating = "creating"
	providerRequestReady    = "ready"
	providerRequestTerminal = "terminal"
)

type TopupInput struct {
	PackageID uint `json:"topup_package_id"`
}

type TopupView struct {
	ID           uint               `json:"id"`
	Credits      int64              `json:"credits"`
	AmountMinor  int64              `json:"amount_minor"`
	Currency     string             `json:"currency"`
	Status       models.TopupStatus `json:"status"`
	PaymentToken string             `json:"payment_token,omitempty"`
	PaymentURL   string             `json:"payment_url,omitempty"`
}

type MidtransPaymentRequest struct {
	OrderID        string
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
}

type MidtransPaymentResponse struct {
	Token       string
	RedirectURL string
}

type MidtransNotification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	TransactionID     string `json:"transaction_id"`
	Currency          string `json:"currency"`
	MerchantID        string `json:"merchant_id"`
}

type MidtransGateway interface {
	CreatePayment(context.Context, MidtransPaymentRequest) (MidtransPaymentResponse, error)
	GetTransactionStatus(context.Context, string) (MidtransNotification, error)
}

type MidtransClient struct {
	serverKey  string
	httpClient *http.Client
	snapURL    string
	apiURL     string
}

type TopupService struct {
	db      *gorm.DB
	wallets *WalletService
	cfg     *config.Config
	gateway MidtransGateway
}

func NewMidtransClient(cfg *config.Config) *MidtransClient {
	snapURL := midtransSandboxSnapURL
	apiURL := midtransSandboxAPIURL
	if cfg != nil && cfg.MidtransProduction {
		snapURL = midtransProductionSnapURL
		apiURL = midtransProductionAPIURL
	}
	serverKey := ""
	if cfg != nil {
		serverKey = cfg.MidtransServerKey
	}
	return &MidtransClient{
		serverKey:  serverKey,
		httpClient: &http.Client{Timeout: midtransRequestTimeout},
		snapURL:    snapURL,
		apiURL:     apiURL,
	}
}

func NewTopupService(db *gorm.DB, wallets *WalletService, cfg *config.Config, gateway MidtransGateway) *TopupService {
	if wallets == nil {
		wallets = NewWalletService(db)
	}
	if gateway == nil {
		gateway = NewMidtransClient(cfg)
	}
	return &TopupService{db: db, wallets: wallets, cfg: cfg, gateway: gateway}
}

func (s *TopupService) Create(ctx context.Context, userID uint, clientKey string, input TopupInput) (TopupView, error) {
	if err := s.validateCreate(ctx, userID, clientKey, input); err != nil {
		return TopupView{}, err
	}
	ctx, cancel := withTopupRequestDeadline(ctx)
	defer cancel()

	wallet, err := s.wallets.GetOrCreateWallet(ctx, userID)
	if err != nil {
		return TopupView{}, err
	}

	var topup models.Topup
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("wallet_id = ? AND client_idempotency_key = ?", wallet.ID, clientKey).First(&topup).Error; err == nil {
			if topup.TopupPackageID == nil || *topup.TopupPackageID != input.PackageID {
				return ErrTopupIdempotencyConflict
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock existing topup: %w", err)
		}

		var topupPackage models.TopupPackage
		if err := tx.Where("id = ? AND provider = ? AND currency = ? AND is_active = ?", input.PackageID, models.BillingProviderMidtrans, models.BillingCurrencyIDR, true).First(&topupPackage).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidTopupInput
			}
			return fmt.Errorf("load active topup package: %w", err)
		}

		topup = models.Topup{
			WalletID:             wallet.ID,
			TopupPackageID:       &topupPackage.ID,
			ClientIdempotencyKey: clientKey,
			Provider:             models.BillingProviderMidtrans,
			ProviderOrderID:      topupProviderOrderID(wallet.ID, clientKey),
			ProviderRequestState: providerRequestPending,
			AmountMinor:          topupPackage.AmountMinor,
			Currency:             topupPackage.Currency,
			Credits:              topupPackage.Credits,
			Status:               models.TopupStatusPending,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "wallet_id"}, {Name: "client_idempotency_key"}},
			DoNothing: true,
		}).Create(&topup)
		if result.Error != nil {
			return fmt.Errorf("create pending topup: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("wallet_id = ? AND client_idempotency_key = ?", wallet.ID, clientKey).First(&topup).Error; err != nil {
				return fmt.Errorf("lock concurrent topup replay: %w", err)
			}
			if topup.TopupPackageID == nil || *topup.TopupPackageID != input.PackageID {
				return ErrTopupIdempotencyConflict
			}
		}
		return nil
	})
	if err != nil {
		return TopupView{}, err
	}
	if err := s.ensurePaymentRequest(ctx, &topup); err != nil {
		return TopupView{}, err
	}
	return topupView(topup), nil
}

func (s *TopupService) Reconcile(ctx context.Context, userID, topupID uint) (TopupView, error) {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return TopupView{}, err
	}
	if userID == 0 || topupID == 0 {
		return TopupView{}, ErrInvalidTopupInput
	}

	var topup models.Topup
	err := s.db.WithContext(ctx).
		Joins("JOIN wallets ON wallets.id = topups.wallet_id").
		Where("topups.id = ? AND wallets.user_id = ?", topupID, userID).
		First(&topup).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TopupView{}, ErrTopupNotFound
	}
	if err != nil {
		return TopupView{}, fmt.Errorf("load topup for reconciliation: %w", err)
	}

	notification, err := s.gateway.GetTransactionStatus(ctx, topup.ProviderOrderID)
	if err != nil {
		return TopupView{}, err
	}
	if err := s.ProcessNotification(ctx, notification); err != nil {
		return TopupView{}, err
	}
	if err := s.db.WithContext(ctx).First(&topup, topup.ID).Error; err != nil {
		return TopupView{}, fmt.Errorf("reload reconciled topup: %w", err)
	}
	if topup.Status == models.TopupStatusPending && topup.ProviderRequestState == providerRequestCreating && topup.ProviderPaymentToken == "" {
		if err := s.recoverPaymentRequest(ctx, &topup); err != nil {
			return TopupView{}, err
		}
		if err := s.db.WithContext(ctx).First(&topup, topup.ID).Error; err != nil {
			return TopupView{}, fmt.Errorf("reload recovered topup: %w", err)
		}
	}
	if topup.Status == models.TopupStatusPending && topup.ProviderRequestState == providerRequestCreating && topup.ProviderPaymentToken == "" {
		return TopupView{}, ErrTopupRecoveryRequired
	}
	return topupView(topup), nil
}

func (s *TopupService) ProcessNotification(ctx context.Context, notification MidtransNotification) error {
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

	var creditedUserID uint
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var topup models.Topup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND provider_order_id = ?", models.BillingProviderMidtrans, validated.OrderID).First(&topup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupNotFound
			}
			return fmt.Errorf("lock topup for payment notification: %w", err)
		}
		if topup.AmountMinor != validated.AmountMinor || topup.Currency != validated.Currency {
			return ErrInvalidPaymentNotification
		}
		if topup.ProviderTransactionID != nil && *topup.ProviderTransactionID != validated.TransactionID {
			return ErrInvalidPaymentNotification
		}

		event := models.PaymentEvent{
			Provider:        models.BillingProviderMidtrans,
			EventKey:        eventKey,
			ProviderOrderID: validated.OrderID,
			PayloadJSON:     string(payloadJSON),
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
		if result.Error != nil {
			return fmt.Errorf("record payment event: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}

		updates := map[string]any{"status": validated.Status}
		if topup.ProviderTransactionID == nil {
			updates["provider_transaction_id"] = validated.TransactionID
		}
		switch validated.Status {
		case models.TopupStatusPaid:
			if topup.Status != models.TopupStatusRefunded && topup.Status != models.TopupStatusChargeback {
				userID, err := loadWalletUserID(tx, topup.WalletID)
				if err != nil {
					return err
				}
				if _, err := s.wallets.applyInTransaction(tx, LedgerMutation{
					UserID:         userID,
					EntryType:      models.WalletLedgerEntryTopup,
					AmountCredits:  topup.Credits,
					IdempotencyKey: fmt.Sprintf("topup:%d:credit", topup.ID),
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
		case models.TopupStatusRefunded, models.TopupStatusChargeback:
			if topup.Status == models.TopupStatusPaid {
				userID, err := loadWalletUserID(tx, topup.WalletID)
				if err != nil {
					return err
				}
				var activeResources []models.BillableResource
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("user_id = ? AND billing_status = ?", userID, models.BillableResourceStatusActive).
					Find(&activeResources).Error; err != nil {
					return fmt.Errorf("lock active resources before top-up reversal: %w", err)
				}
				reversal, err := s.wallets.applyInTransaction(tx, LedgerMutation{
					UserID:         userID,
					EntryType:      models.WalletLedgerEntryTopupReversal,
					AmountCredits:  topup.Credits,
					IdempotencyKey: fmt.Sprintf("topup:%d:reversal", topup.ID),
					ReferenceType:  "topup",
					ReferenceID:    strconv.FormatUint(uint64(topup.ID), 10),
				}, false)
				if err != nil {
					return err
				}
				if reversal.BalanceAfter < 0 && len(activeResources) > 0 {
					now := time.Now().UTC()
					if err := recordTopupReversalPaymentDueTx(tx, &topup, activeResources, now); err != nil {
						return err
					}
					resourceIDs := make([]uint, 0, len(activeResources))
					for _, resource := range activeResources {
						resourceIDs = append(resourceIDs, resource.ID)
					}
					if err := tx.Model(&models.BillableResource{}).Where("id IN ?", resourceIDs).Update("billing_status", models.BillableResourceStatusPaymentDue).Error; err != nil {
						return fmt.Errorf("move overdue resources to payment due after top-up reversal: %w", err)
					}
				}
			}
		case models.TopupStatusPending:
			if topup.Status != models.TopupStatusPending {
				updates["status"] = topup.Status
			}
		default:
			if topup.Status == models.TopupStatusPaid || topup.Status == models.TopupStatusRefunded || topup.Status == models.TopupStatusChargeback {
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
	return nil
}

func recordTopupReversalPaymentDueTx(tx *gorm.DB, topup *models.Topup, resources []models.BillableResource, dueAt time.Time) error {
	if tx == nil || topup == nil || topup.ID == 0 || len(resources) == 0 {
		return ErrInvalidPaymentNotification
	}
	idempotencyKey := fmt.Sprintf("topup:%d:payment-due", topup.ID)
	var invoice models.Invoice
	err := tx.Where("idempotency_key = ?", idempotencyKey).First(&invoice).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		periodStart := topup.CreatedAt.UTC()
		invoice = models.Invoice{
			UserID:         resources[0].UserID,
			WalletID:       topup.WalletID,
			PeriodStart:    periodStart,
			PeriodEnd:      periodStart.Add(time.Second),
			TotalCredits:   0,
			Status:         models.InvoiceStatusPaymentDue,
			IdempotencyKey: idempotencyKey,
			DueAt:          &dueAt,
		}
		if err := tx.Create(&invoice).Error; err != nil {
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
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item)
		if result.Error != nil {
			return fmt.Errorf("create top-up reversal payment-due item: %w", result.Error)
		}
	}
	return nil
}

func (s *TopupService) retryDueInvoices(ctx context.Context, userID uint) {
	// ponytail: bound webhook-side retry latency; the scheduler retries any remaining payment-due resources.
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

	payment, err := s.gateway.CreatePayment(ctx, MidtransPaymentRequest{OrderID: topup.ProviderOrderID, AmountMinor: topup.AmountMinor, Currency: topup.Currency, IdempotencyKey: topup.ProviderOrderID})
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
		if err := tx.Model(topup).Updates(map[string]any{"provider_payment_token": payment.Token, "provider_payment_url": payment.RedirectURL, "provider_request_state": providerRequestReady}).Error; err != nil {
			return fmt.Errorf("store payment request: %w", err)
		}
		topup.ProviderPaymentToken = payment.Token
		topup.ProviderPaymentURL = payment.RedirectURL
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
	case models.TopupStatusPaid, models.TopupStatusFailed, models.TopupStatusExpired, models.TopupStatusRefunded, models.TopupStatusChargeback:
		return true
	default:
		return false
	}
}

func (s *TopupService) validateCreate(ctx context.Context, userID uint, clientKey string, input TopupInput) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	if !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
	}
	if userID == 0 || input.PackageID == 0 || clientKey == "" || len(clientKey) > 255 || strings.TrimSpace(clientKey) != clientKey {
		return ErrInvalidTopupInput
	}
	return nil
}

func (s *TopupService) validatePaymentProcessing(ctx context.Context) error {
	if s == nil || s.db == nil || s.wallets == nil || s.cfg == nil || s.gateway == nil {
		return ErrTopupDisabled
	}
	if ctx == nil || !s.cfg.BillingEnabled || s.cfg.MidtransServerKey == "" || s.cfg.MidtransMerchantID == "" {
		return ErrTopupDisabled
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
	return validatedPaymentNotification{
		OrderID:           notification.OrderID,
		StatusCode:        notification.StatusCode,
		AmountMinor:       amount,
		Currency:          notification.Currency,
		TransactionID:     notification.TransactionID,
		TransactionStatus: notification.TransactionStatus,
		Status:            status,
		FraudStatus:       notification.FraudStatus,
	}, nil
}

type validatedPaymentNotification struct {
	OrderID           string             `json:"order_id"`
	StatusCode        string             `json:"status_code"`
	AmountMinor       int64              `json:"amount_minor"`
	Currency          string             `json:"currency"`
	TransactionID     string             `json:"transaction_id"`
	TransactionStatus string             `json:"transaction_status"`
	Status            models.TopupStatus `json:"status"`
	FraudStatus       string             `json:"fraud_status,omitempty"`
}

func (c *MidtransClient) CreatePayment(ctx context.Context, payment MidtransPaymentRequest) (MidtransPaymentResponse, error) {
	if c == nil || c.httpClient == nil || c.serverKey == "" || payment.OrderID == "" || payment.AmountMinor <= 0 || payment.Currency != models.BillingCurrencyIDR {
		return MidtransPaymentResponse{}, ErrPaymentProvider
	}
	body, err := json.Marshal(struct {
		TransactionDetails struct {
			OrderID     string `json:"order_id"`
			GrossAmount int64  `json:"gross_amount"`
		} `json:"transaction_details"`
	}{TransactionDetails: struct {
		OrderID     string `json:"order_id"`
		GrossAmount int64  `json:"gross_amount"`
	}{OrderID: payment.OrderID, GrossAmount: payment.AmountMinor}})
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("marshal Midtrans payment request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.snapURL, bytes.NewReader(body))
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("create Midtrans payment request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Idempotency-Key", payment.IdempotencyKey)
	request.SetBasicAuth(c.serverKey, "")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: create payment request", ErrPaymentProvider)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: create payment status %d", ErrPaymentProvider, response.StatusCode)
	}
	var parsed struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := decodeProviderJSON(io.LimitReader(response.Body, 8*1024), &parsed); err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: decode payment response", ErrPaymentProvider)
	}
	return MidtransPaymentResponse{Token: parsed.Token, RedirectURL: parsed.RedirectURL}, nil
}

func (c *MidtransClient) GetTransactionStatus(ctx context.Context, orderID string) (MidtransNotification, error) {
	if c == nil || c.httpClient == nil || c.serverKey == "" || orderID == "" {
		return MidtransNotification{}, ErrPaymentProvider
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.apiURL, "/")+"/v2/"+url.PathEscape(orderID)+"/status", nil)
	if err != nil {
		return MidtransNotification{}, fmt.Errorf("create Midtrans status request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(c.serverKey, "")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return MidtransNotification{}, fmt.Errorf("%w: get transaction status", ErrPaymentProvider)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return MidtransNotification{}, fmt.Errorf("%w: transaction status %d", ErrPaymentProvider, response.StatusCode)
	}
	var notification MidtransNotification
	if err := decodeProviderJSON(io.LimitReader(response.Body, 8*1024), &notification); err != nil {
		return MidtransNotification{}, fmt.Errorf("%w: decode transaction status", ErrPaymentProvider)
	}
	return notification, nil
}

func validMidtransSignature(notification MidtransNotification, serverKey string) bool {
	sum := sha512.Sum512([]byte(notification.OrderID + notification.StatusCode + notification.GrossAmount + serverKey))
	expected := hex.EncodeToString(sum[:])
	return len(notification.SignatureKey) == len(expected) && subtle.ConstantTimeCompare([]byte(strings.ToLower(notification.SignatureKey)), []byte(expected)) == 1
}

func topupStatusFromNotification(statusCode, transactionStatus, fraudStatus string) (models.TopupStatus, error) {
	switch transactionStatus {
	case "capture":
		switch fraudStatus {
		case "accept":
			if statusCode == "200" {
				return models.TopupStatusPaid, nil
			}
		case "challenge":
			if statusCode == "201" {
				return models.TopupStatusPending, nil
			}
		case "deny":
			if statusCode == "202" {
				return models.TopupStatusFailed, nil
			}
		}
	case "settlement":
		if statusCode == "200" && (fraudStatus == "" || fraudStatus == "accept") {
			return models.TopupStatusPaid, nil
		}
	case "pending":
		if statusCode == "201" && (fraudStatus == "" || fraudStatus == "accept" || fraudStatus == "challenge") {
			return models.TopupStatusPending, nil
		}
	case "deny", "cancel":
		if statusCode == "202" {
			return models.TopupStatusFailed, nil
		}
	case "expire":
		if statusCode == "202" {
			return models.TopupStatusExpired, nil
		}
	case "refund":
		if statusCode == "200" {
			return models.TopupStatusRefunded, nil
		}
	case "chargeback":
		if statusCode == "200" {
			return models.TopupStatusChargeback, nil
		}
	}
	return "", ErrInvalidPaymentNotification
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

func decodeProviderJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func paymentEventKey(payload []byte) string {
	sum := sha256.Sum256(payload)
	return models.BillingProviderMidtrans + ":" + hex.EncodeToString(sum[:])
}

func topupProviderOrderID(walletID uint, clientKey string) string {
	sum := sha256.Sum256([]byte(clientKey))
	return fmt.Sprintf("topup-%d-%x", walletID, sum[:16])
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
	if err := tx.Where("id = ?", walletID).First(&wallet).Error; err != nil {
		return 0, fmt.Errorf("load topup wallet: %w", err)
	}
	return wallet.UserID, nil
}
