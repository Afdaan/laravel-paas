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

	logs, err := s.dockerService.GetLogs(*containerID, lines*4)
	if err != nil {
		return "", err
	}

	webLogs := filterWorkerRuntimeLines(logs)
	appLogs := readLaravelApplicationLogs(*containerID, lines)
	if strings.TrimSpace(appLogs) != "" {
		webLogs = strings.TrimRight(webLogs, "\n") + "\n" + strings.TrimRight(appLogs, "\n")
	}

	return trimLogLines(webLogs, lines), nil
}

func readLaravelApplicationLogs(containerID string, lines int) string {
	if lines <= 0 {
		lines = 100
	}

	res, err := utils.Run(15*time.Second, "docker", "exec", containerID, "sh", "-c",
		fmt.Sprintf(`for f in /var/www/html/storage/logs/laravel.log /app/storage/logs/laravel.log; do if [ -f "$f" ]; then tail -n %d "$f"; exit 0; fi; done`, lines))
	if err != nil {
		return ""
	}

	return utils.StripLogControlSequences(res.Stdout + res.Stderr)
}

func filterWorkerRuntimeLines(logs string) string {
	lines := strings.Split(logs, "\n")
	filtered := make([]string, 0, len(lines))

	for _, line := range lines {
		normalized := strings.ToLower(line)
		if strings.Contains(normalized, "laravel-worker") ||
			strings.Contains(normalized, "queue:work") ||
			strings.Contains(normalized, "storage/logs/worker.log") ||
			strings.Contains(normalized, "storage/logs/laravel-worker.log") ||
			strings.Contains(normalized, "/app/worker.log") ||
			strings.Contains(normalized, "/var/log/worker.log") {
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.Join(filtered, "\n")
}

func trimLogLines(logs string, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 100
	}

	lines := strings.Split(logs, "\n")
	if len(lines) <= maxLines {
		return logs
	}

	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

// GetStats returns container resource usage
func (s *ProjectService) GetStats(containerID string) (*docker.ContainerStats, error) {
	return s.dockerService.GetContainerStats(containerID)
}

// GetAllStats returns resource usage for all containers
func (s *ProjectService) GetAllStats() (map[string]docker.ContainerStats, error) {
	return s.dockerService.GetAllContainerStats()
}
