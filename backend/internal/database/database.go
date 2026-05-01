// ===========================================
// Database Package
// ===========================================
// Handles database connection, migrations, and seeding
// ===========================================
package database

import (
	"fmt"
	"log/slog"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

	err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.Setting{},
		&models.ResourceLog{},
		&models.Feedback{},
	)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	slog.Info("Migrations completed")
	return nil
}

// Seed creates default data if not exists
func Seed(db *gorm.DB, cfg *config.Config) error {
	slog.Info("Seeding database")

	// Create default settings if not exists
	defaultSettings := []models.Setting{
		{Key: models.SettingMaxProjects, Value: models.DefaultMaxProjects, Description: "Maximum projects per student", Type: "int"},
		{Key: models.SettingProjectExpiry, Value: models.DefaultProjectExpiry, Description: "Days until project auto-delete (0=never)", Type: "int"},
		{Key: models.SettingCPULimit, Value: models.DefaultCPULimit, Description: "CPU limit per container (%)", Type: "int"},
		{Key: models.SettingMemoryLimit, Value: models.DefaultMemoryLimit, Description: "Memory limit per container (MB)", Type: "int"},
		{Key: models.SettingBaseDomain, Value: cfg.BaseDomain, Description: "Base domain for subdomains", Type: "string"},
		{Key: models.SettingProjectDomain, Value: cfg.ProjectDomain, Description: "Dedicated domain for student projects", Type: "string"},
		{Key: models.SettingAdminIdleTimeout, Value: models.DefaultAdminIdleTimeout, Description: "Admin inactivity logout timeout (minutes)", Type: "int"},
		{Key: models.SettingMaxConcurrent, Value: models.DefaultMaxConcurrent, Description: "Maximum simultaneous builds", Type: "int"},
	}

	for _, setting := range defaultSettings {
		var existing models.Setting
		if db.Where("setting_key = ?", setting.Key).First(&existing).Error != nil {
			if err := db.Create(&setting).Error; err != nil {
				slog.Warn("Failed to create default setting", "key", setting.Key, "error", err)
			}
		}
	}

	slog.Info("Database seeding completed")
	return nil
}
