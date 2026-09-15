package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestCreateDatabaseRequiresBillableSpecWhenBillingEnabled(t *testing.T) {
	handler := &DatabaseHandler{cfg: &config.Config{BillingEnabled: true}}
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/databases", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(1))
		return handler.CreateDatabase(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/databases", bytes.NewBufferString(`{"engine":"mysql","name":"managed_db","username":"managed_user","password":"StrongPassword123!"}`))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestDatabaseResumeRequiresActiveBillingResource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.BillableResource{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "billing_resume", Username: "billing_resume_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: instance.UserID, Type: models.BillableTypeDatabase, ResourceID: instance.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	handler := &DatabaseHandler{db: db, cfg: &config.Config{BillingEnabled: true}}
	if err := handler.requireActiveDatabaseBillingResource(context.Background(), instance.ID); err == nil {
		t.Fatal("suspended billing resource allowed database resume")
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	if err := handler.requireActiveDatabaseBillingResource(context.Background(), instance.ID); err != nil {
		t.Fatalf("active billing resource blocked database resume: %v", err)
	}
}

func TestRecordDatabaseCleanupTaskIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("drop database connection failed")
	ownership := infrastructure.ProvisioningOwnership{DatabaseCreated: true, UserCreated: true}
	if err := recordDatabaseCleanupTask(db, "mysql", "orphan_db", "orphan_user", ownership, cleanupErr); err != nil {
		t.Fatal(err)
	}
	if err := recordDatabaseCleanupTask(db, "mysql", "orphan_db", "orphan_user", ownership, cleanupErr); err != nil {
		t.Fatal(err)
	}
	var task models.DatabaseCleanupTask
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.RetryCount != 2 || task.LastError != cleanupErr.Error() || !task.DatabaseOwned || !task.UserOwned {
		t.Fatalf("task=%#v", task)
	}
}

func TestEnsureNoDatabaseCleanupTaskBlocksReprovision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: "mysql", Name: "orphan_db", Username: "orphan_user", DatabaseOwned: true, LastError: "initial"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureNoDatabaseCleanupTask(db, task.Engine, task.Name, task.Username); err == nil {
		t.Fatal("reprovision allowed while cleanup task exists")
	}
}

func TestRecordDatabaseDeletionCleanupTaskCarriesStableInstanceIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "delete_db", Username: "delete_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	if err := recordDatabaseDeletionCleanupTask(db, instance, infrastructure.ProvisioningOwnership{DatabaseCreated: true, UserCreated: true}, errors.New("engine unavailable")); err != nil {
		t.Fatal(err)
	}
	var task models.DatabaseCleanupTask
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Reason != models.DatabaseCleanupReasonRequestedDeletion || task.DatabaseInstanceID == nil || *task.DatabaseInstanceID != instance.ID || task.DatabaseInstanceUID != instance.UID {
		t.Fatalf("task=%#v", task)
	}
}

func TestCompensateStandaloneDatabaseSkipsUnownedResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	called := false
	compensateStandaloneDatabaseWithCleanup(db, "mysql", "existing_db", "existing_user", infrastructure.ProvisioningOwnership{}, func(_ string, _ string, _ string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		called = true
		return ownership, nil
	})
	if called {
		t.Fatal("cleanup called for unowned resources")
	}
	var count int64
	if err := db.Model(&models.DatabaseCleanupTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cleanup tasks=%d err=%v", count, err)
	}
}

func TestCompensateStandaloneDatabaseQueuesOwnedResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	ownership := infrastructure.ProvisioningOwnership{DatabaseCreated: true}
	compensateStandaloneDatabaseWithCleanup(db, "mysql", "orphan_db", "orphan_user", ownership, func(_ string, _ string, _ string, remaining infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		return remaining, errors.New("engine unavailable")
	})
	var task models.DatabaseCleanupTask
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if !task.DatabaseOwned || task.UserOwned {
		t.Fatalf("task ownership=%#v", task)
	}
}

func TestDatabaseInstancePersistedUsesStableIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "recovery_db", Username: "recovery_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	persisted, err := databaseInstancePersisted(db, &instance)
	if err != nil || !persisted {
		t.Fatalf("persisted=%t err=%v", persisted, err)
	}
	missing := instance
	missing.ID++
	persisted, err = databaseInstancePersisted(db, &missing)
	if err != nil || persisted {
		t.Fatalf("missing persisted=%t err=%v", persisted, err)
	}
}

func TestSuspendStandaloneDatabaseForDeletionTerminatesBilling(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.BillableResource{}, &models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "delete_db", Username: "delete_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: instance.UserID, Type: models.BillableTypeDatabase, ResourceID: instance.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: instance.CreatedAt, NextInvoiceAt: instance.CreatedAt.AddDate(0, 1, 0), BillingAnchorDay: instance.CreatedAt.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	handler := &DatabaseHandler{db: db}
	suspended, err := handler.suspendStandaloneDatabaseForDeletion(instance.UID, instance.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if suspended.Status != models.DBStatusSuspended {
		t.Fatalf("instance status=%s", suspended.Status)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusDeleted {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	var task models.DatabaseCleanupTask
	if err := db.Where("engine = ? AND name = ? AND username = ?", instance.Engine, instance.Name, instance.Username).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Reason != models.DatabaseCleanupReasonRequestedDeletion || !task.DatabaseOwned || !task.UserOwned {
		t.Fatalf("task=%#v", task)
	}
}

func TestStandaloneProvisioningIntentExistsBeforePhysicalProvisioning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	if err := createStandaloneProvisioningIntent(db, "mysql", "intent_db", "intent_user"); err != nil {
		t.Fatal(err)
	}
	var task models.DatabaseCleanupTask
	if err := db.Where("engine = ? AND name = ? AND username = ?", "mysql", "intent_db", "intent_user").First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.LeaseExpiresAt == nil || task.DatabaseOwned || task.UserOwned || task.Reason != models.DatabaseCleanupReasonProvisioning {
		t.Fatalf("task=%#v", task)
	}
	if err := completeStandaloneProvisioningIntent(db, "mysql", "intent_db", "intent_user"); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("completed provisioning intent err=%v", err)
	}
}
