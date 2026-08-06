package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func uintPtr(value uint) *uint {
	return &value
}

func TestFinalizeDatabaseDeletionRemovesBackupsAndMarksDeleted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseBackup{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "finalize_db", Username: "finalize_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	projectsPath := t.TempDir()
	backupPath := filepath.Join(projectsPath, "database-backups", instance.UID, "finalize.sql")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := models.DatabaseBackup{DatabaseInstanceID: instance.ID, ProjectID: uintPtr(1), Name: "finalize", Path: backupPath, Status: models.BackupStatusCompleted}
	if err := db.Create(&backup).Error; err != nil {
		t.Fatal(err)
	}
	if err := FinalizeDatabaseDeletion(context.Background(), db, instance.ID, instance.UID, projectsPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup file still exists: %v", err)
	}
	var backupCount int64
	if err := db.Model(&models.DatabaseBackup{}).Where("database_instance_id = ?", instance.ID).Count(&backupCount).Error; err != nil || backupCount != 0 {
		t.Fatalf("backup count=%d err=%v", backupCount, err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusDeleted {
		t.Fatalf("status=%s", instance.Status)
	}
}

func TestFinalizeDatabaseDeletionRejectsBackupOutsideProjectsPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseBackup{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "finalize_db", Username: "finalize_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	projectsPath := t.TempDir()
	backupPath := filepath.Join(t.TempDir(), "backups", "outside.sql")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := models.DatabaseBackup{DatabaseInstanceID: instance.ID, ProjectID: uintPtr(1), Name: "outside", Path: backupPath, Status: models.BackupStatusCompleted}
	if err := db.Create(&backup).Error; err != nil {
		t.Fatal(err)
	}
	if err := FinalizeDatabaseDeletion(context.Background(), db, instance.ID, instance.UID, projectsPath); err == nil {
		t.Fatal("expected backup path validation error")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file was removed: %v", err)
	}
	var backupCount int64
	if err := db.Model(&models.DatabaseBackup{}).Where("database_instance_id = ?", instance.ID).Count(&backupCount).Error; err != nil || backupCount != 1 {
		t.Fatalf("backup count=%d err=%v", backupCount, err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != models.DBStatusSuspended {
		t.Fatalf("status=%s", instance.Status)
	}
}

func TestFinalizeDatabaseDeletionRejectsUIDMismatchBeforeBackupCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseBackup{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "uid_mismatch", Username: "uid_mismatch_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	projectsPath := t.TempDir()
	backupPath := filepath.Join(projectsPath, "user-1", "uid-mismatch", "backups", "backup.sql")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.DatabaseBackup{DatabaseInstanceID: instance.ID, ProjectID: uintPtr(1), Name: "uid-mismatch", Path: backupPath, Status: models.BackupStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	if err := FinalizeDatabaseDeletion(context.Background(), db, instance.ID, "stale-uid", projectsPath); err == nil {
		t.Fatal("expected UID mismatch")
	}
	assertDatabaseFinalizerPreservedBackup(t, db, instance, backupPath, models.DBStatusSuspended)
}

func TestFinalizeDatabaseDeletionRejectsActiveInstanceBeforeBackupCleanup(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseBackup{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "active_instance", Username: "active_instance_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusActive}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	projectsPath := t.TempDir()
	backupPath := filepath.Join(projectsPath, "user-1", "active-instance", "backups", "backup.sql")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.DatabaseBackup{DatabaseInstanceID: instance.ID, ProjectID: uintPtr(1), Name: "active-instance", Path: backupPath, Status: models.BackupStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	if err := FinalizeDatabaseDeletion(context.Background(), db, instance.ID, instance.UID, projectsPath); err == nil {
		t.Fatal("expected active instance rejection")
	}
	assertDatabaseFinalizerPreservedBackup(t, db, instance, backupPath, models.DBStatusActive)
}

func TestFinalizeDatabaseDeletionRejectsIntermediateSymlinkEscape(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseInstance{}, &models.DatabaseBackup{}); err != nil {
		t.Fatal(err)
	}
	instance := models.DatabaseInstance{UserID: 1, Engine: "mysql", Name: "symlink_escape", Username: "symlink_escape_user", Password: "password", Host: "mysql", Port: 3306, Status: models.DBStatusSuspended}
	if err := db.Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	projectsPath := t.TempDir()
	backupDir := filepath.Join(projectsPath, "user-1", "symlink-escape", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	backupPath := filepath.Join(backupDir, "escape", "backup.sql")
	if err := os.Symlink(outsideDir, filepath.Join(backupDir, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "backup.sql"), []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.DatabaseBackup{DatabaseInstanceID: instance.ID, ProjectID: uintPtr(1), Name: "symlink-escape", Path: backupPath, Status: models.BackupStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	if err := FinalizeDatabaseDeletion(context.Background(), db, instance.ID, instance.UID, projectsPath); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
	assertDatabaseFinalizerPreservedBackup(t, db, instance, filepath.Join(outsideDir, "backup.sql"), models.DBStatusSuspended)
}

func assertDatabaseFinalizerPreservedBackup(t *testing.T, db *gorm.DB, instance models.DatabaseInstance, backupPath string, expectedStatus models.DatabaseInstanceStatus) {
	t.Helper()
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file was removed: %v", err)
	}
	var backupCount int64
	if err := db.Model(&models.DatabaseBackup{}).Where("database_instance_id = ?", instance.ID).Count(&backupCount).Error; err != nil || backupCount != 1 {
		t.Fatalf("backup count=%d err=%v", backupCount, err)
	}
	if err := db.First(&instance, instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if instance.Status != expectedStatus {
		t.Fatalf("status=%s", instance.Status)
	}
}

func TestDatabaseCleanupServiceFinalizesRequestedDeletionBeforeTaskDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	instanceID := uint(1)
	task := models.DatabaseCleanupTask{Engine: "mysql", Name: "finalize_db", Username: "finalize_user", Reason: models.DatabaseCleanupReasonRequestedDeletion, DatabaseInstanceID: &instanceID, DatabaseInstanceUID: "instance-uid", DatabaseOwned: true, LastError: "initial"}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	finalized := false
	service := newDatabaseCleanupServiceWithFinalizer(db, func(_ string, _ string, _ string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		if !ownership.DatabaseCreated || ownership.UserCreated {
			t.Fatalf("cleanup ownership=%#v", ownership)
		}
		return infrastructure.ProvisioningOwnership{}, nil
	}, func(_ context.Context, claimed models.DatabaseCleanupTask) error {
		if claimed.DatabaseOwned || claimed.UserOwned {
			t.Fatalf("finalizer ran before physical cleanup checkpoint: %#v", claimed)
		}
		finalized = true
		return nil
	})
	if err := service.processPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !finalized {
		t.Fatal("requested deletion finalizer was not called")
	}
	var count int64
	if err := db.Model(&models.DatabaseCleanupTask{}).Where("id = ?", task.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("task count=%d err=%v", count, err)
	}
}

func TestDatabaseCleanupServiceRetainsRequestedDeletionTaskWhenFinalizationFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	instanceID := uint(1)
	task := models.DatabaseCleanupTask{Engine: "mysql", Name: "finalize_db", Username: "finalize_user", Reason: models.DatabaseCleanupReasonRequestedDeletion, DatabaseInstanceID: &instanceID, DatabaseInstanceUID: "instance-uid", LastError: "initial", RetryCount: 1}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	service := newDatabaseCleanupServiceWithFinalizer(db, func(_ string, _ string, _ string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		return ownership, nil
	}, func(context.Context, models.DatabaseCleanupTask) error { return errors.New("backup cleanup failed") })
	if err := service.processPending(context.Background()); err == nil {
		t.Fatal("finalization failure was swallowed")
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.RetryCount != 2 || task.LeaseToken != "" || task.DatabaseOwned || task.UserOwned {
		t.Fatalf("task=%#v", task)
	}
}
