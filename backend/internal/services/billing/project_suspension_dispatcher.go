package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const projectSuspensionDispatchInterval = time.Minute

// ProjectSuspensionDispatcher retries stop requests until the project is durably stopped.
type ProjectSuspensionDispatcher struct {
	db            *gorm.DB
	enqueueStop   func(projectID, userID, taskID uint) (string, error)
	enqueueResume func(projectID, userID, taskID uint) (string, error)
}

func NewProjectSuspensionDispatcher(db *gorm.DB, redis *infrastructure.RedisService) *ProjectSuspensionDispatcher {
	if redis == nil {
		return nil
	}
	return newProjectSuspensionDispatcher(db, redis.EnqueueBillingSuspensionStop, redis.EnqueueBillingSuspensionResume)
}

func newProjectSuspensionDispatcher(db *gorm.DB, enqueueStop func(projectID, userID, taskID uint) (string, error), enqueueResume ...func(projectID, userID, taskID uint) (string, error)) *ProjectSuspensionDispatcher {
	dispatcher := &ProjectSuspensionDispatcher{db: db, enqueueStop: enqueueStop}
	if len(enqueueResume) > 0 {
		dispatcher.enqueueResume = enqueueResume[0]
	}
	return dispatcher
}

func (d *ProjectSuspensionDispatcher) Run(ctx context.Context) {
	if d == nil || d.db == nil || d.enqueueStop == nil || ctx == nil {
		return
	}
	d.dispatch(ctx)
	ticker := time.NewTicker(projectSuspensionDispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.dispatch(ctx)
		}
	}
}

func (d *ProjectSuspensionDispatcher) dispatch(ctx context.Context) {
	if err := d.DispatchPending(ctx); err != nil {
		slog.Error("Billing project suspension dispatcher failed", "error", err)
	}
}

func (d *ProjectSuspensionDispatcher) DispatchPending(ctx context.Context) error {
	var tasks []models.ProjectSuspensionTask
	if err := d.db.WithContext(ctx).Where("(completed_at IS NULL AND stop_completed_at IS NULL) OR (resume_requested_at IS NOT NULL AND resume_completed_at IS NULL)").Order("id ASC").Limit(25).Find(&tasks).Error; err != nil {
		return fmt.Errorf("list project suspension tasks: %w", err)
	}
	var dispatchErr error
	for _, task := range tasks {
		if task.ResumeRequestedAt != nil && task.ResumeCompletedAt == nil {
			if err := d.dispatchResume(ctx, task); err != nil {
				dispatchErr = errors.Join(dispatchErr, err)
			}
			continue
		}
		var project models.Project
		err := d.db.WithContext(ctx).First(&project, task.ProjectID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if deleteErr := d.db.WithContext(ctx).Delete(&models.ProjectSuspensionTask{}, task.ID).Error; deleteErr != nil {
				dispatchErr = errors.Join(dispatchErr, deleteErr)
			}
			continue
		}
		if err != nil {
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("load project suspension task %d: %w", task.ID, err))
			continue
		}
		var resource models.BillableResource
		if err := d.db.WithContext(ctx).Where("id = ? AND user_id = ? AND type = ? AND resource_id = ?", task.BillableResourceID, task.UserID, models.BillableTypeProject, task.ProjectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				now := time.Now().UTC()
				if updateErr := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Update("completed_at", &now).Error; updateErr != nil {
					dispatchErr = errors.Join(dispatchErr, fmt.Errorf("complete stale project suspension task %d: %w", task.ID, updateErr))
				}
				continue
			}
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("load billing resource for suspension task %d: %w", task.ID, err))
			continue
		}
		if resource.BillingStatus != models.BillableResourceStatusSuspended {
			now := time.Now().UTC()
			if err := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Update("completed_at", &now).Error; err != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("complete paid project suspension task %d: %w", task.ID, err))
			}
			continue
		}
		if project.Status == models.StatusDeleting {
			now := time.Now().UTC()
			if err := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Update("completed_at", &now).Error; err != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("complete project suspension task %d: %w", task.ID, err))
			}
			continue
		}
		if _, err := d.enqueueStop(project.ID, project.UserID, task.ID); err != nil {
			if updateErr := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Updates(map[string]any{"last_error": err.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; updateErr != nil {
				dispatchErr = errors.Join(dispatchErr, fmt.Errorf("enqueue project suspension %d: %w", task.ID, err), updateErr)
				continue
			}
			dispatchErr = errors.Join(dispatchErr, fmt.Errorf("enqueue project suspension %d: %w", task.ID, err))
		}
	}
	return dispatchErr
}

func (d *ProjectSuspensionDispatcher) dispatchResume(ctx context.Context, task models.ProjectSuspensionTask) error {
	if d.enqueueResume == nil {
		return fmt.Errorf("billing project resume queue unavailable")
	}
	var project models.Project
	err := d.db.WithContext(ctx).First(&project, task.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.completeResumeTask(ctx, task.ID)
	}
	if err != nil {
		return fmt.Errorf("load project billing resume task %d: %w", task.ID, err)
	}
	var resource models.BillableResource
	err = d.db.WithContext(ctx).Where("id = ? AND user_id = ? AND type = ? AND resource_id = ?", task.BillableResourceID, task.UserID, models.BillableTypeProject, task.ProjectID).First(&resource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.completeResumeTask(ctx, task.ID)
	}
	if err != nil {
		return fmt.Errorf("load billing resource for resume task %d: %w", task.ID, err)
	}
	if resource.BillingStatus == models.BillableResourceStatusSuspended {
		return d.reopenSuspendedTask(ctx, task, resource)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		return d.completeResumeTask(ctx, task.ID)
	}
	if !task.MainWasRunning && !task.WorkerWasRunning {
		return d.completeResumeTask(ctx, task.ID)
	}
	if _, err := d.enqueueResume(project.ID, project.UserID, task.ID); err != nil {
		if updateErr := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Updates(map[string]any{"last_error": err.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; updateErr != nil {
			return errors.Join(fmt.Errorf("enqueue project billing resume %d: %w", task.ID, err), updateErr)
		}
		return fmt.Errorf("enqueue project billing resume %d: %w", task.ID, err)
	}
	return nil
}

func (d *ProjectSuspensionDispatcher) reopenSuspendedTask(ctx context.Context, task models.ProjectSuspensionTask, resource models.BillableResource) error {
	if d.enqueueStop == nil {
		return fmt.Errorf("billing project suspension queue unavailable")
	}

	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedResource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND type = ? AND resource_id = ?", resource.ID, resource.UserID, models.BillableTypeProject, task.ProjectID).First(&lockedResource).Error; err != nil {
			return err
		}
		if lockedResource.BillingStatus != models.BillableResourceStatusSuspended {
			return nil
		}
		var lockedTask models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND resume_requested_at IS NOT NULL AND resume_completed_at IS NULL", task.ID, task.ProjectID, resource.ID, resource.UserID).First(&lockedTask).Error; err != nil {
			return err
		}
		return tx.Model(&lockedTask).UpdateColumns(map[string]any{
			"completed_at":        nil,
			"stop_completed_at":   nil,
			"resume_requested_at": nil,
			"resume_completed_at": nil,
			"last_error":          "",
		}).Error
	})
	if err != nil {
		return fmt.Errorf("reopen renewed project suspension %d: %w", task.ID, err)
	}
	if _, err := d.enqueueStop(task.ProjectID, task.UserID, task.ID); err != nil {
		if updateErr := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ?", task.ID).Updates(map[string]any{"last_error": err.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; updateErr != nil {
			return errors.Join(fmt.Errorf("enqueue renewed project suspension %d: %w", task.ID, err), updateErr)
		}
		return fmt.Errorf("enqueue renewed project suspension %d: %w", task.ID, err)
	}
	return nil
}

func (d *ProjectSuspensionDispatcher) completeResumeTask(ctx context.Context, taskID uint) error {
	now := time.Now().UTC()
	if err := d.db.WithContext(ctx).Model(&models.ProjectSuspensionTask{}).Where("id = ? AND resume_completed_at IS NULL", taskID).Update("resume_completed_at", &now).Error; err != nil {
		return fmt.Errorf("complete project billing resume %d: %w", taskID, err)
	}
	return nil
}
