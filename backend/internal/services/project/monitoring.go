package project

import (
	"fmt"

	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
)

// GetLogs returns container logs (either web or worker)
func (s *ProjectService) GetLogs(project *models.Project, logType string, lines int) (string, error) {
	containerID := project.ContainerID
	if logType == "worker" {
		containerID = project.WorkerContainerID
	}

	if containerID == nil || *containerID == "" {
		return "", fmt.Errorf("no container running for type %s", logType)
	}

	return s.dockerService.GetLogs(*containerID, lines)
}

// GetStats returns container resource usage
func (s *ProjectService) GetStats(containerID string) (*docker.ContainerStats, error) {
	return s.dockerService.GetContainerStats(containerID)
}

// GetAllStats returns resource usage for all containers
func (s *ProjectService) GetAllStats() (map[string]docker.ContainerStats, error) {
	return s.dockerService.GetAllContainerStats()
}
