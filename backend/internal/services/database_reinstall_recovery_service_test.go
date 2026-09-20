package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestDatabaseReinstallRecoveryRetriesSuspendedTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DatabaseInstance{}, &models.BillableResource{}, &models.DatabaseReinstallRecoveryTask{}, &models.ProjectEnvSyncTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 7, Name: "Recovery project", Subdomain: "recovery-project"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 7, ProjectID: &project.ID, Engine: "mysql", Name: "recovery_db", Username: "recovery_user", Password: "old-password", Host: "mysql", Port: 3306, ConnectionLimit: 50, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: 7, Type: models.BillableTypeDatabase, ResourceID: instance.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: time.Now().UTC(), NextInvoiceAt: time.Now().UTC().AddDate(0, 1, 0), BillingAnchorDay: 1}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	failed := true
	calls := 0
	service := newDatabaseReinstallRecoveryService(db, func(got models.DatabaseInstance, password string) error {
		calls++
		if got.ID != instance.ID || password == "" {
			t.Fatalf("recreate input=%#v password=%q", got, password)
		}
		if failed {
			return errors.New("engine unavailable")
		}
		return nil
	}, func(*models.Project, uint) error { return nil })

	if _, _, err := service.StartOrResumeDatabaseReinstall(context.Background(), instance.UID, instance.UserID); err == nil {
		t.Fatal("expected initial recreate failure")
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusSuspended {
		t.Fatalf("status=%s", instance.Status)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}

	failed = false
	projectResult, instanceResult, err := service.StartOrResumeDatabaseReinstall(context.Background(), instance.UID, instance.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if projectResult.ID != project.ID || projectResult.DatabasePassword != instanceResult.Password || instanceResult.Status != models.DBStatusActive || instanceResult.Password == "old-password" || calls != 2 {
		t.Fatalf("project=%#v instance=%#v calls=%d", projectResult, instanceResult, calls)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	var task models.DatabaseReinstallRecoveryTask
	if err := db.First(&task).Error; err != nil || task.Checkpoint != models.DatabaseReinstallCheckpointEnvSyncPending || task.EnvSyncGeneration == 0 {
		t.Fatalf("reinstall task=%#v err=%v", task, err)
	}
}

func TestDatabaseReinstallRecoveryCompletesPhysicalCheckpoint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseReinstallRecoveryTask{}, &models.ProjectEnvSyncTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 8, Engine: "postgresql", Name: "checkpoint_db", Username: "checkpoint_user", Password: "old-password", Host: "postgres", Port: 5432, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseReinstallRecoveryTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, Password: "new-password", Checkpoint: models.DatabaseReinstallCheckpointPhysicalRecreated}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseReinstallRecoveryService(db, func(models.DatabaseInstance, string) error {
		t.Fatal("physical recreation repeated after checkpoint")
		return nil
	}, func(*models.Project, uint) error { return nil })
	if _, err := service.resumeTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusActive || instance.Password != "new-password" {
		t.Fatalf("instance=%#v", instance)
	}
}

func TestDatabaseReinstallRecoveryKeepsTaskUntilEnvironmentSyncAcknowledged(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DatabaseInstance{}, &models.DatabaseReinstallRecoveryTask{}, &models.ProjectEnvSyncTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 9, Name: "Env sync", Subdomain: "env-sync"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: project.UserID, ProjectID: &project.ID, Engine: "mysql", Name: "env_sync_db", Username: "env_sync_user", Password: "new-password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	envSyncTask := models.ProjectEnvSyncTask{ProjectID: project.ID, DesiredGeneration: 1}
	if err := db.Create(&envSyncTask).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseReinstallRecoveryTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, Password: instance.Password, Checkpoint: models.DatabaseReinstallCheckpointEnvSyncPending, EnvSyncGeneration: envSyncTask.DesiredGeneration}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	failSync := true
	service := newDatabaseReinstallRecoveryService(db, func(models.DatabaseInstance, string) error {
		t.Fatal("physical recreation called after env-sync checkpoint")
		return nil
	}, func(*models.Project, uint) error {
		if failSync {
			return errors.New("redis unavailable")
		}
		return nil
	})
	if _, err := service.resumeTask(context.Background(), task.ID); err == nil {
		t.Fatal("expected env sync failure")
	}
	if err := db.First(&task, task.ID).Error; err != nil || task.Checkpoint != models.DatabaseReinstallCheckpointEnvSyncPending {
		t.Fatalf("task=%#v err=%v", task, err)
	}
	failSync = false
	if _, err := service.resumeTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ProjectEnvSyncTask{}).Where("project_id = ?", project.ID).Update("acknowledged_generation", task.EnvSyncGeneration).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.resumeTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.DatabaseReinstallRecoveryTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
