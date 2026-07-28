package database

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func billingTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.BillableSpec{}, &models.TopupPackage{}); err != nil {
		t.Fatalf("migrate billing catalog: %v", err)
	}
	return db
}

func TestSeedBillingCatalogIsIdempotent(t *testing.T) {
	db := billingTestDB(t, t.Name())
	for range 2 {
		if err := seedBillingCatalog(db); err != nil {
			t.Fatalf("seed billing catalog: %v", err)
		}
	}

	type specWant struct {
		typ                                          models.BillableType
		slug                                         string
		cpu, memory, storage, connections, retention int
		credits                                      int64
	}
	for _, want := range []specWant{
		{models.BillableTypeProject, "small", 500, 1024, 5, 0, 0, 100000},
		{models.BillableTypeProject, "medium", 1000, 2048, 10, 0, 0, 200000},
		{models.BillableTypeProject, "large", 2000, 4096, 20, 0, 0, 400000},
		{models.BillableTypeDatabase, "small", 500, 1024, 10, 50, 7, 150000},
		{models.BillableTypeDatabase, "medium", 1000, 2048, 25, 100, 14, 300000},
		{models.BillableTypeDatabase, "large", 2000, 4096, 50, 200, 30, 600000},
	} {
		var got models.BillableSpec
		if err := db.Where("type = ? AND slug = ? AND version = ?", want.typ, want.slug, 1).First(&got).Error; err != nil {
			t.Fatalf("find %s/%s: %v", want.typ, want.slug, err)
		}
		if got.CPUMillicores != want.cpu || got.MemoryMB != want.memory || got.StorageGB != want.storage || got.MonthlyCredits != want.credits || pointerValue(got.ConnectionLimit) != want.connections || pointerValue(got.BackupRetentionDays) != want.retention {
			t.Fatalf("%s/%s = %#v", want.typ, want.slug, got)
		}
	}

	for order, credits := range []int64{100000, 250000, 500000, 1000000} {
		var got models.TopupPackage
		if err := db.Where("provider = ? AND currency = ? AND credits = ? AND version = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, credits, 1).First(&got).Error; err != nil {
			t.Fatalf("find package %d: %v", credits, err)
		}
		if got.AmountMinor != credits || !got.IsActive || got.SortOrder != order+1 {
			t.Fatalf("package %d = %#v", credits, got)
		}
	}
}

func TestRepairBillingCatalogOnlyFixesKnownBadRow(t *testing.T) {
	db := billingTestDB(t, t.Name())
	bad := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Large", Slug: "large", CPUMillicores: 2000, MemoryMB: 4096, StorageGB: 50, ConnectionLimit: intPtr(600000), BackupRetentionDays: intPtr(30), MonthlyCredits: 400000, Version: 1, IsActive: true}
	if err := db.Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairBillingCatalog(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&bad, bad.ID).Error; err != nil || pointerValue(bad.ConnectionLimit) != 200 || bad.MonthlyCredits != 600000 {
		t.Fatalf("known bad row not repaired: %#v, %v", bad, err)
	}

	custom := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Custom", Slug: "custom", CPUMillicores: 1, MemoryMB: 1, StorageGB: 1, ConnectionLimit: intPtr(600000), MonthlyCredits: 400000, Version: 1, IsActive: true}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairBillingCatalog(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&custom, custom.ID).Error; err != nil || pointerValue(custom.ConnectionLimit) != 600000 || custom.MonthlyCredits != 400000 {
		t.Fatalf("custom row changed: %#v, %v", custom, err)
	}
}

func TestTopupPackageRepricingUsesNewVersion(t *testing.T) {
	db := billingTestDB(t, t.Name())
	old := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: 100000, AmountMinor: 100000, Version: 1, IsActive: false}
	new := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: 100000, AmountMinor: 125000, Version: 2, IsActive: true}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&new).Error; err != nil {
		t.Fatal(err)
	}
}

func pointerValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
