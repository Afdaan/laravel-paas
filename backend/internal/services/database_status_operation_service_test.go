package services

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestDatabaseStatusOperationRetriesWithoutMetadataSplit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "status_db", Username: "status_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	failTransition := true
	service := newDatabaseStatusOperationService(db, func(models.DatabaseInstance, models.DatabaseInstanceStatus) error {
		if failTransition {
			return errors.New("engine unavailable")
		}
		return nil
	})
	if _, err := service.Request(context.Background(), instance.ID, models.DBStatusSuspended); err == nil {
		t.Fatal("expected physical transition failure")
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusActive {
		t.Fatalf("metadata advanced before physical success: %s", instance.Status)
	}
	failTransition = false
	if _, err := service.Request(context.Background(), instance.ID, models.DBStatusSuspended); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusSuspended {
		t.Fatalf("status=%s", instance.Status)
	}
	var taskCount int64
	if err := db.Model(&models.DatabaseStatusOperationTask{}).Count(&taskCount).Error; err != nil || taskCount != 0 {
		t.Fatalf("task_count=%d err=%v", taskCount, err)
	}
}

func TestDatabaseStatusOperationFinalizesCheckpointedPhysicalTransition(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "checkpoint_db", Username: "checkpoint_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseStatusOperationTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, DesiredStatus: models.DBStatusSuspended, PhysicalApplied: true}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseStatusOperationService(db, func(models.DatabaseInstance, models.DatabaseInstanceStatus) error {
		t.Fatal("checkpointed transition was applied twice")
		return nil
	})
	if _, err := service.processTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusSuspended {
		t.Fatalf("status=%s", instance.Status)
	}
}

func TestDatabaseStatusOperationDoesNotFinalizeSupersededSuspend(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "superseded_db", Username: "superseded_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseStatusOperationTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, DesiredStatus: models.DBStatusSuspended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	transitions := make([]models.DatabaseInstanceStatus, 0, 2)
	service := newDatabaseStatusOperationService(db, func(_ models.DatabaseInstance, desired models.DatabaseInstanceStatus) error {
		transitions = append(transitions, desired)
		if desired == models.DBStatusSuspended {
			return db.Model(&models.DatabaseStatusOperationTask{}).Where("id = ?", task.ID).Updates(map[string]any{"desired_status": models.DBStatusActive, "physical_applied": false}).Error
		}
		return nil
	})
	if _, err := service.processTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusActive {
		t.Fatalf("stale suspend finalized status=%s", instance.Status)
	}
	var taskCount int64
	if err := db.Model(&models.DatabaseStatusOperationTask{}).Count(&taskCount).Error; err != nil || taskCount != 0 {
		t.Fatalf("task_count=%d err=%v", taskCount, err)
	}
	if len(transitions) != 2 || transitions[0] != models.DBStatusSuspended || transitions[1] != models.DBStatusActive {
		t.Fatalf("transitions=%v", transitions)
	}
}

func TestDatabaseStatusOperationDiscardsDeletedInstanceTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseStatusOperationTask{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "deleted_db", Username: "deleted_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusDeleted}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	task := models.DatabaseStatusOperationTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, DesiredStatus: models.DBStatusSuspended}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseStatusOperationService(db, func(models.DatabaseInstance, models.DatabaseInstanceStatus) error {
		t.Fatal("stale deleted-instance task reached physical transition")
		return nil
	})
	if _, err := service.processTask(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.DatabaseStatusOperationTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("task_count=%d err=%v", count, err)
	}
}
