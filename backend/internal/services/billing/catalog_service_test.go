package billing

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/setting"
	"gorm.io/gorm"
)

func TestCatalogServiceListsBoundedAdminBillingCollections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Invoice{}, &models.Topup{}, &models.BillableSpec{}, &models.BillableResource{}, &models.InvoiceItem{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	wallet := models.Wallet{UserID: user.ID, BalanceCredits: 250}
	if err := db.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: now.AddDate(0, -1, 0), PeriodEnd: now, TotalCredits: 75, Status: models.InvoiceStatusPaid, IdempotencyKey: "invoice-collection"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Topup{WalletID: wallet.ID, ClientIdempotencyKey: "topup-client-collection", Provider: models.BillingProviderMidtrans, ProviderOrderID: "topup-order-collection", AmountMinor: 50000, Currency: models.BillingCurrencyIDR, Credits: 500, Status: models.TopupStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewCatalogService(db)
	wallets, err := service.ListAdminWallets(context.Background(), 1, 25)
	if err != nil || wallets.Total != 1 || len(wallets.Data) != 1 || wallets.Data[0].UserID != user.ID || wallets.Data[0].BalanceCredits != 250 {
		t.Fatalf("wallet collection = %#v, err=%v", wallets, err)
	}
	invoices, err := service.ListAdminInvoices(context.Background(), 1, 25)
	if err != nil || invoices.Total != 1 || len(invoices.Data) != 1 || invoices.Data[0].UserID != user.ID || invoices.Data[0].TotalCredits != 75 {
		t.Fatalf("invoice collection = %#v, err=%v", invoices, err)
	}
	topups, err := service.ListAdminTopups(context.Background(), 1, 25)
	if err != nil || topups.Total != 1 || len(topups.Data) != 1 || topups.Data[0].UserID != user.ID || topups.Data[0].Credits != 500 {
		t.Fatalf("topup collection = %#v, err=%v", topups, err)
	}
	if _, err := service.ListAdminWallets(context.Background(), 0, 25); err != ErrInvalidCatalogInput {
		t.Fatalf("invalid page error = %v", err)
	}
	if _, err := service.ListAdminTopups(context.Background(), 1, 101); err != ErrInvalidCatalogInput {
		t.Fatalf("invalid limit error = %v", err)
	}
}

func TestCatalogServiceReportsUpcomingActiveResourceCharge(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Wallet{UserID: user.ID, BalanceCredits: 20}).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Active", Subdomain: "active", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "starter", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, AutoRenew: true, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.UpcomingRequiredCredits != spec.MonthlyCredits {
		t.Fatalf("upcoming required credits = %d, want %d", overview.UpcomingRequiredCredits, spec.MonthlyCredits)
	}
	if len(overview.Resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(overview.Resources))
	}
	resource := overview.Resources[0]
	if resource.ResourceID != project.ID || resource.ResourceType != models.BillableTypeProject || resource.ResourceName != project.Name {
		t.Fatalf("resource identity = %#v", resource)
	}
	if resource.SpecName != spec.Name || resource.MonthlyCredits != spec.MonthlyCredits || !resource.CurrentPeriodStart.Equal(now) || !resource.NextInvoiceAt.Equal(now.AddDate(0, 1, 0)) {
		t.Fatalf("resource billing period = %#v", resource)
	}
}

func TestGetOwnBillingOverviewExcludesDeletedResourcesFromUpcomingCredits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "starter", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: 99999, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, AutoRenew: true, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.UpcomingRequiredCredits != 0 {
		t.Fatalf("upcoming required credits = %d, want 0", overview.UpcomingRequiredCredits)
	}
	if len(overview.Resources) != 0 {
		t.Fatalf("resources = %d, want 0", len(overview.Resources))
	}
}

func TestGetOwnBillingOverviewExcludesNonAutoRenewResourcesFromUpcomingCredits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Disabled Auto Renew", Subdomain: "disabled-auto-renew", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "starter", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&resource).Update("auto_renew", false).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.UpcomingRequiredCredits != 0 {
		t.Fatalf("upcoming required credits = %d, want 0", overview.UpcomingRequiredCredits)
	}
}

func TestGetOwnBillingOverviewFiltersAndCleansOrphanedBillableResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "starter", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	// Orphaned resource: project ID 888888 does not exist in projects table
	orphaned := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: 888888, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&orphaned).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Resources) != 0 {
		t.Fatalf("expected 0 active resources in overview, got %d", len(overview.Resources))
	}

	var check models.BillableResource
	if err := db.First(&check, orphaned.ID).Error; err != nil {
		t.Fatal(err)
	}
	if check.BillingStatus != models.BillableResourceStatusDeleted {
		t.Fatalf("expected orphaned resource billing_status to be deleted, got %s", check.BillingStatus)
	}
}
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
		Type: models.BillableTypeProject, Name: "Small", Slug: "small", CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100, Reason: "Initial project pricing",
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

	firstPackage, err := service.CreateTopupPackage(ctx, catalogAudit("request-package-1", "Initial package pricing"), TopupPackageInput{Credits: 100, AmountMinor: 100000, SortOrder: 1, Reason: "Initial package pricing"})
	if err != nil {
		t.Fatal(err)
	}
	secondPackage, err := service.CreateTopupPackage(ctx, catalogAudit("request-package-2", "Package price correction"), TopupPackageInput{Credits: 100, AmountMinor: 125000, SortOrder: 1, Reason: "Package price correction"})
	if err != nil {
		t.Fatal(err)
	}
	var storedFirstPackage models.TopupPackage
	if err := db.Where("currency = ? AND credits = ? AND version = ?", models.BillingCurrencyIDR, firstPackage.Credits, 1).First(&storedFirstPackage).Error; err != nil {
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

func TestCatalogHidesUnenforcedDatabaseStorageTier(t *testing.T) {
	databaseSpec := models.BillableSpec{ID: 1, Type: models.BillableTypeDatabase, Name: "Database", Slug: "database", CPUMillicores: 500, MemoryMB: 512, StorageGB: 100, MonthlyCredits: 100, Version: 1, IsActive: true}
	projectSpec := models.BillableSpec{ID: 2, Type: models.BillableTypeProject, Name: "Project", Slug: "project", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}

	catalog := catalogFromModels([]models.BillableSpec{databaseSpec, projectSpec}, nil)
	if catalog.Specs[0].StorageGB != databaseSpec.StorageGB || catalog.Specs[1].StorageGB != projectSpec.StorageGB {
		t.Fatalf("catalog specs=%#v", catalog.Specs)
	}
}

func catalogAudit(requestID, reason string) AuditContext {
	return AuditContext{ActorUserID: 1, EffectiveUserID: 1, ActorRole: string(models.RoleSuperAdmin), SourceIP: "127.0.0.1", Reason: reason, RequestID: requestID}
}

func intPointer(value int) *int { return &value }

func TestCatalogServiceSuperadminAdjustsUserAndAdminCredits(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}

	superadmin := models.User{Email: "superadmin@example.test", Password: "test", Name: "Super Admin", Role: models.RoleSuperAdmin}
	admin := models.User{Email: "admin@example.test", Password: "test", Name: "Admin User", Role: models.RoleAdmin}
	regularUser := models.User{Email: "user@example.test", Password: "test", Name: "Regular User", Role: models.RoleUser}

	for _, u := range []*models.User{&superadmin, &admin, &regularUser} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}

	wallets := NewWalletService(db)
	service := NewCatalogServiceWithWallets(db, wallets)
	ctx := context.Background()
	audit := catalogAudit("req-adjust-1", "Manual top-up by superadmin")

	// 1. Adjust credits for regular user (lazy wallet creation)
	view1, err := service.AdjustWalletCredits(ctx, audit, regularUser.ID, "idempotency-key-user-1", WalletCreditAdjustmentInput{
		Credits: 500,
		Reason:  "Initial bonus credits for user",
	})
	if err != nil {
		t.Fatalf("failed to adjust regular user credits: %v", err)
	}
	if view1.BalanceCredits != 500 {
		t.Fatalf("user balance = %d, want 500", view1.BalanceCredits)
	}

	// 2. Adjust credits for admin user (lazy wallet creation)
	view2, err := service.AdjustWalletCredits(ctx, audit, admin.ID, "idempotency-key-admin-1", WalletCreditAdjustmentInput{
		Credits: 1000,
		Reason:  "Testing allocation for admin",
	})
	if err != nil {
		t.Fatalf("failed to adjust admin credits: %v", err)
	}
	if view2.BalanceCredits != 1000 {
		t.Fatalf("admin balance = %d, want 1000", view2.BalanceCredits)
	}

	// 3. Replaying same idempotency key returns replayed result without double-crediting
	view2Replayed, err := service.AdjustWalletCredits(ctx, audit, admin.ID, "idempotency-key-admin-1", WalletCreditAdjustmentInput{
		Credits: 1000,
		Reason:  "Testing allocation for admin",
	})
	if err != nil {
		t.Fatalf("replaying adjustment failed: %v", err)
	}
	if view2Replayed.BalanceCredits != 1000 {
		t.Fatalf("replayed admin balance = %d, want 1000", view2Replayed.BalanceCredits)
	}
}

func TestCatalogServiceAdjustmentRollsBackWhenAuditWriteFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "audit-rollback@example.test", Password: "test", Name: "Audit rollback"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_wallet_adjustment_audit BEFORE INSERT ON billing_audit_events
		WHEN NEW.event = 'wallet.credit_adjusted'
		BEGIN SELECT RAISE(ABORT, 'audit insert rejected'); END;`).Error; err != nil {
		t.Fatal(err)
	}

	service := NewCatalogServiceWithWallets(db, NewWalletService(db))
	_, err = service.AdjustWalletCredits(context.Background(), catalogAudit("req-audit-rollback", "Manual adjustment"), user.ID, "audit-rollback", WalletCreditAdjustmentInput{Credits: 500, Reason: "Manual adjustment"})
	if err == nil {
		t.Fatal("expected audit failure")
	}

	var walletCount, ledgerCount int64
	if err := db.Model(&models.Wallet{}).Where("user_id = ?", user.ID).Count(&walletCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WalletLedgerEntry{}).Count(&ledgerCount).Error; err != nil {
		t.Fatal(err)
	}
	if walletCount != 0 || ledgerCount != 0 {
		t.Fatalf("wallets=%d ledger_entries=%d", walletCount, ledgerCount)
	}
}

func TestCatalogServiceUpdatePaymentProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		BillingEnabled:       true,
		BillingTopupEnabled:  true,
		PakasirEnabled:       true,
		PakasirProjectSlug:   "slug-1",
		PakasirAPIKey:        "key-1",
		MidtransServerKey:    "midtrans-server-key",
		MidtransMerchantID:   "midtrans-merchant-id",
	}
	service := NewCatalogService(db, cfg)
	audit := catalogAudit("req-provider-1", "Switching provider to pakasir")

	// 1. Update to pakasir
	if err := service.UpdateDefaultPaymentProvider(context.Background(), audit, UpdatePaymentProviderInput{
		Provider: "pakasir",
		Reason:   "Switching provider to pakasir",
	}); err != nil {
		t.Fatalf("failed to update payment provider: %v", err)
	}

	var setting models.Setting
	if err := db.Where("setting_key = ?", models.SettingDefaultPaymentProvider).First(&setting).Error; err != nil {
		t.Fatalf("failed to find updated setting: %v", err)
	}
	if setting.Value != "pakasir" {
		t.Fatalf("setting value = %q, want pakasir", setting.Value)
	}

	var auditEvent models.BillingAuditEvent
	if err := db.Where("event = ?", "update_payment_provider").First(&auditEvent).Error; err != nil {
		t.Fatalf("audit event not written: %v", err)
	}
	if auditEvent.TargetType != "setting" {
		t.Fatalf("audit target_type = %q, want setting", auditEvent.TargetType)
	}

	// 2. Disabled pakasir rejects update
	cfg.PakasirEnabled = false
	if err := service.UpdateDefaultPaymentProvider(context.Background(), audit, UpdatePaymentProviderInput{
		Provider: "pakasir",
		Reason:   "Try switch to disabled pakasir",
	}); err == nil {
		t.Fatal("expected error when PAKASIR_ENABLED=false, got nil")
	}
}

func TestCatalogServiceUpdateDefaultPaymentProviderIndependentOfRedis(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}

	// Redis mock is configured with 0 expected calls. Any unexpected Redis command will fail the test.
	client, mock := redismock.NewClientMock()
	redisSvc := infrastructure.NewRedisServiceWithClient(client)
	settingRepo := repositories.NewSettingRepository(db)
	settingSvc := setting.NewSettingService(settingRepo, redisSvc)

	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "slug-1",
		PakasirAPIKey:       "key-1",
		MidtransServerKey:   "midtrans-server-key",
		MidtransMerchantID:  "midtrans-merchant-id",
	}

	catalogSvc := NewCatalogService(db, cfg)
	topupSvc := NewTopupService(db, NewWalletService(db), cfg, nil, nil)
	topupSvc.SetSettingService(settingSvc)

	// 1. Initial state: DB is empty, activeProvider() defaults to Pakasir
	prov, err := topupSvc.activeProvider(context.Background())
	if err != nil || prov != models.BillingProviderPakasir {
		t.Fatalf("expected active provider pakasir, got %s (err: %v)", prov, err)
	}

	// 2. Switch provider to midtrans: should succeed without invoking Redis
	audit := catalogAudit("req-provider-switch-1", "Switching to Midtrans")
	if err := catalogSvc.UpdateDefaultPaymentProvider(context.Background(), audit, UpdatePaymentProviderInput{
		Provider: "midtrans",
		Reason:   "Switching to Midtrans",
	}); err != nil {
		t.Fatalf("failed to update payment provider: %v", err)
	}

	// 3. Verify topupSvc and SettingService now route to Midtrans immediately from DB
	prov, err = topupSvc.activeProvider(context.Background())
	if err != nil || prov != models.BillingProviderMidtrans {
		t.Fatalf("expected active provider midtrans after switch, got %s (err: %v)", prov, err)
	}
	if val := settingSvc.Get(models.SettingDefaultPaymentProvider, ""); val != "midtrans" {
		t.Fatalf("expected settingService.Get to return midtrans, got %s", val)
	}

	// Ensure no Redis commands were issued
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected redis commands called: %v", err)
	}
}

func TestCatalogServiceUpdateDefaultPaymentProviderRetryIsDeterministicAndSkipsDuplicateAudit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "slug-1",
		PakasirAPIKey:       "key-1",
		MidtransServerKey:   "midtrans-server-key",
		MidtransMerchantID:  "midtrans-merchant-id",
	}

	catalogSvc := NewCatalogService(db, cfg)

	// Attempt 1: DB commits 'midtrans' and writes 1 audit row
	audit := catalogAudit("req-provider-retry-1", "Switching to Midtrans")
	if err := catalogSvc.UpdateDefaultPaymentProvider(context.Background(), audit, UpdatePaymentProviderInput{
		Provider: "midtrans",
		Reason:   "Switching to Midtrans",
	}); err != nil {
		t.Fatalf("attempt 1 failed: %v", err)
	}

	// Verify exactly 1 audit event was written
	var count1 int64
	if err := db.Model(&models.BillingAuditEvent{}).Count(&count1).Error; err != nil || count1 != 1 {
		t.Fatalf("expected count 1 after attempt 1, got %d (err: %v)", count1, err)
	}

	// Attempt 2 (Retry with same provider): Idempotent no-op
	if err := catalogSvc.UpdateDefaultPaymentProvider(context.Background(), audit, UpdatePaymentProviderInput{
		Provider: "midtrans",
		Reason:   "Switching to Midtrans",
	}); err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}

	// Verify no duplicate audit event was created on retry
	var count2 int64
	if err := db.Model(&models.BillingAuditEvent{}).Count(&count2).Error; err != nil || count2 != 1 {
		t.Fatalf("expected count 1 after retry (no duplicates), got %d (err: %v)", count2, err)
	}
}

func TestCatalogServiceInvoiceItemsSnapshotImmunity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}

	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	wallet := models.Wallet{UserID: user.ID, BalanceCredits: 100}
	if err := db.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	proj := models.Project{UserID: user.ID, Name: "Initial App Name", Subdomain: "initial-app", Status: models.StatusRunning}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter Spec", Slug: "starter-spec", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 45, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resource := models.BillableResource{
		UserID:             user.ID,
		Type:               models.BillableTypeProject,
		ResourceID:         proj.ID,
		SpecID:             spec.ID,
		BillingStatus:      models.BillableResourceStatusActive,
		AutoRenew:          true,
		CurrentPeriodStart: now,
		NextInvoiceAt:      now.AddDate(0, 1, 0),
		BillingAnchorDay:   now.Day(),
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	invoice := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		PeriodStart:    now.AddDate(0, -1, 0),
		PeriodEnd:      now,
		TotalCredits:   45,
		Status:         models.InvoiceStatusPaid,
		IdempotencyKey: "test-snapshot-inv-1",
	}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}

	item := models.InvoiceItem{
		InvoiceID:          invoice.ID,
		BillableResourceID: resource.ID,
		SpecID:             spec.ID,
		ResourceName:       "Initial App Name",
		SpecName:           "Starter Spec",
		Description:        "project resource monthly credits",
		Credits:            45,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	// 1. Verify invoice overview returns snapshotted name
	svc := NewCatalogService(db)
	overview, err := svc.GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetOwnBillingOverview failed: %v", err)
	}
	if len(overview.Invoices) != 1 || len(overview.Invoices[0].Items) != 1 {
		t.Fatalf("expected 1 invoice with 1 item, got %+v", overview.Invoices)
	}
	if overview.Invoices[0].Items[0].ResourceName != "Initial App Name" {
		t.Fatalf("expected ResourceName 'Initial App Name', got %q", overview.Invoices[0].Items[0].ResourceName)
	}
	if overview.Invoices[0].Items[0].SpecName != "Starter Spec" {
		t.Fatalf("expected SpecName 'Starter Spec', got %q", overview.Invoices[0].Items[0].SpecName)
	}

	// 2. Rename and delete the live project
	if err := db.Model(&proj).Updates(map[string]any{"name": "Renamed After Fact", "status": models.StatusDeleting}).Error; err != nil {
		t.Fatal(err)
	}

	// 3. Verify invoice overview STILL returns original snapshotted name (immutable historical record)
	overviewAfterRename, err := svc.GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetOwnBillingOverview failed after rename: %v", err)
	}
	if len(overviewAfterRename.Invoices) != 1 || len(overviewAfterRename.Invoices[0].Items) != 1 {
		t.Fatalf("expected 1 invoice with 1 item, got %+v", overviewAfterRename.Invoices)
	}
	if overviewAfterRename.Invoices[0].Items[0].ResourceName != "Initial App Name" {
		t.Fatalf("historical invoice resource name mutated! expected 'Initial App Name', got %q", overviewAfterRename.Invoices[0].Items[0].ResourceName)
	}
	if overviewAfterRename.Invoices[0].Items[0].SpecName != "Starter Spec" {
		t.Fatalf("historical invoice spec name mutated! expected 'Starter Spec', got %q", overviewAfterRename.Invoices[0].Items[0].SpecName)
	}
}

func TestCatalogServiceReportsPaymentDueInvoicePeriod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	wallet := models.Wallet{UserID: user.ID}
	if err := db.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Suspended app", Subdomain: "suspended-app", Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "payment-due-period", MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusSuspended, CurrentPeriodStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC), NextInvoiceAt: time.Date(2026, time.September, 19, 0, 0, 0, 0, time.UTC), BillingAnchorDay: 19}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	periodStart := time.Date(2026, time.August, 19, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.September, 19, 0, 0, 0, 0, time.UTC)
	invoice := models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, TotalCredits: spec.MonthlyCredits, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: "payment-due-period"}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "project resource monthly credits", Credits: spec.MonthlyCredits}).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Resources) != 1 || overview.Resources[0].PaymentDuePeriodStart == nil || overview.Resources[0].PaymentDuePeriodEnd == nil {
		t.Fatalf("payment-due resource period missing: %#v", overview.Resources)
	}
	if !overview.Resources[0].PaymentDuePeriodStart.Equal(periodStart) || !overview.Resources[0].PaymentDuePeriodEnd.Equal(periodEnd) {
		t.Fatalf("payment-due resource period=%#v", overview.Resources[0])
	}
}
