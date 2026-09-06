//go:build integration

package billing

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	sharedDatabase "github.com/laravel-paas/shared/database"
	"github.com/laravel-paas/shared/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWalletServicePostgresConcurrentIdempotency(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := models.User{
		Email:    fmt.Sprintf("wallet-service-%d@example.test", time.Now().UTC().UnixNano()),
		Password: "test",
		Name:     "Wallet Service Test",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	service := NewWalletService(db)
	credit := LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  150,
		IdempotencyKey: fmt.Sprintf("wallet-concurrent-credit-%d", user.ID),
	}
	runConcurrentMutation(t, 12, func() error {
		_, err := service.Credit(context.Background(), credit)
		return err
	})

	debit := LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryInvoiceDebit,
		AmountCredits:  30,
		IdempotencyKey: fmt.Sprintf("wallet-concurrent-debit-%d", user.ID),
	}
	runConcurrentMutation(t, 12, func() error {
		_, err := service.Debit(context.Background(), debit)
		return err
	})

	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.BalanceCredits != 120 {
		t.Fatalf("wallet balance = %d, want 120", wallet.BalanceCredits)
	}

	var entries []models.WalletLedgerEntry
	if err := db.Where("wallet_id = ?", wallet.ID).Order("id ASC").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("ledger count = %d, want 2", len(entries))
	}

	ledgerByType := make(map[models.WalletLedgerEntryType]models.WalletLedgerEntry, len(entries))
	for _, entry := range entries {
		ledgerByType[entry.Type] = entry
	}
	if entry := ledgerByType[models.WalletLedgerEntryTopup]; entry.AmountCredits != 150 || entry.BalanceAfter != 150 {
		t.Fatalf("topup ledger entry = %#v", entry)
	}
	if entry := ledgerByType[models.WalletLedgerEntryInvoiceDebit]; entry.AmountCredits != -30 || entry.BalanceAfter != 120 {
		t.Fatalf("invoice debit ledger entry = %#v", entry)
	}
}

func TestWalletServicePostgresConcurrentFirstWalletCreation(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	service := NewWalletService(db)

	calls := make([]func() error, 8)
	for index := range calls {
		mutation := LedgerMutation{
			UserID:         user.ID,
			EntryType:      models.WalletLedgerEntryTopup,
			AmountCredits:  1,
			IdempotencyKey: fmt.Sprintf("wallet-first-create-%d-%d", user.ID, index),
		}
		calls[index] = func() error {
			_, err := service.Credit(context.Background(), mutation)
			return err
		}
	}
	for _, err := range runConcurrentCalls(t, calls) {
		if err != nil {
			t.Fatalf("concurrent first wallet creation: %v", err)
		}
	}

	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.BalanceCredits != int64(len(calls)) {
		t.Fatalf("wallet balance = %d, want %d", wallet.BalanceCredits, len(calls))
	}
	var ledgerCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("wallet_id = ?", wallet.ID).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != int64(len(calls)) {
		t.Fatalf("ledger count = %d, want %d", ledgerCount, len(calls))
	}
}

func TestWalletServicePostgresConcurrentGlobalIdempotencyConflict(t *testing.T) {
	db := walletPostgresTestDB(t)
	firstUser := createPostgresWalletTestUser(t, db)
	secondUser := createPostgresWalletTestUser(t, db)
	service := NewWalletService(db)
	key := fmt.Sprintf("wallet-global-race-%d", firstUser.ID)

	errs := runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.Credit(context.Background(), LedgerMutation{
				UserID: firstUser.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 100, IdempotencyKey: key,
			})
			return err
		},
		func() error {
			_, err := service.Credit(context.Background(), LedgerMutation{
				UserID: secondUser.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 200, IdempotencyKey: key,
			})
			return err
		},
	})

	successes, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("global idempotency race: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("global idempotency race successes=%d conflicts=%d", successes, conflicts)
	}

	var ledgerCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("idempotency_key = ?", key).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("global idempotency ledger count = %d, want 1", ledgerCount)
	}
}

func TestWalletServicePostgresConcurrentDebitsRejectOverdraft(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	service := NewWalletService(db)
	if _, err := service.Credit(context.Background(), LedgerMutation{
		UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 100, IdempotencyKey: fmt.Sprintf("wallet-debit-funds-%d", user.ID),
	}); err != nil {
		t.Fatalf("fund wallet: %v", err)
	}

	errs := runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.Debit(context.Background(), LedgerMutation{
				UserID: user.ID, EntryType: models.WalletLedgerEntryInvoiceDebit, AmountCredits: 75, IdempotencyKey: fmt.Sprintf("wallet-debit-a-%d", user.ID),
			})
			return err
		},
		func() error {
			_, err := service.Debit(context.Background(), LedgerMutation{
				UserID: user.ID, EntryType: models.WalletLedgerEntryInvoiceDebit, AmountCredits: 75, IdempotencyKey: fmt.Sprintf("wallet-debit-b-%d", user.ID),
			})
			return err
		},
	})

	successes, insufficient := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrInsufficientBalance):
			insufficient++
		default:
			t.Fatalf("contended debit: %v", err)
		}
	}
	if successes != 1 || insufficient != 1 {
		t.Fatalf("contended debit successes=%d insufficient=%d", successes, insufficient)
	}

	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.BalanceCredits != 25 {
		t.Fatalf("wallet balance = %d, want 25", wallet.BalanceCredits)
	}
	var debit models.WalletLedgerEntry
	if err := db.Where("wallet_id = ? AND type = ?", wallet.ID, models.WalletLedgerEntryInvoiceDebit).First(&debit).Error; err != nil {
		t.Fatal(err)
	}
	if debit.AmountCredits != -75 || debit.BalanceAfter != 25 {
		t.Fatalf("debit ledger entry = %#v", debit)
	}
}

func TestWalletServicePostgresRollsBackAfterLedgerInsertFailure(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	service := NewWalletService(db)
	initialKey := fmt.Sprintf("wallet-rollback-initial-%d", user.ID)
	failedKey := fmt.Sprintf("wallet-rollback-failed-%d", user.ID)
	if _, err := service.Credit(context.Background(), LedgerMutation{
		UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 100, IdempotencyKey: initialKey,
	}); err != nil {
		t.Fatalf("fund wallet: %v", err)
	}

	if err := db.Exec(`
		CREATE FUNCTION reject_test_wallet_balance_update() RETURNS trigger AS $fn$
		BEGIN
			RAISE EXCEPTION USING MESSAGE = $msg$reject wallet update for rollback test$msg$;
		END;
		$fn$ LANGUAGE plpgsql;
		CREATE TRIGGER trg_reject_test_wallet_balance_update
		BEFORE UPDATE ON wallets
		FOR EACH ROW EXECUTE FUNCTION reject_test_wallet_balance_update();
	`).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Exec(`
			DROP TRIGGER IF EXISTS trg_reject_test_wallet_balance_update ON wallets;
			DROP FUNCTION IF EXISTS reject_test_wallet_balance_update();
		`).Error; err != nil {
			t.Fatalf("cleanup rollback trigger: %v", err)
		}
	}()

	if _, err := service.Credit(context.Background(), LedgerMutation{
		UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 25, IdempotencyKey: failedKey,
	}); err == nil {
		t.Fatal("credit succeeded despite wallet update trigger")
	}

	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.BalanceCredits != 100 {
		t.Fatalf("wallet balance after rollback = %d, want 100", wallet.BalanceCredits)
	}
	var failedLedgerCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("idempotency_key = ?", failedKey).Count(&failedLedgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if failedLedgerCount != 0 {
		t.Fatalf("failed ledger count = %d, want 0", failedLedgerCount)
	}
}

func walletPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := walletBillingTestDSN(t)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sharedDatabase.DefensiveMigrationBootstrap(db); err != nil {
		t.Fatal(err)
	}
	if err := sharedDatabase.Seed(db, &config.Config{BaseDomain: "console.example.com", ProjectDomain: "apps.example.net"}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestValidateWalletBillingTestDSN(t *testing.T) {
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
			err := validateWalletBillingTestDSN(test.dsn)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateWalletBillingTestDSN(%q) error = %v, wantErr=%t", test.dsn, err, test.wantErr)
			}
		})
	}
}

func walletBillingTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("BILLING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BILLING_TEST_DATABASE_URL not set")
	}
	if err := validateWalletBillingTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
	return dsn
}

func validateWalletBillingTestDSN(dsn string) error {
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

func createPostgresWalletTestUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{
		Email:    fmt.Sprintf("wallet-service-%d@example.test", time.Now().UTC().UnixNano()),
		Password: "test",
		Name:     "Wallet Service Test",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func runConcurrentMutation(t *testing.T, workers int, mutation func() error) {
	t.Helper()
	calls := make([]func() error, workers)
	for index := range calls {
		calls[index] = mutation
	}
	for _, err := range runConcurrentCalls(t, calls) {
		if err != nil {
			t.Fatalf("concurrent mutation: %v", err)
		}
	}
}

func runConcurrentCalls(t *testing.T, calls []func() error) []error {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, len(calls))
	var group sync.WaitGroup
	for _, call := range calls {
		group.Add(1)
		go func(call func() error) {
			defer group.Done()
			<-start
			errs <- call()
		}(call)
	}
	close(start)
	group.Wait()
	close(errs)
	results := make([]error, 0, len(calls))
	for err := range errs {
		results = append(results, err)
	}
	return results
}
