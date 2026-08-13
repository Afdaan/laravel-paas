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
	db             *gorm.DB
	wallets        *WalletService
	cfg            *config.Config
	gateway        MidtransGateway
	pakasirGateway PakasirGateway
	settingService *setting.SettingService
	billingProfile *BillingProfileService
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
		if (s.cfg.PakasirEnabled || s.cfg.PakasirProjectSlug != "") && s.cfg.MidtransServerKey == "" {
			return models.BillingProviderPakasir
		}
		if s.cfg.MidtransServerKey != "" {
			return models.BillingProviderMidtrans
		}
	}
	return models.BillingProviderPakasir
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

func (s *TopupService) validateCreate(ctx context.Context, userID uint, clientKey string, input TopupInput) error {
	if err := s.validatePaymentProcessing(ctx); err != nil {
		return err
	}
	if s.cfg != nil && !s.cfg.BillingTopupEnabled {
		return ErrTopupDisabled
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
	if s.cfg != nil && (!s.cfg.BillingEnabled || !s.cfg.BillingTopupEnabled) {
		return ErrTopupDisabled
	}
	return nil
}
