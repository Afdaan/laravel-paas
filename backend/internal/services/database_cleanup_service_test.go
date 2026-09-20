package services

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestDatabaseCleanupServiceDeletesCompletedTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: "mysql", Name: "orphan_db", Username: "orphan_user", DatabaseOwned: true, LastError: "initial", RetryCount: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseCleanupService(db, func(engine, name, username string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		if engine != task.Engine || name != task.Name || username != task.Username {
			t.Fatalf("cleanup target=%s/%s/%s", engine, name, username)
		}
		if !ownership.DatabaseCreated || ownership.UserCreated {
			t.Fatalf("ownership=%#v", ownership)
		}
		return infrastructure.ProvisioningOwnership{}, nil
	})
	if err := service.processPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.DatabaseCleanupTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestDatabaseCleanupServiceRecordsRetryFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: "mysql", Name: "orphan_db", Username: "orphan_user", UserOwned: true, LastError: "initial", RetryCount: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("engine unavailable")
	service := newDatabaseCleanupService(db, func(_ string, _ string, _ string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		return ownership, cleanupErr
	})
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("cleanup failure was swallowed")
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.RetryCount != 2 || task.LastError != cleanupErr.Error() {
		t.Fatalf("task=%#v", task)
	}
}

func TestDatabaseCleanupServiceCheckpointsDatabaseBeforeUserRetry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: "postgresql", Name: "orphan_db", Username: "orphan_user", DatabaseOwned: true, UserOwned: true, LastError: "initial", RetryCount: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("drop role failed")
	service := newDatabaseCleanupService(db, func(_ string, _ string, _ string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		if ownership.DatabaseCreated {
			return infrastructure.ProvisioningOwnership{}, nil
		}
		return ownership, cleanupErr
	})
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("partial cleanup failure was swallowed")
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.DatabaseOwned || !task.UserOwned || task.RetryCount != 2 {
		t.Fatalf("task=%#v", task)
	}
}

func TestDatabaseCleanupServiceRefusesProvisioningCleanupForActiveInstance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "active_db", Username: "active_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: instance.Engine, Name: instance.Name, Username: instance.Username, Reason: models.DatabaseCleanupReasonProvisioning, DatabaseOwned: true, UserOwned: true, LastError: "stale provisioning cleanup"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseCleanupService(db, func(string, string, string, infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		t.Fatal("cleanup called for active database instance")
		return infrastructure.ProvisioningOwnership{}, nil
	})
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("expected active-instance cleanup refusal")
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !task.DatabaseOwned || !task.UserOwned {
		t.Fatalf("task ownership lost: %#v", task)
	}
}

func TestDatabaseCleanupServiceRefusesProvisioningCleanupForSuspendedInstance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "suspended_db", Username: "suspended_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseCleanupTask{Engine: instance.Engine, Name: instance.Name, Username: instance.Username, Reason: models.DatabaseCleanupReasonProvisioning, DatabaseOwned: true, LastError: "stale provisioning cleanup"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseCleanupService(db, func(string, string, string, infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		t.Fatal("cleanup called for suspended database instance")
		return infrastructure.ProvisioningOwnership{}, nil
	})
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("expected suspended-instance cleanup refusal")
	}
}

func TestDatabaseCleanupServiceGuardsDatabaseAndUserIdentitiesSeparately(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}, &models.DatabaseInstance{}); err != nil {
		t.Fatal(err)
	}
	instances := []models.DatabaseInstance{
		{UserID: 1, Engine: "mysql", Name: "reused_name", Username: "replacement_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive},
		{UserID: 1, Engine: "mysql", Name: "replacement_name", Username: "reused_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended},
	}
	if err := db.Create(&instances).Error; err != nil {
		t.Fatal(err)
	}
	tasks := []models.DatabaseCleanupTask{
		{Engine: "mysql", Name: "reused_name", Username: "old_user", Reason: models.DatabaseCleanupReasonProvisioning, DatabaseOwned: true, LastError: "stale"},
		{Engine: "mysql", Name: "old_name", Username: "reused_user", Reason: models.DatabaseCleanupReasonProvisioning, UserOwned: true, LastError: "stale"},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseCleanupService(db, func(string, string, string, infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		t.Fatal("cleanup called for reused database or user identity")
		return infrastructure.ProvisioningOwnership{}, nil
	})
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("expected identity cleanup refusals")
	}
}
