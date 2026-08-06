package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const databaseStatusOperationInterval = time.Minute

var errDatabaseStatusOperationSuperseded = errors.New("database status operation superseded")

type databaseStatusTransitionFunc func(models.DatabaseInstance, models.DatabaseInstanceStatus) error

type DatabaseStatusOperationService struct {
	db         *gorm.DB
	transition databaseStatusTransitionFunc
}

func NewDatabaseStatusOperationService(db *gorm.DB) *DatabaseStatusOperationService {
	return newDatabaseStatusOperationService(db, applyDatabaseStatusTransition)
}

func newDatabaseStatusOperationService(db *gorm.DB, transition databaseStatusTransitionFunc) *DatabaseStatusOperationService {
	return &DatabaseStatusOperationService{db: db, transition: transition}
}

func (s *DatabaseStatusOperationService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.transition == nil || ctx == nil {
		return
	}
	s.runOnce(ctx)
	ticker := time.NewTicker(databaseStatusOperationInterval)
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

func (s *DatabaseStatusOperationService) runOnce(ctx context.Context) {
	if err := s.ProcessPending(ctx); err != nil {
		slog.Error("Database status operation processor failed", "error", err)
	}
}

func (s *DatabaseStatusOperationService) ProcessPending(ctx context.Context) error {
	var taskIDs []uint
	if err := s.db.WithContext(ctx).Model(&models.DatabaseStatusOperationTask{}).Order("id ASC").Limit(databaseCleanupBatch).Pluck("id", &taskIDs).Error; err != nil {
		return fmt.Errorf("list database status operations: %w", err)
	}
	var runErr error
	for _, taskID := range taskIDs {
		if _, err := s.processTask(ctx, taskID); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	return runErr
}

func (s *DatabaseStatusOperationService) Request(ctx context.Context, databaseID uint, desired models.DatabaseInstanceStatus) (models.DatabaseInstance, error) {
	if desired != models.DBStatusActive && desired != models.DBStatusSuspended {
		return models.DatabaseInstance{}, apperr.NewBadRequest("Invalid database status")
	}
	var identity models.DatabaseInstance
	if err := s.db.WithContext(ctx).First(&identity, databaseID).Error; err != nil {
		return models.DatabaseInstance{}, err
	}
	var result models.DatabaseInstance
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, identity.Engine, identity.Name, identity.Username, func(lockDB *gorm.DB) error {
		if err := s.ensureTask(lockDB, identity.ID, desired); err != nil {
			return err
		}
		var err error
		result, err = s.processTaskLocked(ctx, lockDB, identity.ID)
		return err
	})
	return result, err
}

func (s *DatabaseStatusOperationService) ensureTask(db *gorm.DB, databaseID uint, desired models.DatabaseInstanceStatus) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var instance models.DatabaseInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&instance, databaseID).Error; err != nil {
			return err
		}
		var task models.DatabaseStatusOperationTask
		err := tx.Where("database_instance_id = ?", instance.ID).First(&task).Error
		if err == nil {
			if task.DesiredStatus != desired {
				if err := tx.Model(&task).Updates(map[string]any{"desired_status": desired, "physical_applied": false, "last_error": "", "retry_count": 0}).Error; err != nil {
					return fmt.Errorf("replace pending database status operation: %w", err)
				}
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load database status operation: %w", err)
		}
		if instance.Status == desired {
			return nil
		}
		task = models.DatabaseStatusOperationTask{DatabaseInstanceID: instance.ID, DatabaseInstanceUID: instance.UID, Engine: instance.Engine, Name: instance.Name, Username: instance.Username, DesiredStatus: desired}
		if err := tx.Create(&task).Error; err != nil {
			return fmt.Errorf("create database status operation: %w", err)
		}
		return nil
	})
}

func (s *DatabaseStatusOperationService) processTask(ctx context.Context, taskID uint) (models.DatabaseInstance, error) {
	var task models.DatabaseStatusOperationTask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.DatabaseInstance{}, nil
		}
		return models.DatabaseInstance{}, err
	}
	var result models.DatabaseInstance
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, task.Engine, task.Name, task.Username, func(lockDB *gorm.DB) error {
		var err error
		result, err = s.processTaskLocked(ctx, lockDB, task.DatabaseInstanceID)
		return err
	})
	return result, err
}

func (s *DatabaseStatusOperationService) processTaskLocked(ctx context.Context, db *gorm.DB, databaseID uint) (models.DatabaseInstance, error) {
	var task models.DatabaseStatusOperationTask
	if err := db.WithContext(ctx).Where("database_instance_id = ?", databaseID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var instance models.DatabaseInstance
			if err := db.WithContext(ctx).First(&instance, databaseID).Error; err != nil {
				return models.DatabaseInstance{}, err
			}
			return instance, nil
		}
		return models.DatabaseInstance{}, err
	}
	var instance models.DatabaseInstance
	if err := db.WithContext(ctx).Where("id = ? AND uid = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID).First(&instance).Error; err != nil {
		return models.DatabaseInstance{}, fmt.Errorf("load database status operation instance: %w", err)
	}
	if instance.Status == models.DBStatusDeleted {
		if err := db.WithContext(ctx).Delete(&models.DatabaseStatusOperationTask{}, task.ID).Error; err != nil {
			return instance, fmt.Errorf("discard stale database status operation: %w", err)
		}
		return instance, nil
	}
	if instance.Status != models.DBStatusActive && instance.Status != models.DBStatusSuspended {
		return instance, fmt.Errorf("database status operation requires an active or suspended instance")
	}
	if !task.PhysicalApplied {
		if err := s.transition(instance, task.DesiredStatus); err != nil {
			if updateErr := db.Model(&models.DatabaseStatusOperationTask{}).Where("id = ?", task.ID).Updates(map[string]any{"last_error": err.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; updateErr != nil {
				return instance, errors.Join(err, updateErr)
			}
			return instance, err
		}
		checkpoint := db.Model(&models.DatabaseStatusOperationTask{}).Where("id = ? AND desired_status = ? AND physical_applied = ?", task.ID, task.DesiredStatus, false).Updates(map[string]any{"physical_applied": true, "last_error": ""})
		if checkpoint.Error != nil {
			return instance, fmt.Errorf("checkpoint physical database status operation: %w", checkpoint.Error)
		}
		if checkpoint.RowsAffected != 1 {
			return s.processTaskLocked(ctx, db, databaseID)
		}
		task.PhysicalApplied = true
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var currentInstance models.DatabaseInstance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND uid = ?", instance.ID, instance.UID).First(&currentInstance).Error; err != nil {
			return fmt.Errorf("lock database status operation instance finalization: %w", err)
		}
		var currentTask models.DatabaseStatusOperationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentTask, task.ID).Error; err != nil {
			return fmt.Errorf("lock database status operation finalization: %w", err)
		}
		if currentTask.DesiredStatus != task.DesiredStatus || !currentTask.PhysicalApplied {
			return errDatabaseStatusOperationSuperseded
		}
		if err := tx.Model(&currentInstance).Update("status", task.DesiredStatus).Error; err != nil {
			return fmt.Errorf("persist database status operation: %w", err)
		}
		result := tx.Where("id = ? AND desired_status = ? AND physical_applied = ?", task.ID, task.DesiredStatus, true).Delete(&models.DatabaseStatusOperationTask{})
		if result.Error != nil {
			return fmt.Errorf("complete database status operation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return errDatabaseStatusOperationSuperseded
		}
		return nil
	}); err != nil {
		if errors.Is(err, errDatabaseStatusOperationSuperseded) {
			return s.processTaskLocked(ctx, db, databaseID)
		}
		return instance, err
	}
	instance.Status = task.DesiredStatus
	return instance, nil
}

func applyDatabaseStatusTransition(instance models.DatabaseInstance, desired models.DatabaseInstanceStatus) error {
	suspend := desired == models.DBStatusSuspended
	if instance.Engine == "postgresql" {
		return infrastructure.NewPostgreSQLService().UpdateStatus(instance.Name, instance.Username, suspend)
	}
	return infrastructure.NewMySQLService().UpdateStatus(instance.Name, instance.Username, instance.ConnectionLimit, suspend)
}
