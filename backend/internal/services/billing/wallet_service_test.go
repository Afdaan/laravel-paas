package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestWalletServiceCreditDebitReplay(t *testing.T) {
	db := walletTestDB(t)
	user := createWalletTestUser(t, db)
	service := NewWalletService(db)
	ctx := context.Background()

	credit := LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  100,
		IdempotencyKey: "wallet-credit-1",
		ReferenceType:  "topup",
		ReferenceID:    "topup-1",
	}
	firstCredit, err := service.Credit(ctx, credit)
	if err != nil {
		t.Fatalf("credit: %v", err)
	}
	if firstCredit.Replayed || firstCredit.BalanceAfter != 100 || firstCredit.LedgerEntry.AmountCredits != 100 {
		t.Fatalf("unexpected first credit: %#v", firstCredit)
	}

	replayedCredit, err := service.Credit(ctx, credit)
	if err != nil {
		t.Fatalf("replay credit: %v", err)
	}
	if !replayedCredit.Replayed || replayedCredit.LedgerEntry.ID != firstCredit.LedgerEntry.ID || replayedCredit.BalanceAfter != 100 {
		t.Fatalf("unexpected replay credit: %#v", replayedCredit)
	}

	if _, err := service.Credit(ctx, LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  50,
		IdempotencyKey: "wallet-credit-2",
	}); err != nil {
		t.Fatalf("second credit: %v", err)
	}
	if wallet, err := service.GetOrCreateWallet(ctx, user.ID); err != nil || wallet.BalanceCredits != 150 {
		t.Fatalf("current wallet after second credit = %#v, %v", wallet, err)
	}

	replayedCredit, err = service.Credit(ctx, credit)
	if err != nil {
		t.Fatalf("replay credit after later mutation: %v", err)
	}
	if !replayedCredit.Replayed || replayedCredit.LedgerEntry.ID != firstCredit.LedgerEntry.ID || replayedCredit.BalanceAfter != 100 {
		t.Fatalf("unexpected replay after later mutation: %#v", replayedCredit)
	}

	debit := LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryInvoiceDebit,
		AmountCredits:  40,
		IdempotencyKey: "wallet-debit-1",
		ReferenceType:  "invoice",
		ReferenceID:    "invoice-1",
	}
	firstDebit, err := service.Debit(ctx, debit)
	if err != nil {
		t.Fatalf("debit: %v", err)
	}
	if firstDebit.Replayed || firstDebit.BalanceAfter != 110 || firstDebit.LedgerEntry.AmountCredits != -40 {
		t.Fatalf("unexpected first debit: %#v", firstDebit)
	}

	replayedDebit, err := service.Debit(ctx, debit)
	if err != nil {
		t.Fatalf("replay debit: %v", err)
	}
	if !replayedDebit.Replayed || replayedDebit.LedgerEntry.ID != firstDebit.LedgerEntry.ID || replayedDebit.BalanceAfter != 110 {
		t.Fatalf("unexpected replay debit: %#v", replayedDebit)
	}

	var ledgerCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 3 {
		t.Fatalf("ledger count = %d, want 3", ledgerCount)
	}
}

func TestWalletServiceRejectsInsufficientDebitAndOverflow(t *testing.T) {
	db := walletTestDB(t)
	user := createWalletTestUser(t, db)
	service := NewWalletService(db)
	ctx := context.Background()

	_, err := service.Debit(ctx, LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryInvoiceDebit,
		AmountCredits:  1,
		IdempotencyKey: "wallet-insufficient",
	})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("insufficient debit error = %v", err)
	}

	var ledgerCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 0 {
		t.Fatalf("ledger count after rejected debit = %d", ledgerCount)
	}
	var walletCount int64
	if err := db.Model(&models.Wallet{}).Where("user_id = ?", user.ID).Count(&walletCount).Error; err != nil {
		t.Fatal(err)
	}
	if walletCount != 0 {
		t.Fatalf("wallet count after rejected first debit = %d", walletCount)
	}

	if _, err := service.GetOrCreateWallet(ctx, user.ID); err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	update := db.Model(&models.Wallet{}).Where("user_id = ?", user.ID).Update("balance_credits", math.MaxInt64)
	if update.Error != nil {
		t.Fatal(update.Error)
	}
	if update.RowsAffected != 1 {
		t.Fatalf("updated wallets = %d, want 1", update.RowsAffected)
	}
	_, err = service.Credit(ctx, LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  1,
		IdempotencyKey: "wallet-overflow",
	})
	if !errors.Is(err, ErrBalanceOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
	if err := db.Model(&models.WalletLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 0 {
		t.Fatalf("ledger count after rejected overflow = %d", ledgerCount)
	}
}

func TestWalletServiceRejectsAdjustmentOverdraft(t *testing.T) {
	db := walletTestDB(t)
	user := createWalletTestUser(t, db)
	service := NewWalletService(db)

	_, err := service.Debit(context.Background(), LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryAdjustment,
		AmountCredits:  1,
		IdempotencyKey: "wallet-adjustment-overdraft",
	})
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("adjustment overdraft error = %v", err)
	}
}

func TestWalletServiceAllowsTopupReversalDebt(t *testing.T) {
	db := walletTestDB(t)
	user := createWalletTestUser(t, db)
	service := NewWalletService(db)

	result, err := service.Debit(context.Background(), LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopupReversal,
		AmountCredits:  25,
		IdempotencyKey: "wallet-topup-reversal",
	})
	if err != nil {
		t.Fatalf("topup reversal: %v", err)
	}
	if result.BalanceAfter != -25 || result.LedgerEntry.AmountCredits != -25 {
		t.Fatalf("unexpected reversal result: %#v", result)
	}
}

func TestWalletServiceValidatesMutation(t *testing.T) {
	db := walletTestDB(t)
	user := createWalletTestUser(t, db)
	service := NewWalletService(db)

	for name, mutation := range map[string]LedgerMutation{
		"zero amount": {
			UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, IdempotencyKey: "wallet-zero",
		},
		"wrong direction": {
			UserID: user.ID, EntryType: models.WalletLedgerEntryInvoiceDebit, AmountCredits: 1, IdempotencyKey: "wallet-direction",
		},
		"blank key": {
			UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: " wallet-key ",
		},
		"partial reference": {
			UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: "wallet-reference", ReferenceType: "topup",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Credit(context.Background(), mutation)
			if !errors.Is(err, ErrInvalidMutation) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestWalletServiceRejectsIdempotencyConflict(t *testing.T) {
	db := walletTestDB(t)
	firstUser := createWalletTestUser(t, db)
	secondUser := createWalletTestUser(t, db)
	service := NewWalletService(db)
	ctx := context.Background()

	mutation := LedgerMutation{
		UserID:         firstUser.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  100,
		IdempotencyKey: "wallet-global-key",
	}
	if _, err := service.Credit(ctx, mutation); err != nil {
		t.Fatalf("first credit: %v", err)
	}

	mutation.AmountCredits = 200
	if _, err := service.Credit(ctx, mutation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed mutation error = %v", err)
	}

	mutation.UserID = secondUser.ID
	mutation.AmountCredits = 100
	if _, err := service.Credit(ctx, mutation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("cross-user mutation error = %v", err)
	}
}

func walletTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}); err != nil {
		t.Fatalf("migrate wallet models: %v", err)
	}
	return db
}

func createWalletTestUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{
		Email:    fmt.Sprintf("wallet-%d@example.test", time.Now().UnixNano()),
		Password: "test",
		Name:     "Wallet Test",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}
