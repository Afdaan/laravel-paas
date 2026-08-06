package billinggate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestProjectRuntimeGateUsesOldestOpenInvoiceDueAt(t *testing.T) {
	db := billingGateFixture(t)
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	resource := createPaymentDueProjectResource(t, db, 101, now.AddDate(0, 0, -2))
	createPaymentDueInvoice(t, db, resource.ID, now.AddDate(0, 0, -4), now.AddDate(0, 0, -4))

	gate := NewProjectRuntimeGate(db, true, 3)
	if err := gate.Check(context.Background(), 101, now); !errors.Is(err, ErrProjectActionBlocked) {
		t.Fatalf("gate error=%v", err)
	}
}

func TestProjectRuntimeGateBoundaryAndSuspension(t *testing.T) {
	db := billingGateFixture(t)
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	resource := createPaymentDueProjectResource(t, db, 102, now)

	gate := NewProjectRuntimeGate(db, true, 3)
	if err := gate.Check(context.Background(), 102, now.AddDate(0, 0, 2)); err != nil {
		t.Fatalf("day two unexpectedly blocked: %v", err)
	}
	if err := gate.Check(context.Background(), 102, now.AddDate(0, 0, 3)); !errors.Is(err, ErrProjectActionBlocked) {
		t.Fatalf("day three error=%v", err)
	}
	if err := db.Model(&models.BillableResource{}).Where("id = ?", resource.ID).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if err := gate.Check(context.Background(), 102, now); !errors.Is(err, ErrProjectActionBlocked) {
		t.Fatalf("suspended error=%v", err)
	}
}

func billingGateFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func createPaymentDueProjectResource(t *testing.T, db *gorm.DB, projectID uint, dueAt time.Time) models.BillableResource {
	t.Helper()
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Billing Gate"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{ID: projectID, UserID: user.ID, Name: "Gate", GithubURL: "https://github.com/example/gate", Subdomain: t.Name() + "-" + time.Now().UTC().Format("150405.000000000"), Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Project", Slug: t.Name() + "-" + project.Subdomain, CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 10, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusPaymentDue, CurrentPeriodStart: dueAt, NextInvoiceAt: dueAt.AddDate(0, 1, 0), BillingAnchorDay: dueAt.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	createPaymentDueInvoice(t, db, resource.ID, dueAt, dueAt)
	return resource
}

func createPaymentDueInvoice(t *testing.T, db *gorm.DB, resourceID uint, periodStart, dueAt time.Time) {
	t.Helper()
	var resource models.BillableResource
	if err := db.First(&resource, resourceID).Error; err != nil {
		t.Fatal(err)
	}
	invoice := models.Invoice{UserID: resource.UserID, WalletID: 1, PeriodStart: periodStart, PeriodEnd: periodStart.AddDate(0, 1, 0), TotalCredits: 10, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: "invoice:" + periodStart.Format(time.RFC3339Nano), DueAt: &dueAt}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	item := models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: resource.SpecID, Description: "Project", Credits: 10}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
}
