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

	_, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "stop")
	return err
}

// StartProject enqueues a start job to the worker queue
func (s *ProjectService) StartProject(project *models.Project) error {
	project.Status = models.StatusStarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		return err
	}

	_, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "start")
	return err
}

// RestartProject enqueues a restart job to the worker queue
func (s *ProjectService) RestartProject(project *models.Project) error {
	project.Status = models.StatusRestarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		slog.Error("Failed to update status to restarting", "id", project.ID, "error", err)
	}

	_, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "restart")
	return err
}
