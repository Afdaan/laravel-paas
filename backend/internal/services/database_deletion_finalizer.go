package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FinalizeDatabaseDeletion removes backups and marks an already-deprovisioned database deleted.
func FinalizeDatabaseDeletion(ctx context.Context, db *gorm.DB, databaseInstanceID uint, databaseInstanceUID, projectsPath string) error {
	if db == nil || databaseInstanceID == 0 || databaseInstanceUID == "" || projectsPath == "" {
		return errors.New("database deletion finalizer is invalid")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instance models.DatabaseInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", databaseInstanceID).First(&instance).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("database deletion finalizer instance not found")
			}
			return fmt.Errorf("lock database deletion finalizer instance: %w", err)
		}
		if instance.UID != databaseInstanceUID {
			return errors.New("database deletion finalizer instance identity mismatch")
		}
		if instance.Status == models.DBStatusDeleted {
			return nil
		}
		if instance.Status != models.DBStatusSuspended {
			return fmt.Errorf("database deletion finalizer requires suspended instance")
		}
		if err := deleteDatabaseBackups(ctx, tx, databaseInstanceID, projectsPath); err != nil {
			return err
		}
		if err := tx.Model(&instance).Updates(map[string]any{"status": models.DBStatusDeleted, "project_id": nil}).Error; err != nil {
			return fmt.Errorf("finalize database deletion status: %w", err)
		}
		return nil
	})
}

func deleteDatabaseBackups(ctx context.Context, db *gorm.DB, databaseInstanceID uint, projectsPath string) error {
	var backups []models.DatabaseBackup
	if err := db.WithContext(ctx).Where("database_instance_id = ?", databaseInstanceID).Find(&backups).Error; err != nil {
		return fmt.Errorf("list database deletion backups: %w", err)
	}
	if len(backups) == 0 {
		return nil
	}
	absProjectsPath, err := filepath.Abs(projectsPath)
	if err != nil {
		return fmt.Errorf("resolve database backup projects path: %w", err)
	}
	realProjectsPath, err := filepath.EvalSymlinks(absProjectsPath)
	if err != nil {
		return fmt.Errorf("resolve database backup projects path: %w", err)
	}
	for _, backup := range backups {
		if backup.Path != "" {
			backupPath, err := resolveManagedBackupPath(realProjectsPath, backup.Path)
			if err != nil {
				return err
			}
			if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete database backup file: %w", err)
			}
		}
		if err := db.WithContext(ctx).Delete(&backup).Error; err != nil {
			return fmt.Errorf("delete database backup record: %w", err)
		}
	}
	return nil
}

func resolveManagedBackupPath(realProjectsPath, backupPath string) (string, error) {
	absBackupPath, err := filepath.Abs(filepath.Clean(backupPath))
	if err != nil {
		return "", fmt.Errorf("resolve database backup path: %w", err)
	}
	realBackupParent, err := filepath.EvalSymlinks(filepath.Dir(absBackupPath))
	if err != nil {
		return "", fmt.Errorf("resolve database backup parent path: %w", err)
	}
	resolvedBackupPath := filepath.Join(realBackupParent, filepath.Base(absBackupPath))
	relativePath, err := filepath.Rel(realProjectsPath, resolvedBackupPath)
	if err != nil || strings.HasPrefix(relativePath, "..") || filepath.IsAbs(relativePath) {
		return "", errors.New("database backup path is outside configured projects path")
	}
	return resolvedBackupPath, nil
}
