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
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/services/setting"
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
	PackageID   uint  `json:"topup_package_id"`
	AmountMinor int64 `json:"amount,omitempty"` // custom topup; ignored when PackageID > 0
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
	FinishURL      string
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
	RefundAmount      string `json:"refund_amount"`
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
	db             *gorm.DB
	wallets        *WalletService
	cfg            *config.Config
	gateway        MidtransGateway
	pakasirGateway PakasirGateway
	settingService *setting.SettingService
	billingProfile *BillingProfileService
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

func NewTopupService(db *gorm.DB, wallets *WalletService, cfg *config.Config, gateway MidtransGateway, pakasirGateway ...PakasirGateway) *TopupService {
	if wallets == nil {
		wallets = NewWalletService(db)
	}
	if gateway == nil {
		gateway = NewMidtransClient(cfg)
	}
	var pGateway PakasirGateway
	if len(pakasirGateway) > 0 && pakasirGateway[0] != nil {
		pGateway = pakasirGateway[0]
	} else {
		pGateway = NewPakasirClient(cfg)
	}
	return &TopupService{db: db, wallets: wallets, cfg: cfg, gateway: gateway, pakasirGateway: pGateway, billingProfile: NewBillingProfileService(db)}
}

func (s *TopupService) SetSettingService(settingService *setting.SettingService) {
	s.settingService = settingService
}

func (s *TopupService) activeProvider() string {
	if s.settingService != nil {
		if val := s.settingService.Get(models.SettingDefaultPaymentProvider, ""); val != "" {
			val = strings.ToLower(strings.TrimSpace(val))
			if val == models.BillingProviderPakasir || val == models.BillingProviderMidtrans {
				return val
			}
		}
	}
	if s.cfg != nil {
		prov := strings.ToLower(strings.TrimSpace(s.cfg.BillingTopupProvider))
		if prov == models.BillingProviderPakasir {
			return models.BillingProviderPakasir
		}
		if prov == models.BillingProviderMidtrans {
			return models.BillingProviderMidtrans
		}
		if s.cfg.PakasirEnabled && (s.cfg.MidtransServerKey == "" || s.cfg.PakasirProjectSlug != "") {
			return models.BillingProviderPakasir
		}
	}
	return models.BillingProviderMidtrans
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
			if !topupIdempotencyMatch(topup, input) {
				return ErrTopupIdempotencyConflict
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock existing topup: %w", err)
		}

		provider := s.activeProvider()
		if input.PackageID > 0 {
			var topupPackage models.TopupPackage
			if err := tx.Where("id = ? AND is_active = ?", input.PackageID, true).First(&topupPackage).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInvalidTopupInput
				}
				return fmt.Errorf("load active topup package: %w", err)
			}

			topup = models.Topup{
				WalletID:             wallet.ID,
				TopupPackageID:       &topupPackage.ID,
				ClientIdempotencyKey: clientKey,
				Provider:             provider,
				ProviderOrderID:      topupProviderOrderID(wallet.ID, clientKey),
				ProviderRequestState: providerRequestPending,
				AmountMinor:          topupPackage.AmountMinor,
				Currency:             topupPackage.Currency,
				Credits:              topupPackage.Credits,
				Status:               models.TopupStatusPending,
			}
		} else {
			// ponytail: IDR-only custom topup; add currency param when supporting USD
			ratePerCredit := int64(1000)
			var basePackage models.TopupPackage
			if err := tx.Where("currency = ? AND is_active = ? AND credits > 0 AND amount_minor > 0", models.BillingCurrencyIDR, true).Order("credits ASC").First(&basePackage).Error; err == nil && basePackage.Credits > 0 {
				rate := basePackage.AmountMinor / basePackage.Credits
				if rate > 0 {
					ratePerCredit = rate
				}
			}
			topup = models.Topup{
				WalletID:             wallet.ID,
				ClientIdempotencyKey: clientKey,
				Provider:             provider,
				ProviderOrderID:      topupProviderOrderID(wallet.ID, clientKey),
				ProviderRequestState: providerRequestPending,
				AmountMinor:          input.AmountMinor,
				Currency:             models.BillingCurrencyIDR,
				Credits:              input.AmountMinor / ratePerCredit,
				Status:               models.TopupStatusPending,
			}
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
			if !topupIdempotencyMatch(topup, input) {
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

	if topup.Provider == models.BillingProviderPakasir {
		if s.pakasirGateway != nil {
			detail, err := s.pakasirGateway.GetTransactionDetail(ctx, topup.ProviderOrderID, topup.AmountMinor)
			if err == nil && detail.OrderID != "" {
				_ = s.ProcessPakasirWebhook(ctx, detail.OrderID, detail.Amount, detail.Status)
			}
		}
	} else if s.gateway != nil {
		notification, err := s.gateway.GetTransactionStatus(ctx, topup.ProviderOrderID)
		if err == nil {
			_ = s.ProcessReconciledNotification(ctx, notification)
		}
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
		var topup models.Topup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND provider_order_id = ?", models.BillingProviderMidtrans, validated.OrderID).First(&topup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupNotFound
			}
			return fmt.Errorf("lock topup for payment notification: %w", err)
		}
		if topup.Currency != validated.Currency {
			return ErrInvalidPaymentNotification
		}
		if topup.ProviderTransactionID != nil && *topup.ProviderTransactionID != validated.TransactionID {
			return ErrInvalidPaymentNotification
		}
		if topup.AmountMinor != validated.AmountMinor {
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
			if topup.Status != models.TopupStatusPartialRefund && topup.Status != models.TopupStatusRefunded && topup.Status != models.TopupStatusChargeback {
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
	return nil
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
	if err := tx.Model(&models.WalletLedgerEntry{}).
		Select("COALESCE(SUM(-amount_credits), 0)").
		Where("type = ? AND reference_type = ? AND reference_id = ?", models.WalletLedgerEntryTopupReversal, "topup", strconv.FormatUint(uint64(topup.ID), 10)).
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
	var activeResources []models.BillableResource
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ? AND billing_status = ?", userID, models.BillableResourceStatusActive).
		Find(&activeResources).Error; err != nil {
		return fmt.Errorf("lock active resources before top-up reversal: %w", err)
	}
	reversal, err := s.wallets.applyInTransaction(tx, LedgerMutation{
		UserID:         userID,
		EntryType:      models.WalletLedgerEntryTopupReversal,
		AmountCredits:  delta,
		IdempotencyKey: fmt.Sprintf("topup:%d:reversal:%d", topup.ID, desiredReversal),
		ReferenceType:  "topup",
		ReferenceID:    strconv.FormatUint(uint64(topup.ID), 10),
	}, false)
	if err != nil {
		return err
	}
	if reversal.BalanceAfter >= 0 || len(activeResources) == 0 {
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
	if err := tx.Model(&models.BillableResource{}).Where("id IN ?", resourceIDs).Update("billing_status", models.BillableResourceStatusPaymentDue).Error; err != nil {
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

// staleTopupRecoveryThreshold is how long a checkout may stay in the
// "creating" request state before the sweeper considers the creating process
// dead and recovers it. It is deliberately much longer than the request
// timeout so an in-flight Create is never interrupted.
const staleTopupRecoveryThreshold = 15 * time.Minute

// RecoverStaleTopups is the background recovery path for checkouts whose
// creating process died after claiming the provider request but before storing
// the Snap token (provider_request_state = "creating", no token). Without it,
// such top-ups wait forever for the user to press reconcile.
//
// For each stale top-up it re-runs the normal recovery (which safely resets a
// dead claim) and then reconciles against the provider: if Midtrans already
// recorded a transaction for the order ID its status is applied, so a payment
// the user completed during the outage is still credited.
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
		paymentURL = res.PaymentNumber
		if !strings.HasPrefix(paymentURL, "http://") && !strings.HasPrefix(paymentURL, "https://") {
			slug := ""
			if s.cfg != nil {
				slug = s.cfg.PakasirProjectSlug
			}
			if slug != "" {
				redirectURL := ""
				if s.cfg != nil {
					if s.cfg.FrontendURL != "" {
						redirectURL = strings.TrimRight(s.cfg.FrontendURL, "/") + "/billing"
					} else if s.cfg.BaseDomain != "" {
						redirectURL = "https://" + strings.TrimRight(s.cfg.BaseDomain, "/") + "/billing"
					}
				}
				paymentURL = fmt.Sprintf("https://app.pakasir.com/pay/%s/%d?order_id=%s&qris_only=1", slug, topup.AmountMinor, topup.ProviderOrderID)
				if redirectURL != "" {
					paymentURL += "&redirect=" + url.QueryEscape(redirectURL)
				}
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

func (s *TopupService) validateCreate(ctx context.Context, userID uint, clientKey string, input TopupInput) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	if !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
	}
	if userID == 0 || clientKey == "" || len(clientKey) > 255 || strings.TrimSpace(clientKey) != clientKey {
		return ErrInvalidTopupInput
	}
	if input.PackageID == 0 && input.AmountMinor == 0 {
		return ErrInvalidTopupInput
	}
	// ponytail: IDR-only custom topup; extend when adding USD
	if input.PackageID == 0 {
		if input.AmountMinor < 10_000 || input.AmountMinor > 10_000_000 || input.AmountMinor%1000 != 0 {
			return ErrInvalidTopupInput
		}
	}
	return nil
}

func (s *TopupService) validatePaymentProcessing(ctx context.Context) error {
	if s == nil || s.db == nil || s.wallets == nil {
		return ErrTopupDisabled
	}
	if ctx == nil {
		return ErrTopupDisabled
	}
	if s.cfg != nil && !s.cfg.BillingEnabled && !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
	}
	provider := s.activeProvider()
	if provider == models.BillingProviderPakasir {
		if s.pakasirGateway == nil {
			return ErrTopupDisabled
		}
	} else {
		if s.gateway == nil {
			return ErrTopupDisabled
		}
	}
	return nil
}

func isReversalTopupStatus(status models.TopupStatus) bool {
	return status == models.TopupStatusPartialRefund || status == models.TopupStatusRefunded || status == models.TopupStatusPartialChargeback || status == models.TopupStatusChargeback
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

func (c *MidtransClient) CreatePayment(ctx context.Context, payment MidtransPaymentRequest) (MidtransPaymentResponse, error) {
	if c == nil || c.httpClient == nil || c.serverKey == "" || payment.OrderID == "" || payment.AmountMinor <= 0 || payment.Currency != models.BillingCurrencyIDR {
		return MidtransPaymentResponse{}, ErrPaymentProvider
	}
	// Snap expects gross_amount in major units. IDR is zero-decimal so this is identity
	// today, but the conversion is explicit so adding a decimal currency cannot 100x a charge.
	grossAmount, err := majorUnits(payment.AmountMinor, payment.Currency)
	if err != nil {
		return MidtransPaymentResponse{}, ErrPaymentProvider
	}
	type snapCallbacks struct {
		Finish string `json:"finish,omitempty"`
	}
	body, err := json.Marshal(struct {
		TransactionDetails struct {
			OrderID     string `json:"order_id"`
			GrossAmount int64  `json:"gross_amount"`
		} `json:"transaction_details"`
		Callbacks *snapCallbacks `json:"callbacks,omitempty"`
	}{TransactionDetails: struct {
		OrderID     string `json:"order_id"`
		GrossAmount int64  `json:"gross_amount"`
	}{OrderID: payment.OrderID, GrossAmount: grossAmount}, Callbacks: func() *snapCallbacks {
		if payment.FinishURL != "" {
			return &snapCallbacks{Finish: payment.FinishURL}
		}
		return nil
	}()})
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
	case "partial_refund":
		if statusCode == "200" {
			return models.TopupStatusPartialRefund, nil
		}
	case "partial_chargeback":
		if statusCode == "200" {
			return models.TopupStatusPartialChargeback, nil
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

// majorUnits converts a minor-unit amount to whole major units, refusing any amount that
// is not a whole number of major units — providers that take major units cannot express
// the remainder, and silently truncating would undercharge.
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




func (s *TopupService) ProcessPakasirWebhook(ctx context.Context, orderID string, amountMinor int64, webhookStatus string) error {
	if s.cfg != nil && !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
	}
	if orderID == "" || amountMinor <= 0 {
		return ErrInvalidPaymentNotification
	}

	detail, err := s.pakasirGateway.GetTransactionDetail(ctx, orderID, amountMinor)
	if err != nil {
		return fmt.Errorf("verify transaction detail with pakasir: %w", err)
	}

	if detail.OrderID != orderID || detail.Amount != amountMinor {
		return ErrInvalidPaymentNotification
	}

	status, err := pakAsirStatusToTopupStatus(detail.Status)
	if err != nil {
		return ErrInvalidPaymentNotification
	}

	var topup models.Topup
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND provider_order_id = ?", models.BillingProviderPakasir, orderID).First(&topup).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopupNotFound
			}
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
			if err := tx.Save(&topup).Error; err != nil {
				return fmt.Errorf("save paid topup: %w", err)
			}
			walletUserID, err := loadWalletUserID(tx, topup.WalletID)
			if err != nil {
				return err
			}
			idempotencyKey := fmt.Sprintf("topup:%s:%d", models.BillingProviderPakasir, topup.ID)
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
		case models.TopupStatusFailed, models.TopupStatusExpired:
			topup.Status = status
			if err := tx.Save(&topup).Error; err != nil {
				return fmt.Errorf("save failed topup: %w", err)
			}
		}
		return nil
	})

	return err
}
