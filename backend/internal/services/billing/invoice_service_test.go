package billing

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestInvoiceServiceChargesInitialResourceAtomically(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 500, IdempotencyKey: "invoice-initial-funds"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, 101, spec.ID, now)
	}); err != nil {
		t.Fatal(err)
	}

	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeProject, 101).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if resource.CurrentPeriodStart != now || !resource.NextInvoiceAt.Equal(nextBillingCycle(now)) || resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("resource=%#v", resource)
	}
	var invoice models.Invoice
	if err := db.First(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if invoice.Status != models.InvoiceStatusPaid || invoice.TotalCredits != spec.MonthlyCredits {
		t.Fatalf("invoice=%#v", invoice)
	}
	assertInvoiceWallet(t, db, user.ID, 400, 2)
}

func TestInvoiceServiceRejectsInitialProvisioningWithoutCredits(t *testing.T) {
	db, user, service, _, spec := invoiceServiceFixture(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, 102, spec.ID, time.Now().UTC())
	})
	if err != ErrInsufficientCredits {
		t.Fatalf("charge error=%v", err)
	}
	var resources, invoices, entries int64
	if err := db.Model(&models.BillableResource{}).Count(&resources).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Invoice{}).Count(&invoices).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.WalletLedgerEntry{}).Count(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if resources != 0 || invoices != 0 || entries != 0 {
		t.Fatalf("resources=%d invoices=%d entries=%d", resources, invoices, entries)
	}
}

func TestInvoiceServiceRetriesPaymentDueAfterTopupWithoutDoubleDebit(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-retry-initial-funds"}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Retry", GithubURL: "https://github.com/example/retry", Subdomain: "invoice-retry", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	initial := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, project.ID, spec.ID, initial)
	}); err != nil {
		t.Fatal(err)
	}
	dueAt := nextBillingCycle(initial)
	if err := service.RunMonthly(context.Background(), dueAt); err != nil {
		t.Fatal(err)
	}
	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeProject, project.ID).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusPaymentDue || !resource.NextInvoiceAt.Equal(dueAt) {
		t.Fatalf("resource=%#v", resource)
	}
	var dueInvoice models.Invoice
	if err := db.Where("period_start = ?", dueAt).First(&dueInvoice).Error; err != nil {
		t.Fatal(err)
	}
	if dueInvoice.Status != models.InvoiceStatusPaymentDue {
		t.Fatalf("invoice=%#v", dueInvoice)
	}
	if dueInvoice.DueAt == nil {
		t.Fatal("payment-due invoice missing due timestamp")
	}
	firstDueAt := *dueInvoice.DueAt

	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-retry-topup"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMonthly(context.Background(), dueAt); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMonthly(context.Background(), dueAt); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive || !resource.NextInvoiceAt.Equal(nextBillingCycle(dueAt)) {
		t.Fatalf("resource=%#v", resource)
	}
	if err := db.First(&dueInvoice, dueInvoice.ID).Error; err != nil {
		t.Fatal(err)
	}
	if dueInvoice.Status != models.InvoiceStatusPaid || dueInvoice.DueAt == nil || !dueInvoice.DueAt.Equal(firstDueAt) {
		t.Fatalf("invoice=%#v", dueInvoice)
	}
	assertInvoiceWallet(t, db, user.ID, 0, 4)
}

func TestInvoiceServiceSettlesSuspendedPaymentDueWithoutStartingRuntime(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-suspended-initial-funds"}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Suspended", GithubURL: "https://github.com/example/suspended", Subdomain: "invoice-suspended", Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	initial := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, project.ID, spec.ID, initial)
	}); err != nil {
		t.Fatal(err)
	}
	dueAt := nextBillingCycle(initial)
	if err := service.RunMonthly(context.Background(), dueAt); err != nil {
		t.Fatal(err)
	}
	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeProject, project.ID).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	stopAttemptedAt := dueAt.Add(time.Minute)
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: user.ID, MainContainerID: "main-suspended", MainWasRunning: true, StopAttemptedAt: &stopAttemptedAt}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-suspended-recovery-funds"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RetryDueForUser(context.Background(), user.ID, dueAt); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive || !resource.NextInvoiceAt.Equal(nextBillingCycle(dueAt)) {
		t.Fatalf("resource=%#v", resource)
	}
	if err := db.First(&project, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if project.Status != models.StatusStopped {
		t.Fatalf("payment recovery started project: %#v", project)
	}
	if err := db.First(&task, task.ID).Error; err != nil || task.CompletedAt == nil || task.ResumeRequestedAt == nil || task.ResumeCompletedAt != nil {
		t.Fatalf("payment recovery left suspension task active: %#v err=%v", task, err)
	}
}

func TestInvoiceServiceQueuesDatabaseResumeAfterSuspendedInvoiceSettlement(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if err := db.AutoMigrate(&models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "database-resume-initial"}); err != nil {
		t.Fatal(err)
	}
	connectionLimit := 15
	database := models.DatabaseInstance{UserID: user.ID, Engine: "mysql", Name: "invoice_resume", Username: "invoice_resume_user", Password: "password", Host: "mysql", Port: 3306, ConnectionLimit: connectionLimit, Status: models.DBStatusActive}
	if err := db.Create(&database).Error; err != nil {
		t.Fatal(err)
	}
	databaseSpec := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Database", Slug: "invoice-resume-database", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: spec.MonthlyCredits, Version: 1, IsActive: true}
	if err := db.Create(&databaseSpec).Error; err != nil {
		t.Fatal(err)
	}
	initial := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeDatabase, database.ID, databaseSpec.ID, initial)
	}); err != nil {
		t.Fatal(err)
	}
	dueAt := nextBillingCycle(initial)
	if err := service.RunMonthly(context.Background(), dueAt); err != nil {
		t.Fatal(err)
	}
	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeDatabase, database.ID).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database).Update("status", models.DBStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	staleTask := models.DatabaseStatusOperationTask{DatabaseInstanceID: database.ID, DatabaseInstanceUID: database.UID, Engine: database.Engine, Name: database.Name, Username: database.Username, DesiredStatus: models.DBStatusSuspended}
	if err := db.Create(&staleTask).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: databaseSpec.MonthlyCredits, IdempotencyKey: "database-resume-recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RetryDueForUser(context.Background(), user.ID, dueAt); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	if err := db.Where("database_instance_id = ?", database.ID).First(&staleTask).Error; err != nil {
		t.Fatal(err)
	}
	if staleTask.DesiredStatus != models.DBStatusActive || staleTask.PhysicalApplied {
		t.Fatalf("resume task=%#v", staleTask)
	}
}

func TestInvoiceServiceRestoresCurrentPeriodPaymentDueAfterTopup(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: "chargeback-recovery-funds"}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Chargeback recovery", GithubURL: "https://github.com/example/chargeback-recovery", Subdomain: "chargeback-recovery", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{
		UserID:             user.ID,
		Type:               models.BillableTypeProject,
		ResourceID:         project.ID,
		SpecID:             spec.ID,
		BillingStatus:      models.BillableResourceStatusPaymentDue,
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		NextInvoiceAt:      now.AddDate(0, 1, 0),
		BillingAnchorDay:   now.Day(),
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	if err := service.RetryDueForUser(context.Background(), user.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	if !resource.NextInvoiceAt.Equal(now.AddDate(0, 1, 0)) {
		t.Fatalf("next invoice=%s", resource.NextInvoiceAt)
	}
}

func TestInvoiceServiceSchedulerRecoversSuspendedCurrentPeriodAfterMissedTopupRetry(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: "missed-topup-retry-funds"}); err != nil {
		t.Fatal(err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Missed payment recovery", GithubURL: "https://github.com/example/missed-payment-recovery", Subdomain: "missed-payment-recovery", Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{
		UserID:             user.ID,
		Type:               models.BillableTypeProject,
		ResourceID:         project.ID,
		SpecID:             spec.ID,
		BillingStatus:      models.BillableResourceStatusSuspended,
		CurrentPeriodStart: now.AddDate(0, -1, 0),
		NextInvoiceAt:      now.AddDate(0, 1, 0),
		BillingAnchorDay:   now.Day(),
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	dueAt := now.Add(-24 * time.Hour)
	evidence := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		PeriodStart:    resource.CurrentPeriodStart,
		PeriodEnd:      resource.NextInvoiceAt,
		TotalCredits:   0,
		Status:         models.InvoiceStatusPaymentDue,
		IdempotencyKey: "chargeback-evidence-missed-retry",
		DueAt:          &dueAt,
	}
	if err := db.Create(&evidence).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: evidence.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "Chargeback payment recovery", Credits: 0}).Error; err != nil {
		t.Fatal(err)
	}
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: user.ID}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := service.RunMonthly(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive || !resource.NextInvoiceAt.Equal(now.AddDate(0, 1, 0)) {
		t.Fatalf("resource=%#v", resource)
	}
	if err := db.First(&evidence, evidence.ID).Error; err != nil {
		t.Fatal(err)
	}
	if evidence.Status != models.InvoiceStatusPaid || evidence.PaidAt == nil || evidence.DueAt == nil || !evidence.DueAt.Equal(dueAt) {
		t.Fatalf("evidence=%#v", evidence)
	}
	if err := db.First(&task, task.ID).Error; err != nil || task.CompletedAt == nil {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}

func TestInvoiceServiceContinuesDueResourcesAfterOneFailure(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-continue-funds"}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	invalid := models.BillableResource{UserID: user.ID, Type: models.BillableType("invalid"), ResourceID: 999, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now.AddDate(0, -1, 0), NextInvoiceAt: now}
	if err := db.Create(&invalid).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Continue", GithubURL: "https://github.com/example/continue", Subdomain: "invoice-continue", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	valid := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now.AddDate(0, -1, 0), NextInvoiceAt: now, BillingAnchorDay: now.Day()}
	if err := db.Create(&valid).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.RunMonthly(context.Background(), now); err == nil {
		t.Fatal("scheduler succeeded despite invalid due resource")
	}
	if err := db.First(&valid, valid.ID).Error; err != nil {
		t.Fatal(err)
	}
	if valid.BillingStatus != models.BillableResourceStatusActive || !valid.NextInvoiceAt.Equal(nextBillingCycle(now)) {
		t.Fatalf("valid resource=%#v", valid)
	}
	assertInvoiceWallet(t, db, user.ID, 0, 2)
}

func TestNextBillingCyclePreservesMonthEnd(t *testing.T) {
	jan31 := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	feb28 := time.Date(2026, time.February, 28, 12, 0, 0, 0, time.UTC)
	mar31 := time.Date(2026, time.March, 31, 12, 0, 0, 0, time.UTC)
	if got := nextBillingCycle(jan31); !got.Equal(feb28) {
		t.Fatalf("January 31 next cycle=%s", got)
	}
	if got := nextBillingCycle(feb28); !got.Equal(mar31) {
		t.Fatalf("February month-end next cycle=%s", got)
	}
}

func TestNextBillingCycleRetainsOriginalAnchorAfterFebruary(t *testing.T) {
	for name, test := range map[string]struct {
		start, feb, march time.Time
		anchorDay         int
		anchorMonthEnd    bool
	}{
		"January 29":        {time.Date(2025, time.January, 29, 12, 0, 0, 0, time.UTC), time.Date(2025, time.February, 28, 12, 0, 0, 0, time.UTC), time.Date(2025, time.March, 29, 12, 0, 0, 0, time.UTC), 29, false},
		"Leap January 30":   {time.Date(2024, time.January, 30, 12, 0, 0, 0, time.UTC), time.Date(2024, time.February, 29, 12, 0, 0, 0, time.UTC), time.Date(2024, time.March, 30, 12, 0, 0, 0, time.UTC), 30, false},
		"January month end": {time.Date(2025, time.January, 31, 12, 0, 0, 0, time.UTC), time.Date(2025, time.February, 28, 12, 0, 0, 0, time.UTC), time.Date(2025, time.March, 31, 12, 0, 0, 0, time.UTC), 31, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := nextBillingCycleWithAnchor(test.start, test.anchorDay, test.anchorMonthEnd); !got.Equal(test.feb) {
				t.Fatalf("first cycle=%s", got)
			}
			if got := nextBillingCycleWithAnchor(test.feb, test.anchorDay, test.anchorMonthEnd); !got.Equal(test.march) {
				t.Fatalf("second cycle=%s", got)
			}
		})
	}
}

func invoiceServiceFixture(t *testing.T) (*gorm.DB, models.User, *InvoiceService, *WalletService, models.BillableSpec) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.DatabaseInstance{}, &models.DatabaseStatusOperationTask{}, &models.ProjectSuspensionTask{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Invoice"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Project", Slug: t.Name(), CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	walletService := NewWalletService(db)
	return db, user, NewInvoiceService(db, walletService), walletService, spec
}

func TestInvoiceServiceStopsBillingDeletedResource(t *testing.T) {
	db, user, service, walletService, spec := invoiceServiceFixture(t)
	if _, err := walletService.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: spec.MonthlyCredits, IdempotencyKey: "invoice-deleted-funds"}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Deleted", GithubURL: "https://github.com/example/deleted", Subdomain: "invoice-deleted", Status: models.StatusDeleting}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	initial := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, project.ID, spec.ID, initial)
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.RunMonthly(context.Background(), nextBillingCycle(initial)); err != nil {
		t.Fatal(err)
	}
	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeProject, project.ID).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("resource=%#v", resource)
	}
	assertInvoiceWallet(t, db, user.ID, 0, 2)
}

func assertInvoiceWallet(t *testing.T, db *gorm.DB, userID uint, wantBalance, wantEntries int64) {
	t.Helper()
	var wallet models.Wallet
	if err := db.Where("user_id = ?", userID).First(&wallet).Error; err != nil || wallet.BalanceCredits != wantBalance {
		t.Fatalf("wallet=%#v err=%v", wallet, err)
	}
	var entries int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("wallet_id = ?", wallet.ID).Count(&entries).Error; err != nil || entries != wantEntries {
		t.Fatalf("entries=%d err=%v", entries, err)
	}
}

func TestInvoiceRecoveryDoesNotReactivateDeletedProject(t *testing.T) {
	db, user, service, wallets, spec := invoiceServiceFixture(t)
	now := time.Now().UTC()
	wallet, err := wallets.GetOrCreateWallet(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: 999999, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusSuspended, CurrentPeriodStart: now.Add(-time.Hour), NextInvoiceAt: now.Add(time.Hour), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	dueAt := now.Add(-time.Minute)
	invoice := models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: now.Add(-time.Hour), PeriodEnd: now, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: "deleted-project-recovery", DueAt: &dueAt}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "Top-up reversal payment due", Credits: 0}).Error; err != nil {
		t.Fatal(err)
	}

	if err := service.RestoreCurrentPeriodResources(context.Background(), user.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
}
