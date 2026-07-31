package billing

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestCatalogServiceRepricingVersionsRowsAndAudits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableSpec{}, &models.TopupPackage{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	service := NewCatalogService(db)
	ctx := context.Background()

	firstSpec, err := service.CreateBillableSpec(ctx, catalogAudit("request-spec-1", "Initial project pricing"), BillableSpecInput{
		Type: models.BillableTypeProject, Name: "Small", Slug: "small", CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100000, Reason: "Initial project pricing",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondSpec, err := service.CreateBillableSpec(ctx, catalogAudit("request-spec-2", "Project price correction"), BillableSpecInput{
		Type: models.BillableTypeProject, Name: "Small", Slug: "small", CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 125000, Reason: "Project price correction",
	})
	if err != nil {
		t.Fatal(err)
	}
	var storedFirstSpec models.BillableSpec
	if err := db.Where("type = ? AND slug = ? AND version = ?", models.BillableTypeProject, firstSpec.Slug, 1).First(&storedFirstSpec).Error; err != nil {
		t.Fatal(err)
	}
	if storedFirstSpec.Version != 1 || storedFirstSpec.IsActive || secondSpec.MonthlyCredits != 125000 {
		t.Fatalf("unexpected spec versions: first=%#v second=%#v", storedFirstSpec, secondSpec)
	}

	firstPackage, err := service.CreateTopupPackage(ctx, catalogAudit("request-package-1", "Initial package pricing"), TopupPackageInput{Credits: 100000, AmountMinor: 100000, SortOrder: 1, Reason: "Initial package pricing"})
	if err != nil {
		t.Fatal(err)
	}
	secondPackage, err := service.CreateTopupPackage(ctx, catalogAudit("request-package-2", "Package price correction"), TopupPackageInput{Credits: 100000, AmountMinor: 125000, SortOrder: 1, Reason: "Package price correction"})
	if err != nil {
		t.Fatal(err)
	}
	var storedFirstPackage models.TopupPackage
	if err := db.Where("provider = ? AND currency = ? AND credits = ? AND version = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, firstPackage.Credits, 1).First(&storedFirstPackage).Error; err != nil {
		t.Fatal(err)
	}
	if storedFirstPackage.Version != 1 || storedFirstPackage.IsActive || secondPackage.AmountMinor != 125000 {
		t.Fatalf("unexpected package versions: first=%#v second=%#v", storedFirstPackage, secondPackage)
	}

	active, err := service.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active.Specs) != 1 || active.Specs[0].MonthlyCredits != secondSpec.MonthlyCredits || len(active.Packages) != 1 || active.Packages[0].AmountMinor != secondPackage.AmountMinor {
		t.Fatalf("active catalog = %#v", active)
	}
	admin, err := service.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(admin.Specs) != 2 || admin.Specs[0].ID == 0 || admin.Specs[0].Version != 2 || !admin.Specs[0].IsActive || len(admin.Packages) != 2 || admin.Packages[0].ID == 0 || admin.Packages[0].Version != 2 || !admin.Packages[0].IsActive {
		t.Fatalf("admin catalog lifecycle identity = %#v", admin)
	}

	var events []models.BillingAuditEvent
	if err := db.Order("id ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[1].Event != "billable_spec.repriced" || events[1].ActorRole != string(models.RoleSuperAdmin) || events[1].EffectiveUserID != 1 || events[1].SourceIP != "127.0.0.1" || events[1].Reason != "Project price correction" || events[3].Event != "topup_package.repriced" {
		t.Fatalf("unexpected audit events: %#v", events)
	}
}

func TestCatalogServiceRejectsUnsafeInput(t *testing.T) {
	service := NewCatalogService(nil)
	_, err := service.CreateBillableSpec(context.Background(), catalogAudit("request", "Unsafe request"), BillableSpecInput{})
	if err != ErrCatalogServiceUnavailable {
		t.Fatalf("nil service error = %v", err)
	}

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableSpec{}, &models.TopupPackage{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	service = NewCatalogService(db)
	_, err = service.CreateBillableSpec(context.Background(), catalogAudit("request", "Unsafe request"), BillableSpecInput{
		Type: models.BillableTypeProject, Name: "Small", Slug: "Small", CPUMillicores: 1, MemoryMB: 1, StorageGB: 1, MonthlyCredits: 1, Reason: "Unsafe request",
	})
	if err != ErrInvalidCatalogInput {
		t.Fatalf("unsafe slug error = %v", err)
	}
	for _, input := range []BillableSpecInput{
		{Type: models.BillableTypeProject, Name: "Project", Slug: "project", CPUMillicores: 1, MemoryMB: 1, StorageGB: 1, MonthlyCredits: 1, ConnectionLimit: intPointer(1), Reason: "Unsafe request"},
		{Type: models.BillableTypeDatabase, Name: "Database", Slug: "database", CPUMillicores: 1, MemoryMB: 1, StorageGB: 1, MonthlyCredits: 1, Reason: "Unsafe request"},
		{Type: models.BillableTypeProject, Name: "Oversized", Slug: "oversized", CPUMillicores: maxBillableCPUMillicores + 1, MemoryMB: 1, StorageGB: 1, MonthlyCredits: 1, Reason: "Unsafe request"},
	} {
		if _, err := service.CreateBillableSpec(context.Background(), catalogAudit("request", "Unsafe request"), input); err != ErrInvalidCatalogInput {
			t.Fatalf("invalid spec %#v error = %v", input, err)
		}
	}
	if _, err := service.CreateTopupPackage(context.Background(), catalogAudit("package", "Unsafe request"), TopupPackageInput{Credits: maxTopupPackageCredits + 1, AmountMinor: 1, Reason: "Unsafe request"}); err != ErrInvalidCatalogInput {
		t.Fatalf("oversized package error = %v", err)
	}
}

func catalogAudit(requestID, reason string) AuditContext {
	return AuditContext{ActorUserID: 1, EffectiveUserID: 1, ActorRole: string(models.RoleSuperAdmin), SourceIP: "127.0.0.1", Reason: reason, RequestID: requestID}
}

func intPointer(value int) *int { return &value }
