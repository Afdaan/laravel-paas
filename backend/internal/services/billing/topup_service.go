// ===========================================
// Topup Service Orchestrator
// ===========================================
package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/services/billing/midtrans"
	"github.com/laravel-paas/backend/internal/services/billing/pakasir"
	"github.com/laravel-paas/backend/internal/services/billing/telegram"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/services/setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MidtransPaymentRequest = midtrans.PaymentRequest
type MidtransPaymentResponse = midtrans.PaymentResponse
type MidtransNotification = midtrans.Notification
type MidtransGateway = midtrans.Gateway
type MidtransClient = midtrans.Client

type PakasirCreateResponse = pakasir.CreateResponse
type PakasirTransactionDetail = pakasir.TransactionDetail
type PakasirGateway = pakasir.Gateway
type PakasirClient = pakasir.Client

func NewMidtransClient(cfg *config.Config) *midtrans.Client {
	return midtrans.NewClient(cfg)
}

func NewPakasirClient(cfg *config.Config) *pakasir.Client {
	return pakasir.NewClient(cfg)
}

func pakAsirStatusToTopupStatus(status string) (models.TopupStatus, error) {
	return pakasir.StatusToTopupStatus(status)
}

func topupStatusFromNotification(statusCode, transactionStatus, fraudStatus string) (models.TopupStatus, error) {
	status, err := midtrans.StatusFromNotification(statusCode, transactionStatus, fraudStatus)
	if err != nil {
		return "", ErrInvalidPaymentNotification
	}
	return status, nil
}

func validMidtransSignature(notification midtrans.Notification, serverKey string) bool {
	return midtrans.ValidSignature(notification, serverKey)
}

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

type TopupService struct {
	db               *gorm.DB
	wallets          *WalletService
	cfg              *config.Config
	gateway          MidtransGateway
	pakasirGateway   PakasirGateway
	telegramNotifier *telegram.Client
	settingService   *setting.SettingService
	billingProfile   *BillingProfileService
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
	return &TopupService{
		db:               db,
		wallets:          wallets,
		cfg:              cfg,
		gateway:          gateway,
		pakasirGateway:   pGateway,
		telegramNotifier: telegram.NewClient(cfg),
		billingProfile:   NewBillingProfileService(db),
	}
}

func (s *TopupService) SetTelegramNotifier(notifier *telegram.Client) {
	s.telegramNotifier = notifier
}

func (s *TopupService) SetSettingService(settingService *setting.SettingService) {
	s.settingService = settingService
}

func (s *TopupService) activeProvider(ctx context.Context) (string, error) {
	if s.settingService != nil {
		val, found, err := s.settingService.GetUncached(ctx, models.SettingDefaultPaymentProvider)
		if err != nil {
			return "", fmt.Errorf("%w: failed to query payment provider setting: %w", ErrPaymentProvider, err)
		}
		if found {
			normalized := models.NormalizePaymentProvider(val)
			if normalized == "" {
				return "", fmt.Errorf("%w: invalid default payment provider configured: %q", ErrPaymentProvider, val)
			}
			return normalized, nil
		}
	}
	if s.cfg != nil && s.cfg.BillingTopupProvider != "" {
		normalized := models.NormalizePaymentProvider(s.cfg.BillingTopupProvider)
		if normalized == "" {
			return "", fmt.Errorf("%w: invalid billing topup provider configured in env: %q", ErrPaymentProvider, s.cfg.BillingTopupProvider)
		}
		return normalized, nil
	}
	return models.BillingProviderPakasir, nil
}

func (s *TopupService) Create(ctx context.Context, userID uint, clientKey string, input TopupInput) (TopupView, error) {
	if ctx == nil {
		return TopupView{}, fmt.Errorf("%w: context is required", ErrInvalidTopupInput)
	}
	ctx, cancel := withTopupRequestDeadline(ctx)
	defer cancel()

	provider, err := s.activeProvider(ctx)
	if err != nil {
		return TopupView{}, err
	}
	if err := s.validateCreate(ctx, userID, clientKey, input, provider); err != nil {
		return TopupView{}, err
	}

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
		if err := s.ProcessPakasirWebhook(ctx, topup.ProviderOrderID, topup.AmountMinor, ""); err != nil {
			return TopupView{}, err
		}
	} else if s.gateway != nil {
		notification, err := s.gateway.GetTransactionStatus(ctx, topup.ProviderOrderID)
		if err != nil {
			return TopupView{}, err
		}
		if err := s.ProcessReconciledNotification(ctx, notification); err != nil {
			return TopupView{}, err
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

func (s *TopupService) validateCreate(ctx context.Context, userID uint, clientKey string, input TopupInput, provider string) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	if s.cfg != nil && !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
	}
	switch provider {
	case models.BillingProviderPakasir:
		if s.cfg != nil && !s.cfg.PakasirEnabled {
			return fmt.Errorf("%w: Pakasir gateway is disabled (PAKASIR_ENABLED=false)", ErrPaymentProvider)
		}
		if s.pakasirGateway == nil || (s.cfg != nil && (s.cfg.PakasirProjectSlug == "" || s.cfg.PakasirAPIKey == "")) {
			return fmt.Errorf("%w: Pakasir provider not configured", ErrPaymentProvider)
		}
	case models.BillingProviderMidtrans:
		if s.gateway == nil || (s.cfg != nil && (s.cfg.MidtransServerKey == "" || s.cfg.MidtransMerchantID == "")) {
			return fmt.Errorf("%w: Midtrans provider not configured", ErrPaymentProvider)
		}
	default:
		return fmt.Errorf("%w: unsupported payment provider %q", ErrPaymentProvider, provider)
	}
	if userID == 0 || clientKey == "" || len(clientKey) > 255 || strings.TrimSpace(clientKey) != clientKey {
		return ErrInvalidTopupInput
	}
	if input.PackageID == 0 && input.AmountMinor == 0 {
		return ErrInvalidTopupInput
	}
	if input.PackageID == 0 {
		if input.AmountMinor < 10_000 || input.AmountMinor > 10_000_000 || input.AmountMinor%1000 != 0 {
			return ErrInvalidTopupInput
		}
	}
	return nil
}

func (s *TopupService) validatePaymentProcessing(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidTopupInput)
	}
	if s.cfg != nil && (!s.cfg.BillingEnabled || !s.cfg.BillingTopupEnabled) {
		return ErrTopupDisabled
	}
	return nil
}
