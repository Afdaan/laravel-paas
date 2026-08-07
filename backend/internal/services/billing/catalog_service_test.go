package billing

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestCatalogServiceListsBoundedAdminBillingCollections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Invoice{}, &models.Topup{}); err != nil {
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
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing user"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Wallet{UserID: user.ID, BalanceCredits: 20}).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Starter", Slug: "starter", CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 75, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: 1, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}).Error; err != nil {
		t.Fatal(err)
	}

	overview, err := NewCatalogService(db).GetOwnBillingOverview(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.UpcomingRequiredCredits != spec.MonthlyCredits {
		t.Fatalf("upcoming required credits = %d, want %d", overview.UpcomingRequiredCredits, spec.MonthlyCredits)
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

func TestCatalogHidesUnenforcedDatabaseStorageTier(t *testing.T) {
	databaseSpec := models.BillableSpec{ID: 1, Type: models.BillableTypeDatabase, Name: "Database", Slug: "database", CPUMillicores: 500, MemoryMB: 512, StorageGB: 100, MonthlyCredits: 100, Version: 1, IsActive: true}
	projectSpec := models.BillableSpec{ID: 2, Type: models.BillableTypeProject, Name: "Project", Slug: "project", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}

	catalog := catalogFromModels([]models.BillableSpec{databaseSpec, projectSpec}, nil)
	if catalog.Specs[0].StorageGB != 0 || catalog.Specs[1].StorageGB != projectSpec.StorageGB {
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
