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
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const databaseReinstallRecoveryInterval = time.Minute

type databaseRecreateFunc func(models.DatabaseInstance, string) error
type databaseEnvironmentSyncFunc func(*models.Project, uint) error

// DatabaseReinstallRecoveryService resumes suspended reinstall operations after a physical
// provisioning or control-plane transaction failure.
type DatabaseReinstallRecoveryService struct {
	db       *gorm.DB
	recreate databaseRecreateFunc
	syncEnv  databaseEnvironmentSyncFunc
}

func NewDatabaseReinstallRecoveryService(db *gorm.DB, secretStore *SecretStoreService) *DatabaseReinstallRecoveryService {
	if secretStore == nil {
		return newDatabaseReinstallRecoveryService(db, recreateManagedDatabaseForRecovery, nil)
	}
	return newDatabaseReinstallRecoveryService(db, recreateManagedDatabaseForRecovery, secretStore.EnqueueDatabaseEnvironmentSync)
}

func newDatabaseReinstallRecoveryService(db *gorm.DB, recreate databaseRecreateFunc, syncEnv databaseEnvironmentSyncFunc) *DatabaseReinstallRecoveryService {
	return &DatabaseReinstallRecoveryService{db: db, recreate: recreate, syncEnv: syncEnv}
}

func (s *DatabaseReinstallRecoveryService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.recreate == nil || s.syncEnv == nil || ctx == nil {
		return
	}
	s.runOnce(ctx)
	ticker := time.NewTicker(databaseReinstallRecoveryInterval)
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

func (s *DatabaseReinstallRecoveryService) runOnce(ctx context.Context) {
	if err := s.processPending(ctx); err != nil {
		slog.Error("Database reinstall recovery processor failed", "error", err)
	}
}

func (s *DatabaseReinstallRecoveryService) processPending(ctx context.Context) error {
	var taskIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&models.DatabaseReinstallRecoveryTask{}).
		Order("id ASC").
		Limit(databaseCleanupBatch).
		Pluck("id", &taskIDs).Error; err != nil {
		return fmt.Errorf("list database reinstall recovery tasks: %w", err)
	}

	var runErr error
	for _, taskID := range taskIDs {
		if _, err := s.resumeTask(ctx, taskID); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("resume database reinstall task %d: %w", taskID, err))
		}
	}
	return runErr
}

// StartOrResumeDatabaseReinstall creates a durable task for an active instance or resumes its
// existing task when it was suspended by a previous failed reinstall.
func (s *DatabaseReinstallRecoveryService) StartOrResumeDatabaseReinstall(ctx context.Context, databaseUID string, userID uint) (models.Project, models.DatabaseInstance, error) {
	if s == nil || s.db == nil || s.recreate == nil || s.syncEnv == nil {
		return models.Project{}, models.DatabaseInstance{}, errors.New("database reinstall recovery service unavailable")
	}

	var identity models.DatabaseInstance
	if err := s.db.WithContext(ctx).Where("uid = ?", databaseUID).First(&identity).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, err
	}

	var project models.Project
	var instance models.DatabaseInstance
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, identity.Engine, identity.Name, identity.Username, func(lockDB *gorm.DB) error {
		task, preparedInstance, err := prepareDatabaseReinstallTask(lockDB, databaseUID, userID)
		if err != nil {
			return err
		}
		instance = preparedInstance
		project, instance, err = s.resumeTaskLocked(ctx, lockDB, task.ID)
		return err
	})
	return project, instance, err
}

func (s *DatabaseReinstallRecoveryService) resumeTask(ctx context.Context, taskID uint) (models.DatabaseInstance, error) {
	var task models.DatabaseReinstallRecoveryTask
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.DatabaseInstance{}, nil
		}
		return models.DatabaseInstance{}, err
	}

	var instance models.DatabaseInstance
	err := WithDatabaseCleanupIdentityLock(ctx, s.db, task.Engine, task.Name, task.Username, func(lockDB *gorm.DB) error {
		_, resumedInstance, err := s.resumeTaskLocked(ctx, lockDB, taskID)
		instance = resumedInstance
		return err
	})
	return instance, err
}

func prepareDatabaseReinstallTask(db *gorm.DB, databaseUID string, userID uint) (models.DatabaseReinstallRecoveryTask, models.DatabaseInstance, error) {
	var task models.DatabaseReinstallRecoveryTask
	var instance models.DatabaseInstance
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", databaseUID).First(&instance).Error; err != nil {
			return err
		}
		if instance.UserID != userID {
			return apperr.New(403, "DATABASE_FORBIDDEN", "Forbidden: You do not own this database")
		}
		switch instance.Status {
		case models.DBStatusActive:
			previousBillingStatus, err := suspendDatabaseBillingForReinstall(tx, instance.ID)
			if err != nil {
				return err
			}
			if err := tx.Model(&instance).Where("status = ?", models.DBStatusActive).Update("status", models.DBStatusSuspended).Error; err != nil {
				return fmt.Errorf("suspend database before reinstall: %w", err)
			}
			task = models.DatabaseReinstallRecoveryTask{
				DatabaseInstanceID:    instance.ID,
				DatabaseInstanceUID:   instance.UID,
				Engine:                instance.Engine,
				Name:                  instance.Name,
				Username:              instance.Username,
				Password:              utils.GeneratePassword(16),
				PreviousBillingStatus: previousBillingStatus,
				Checkpoint:            models.DatabaseReinstallCheckpointSuspended,
			}
			if err := tx.Create(&task).Error; err != nil {
				return fmt.Errorf("create database reinstall recovery task: %w", err)
			}
			instance.Status = models.DBStatusSuspended
			return nil
		case models.DBStatusSuspended:
			if err := tx.Where("database_instance_id = ? AND database_instance_uid = ?", instance.ID, instance.UID).First(&task).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperr.New(409, "DATABASE_NOT_ACTIVE", "Database is not active")
				}
				return fmt.Errorf("load database reinstall recovery task: %w", err)
			}
			return nil
		default:
			return apperr.New(409, "DATABASE_NOT_ACTIVE", "Database is not active")
		}
	})
	return task, instance, err
}

func suspendDatabaseBillingForReinstall(tx *gorm.DB, databaseID uint) (*models.BillableResourceStatus, error) {
	var resource models.BillableResource
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("type = ? AND resource_id = ?", models.BillableTypeDatabase, databaseID).
		First(&resource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock database billing during reinstall: %w", err)
	}
	previous := resource.BillingStatus
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
			return nil, fmt.Errorf("suspend database billing during reinstall: %w", err)
		}
	}
	return &previous, nil
}

func (s *DatabaseReinstallRecoveryService) resumeTaskLocked(ctx context.Context, db *gorm.DB, taskID uint) (models.Project, models.DatabaseInstance, error) {
	var task models.DatabaseReinstallRecoveryTask
	if err := db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Project{}, models.DatabaseInstance{}, nil
		}
		return models.Project{}, models.DatabaseInstance{}, fmt.Errorf("load database reinstall recovery task: %w", err)
	}
	if task.Checkpoint == models.DatabaseReinstallCheckpointSuspended {
		instance, err := loadReinstallInstance(db, task)
		if err != nil {
			return models.Project{}, models.DatabaseInstance{}, err
		}
		if err := s.recreate(instance, task.Password); err != nil {
			return models.Project{}, instance, recordDatabaseReinstallFailure(db, task.ID, err)
		}
		if err := db.Model(&models.DatabaseReinstallRecoveryTask{}).Where("id = ? AND checkpoint = ?", task.ID, models.DatabaseReinstallCheckpointSuspended).Updates(map[string]any{
			"checkpoint":  models.DatabaseReinstallCheckpointPhysicalRecreated,
			"last_error":  "",
			"retry_count": gorm.Expr("retry_count + 1"),
		}).Error; err != nil {
			return models.Project{}, instance, fmt.Errorf("checkpoint recreated database: %w", err)
		}
		task.Checkpoint = models.DatabaseReinstallCheckpointPhysicalRecreated
	}

	var project models.Project
	var instance models.DatabaseInstance
	if task.Checkpoint == models.DatabaseReinstallCheckpointPhysicalRecreated {
		var err error
		project, instance, err = completeDatabaseReinstallRecovery(db, task)
		if err != nil {
			if recoveredProject, recoveredInstance, recovered := verifyDatabaseReinstallCompletion(db, task); recovered {
				project, instance = recoveredProject, recoveredInstance
			} else {
				return models.Project{}, instance, recordDatabaseReinstallFailure(db, task.ID, err)
			}
		}
		if err := db.First(&task, task.ID).Error; err != nil {
			return project, instance, fmt.Errorf("reload database reinstall env sync checkpoint: %w", err)
		}
	}
	if task.Checkpoint != models.DatabaseReinstallCheckpointEnvSyncPending {
		return project, instance, fmt.Errorf("invalid database reinstall checkpoint %q", task.Checkpoint)
	}
	if project.ID == 0 && task.DatabaseInstanceID != 0 {
		var loadErr error
		project, instance, loadErr = loadReinstalledProject(db, task)
		if loadErr != nil {
			return models.Project{}, instance, recordDatabaseReinstallFailure(db, task.ID, loadErr)
		}
	}
	if project.ID == 0 {
		if err := deleteDatabaseReinstallRecoveryTask(db, task.ID); err != nil {
			return project, instance, recordDatabaseReinstallFailure(db, task.ID, fmt.Errorf("complete standalone database reinstall: %w", err))
		}
		return project, instance, nil
	}
	if task.EnvSyncGeneration == 0 {
		generation, err := ensureDatabaseReinstallEnvSyncGeneration(db, task.ID, project.ID)
		if err != nil {
			return project, instance, recordDatabaseReinstallFailure(db, task.ID, err)
		}
		task.EnvSyncGeneration = generation
	}
	acknowledged, err := projectEnvSyncAcknowledged(db, project.ID, task.EnvSyncGeneration)
	if err != nil {
		return project, instance, recordDatabaseReinstallFailure(db, task.ID, err)
	}
	if acknowledged {
		if err := deleteDatabaseReinstallRecoveryTask(db, task.ID); err != nil {
			return project, instance, recordDatabaseReinstallFailure(db, task.ID, fmt.Errorf("complete acknowledged database environment sync: %w", err))
		}
		return project, instance, nil
	}
	if err := s.syncEnv(&project, task.EnvSyncGeneration); err != nil {
		return project, instance, recordDatabaseReinstallFailure(db, task.ID, fmt.Errorf("enqueue database environment sync: %w", err))
	}
	return project, instance, nil
}

func loadReinstallInstance(db *gorm.DB, task models.DatabaseReinstallRecoveryTask) (models.DatabaseInstance, error) {
	var instance models.DatabaseInstance
	if err := db.Where("id = ? AND uid = ? AND status = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID, models.DBStatusSuspended).First(&instance).Error; err != nil {
		return models.DatabaseInstance{}, fmt.Errorf("load suspended database reinstall instance: %w", err)
	}
	return instance, nil
}

func completeDatabaseReinstallRecovery(db *gorm.DB, task models.DatabaseReinstallRecoveryTask) (models.Project, models.DatabaseInstance, error) {
	var project models.Project
	var instance models.DatabaseInstance
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND uid = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID).First(&instance).Error; err != nil {
			return fmt.Errorf("lock database reinstall completion: %w", err)
		}
		if instance.Status != models.DBStatusSuspended {
			return apperr.New(409, "DATABASE_REINSTALL_CONFLICT", "Database state changed during reinstall")
		}
		if err := tx.Model(&instance).Updates(map[string]any{"password": task.Password, "status": models.DBStatusActive}).Error; err != nil {
			return fmt.Errorf("activate reinstalled database: %w", err)
		}
		if task.PreviousBillingStatus != nil {
			var resource models.BillableResource
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeDatabase, instance.ID).First(&resource).Error; err != nil {
				return fmt.Errorf("lock database billing completion: %w", err)
			}
			if resource.BillingStatus != models.BillableResourceStatusSuspended {
				return apperr.New(409, "DATABASE_REINSTALL_CONFLICT", "Database billing state changed during reinstall")
			}
			if err := tx.Model(&resource).Update("billing_status", *task.PreviousBillingStatus).Error; err != nil {
				return fmt.Errorf("restore database billing after reinstall: %w", err)
			}
		}
		if instance.ProjectID != nil {
			if err := tx.First(&project, *instance.ProjectID).Error; err != nil {
				return fmt.Errorf("load project after database reinstall: %w", err)
			}
			if err := tx.Model(&project).Update("database_password", task.Password).Error; err != nil {
				return fmt.Errorf("update project database password after reinstall: %w", err)
			}
			project.DatabasePassword = task.Password
		}
		updates := map[string]any{"checkpoint": models.DatabaseReinstallCheckpointEnvSyncPending, "last_error": ""}
		if project.ID != 0 {
			generation, err := createProjectEnvSyncGeneration(tx, project.ID)
			if err != nil {
				return err
			}
			updates["env_sync_generation"] = generation
		}
		if err := tx.Model(&models.DatabaseReinstallRecoveryTask{}).Where("id = ? AND checkpoint = ?", task.ID, models.DatabaseReinstallCheckpointPhysicalRecreated).Updates(updates).Error; err != nil {
			return fmt.Errorf("checkpoint database environment sync: %w", err)
		}
		instance.Status = models.DBStatusActive
		instance.Password = task.Password
		return nil
	})
	return project, instance, err
}

func createProjectEnvSyncGeneration(tx *gorm.DB, projectID uint) (uint, error) {
	var task models.ProjectEnvSyncTask
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("project_id = ?", projectID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		task = models.ProjectEnvSyncTask{ProjectID: projectID, DesiredGeneration: 1}
		if err := tx.Create(&task).Error; err != nil {
			return 0, fmt.Errorf("create project environment sync task: %w", err)
		}
		return task.DesiredGeneration, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock project environment sync task: %w", err)
	}
	next := task.DesiredGeneration + 1
	if err := tx.Model(&task).Updates(map[string]any{"desired_generation": next, "last_error": ""}).Error; err != nil {
		return 0, fmt.Errorf("advance project environment sync generation: %w", err)
	}
	return next, nil
}

func ensureDatabaseReinstallEnvSyncGeneration(db *gorm.DB, reinstallTaskID, projectID uint) (uint, error) {
	var generation uint
	err := db.Transaction(func(tx *gorm.DB) error {
		generation, err := createProjectEnvSyncGeneration(tx, projectID)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.DatabaseReinstallRecoveryTask{}).Where("id = ? AND checkpoint = ? AND env_sync_generation = ?", reinstallTaskID, models.DatabaseReinstallCheckpointEnvSyncPending, 0).Update("env_sync_generation", generation).Error; err != nil {
			return fmt.Errorf("checkpoint database environment sync generation: %w", err)
		}
		return nil
	})
	return generation, err
}

func projectEnvSyncAcknowledged(db *gorm.DB, projectID, generation uint) (bool, error) {
	var task models.ProjectEnvSyncTask
	if err := db.Where("project_id = ?", projectID).First(&task).Error; err != nil {
		return false, fmt.Errorf("load project environment sync acknowledgement: %w", err)
	}
	return task.AcknowledgedGeneration >= generation, nil
}

func deleteDatabaseReinstallRecoveryTask(db *gorm.DB, taskID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND checkpoint = ?", taskID, models.DatabaseReinstallCheckpointEnvSyncPending).Delete(&models.DatabaseReinstallRecoveryTask{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func verifyDatabaseReinstallCompletion(db *gorm.DB, task models.DatabaseReinstallRecoveryTask) (models.Project, models.DatabaseInstance, bool) {
	var instance models.DatabaseInstance
	if err := db.Where("id = ? AND uid = ? AND status = ? AND password = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID, models.DBStatusActive, task.Password).First(&instance).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, false
	}
	var recoveryTask models.DatabaseReinstallRecoveryTask
	if err := db.Where("id = ? AND checkpoint = ?", task.ID, models.DatabaseReinstallCheckpointEnvSyncPending).First(&recoveryTask).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, false
	}
	var project models.Project
	if instance.ProjectID != nil && db.First(&project, *instance.ProjectID).Error != nil {
		return models.Project{}, models.DatabaseInstance{}, false
	}
	return project, instance, true
}

func loadReinstalledProject(db *gorm.DB, task models.DatabaseReinstallRecoveryTask) (models.Project, models.DatabaseInstance, error) {
	var instance models.DatabaseInstance
	if err := db.Where("id = ? AND uid = ? AND status = ?", task.DatabaseInstanceID, task.DatabaseInstanceUID, models.DBStatusActive).First(&instance).Error; err != nil {
		return models.Project{}, models.DatabaseInstance{}, fmt.Errorf("load completed database reinstall instance: %w", err)
	}
	if instance.ProjectID == nil {
		return models.Project{}, instance, nil
	}
	var project models.Project
	if err := db.First(&project, *instance.ProjectID).Error; err != nil {
		return models.Project{}, instance, fmt.Errorf("load completed database reinstall project: %w", err)
	}
	return project, instance, nil
}

func recordDatabaseReinstallFailure(db *gorm.DB, taskID uint, reinstallErr error) error {
	if updateErr := db.Model(&models.DatabaseReinstallRecoveryTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"last_error":  reinstallErr.Error(),
		"retry_count": gorm.Expr("retry_count + 1"),
	}).Error; updateErr != nil {
		return errors.Join(reinstallErr, fmt.Errorf("record database reinstall recovery failure: %w", updateErr))
	}
	return reinstallErr
}

func recreateManagedDatabaseForRecovery(instance models.DatabaseInstance, password string) error {
	connectionLimit := instance.ConnectionLimit
	if connectionLimit <= 0 {
		connectionLimit = infrastructure.DefaultManagedDatabaseConnectionLimit
	}
	if instance.Engine == "postgresql" {
		service := infrastructure.NewPostgreSQLService()
		if err := service.DropDatabaseCustom(instance.Name, instance.Username); err != nil {
			return fmt.Errorf("drop postgresql database during reinstall: %w", err)
		}
		if err := service.CreateDatabaseCustomWithConnectionLimit(instance.Name, instance.Username, password, connectionLimit); err != nil {
			return fmt.Errorf("recreate postgresql database during reinstall: %w", err)
		}
		return nil
	}
	service := infrastructure.NewMySQLService()
	if err := service.DropDatabaseCustom(instance.Name, instance.Username); err != nil {
		return fmt.Errorf("drop mysql database during reinstall: %w", err)
	}
	if err := service.CreateDatabaseCustomWithConnectionLimit(instance.Name, instance.Username, password, connectionLimit); err != nil {
		return fmt.Errorf("recreate mysql database during reinstall: %w", err)
	}
	return nil
}
