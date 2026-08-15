package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCatalogServiceUnavailable = errors.New("billing catalog service unavailable")
	ErrInvalidCatalogInput       = errors.New("invalid billing catalog input")
	ErrWalletNotFound            = errors.New("wallet not found")
)

const (
	maxBillableCPUMillicores  = 128000
	maxBillableMemoryMB       = 262144
	maxBillableStorageGB      = 65536
	maxBillableMonthlyCredits = 1000000000
	maxConnectionLimit        = 1000000
	maxBackupRetentionDays    = 3650
	maxTopupPackageCredits    = 1000000000
	maxTopupPackageAmount     = 1000000000
	maxTopupPackageSortOrder  = 10000
	maxBillingAuditReason     = 500
	maxAdminCollectionPage    = 1000000
	maxAdminCollectionLimit   = 100
)

type BillableSpecInput struct {
	Type                models.BillableType `json:"type"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	CPUMillicores       int                 `json:"cpu_millicores"`
	MemoryMB            int                 `json:"memory_mb"`
	StorageGB           int                 `json:"storage_gb,omitempty"`
	MonthlyCredits      int64               `json:"monthly_credits"`
	ConnectionLimit     *int                `json:"connection_limit,omitempty"`
	BackupRetentionDays *int                `json:"backup_retention_days,omitempty"`
	BadgeText           string              `json:"badge_text,omitempty"`
	Reason              string              `json:"reason"`
}

type TopupPackageInput struct {
	Credits int64 `json:"credits"`
	// Currency defaults to IDR when empty so existing admin clients keep working.
	Currency    string `json:"currency,omitempty"`
	AmountMinor int64  `json:"amount_minor"`
	SortOrder   int    `json:"sort_order"`
	Reason      string `json:"reason"`
}

// TopupPackageUpdateInput reuses the same numeric fields plus reason.
// The backend creates a new version and deactivates the old package.
type TopupPackageUpdateInput struct {
	Credits     int64  `json:"credits"`
	AmountMinor int64  `json:"amount_minor"`
	SortOrder   int    `json:"sort_order"`
	Reason      string `json:"reason"`
}

type WalletCreditAdjustmentInput struct {
	UserID  uint   `json:"user_id,omitempty"`
	Credits int64  `json:"credits"`
	Reason  string `json:"reason"`
}

type AuditContext struct {
	ActorUserID     uint
	EffectiveUserID uint
	ActorRole       string
	SourceIP        string
	Reason          string
	RequestID       string
}

type CatalogSpec struct {
	ID                  uint                `json:"id"`
	Type                models.BillableType `json:"type"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	CPUMillicores       int                 `json:"cpu_millicores"`
	MemoryMB            int                 `json:"memory_mb"`
	StorageGB           int                 `json:"storage_gb"`
	MonthlyCredits      int64               `json:"monthly_credits"`
	ConnectionLimit     *int                `json:"connection_limit,omitempty"`
	BackupRetentionDays *int                `json:"backup_retention_days,omitempty"`
	BadgeText           string              `json:"badge_text,omitempty"`
}

type CatalogPackage struct {
	ID          uint   `json:"id"`
	Credits     int64  `json:"credits"`
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amount_minor"`
	SortOrder   int    `json:"sort_order"`
}

type Catalog struct {
	Specs    []CatalogSpec    `json:"specs"`
	Packages []CatalogPackage `json:"packages"`
}

type AdminCatalogSpec struct {
	ID                  uint                `json:"id"`
	Version             int                 `json:"version"`
	IsActive            bool                `json:"is_active"`
	Type                models.BillableType `json:"type"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	CPUMillicores       int                 `json:"cpu_millicores"`
	MemoryMB            int                 `json:"memory_mb"`
	StorageGB           int                 `json:"storage_gb"`
	MonthlyCredits      int64               `json:"monthly_credits"`
	ConnectionLimit     *int                `json:"connection_limit,omitempty"`
	BackupRetentionDays *int                `json:"backup_retention_days,omitempty"`
	BadgeText           string              `json:"badge_text,omitempty"`
}

type AdminCatalogPackage struct {
	ID          uint   `json:"id"`
	Version     int    `json:"version"`
	IsActive    bool   `json:"is_active"`
	Credits     int64  `json:"credits"`
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amount_minor"`
	SortOrder   int    `json:"sort_order"`
}

type AdminCatalog struct {
	Specs    []AdminCatalogSpec    `json:"specs"`
	Packages []AdminCatalogPackage `json:"packages"`
}

type WalletLedgerEntryView struct {
	Type          models.WalletLedgerEntryType `json:"type"`
	AmountCredits int64                        `json:"amount_credits"`
	BalanceAfter  int64                        `json:"balance_after"`
	CreatedAt     string                       `json:"created_at"`
}

type WalletView struct {
	BalanceCredits int64                   `json:"balance_credits"`
	LedgerEntries  []WalletLedgerEntryView `json:"ledger_entries"`
}

type InvoiceView struct {
	ID           uint                 `json:"id"`
	PeriodStart  time.Time            `json:"period_start"`
	PeriodEnd    time.Time            `json:"period_end"`
	TotalCredits int64                `json:"total_credits"`
	Status       models.InvoiceStatus `json:"status"`
	DueAt        *time.Time           `json:"due_at,omitempty"`
	PaidAt       *time.Time           `json:"paid_at,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
}

type TopupHistoryView struct {
	ID          uint               `json:"id"`
	Credits     int64              `json:"credits"`
	AmountMinor int64              `json:"amount_minor"`
	Currency    string             `json:"currency"`
	Status      models.TopupStatus `json:"status"`
	PaidAt      *time.Time         `json:"paid_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

type BillableResourceView struct {
	ResourceID         uint                          `json:"resource_id"`
	ResourceType       models.BillableType           `json:"resource_type"`
	ResourceName       string                        `json:"resource_name"`
	SpecName           string                        `json:"spec_name"`
	CPUMillicores      int                           `json:"cpu_millicores,omitempty"`
	MemoryMB           int                           `json:"memory_mb,omitempty"`
	StorageGB          int                           `json:"storage_gb,omitempty"`
	Engine             string                        `json:"engine,omitempty"`
	MonthlyCredits     int64                         `json:"monthly_credits"`
	Status             models.BillableResourceStatus `json:"status"`
	AutoRenew          bool                          `json:"auto_renew"`
	CurrentPeriodStart time.Time                     `json:"current_period_start"`
	NextInvoiceAt      time.Time                     `json:"next_invoice_at"`
}

type OwnBillingOverview struct {
	Wallet                  WalletView             `json:"wallet"`
	Invoices                []InvoiceView          `json:"invoices"`
	Topups                  []TopupHistoryView     `json:"topups"`
	Resources               []BillableResourceView `json:"resources"`
	UpcomingRequiredCredits int64                  `json:"upcoming_required_credits"`
}

// AdminCollection is a bounded, sanitized billing history page.
type AdminCollection[T any] struct {
	Data  []T   `json:"data"`
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type AdminWalletView struct {
	UserID         uint      `json:"user_id"`
	BalanceCredits int64     `json:"balance_credits"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type AdminInvoiceView struct {
	UserID uint `json:"user_id"`
	InvoiceView
}

type AdminTopupView struct {
	UserID uint `json:"user_id"`
	TopupHistoryView
}

type CatalogService struct {
	db             *gorm.DB
	wallets *WalletService
	cfg     *config.Config
}

func NewCatalogService(db *gorm.DB, cfg ...*config.Config) *CatalogService {
	var c *config.Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &CatalogService{db: db, cfg: c}
}

func NewCatalogServiceWithWallets(db *gorm.DB, wallets *WalletService, cfg ...*config.Config) *CatalogService {
	var c *config.Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &CatalogService{db: db, wallets: wallets, cfg: c}
}

func (s *CatalogService) ListActive(ctx context.Context) (Catalog, error) {
	specs, packages, err := s.listModels(ctx, true)
	if err != nil {
		return Catalog{}, err
	}
	return catalogFromModels(specs, packages), nil
}

func (s *CatalogService) ListAll(ctx context.Context) (AdminCatalog, error) {
	specs, packages, err := s.listModels(ctx, false)
	if err != nil {
		return AdminCatalog{}, err
	}
	return adminCatalogFromModels(specs, packages), nil
}

func (s *CatalogService) GetOwnBillingOverview(ctx context.Context, userID uint) (OwnBillingOverview, error) {
	if err := s.validateContext(ctx); err != nil {
		return OwnBillingOverview{}, err
	}
	if userID == 0 {
		return OwnBillingOverview{}, ErrInvalidCatalogInput
	}

	overview := OwnBillingOverview{
		Wallet:    WalletView{LedgerEntries: []WalletLedgerEntryView{}},
		Invoices:  []InvoiceView{},
		Topups:    []TopupHistoryView{},
		Resources: []BillableResourceView{},
	}
	if wallet, err := s.GetWalletView(ctx, userID); err == nil {
		overview.Wallet = wallet
	} else if !errors.Is(err, ErrWalletNotFound) {
		return OwnBillingOverview{}, err
	}

	var invoices []models.Invoice
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("period_start DESC, id DESC").Limit(50).Find(&invoices).Error; err != nil {
		return OwnBillingOverview{}, fmt.Errorf("list own invoices: %w", err)
	}
	for _, invoice := range invoices {
		overview.Invoices = append(overview.Invoices, InvoiceView{ID: invoice.ID, PeriodStart: invoice.PeriodStart, PeriodEnd: invoice.PeriodEnd, TotalCredits: invoice.TotalCredits, Status: invoice.Status, DueAt: invoice.DueAt, PaidAt: invoice.PaidAt, CreatedAt: invoice.CreatedAt})
	}

	var topups []models.Topup
	if err := s.db.WithContext(ctx).Joins("JOIN wallets ON wallets.id = topups.wallet_id").Where("wallets.user_id = ?", userID).Order("topups.created_at DESC, topups.id DESC").Limit(50).Find(&topups).Error; err != nil {
		return OwnBillingOverview{}, fmt.Errorf("list own topups: %w", err)
	}
	for _, topup := range topups {
		overview.Topups = append(overview.Topups, TopupHistoryView{ID: topup.ID, Credits: topup.Credits, AmountMinor: topup.AmountMinor, Currency: topup.Currency, Status: topup.Status, PaidAt: topup.PaidAt, CreatedAt: topup.CreatedAt})
	}
	resources, err := s.listOwnBillableResources(ctx, userID)
	if err != nil {
		return OwnBillingOverview{}, err
	}
	overview.Resources = resources
	if err := s.db.WithContext(ctx).Model(&models.BillableResource{}).
		Joins("JOIN billable_specs ON billable_specs.id = billable_resources.spec_id").
		Where("billable_resources.user_id = ? AND billable_resources.billing_status = ? AND billable_resources.auto_renew = ?", userID, models.BillableResourceStatusActive, true).
		Where(`
			(billable_resources.type = ? AND EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?))
			OR (billable_resources.type = ? AND EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?))
		`, models.BillableTypeProject, models.StatusDeleting, models.BillableTypeDatabase, models.DBStatusDeleted).
		Select("COALESCE(SUM(billable_specs.monthly_credits), 0)").
		Scan(&overview.UpcomingRequiredCredits).Error; err != nil {
		return OwnBillingOverview{}, fmt.Errorf("sum upcoming billing requirement: %w", err)
	}
	return overview, nil
}

func (s *CatalogService) listOwnBillableResources(ctx context.Context, userID uint) ([]BillableResourceView, error) {
	// Clean up any orphaned billable resources for this user whose target project or database no longer exists
	_ = s.db.WithContext(ctx).Model(&models.BillableResource{}).
		Where("user_id = ? AND billing_status <> ?", userID, models.BillableResourceStatusDeleted).
		Where(`
			(type = ? AND NOT EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?))
			OR (type = ? AND NOT EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?))
		`, models.BillableTypeProject, models.StatusDeleting, models.BillableTypeDatabase, models.DBStatusDeleted).
		Update("billing_status", models.BillableResourceStatusDeleted).Error

	var resources []models.BillableResource
	if err := s.db.WithContext(ctx).
		Preload("Spec").
		Where("billable_resources.user_id = ? AND billable_resources.billing_status <> ?", userID, models.BillableResourceStatusDeleted).
		Where(`
			(billable_resources.type = ? AND EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?))
			OR (billable_resources.type = ? AND EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?))
		`, models.BillableTypeProject, models.StatusDeleting, models.BillableTypeDatabase, models.DBStatusDeleted).
		Order("billable_resources.next_invoice_at ASC, billable_resources.id ASC").
		Find(&resources).Error; err != nil {
		return nil, fmt.Errorf("list own billable resources: %w", err)
	}

	projectIDs := make([]uint, 0, len(resources))
	databaseIDs := make([]uint, 0, len(resources))
	for _, resource := range resources {
		switch resource.Type {
		case models.BillableTypeProject:
			projectIDs = append(projectIDs, resource.ResourceID)
		case models.BillableTypeDatabase:
			databaseIDs = append(databaseIDs, resource.ResourceID)
		}
	}

	projectNames := make(map[uint]string, len(projectIDs))
	if len(projectIDs) > 0 {
		var projects []models.Project
		if err := s.db.WithContext(ctx).Select("id, name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
			return nil, fmt.Errorf("list own billable projects: %w", err)
		}
		for _, project := range projects {
			projectNames[project.ID] = project.Name
		}
	}

	databaseNames := make(map[uint]string, len(databaseIDs))
	databaseEngines := make(map[uint]string, len(databaseIDs))
	if len(databaseIDs) > 0 {
		type dbInfo struct {
			ID     uint
			Name   string
			Engine string
		}
		var databases []dbInfo
		if err := s.db.WithContext(ctx).Model(&models.DatabaseInstance{}).Select("id, name, engine").Where("id IN ?", databaseIDs).Find(&databases).Error; err != nil {
			return nil, fmt.Errorf("list own billable databases: %w", err)
		}
		for _, database := range databases {
			databaseNames[database.ID] = database.Name
			databaseEngines[database.ID] = database.Engine
		}
	}

	views := make([]BillableResourceView, 0, len(resources))
	orphanedIDs := make([]uint, 0)
	for _, resource := range resources {
		resourceName := databaseNames[resource.ResourceID]
		if resource.Type == models.BillableTypeProject {
			resourceName = projectNames[resource.ResourceID]
		}
		if resourceName == "" {
			orphanedIDs = append(orphanedIDs, resource.ID)
			continue
		}
		views = append(views, BillableResourceView{
			ResourceID:         resource.ResourceID,
			ResourceType:       resource.Type,
			ResourceName:       resourceName,
			SpecName:           resource.Spec.Name,
			CPUMillicores:      resource.Spec.CPUMillicores,
			MemoryMB:           resource.Spec.MemoryMB,
			StorageGB:          resource.Spec.StorageGB,
			Engine:             databaseEngines[resource.ResourceID],
			MonthlyCredits:     resource.Spec.MonthlyCredits,
			Status:             resource.BillingStatus,
			AutoRenew:          resource.AutoRenew,
			CurrentPeriodStart: resource.CurrentPeriodStart,
			NextInvoiceAt:      resource.NextInvoiceAt,
		})
	}

	if len(orphanedIDs) > 0 {
		_ = s.db.WithContext(ctx).Model(&models.BillableResource{}).
			Where("id IN ?", orphanedIDs).
			Update("billing_status", models.BillableResourceStatusDeleted).Error
	}

	return views, nil
}

func (s *CatalogService) listModels(ctx context.Context, activeOnly bool) ([]models.BillableSpec, []models.TopupPackage, error) {
	if err := s.validateContext(ctx); err != nil {
		return nil, nil, err
	}
	db := s.db.WithContext(ctx)
	if activeOnly {
		db = db.Where("is_active = ?", true)
	}
	var specs []models.BillableSpec
	if err := db.Order("type ASC, name ASC, version DESC").Find(&specs).Error; err != nil {
		return nil, nil, fmt.Errorf("list billable specs: %w", err)
	}
	db = s.db.WithContext(ctx)
	if activeOnly {
		db = db.Where("is_active = ?", true)
	}
	var packages []models.TopupPackage
	if err := db.Order("sort_order ASC, credits ASC, version DESC").Find(&packages).Error; err != nil {
		return nil, nil, fmt.Errorf("list topup packages: %w", err)
	}
	return specs, packages, nil
}

func (s *CatalogService) CreateBillableSpec(ctx context.Context, audit AuditContext, input BillableSpecInput) (CatalogSpec, error) {
	if err := s.validateContext(ctx); err != nil {
		return CatalogSpec{}, err
	}
	if err := validateBillableSpecInput(audit, input); err != nil {
		return CatalogSpec{}, err
	}

	var created models.BillableSpec
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCatalogIdentity(tx, fmt.Sprintf("spec:%s:%s", input.Type, input.Slug)); err != nil {
			return err
		}

		var previous models.BillableSpec
		previousFound := true
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND slug = ? AND is_active = ?", input.Type, input.Slug, true).First(&previous).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("lock active billable spec: %w", err)
			}
			previousFound = false
		}

		var latestVersion int
		if err := tx.Model(&models.BillableSpec{}).Where("type = ? AND slug = ?", input.Type, input.Slug).Select("COALESCE(MAX(version), 0)").Scan(&latestVersion).Error; err != nil {
			return fmt.Errorf("find billable spec version: %w", err)
		}
		if previousFound {
			if err := tx.Model(&previous).Update("is_active", false).Error; err != nil {
				return fmt.Errorf("deactivate billable spec: %w", err)
			}
		}

		created = models.BillableSpec{
			Type:                input.Type,
			Name:                input.Name,
			Slug:                input.Slug,
			CPUMillicores:       input.CPUMillicores,
			MemoryMB:            input.MemoryMB,
			StorageGB:           input.StorageGB,
			MonthlyCredits:      input.MonthlyCredits,
			ConnectionLimit:     input.ConnectionLimit,
			BackupRetentionDays: input.BackupRetentionDays,
			BadgeText:           strings.TrimSpace(input.BadgeText),
			Version:             latestVersion + 1,
			IsActive:            true,
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("create billable spec: %w", err)
		}
		event := "billable_spec.created"
		var before any
		if previousFound {
			event = "billable_spec.repriced"
			before = previous
		}
		return createAuditEvent(tx, audit, event, "billable_spec", created.ID, before, created)
	})
	if err != nil {
		return CatalogSpec{}, err
	}
	return catalogSpecFromModel(created), nil
}

func (s *CatalogService) CreateTopupPackage(ctx context.Context, audit AuditContext, input TopupPackageInput) (CatalogPackage, error) {
	if err := s.validateContext(ctx); err != nil {
		return CatalogPackage{}, err
	}
	if input.Currency == "" {
		input.Currency = models.BillingCurrencyIDR
	}
	if err := validateTopupPackageInput(audit, input); err != nil {
		return CatalogPackage{}, err
	}

	var created models.TopupPackage
	var previous *models.TopupPackage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, previous, err = s.createTopupPackageVersion(tx, input.Currency, input.Credits, input.AmountMinor, input.SortOrder)
		if err != nil {
			return err
		}
		event := "topup_package.created"
		if previous != nil {
			event = "topup_package.repriced"
		}
		return createAuditEvent(tx, audit, event, "topup_package", created.ID, previous, created)
	})
	if err != nil {
		return CatalogPackage{}, err
	}

	return catalogPackageFromModel(created), nil
}

// UpdateTopupPackage edits an active package by creating a new version and
// deactivating the old one. The credits identity must stay the same.
func (s *CatalogService) UpdateTopupPackage(ctx context.Context, audit AuditContext, id uint, input TopupPackageUpdateInput) (CatalogPackage, error) {
	if err := s.validateContext(ctx); err != nil {
		return CatalogPackage{}, err
	}
	if err := validateTopupPackageUpdateInput(audit, input); err != nil {
		return CatalogPackage{}, err
	}

	var current models.TopupPackage
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CatalogPackage{}, apperr.ErrNotFound
		}
		return CatalogPackage{}, fmt.Errorf("load topup package: %w", err)
	}
	if !current.IsActive {
		return CatalogPackage{}, ErrInvalidCatalogInput
	}

	credits := current.Credits
	if input.Credits > 0 {
		credits = input.Credits
	}

	var created models.TopupPackage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		created, _, err = s.createTopupPackageVersion(tx, current.Currency, credits, input.AmountMinor, input.SortOrder)
		if err != nil {
			return err
		}
		if current.Credits != credits && current.IsActive {
			if err := tx.Model(&current).Update("is_active", false).Error; err != nil {
				return fmt.Errorf("deactivate topup package: %w", err)
			}
		}
		return createAuditEvent(tx, audit, "topup_package.repriced", "topup_package", created.ID, &current, created)
	})
	if err != nil {
		return CatalogPackage{}, err
	}

	return catalogPackageFromModel(created), nil
}

func (s *CatalogService) createTopupPackageVersion(tx *gorm.DB, currency string, credits, amountMinor int64, sortOrder int) (models.TopupPackage, *models.TopupPackage, error) {
	if err := lockCatalogIdentity(tx, fmt.Sprintf("package:%s:%d", currency, credits)); err != nil {
		return models.TopupPackage{}, nil, err
	}

	var previous models.TopupPackage
	previousFound := true
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("currency = ? AND credits = ? AND is_active = ?", currency, credits, true).First(&previous).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return models.TopupPackage{}, nil, fmt.Errorf("lock active topup package: %w", err)
		}
		previousFound = false
	}

	var latestVersion int
	if err := tx.Model(&models.TopupPackage{}).Where("currency = ? AND credits = ?", currency, credits).Select("COALESCE(MAX(version), 0)").Scan(&latestVersion).Error; err != nil {
		return models.TopupPackage{}, nil, fmt.Errorf("find topup package version: %w", err)
	}
	if previousFound {
		if err := tx.Model(&previous).Update("is_active", false).Error; err != nil {
			return models.TopupPackage{}, nil, fmt.Errorf("deactivate topup package: %w", err)
		}
	}

	created := models.TopupPackage{
		Currency:    currency,
		Credits:     credits,
		AmountMinor: amountMinor,
		Version:     latestVersion + 1,
		IsActive:    true,
		SortOrder:   sortOrder,
	}
	if err := tx.Create(&created).Error; err != nil {
		return models.TopupPackage{}, nil, fmt.Errorf("create topup package: %w", err)
	}
	var previousPtr *models.TopupPackage
	if previousFound {
		previousPtr = &previous
	}
	return created, previousPtr, nil
}

type UpdatePaymentProviderInput struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason"`
}

func (s *CatalogService) UpdateDefaultPaymentProvider(ctx context.Context, audit AuditContext, input UpdatePaymentProviderInput) error {
	if err := s.validateContext(ctx); err != nil {
		return err
	}
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.Reason = strings.TrimSpace(input.Reason)
	audit.Reason = strings.TrimSpace(audit.Reason)
	if !validAuditContext(audit) {
		return ErrInvalidCatalogInput
	}
	switch input.Provider {
	case models.BillingProviderPakasir:
		if s.cfg != nil && !s.cfg.PakasirEnabled {
			return fmt.Errorf("%w: cannot switch to Pakasir because PAKASIR_ENABLED=false", ErrInvalidCatalogInput)
		}
		if s.cfg != nil && (s.cfg.PakasirProjectSlug == "" || s.cfg.PakasirAPIKey == "") {
			return fmt.Errorf("%w: Pakasir credentials incomplete", ErrInvalidCatalogInput)
		}
	case models.BillingProviderMidtrans:
		if s.cfg != nil && (s.cfg.MidtransServerKey == "" || s.cfg.MidtransMerchantID == "") {
			return fmt.Errorf("%w: Midtrans credentials incomplete", ErrInvalidCatalogInput)
		}
	default:
		return fmt.Errorf("%w: invalid provider %q", ErrInvalidCatalogInput, input.Provider)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Ensure the setting row exists so FOR UPDATE serializes concurrent first writes
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "setting_key"}},
			DoNothing: true,
		}).Create(&models.Setting{
			Key:   models.SettingDefaultPaymentProvider,
			Value: "",
		}).Error; err != nil {
			return fmt.Errorf("ensure payment provider setting row: %w", err)
		}

		var setting models.Setting
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("setting_key = ?", models.SettingDefaultPaymentProvider).First(&setting).Error; err != nil {
			return fmt.Errorf("lock payment provider setting: %w", err)
		}

		oldValue := setting.Value
		if oldValue == input.Provider {
			return nil
		}

		if err := tx.Model(&setting).Update("value", input.Provider).Error; err != nil {
			return fmt.Errorf("update payment provider setting: %w", err)
		}

		if err := createAuditEvent(tx, audit, "update_payment_provider", "setting", 0, map[string]string{"provider": oldValue}, map[string]string{"provider": input.Provider}); err != nil {
			return fmt.Errorf("audit payment provider update: %w", err)
		}
		return nil
	})
}

// AdjustWalletCredits applies a manual credit adjustment to a user wallet.
// Positive credits adds balance; negative debits balance (cannot overdraw).
func (s *CatalogService) AdjustWalletCredits(ctx context.Context, audit AuditContext, userID uint, idempotencyKey string, input WalletCreditAdjustmentInput) (WalletView, error) {
	if err := s.validateContext(ctx); err != nil {
		return WalletView{}, err
	}
	if userID == 0 || idempotencyKey == "" {
		return WalletView{}, ErrInvalidCatalogInput
	}
	input.Reason = strings.TrimSpace(input.Reason)
	audit.Reason = strings.TrimSpace(audit.Reason)
	if !validAuditContext(audit) {
		return WalletView{}, ErrInvalidCatalogInput
	}
	if err := validateWalletCreditAdjustmentInput(input); err != nil {
		return WalletView{}, err
	}
	if s.wallets == nil {
		return WalletView{}, ErrCatalogServiceUnavailable
	}

	actorUserID := audit.ActorUserID
	mutation := LedgerMutation{
		UserID:         userID,
		EntryType:      models.WalletLedgerEntryAdjustment,
		AmountCredits:  input.Credits,
		IdempotencyKey: fmt.Sprintf("admin:adjustment:%s", idempotencyKey),
		ReferenceType:  "admin_adjustment",
		ReferenceID:    fmt.Sprintf("user:%d", userID),
		CreatedBy:      &actorUserID,
	}

	credit := input.Credits > 0
	if !credit {
		// WalletService.Debit expects a positive amount; negation happens inside.
		mutation.AmountCredits = -input.Credits
	}

	var wallet models.Wallet
	var mutResult MutationResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Select("id").Where("id = ?", userID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.ErrNotFound
			}
			return fmt.Errorf("load user: %w", err)
		}

		var err error
		wallet, err = getOrCreateLockedWallet(tx, userID)
		if err != nil {
			return err
		}
		mutResult, err = s.wallets.applyInTransaction(tx, mutation, credit)
		if err != nil {
			return err
		}
		if mutResult.Replayed {
			return nil
		}
		return createAuditEvent(tx, audit, "wallet.credit_adjusted", "wallet", wallet.ID, nil, mutResult)
	})
	if err != nil {
		if errors.Is(err, ErrInsufficientBalance) {
			return WalletView{}, apperr.NewBadRequest("Insufficient wallet balance for debit")
		}
		if errors.Is(err, ErrInvalidMutation) {
			return WalletView{}, apperr.NewBadRequest("Invalid credit adjustment")
		}
		return WalletView{}, err
	}
	if credit {
		s.retryDueInvoicesAfterCredit(ctx, userID)
	}

	return s.GetWalletView(ctx, userID)
}

func (s *CatalogService) retryDueInvoicesAfterCredit(ctx context.Context, userID uint) {
	// ponytail: keep an admin adjustment responsive; the minute scheduler is
	// the durable fallback if this best-effort post-commit retry fails.
	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), invoiceRetryTimeout)
	defer cancel()
	if err := NewInvoiceService(s.db, s.wallets).RetryDueForUser(retryCtx, userID, time.Now().UTC()); err != nil {
		slog.Error("Billing invoice retry after wallet adjustment failed", "user_id", userID, "error", err)
	}
}

func (s *CatalogService) GetWalletView(ctx context.Context, userID uint) (WalletView, error) {
	if err := s.validateContext(ctx); err != nil {
		return WalletView{}, err
	}
	if userID == 0 {
		return WalletView{}, ErrWalletNotFound
	}
	var wallet models.Wallet
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&wallet).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return WalletView{}, fmt.Errorf("load wallet: %w", err)
		}
		if s.wallets != nil {
			var user models.User
			if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return WalletView{}, ErrWalletNotFound
				}
				return WalletView{}, fmt.Errorf("load user: %w", err)
			}
			wallet, err = s.wallets.GetOrCreateWallet(ctx, userID)
			if err != nil {
				return WalletView{}, fmt.Errorf("get or create wallet: %w", err)
			}
		} else {
			return WalletView{}, ErrWalletNotFound
		}
	}
	var entries []models.WalletLedgerEntry
	if err := s.db.WithContext(ctx).Where("wallet_id = ?", wallet.ID).Order("id DESC").Limit(100).Find(&entries).Error; err != nil {
		return WalletView{}, fmt.Errorf("list wallet ledger entries: %w", err)
	}
	view := WalletView{BalanceCredits: wallet.BalanceCredits, LedgerEntries: make([]WalletLedgerEntryView, 0, len(entries))}
	for _, entry := range entries {
		view.LedgerEntries = append(view.LedgerEntries, WalletLedgerEntryView{
			Type:          entry.Type,
			AmountCredits: entry.AmountCredits,
			BalanceAfter:  entry.BalanceAfter,
			CreatedAt:     entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return view, nil
}

func (s *CatalogService) ListAdminWallets(ctx context.Context, page, limit int) (AdminCollection[AdminWalletView], error) {
	page, limit, err := normalizeAdminCollectionPage(page, limit)
	if err != nil {
		return AdminCollection[AdminWalletView]{}, err
	}
	if err := s.validateContext(ctx); err != nil {
		return AdminCollection[AdminWalletView]{}, err
	}

	query := s.db.WithContext(ctx).Model(&models.Wallet{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return AdminCollection[AdminWalletView]{}, fmt.Errorf("count wallets: %w", err)
	}

	var wallets []models.Wallet
	if err := query.Order("updated_at DESC, id DESC").Offset((page - 1) * limit).Limit(limit).Find(&wallets).Error; err != nil {
		return AdminCollection[AdminWalletView]{}, fmt.Errorf("list wallets: %w", err)
	}
	result := AdminCollection[AdminWalletView]{Data: make([]AdminWalletView, 0, len(wallets)), Page: page, Limit: limit, Total: total}
	for _, wallet := range wallets {
		result.Data = append(result.Data, AdminWalletView{UserID: wallet.UserID, BalanceCredits: wallet.BalanceCredits, UpdatedAt: wallet.UpdatedAt})
	}
	return result, nil
}

func (s *CatalogService) ListAdminInvoices(ctx context.Context, page, limit int) (AdminCollection[AdminInvoiceView], error) {
	page, limit, err := normalizeAdminCollectionPage(page, limit)
	if err != nil {
		return AdminCollection[AdminInvoiceView]{}, err
	}
	if err := s.validateContext(ctx); err != nil {
		return AdminCollection[AdminInvoiceView]{}, err
	}

	query := s.db.WithContext(ctx).Model(&models.Invoice{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return AdminCollection[AdminInvoiceView]{}, fmt.Errorf("count invoices: %w", err)
	}

	var invoices []models.Invoice
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * limit).Limit(limit).Find(&invoices).Error; err != nil {
		return AdminCollection[AdminInvoiceView]{}, fmt.Errorf("list invoices: %w", err)
	}
	result := AdminCollection[AdminInvoiceView]{Data: make([]AdminInvoiceView, 0, len(invoices)), Page: page, Limit: limit, Total: total}
	for _, invoice := range invoices {
		result.Data = append(result.Data, AdminInvoiceView{UserID: invoice.UserID, InvoiceView: invoiceViewFromModel(invoice)})
	}
	return result, nil
}

func (s *CatalogService) ListAdminTopups(ctx context.Context, page, limit int) (AdminCollection[AdminTopupView], error) {
	page, limit, err := normalizeAdminCollectionPage(page, limit)
	if err != nil {
		return AdminCollection[AdminTopupView]{}, err
	}
	if err := s.validateContext(ctx); err != nil {
		return AdminCollection[AdminTopupView]{}, err
	}

	query := s.db.WithContext(ctx).Model(&models.Topup{}).Joins("JOIN wallets ON wallets.id = topups.wallet_id")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return AdminCollection[AdminTopupView]{}, fmt.Errorf("count topups: %w", err)
	}

	type topupWithUser struct {
		models.Topup
		UserID uint `gorm:"column:billing_user_id"`
	}
	var topups []topupWithUser
	if err := query.Select("topups.*, wallets.user_id AS billing_user_id").Order("topups.created_at DESC, topups.id DESC").Offset((page - 1) * limit).Limit(limit).Scan(&topups).Error; err != nil {
		return AdminCollection[AdminTopupView]{}, fmt.Errorf("list topups: %w", err)
	}
	result := AdminCollection[AdminTopupView]{Data: make([]AdminTopupView, 0, len(topups)), Page: page, Limit: limit, Total: total}
	for _, topup := range topups {
		result.Data = append(result.Data, AdminTopupView{UserID: topup.UserID, TopupHistoryView: topupHistoryViewFromModel(topup.Topup)})
	}
	return result, nil
}

func normalizeAdminCollectionPage(page, limit int) (int, int, error) {
	if page < 1 || page > maxAdminCollectionPage || limit < 1 || limit > maxAdminCollectionLimit {
		return 0, 0, ErrInvalidCatalogInput
	}
	return page, limit, nil
}

func invoiceViewFromModel(invoice models.Invoice) InvoiceView {
	return InvoiceView{ID: invoice.ID, PeriodStart: invoice.PeriodStart, PeriodEnd: invoice.PeriodEnd, TotalCredits: invoice.TotalCredits, Status: invoice.Status, DueAt: invoice.DueAt, PaidAt: invoice.PaidAt, CreatedAt: invoice.CreatedAt}
}

func topupHistoryViewFromModel(topup models.Topup) TopupHistoryView {
	return TopupHistoryView{ID: topup.ID, Credits: topup.Credits, AmountMinor: topup.AmountMinor, Currency: topup.Currency, Status: topup.Status, PaidAt: topup.PaidAt, CreatedAt: topup.CreatedAt}
}

func (s *CatalogService) validateContext(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrCatalogServiceUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidCatalogInput)
	}
	return nil
}

func lockCatalogIdentity(tx *gorm.DB, identity string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", identity).Error; err != nil {
		return fmt.Errorf("lock billing catalog identity: %w", err)
	}
	return nil
}

func validateBillableSpecInput(audit AuditContext, input BillableSpecInput) error {
	if !validAuditContext(audit) || input.Reason != audit.Reason || input.Name == "" || len(input.Name) > 100 || strings.TrimSpace(input.Name) != input.Name || !validCatalogSlug(input.Slug) || input.CPUMillicores <= 0 || input.CPUMillicores > maxBillableCPUMillicores || input.MemoryMB <= 0 || input.MemoryMB > maxBillableMemoryMB || input.StorageGB <= 0 || input.StorageGB > maxBillableStorageGB || input.MonthlyCredits <= 0 || input.MonthlyCredits > maxBillableMonthlyCredits {
		return ErrInvalidCatalogInput
	}
	switch input.Type {
	case models.BillableTypeProject:
		if input.ConnectionLimit != nil || input.BackupRetentionDays != nil {
			return ErrInvalidCatalogInput
		}
	case models.BillableTypeDatabase:
		if input.ConnectionLimit == nil || *input.ConnectionLimit <= 0 || *input.ConnectionLimit > maxConnectionLimit || input.BackupRetentionDays == nil || *input.BackupRetentionDays <= 0 || *input.BackupRetentionDays > maxBackupRetentionDays {
			return ErrInvalidCatalogInput
		}
	default:
		return ErrInvalidCatalogInput
	}
	return nil
}

func validateTopupPackageInput(audit AuditContext, input TopupPackageInput) error {
	if _, supported := models.CurrencyMinorUnits(input.Currency); !supported {
		return ErrInvalidCatalogInput
	}
	if !validAuditContext(audit) || input.Reason != audit.Reason || input.Credits <= 0 || input.Credits > maxTopupPackageCredits || input.AmountMinor <= 0 || input.AmountMinor > maxTopupPackageAmount || input.SortOrder < 0 || input.SortOrder > maxTopupPackageSortOrder {
		return ErrInvalidCatalogInput
	}
	return nil
}

func validateTopupPackageUpdateInput(audit AuditContext, input TopupPackageUpdateInput) error {
	if !validAuditContext(audit) || input.Reason != audit.Reason || (input.Credits != 0 && (input.Credits <= 0 || input.Credits > maxTopupPackageCredits)) || input.AmountMinor <= 0 || input.AmountMinor > maxTopupPackageAmount || input.SortOrder < 0 || input.SortOrder > maxTopupPackageSortOrder {
		return ErrInvalidCatalogInput
	}
	return nil
}

func validateWalletCreditAdjustmentInput(input WalletCreditAdjustmentInput) error {
	reason := strings.TrimSpace(input.Reason)
	if input.Credits == 0 || input.Credits > maxTopupPackageCredits || input.Credits < -maxTopupPackageCredits || reason == "" || len(reason) > maxBillingAuditReason {
		return ErrInvalidCatalogInput
	}
	return nil
}

type UpdateAutoRenewInput struct {
	ResourceID   uint   `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	AutoRenew    bool   `json:"auto_renew"`
}

func (s *CatalogService) UpdateAutoRenew(ctx context.Context, userID uint, input UpdateAutoRenewInput) error {
	if err := s.validateContext(ctx); err != nil {

		return err
	}
	if userID == 0 {
		return ErrInvalidCatalogInput
	}

	resourceType, err := models.ParseBillableType(input.ResourceType)
	if err != nil {
		return apperr.NewBadRequest(fmt.Sprintf("Invalid resource type: %s", err.Error()))
	}
	if input.ResourceID == 0 {
		return apperr.NewBadRequest("Resource ID is required")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("type = ? AND resource_id = ? AND user_id = ?", resourceType, input.ResourceID, userID).
			First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewNotFound("RESOURCE_NOT_FOUND", "Billable resource not found")
			}
			return fmt.Errorf("lock billable resource: %w", err)
		}
		if resource.BillingStatus == models.BillableResourceStatusDeleted {
			return apperr.NewBadRequest("Resource billing is closed")
		}

		exists, existsErr := billableResourceExistsTx(tx, &resource)
		if existsErr != nil {
			return existsErr
		}
		if !exists {
			return apperr.NewNotFound("RESOURCE", "Underlying resource no longer exists")
		}

		if err := tx.Model(&resource).Update("auto_renew", input.AutoRenew).Error; err != nil {
			return fmt.Errorf("update auto_renew: %w", err)
		}

		return nil
	})
}

func validAuditContext(audit AuditContext) bool {
	reason := strings.TrimSpace(audit.Reason)
	return audit.ActorUserID > 0 && audit.EffectiveUserID > 0 && audit.ActorRole == string(models.RoleSuperAdmin) && net.ParseIP(audit.SourceIP) != nil && reason != "" && len(reason) <= maxBillingAuditReason && audit.RequestID != "" && len(audit.RequestID) <= 64
}

func validCatalogSlug(value string) bool {
	if value == "" || len(value) > 100 || strings.TrimSpace(value) != value || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func createAuditEvent(tx *gorm.DB, audit AuditContext, event, targetType string, targetID uint, before, after any) error {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("marshal audit event before state: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("marshal audit event after state: %w", err)
	}
	entry := models.BillingAuditEvent{
		ActorUserID:     audit.ActorUserID,
		EffectiveUserID: audit.EffectiveUserID,
		ActorRole:       audit.ActorRole,
		SourceIP:        audit.SourceIP,
		Reason:          audit.Reason,
		Event:           event,
		TargetType:      targetType,
		TargetID:        targetID,
		BeforeJSON:      string(beforeJSON),
		AfterJSON:       string(afterJSON),
		RequestID:       audit.RequestID,
	}
	if err := tx.Create(&entry).Error; err != nil {
		return fmt.Errorf("create billing audit event: %w", err)
	}
	return nil
}

func catalogFromModels(specs []models.BillableSpec, packages []models.TopupPackage) Catalog {
	catalog := Catalog{Specs: make([]CatalogSpec, 0, len(specs)), Packages: make([]CatalogPackage, 0, len(packages))}
	for _, spec := range specs {
		catalog.Specs = append(catalog.Specs, catalogSpecFromModel(spec))
	}
	for _, topupPackage := range packages {
		catalog.Packages = append(catalog.Packages, catalogPackageFromModel(topupPackage))
	}
	return catalog
}

func adminCatalogFromModels(specs []models.BillableSpec, packages []models.TopupPackage) AdminCatalog {
	catalog := AdminCatalog{Specs: make([]AdminCatalogSpec, 0, len(specs)), Packages: make([]AdminCatalogPackage, 0, len(packages))}
	for _, spec := range specs {
		catalog.Specs = append(catalog.Specs, adminCatalogSpecFromModel(spec))
	}
	for _, topupPackage := range packages {
		catalog.Packages = append(catalog.Packages, adminCatalogPackageFromModel(topupPackage))
	}
	return catalog
}

func catalogSpecFromModel(spec models.BillableSpec) CatalogSpec {
	return CatalogSpec{
		ID:                  spec.ID,
		Type:                spec.Type,
		Name:                spec.Name,
		Slug:                spec.Slug,
		CPUMillicores:       spec.CPUMillicores,
		MemoryMB:            spec.MemoryMB,
		StorageGB:           spec.StorageGB,
		MonthlyCredits:      spec.MonthlyCredits,
		ConnectionLimit:     spec.ConnectionLimit,
		BackupRetentionDays: spec.BackupRetentionDays,
		BadgeText:           spec.BadgeText,
	}
}

func catalogPackageFromModel(topupPackage models.TopupPackage) CatalogPackage {
	return CatalogPackage{
		ID:          topupPackage.ID,
		Credits:     topupPackage.Credits,
		Currency:    topupPackage.Currency,
		AmountMinor: topupPackage.AmountMinor,
		SortOrder:   topupPackage.SortOrder,
	}
}

func adminCatalogSpecFromModel(spec models.BillableSpec) AdminCatalogSpec {
	return AdminCatalogSpec{
		ID:                  spec.ID,
		Version:             spec.Version,
		IsActive:            spec.IsActive,
		Type:                spec.Type,
		Name:                spec.Name,
		Slug:                spec.Slug,
		CPUMillicores:       spec.CPUMillicores,
		MemoryMB:            spec.MemoryMB,
		StorageGB:           spec.StorageGB,
		MonthlyCredits:      spec.MonthlyCredits,
		ConnectionLimit:     spec.ConnectionLimit,
		BackupRetentionDays: spec.BackupRetentionDays,
		BadgeText:           spec.BadgeText,
	}
}

func adminCatalogPackageFromModel(topupPackage models.TopupPackage) AdminCatalogPackage {
	return AdminCatalogPackage{
		ID:          topupPackage.ID,
		Version:     topupPackage.Version,
		IsActive:    topupPackage.IsActive,
		Credits:     topupPackage.Credits,
		Currency:    topupPackage.Currency,
		AmountMinor: topupPackage.AmountMinor,
		SortOrder:   topupPackage.SortOrder,
	}
}
