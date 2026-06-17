package project

import (
	"log/slog"

	"github.com/laravel-paas/shared/models"
)

// StopProject enqueues a stop job to the worker queue
func (s *ProjectService) StopProject(project *models.Project) error {
	project.Status = models.StatusStopped
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		return err
	}

	jobID, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "stop")
	if err == nil {
		_ = s.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Stopping container", 0, jobID)
	}
	return err
}

// StartProject enqueues a start job to the worker queue
func (s *ProjectService) StartProject(project *models.Project) error {
	project.Status = models.StatusStarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		return err
	}

	jobID, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "start")
	if err == nil {
		_ = s.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Starting container", 0, jobID)
	}
	return err
}

// RestartProject enqueues a restart job to the worker queue
func (s *ProjectService) RestartProject(project *models.Project) error {
	project.Status = models.StatusRestarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		slog.Error("Failed to update status to restarting", "id", project.ID, "error", err)
	}

	jobID, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "restart")
	if err == nil {
		_ = s.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Restarting container", 0, jobID)
	}
	return err
}
