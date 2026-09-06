//go:build integration

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestInvoiceServicePostgresConcurrentMonthlyChargeDebitsOnce(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(900000000 + time.Now().UTC().UnixNano()%99999999)
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Postgres Invoice", Slug: fmt.Sprintf("postgres-invoice-%d", credits), CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: credits, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	wallets := NewWalletService(db)
	if _, err := wallets.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: credits * 2, IdempotencyKey: fmt.Sprintf("invoice-postgres-funds-%d", credits)}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Postgres Invoice", GithubURL: "https://github.com/example/invoice", Subdomain: fmt.Sprintf("postgres-invoice-%d", credits), Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	service := NewInvoiceService(db, wallets)
	initial := time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC)
	if err := db.Transaction(func(tx *gorm.DB) error {
		return service.ChargeInitialResourceTx(tx, user.ID, models.BillableTypeProject, project.ID, spec.ID, initial)
	}); err != nil {
		t.Fatal(err)
	}
	dueAt := nextBillingCycle(initial)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error { return service.RunMonthly(context.Background(), dueAt) },
		func() error { return service.RunMonthly(context.Background(), dueAt) },
	}) {
		if err != nil {
			t.Fatalf("concurrent monthly charge: %v", err)
		}
	}
	assertInvoiceWallet(t, db, user.ID, 0, 3)
	var resource models.BillableResource
	if err := db.Where("type = ? AND resource_id = ?", models.BillableTypeProject, project.ID).First(&resource).Error; err != nil {
		t.Fatal(err)
	}
	if !resource.NextInvoiceAt.Equal(nextBillingCycle(dueAt)) {
		t.Fatalf("resource=%#v", resource)
	}
}

func TestInvoiceSchedulerPostgresRecoversSuspendedCurrentPeriodAfterMissedWebhookRetry(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	unique := time.Now().UTC().UnixNano()
	wallets := NewWalletService(db)
	if _, err := wallets.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: fmt.Sprintf("scheduler-recovery-funds-%d", unique)}); err != nil {
		t.Fatal(err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Scheduler Recovery", Slug: fmt.Sprintf("scheduler-recovery-%d", unique), CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Scheduler Recovery", GithubURL: "https://github.com/example/scheduler-recovery", Subdomain: fmt.Sprintf("scheduler-recovery-%d", unique), Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusSuspended, CurrentPeriodStart: now.AddDate(0, -1, 0), NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	dueAt := now.Add(-24 * time.Hour)
	evidence := models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: resource.CurrentPeriodStart, PeriodEnd: resource.NextInvoiceAt, TotalCredits: 0, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: fmt.Sprintf("scheduler-recovery-evidence-%d", unique), DueAt: &dueAt}
	if err := db.Create(&evidence).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: evidence.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "Chargeback recovery", Credits: 0}).Error; err != nil {
		t.Fatalf("create chargeback evidence item: %T %#v", err, err)
	}
	stopAttemptedAt := now.Add(-time.Minute)
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: user.ID, MainContainerID: "main-suspended", MainWasRunning: true, StopAttemptedAt: &stopAttemptedAt}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	NewInvoiceScheduler(NewInvoiceService(db, wallets)).runOnce(context.Background())

	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("resource=%#v", resource)
	}
	if err := db.First(&evidence, evidence.ID).Error; err != nil {
		t.Fatal(err)
	}
	if evidence.Status != models.InvoiceStatusPaid {
		t.Fatalf("evidence=%#v", evidence)
	}
	if err := db.First(&task, task.ID).Error; err != nil || task.ResumeRequestedAt == nil || task.ResumeCompletedAt != nil {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}

func TestSuspensionServicePostgresConcurrentCreatesOnePhysicalIntent(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	unique := time.Now().UTC().UnixNano()
	spec := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Postgres Suspension", Slug: fmt.Sprintf("postgres-suspension-%d", unique), CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: user.ID, Engine: "postgresql", Name: fmt.Sprintf("suspension_%d", unique), Username: fmt.Sprintf("suspension_u_%d", unique), Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	wallets := NewWalletService(db)
	if _, err := wallets.Credit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryTopup, AmountCredits: 1, IdempotencyKey: fmt.Sprintf("postgres-suspension-wallet-%d", unique)}); err != nil {
		t.Fatal(err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	dueAt := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeDatabase, ResourceID: instance.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusPaymentDue, CurrentPeriodStart: dueAt, NextInvoiceAt: dueAt.AddDate(0, 1, 0), BillingAnchorDay: dueAt.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	invoice := models.Invoice{UserID: user.ID, WalletID: wallet.ID, PeriodStart: dueAt, PeriodEnd: dueAt.AddDate(0, 1, 0), TotalCredits: 100, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: fmt.Sprintf("postgres-suspension-invoice-%d", unique), DueAt: &dueAt}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "Database", Credits: 100}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewSuspensionService(db, &config.Config{BillingEnabled: true, BillingGraceDays: 7})
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			return service.suspendResource(context.Background(), resource.ID, dueAt.AddDate(0, 0, 7))
		},
		func() error {
			return service.suspendResource(context.Background(), resource.ID, dueAt.AddDate(0, 0, 7))
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent suspension: %v", err)
		}
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("resource=%#v", resource)
	}
	var taskCount int64
	if err := db.Model(&models.DatabaseStatusOperationTask{}).Where("database_instance_id = ?", instance.ID).Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("database suspension task count=%d", taskCount)
	}
}
