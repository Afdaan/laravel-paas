package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidMutation     = errors.New("invalid wallet mutation")
	ErrIdempotencyConflict = errors.New("wallet idempotency key conflict")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
	ErrBalanceOverflow     = errors.New("wallet balance overflow")
	ErrServiceUnavailable  = errors.New("wallet service unavailable")
)

// WalletService owns all wallet balance mutations and immutable ledger writes.
type WalletService struct {
	db *gorm.DB
}

type LedgerMutation struct {
	UserID        uint
	EntryType     models.WalletLedgerEntryType
	AmountCredits int64
	// IdempotencyKey is globally unique across all ledger mutations. Format:
	// <domain>:<stable-resource-id>:<operation>. Future provider and invoice
	// callers must use this canonical namespace before invoking WalletService.
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	CreatedBy      *uint
}

type MutationResult struct {
	WalletID     uint
	BalanceAfter int64
	LedgerEntry  models.WalletLedgerEntry
	Replayed     bool
}

func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{db: db}
}

// GetOrCreateWallet lazily creates the wallet owned by userID.
func (s *WalletService) GetOrCreateWallet(ctx context.Context, userID uint) (models.Wallet, error) {
	if err := s.validateContext(ctx); err != nil {
		return models.Wallet{}, err
	}
	if userID == 0 {
		return models.Wallet{}, fmt.Errorf("%w: user ID is required", ErrInvalidMutation)
	}

	var wallet models.Wallet
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(&models.Wallet{UserID: userID}).Error; err != nil {
			return fmt.Errorf("create wallet: %w", err)
		}
		if err := tx.Where("user_id = ?", userID).First(&wallet).Error; err != nil {
			return fmt.Errorf("load wallet: %w", err)
		}
		return nil
	})
	if err != nil {
		return models.Wallet{}, err
	}
	return wallet, nil
}

// Credit records a positive immutable ledger entry and increases the wallet balance.
func (s *WalletService) Credit(ctx context.Context, mutation LedgerMutation) (MutationResult, error) {
	return s.apply(ctx, mutation, true)
}

// Debit records a negative immutable ledger entry and decreases the wallet balance.
// Invoice debits and adjustments cannot overdraw. Top-up reversals may create debt.
func (s *WalletService) Debit(ctx context.Context, mutation LedgerMutation) (MutationResult, error) {
	return s.apply(ctx, mutation, false)
}

func (s *WalletService) apply(ctx context.Context, mutation LedgerMutation, credit bool) (MutationResult, error) {
	if err := s.validateContext(ctx); err != nil {
		return MutationResult{}, err
	}
	if err := validateMutation(mutation, credit); err != nil {
		return MutationResult{}, err
	}

	var result MutationResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var applyErr error
		result, applyErr = s.applyInTransaction(tx, mutation, credit)
		return applyErr
	})
	if err == nil {
		return result, nil
	}

	if errors.Is(err, ErrInsufficientBalance) || errors.Is(err, ErrInvalidMutation) || errors.Is(err, ErrIdempotencyConflict) || errors.Is(err, ErrBalanceOverflow) {
		return MutationResult{}, err
	}

	existing, lookupErr := findMutationByKey(s.db.WithContext(ctx), mutation, credit)
	if lookupErr == nil && existing != nil {
		return *existing, nil
	}
	if errors.Is(lookupErr, ErrIdempotencyConflict) {
		return MutationResult{}, lookupErr
	}
	return MutationResult{}, err
}

func (s *WalletService) applyInTransaction(tx *gorm.DB, mutation LedgerMutation, credit bool) (MutationResult, error) {
	if s == nil || s.db == nil || tx == nil {
		return MutationResult{}, ErrServiceUnavailable
	}
	if err := validateMutation(mutation, credit); err != nil {
		return MutationResult{}, err
	}

	wallet, err := getOrCreateLockedWallet(tx, mutation.UserID)
	if err != nil {
		return MutationResult{}, err
	}

	existing, err := findMutationByKey(tx, mutation, credit)
	if err != nil {
		return MutationResult{}, err
	}
	if existing != nil {
		return *existing, nil
	}

	signedAmount := mutation.AmountCredits
	if !credit {
		signedAmount = -signedAmount
		if mutation.EntryType != models.WalletLedgerEntryTopupReversal && wallet.BalanceCredits < mutation.AmountCredits {
			return MutationResult{}, ErrInsufficientBalance
		}
	}
	balanceAfter, err := addCredits(wallet.BalanceCredits, signedAmount)
	if err != nil {
		return MutationResult{}, err
	}

	entry := models.WalletLedgerEntry{
		WalletID:       wallet.ID,
		Type:           mutation.EntryType,
		AmountCredits:  signedAmount,
		BalanceAfter:   balanceAfter,
		IdempotencyKey: mutation.IdempotencyKey,
		CreatedBy:      mutation.CreatedBy,
	}
	if mutation.ReferenceType != "" {
		entry.ReferenceType = &mutation.ReferenceType
		entry.ReferenceID = &mutation.ReferenceID
	}
	if err := tx.Create(&entry).Error; err != nil {
		return MutationResult{}, fmt.Errorf("create wallet ledger entry: %w", err)
	}
	if err := tx.Model(&wallet).Update("balance_credits", balanceAfter).Error; err != nil {
		return MutationResult{}, fmt.Errorf("update wallet balance: %w", err)
	}

	return MutationResult{
		WalletID:     wallet.ID,
		BalanceAfter: balanceAfter,
		LedgerEntry:  entry,
	}, nil
}

func (s *WalletService) validateContext(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrServiceUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is required", ErrInvalidMutation)
	}
	return nil
}

func getOrCreateLockedWallet(tx *gorm.DB, userID uint) (models.Wallet, error) {
	if err := tx.Session(&gorm.Session{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoNothing: true,
	}).Create(&models.Wallet{UserID: userID}).Error; err != nil {
		return models.Wallet{}, fmt.Errorf("create wallet: %w", err)
	}

	var wallet models.Wallet
	if err := tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&wallet).Error; err != nil {
		return models.Wallet{}, fmt.Errorf("lock wallet: %w", err)
	}
	return wallet, nil
}

func findMutationByKey(db *gorm.DB, mutation LedgerMutation, credit bool) (*MutationResult, error) {
	var entry models.WalletLedgerEntry
	err := db.Session(&gorm.Session{}).Model(&models.WalletLedgerEntry{}).Where("idempotency_key = ?", mutation.IdempotencyKey).First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find wallet ledger entry: %w", err)
	}

	var wallet models.Wallet
	if err := db.Session(&gorm.Session{}).Model(&models.Wallet{}).Where("id = ?", entry.WalletID).First(&wallet).Error; err != nil {
		return nil, fmt.Errorf("load wallet for existing ledger entry: %w", err)
	}
	if !sameMutation(wallet, entry, mutation, credit) {
		return nil, ErrIdempotencyConflict
	}
	return &MutationResult{
		WalletID:     wallet.ID,
		BalanceAfter: entry.BalanceAfter,
		LedgerEntry:  entry,
		Replayed:     true,
	}, nil
}

func sameMutation(wallet models.Wallet, entry models.WalletLedgerEntry, mutation LedgerMutation, credit bool) bool {
	expectedAmount := mutation.AmountCredits
	if !credit {
		expectedAmount = -expectedAmount
	}
	return wallet.UserID == mutation.UserID &&
		entry.Type == mutation.EntryType &&
		entry.AmountCredits == expectedAmount &&
		stringValue(entry.ReferenceType) == mutation.ReferenceType &&
		stringValue(entry.ReferenceID) == mutation.ReferenceID &&
		uintValue(entry.CreatedBy) == uintValue(mutation.CreatedBy)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func uintValue(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func validateMutation(mutation LedgerMutation, credit bool) error {
	if mutation.UserID == 0 {
		return fmt.Errorf("%w: user ID is required", ErrInvalidMutation)
	}
	if mutation.AmountCredits <= 0 {
		return fmt.Errorf("%w: amount must be positive", ErrInvalidMutation)
	}
	if mutation.IdempotencyKey == "" || len(mutation.IdempotencyKey) > 255 || strings.TrimSpace(mutation.IdempotencyKey) != mutation.IdempotencyKey {
		return fmt.Errorf("%w: idempotency key is invalid", ErrInvalidMutation)
	}
	if (mutation.ReferenceType == "") != (mutation.ReferenceID == "") || len(mutation.ReferenceType) > 50 || len(mutation.ReferenceID) > 255 {
		return fmt.Errorf("%w: reference is invalid", ErrInvalidMutation)
	}
	if mutation.CreatedBy != nil && *mutation.CreatedBy == 0 {
		return fmt.Errorf("%w: creator ID is invalid", ErrInvalidMutation)
	}

	if credit {
		switch mutation.EntryType {
		case models.WalletLedgerEntryTopup, models.WalletLedgerEntryInvoiceRefund, models.WalletLedgerEntryAdjustment:
			return nil
		}
	} else {
		switch mutation.EntryType {
		case models.WalletLedgerEntryTopupReversal, models.WalletLedgerEntryInvoiceDebit, models.WalletLedgerEntryAdjustment:
			return nil
		}
	}
	return fmt.Errorf("%w: entry type %q has the wrong direction", ErrInvalidMutation, mutation.EntryType)
}

func addCredits(balance, amount int64) (int64, error) {
	if amount > 0 && balance > math.MaxInt64-amount {
		return 0, ErrBalanceOverflow
	}
	if amount < 0 && balance < math.MinInt64-amount {
		return 0, ErrBalanceOverflow
	}
	return balance + amount, nil
}
