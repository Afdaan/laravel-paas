//go:build integration

package database

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBillingPostgres(t *testing.T) {
	dsn := os.Getenv("BILLING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BILLING_TEST_DATABASE_URL not set")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("BILLING_TEST_DATABASE_URL must target a test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := DefensiveMigrationBootstrap(db); err != nil {
		t.Fatal(err)
	}
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
	if err := db.Model(&entry).Update("balance_after", 0).Error; err == nil {
		t.Fatal("ledger update allowed")
	}
	if err := db.Delete(&entry).Error; err == nil {
		t.Fatal("ledger delete allowed")
	}

	var pkg models.TopupPackage
	if err := db.Where("credits = ? AND version = ?", 100000, 1).First(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&pkg).Update("amount_minor", 1).Error; err == nil {
		t.Fatal("topup package price mutation allowed")
	}
	if err := db.Create(&models.TopupPackage{Provider: pkg.Provider, Currency: pkg.Currency, Credits: pkg.Credits, AmountMinor: pkg.AmountMinor + 1, Version: 2, IsActive: true}).Error; err == nil {
		t.Fatal("multiple active package versions allowed")
	}
}
