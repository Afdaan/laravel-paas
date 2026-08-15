//go:build integration

package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBillingPostgres(t *testing.T) {
	dsn := billingTestDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := DefensiveMigrationBootstrap(db); err != nil {
		t.Fatal(err)
	}
	_ = db.Exec("DELETE FROM billable_specs WHERE slug LIKE 'reconcile%'; DELETE FROM topup_packages WHERE credits = 987654321;").Error
	cfg := &config.Config{BaseDomain: "console.example.com", ProjectDomain: "apps.example.net"}
	if err := Seed(db, cfg); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "billing-postgres-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.test", Password: "test", Name: "Billing Test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	wallet := models.Wallet{UserID: user.ID, BalanceCredits: -100}
	if err := db.Create(&wallet).Error; err != nil {
		t.Fatalf("negative wallet balance rejected: %v", err)
	}

	entry := models.WalletLedgerEntry{WalletID: wallet.ID, Type: models.WalletLedgerEntryTopupReversal, AmountCredits: -100, BalanceAfter: -100, IdempotencyKey: fmt.Sprintf("billing-postgres-reversal-%d", user.ID), CreatedBy: &user.ID}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	var pkg models.TopupPackage
	if err := db.Where("credits = ? AND version = ?", 100, 1).First(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TopupPackage{Currency: pkg.Currency, Credits: pkg.Credits, AmountMinor: pkg.AmountMinor + 1, Version: 2, IsActive: true}).Error; err == nil {
		t.Fatal("multiple active package versions allowed after first migration")
	}
	var spec models.BillableSpec
	if err := db.Where("type = ? AND slug = ? AND version = ?", models.BillableTypeProject, "small", 1).First(&spec).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.BillableSpec{Type: spec.Type, Name: spec.Name, Slug: spec.Slug, CPUMillicores: spec.CPUMillicores, MemoryMB: spec.MemoryMB, StorageGB: spec.StorageGB, MonthlyCredits: spec.MonthlyCredits + 1, Version: 2, IsActive: true}).Error; err == nil {
		t.Fatal("multiple active billable spec versions allowed after first migration")
	}
	if err := db.Exec(`
		DROP INDEX IF EXISTS uni_topup_packages_one_active;
		DROP INDEX IF EXISTS uni_billable_specs_one_active;
	`).Error; err != nil {
		t.Fatal(err)
	}
	testRunID := time.Now().UnixNano()
	duplicatePackageV1 := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: testRunID, AmountMinor: testRunID, Version: 1, IsActive: true}
	duplicatePackageV2 := duplicatePackageV1
	duplicatePackageV2.Version = 2
	duplicatePackageV2.AmountMinor++
	if err := db.Create(&duplicatePackageV1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&duplicatePackageV2).Error; err != nil {
		t.Fatal(err)
	}
	duplicateSpecSlug := fmt.Sprintf("reconcile-%d", testRunID)
	duplicateSpecV1 := models.BillableSpec{Type: models.BillableTypeProject, Name: "Reconcile", Slug: duplicateSpecSlug, CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100, Version: 1, IsActive: true}
	duplicateSpecV2 := duplicateSpecV1
	duplicateSpecV2.Version = 2
	duplicateSpecV2.MonthlyCredits++
	if err := db.Create(&duplicateSpecV1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&duplicateSpecV2).Error; err != nil {
		t.Fatal(err)
	}

	var fkCount int
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_constraint c
		JOIN pg_class source ON source.oid = c.conrelid
		JOIN pg_class target ON target.oid = c.confrelid
		WHERE c.conname = 'fk_wallet_ledger_entries_created_by'
		  AND source.relname = 'wallet_ledger_entries'
		  AND target.relname = 'users'
	`).Scan(&fkCount).Error; err != nil || fkCount != 1 {
		t.Fatalf("ledger creator FK missing or reversed: count=%d err=%v", fkCount, err)
	}
	if err := DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	var activeIndexCount int
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND tablename = $table$topup_packages$table$
		  AND indexname = $index$uni_topup_packages_one_active$index$
	`).Scan(&activeIndexCount).Error; err != nil || activeIndexCount != 1 {
		t.Fatalf("active topup package index missing: count=%d err=%v", activeIndexCount, err)
	}
	if err := db.First(&duplicatePackageV1, duplicatePackageV1.ID).Error; err != nil || duplicatePackageV1.IsActive {
		t.Fatalf("lower active package version not retired: %#v, %v", duplicatePackageV1, err)
	}
	if err := db.First(&duplicatePackageV2, duplicatePackageV2.ID).Error; err != nil || !duplicatePackageV2.IsActive {
		t.Fatalf("highest active package version not retained: %#v, %v", duplicatePackageV2, err)
	}
	if err := db.First(&duplicateSpecV1, duplicateSpecV1.ID).Error; err != nil || duplicateSpecV1.IsActive {
		t.Fatalf("lower active spec version not retired: %#v, %v", duplicateSpecV1, err)
	}
	if err := db.First(&duplicateSpecV2, duplicateSpecV2.ID).Error; err != nil || !duplicateSpecV2.IsActive {
		t.Fatalf("highest active spec version not retained: %#v, %v", duplicateSpecV2, err)
	}
	if err := db.Model(&entry).Update("balance_after", 0).Error; err == nil {
		t.Fatal("ledger update allowed")
	}
	if err := db.Delete(&entry).Error; err == nil {
		t.Fatal("ledger delete allowed")
	}

	if err := db.Model(&pkg).Update("amount_minor", 1).Error; err == nil {
		t.Fatal("topup package price mutation allowed")
	}
	if err := db.Delete(&pkg).Error; err == nil {
		t.Fatal("topup package delete allowed")
	}
	if err := db.Model(&spec).Update("monthly_credits", 1).Error; err == nil {
		t.Fatal("billable spec price mutation allowed")
	}
	periodStart := time.Now().UTC().Truncate(time.Second)
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: uint(time.Now().UTC().UnixNano()), SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: periodStart, NextInvoiceAt: periodStart.AddDate(0, 1, 0), BillingAnchorDay: periodStart.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	invoice := models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0), Status: models.InvoiceStatusPending, IdempotencyKey: fmt.Sprintf("invoice-immutable-%d", user.ID)}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	item := models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "PostgreSQL immutability test", Credits: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&invoice).Update("period_start", periodStart.AddDate(0, 1, 0)).Error; err == nil {
		t.Fatal("invoice period mutation allowed")
	}
	if err := db.Model(&invoice).Update("status", models.InvoiceStatusPaymentDue).Error; err != nil {
		t.Fatalf("invoice status mutation rejected: %v", err)
	}
	if err := db.Model(&item).Update("credits", 2).Error; err == nil {
		t.Fatal("invoice item identity mutation allowed")
	}
	if err := db.Delete(&item).Error; err == nil {
		t.Fatal("invoice item deletion allowed")
	}
	if err := db.Delete(&invoice).Error; err == nil {
		t.Fatal("invoice deletion allowed")
	}
	topup := models.Topup{
		WalletID:             wallet.ID,
		ClientIdempotencyKey: fmt.Sprintf("topup-provider-identity-%d", user.ID),
		Provider:             string(models.BillingProviderMidtrans),
		ProviderOrderID:      fmt.Sprintf("order-provider-identity-%d", user.ID),
		AmountMinor:          10000,
		Currency:             string(models.BillingCurrencyIDR),
		Credits:              100,
		Status:               models.TopupStatusPending,
	}
	if err := db.Create(&topup).Error; err != nil {
		t.Fatal(err)
	}
	transactionID := "midtrans-transaction-1"
	if err := db.Model(&topup).Update("provider_transaction_id", transactionID).Error; err != nil {
		t.Fatalf("initial provider transaction ID rejected: %v", err)
	}
	if err := db.Model(&topup).Update("provider_transaction_id", "midtrans-transaction-2").Error; err == nil {
		t.Fatal("provider transaction ID mutation allowed")
	}
	audit := models.BillingAuditEvent{ActorUserID: user.ID, EffectiveUserID: user.ID, ActorRole: string(models.RoleSuperAdmin), SourceIP: "127.0.0.1", Reason: "PostgreSQL immutability test", Event: "billable_spec.repriced", TargetType: "billable_spec", TargetID: spec.ID, BeforeJSON: "{}", AfterJSON: "{}", RequestID: "billing-postgres-test"}
	if err := db.Create(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&audit).Update("event", "changed").Error; err == nil {
		t.Fatal("billing audit event update allowed")
	}
	if err := db.Delete(&audit).Error; err == nil {
		t.Fatal("billing audit event delete allowed")
	}
	if err := db.Delete(&user).Error; err != nil {
		t.Fatalf("soft delete user: %v", err)
	}
	if err := db.Unscoped().Delete(&user).Error; err == nil {
		t.Fatal("hard delete user with wallet allowed")
	}
}

func TestDefensiveMigrationBootstrapWaitsForSessionLock(t *testing.T) {
	dsn := billingTestDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	blockedDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Connection(func(connection *gorm.DB) error {
		sessionConn, err := postgresSessionConn(connection)
		if err != nil {
			return err
		}
		if _, err := sessionConn.ExecContext(context.Background(), "SELECT pg_advisory_lock(hashtextextended($1, 0))", defensiveMigrationLockIdentity); err != nil {
			return err
		}
		locked := true
		defer func() {
			if locked {
				_, _ = sessionConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", defensiveMigrationLockIdentity)
			}
		}()

		completed := make(chan error, 1)
		go func() { completed <- DefensiveMigrationBootstrap(blockedDB) }()
		select {
		case err := <-completed:
			return fmt.Errorf("migration bypassed session lock: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
		if _, err := sessionConn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtextextended($1, 0))", defensiveMigrationLockIdentity); err != nil {
			return err
		}
		locked = false
		if err := <-completed; err != nil {
			return fmt.Errorf("migration after session lock release: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateBillingTestDSN(t *testing.T) {
	for name, test := range map[string]struct {
		dsn     string
		wantErr bool
	}{
		"postgres URI":        {dsn: "postgres://runara:test-password@localhost/runara_billing_test?sslmode=disable"},
		"postgresql URI":      {dsn: "postgresql://runara:test-password@localhost/runara_billing_test?sslmode=disable"},
		"production database": {dsn: "postgres://runara:test-password@prod/runara?sslmode=disable", wantErr: true},
		"test password only":  {dsn: "postgres://runara:test-password@prod/runara?application_name=test", wantErr: true},
		"dbname override":     {dsn: "postgres://runara:test-password@prod/runara_billing_test?dbname=production", wantErr: true},
		"database override":   {dsn: "postgres://runara:test-password@prod/runara_billing_test?database=production", wantErr: true},
		"nested path":         {dsn: "postgres://runara:test-password@localhost/runara_billing_test/extra", wantErr: true},
		"invalid URI":         {dsn: "://runara_billing_test", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateBillingTestDSN(test.dsn)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateBillingTestDSN(%q) error = %v, wantErr=%t", test.dsn, err, test.wantErr)
			}
		})
	}
}

func TestPostgresUpgradeFromLegacyTopupPackagesProviderSchema(t *testing.T) {
	dsn := billingTestDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Teardown tables and simulate legacy schema with provider NOT NULL and old constraints
	_ = db.Exec("DROP TABLE IF EXISTS topups CASCADE").Error
	_ = db.Exec("DROP TABLE IF EXISTS topup_packages CASCADE").Error
	if err := db.Exec(`
		CREATE TABLE topup_packages (
			id bigserial PRIMARY KEY,
			created_at timestamptz,
			updated_at timestamptz,
			provider text NOT NULL,
			currency text NOT NULL,
			credits bigint NOT NULL,
			amount_minor bigint NOT NULL,
			is_active boolean DEFAULT true,
			sort_order integer DEFAULT 0,
			CONSTRAINT chk_topup_packages_provider_currency CHECK (provider IN ('midtrans') AND currency IN ('IDR', 'USD')),
			CONSTRAINT uni_topup_packages_provider_currency_credits UNIQUE (provider, currency, credits)
		);
		CREATE UNIQUE INDEX uni_topup_packages_one_active ON topup_packages (provider, currency, credits) WHERE is_active;
		INSERT INTO topup_packages (created_at, updated_at, provider, currency, credits, amount_minor, is_active, sort_order)
		VALUES (NOW(), NOW(), 'midtrans', 'IDR', 100, 100000, true, 1);
	`).Error; err != nil {
		t.Fatalf("failed to create legacy topup_packages table: %v", err)
	}

	// 2. Run migration
	if err := DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("migration bootstrap failed: %v", err)
	}

	// 3. Verify existing legacy row was preserved and migrated
	var legacyPkg models.TopupPackage
	if err := db.Where("credits = ? AND currency = ?", 100, "IDR").First(&legacyPkg).Error; err != nil {
		t.Fatalf("failed to find legacy package after migration: %v", err)
	}
	if legacyPkg.Version != 1 || legacyPkg.AmountMinor != 100000 {
		t.Fatalf("unexpected legacy package data: %+v", legacyPkg)
	}

	// 4. Verify new package can be inserted without provider column
	newPkg := models.TopupPackage{
		Currency:    "IDR",
		Credits:     250,
		AmountMinor: 250000,
		Version:     1,
		IsActive:    true,
		SortOrder:   2,
	}
	if err := db.Create(&newPkg).Error; err != nil {
		t.Fatalf("failed to create new topup package on upgraded schema: %v", err)
	}

	// 5. Verify repricing (version 2) works on upgraded schema
	if err := db.Model(&newPkg).Update("is_active", false).Error; err != nil {
		t.Fatalf("failed to deactivate v1: %v", err)
	}
	v2Pkg := models.TopupPackage{
		Currency:    "IDR",
		Credits:     250,
		AmountMinor: 275000,
		Version:     2,
		IsActive:    true,
		SortOrder:   2,
	}
	if err := db.Create(&v2Pkg).Error; err != nil {
		t.Fatalf("failed to create v2 topup package on upgraded schema: %v", err)
	}
}

func billingTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("BILLING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BILLING_TEST_DATABASE_URL not set")
	}
	if err := validateBillingTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	return dsn
}

func validateBillingTestDSN(dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse BILLING_TEST_DATABASE_URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("BILLING_TEST_DATABASE_URL must use a PostgreSQL URI")
	}
	if parsed.Path != "/runara_billing_test" {
		return fmt.Errorf("BILLING_TEST_DATABASE_URL must target database runara_billing_test")
	}
	query := parsed.Query()
	if query.Has("dbname") || query.Has("database") {
		return fmt.Errorf("BILLING_TEST_DATABASE_URL must not override database")
	}
	return nil
}
