package database

import (
	"fmt"
	"testing"
	"time"

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
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}, &models.TopupPackage{}); err != nil {
		t.Fatalf("migrate billing catalog: %v", err)
	}
	return db
}

func TestBackfillBillableResourceAnchorsUsesEarliestInvoicePeriod(t *testing.T) {
	db := billingTestDB(t, t.Name())
	resource := models.BillableResource{UserID: 1, Type: models.BillableTypeProject, ResourceID: 1, SpecID: 1, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC), NextInvoiceAt: time.Date(2025, time.March, 28, 0, 0, 0, 0, time.UTC)}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	firstStart := time.Date(2025, time.January, 29, 0, 0, 0, 0, time.UTC)
	invoice := models.Invoice{UserID: 1, WalletID: 1, PeriodStart: firstStart, PeriodEnd: firstStart.AddDate(0, 1, 0), Status: models.InvoiceStatusPaid, IdempotencyKey: "invoice-anchor-29"}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	item := models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: 1, Description: "anchor", Credits: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillBillableResourceAnchors(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingAnchorDay != 29 || resource.BillingAnchorMonthEnd {
		t.Fatalf("anchor=%d month_end=%t", resource.BillingAnchorDay, resource.BillingAnchorMonthEnd)
	}
}

func TestBackfillBillableResourceAnchorsFailsWithoutInvoiceEvidence(t *testing.T) {
	db := billingTestDB(t, t.Name())
	resource := models.BillableResource{UserID: 1, Type: models.BillableTypeProject, ResourceID: 1, SpecID: 1, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC), NextInvoiceAt: time.Date(2025, time.March, 28, 0, 0, 0, 0, time.UTC)}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillBillableResourceAnchors(db); err == nil {
		t.Fatal("missing invoice evidence allowed anchor backfill")
	}
}

func TestEnsureBillableResourceCoverageBlocksUnmappedResources(t *testing.T) {
	db := billingTestDB(t, t.Name())
	user := models.User{Email: "billing-coverage@example.test", Password: "test", Name: "Billing Coverage"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Coverage", GithubURL: "https://github.com/example/coverage", Subdomain: "coverage", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	database := models.DatabaseInstance{UserID: user.ID, Engine: "mysql", Status: models.DBStatusActive, Name: "coverage_db", Username: "coverage_user", Password: "test", Host: "mysql", Port: 3306}
	if err := db.Create(&database).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureBillableResourceCoverage(db); err == nil {
		t.Fatal("unmapped resources allowed billing activation")
	}
	periodStart := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	for _, resource := range []models.BillableResource{
		{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, CurrentPeriodStart: periodStart, NextInvoiceAt: periodStart.Add(time.Hour)},
		{UserID: user.ID, Type: models.BillableTypeDatabase, ResourceID: database.ID, SpecID: 1, CurrentPeriodStart: periodStart, NextInvoiceAt: periodStart.Add(time.Hour)},
	} {
		if err := db.Create(&resource).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := ensureBillableResourceCoverage(db); err != nil {
		t.Fatalf("mapped resources blocked billing activation: %v", err)
	}
}

func TestRetireDeletedBillableResourcesPreservesInvoiceReferences(t *testing.T) {
	db := billingTestDB(t, t.Name())
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	resources := []models.BillableResource{
		{UserID: 1, Type: models.BillableTypeProject, ResourceID: 999, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()},
		{UserID: 1, Type: models.BillableTypeDatabase, ResourceID: 998, SpecID: 1, BillingStatus: models.BillableResourceStatusPaymentDue, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()},
	}
	if err := db.Create(&resources).Error; err != nil {
		t.Fatal(err)
	}

	if err := retireDeletedBillableResources(db); err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		var persisted models.BillableResource
		if err := db.First(&persisted, resource.ID).Error; err != nil {
			t.Fatal(err)
		}
		if persisted.BillingStatus != models.BillableResourceStatusDeleted {
			t.Fatalf("resource %d billing status=%s", resource.ID, persisted.BillingStatus)
		}
	}
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
		{models.BillableTypeProject, "small", 500, 1024, 5, 0, 0, 100},
		{models.BillableTypeProject, "medium", 1000, 2048, 10, 0, 0, 200},
		{models.BillableTypeProject, "large", 2000, 4096, 20, 0, 0, 400},
		{models.BillableTypeDatabase, "small", 500, 1024, 10, 50, 7, 150},
		{models.BillableTypeDatabase, "medium", 1000, 2048, 25, 100, 14, 300},
		{models.BillableTypeDatabase, "large", 2000, 4096, 50, 200, 30, 600},
	} {
		var got models.BillableSpec
		if err := db.Where("type = ? AND slug = ? AND version = ?", want.typ, want.slug, 1).First(&got).Error; err != nil {
			t.Fatalf("find %s/%s: %v", want.typ, want.slug, err)
		}
		if got.CPUMillicores != want.cpu || got.MemoryMB != want.memory || got.StorageGB != want.storage || got.MonthlyCredits != want.credits || pointerValue(got.ConnectionLimit) != want.connections || pointerValue(got.BackupRetentionDays) != want.retention {
			t.Fatalf("%s/%s = %#v", want.typ, want.slug, got)
		}
	}

	for order, price := range topupPackagePrices {
		var got models.TopupPackage
		if err := db.Where("provider = ? AND currency = ? AND credits = ? AND version = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, price.Credits, 1).First(&got).Error; err != nil {
			t.Fatalf("find package %d: %v", price.Credits, err)
		}
		if got.AmountMinor != price.AmountMinor || !got.IsActive || got.SortOrder != order+1 {
			t.Fatalf("package %d = %#v", price.Credits, got)
		}
	}
}

// Legacy packages were seeded at 1 credit = 1 IDR. Reseeding must retire them and
// publish a corrected version rather than mutating the immutable price row.
func TestSeedBillingCatalogRepricesLegacyPackages(t *testing.T) {
	db := billingTestDB(t, t.Name())
	if err := seedTopupPackages(db); err != nil {
		t.Fatal(err)
	}

	var active []models.TopupPackage
	if err := db.Where("is_active = ?", true).Order("sort_order").Find(&active).Error; err != nil {
		t.Fatal(err)
	}
	if len(active) != len(topupPackagePrices) {
		t.Fatalf("active packages = %d, want %d", len(active), len(topupPackagePrices))
	}
	for i, price := range topupPackagePrices {
		if active[i].Credits != price.Credits || active[i].AmountMinor != price.AmountMinor {
			t.Fatalf("active[%d] = %#v, want %d credits at %d", i, active[i], price.Credits, price.AmountMinor)
		}
	}

	// A second pass must not create any new versions or overwrite existing packages.
	if err := seedTopupPackages(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.TopupPackage{}).Where("is_active = ?", true).Count(&count).Error; err != nil || count != int64(len(topupPackagePrices)) {
		t.Fatalf("reseed not idempotent: count=%d err=%v", count, err)
	}
}

func TestRepairBillingCatalogOnlyFixesKnownBadRow(t *testing.T) {
	db := billingTestDB(t, t.Name())
	bad := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Large", Slug: "large", CPUMillicores: 2000, MemoryMB: 4096, StorageGB: 50, ConnectionLimit: intPtr(600000), BackupRetentionDays: intPtr(30), MonthlyCredits: 400, Version: 1, IsActive: true}
	if err := db.Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairBillingCatalog(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&bad, bad.ID).Error; err != nil || bad.IsActive {
		t.Fatalf("known bad row not retired: %#v, %v", bad, err)
	}
	var corrected models.BillableSpec
	if err := db.Where("type = ? AND slug = ? AND version = ?", models.BillableTypeDatabase, "large", 2).First(&corrected).Error; err != nil || !corrected.IsActive || pointerValue(corrected.ConnectionLimit) != 200 || corrected.MonthlyCredits != 600 {
		t.Fatalf("corrected replacement missing: %#v, %v", corrected, err)
	}

	custom := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Custom", Slug: "custom", CPUMillicores: 1, MemoryMB: 1, StorageGB: 1, ConnectionLimit: intPtr(600000), MonthlyCredits: 400, Version: 1, IsActive: true}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairBillingCatalog(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&custom, custom.ID).Error; err != nil || pointerValue(custom.ConnectionLimit) != 600000 || custom.MonthlyCredits != 400 {
		t.Fatalf("custom row changed: %#v, %v", custom, err)
	}
}

func TestTopupPackageRepricingUsesNewVersion(t *testing.T) {
	db := billingTestDB(t, t.Name())
	old := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 100000, Version: 1, IsActive: false}
	new := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 125000, Version: 2, IsActive: true}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&new).Error; err != nil {
		t.Fatal(err)
	}
}

func TestSeedBillingCatalogPreservesAdminEditedPackages(t *testing.T) {
	db := billingTestDB(t, t.Name())
	if err := seedBillingCatalog(db); err != nil {
		t.Fatalf("initial seed failed: %v", err)
	}

	// Simulate admin editing package for 100 credits from IDR 100.000 to IDR 10.000 (new version)
	var active100 models.TopupPackage
	if err := db.Where("credits = ? AND is_active = ?", 100, true).First(&active100).Error; err != nil {
		t.Fatalf("find active 100 credit package: %v", err)
	}
	if err := db.Model(&active100).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate old package: %v", err)
	}
	adminEditedPackage := models.TopupPackage{
		Provider:    active100.Provider,
		Currency:    active100.Currency,
		Credits:     100,
		AmountMinor: 10000,
		Version:     active100.Version + 1,
		IsActive:    true,
		SortOrder:   active100.SortOrder,
	}
	if err := db.Create(&adminEditedPackage).Error; err != nil {
		t.Fatalf("create admin edited package: %v", err)
	}

	// Reseed database (simulating server restart)
	if err := seedBillingCatalog(db); err != nil {
		t.Fatalf("reseed failed: %v", err)
	}

	// Admin edited package MUST remain active and NOT be reverted back to 100.000 IDR
	var checkActive models.TopupPackage
	if err := db.Where("credits = ? AND is_active = ?", 100, true).First(&checkActive).Error; err != nil {
		t.Fatalf("find active 100 credit package after reseed: %v", err)
	}
	if checkActive.ID != adminEditedPackage.ID || checkActive.AmountMinor != 10000 {
		t.Fatalf("admin edited package was reverted: got amount_minor=%d version=%d (expected amount_minor=10000, version=%d)", checkActive.AmountMinor, checkActive.Version, adminEditedPackage.Version)
	}
}

func pointerValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
