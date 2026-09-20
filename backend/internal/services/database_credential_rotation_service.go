package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const databaseCredentialRotationInterval = time.Minute

type DatabaseCredentialRotationService struct {
	db    *gorm.DB
	apply func(string, string, string) error
}

func NewDatabaseCredentialRotationService(db *gorm.DB) *DatabaseCredentialRotationService {
	return newDatabaseCredentialRotationService(db, applyDatabasePassword)
}

func newDatabaseCredentialRotationService(db *gorm.DB, apply func(string, string, string) error) *DatabaseCredentialRotationService {
	return &DatabaseCredentialRotationService{db: db, apply: apply}
}

func (s *DatabaseCredentialRotationService) StartOrResume(ctx context.Context, projectID, instanceID uint) (models.Project, models.DatabaseInstance, uint, error) {
	var identity models.DatabaseInstance
	if err := s.db.WithContext(ctx).Where("id = ? AND project_id = ?", instanceID, projectID).First(&identity).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, 0, err
	}
	var taskID uint
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, identity.Engine, identity.Name, identity.Username, func(lockDB *gorm.DB) error {
		return lockDB.Transaction(func(tx *gorm.DB) error {
			var project models.Project
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
				return err
			}
			var instance models.DatabaseInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", instanceID, projectID).First(&instance).Error; err != nil {
				return err
			}
			var task models.DatabaseCredentialRotationTask
			err := tx.Where("database_instance_id = ?", instance.ID).First(&task).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				task = models.DatabaseCredentialRotationTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, ProjectID: project.ID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, PreviousPassword: instance.Password, NewPassword: utils.GeneratePassword(16)}
				if err := tx.Create(&task).Error; err != nil {
					return fmt.Errorf("create database credential rotation task: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("load database credential rotation task: %w", err)
			}
			taskID = task.ID
			return nil
		})
	})
	if err != nil {
		return models.Project{}, models.DatabaseInstance{}, 0, err
	}
	return s.resume(ctx, taskID)
}

func (s *DatabaseCredentialRotationService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.apply == nil || ctx == nil {
		return
	}
	s.runOnce(ctx)
	ticker := time.NewTicker(databaseCredentialRotationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *DatabaseCredentialRotationService) runOnce(ctx context.Context) {
	var taskIDs []uint
	if err := s.db.WithContext(ctx).Model(&models.DatabaseCredentialRotationTask{}).Order("id ASC").Limit(databaseCleanupBatch).Pluck("id", &taskIDs).Error; err != nil {
		slog.Error("List database credential rotation tasks failed", "error", err)
		return
	}
	for _, taskID := range taskIDs {
		if _, _, _, err := s.resume(ctx, taskID); err != nil {
			slog.Error("Resume database credential rotation failed", "task_id", taskID, "error", err)
		}
	}
}

func (s *DatabaseCredentialRotationService) resume(ctx context.Context, taskID uint) (models.Project, models.DatabaseInstance, uint, error) {
	var task models.DatabaseCredentialRotationTask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, 0, err
	}
	var project models.Project
	var instance models.DatabaseInstance
	var generation uint
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, task.Engine, task.Name, task.Username, func(lockDB *gorm.DB) error {
		if !task.PhysicalApplied {
			if err := s.apply(task.Engine, task.Username, task.NewPassword); err != nil {
				return s.recordFailure(lockDB, task.ID, err)
			}
			if err := lockDB.Model(&models.DatabaseCredentialRotationTask{}).Where("id = ? AND physical_applied = ?", task.ID, false).Updates(map[string]any{"physical_applied": true, "last_error": ""}).Error; err != nil {
				return fmt.Errorf("checkpoint physical credential rotation: %w", err)
			}
			task.PhysicalApplied = true
		}
		return lockDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND uid = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID).First(&instance).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, task.ProjectID).Error; err != nil {
				return err
			}
			if instance.Password != task.NewPassword {
				if err := tx.Model(&instance).Update("password", task.NewPassword).Error; err != nil {
					return err
				}
			}
			if project.DatabasePassword != task.NewPassword {
				if err := tx.Model(&project).Update("database_password", task.NewPassword).Error; err != nil {
					return err
				}
			}
			var err error
			generation, err = RequestProjectEnvSyncTx(tx, project.ID)
			if err != nil {
				return err
			}
			if err := tx.Delete(&models.DatabaseCredentialRotationTask{}, task.ID).Error; err != nil {
				return err
			}
			instance.Password = task.NewPassword
			project.DatabasePassword = task.NewPassword
			return nil
		})
	})
	if err != nil {
		return project, instance, 0, err
	}
	return project, instance, generation, nil
}

func (s *DatabaseCredentialRotationService) recordFailure(db *gorm.DB, taskID uint, rotationErr error) error {
	if err := db.Model(&models.DatabaseCredentialRotationTask{}).Where("id = ?", taskID).Updates(map[string]any{"last_error": rotationErr.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; err != nil {
		return errors.Join(rotationErr, err)
	}
	return rotationErr
}

func applyDatabasePassword(engine, username, password string) error {
	if engine == "postgresql" {
		return infrastructure.NewPostgreSQLService().UpdatePassword(username, password)
	}
	return infrastructure.NewMySQLService().UpdatePassword(username, password)
}
