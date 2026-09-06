package services

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestDatabaseCredentialRotationFinalizesDurableTask(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DatabaseInstance{}, &models.DatabaseCredentialRotationTask{}, &models.ProjectEnvSyncTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Rotation", Subdomain: "rotation"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: project.UserID, ProjectID: &project.ID, Engine: "mysql", Name: "rotation_db", Username: "rotation_user", Password: "old-password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	service := newDatabaseCredentialRotationService(db, func(engine, username, password string) error {
		calls++
		if engine != instance.Engine || username != instance.Username || password == "" || password == instance.Password {
			t.Fatalf("apply input=%q/%q/%q", engine, username, password)
		}
		return nil
	})
	_, updated, generation, err := service.StartOrResume(context.Background(), project.ID, instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || updated.Password == "old-password" || generation == 0 {
		t.Fatalf("calls=%d updated=%#v generation=%d", calls, updated, generation)
	}
	var taskCount int64
	if err := db.Model(&models.DatabaseCredentialRotationTask{}).Count(&taskCount).Error; err != nil || taskCount != 0 {
		t.Fatalf("rotation_tasks=%d err=%v", taskCount, err)
	}
}
