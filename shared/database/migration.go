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
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
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

	// 3. Promote historical unique indexes to table constraints before AutoMigrate.
	// This prevents GORM from throwing SQLSTATE 42704 ("constraint does not exist") when checking historical schemas.
	if err := EnsureUniqueIndexesAreConstraints(db); err != nil {
		slog.Warn("Failed during unique index promotion", "error", err)
	}

	// 4. Clean up duplicate deployment events before applying new unique indexes.
	// This resolves SQLSTATE 23505 duplicate key violations during AutoMigrate.
	slog.Info("Cleaning up duplicate deployment events...")
	if err := db.Exec(`
		DELETE FROM deployment_events a
		USING deployment_events b
		WHERE a.project_id = b.project_id
		  AND a.job_id = b.job_id
		  AND a.sequence_number = b.sequence_number
		  AND a.id < b.id
	`).Error; err != nil {
		slog.Warn("Failed to clean duplicate deployment events", "error", err)
	}

	// 5. Run explicit, ordered AutoMigrate for models.
	modelsList := []interface{}{
		&models.User{},
		&models.Project{},
		&models.Setting{},
		&models.ResourceLog{},
		&models.Feedback{},
		&models.CustomDomain{},
		&models.DeploymentEvent{},
		&models.DomainEvent{},
		&models.OutboxEvent{},
		&models.IdempotentOperation{},
		&models.PendingReconcile{},
		&models.AuditLog{},
		&models.GithubAppInstallation{},
		&models.DatabaseInstance{},
		&models.DatabaseBackup{},
		&models.SecretStore{},
		&models.SecretStoreItem{},
		&models.SecretStoreItemValue{},
		&models.SecretStoreBinding{},
		&models.SecretStoreActivityLog{},
	}

	for _, m := range modelsList {
		tableName := getTableName(db, m)
		slog.Info("AutoMigrating model", "table", tableName)
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate failed for table %s: %w", tableName, err)
		}
	}

	// 5. Post-migration Safe Schema Reconciliation using exact historical names.
	if err := ReconcileSchemas(db); err != nil {
		return fmt.Errorf("schema reconciliation failed: %w", err)
	}

	slog.Info("Defensive migration bootstrap completed successfully.")

	cfg := config.Load()

	// Perform filesystem migration to user tenant subdirectories.
	if err := MigrateProjectsFilesToTenantLayout(db, cfg.ProjectsPath); err != nil {
		slog.Error("Filesystem tenant migration failed", "error", err)
	}

	// Backfill UIDs for projects that don't have one.
	if err := BackfillUIDs(db, &config.Config{UIDSalt: os.Getenv("UID_SALT")}); err != nil {
		slog.Warn("Failed to backfill UIDs", "error", err)
	}

	// Backfill DatabaseInstance records for existing projects that have MySQL databases.
	if err := BackfillDatabaseInstances(db); err != nil {
		slog.Warn("Failed to backfill database instances", "error", err)
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

// EnsureUniqueIndexesAreConstraints detects if historical unique indexes exist in PostgreSQL
// without corresponding table constraints in pg_constraint. If so, it promotes the index to a table constraint
// (via ALTER TABLE ... ADD CONSTRAINT ... UNIQUE USING INDEX ...) before AutoMigrate runs.
// This prevents GORM from throwing SQLSTATE 42704 ("constraint does not exist") when inspecting historical schemas.
func EnsureUniqueIndexesAreConstraints(db *gorm.DB) error {
	if !isPostgres(db) {
		return nil
	}

	uniqueDefs := []struct {
		table   string
		name    string
		columns []string
	}{
		{"users", "uni_users_email", []string{"email"}},
		{"projects", "uni_projects_subdomain", []string{"subdomain"}},
		{"projects", "uni_projects_database_name", []string{"database_name"}},
		{"projects", "uni_projects_uid", []string{"uid"}},
		{"settings", "uni_settings_setting_key", []string{"setting_key"}},
		{"custom_domains", "uni_custom_domains_domain", []string{"domain"}},
		{"deployment_events", "uni_project_job_seq", []string{"project_id", "job_id", "sequence_number"}},
		{"idempotent_operations", "uni_idempotent_operations_key", []string{"key"}},
		{"pending_reconciles", "uni_pending_reconciles_domain_id", []string{"domain_id"}},
		{"database_instances", "uni_database_instances_project_id", []string{"project_id"}},
	}

	for _, def := range uniqueDefs {
		// Check if constraint exists in pg_constraint
		var conCount int64
		_ = db.Raw(`
			SELECT count(1) 
			FROM pg_constraint c
			JOIN pg_class t ON t.oid = c.conrelid
			WHERE t.relname = ? AND c.conname = ?;
		`, def.table, def.name).Scan(&conCount)

		if conCount > 0 {
			continue
		}

		// If the column already has a UNIQUE constraint with a historical/default
		// PostgreSQL name (for example users_email_key), rename it to the name GORM
		// expects before AutoMigrate. Otherwise GORM may detect the column as unique
		// and try to drop a non-existent constraint named uni_<table>_<column>.
		if existingConstraint := findUniqueConstraintForColumns(db, def.table, def.columns); existingConstraint != "" {
			slog.Info("Renaming existing unique constraint to expected GORM name", "table", def.table, "from", existingConstraint, "to", def.name)
			if hasIndexName(db, def.table, def.name) {
				if err := db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s;", quoteIdent(def.name))).Error; err != nil {
					slog.Warn("Failed to drop duplicate index before renaming unique constraint", "table", def.table, "index", def.name, "error", err)
					continue
				}
			}
			if err := db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME CONSTRAINT %s TO %s;", quoteIdent(def.table), quoteIdent(existingConstraint), quoteIdent(def.name))).Error; err != nil {
				slog.Warn("Failed to rename unique constraint", "table", def.table, "from", existingConstraint, "to", def.name, "error", err)
			}
			continue
		}

		// Check if index exists in pg_class/pg_index
		var idxCount int64
		_ = db.Raw(`
			SELECT count(1)
			FROM pg_class c
			JOIN pg_index i ON i.indexrelid = c.oid
			JOIN pg_class t ON t.oid = i.indrelid
			WHERE t.relname = ? AND c.relname = ?;
		`, def.table, def.name).Scan(&idxCount)

		if idxCount > 0 {
			slog.Info("Promoting existing unique index to table constraint to satisfy GORM AutoMigrate", "table", def.table, "name", def.name)
			sql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE USING INDEX %s;", quoteIdent(def.table), quoteIdent(def.name), quoteIdent(def.name))
			if err := db.Exec(sql).Error; err != nil {
				slog.Warn("Failed to promote unique index to constraint", "table", def.table, "name", def.name, "error", err)
			}
		}
	}

	return nil
}

func findUniqueConstraintForColumns(db *gorm.DB, table string, columns []string) string {
	var constraintName string
	query := `
		SELECT c.conname
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		JOIN unnest(c.conkey) WITH ORDINALITY AS cols(attnum, ordinality) ON true
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = cols.attnum
		WHERE n.nspname = current_schema()
			AND t.relname = ?
			AND c.contype = 'u'
		GROUP BY c.conname
		HAVING string_agg(a.attname, ',' ORDER BY cols.ordinality) = ?
		LIMIT 1;
	`
	if err := db.Raw(query, table, strings.Join(columns, ",")).Scan(&constraintName).Error; err != nil {
		return ""
	}
	return constraintName
}

func hasIndexName(db *gorm.DB, table string, indexName string) bool {
	var count int64
	query := `
		SELECT count(1)
		FROM pg_class c
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_class t ON t.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = current_schema() AND t.relname = ? AND c.relname = ?;
	`
	if err := db.Raw(query, table, indexName).Scan(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// ReconcileSchemas performs safe, non-destructive reconciliation of missing indexes and unique constraints
// after AutoMigrate runs, using exact historical database names to prevent duplication.
func ReconcileSchemas(db *gorm.DB) error {
	// Migrate legacy "student" role to "user" role
	if err := db.Exec("UPDATE users SET role = 'user' WHERE role = 'student';").Error; err != nil {
		slog.Warn("Failed to migrate student roles to user roles", "error", err)
	}

	// Migrate legacy "sleeping" status to "stopped" status
	if err := db.Exec("UPDATE projects SET status = 'stopped' WHERE status = 'sleeping';").Error; err != nil {
		slog.Warn("Failed to migrate sleeping projects to stopped status", "error", err)
	}

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
	if isPostgres(db) {
		_ = db.Exec("UPDATE custom_domains SET current_sequence = 0 WHERE current_sequence IS NULL;").Error
		_ = db.Exec("ALTER TABLE custom_domains ALTER COLUMN current_sequence SET DEFAULT 0;").Error
		_ = db.Exec("ALTER TABLE custom_domains ALTER COLUMN current_sequence SET NOT NULL;").Error
	}

	// Reconcile DeploymentEvent
	if isPostgres(db) {
		_ = db.Exec("DROP INDEX IF EXISTS idx_project_job_seq;")
	}
	_ = EnsureConstraint(db, &models.DeploymentEvent{}, "uni_project_job_seq", "UNIQUE (project_id, job_id, sequence_number)")

	// Reconcile IdempotentOperation — constraint name must match GORM's uni_<table>_<column> convention
	_ = EnsureConstraint(db, &models.IdempotentOperation{}, "uni_idempotent_operations_key", "UNIQUE (key)")

	// Reconcile PendingReconcile — constraint name must match GORM's uni_<table>_<column> convention
	_ = EnsureConstraint(db, &models.PendingReconcile{}, "uni_pending_reconciles_domain_id", "UNIQUE (domain_id)")

	// Reconcile DatabaseInstance
	_ = EnsureConstraint(db, &models.DatabaseInstance{}, "uni_database_instances_project_id", "UNIQUE (project_id)")

	// Reconcile GithubAppInstallation
	_ = EnsureIndex(db, &models.GithubAppInstallation{}, "idx_gh_install_acc", "account_name", false)

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

// BackfillDatabaseInstances creates DatabaseInstance records for existing projects
// that have a database_name set but no corresponding DatabaseInstance row.
// This ensures seamless migration from the legacy inline credentials model.
func BackfillDatabaseInstances(db *gorm.DB) error {
	var projects []models.Project
	if err := db.Where("database_name != '' AND database_name IS NOT NULL").Find(&projects).Error; err != nil {
		return err
	}

	backfilled := 0
	for _, p := range projects {
		var count int64
		db.Model(&models.DatabaseInstance{}).Where("project_id = ?", p.ID).Count(&count)
		if count > 0 {
			continue
		}

		instance := models.DatabaseInstance{
			ProjectID: p.ID,
			Engine:    "mysql",
			Status:    models.DBStatusActive,
			Name:      p.DatabaseName,
			Username:  p.DatabaseName,
			Password:  p.DatabasePassword,
			Host:      "paas-mysql",
			Port:      3306,
		}
		if err := db.Create(&instance).Error; err != nil {
			slog.Warn("Failed to backfill database instance", "project_id", p.ID, "error", err)
			continue
		}
		backfilled++
	}

	if backfilled > 0 {
		slog.Info("Backfilled database instances for legacy projects", "count", backfilled)
	}
	return nil
}

// MigrateProjectsFilesToTenantLayout re-arranges legacy project files into multi-tenant user-based folders.
func MigrateProjectsFilesToTenantLayout(db *gorm.DB, projectsPath string) error {
	slog.Info("Checking if filesystem migration to multi-tenant layout is needed...", "projects_path", projectsPath)

	var projects []models.Project
	if err := db.Find(&projects).Error; err != nil {
		return fmt.Errorf("failed to fetch projects: %w", err)
	}

	for _, p := range projects {
		legacyPath := filepath.Join(projectsPath, p.Subdomain)
		newParentPath := filepath.Join(projectsPath, fmt.Sprintf("user-%d", p.UserID))
		newPath := filepath.Join(newParentPath, p.Subdomain)

		// Move directory to nested tenant path if it still exists in the flat format.
		if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
			slog.Info("Migrating project directory to user tenant folder", "project", p.Name, "legacy", legacyPath, "new", newPath)

			if err := os.MkdirAll(newParentPath, 0755); err != nil {
				slog.Error("Failed to create user directory", "path", newParentPath, "error", err)
				continue
			}

			if err := os.Rename(legacyPath, newPath); err != nil {
				slog.Error("Failed to rename project directory", "from", legacyPath, "to", newPath, "error", err)
				continue
			}
			slog.Info("Successfully migrated project directory", "subdomain", p.Subdomain)
		}

		// Cleanup legacy and new backup environment files to protect secret data.
		legacyBakFile := filepath.Join(projectsPath, p.Subdomain+".env.bak")
		if _, err := os.Stat(legacyBakFile); err == nil {
			_ = os.Remove(legacyBakFile)
		}
		newBakFile := filepath.Join(newPath, ".env.bak")
		if _, err := os.Stat(newBakFile); err == nil {
			_ = os.Remove(newBakFile)
		}
	}

	// Delete any stray backup files in the root projects path.
	files, err := os.ReadDir(projectsPath)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".bak") {
				_ = os.Remove(filepath.Join(projectsPath, f.Name()))
			}
		}
	}

	return nil
}
