package project

import (
	"fmt"
	"log/slog"

	"github.com/laravel-paas/backend/internal/models"
)

// StopContainer stops a container
func (s *ProjectService) StopContainer(containerID string) error {
	return s.dockerService.StopContainer(containerID)
}

// StopProject stops both web and worker containers and updates status
func (s *ProjectService) StopProject(project *models.Project) error {
	if project.ContainerID != nil {
		if err := s.dockerService.StopContainer(*project.ContainerID); err != nil {
			slog.Warn("Failed to stop web container", "id", *project.ContainerID, "error", err)
		}
	}
	if project.WorkerContainerID != nil {
		if err := s.dockerService.StopContainer(*project.WorkerContainerID); err != nil {
			slog.Warn("Failed to stop worker container", "id", *project.WorkerContainerID, "error", err)
		}
	}

	project.Status = models.StatusStopped
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
}

// StartProject starts both web and worker containers and updates status
func (s *ProjectService) StartProject(project *models.Project) error {
	if project.ContainerID != nil {
		if err := s.dockerService.StartContainer(*project.ContainerID); err != nil {
			return fmt.Errorf("failed to start web container: %w", err)
		}
	}
	if project.WorkerContainerID != nil {
		if err := s.dockerService.StartContainer(*project.WorkerContainerID); err != nil {
			slog.Warn("Failed to start worker container", "id", *project.WorkerContainerID, "error", err)
		}
	}

	project.Status = models.StatusRunning
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
}

// RestartProject restarts both web and worker containers
func (s *ProjectService) RestartProject(project *models.Project) error {
	project.Status = models.StatusRestarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		slog.Error("Failed to update status to restarting", "id", project.ID, "error", err)
	}

	if err := s.RecreateProjectZeroDowntime(project); err != nil {
		return fmt.Errorf("zero-downtime restart failed: %w", err)
	}

	project.Status = models.StatusRunning
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
}
