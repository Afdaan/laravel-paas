// ===========================================
// Database Migration Architecture
// ===========================================
// Provides defensive, idempotent schema migrations
// preserving exact historical constraint names to prevent duplication.
// ===========================================
package database

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

// DefensiveMigrationBootstrap orchestrates the safe migration pipeline.
// Existence checks matter because historical databases experience schema drift
// over time across different environments. Blindly dropping or renaming constraints
// can cause production outages or duplicated indexes if the names do not match perfectly.
func DefensiveMigrationBootstrap(db *gorm.DB) error {
	slog.Info("Starting defensive database migration bootstrap...")

	// 1. Verify GORM and Database compatibility / connectivity.
	if err := verifyCompatibility(db); err != nil {
		return fmt.Errorf("compatibility check failed: %w", err)
	}

	// 2. Log startup schema state (detected indexes & constraints).
	logStartupSchemaState(db)

	// 3. Run explicit, ordered AutoMigrate for models.
	modelsList := []interface{}{
		&models.User{},
		&models.Project{},
		&models.Setting{},
		&models.ResourceLog{},
		&models.Feedback{},
		&models.CustomDomain{},
		&models.DeploymentEvent{},
		&models.DomainEvent{},
	}

	for _, m := range modelsList {
		tableName := getTableName(db, m)
		slog.Info("AutoMigrating model", "table", tableName)
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate failed for table %s: %w", tableName, err)
		}
	}

	// 4. Post-migration Safe Schema Reconciliation using exact historical names.
	if err := ReconcileSchemas(db); err != nil {
		return fmt.Errorf("schema reconciliation failed: %w", err)
	}

	slog.Info("Defensive migration bootstrap completed successfully.")

	// Backfill UIDs for projects that don't have one.
	if err := BackfillUIDs(db, &config.Config{UIDSalt: os.Getenv("UID_SALT")}); err != nil {
		slog.Warn("Failed to backfill UIDs", "error", err)
	}

	return nil
}

func isPostgres(db *gorm.DB) bool {
	return db.Dialector.Name() == "postgres"
}

func hasConstraintSafe(db *gorm.DB, model interface{}, constraintName string) bool {
	if isPostgres(db) {
		tableName := getTableName(db, model)
		var count int64
		query := `
			SELECT count(1) 
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			WHERE t.relname = ? AND c.conname = ?;
		`
		if err := db.Raw(query, tableName, constraintName).Scan(&count).Error; err != nil {
			return db.Migrator().HasConstraint(model, constraintName)
		}
		return count > 0
	}
	return db.Migrator().HasConstraint(model, constraintName)
}

func hasIndexSafe(db *gorm.DB, model interface{}, indexName string) bool {
	if isPostgres(db) {
		tableName := getTableName(db, model)
		var count int64
		query := `
			SELECT count(1)
			FROM pg_class c
			JOIN pg_index i ON i.indexrelid = c.oid
			JOIN pg_class t ON t.oid = i.indrelid
			WHERE t.relname = ? AND c.relname = ?;
		`
		if err := db.Raw(query, tableName, indexName).Scan(&count).Error; err != nil {
			return db.Migrator().HasIndex(model, indexName)
		}
		return count > 0
	}
	return db.Migrator().HasIndex(model, indexName)
}

// ReconcileSchemas performs safe, non-destructive reconciliation of missing indexes and unique constraints
// after AutoMigrate runs, using exact historical database names to prevent duplication.
func ReconcileSchemas(db *gorm.DB) error {
	// Reconcile User
	_ = EnsureConstraint(db, &models.User{}, "uni_users_email", "UNIQUE (email)")

	// Reconcile Project
	_ = EnsureIndex(db, &models.Project{}, "idx_status_active", "status", false)
	_ = EnsureIndex(db, &models.Project{}, "idx_dep_status", "deployment_status", false)
	_ = EnsureConstraint(db, &models.Project{}, "uni_projects_subdomain", "UNIQUE (subdomain)")
	_ = EnsureConstraint(db, &models.Project{}, "uni_projects_database_name", "UNIQUE (database_name)")
	_ = EnsureConstraint(db, &models.Project{}, "uni_projects_uid", "UNIQUE (uid)")

	// Reconcile Setting
	_ = EnsureConstraint(db, &models.Setting{}, "idx_settings_key", "UNIQUE (setting_key)")

	// Reconcile CustomDomain
	_ = EnsureConstraint(db, &models.CustomDomain{}, "uni_custom_domains_domain", "UNIQUE (domain)")

	return nil
}

// EnsureConstraint ensures a unique constraint exists on a table defensively.
func EnsureConstraint(db *gorm.DB, model interface{}, constraintName string, sqlDefinition string) error {
	if !isPostgres(db) {
		return nil
	}
	tableName := getTableName(db, model)
	if hasConstraintSafe(db, model, constraintName) || hasIndexSafe(db, model, constraintName) {
		return nil
	}
	slog.Info("Creating missing constraint", "table", tableName, "constraint", constraintName)
	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s;", tableName, constraintName, sqlDefinition)
	return db.Exec(sql).Error
}

// EnsureIndex ensures a database index exists on a table defensively.
func EnsureIndex(db *gorm.DB, model interface{}, indexName string, columnName string, unique bool) error {
	tableName := getTableName(db, model)
	if hasIndexSafe(db, model, indexName) || hasConstraintSafe(db, model, indexName) {
		return nil
	}
	slog.Info("Creating missing index", "table", tableName, "index", indexName, "column", columnName)
	uniqueStr := ""
	if unique {
		uniqueStr = "UNIQUE"
	}
	sql := fmt.Sprintf("CREATE %s INDEX IF NOT EXISTS %s ON %s (%s);", uniqueStr, indexName, tableName, columnName)
	return db.Exec(sql).Error
}

// EnsureForeignKey ensures a foreign key constraint exists defensively.
func EnsureForeignKey(db *gorm.DB, model interface{}, fkName string, column string, refTable string, refColumn string, onDelete string, onUpdate string) error {
	if !isPostgres(db) {
		return nil
	}
	tableName := getTableName(db, model)
	if hasConstraintSafe(db, model, fkName) {
		return nil
	}
	slog.Info("Creating missing foreign key", "table", tableName, "fk", fkName)
	sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s ON UPDATE %s;",
		tableName, fkName, column, refTable, refColumn, onDelete, onUpdate)
	return db.Exec(sql).Error
}

func verifyCompatibility(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying SQL DB: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	if isPostgres(db) {
		var pgVersion string
		if err := db.Raw("SELECT version();").Scan(&pgVersion).Error; err != nil {
			return fmt.Errorf("failed to query postgres version: %w", err)
		}
		slog.Info("GORM and Database compatibility verified", "pg_version", pgVersion)
	} else {
		slog.Info("Database compatibility verified", "dialect", db.Dialector.Name())
	}
	return nil
}

func logStartupSchemaState(db *gorm.DB) {
	if !isPostgres(db) {
		return
	}
	slog.Info("Inspecting current database schema state...")

	type IndexInfo struct {
		Tablename string
		Indexname string
	}
	var indexes []IndexInfo
	_ = db.Raw("SELECT tablename, indexname FROM pg_indexes WHERE schemaname = 'public'").Scan(&indexes).Error

	type ConstraintInfo struct {
		Relname string
		Conname string
	}
	var constraints []ConstraintInfo
	_ = db.Raw(`
		SELECT t.relname, c.conname 
		FROM pg_constraint c 
		JOIN pg_class t ON t.oid = c.conrelid 
		JOIN pg_namespace n ON n.oid = t.relnamespace 
		WHERE n.nspname = 'public'
	`).Scan(&constraints).Error

	slog.Info("Startup schema state detected", "index_count", len(indexes), "constraint_count", len(constraints))
	for _, idx := range indexes {
		slog.Debug("Detected index", "table", idx.Tablename, "index", idx.Indexname)
	}
	for _, con := range constraints {
		slog.Debug("Detected constraint", "table", con.Relname, "constraint", con.Conname)
	}
}

func getTableName(db *gorm.DB, model interface{}) string {
	stmt := &gorm.Statement{DB: db}
	_ = stmt.Parse(model)
	if stmt.Schema != nil {
		return stmt.Schema.Table
	}
	return fmt.Sprintf("%Ts", model)
}
