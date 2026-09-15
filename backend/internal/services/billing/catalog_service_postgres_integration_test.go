//go:build integration

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestCatalogServicePostgresSerializesIdentityWrites(t *testing.T) {
	db := walletPostgresTestDB(t)
	service := NewCatalogService(db)
	ctx := context.Background()
	unique := time.Now().UTC().UnixNano()
	slug := fmt.Sprintf("concurrent-%d", unique)
	credits := int64(900000000 + unique%99999999)

	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.CreateBillableSpec(ctx, catalogAudit(fmt.Sprintf("spec-%d-a", unique), "Concurrent specification pricing"), BillableSpecInput{
				Type: models.BillableTypeProject, Name: "Concurrent", Slug: slug, CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100, Reason: "Concurrent specification pricing",
			})
			return err
		},
		func() error {
			_, err := service.CreateBillableSpec(ctx, catalogAudit(fmt.Sprintf("spec-%d-b", unique), "Concurrent specification pricing"), BillableSpecInput{
				Type: models.BillableTypeProject, Name: "Concurrent", Slug: slug, CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 125000, Reason: "Concurrent specification pricing",
			})
			return err
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent billable spec write: %v", err)
		}
	}

	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.CreateTopupPackage(ctx, catalogAudit(fmt.Sprintf("package-%d-a", unique), "Concurrent package pricing"), TopupPackageInput{
				Credits: credits, AmountMinor: credits, SortOrder: 1, Reason: "Concurrent package pricing",
			})
			return err
		},
		func() error {
			_, err := service.CreateTopupPackage(ctx, catalogAudit(fmt.Sprintf("package-%d-b", unique), "Concurrent package pricing"), TopupPackageInput{
				Credits: credits, AmountMinor: credits + 1, SortOrder: 1, Reason: "Concurrent package pricing",
			})
			return err
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent topup package write: %v", err)
		}
	}

	var specs []models.BillableSpec
	if err := db.Where("type = ? AND slug = ?", models.BillableTypeProject, slug).Order("version ASC").Find(&specs).Error; err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Version != 1 || specs[0].IsActive || specs[1].Version != 2 || !specs[1].IsActive {
		t.Fatalf("serialized billable spec versions = %#v", specs)
	}

	var packages []models.TopupPackage
	if err := db.Where("currency = ? AND credits = ?", models.BillingCurrencyIDR, credits).Order("version ASC").Find(&packages).Error; err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].Version != 1 || packages[0].IsActive || packages[1].Version != 2 || !packages[1].IsActive {
		t.Fatalf("serialized topup package versions = %#v", packages)
	}
}

func TestCatalogServicePostgresConcurrentInitialPaymentProviderSwitch(t *testing.T) {
	db := walletPostgresTestDB(t)
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "slug-1",
		PakasirAPIKey:       "key-1",
		MidtransServerKey:   "midtrans-server-key",
		MidtransMerchantID:  "midtrans-merchant-id",
	}
	service := NewCatalogService(db, cfg)
	ctx := context.Background()
	unique := time.Now().UTC().UnixNano()

	// Ensure clean initial state with no setting row
	if err := db.Exec("DELETE FROM settings WHERE setting_key = ?", models.SettingDefaultPaymentProvider).Error; err != nil {
		t.Fatal(err)
	}

	// Two concurrent requests attempt to set default_payment_provider to midtrans simultaneously
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			return service.UpdateDefaultPaymentProvider(ctx, catalogAudit(fmt.Sprintf("switch-%d-a", unique), "Concurrent initial switch"), UpdatePaymentProviderInput{
				Provider: "midtrans",
				Reason:   "Concurrent initial switch A",
			})
		},
		func() error {
			return service.UpdateDefaultPaymentProvider(ctx, catalogAudit(fmt.Sprintf("switch-%d-b", unique), "Concurrent initial switch"), UpdatePaymentProviderInput{
				Provider: "midtrans",
				Reason:   "Concurrent initial switch B",
			})
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent initial payment provider switch: %v", err)
		}
	}

	// Setting must be midtrans
	var setting models.Setting
	if err := db.Where("setting_key = ?", models.SettingDefaultPaymentProvider).First(&setting).Error; err != nil {
		t.Fatalf("failed to query payment provider setting: %v", err)
	}
	if setting.Value != models.BillingProviderMidtrans {
		t.Fatalf("expected midtrans, got %s", setting.Value)
	}

	// Exactly 1 audit event should exist for this unique transition
	var auditCount int64
	reqA := fmt.Sprintf("switch-%d-a", unique)
	reqB := fmt.Sprintf("switch-%d-b", unique)
	if err := db.Model(&models.BillingAuditEvent{}).Where("event = ? AND target_type = ? AND (request_id = ? OR request_id = ?)", "update_payment_provider", "setting", reqA, reqB).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly 1 audit event for concurrent initial switch to same provider, got %d", auditCount)
	}
}
