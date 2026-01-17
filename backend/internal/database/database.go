// ===========================================
// Database Package
// ===========================================
// Handles database connection, migrations, and seeding
// ===========================================
package database

import (
	"fmt"
	"log"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect establishes database connection
func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
	)

	// Configure GORM logger based on environment
	logMode := logger.Silent
	if cfg.AppDebug {
		logMode = logger.Info
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logMode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("✅ Database connected successfully")
	return db, nil
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	log.Println("Running database migrations...")

	err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.Setting{},
		&models.ResourceLog{},
	)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("✅ Migrations completed")
	return nil
}

// Seed creates default data if not exists
func Seed(db *gorm.DB, cfg *config.Config) error {
	log.Println("Seeding database...")

	// Create default settings if not exists
	defaultSettings := []models.Setting{
		{Key: "max_projects_per_user", Value: "3", Description: "Maximum projects per student", Type: "int"},
		{Key: "project_expiry_days", Value: "30", Description: "Days until project auto-delete (0=never)", Type: "int"},
		{Key: "cpu_limit_percent", Value: "50", Description: "CPU limit per container (%)", Type: "int"},
		{Key: "memory_limit_mb", Value: "512", Description: "Memory limit per container (MB)", Type: "int"},
		{Key: "base_domain", Value: cfg.BaseDomain, Description: "Base domain for subdomains", Type: "string"},
		{Key: "project_domain", Value: cfg.ProjectDomain, Description: "Dedicated domain for student projects", Type: "string"},
	}

	for _, setting := range defaultSettings {
		var existing models.Setting
		if db.Where("setting_key = ?", setting.Key).First(&existing).Error != nil {
			if err := db.Create(&setting).Error; err != nil {
				log.Printf("Warning: failed to create setting %s: %v", setting.Key, err)
			}
		}
	}

	log.Println("✅ Database seeding completed")
	return nil
}
