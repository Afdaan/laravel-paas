package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

var ErrDeploymentBusy = errors.New("deployment already in progress or queued")

// EnqueueGuardedDeployment reserves deployment admission, atomically updates DB state to queued
// with the generated job ID, publishes the job with lock fencing, and compensates on publish failure.
func (s *ProjectService) EnqueueGuardedDeployment(ctx context.Context, projectID, userID uint, jobType string, statusMsg string, initialStatus ...models.ProjectStatus) (string, error) {
	return s.enqueueGuardedDeployment(ctx, projectID, userID, jobType, statusMsg, "", initialStatus...)
}

func (s *ProjectService) EnqueueGuardedDeploymentForCommit(ctx context.Context, projectID, userID uint, jobType, statusMsg, targetCommitHash string) (string, error) {
	return s.enqueueGuardedDeployment(ctx, projectID, userID, jobType, statusMsg, targetCommitHash)
}

func (s *ProjectService) enqueueGuardedDeployment(ctx context.Context, projectID, userID uint, jobType, statusMsg, targetCommitHash string, initialStatus ...models.ProjectStatus) (string, error) {
	expected, err := s.projectRepo.GetByID(projectID)
	if err != nil {
		return "", fmt.Errorf("failed to read project before queueing: %w", err)
	}
	lockToken, err := s.redisService.ReserveDeployment(projectID)
	if err != nil {
		return "", fmt.Errorf("failed to reserve deployment lock: %w", err)
	}
	if lockToken == "" {
		return "", ErrDeploymentBusy
	}
	var restoreStatus []models.ProjectStatus
	if len(initialStatus) > 0 {
		restoreStatus = []models.ProjectStatus{expected.Status, initialStatus[0]}
	}

	jobID := utils.GenerateRandomUID()
	job := &infrastructure.DeploymentJob{
		ProjectID:          projectID,
		UserID:             userID,
		Type:               jobType,
		JobID:              jobID,
		TargetCommitHash:   targetCommitHash,
		PreviousCommitHash: expected.LastCommitHash,
		EnqueuedAt:         time.Now(),
	}

	// 1. Record DB state first under transaction/row-lock with initial status
	if err := s.RequeueDeploymentIfMatch(ctx, expected, jobID, statusMsg, initialStatus...); err != nil {
		_ = s.redisService.ReleaseDeploymentLock(projectID, lockToken)
		return "", fmt.Errorf("failed to queue deployment state in database: %w", err)
	}

	// 2. Publish to Redis with lockToken fencing
	if err := s.redisService.EnqueueReplacingDeploymentJob(job, lockToken); err != nil {
		if infrastructure.IsStaleOwnerError(err) {
			_, _ = s.FailQueuedDeploymentIfUnchanged(ctx, projectID, jobID, "Deployment lock lost or expired before publication", false, restoreStatus...)
			return "", infrastructure.ErrStaleDeploymentOwner
		}
		inQueue, checkErr := s.redisService.HasDeploymentJob(jobID)
		if checkErr == nil && inQueue {
			return jobID, nil
		}
		if checkErr == nil && !inQueue {
			current, readErr := s.projectRepo.GetByID(projectID)
			if readErr == nil && current.DeploymentJobID != nil && *current.DeploymentJobID == jobID && current.DeploymentStatus != models.DepStatusQueued {
				return jobID, nil
			}
			_, _ = s.FailQueuedDeploymentIfUnchanged(ctx, projectID, jobID, "Failed to enqueue deployment to queue", false, restoreStatus...)
		}
		return "", fmt.Errorf("failed to publish deployment to queue: %w", err)
	}

	return jobID, nil
}

// StopProject enqueues a stop job to the worker queue
func (s *ProjectService) StopProject(project *models.Project) error {
	_, err := s.EnqueueGuardedDeployment(context.Background(), project.ID, project.UserID, "stop", "Stopping container", models.StatusStopped)
	if err != nil {
		return err
	}
	project.Status = models.StatusStopped
	return nil
}

// StartProject enqueues a start job to the worker queue
func (s *ProjectService) StartProject(project *models.Project) error {
	_, err := s.EnqueueGuardedDeployment(context.Background(), project.ID, project.UserID, "start", "Starting container", models.StatusStarting)
	if err != nil {
		return err
	}
	project.Status = models.StatusStarting
	return nil
}

// RestartProject enqueues a restart job to the worker queue
func (s *ProjectService) RestartProject(project *models.Project) error {
	_, err := s.EnqueueGuardedDeployment(context.Background(), project.ID, project.UserID, "restart", "Restarting container", models.StatusRestarting)
	if err != nil {
		return err
	}
	project.Status = models.StatusRestarting
	return nil
}
