package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	databaseCleanupInterval = time.Minute
	databaseCleanupBatch    = 25
	databaseCleanupLease    = 5 * time.Minute
)

type databaseCleanupFunc func(string, string, string, infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error)
type databaseDeletionFinalizer func(context.Context, models.DatabaseCleanupTask) error

// DatabaseCleanupService retries compensating cleanup after a provisioning transaction fails.
type DatabaseCleanupService struct {
	db       *gorm.DB
	cleanup  databaseCleanupFunc
	finalize databaseDeletionFinalizer
}

func NewDatabaseCleanupService(db *gorm.DB, projectsPath string) *DatabaseCleanupService {
	return newDatabaseCleanupServiceWithFinalizer(db, DeprovisionStandaloneDatabase, func(ctx context.Context, task models.DatabaseCleanupTask) error {
		if task.DatabaseInstanceID == nil {
			return errors.New("requested database deletion cleanup task has no database instance ID")
		}
		return FinalizeDatabaseDeletion(ctx, db, *task.DatabaseInstanceID, task.DatabaseInstanceUID, projectsPath)
	})
}

func newDatabaseCleanupService(db *gorm.DB, cleanup databaseCleanupFunc) *DatabaseCleanupService {
	return newDatabaseCleanupServiceWithFinalizer(db, cleanup, nil)
}

func newDatabaseCleanupServiceWithFinalizer(db *gorm.DB, cleanup databaseCleanupFunc, finalize databaseDeletionFinalizer) *DatabaseCleanupService {
	return &DatabaseCleanupService{db: db, cleanup: cleanup, finalize: finalize}
}

func (s *DatabaseCleanupService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.cleanup == nil || ctx == nil {
		return
	}
	s.runOnce(ctx)
	ticker := time.NewTicker(databaseCleanupInterval)
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

func (s *DatabaseCleanupService) runOnce(ctx context.Context) {
	if err := s.processPending(ctx); err != nil {
		slog.Error("Database cleanup processor failed", "error", err)
	}
}

func (s *DatabaseCleanupService) processPending(ctx context.Context) error {
	if s == nil || s.db == nil || s.cleanup == nil || ctx == nil {
		return fmt.Errorf("database cleanup processor unavailable")
	}
	tasks, err := s.claimPending(ctx)
	if err != nil {
		return err
	}
	var processErr error
	for _, task := range tasks {
		processErr = errors.Join(processErr, s.processClaimedTask(ctx, task))
	}
	return processErr
}

func (s *DatabaseCleanupService) claimPending(ctx context.Context) ([]models.DatabaseCleanupTask, error) {
	now := time.Now().UTC()
	leaseToken := uuid.NewString()
	leaseExpiresAt := now.Add(databaseCleanupLease)
	var tasks []models.DatabaseCleanupTask
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("lease_expires_at IS NULL OR lease_expires_at <= ?", now).Order("id ASC").Limit(databaseCleanupBatch)
		if isPostgreSQL(tx) {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := query.Find(&tasks).Error; err != nil {
			return fmt.Errorf("list database cleanup tasks: %w", err)
		}
		for index := range tasks {
			task := &tasks[index]
			if err := tx.Model(task).Updates(map[string]any{
				"lease_token":      leaseToken,
				"lease_expires_at": leaseExpiresAt,
			}).Error; err != nil {
				return fmt.Errorf("claim database cleanup task %d: %w", task.ID, err)
			}
			task.LeaseToken = leaseToken
			task.LeaseExpiresAt = &leaseExpiresAt
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *DatabaseCleanupService) processClaimedTask(ctx context.Context, task models.DatabaseCleanupTask) error {
	return WithDatabaseCleanupIdentityLock(ctx, s.db, task.Engine, task.Name, task.Username, func(lockDB *gorm.DB) error {
		var claimed models.DatabaseCleanupTask
		now := time.Now().UTC()
		if err := lockDB.WithContext(ctx).
			Where("id = ? AND lease_token = ? AND lease_expires_at > ?", task.ID, task.LeaseToken, now).
			First(&claimed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("reload claimed database cleanup task %d: %w", task.ID, err)
		}
		if claimed.Reason == models.DatabaseCleanupReasonProvisioning && (claimed.DatabaseOwned || claimed.UserOwned) {
			if err := s.rejectCleanupForExistingInstance(ctx, lockDB, &claimed); err != nil {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, infrastructure.ProvisioningOwnership{DatabaseCreated: claimed.DatabaseOwned, UserCreated: claimed.UserOwned}, err)
			}
		}

		if claimed.DatabaseOwned {
			remaining, err := s.cleanup(claimed.Engine, claimed.Name, claimed.Username, infrastructure.ProvisioningOwnership{DatabaseCreated: true})
			if err != nil {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, remaining, err)
			}
			if remaining.DatabaseCreated {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, remaining, errors.New("database cleanup completed without clearing ownership"))
			}
			if err := checkpointDatabaseCleanupOwnership(ctx, lockDB, &claimed, false, claimed.UserOwned); err != nil {
				return err
			}
			claimed.DatabaseOwned = false
		}
		if claimed.UserOwned {
			remaining, err := s.cleanup(claimed.Engine, claimed.Name, claimed.Username, infrastructure.ProvisioningOwnership{UserCreated: true})
			if err != nil {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, remaining, err)
			}
			if remaining.UserCreated {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, remaining, errors.New("database user cleanup completed without clearing ownership"))
			}
			if err := checkpointDatabaseCleanupOwnership(ctx, lockDB, &claimed, claimed.DatabaseOwned, false); err != nil {
				return err
			}
			claimed.UserOwned = false
		}
		if claimed.Reason == models.DatabaseCleanupReasonRequestedDeletion {
			if s.finalize == nil {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, infrastructure.ProvisioningOwnership{}, errors.New("database deletion finalizer is unavailable"))
			}
			if err := s.finalize(ctx, claimed); err != nil {
				return s.releaseClaimAfterFailure(ctx, lockDB, &claimed, infrastructure.ProvisioningOwnership{}, err)
			}
		}
		if err := lockDB.WithContext(ctx).Where("id = ? AND lease_token = ?", claimed.ID, claimed.LeaseToken).Delete(&models.DatabaseCleanupTask{}).Error; err != nil {
			return fmt.Errorf("delete completed database cleanup task %d: %w", claimed.ID, err)
		}
		return nil
	})
}

func (s *DatabaseCleanupService) rejectCleanupForExistingInstance(ctx context.Context, db *gorm.DB, task *models.DatabaseCleanupTask) error {
	if task.DatabaseOwned {
		var databaseCount int64
		if err := db.WithContext(ctx).Model(&models.DatabaseInstance{}).
			Where("engine = ? AND name = ? AND status <> ?", task.Engine, task.Name, models.DBStatusDeleted).
			Count(&databaseCount).Error; err != nil {
			return fmt.Errorf("verify database cleanup ownership: %w", err)
		}
		if databaseCount > 0 {
			return errors.New("refusing provisioning cleanup for an existing database identity")
		}
	}
	if task.UserOwned {
		var userCount int64
		if err := db.WithContext(ctx).Model(&models.DatabaseInstance{}).
			Where("engine = ? AND username = ? AND status <> ?", task.Engine, task.Username, models.DBStatusDeleted).
			Count(&userCount).Error; err != nil {
			return fmt.Errorf("verify database user cleanup ownership: %w", err)
		}
		if userCount > 0 {
			return errors.New("refusing provisioning cleanup for an existing database user identity")
		}
	}
	return nil
}

func checkpointDatabaseCleanupOwnership(ctx context.Context, db *gorm.DB, task *models.DatabaseCleanupTask, databaseOwned, userOwned bool) error {
	if err := db.WithContext(ctx).Model(task).Where("lease_token = ?", task.LeaseToken).Updates(map[string]any{
		"database_owned": databaseOwned,
		"user_owned":     userOwned,
	}).Error; err != nil {
		return fmt.Errorf("checkpoint database cleanup task %d: %w", task.ID, err)
	}
	return nil
}

func (s *DatabaseCleanupService) releaseClaimAfterFailure(ctx context.Context, db *gorm.DB, task *models.DatabaseCleanupTask, remaining infrastructure.ProvisioningOwnership, cleanupErr error) error {
	if updateErr := db.WithContext(ctx).Model(task).
		Where("lease_token = ?", task.LeaseToken).
		Updates(map[string]any{
			"database_owned":   remaining.DatabaseCreated,
			"user_owned":       remaining.UserCreated,
			"last_error":       cleanupErr.Error(),
			"retry_count":      gorm.Expr("retry_count + ?", 1),
			"lease_token":      "",
			"lease_expires_at": nil,
		}).Error; updateErr != nil {
		return errors.Join(fmt.Errorf("cleanup database task %d: %w", task.ID, cleanupErr), fmt.Errorf("record database cleanup retry %d: %w", task.ID, updateErr))
	}
	return fmt.Errorf("cleanup database task %d: %w", task.ID, cleanupErr)
}

// WithDatabaseCleanupIdentityLock serializes provisioning and destructive cleanup
// by database identity and database-user identity across backend instances.
func WithDatabaseCleanupIdentityLock(ctx context.Context, db *gorm.DB, engine, name, username string, fn func(*gorm.DB) error) error {
	if db == nil || fn == nil {
		return fmt.Errorf("database cleanup identity lock is unavailable")
	}
	if !isPostgreSQL(db) {
		return fn(db.WithContext(ctx))
	}
	identities := []string{
		fmt.Sprintf("runara:database-cleanup:database:%s:%s", engine, name),
		fmt.Sprintf("runara:database-cleanup:user:%s:%s", engine, username),
	}
	return db.WithContext(ctx).Connection(func(connection *gorm.DB) (lockErr error) {
		for _, identity := range identities {
			if err := connection.Exec("SELECT pg_advisory_lock(hashtextextended(?, 0))", identity).Error; err != nil {
				return fmt.Errorf("acquire database cleanup identity lock: %w", err)
			}
		}
		defer func() {
			for index := len(identities) - 1; index >= 0; index-- {
				if err := connection.Exec("SELECT pg_advisory_unlock(hashtextextended(?, 0))", identities[index]).Error; err != nil && lockErr == nil {
					lockErr = fmt.Errorf("release database cleanup identity lock: %w", err)
				}
			}
		}()
		return fn(connection)
	})
}

func isPostgreSQL(db *gorm.DB) bool {
	return db != nil && db.Dialector.Name() == "postgres"
}

func DeprovisionStandaloneDatabase(engine, name, username string, ownership infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
	remaining := ownership
	if ownership.DatabaseCreated {
		if err := deprovisionStandaloneDatabaseResource(engine, name, username, infrastructure.ProvisioningOwnership{DatabaseCreated: true}); err != nil {
			return remaining, err
		}
		remaining.DatabaseCreated = false
	}
	if ownership.UserCreated {
		if err := deprovisionStandaloneDatabaseResource(engine, name, username, infrastructure.ProvisioningOwnership{UserCreated: true}); err != nil {
			return remaining, err
		}
		remaining.UserCreated = false
	}
	return remaining, nil
}

func deprovisionStandaloneDatabaseResource(engine, name, username string, ownership infrastructure.ProvisioningOwnership) error {
	if engine == "postgresql" {
		return infrastructure.NewPostgreSQLService().DropDatabaseCustomOwned(name, username, ownership)
	}
	return infrastructure.NewMySQLService().DropDatabaseCustomOwned(name, username, ownership)
}
