// ===========================================
// Database Migration Architecture
// ===========================================
// Provides defensive, idempotent schema migrations
// preserving exact historical constraint names to prevent duplication.
// ===========================================
package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
)

const defensiveMigrationLockIdentity = "runara:defensive-migration-bootstrap"

// DefensiveMigrationBootstrap orchestrates the safe migration pipeline.
// Existence checks matter because historical databases experience schema drift
// over time across different environments. Blindly dropping or renaming constraints
// can cause production outages or duplicated indexes if the names do not match perfectly.
func DefensiveMigrationBootstrap(db *gorm.DB) error {
	if !isPostgres(db) {
		return defensiveMigrationBootstrap(db)
	}
	// Hold a dedicated session lock while migrations run through GORM's normal
	// pool. GORM AutoMigrate requires *sql.DB and can panic when bound directly
	// to *sql.Conn; PostgreSQL advisory locks are session-scoped globally.
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get database for migration lock: %w", err)
	}
	sessionConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("open migration lock session: %w", err)
	}
	defer sessionConn.Close()
	if _, err := sessionConn.ExecContext(context.Background(), "SELECT pg_advisory_lock(hashtextextended($1, 0))", defensiveMigrationLockIdentity); err != nil {
		return fmt.Errorf("acquire database migration lock: %w", err)
	}
	defer func() {
		if _, err := sessionConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", defensiveMigrationLockIdentity); err != nil {
			slog.Error("Failed to release database migration lock", "error", err)
		}
	}()
	return defensiveMigrationBootstrap(db)
}

func postgresSessionConn(db *gorm.DB) (*sql.Conn, error) {
	connection, ok := db.Statement.ConnPool.(*sql.Conn)
	if !ok {
		return nil, fmt.Errorf("expected PostgreSQL session connection, got %T", db.Statement.ConnPool)
	}
	return connection, nil
}

func defensiveMigrationBootstrap(db *gorm.DB) error {
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
	if db.Migrator().HasTable(&models.DeploymentEvent{}) {
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
	}

	// 4.5 Migrate database_instances table fields user_id and project_id.
	if err := migrateDatabaseInstancesTable(db); err != nil {
		return fmt.Errorf("failed database_instances table migration: %w", err)
	}

	// Reconcile legacy billing package identity before AutoMigrate creates versioned indexes.
	if isPostgres(db) && db.Migrator().HasTable(&models.TopupPackage{}) {
		if err := reconcileTopupPackageIdentity(db); err != nil {
			return err
		}
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
		&models.ImpersonationAudit{},
		&models.GithubAppInstallation{},
		&models.DatabaseInstance{},
		&models.DatabaseCleanupTask{},
		&models.DatabaseReinstallRecoveryTask{},
		&models.DatabaseCredentialRotationTask{},
		&models.ProjectEnvSyncTask{},
		&models.ProjectDeletionTask{},
		&models.ProjectSuspensionTask{},
		&models.DatabaseStatusOperationTask{},
		&models.DatabaseBackup{},
		&models.SecretStore{},
		&models.SecretStoreItem{},
		&models.SecretStoreItemValue{},
		&models.SecretStoreBinding{},
		&models.SecretStoreActivityLog{},
		&models.Wallet{},
		&models.WalletLedgerEntry{},
		&models.BillingAuditEvent{},
		&models.TopupPackage{},
		&models.Topup{},
		&models.BillableSpec{},
		&models.BillableResource{},
		&models.Invoice{},
		&models.InvoiceItem{},
		&models.PaymentEvent{},
	}

	for _, m := range modelsList {
		tableName := getTableName(db, m)
		slog.Info("AutoMigrating model", "table", tableName)
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate failed for table %s: %w", tableName, err)
		}
	}
	if err := repairBillingCatalog(db); err != nil {
		return fmt.Errorf("repair billing catalog failed: %w", err)
	}
	if err := seedBillingCatalog(db); err != nil {
		return fmt.Errorf("seed billing catalog failed: %w", err)
	}
	if err := reconcileDuplicateActiveBillingCatalog(db); err != nil {
		return err
	}
	if err := ensureActiveCatalogIndexes(db); err != nil {
		return err
	}
	if err := backfillBillableResourceAnchors(db); err != nil {
		return err
	}
	if err := backfillBillableResourceAutoRenew(db); err != nil {
		return err
	}
	// 5. Post-migration Safe Schema Reconciliation using exact historical names.
	if err := ReconcileSchemas(db); err != nil {
		return fmt.Errorf("schema reconciliation failed: %w", err)
	}
	if err := retireDeletedBillableResources(db); err != nil {
		return err
	}

	// 5.1. Populate empty UIDs for database_instances to support secure communications.
	var emptyUIDInstances []models.DatabaseInstance
	if err := db.Where("uid = ? OR uid IS NULL", "").Find(&emptyUIDInstances).Error; err != nil {
		return fmt.Errorf("failed to scan for database instances with empty UIDs: %w", err)
	}
	for _, inst := range emptyUIDInstances {
		newUID := utils.GenerateRandomUID()
		if err := db.Model(&inst).Update("uid", newUID).Error; err != nil {
			slog.Error("Failed to update database instance UID during backfill migration", "db_id", inst.ID, "error", err)
			return fmt.Errorf("failed to backfill database instance UID for ID %d: %w", inst.ID, err)
		}
	}

	// Verify that no database instances still have an empty/null UID
	var count int64
	if err := db.Model(&models.DatabaseInstance{}).Where("uid = ? OR uid IS NULL", "").Count(&count).Error; err != nil {
		return fmt.Errorf("failed to verify database instance UID backfill count: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("migration verification failed: %d database instances still have empty UIDs", count)
	}

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
	if cfg.BillingEnabled {
		if err := ensureBillableResourceCoverage(db); err != nil {
			return err
		}
	}

	// Backfill Laravel APP_KEY for existing Laravel projects.
	if err := BackfillLaravelAppKeys(db, cfg); err != nil {
		slog.Warn("Failed to backfill Laravel app keys", "error", err)
	}

	// Backfill Primary Custom Domains for existing projects.
	if err := BackfillPrimaryDomains(db); err != nil {
		slog.Warn("Failed to backfill primary custom domains", "error", err)
	}

	slog.Info("Defensive migration bootstrap completed successfully.")
	return nil
}

func backfillBillableResourceAutoRenew(db *gorm.DB) error {
	if !isPostgres(db) {
		return nil
	}
	result := db.Exec("UPDATE billable_resources SET auto_renew = TRUE WHERE auto_renew = FALSE")
	if result.Error != nil {
		return fmt.Errorf("backfill billable resource auto_renew: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		slog.Info("Backfilled billable resource auto_renew defaults", "rows", result.RowsAffected)
	}
	return nil
}

func ensureBillableResourceCoverage(db *gorm.DB) error {
	var unmappedProjects int64
	if err := db.Model(&models.Project{}).
		Where("status <> ?", models.StatusDeleting).
		Where("NOT EXISTS (SELECT 1 FROM billable_resources WHERE billable_resources.type = ? AND billable_resources.resource_id = projects.id)", models.BillableTypeProject).
		Count(&unmappedProjects).Error; err != nil {
		return fmt.Errorf("check unmapped billable projects: %w", err)
	}
	var unmappedDatabases int64
	if err := db.Model(&models.DatabaseInstance{}).
		Where("status <> ?", models.DBStatusDeleted).
		Where("NOT EXISTS (SELECT 1 FROM billable_resources WHERE billable_resources.type = ? AND billable_resources.resource_id = database_instances.id)", models.BillableTypeDatabase).
		Count(&unmappedDatabases).Error; err != nil {
		return fmt.Errorf("check unmapped billable databases: %w", err)
	}
	if unmappedProjects == 0 && unmappedDatabases == 0 {
		return nil
	}
	return fmt.Errorf("billing activation blocked: %d project(s) and %d database(s) require explicit billable-spec mapping", unmappedProjects, unmappedDatabases)
}

func retireDeletedBillableResources(db *gorm.DB) error {
	if err := db.Model(&models.BillableResource{}).
		Where("type = ? AND billing_status <> ?", models.BillableTypeProject, models.BillableResourceStatusDeleted).
		Where("NOT EXISTS (SELECT 1 FROM projects WHERE projects.id = billable_resources.resource_id AND projects.status <> ?)", models.StatusDeleting).
		Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
		return fmt.Errorf("retire deleted project billable resources: %w", err)
	}
	if err := db.Model(&models.BillableResource{}).
		Where("type = ? AND billing_status <> ?", models.BillableTypeDatabase, models.BillableResourceStatusDeleted).
		Where("NOT EXISTS (SELECT 1 FROM database_instances WHERE database_instances.id = billable_resources.resource_id AND database_instances.status <> ?)", models.DBStatusDeleted).
		Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
		return fmt.Errorf("retire deleted database billable resources: %w", err)
	}
	return nil
}

func backfillBillableResourceAnchors(db *gorm.DB) error {
	var resources []models.BillableResource
	if err := db.Model(&models.BillableResource{}).Where("billing_anchor_day = ?", 0).Find(&resources).Error; err != nil {
		return fmt.Errorf("list billable resources without anchor: %w", err)
	}
	for _, resource := range resources {
		var firstInvoice models.Invoice
		if err := db.Model(&models.Invoice{}).
			Select("invoices.*").
			Joins("JOIN invoice_items ON invoice_items.invoice_id = invoices.id").
			Where("invoice_items.billable_resource_id = ?", resource.ID).
			Order("invoices.period_start ASC, invoice_items.id ASC").
			First(&firstInvoice).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("billing anchor for resource %d is unprovable; add an explicit billing anchor before enabling billing", resource.ID)
			}
			return fmt.Errorf("load first invoice for billable resource %d: %w", resource.ID, err)
		}
		anchorStart := firstInvoice.PeriodStart.UTC()
		anchorDay := anchorStart.Day()
		monthEnd := anchorDay == time.Date(anchorStart.Year(), anchorStart.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if err := db.Model(&resource).Updates(map[string]any{"billing_anchor_day": anchorDay, "billing_anchor_month_end": monthEnd}).Error; err != nil {
			return fmt.Errorf("backfill billable resource anchor %d: %w", resource.ID, err)
		}
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
		{"wallets", "uni_wallets_user_id", []string{"user_id"}},
		{"wallets", "uni_wallets_id_user_id", []string{"id", "user_id"}},
		{"wallet_ledger_entries", "uni_wallet_ledger_entries_idempotency_key", []string{"idempotency_key"}},
		{"topup_packages", "uni_topup_packages_identity_version", []string{"provider", "currency", "credits", "version"}},
		{"topups", "uni_topups_wallet_client_key", []string{"wallet_id", "client_idempotency_key"}},
		{"topups", "uni_topups_provider_order", []string{"provider", "provider_order_id"}},
		{"billable_specs", "uni_billable_specs_slug_version", []string{"type", "slug", "version"}},
		{"billable_resources", "uni_billable_resources_type_resource", []string{"type", "resource_id"}},
		{"invoices", "uni_invoices_user_period", []string{"user_id", "period_start", "period_end"}},
		{"invoices", "uni_invoices_idempotency_key", []string{"idempotency_key"}},
		{"invoice_items", "uni_invoice_items_invoice_resource", []string{"invoice_id", "billable_resource_id"}},
		{"payment_events", "uni_payment_events_event_key", []string{"event_key"}},
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

	// Reconcile SecretStoreItem unique index
	if isPostgres(db) {
		if !hasIndexSafe(db, &models.SecretStoreItem{}, "idx_secret_store_items_key_active") {
			slog.Info("Creating missing partial unique index idx_secret_store_items_key_active concurrently")
			if err := db.Exec("CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_secret_store_items_key_active ON secret_store_items (secret_store_id, key) WHERE deleted_at IS NULL;").Error; err != nil {
				slog.Error("Failed to create idx_secret_store_items_key_active", "error", err)
			}
		}
	}

	// Reconcile SecretStoreBinding unique constraint
	_ = EnsureConstraint(db, &models.SecretStoreBinding{}, "uni_secret_store_bindings_proj_store_env", "UNIQUE (project_id, secret_store_id, environment)")

	if err := reconcileBillingSchema(db); err != nil {
		return err
	}
	if err := repairBillingCatalog(db); err != nil {
		return err
	}

	return nil
}

func reconcileTopupPackageIdentity(db *gorm.DB) error {
	if err := db.Exec(`
		ALTER TABLE topup_packages ADD COLUMN IF NOT EXISTS version integer;
		UPDATE topup_packages SET version = 1 WHERE version IS NULL;
		ALTER TABLE topup_packages ALTER COLUMN version SET DEFAULT 1;
		ALTER TABLE topup_packages ALTER COLUMN version SET NOT NULL;
		ALTER TABLE topup_packages DROP CONSTRAINT IF EXISTS uni_topup_packages_provider_currency_credits;
		DROP INDEX IF EXISTS uni_topup_packages_provider_currency_credits;
	`).Error; err != nil {
		return fmt.Errorf("migrate topup package version: %w", err)
	}

	correct, err := hasTopupPackageIdentityVersionConstraint(db)
	if err != nil {
		return err
	}
	if correct {
		return nil
	}

	if err := db.Exec(`
		ALTER TABLE topup_packages DROP CONSTRAINT IF EXISTS uni_topup_packages_identity_version;
		DROP INDEX IF EXISTS uni_topup_packages_identity_version;
		ALTER TABLE topup_packages ADD CONSTRAINT uni_topup_packages_identity_version UNIQUE (provider, currency, credits, version);
	`).Error; err != nil {
		return fmt.Errorf("reconcile topup package identity: %w", err)
	}
	return nil
}

func hasTopupPackageIdentityVersionConstraint(db *gorm.DB) (bool, error) {
	var exists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint
			WHERE conrelid = 'topup_packages'::regclass
			  AND conname = 'uni_topup_packages_identity_version'
			  AND contype = 'u'
			  AND pg_get_constraintdef(oid) = 'UNIQUE (provider, currency, credits, version)'
		)
	`).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check topup package identity constraint: %w", err)
	}
	return exists, nil
}

func reconcileDuplicateActiveBillingCatalog(db *gorm.DB) error {
	if !isPostgres(db) || !db.Migrator().HasTable(&models.TopupPackage{}) || !db.Migrator().HasTable(&models.BillableSpec{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		topupResult := tx.Exec(`
			WITH ranked AS (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY provider, currency, credits
					ORDER BY version DESC, id DESC
				) AS rank
				FROM topup_packages
				WHERE is_active
			)
			UPDATE topup_packages
			SET is_active = false, updated_at = NOW()
			WHERE id IN (SELECT id FROM ranked WHERE rank > 1);
		`)
		if topupResult.Error != nil {
			return fmt.Errorf("deactivate duplicate active topup packages: %w", topupResult.Error)
		}

		specResult := tx.Exec(`
			WITH ranked AS (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY type, slug
					ORDER BY version DESC, id DESC
				) AS rank
				FROM billable_specs
				WHERE is_active
			)
			UPDATE billable_specs
			SET is_active = false, updated_at = NOW()
			WHERE id IN (SELECT id FROM ranked WHERE rank > 1);
		`)
		if specResult.Error != nil {
			return fmt.Errorf("deactivate duplicate active billable specs: %w", specResult.Error)
		}
		if topupResult.RowsAffected > 0 || specResult.RowsAffected > 0 {
			slog.Warn("Reconciled duplicate active billing catalog rows", "deactivated_topup_packages", topupResult.RowsAffected, "deactivated_billable_specs", specResult.RowsAffected)
		}
		return nil
	})
}

func ensureActiveCatalogIndexes(db *gorm.DB) error {
	if !isPostgres(db) || !db.Migrator().HasTable(&models.TopupPackage{}) || !db.Migrator().HasTable(&models.BillableSpec{}) {
		return nil
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uni_topup_packages_one_active
		ON topup_packages (provider, currency, credits)
		WHERE is_active;
	`).Error; err != nil {
		return fmt.Errorf("create active topup package index: %w", err)
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uni_billable_specs_one_active
		ON billable_specs (type, slug)
		WHERE is_active;
	`).Error; err != nil {
		return fmt.Errorf("create active billable spec index: %w", err)
	}
	return nil
}

func reconcileBillingSchema(db *gorm.DB) error {
	if isPostgres(db) {
		if err := db.Exec("ALTER TABLE wallets DROP CONSTRAINT IF EXISTS chk_wallets_balance_nonnegative").Error; err != nil {
			return fmt.Errorf("remove wallet nonnegative constraint: %w", err)
		}
		if err := upgradeTopupStatusConstraint(db); err != nil {
			return err
		}
		if err := dropCheckMissingMarker(db, "billable_resources", "chk_billable_resources_status", "deleted"); err != nil {
			return err
		}
		// Currency checks predating multi-currency pin the column to IDR. Drop the stale
		// definition so the checks loop below recreates it with the widened rule.
		for _, target := range []struct{ table, name string }{
			{"topup_packages", "chk_topup_packages_provider_currency"},
			{"topups", "chk_topups_provider_currency"},
		} {
			if err := dropCheckMissingMarker(db, target.table, target.name, "USD"); err != nil {
				return err
			}
		}
	}

	constraints := []struct {
		model any
		name  string
		check string
	}{
		{&models.Wallet{}, "uni_wallets_user_id", "UNIQUE (user_id)"},
		{&models.Wallet{}, "uni_wallets_id_user_id", "UNIQUE (id, user_id)"},
		{&models.WalletLedgerEntry{}, "uni_wallet_ledger_entries_idempotency_key", "UNIQUE (idempotency_key)"},
		{&models.TopupPackage{}, "uni_topup_packages_identity_version", "UNIQUE (provider, currency, credits, version)"},
		{&models.Topup{}, "uni_topups_wallet_client_key", "UNIQUE (wallet_id, client_idempotency_key)"},
		{&models.Topup{}, "uni_topups_provider_order", "UNIQUE (provider, provider_order_id)"},
		{&models.BillableSpec{}, "uni_billable_specs_slug_version", "UNIQUE (type, slug, version)"},
		{&models.BillableResource{}, "uni_billable_resources_type_resource", "UNIQUE (type, resource_id)"},
		{&models.Invoice{}, "uni_invoices_user_period", "UNIQUE (user_id, period_start, period_end)"},
		{&models.Invoice{}, "uni_invoices_idempotency_key", "UNIQUE (idempotency_key)"},
		{&models.InvoiceItem{}, "uni_invoice_items_invoice_resource", "UNIQUE (invoice_id, billable_resource_id)"},
		{&models.Invoice{}, "fk_invoices_wallet_owner", "FOREIGN KEY (wallet_id, user_id) REFERENCES wallets (id, user_id) ON DELETE RESTRICT ON UPDATE RESTRICT"},
		{&models.PaymentEvent{}, "uni_payment_events_event_key", "UNIQUE (event_key)"},
	}
	for _, constraint := range constraints {
		if err := EnsureConstraint(db, constraint.model, constraint.name, constraint.check); err != nil {
			return err
		}
	}
	if !isPostgres(db) {
		return nil
	}

	foreignKeys := []struct {
		model     any
		name, col string
		reference string
	}{
		{&models.Wallet{}, "fk_wallets_user", "user_id", "users"},
		{&models.WalletLedgerEntry{}, "fk_wallet_ledger_entries_wallet", "wallet_id", "wallets"},
		{&models.WalletLedgerEntry{}, "fk_wallet_ledger_entries_created_by", "created_by", "users"},
		{&models.Topup{}, "fk_topups_wallet", "wallet_id", "wallets"},
		{&models.Topup{}, "fk_topups_package", "topup_package_id", "topup_packages"},
		{&models.BillableResource{}, "fk_billable_resources_user", "user_id", "users"},
		{&models.BillableResource{}, "fk_billable_resources_spec", "spec_id", "billable_specs"},
		{&models.Invoice{}, "fk_invoices_user", "user_id", "users"},
		{&models.InvoiceItem{}, "fk_invoice_items_invoice", "invoice_id", "invoices"},
		{&models.InvoiceItem{}, "fk_invoice_items_resource", "billable_resource_id", "billable_resources"},
		{&models.InvoiceItem{}, "fk_invoice_items_spec", "spec_id", "billable_specs"},
	}
	for _, fk := range foreignKeys {
		if err := EnsureForeignKey(db, fk.model, fk.name, fk.col, fk.reference, "id", "RESTRICT", "RESTRICT"); err != nil {
			return err
		}
	}

	checks := []struct {
		model      any
		name, rule string
	}{
		{&models.WalletLedgerEntry{}, "chk_wallet_ledger_entries_amount_nonzero", "amount_credits <> 0"},
		{&models.WalletLedgerEntry{}, "chk_wallet_ledger_entries_type", "type IN ('topup', 'topup_reversal', 'invoice_debit', 'invoice_refund', 'adjustment')"},
		{&models.WalletLedgerEntry{}, "chk_wallet_ledger_entries_amount_direction", "(type IN ('topup', 'invoice_refund') AND amount_credits > 0) OR (type IN ('topup_reversal', 'invoice_debit') AND amount_credits < 0) OR type = 'adjustment'"},
		{&models.TopupPackage{}, "chk_topup_packages_positive", "credits > 0 AND amount_minor > 0 AND version > 0"},
		{&models.TopupPackage{}, "chk_topup_packages_provider_currency", "provider = 'midtrans' AND currency IN ('IDR', 'USD')"},
		{&models.Topup{}, "chk_topups_positive", "credits > 0 AND amount_minor > 0"},
		{&models.Topup{}, "chk_topups_provider_currency", "provider = 'midtrans' AND currency IN ('IDR', 'USD')"},
		{&models.Topup{}, "chk_topups_status", "status IN ('pending', 'paid', 'failed', 'expired', 'partial_refund', 'refunded', 'partial_chargeback', 'chargeback')"},
		{&models.BillableSpec{}, "chk_billable_specs_type", "type IN ('project', 'database')"},
		{&models.BillableSpec{}, "chk_billable_specs_positive", "cpu_millicores > 0 AND memory_mb > 0 AND storage_gb > 0 AND monthly_credits > 0 AND version > 0"},
		{&models.BillableResource{}, "chk_billable_resources_type", "type IN ('project', 'database')"},
		{&models.BillableResource{}, "chk_billable_resources_status", "billing_status IN ('active', 'payment_due', 'suspended', 'deleted')"},
		{&models.BillableResource{}, "chk_billable_resources_period", "next_invoice_at > current_period_start"},
		{&models.BillableResource{}, "chk_billable_resources_anchor_day", "billing_anchor_day BETWEEN 1 AND 31"},
		{&models.Invoice{}, "chk_invoices_total_nonnegative", "total_credits >= 0"},
		{&models.Invoice{}, "chk_invoices_status", "status IN ('pending', 'paid', 'payment_due', 'void')"},
		{&models.Invoice{}, "chk_invoices_period", "period_end > period_start"},
		{&models.InvoiceItem{}, "chk_invoice_items_credits_nonnegative", "credits >= 0"},
		{&models.PaymentEvent{}, "chk_payment_events_provider", "provider = 'midtrans'"},
	}
	for _, check := range checks {
		if err := EnsureConstraint(db, check.model, check.name, "CHECK ("+check.rule+")"); err != nil {
			return err
		}
	}

	if err := db.Exec(`
		CREATE OR REPLACE FUNCTION reject_wallet_ledger_entry_mutation() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'wallet ledger entries are append-only';
		END;
		$$ LANGUAGE plpgsql;
		CREATE OR REPLACE FUNCTION reject_topup_package_price_mutation() RETURNS trigger AS $$
		BEGIN
			IF TG_OP = 'DELETE' THEN
				RAISE EXCEPTION 'topup packages cannot be deleted';
			END IF;
			IF NEW.credits <> OLD.credits OR NEW.currency <> OLD.currency
				OR NEW.amount_minor <> OLD.amount_minor OR NEW.provider <> OLD.provider
				OR NEW.version <> OLD.version THEN
				RAISE EXCEPTION 'topup package prices are immutable';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE OR REPLACE FUNCTION reject_billable_spec_price_mutation() RETURNS trigger AS $$
		BEGIN
			IF TG_OP = 'DELETE' THEN
				RAISE EXCEPTION 'billable specs cannot be deleted';
			END IF;
			IF NEW.type IS DISTINCT FROM OLD.type OR NEW.name IS DISTINCT FROM OLD.name
				OR NEW.slug IS DISTINCT FROM OLD.slug OR NEW.cpu_millicores IS DISTINCT FROM OLD.cpu_millicores
				OR NEW.memory_mb IS DISTINCT FROM OLD.memory_mb OR NEW.storage_gb IS DISTINCT FROM OLD.storage_gb
				OR NEW.monthly_credits IS DISTINCT FROM OLD.monthly_credits
				OR NEW.connection_limit IS DISTINCT FROM OLD.connection_limit
				OR NEW.backup_retention_days IS DISTINCT FROM OLD.backup_retention_days
				OR NEW.version IS DISTINCT FROM OLD.version THEN
				RAISE EXCEPTION 'billable spec prices are immutable';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE OR REPLACE FUNCTION reject_invoice_identity_mutation() RETURNS trigger AS $$
		BEGIN
			IF TG_OP = 'DELETE' THEN
				RAISE EXCEPTION 'invoices cannot be deleted';
			END IF;
			IF NEW.user_id IS DISTINCT FROM OLD.user_id OR NEW.wallet_id IS DISTINCT FROM OLD.wallet_id
				OR NEW.period_start IS DISTINCT FROM OLD.period_start OR NEW.period_end IS DISTINCT FROM OLD.period_end
				OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key THEN
				RAISE EXCEPTION 'invoice identity fields are immutable';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
	CREATE OR REPLACE FUNCTION reject_invoice_item_identity_mutation() RETURNS trigger AS $$
		BEGIN
			IF TG_OP = 'DELETE' THEN
				RAISE EXCEPTION 'invoice items cannot be deleted';
			END IF;
			IF NEW.invoice_id IS DISTINCT FROM OLD.invoice_id OR NEW.billable_resource_id IS DISTINCT FROM OLD.billable_resource_id
				OR NEW.spec_id IS DISTINCT FROM OLD.spec_id OR NEW.description IS DISTINCT FROM OLD.description
				OR NEW.credits IS DISTINCT FROM OLD.credits THEN
				RAISE EXCEPTION 'invoice item identity fields are immutable';
			END IF;
			RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;
	CREATE OR REPLACE FUNCTION reject_topup_economic_mutation() RETURNS trigger AS $$
	BEGIN
		IF TG_OP = 'DELETE' THEN
			RAISE EXCEPTION 'topups cannot be deleted';
		END IF;
		IF NEW.wallet_id IS DISTINCT FROM OLD.wallet_id
			OR NEW.topup_package_id IS DISTINCT FROM OLD.topup_package_id
			OR NEW.client_idempotency_key IS DISTINCT FROM OLD.client_idempotency_key
			OR NEW.provider IS DISTINCT FROM OLD.provider
			OR NEW.provider_order_id IS DISTINCT FROM OLD.provider_order_id
			OR (OLD.provider_transaction_id IS NOT NULL AND NEW.provider_transaction_id IS DISTINCT FROM OLD.provider_transaction_id)
			OR NEW.amount_minor IS DISTINCT FROM OLD.amount_minor
			OR NEW.currency IS DISTINCT FROM OLD.currency
			OR NEW.credits IS DISTINCT FROM OLD.credits THEN
			RAISE EXCEPTION 'topup economic and provider identity fields are immutable';
		END IF;
		IF OLD.paid_at IS NOT NULL AND NEW.paid_at IS DISTINCT FROM OLD.paid_at THEN
			RAISE EXCEPTION 'topup paid timestamp is immutable once set';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;
	CREATE OR REPLACE FUNCTION reject_payment_event_mutation() RETURNS trigger AS $$
	BEGIN
		IF TG_OP = 'DELETE' THEN
			RAISE EXCEPTION 'payment events cannot be deleted';
		END IF;
		IF NEW.provider IS DISTINCT FROM OLD.provider
			OR NEW.event_key IS DISTINCT FROM OLD.event_key
			OR NEW.provider_order_id IS DISTINCT FROM OLD.provider_order_id
			OR NEW.payload_json IS DISTINCT FROM OLD.payload_json
			OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
			RAISE EXCEPTION 'payment event evidence is append-only';
		END IF;
		IF OLD.processed_at IS NOT NULL AND NEW.processed_at IS DISTINCT FROM OLD.processed_at THEN
			RAISE EXCEPTION 'payment event processed timestamp is immutable once set';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;
		CREATE OR REPLACE FUNCTION reject_impersonation_audit_mutation() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'impersonation audit entries are append-only';
		END;
		$$ LANGUAGE plpgsql;
		CREATE OR REPLACE FUNCTION reject_billing_audit_event_mutation() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'billing audit events are append-only';
		END;
		$$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS trg_wallet_ledger_entries_append_only ON wallet_ledger_entries;
		CREATE TRIGGER trg_wallet_ledger_entries_append_only
		BEFORE UPDATE OR DELETE ON wallet_ledger_entries
		FOR EACH ROW EXECUTE FUNCTION reject_wallet_ledger_entry_mutation();
		DROP TRIGGER IF EXISTS trg_topup_packages_immutable_price ON topup_packages;
		CREATE TRIGGER trg_topup_packages_immutable_price
		BEFORE UPDATE OR DELETE ON topup_packages
		FOR EACH ROW EXECUTE FUNCTION reject_topup_package_price_mutation();
		DROP TRIGGER IF EXISTS trg_billable_specs_immutable_price ON billable_specs;
		CREATE TRIGGER trg_billable_specs_immutable_price
		BEFORE UPDATE OR DELETE ON billable_specs
		FOR EACH ROW EXECUTE FUNCTION reject_billable_spec_price_mutation();
		DROP TRIGGER IF EXISTS trg_invoices_immutable_identity ON invoices;
		CREATE TRIGGER trg_invoices_immutable_identity
		BEFORE UPDATE OR DELETE ON invoices
		FOR EACH ROW EXECUTE FUNCTION reject_invoice_identity_mutation();
	DROP TRIGGER IF EXISTS trg_invoice_items_immutable_identity ON invoice_items;
	CREATE TRIGGER trg_invoice_items_immutable_identity
	BEFORE UPDATE OR DELETE ON invoice_items
	FOR EACH ROW EXECUTE FUNCTION reject_invoice_item_identity_mutation();
	DROP TRIGGER IF EXISTS trg_topups_immutable_economic_identity ON topups;
	CREATE TRIGGER trg_topups_immutable_economic_identity
	BEFORE UPDATE OR DELETE ON topups
	FOR EACH ROW EXECUTE FUNCTION reject_topup_economic_mutation();
	DROP TRIGGER IF EXISTS trg_payment_events_append_only ON payment_events;
	CREATE TRIGGER trg_payment_events_append_only
	BEFORE UPDATE OR DELETE ON payment_events
	FOR EACH ROW EXECUTE FUNCTION reject_payment_event_mutation();
		DROP TRIGGER IF EXISTS trg_impersonation_audits_append_only ON impersonation_audits;
		CREATE TRIGGER trg_impersonation_audits_append_only
		BEFORE UPDATE OR DELETE ON impersonation_audits
		FOR EACH ROW EXECUTE FUNCTION reject_impersonation_audit_mutation();
		DROP TRIGGER IF EXISTS trg_billing_audit_events_append_only ON billing_audit_events;
		CREATE TRIGGER trg_billing_audit_events_append_only
		BEFORE UPDATE OR DELETE ON billing_audit_events
		FOR EACH ROW EXECUTE FUNCTION reject_billing_audit_event_mutation();
	`).Error; err != nil {
		return fmt.Errorf("create billing immutability triggers: %w", err)
	}
	return nil
}

// dropCheckMissingMarker drops a CHECK constraint whose definition lacks marker, so the
// EnsureConstraint pass can recreate it. EnsureConstraint is a no-op when the name exists,
// which makes dropping the only way to widen an existing rule.
func dropCheckMissingMarker(db *gorm.DB, table, constraint, marker string) error {
	var definition string
	err := db.Raw(`
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = ? AND c.conname = ?
	`, table, constraint).Scan(&definition).Error
	if err != nil {
		return fmt.Errorf("read %s constraint: %w", constraint, err)
	}
	if definition == "" || strings.Contains(definition, marker) {
		return nil
	}
	if err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", table, constraint)).Error; err != nil {
		return fmt.Errorf("upgrade %s constraint: %w", constraint, err)
	}
	return nil
}

func upgradeTopupStatusConstraint(db *gorm.DB) error {
	if !isPostgres(db) {
		return nil
	}
	var definition string
	err := db.Raw(`
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = 'topups' AND c.conname = 'chk_topups_status'
	`).Scan(&definition).Error
	if err != nil {
		return fmt.Errorf("read topup status constraint: %w", err)
	}
	if definition == "" || (strings.Contains(definition, "partial_refund") && strings.Contains(definition, "partial_chargeback")) {
		return nil
	}
	if err := db.Exec("ALTER TABLE topups DROP CONSTRAINT chk_topups_status").Error; err != nil {
		return fmt.Errorf("upgrade topup status constraint: %w", err)
	}
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
	if db == nil {
		return errors.New("database handle is nil")
	}
	// db.Connection installs a session-scoped *sql.Conn so PostgreSQL advisory
	// locks span the full migration. gorm.DB() rejects that connection type;
	// execute the probe through the active GORM session instead.
	if err := db.Exec("SELECT 1").Error; err != nil {
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
	if err := db.Where("database_name != '' AND database_name IS NOT NULL AND database_name NOT LIKE 'detached_%'").Find(&projects).Error; err != nil {
		return err
	}

	backfilled := 0
	for _, p := range projects {
		var count int64
		db.Model(&models.DatabaseInstance{}).Where("project_id = ?", p.ID).Count(&count)
		if count > 0 {
			continue
		}

		projID := p.ID
		instance := models.DatabaseInstance{
			UserID:    p.UserID,
			ProjectID: &projID,
			Engine:    "mysql",
			Status:    models.DBStatusActive,
			Name:      p.GetDatabaseName(),
			Username:  p.GetDatabaseName(),
			Password:  p.DatabasePassword,
			Host:      infrastructure.MySQLContainerName(),
			Port:      infrastructure.MySQLPort(),
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
	cfg := config.Load()
	dataPath := cfg.DataPath

	slog.Info("Starting database UserSlug population and filesystem tenant layout migration...")

	// 1. Populate UserSlug in the database for any legacy projects
	var allProjects []models.Project
	if err := db.Preload("User").Find(&allProjects).Error; err == nil {
		for _, p := range allProjects {
			if p.UserSlug == "" || p.UserSlug == "user-unknown" {
				slug := models.NormalizeSlug(p.User.Name)
				if slug == "" {
					parts := strings.Split(p.User.Email, "@")
					if len(parts) > 0 {
						slug = models.NormalizeSlug(parts[0])
					}
				}
				if slug == "" {
					slug = "user"
				}
				p.UserSlug = fmt.Sprintf("%s-%d", slug, p.UserID)
				if err := db.Model(&p).Update("user_slug", p.UserSlug).Error; err != nil {
					slog.Error("Failed to update user_slug for project", "id", p.ID, "error", err)
				}
			}
		}
	}

	// 2. Fetch projects again to ensure we have updated slugs
	var projects []models.Project
	if err := db.Find(&projects).Error; err != nil {
		return fmt.Errorf("failed to fetch projects for file migration: %w", err)
	}

	// Track parent migrations we have performed to avoid duplicate log spams
	migratedParents := make(map[string]bool)

	for _, p := range projects {
		userFolder := p.UserSlug
		if userFolder == "" {
			userFolder = fmt.Sprintf("user-%d", p.UserID)
		}

		// A. Rename projects/user-<id> to projects/<slug>-<id>
		legacyParentPath := filepath.Join(projectsPath, fmt.Sprintf("user-%d", p.UserID))
		newParentPath := filepath.Join(projectsPath, userFolder)

		if !migratedParents[legacyParentPath] && legacyParentPath != newParentPath {
			if info, err := os.Stat(legacyParentPath); err == nil && info.IsDir() {
				slog.Info("Migrating projects parent directory to named user slug", "from", legacyParentPath, "to", newParentPath)
				if err := os.Rename(legacyParentPath, newParentPath); err != nil {
					slog.Warn("Failed to rename legacy projects user folder, attempting subfolder-level migration", "from", legacyParentPath, "to", newParentPath, "error", err)
					files, readErr := os.ReadDir(legacyParentPath)
					if readErr == nil {
						_ = os.MkdirAll(newParentPath, 0755)
						for _, f := range files {
							oldSub := filepath.Join(legacyParentPath, f.Name())
							newSub := filepath.Join(newParentPath, f.Name())
							if renameErr := os.Rename(oldSub, newSub); renameErr != nil {
								slog.Error("Failed to migrate project subfolder during fallback", "from", oldSub, "to", newSub, "error", renameErr)
							} else {
								slog.Info("Migrated project subfolder during fallback", "from", oldSub, "to", newSub)
							}
						}
						_ = os.Remove(legacyParentPath)
					}
				}
			}
			migratedParents[legacyParentPath] = true
		}

		// B. Rename data/user-<id> to data/<slug>-<id> (for backups and storage volumes)
		legacyDataParentPath := filepath.Join(dataPath, fmt.Sprintf("user-%d", p.UserID))
		newDataParentPath := filepath.Join(dataPath, userFolder)

		if !migratedParents[legacyDataParentPath] && legacyDataParentPath != newDataParentPath {
			if info, err := os.Stat(legacyDataParentPath); err == nil && info.IsDir() {
				slog.Info("Migrating data parent directory to named user slug", "from", legacyDataParentPath, "to", newDataParentPath)
				if err := os.Rename(legacyDataParentPath, newDataParentPath); err != nil {
					slog.Warn("Failed to rename legacy data user folder, attempting subfolder-level migration", "from", legacyDataParentPath, "to", newDataParentPath, "error", err)
					files, readErr := os.ReadDir(legacyDataParentPath)
					if readErr == nil {
						_ = os.MkdirAll(newDataParentPath, 0755)
						for _, f := range files {
							oldSub := filepath.Join(legacyDataParentPath, f.Name())
							newSub := filepath.Join(newDataParentPath, f.Name())
							if renameErr := os.Rename(oldSub, newSub); renameErr != nil {
								slog.Error("Failed to migrate data subfolder during fallback", "from", oldSub, "to", newSub, "error", renameErr)
							} else {
								slog.Info("Migrated data subfolder during fallback", "from", oldSub, "to", newSub)
							}
						}
						_ = os.Remove(legacyDataParentPath)
					}
				}
			}
			migratedParents[legacyDataParentPath] = true
		}

		// C. Handle old flat-layout to tenant-layout fallback migrations (if any old files exist)
		legacyPath := filepath.Join(projectsPath, p.Subdomain)
		newPath := filepath.Join(newParentPath, p.Subdomain)
		if info, err := os.Stat(legacyPath); err == nil && info.IsDir() {
			slog.Info("Migrating project directory from legacy flat format to named user tenant folder", "project", p.Name, "legacy", legacyPath, "new", newPath)
			if err := os.MkdirAll(newParentPath, 0755); err != nil {
				slog.Error("Failed to create user directory", "path", newParentPath, "error", err)
				continue
			}
			if err := os.Rename(legacyPath, newPath); err != nil {
				slog.Error("Failed to rename project directory", "from", legacyPath, "to", newPath, "error", err)
				continue
			}
		}

		// Cleanup env backup files to protect secrets
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

// BackfillLaravelAppKeys ensures all existing Laravel projects have an APP_KEY in their secret store.
func BackfillLaravelAppKeys(db *gorm.DB, cfg *config.Config) error {
	backfilled := 0
	stretchedKey := utils.DeriveKey(cfg.CredentialEncryptionKey)

	var projects []models.Project
	err := db.Where("framework = 'Laravel'").FindInBatches(&projects, 100, func(tx *gorm.DB, batch int) error {
		for _, p := range projects {
			var bindings []models.SecretStoreBinding
			if err := tx.Where("project_id = ?", p.ID).Find(&bindings).Error; err != nil {
				continue
			}

			appKeyExists := false
			var storeID uint
			if len(bindings) > 0 {
				storeID = bindings[0].SecretStoreID
				var storeIDs []uint
				for _, b := range bindings {
					storeIDs = append(storeIDs, b.SecretStoreID)
				}
				var count int64
				tx.Model(&models.SecretStoreItem{}).Where("secret_store_id IN (?) AND key = ?", storeIDs, "APP_KEY").Count(&count)
				if count > 0 {
					appKeyExists = true
				}
			}

			if appKeyExists {
				continue
			}

			// Create store and binding if none exist.
			if storeID == 0 {
				store := models.SecretStore{
					UserID:      p.UserID,
					Name:        fmt.Sprintf("Environment Secrets (%s)", p.Name),
					Description: "Managed variables for project " + p.Name,
				}
				if err := tx.Create(&store).Error; err != nil {
					slog.Warn("Failed to create secret store during backfill", "projectID", p.ID, "error", err)
					continue
				}
				storeID = store.ID

				newBinding := models.SecretStoreBinding{
					ProjectID:     p.ID,
					SecretStoreID: store.ID,
					Environment:   "production",
				}
				if err := tx.Create(&newBinding).Error; err != nil {
					slog.Warn("Failed to bind secret store during backfill", "projectID", p.ID, "error", err)
					continue
				}
			}

			keyBytes := make([]byte, 32)
			if _, err := rand.Read(keyBytes); err != nil {
				slog.Warn("Failed to generate random key bytes during backfill", "error", err)
				continue
			}

			appKey := "base64:" + base64.StdEncoding.EncodeToString(keyBytes)
			encryptedVal, err := utils.Encrypt(appKey, stretchedKey)
			if err != nil {
				slog.Warn("Failed to encrypt APP_KEY during backfill", "error", err)
				continue
			}

			errTx := tx.Transaction(func(txInner *gorm.DB) error {
				item := models.SecretStoreItem{
					SecretStoreID:         storeID,
					Key:                   "APP_KEY",
					LatestSnapshotVersion: 1,
				}
				if err := txInner.Create(&item).Error; err != nil {
					return err
				}
				itemValue := models.SecretStoreItemValue{
					SecretStoreItemID: item.ID,
					Version:           1,
					EncryptedValue:    encryptedVal,
					CreatedBy:         p.UserID,
				}
				return txInner.Create(&itemValue).Error
			})

			if errTx != nil {
				slog.Warn("Failed to save APP_KEY during backfill", "projectID", p.ID, "error", errTx)
				continue
			}

			backfilled++
		}
		return nil
	}).Error

	if err != nil {
		return err
	}

	if backfilled > 0 {
		slog.Info("Backfilled APP_KEY for existing Laravel projects", "count", backfilled)
	}

	return nil
}

// BackfillPrimaryDomains sets IsPrimary = true for the first custom domain of existing projects
func BackfillPrimaryDomains(db *gorm.DB) error {
	var projects []models.Project
	err := db.FindInBatches(&projects, 100, func(tx *gorm.DB, batch int) error {
		for _, p := range projects {
			var firstDomain models.CustomDomain
			errFirst := tx.Where("project_id = ?", p.ID).Order("created_at ASC").First(&firstDomain).Error
			if errFirst == nil {
				var count int64
				tx.Model(&models.CustomDomain{}).Where("project_id = ? AND is_primary = ?", p.ID, true).Count(&count)
				if count == 0 {
					firstDomain.IsPrimary = true
					if errSave := tx.Save(&firstDomain).Error; errSave != nil {
						slog.Warn("Failed to save primary domain during backfill", "projectID", p.ID, "domain", firstDomain.Domain, "error", errSave)
					}
				}
			}
		}
		return nil
	}).Error

	return err
}

func migrateDatabaseInstancesTable(db *gorm.DB) error {
	if !db.Migrator().HasTable("database_instances") {
		return nil
	}

	// 1. Add user_id column if not exists
	hasUserID, err := databaseInstancesHasUserID(db)
	if err != nil {
		return err
	}
	if !hasUserID {
		slog.Info("Migrating database_instances: adding user_id column")
		if isPostgres(db) {
			if err := db.Exec("ALTER TABLE database_instances ADD COLUMN user_id INTEGER;").Error; err != nil {
				return fmt.Errorf("failed to add user_id column: %w", err)
			}
			if err := db.Exec("UPDATE database_instances di SET user_id = p.user_id FROM projects p WHERE di.project_id = p.id;").Error; err != nil {
				slog.Warn("Failed to backfill user_id from projects table", "error", err)
			}
			// Check for unresolved rows
			var unresolvedCount int64
			if err := db.Raw("SELECT COUNT(*) FROM database_instances WHERE user_id IS NULL").Scan(&unresolvedCount).Error; err == nil && unresolvedCount > 0 {
				return fmt.Errorf("migration failed: found %d database_instance rows with unresolvable user_id owner. Explicit remediation is required", unresolvedCount)
			}
			if err := db.Exec("ALTER TABLE database_instances ALTER COLUMN user_id SET NOT NULL;").Error; err != nil {
				return fmt.Errorf("failed to make user_id NOT NULL: %w", err)
			}
		} else {
			// MySQL or SQLite
			if err := db.Migrator().AddColumn(&models.DatabaseInstance{}, "UserID"); err != nil {
				return fmt.Errorf("failed to add user_id column: %w", err)
			}
			if err := db.Exec("UPDATE database_instances di JOIN projects p ON di.project_id = p.id SET di.user_id = p.user_id;").Error; err != nil {
				slog.Warn("Failed to backfill user_id from projects table", "error", err)
			}
			// Check for unresolved rows
			var unresolvedCount int64
			if err := db.Raw("SELECT COUNT(*) FROM database_instances WHERE user_id IS NULL").Scan(&unresolvedCount).Error; err == nil && unresolvedCount > 0 {
				return fmt.Errorf("migration failed: found %d database_instance rows with unresolvable user_id owner. Explicit remediation is required", unresolvedCount)
			}
		}
	}

	// 2. Make project_id nullable if it was not null
	if isPostgres(db) {
		slog.Info("Migrating database_instances: making project_id nullable")
		if err := db.Exec("ALTER TABLE database_instances ALTER COLUMN project_id DROP NOT NULL;").Error; err != nil {
			slog.Warn("Failed to make project_id nullable in migration", "error", err)
		}
	}

	return nil
}

func databaseInstancesHasUserID(db *gorm.DB) (bool, error) {
	if !isPostgres(db) {
		return db.Migrator().HasColumn(&models.DatabaseInstance{}, "user_id"), nil
	}
	var exists bool
	if err := db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA()
			  AND table_name = 'database_instances'
			  AND column_name = 'user_id'
		)
	`).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("check database_instances.user_id column: %w", err)
	}
	return exists, nil
}
