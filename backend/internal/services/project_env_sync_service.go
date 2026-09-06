package services

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

const projectEnvSyncInterval = time.Minute

type ProjectEnvSyncService struct {
	db    *gorm.DB
	redis *infrastructure.RedisService
}

func NewProjectEnvSyncService(db *gorm.DB, redis *infrastructure.RedisService) *ProjectEnvSyncService {
	return &ProjectEnvSyncService{db: db, redis: redis}
}

func RequestProjectEnvSyncTx(tx *gorm.DB, projectID uint) (uint, error) {
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

func (s *ProjectEnvSyncService) Run(ctx context.Context) {
	if s == nil || s.db == nil || s.redis == nil || ctx == nil {
		return
	}
	s.dispatch(ctx)
	ticker := time.NewTicker(projectEnvSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatch(ctx)
		}
	}
}

func (s *ProjectEnvSyncService) dispatch(ctx context.Context) {
	var tasks []models.ProjectEnvSyncTask
	if err := s.db.WithContext(ctx).Where("desired_generation > acknowledged_generation").Order("id ASC").Limit(databaseCleanupBatch).Find(&tasks).Error; err != nil {
		slog.Error("List project environment sync tasks failed", "error", err)
		return
	}
	for _, task := range tasks {
		var project models.Project
		if err := s.db.WithContext(ctx).First(&project, task.ProjectID).Error; err != nil {
			s.recordFailure(ctx, task.ID, err)
			continue
		}
		if _, err := s.redis.EnqueueDeploymentEnvSync(project.ID, project.UserID, task.DesiredGeneration); err != nil {
			s.recordFailure(ctx, task.ID, err)
		}
	}
}

func (s *ProjectEnvSyncService) recordFailure(ctx context.Context, taskID uint, syncErr error) {
	if err := s.db.WithContext(ctx).Model(&models.ProjectEnvSyncTask{}).Where("id = ?", taskID).Updates(map[string]any{"last_error": syncErr.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; err != nil {
		slog.Error("Record project environment sync retry failed", "task_id", taskID, "error", err)
	}
}
