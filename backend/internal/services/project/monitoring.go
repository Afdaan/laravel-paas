package project

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// GetLogs returns container logs (either web or worker)
func (s *ProjectService) GetLogs(project *models.Project, logType string, lines int) (string, error) {
	// Clamp lines to a safe maximum to prevent resource exhaustion (DoS)
	if lines > 2000 {
		lines = 2000
	} else if lines <= 0 {
		lines = 100
	}

	if logType == "worker" {
		// If running in a separate worker container, retrieve standard stdout/stderr logs
		if project.WorkerContainerID != nil && *project.WorkerContainerID != "" {
			return s.dockerService.GetLogs(*project.WorkerContainerID, lines)
		}

		// Otherwise, if running inside the main container (such as under Supervisord or background scripts),
		// dynamically detect the log file inside the container.
		if project.ContainerID != nil && *project.ContainerID != "" {
			res, err := utils.Run(15*time.Second, "docker", "exec", *project.ContainerID, "sh", "-c",
				`for f in /var/www/html/storage/logs/laravel-worker.log /var/www/html/storage/logs/worker.log /app/storage/logs/worker.log /app/worker.log /var/log/worker.log; do if [ -f "$f" ]; then echo "$f"; break; fi; done`)

			if err == nil {
				logPath := strings.TrimSpace(res.Stdout)
				if logPath != "" {
					resTail, errTail := utils.Run(15*time.Second, "docker", "exec", *project.ContainerID, "tail", "-n", strconv.Itoa(lines), logPath)
					if errTail != nil {
						return "", fmt.Errorf("failed to get worker logs: %s", resTail.Stderr)
					}
					return utils.StripLogControlSequences(resTail.Stdout + resTail.Stderr), nil
				}
			}
			return "No worker logs found yet.\n", nil
		}
		return "", fmt.Errorf("no container running for worker logs")
	}

	containerID := project.ContainerID
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
