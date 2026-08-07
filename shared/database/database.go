// ===========================================
// Database Package
// ===========================================
// Handles database connection, migrations, and seeding
// ===========================================
package database

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Connect establishes database connection
func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Jakarta",
		cfg.PGHost,
		cfg.PGUser,
		cfg.PGPassword,
		cfg.PGDatabase,
		cfg.PGPort,
	)

	// Configure GORM logger based on environment
	logMode := logger.Silent
	if cfg.AppDebug {
		logMode = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logMode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	slog.Info("Database connected successfully")
	return db, nil
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	slog.Info("Running database migrations")
	return DefensiveMigrationBootstrap(db)
}

// BackfillUIDs ensures all projects have a persistent UID.
// For existing projects, it uses the legacy encoding to keep current URLs working.
func BackfillUIDs(db *gorm.DB, cfg *config.Config) error {
	var projects []models.Project
	if err := db.Where("uid = '' OR uid IS NULL").Find(&projects).Error; err != nil {
		return err
	}

	if len(projects) == 0 {
		return nil
	}

	slog.Info("Backfilling UIDs for existing projects", "count", len(projects))
	for _, p := range projects {
		// Use legacy encoding to preserve existing URLs for these projects
		legacyUID := utils.EncodeUID(p.ID, cfg.UIDSalt)
		if err := db.Model(&p).Update("uid", legacyUID).Error; err != nil {
			slog.Error("Failed to update project UID", "projectID", p.ID, "error", err)
		}
	}

	return nil
}

// Seed creates default data if not exists
func Seed(db *gorm.DB, cfg *config.Config) error {
	slog.Info("Seeding database")

	// Create default settings if not exists
	defaultSettings := []models.Setting{
		{Key: models.SettingMaxProjects, Value: models.DefaultMaxProjects, Description: "Maximum projects per user", Type: "int"},
		{Key: models.SettingProjectExpiry, Value: models.DefaultProjectExpiry, Description: "Days until project auto-delete (0=never)", Type: "int"},
		{Key: models.SettingCPULimit, Value: models.DefaultCPULimit, Description: "CPU limit per container (%)", Type: "int"},
		{Key: models.SettingMemoryLimit, Value: models.DefaultMemoryLimit, Description: "Memory limit per container (MB)", Type: "int"},
		{Key: models.SettingBaseDomain, Value: cfg.BaseDomain, Description: "Base domain for subdomains", Type: "string"},
		{Key: models.SettingProjectDomain, Value: cfg.ProjectDomain, Description: "Dedicated domain for user projects", Type: "string"},
		{Key: models.SettingAdminIdleTimeout, Value: models.DefaultAdminIdleTimeout, Description: "Admin inactivity logout timeout (minutes)", Type: "int"},
		{Key: models.SettingMaxConcurrent, Value: models.DefaultMaxConcurrent, Description: "Maximum simultaneous builds", Type: "int"},
		{Key: models.SettingBuildTimeout, Value: models.DefaultBuildTimeout, Description: "Build timeout (seconds)", Type: "int"},
		{Key: models.SettingMaxDomainsPerProject, Value: models.DefaultMaxDomainsPerProject, Description: "Maximum custom domains per project", Type: "int"},
	}

	for _, setting := range defaultSettings {
		var existing models.Setting
		if db.Where("setting_key = ?", setting.Key).First(&existing).Error != nil {
			if err := db.Create(&setting).Error; err != nil {
				slog.Warn("Failed to create default setting", "key", setting.Key, "error", err)
			}
			continue
		}
		if setting.Key == models.SettingBaseDomain || setting.Key == models.SettingProjectDomain {
			if err := db.Model(&existing).Update("value", setting.Value).Error; err != nil {
				return fmt.Errorf("sync %s from environment: %w", setting.Key, err)
			}
		}
	}

	if err := repairBillingCatalog(db); err != nil {
		return err
	}
	if err := seedBillingCatalog(db); err != nil {
		return err
	}

	slog.Info("Database seeding completed")
	return nil
}

func repairBillingCatalog(db *gorm.DB) error {
	var incorrect models.BillableSpec
	err := db.Where("type = ? AND name = ? AND slug = ? AND version = ? AND is_active = ? AND cpu_millicores = ? AND memory_mb = ? AND storage_gb = ? AND connection_limit = ? AND backup_retention_days = ? AND monthly_credits = ?", models.BillableTypeDatabase, "Large", "large", 1, true, 2000, 4096, 50, 600000, 30, 400000).First(&incorrect).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find incorrect billing database large catalog: %w", err)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&incorrect).Update("is_active", false).Error; err != nil {
			return fmt.Errorf("retire incorrect billing database large catalog: %w", err)
		}
		corrected := incorrect
		corrected.ID = 0
		corrected.ConnectionLimit = intPtr(200)
		corrected.MonthlyCredits = 600000
		corrected.Version++
		corrected.IsActive = true
		corrected.CreatedAt = time.Time{}
		corrected.UpdatedAt = time.Time{}
		if err := tx.Create(&corrected).Error; err != nil {
			return fmt.Errorf("create corrected billing database large catalog: %w", err)
		}
		slog.Info("Replaced incorrect seeded billing database large catalog", "retired_spec_id", incorrect.ID, "replacement_spec_id", corrected.ID)
		return nil
	})
}

func seedBillingCatalog(db *gorm.DB) error {
	specs := []models.BillableSpec{
		{Type: models.BillableTypeProject, Name: "Small", Slug: "small", CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100, Version: 1, IsActive: true},
		{Type: models.BillableTypeProject, Name: "Medium", Slug: "medium", CPUMillicores: 1000, MemoryMB: 2048, StorageGB: 10, MonthlyCredits: 200, Version: 1, IsActive: true},
		{Type: models.BillableTypeProject, Name: "Large", Slug: "large", CPUMillicores: 2000, MemoryMB: 4096, StorageGB: 20, MonthlyCredits: 400, Version: 1, IsActive: true},
		{Type: models.BillableTypeDatabase, Name: "Small", Slug: "small", CPUMillicores: 500, MemoryMB: 1024, StorageGB: 10, ConnectionLimit: intPtr(50), BackupRetentionDays: intPtr(7), MonthlyCredits: 150, Version: 1, IsActive: true},
		{Type: models.BillableTypeDatabase, Name: "Medium", Slug: "medium", CPUMillicores: 1000, MemoryMB: 2048, StorageGB: 25, ConnectionLimit: intPtr(100), BackupRetentionDays: intPtr(14), MonthlyCredits: 300, Version: 1, IsActive: true},
		{Type: models.BillableTypeDatabase, Name: "Large", Slug: "large", CPUMillicores: 2000, MemoryMB: 4096, StorageGB: 50, ConnectionLimit: intPtr(200), BackupRetentionDays: intPtr(30), MonthlyCredits: 600, Version: 1, IsActive: true},
	}
	for _, spec := range specs {
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&spec).Error; err != nil {
			return fmt.Errorf("seed billing spec %s/%s: %w", spec.Type, spec.Slug, err)
		}
	}

	for index, credits := range []int64{100, 250, 500, 1000} {
		pkg := models.TopupPackage{Credits: credits, Currency: models.BillingCurrencyIDR, AmountMinor: credits, Provider: models.BillingProviderMidtrans, Version: 1, IsActive: true, SortOrder: index + 1}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&pkg).Error; err != nil {
			return fmt.Errorf("seed topup package %d: %w", credits, err)
		}
	}
	return nil
}

func intPtr(value int) *int { return &value }
