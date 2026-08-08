package billing

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestSuspensionServiceCreatesProjectStopIntentAfterGrace(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	db, resource, dueAt := fixture.db, fixture.resource, fixture.dueAt
	service := NewSuspensionService(db, &config.Config{BillingEnabled: true, BillingGraceDays: 7})

	if err := service.SuspendOverdue(context.Background(), dueAt.AddDate(0, 0, 6)); err != nil {
		t.Fatal(err)
	}
	assertBillingResourceStatus(t, db, resource.ID, models.BillableResourceStatusPaymentDue)

	if err := service.SuspendOverdue(context.Background(), dueAt.AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	assertBillingResourceStatus(t, db, resource.ID, models.BillableResourceStatusSuspended)
	var task models.ProjectSuspensionTask
	if err := db.Where("project_id = ?", fixture.projectID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.BillableResourceID != resource.ID || task.UserID != resource.UserID {
		t.Fatalf("task=%#v", task)
	}
	var persisted models.Project
	if err := db.First(&persisted, fixture.projectID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != models.StatusRunning {
		t.Fatalf("project data was changed before stop worker: %#v", persisted)
	}
}

func TestSuspensionServiceCreatesDatabaseStatusIntentAfterGrace(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeDatabase)
	db, resource, dueAt := fixture.db, fixture.resource, fixture.dueAt
	service := NewSuspensionService(db, &config.Config{BillingEnabled: true, BillingGraceDays: 7})
	if err := service.SuspendOverdue(context.Background(), dueAt.AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	assertBillingResourceStatus(t, db, resource.ID, models.BillableResourceStatusSuspended)
	var task models.DatabaseStatusOperationTask
	if err := db.Where("database_instance_id = ?", fixture.databaseID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.DesiredStatus != models.DBStatusSuspended || task.DatabaseInstanceUID != fixture.databaseUID {
		t.Fatalf("task=%#v", task)
	}
	var persisted models.DatabaseInstance
	if err := db.First(&persisted, fixture.databaseID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != models.DBStatusActive {
		t.Fatalf("physical database status changed before status worker: %#v", persisted)
	}
}

func TestSuspensionViewsExcludeDeletingProject(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	if err := fixture.db.Model(&models.Project{}).Where("id = ?", fixture.projectID).Update("status", models.StatusDeleting).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&fixture.resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}

	service := NewSuspensionService(fixture.db, &config.Config{BillingEnabled: true, BillingGraceDays: 7})
	views, err := service.SuspensionViewsForUser(context.Background(), fixture.resource.UserID, fixture.dueAt.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("deleting project was exposed as suspended: %#v", views)
	}
}

type suspensionFixtureData struct {
	db          *gorm.DB
	resource    models.BillableResource
	dueAt       time.Time
	projectID   uint
	databaseID  uint
	databaseUID string
}

func suspensionFixture(t *testing.T, resourceType models.BillableType) suspensionFixtureData {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.DatabaseInstance{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}, &models.ProjectSuspensionTask{}, &models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: t.Name() + "@example.test", Password: "test", Name: "Suspension"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	dueAt := time.Date(2026, time.July, 1, 12, 0, 0, 0, time.UTC)
	spec := models.BillableSpec{Type: resourceType, Name: "Resource", Slug: t.Name(), CPUMillicores: 500, MemoryMB: 512, StorageGB: 1, MonthlyCredits: 10, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}

	var resourceID uint
	fixture := suspensionFixtureData{db: db, dueAt: dueAt}
	if resourceType == models.BillableTypeProject {
		project := models.Project{UserID: user.ID, Name: "Suspended", GithubURL: "https://github.com/example/suspended", Subdomain: t.Name(), Status: models.StatusRunning}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}
		resourceID, fixture.projectID = project.ID, project.ID
	} else {
		database := models.DatabaseInstance{UserID: user.ID, Engine: "mysql", Name: t.Name(), Username: "user_" + t.Name(), Status: models.DBStatusActive}
		if err := db.Create(&database).Error; err != nil {
			t.Fatal(err)
		}
		resourceID, fixture.databaseID, fixture.databaseUID = database.ID, database.ID, database.UID
	}
	resource := models.BillableResource{UserID: user.ID, Type: resourceType, ResourceID: resourceID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusPaymentDue, CurrentPeriodStart: dueAt, NextInvoiceAt: dueAt.AddDate(0, 1, 0), BillingAnchorDay: dueAt.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	invoice := models.Invoice{UserID: user.ID, WalletID: 1, PeriodStart: dueAt, PeriodEnd: dueAt.AddDate(0, 1, 0), TotalCredits: 10, Status: models.InvoiceStatusPaymentDue, IdempotencyKey: "invoice:" + t.Name(), DueAt: &dueAt}
	if err := db.Create(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.InvoiceItem{InvoiceID: invoice.ID, BillableResourceID: resource.ID, SpecID: spec.ID, Description: "Resource", Credits: 10}).Error; err != nil {
		t.Fatal(err)
	}
	fixture.resource = resource
	return fixture
}

func assertBillingResourceStatus(t *testing.T, db *gorm.DB, resourceID uint, want models.BillableResourceStatus) {
	t.Helper()
	var resource models.BillableResource
	if err := db.First(&resource, resourceID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != want {
		t.Fatalf("billing_status=%q want=%q", resource.BillingStatus, want)
	}
}
