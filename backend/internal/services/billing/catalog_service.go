package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

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
)

type BillableSpecInput struct {
	Type                models.BillableType `json:"type"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	CPUMillicores       int                 `json:"cpu_millicores"`
	MemoryMB            int                 `json:"memory_mb"`
	StorageGB           int                 `json:"storage_gb"`
	MonthlyCredits      int64               `json:"monthly_credits"`
	ConnectionLimit     *int                `json:"connection_limit,omitempty"`
	BackupRetentionDays *int                `json:"backup_retention_days,omitempty"`
	Reason              string              `json:"reason"`
}

type TopupPackageInput struct {
	Credits     int64  `json:"credits"`
	AmountMinor int64  `json:"amount_minor"`
	SortOrder   int    `json:"sort_order"`
	Reason      string `json:"reason"`
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
	Type                models.BillableType `json:"type"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	CPUMillicores       int                 `json:"cpu_millicores"`
	MemoryMB            int                 `json:"memory_mb"`
	StorageGB           int                 `json:"storage_gb"`
	MonthlyCredits      int64               `json:"monthly_credits"`
	ConnectionLimit     *int                `json:"connection_limit,omitempty"`
	BackupRetentionDays *int                `json:"backup_retention_days,omitempty"`
}

type CatalogPackage struct {
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

type CatalogService struct {
	db *gorm.DB
}

func NewCatalogService(db *gorm.DB) *CatalogService {
	return &CatalogService{db: db}
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
	if err := validateTopupPackageInput(audit, input); err != nil {
		return CatalogPackage{}, err
	}

	var created models.TopupPackage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockCatalogIdentity(tx, fmt.Sprintf("package:%s:%s:%d", models.BillingProviderMidtrans, models.BillingCurrencyIDR, input.Credits)); err != nil {
			return err
		}

		var previous models.TopupPackage
		previousFound := true
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND currency = ? AND credits = ? AND is_active = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, input.Credits, true).First(&previous).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("lock active topup package: %w", err)
			}
			previousFound = false
		}

		var latestVersion int
		if err := tx.Model(&models.TopupPackage{}).Where("provider = ? AND currency = ? AND credits = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, input.Credits).Select("COALESCE(MAX(version), 0)").Scan(&latestVersion).Error; err != nil {
			return fmt.Errorf("find topup package version: %w", err)
		}
		if previousFound {
			if err := tx.Model(&previous).Update("is_active", false).Error; err != nil {
				return fmt.Errorf("deactivate topup package: %w", err)
			}
		}

		created = models.TopupPackage{
			Provider:    models.BillingProviderMidtrans,
			Currency:    models.BillingCurrencyIDR,
			Credits:     input.Credits,
			AmountMinor: input.AmountMinor,
			Version:     latestVersion + 1,
			IsActive:    true,
			SortOrder:   input.SortOrder,
		}
		if err := tx.Create(&created).Error; err != nil {
			return fmt.Errorf("create topup package: %w", err)
		}
		event := "topup_package.created"
		var before any
		if previousFound {
			event = "topup_package.repriced"
			before = previous
		}
		return createAuditEvent(tx, audit, event, "topup_package", created.ID, before, created)
	})
	if err != nil {
		return CatalogPackage{}, err
	}
	return catalogPackageFromModel(created), nil
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return WalletView{}, ErrWalletNotFound
		}
		return WalletView{}, fmt.Errorf("load wallet: %w", err)
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
	if !validAuditContext(audit) || input.Reason != audit.Reason || input.Credits <= 0 || input.Credits > maxTopupPackageCredits || input.AmountMinor <= 0 || input.AmountMinor > maxTopupPackageAmount || input.SortOrder < 0 || input.SortOrder > maxTopupPackageSortOrder {
		return ErrInvalidCatalogInput
	}
	return nil
}

func validAuditContext(audit AuditContext) bool {
	return audit.ActorUserID > 0 && audit.EffectiveUserID > 0 && audit.ActorRole == string(models.RoleSuperAdmin) && net.ParseIP(audit.SourceIP) != nil && audit.Reason != "" && len(audit.Reason) <= maxBillingAuditReason && strings.TrimSpace(audit.Reason) == audit.Reason && audit.RequestID != "" && len(audit.RequestID) <= 64
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
		Type:                spec.Type,
		Name:                spec.Name,
		Slug:                spec.Slug,
		CPUMillicores:       spec.CPUMillicores,
		MemoryMB:            spec.MemoryMB,
		StorageGB:           spec.StorageGB,
		MonthlyCredits:      spec.MonthlyCredits,
		ConnectionLimit:     spec.ConnectionLimit,
		BackupRetentionDays: spec.BackupRetentionDays,
	}
}

func catalogPackageFromModel(topupPackage models.TopupPackage) CatalogPackage {
	return CatalogPackage{
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
