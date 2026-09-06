package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

const projectDeletionDispatchInterval = time.Minute

type DeletionDispatcher struct {
	db    *gorm.DB
	redis *infrastructure.RedisService
}

func NewDeletionDispatcher(db *gorm.DB, redis *infrastructure.RedisService) *DeletionDispatcher {
	return &DeletionDispatcher{db: db, redis: redis}
}

func (d *DeletionDispatcher) Run(ctx context.Context) {
	if d == nil || d.db == nil || d.redis == nil || ctx == nil {
		return
	}
	d.runOnce(ctx)
	ticker := time.NewTicker(projectDeletionDispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.runOnce(ctx)
		}
	}
}

func (d *DeletionDispatcher) runOnce(ctx context.Context) {
	if err := d.DispatchPending(ctx); err != nil {
		slog.Error("Project deletion dispatcher failed", "error", err)
	}
}

func (d *DeletionDispatcher) DispatchPending(ctx context.Context) error {
	var tasks []models.ProjectDeletionTask
	if err := d.db.WithContext(ctx).Order("id ASC").Limit(25).Find(&tasks).Error; err != nil {
		return fmt.Errorf("list project deletion tasks: %w", err)
	}
	var dispatchErr error
	for _, task := range tasks {
		if task.CompletedAt != nil {
			if err := d.db.WithContext(ctx).Delete(&models.ProjectDeletionTask{}, task.ID).Error; err != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("complete acknowledged project deletion %d: %w", task.ID, err))
			}
			continue
		}
		var project models.Project
		if err := d.db.WithContext(ctx).First(&project, task.ProjectID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := d.db.WithContext(ctx).Delete(&models.ProjectDeletionTask{}, task.ID).Error; err != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("complete project deletion %d: %w", task.ID, err))
			}
			continue
		} else if err != nil {
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("load project deletion %d: %w", task.ProjectID, err))
			continue
		}
		if _, err := d.redis.EnqueueDeploymentNonDestructive(task.ProjectID, task.UserID, "delete"); err != nil {
			if updateErr := d.db.WithContext(ctx).Model(&models.ProjectDeletionTask{}).Where("id = ?", task.ID).Updates(map[string]any{
				"last_error":  err.Error(),
				"retry_count": gorm.Expr("retry_count + 1"),
			}).Error; updateErr != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("dispatch project deletion %d: %w", task.ProjectID, err), fmt.Errorf("record project deletion retry %d: %w", task.ID, updateErr))
				continue
			}
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("dispatch project deletion %d: %w", task.ProjectID, err))
			continue
		}
	}
	return dispatchErr
}
