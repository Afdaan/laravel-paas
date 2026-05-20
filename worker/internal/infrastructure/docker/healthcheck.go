// ===========================================
// Advanced Healthcheck Orchestration
// ===========================================
package docker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/models"
)

// AdvancedHealthcheck runs a production readiness probe with exponential backoff and a stabilization monitoring window.
func (s *DockerService) AdvancedHealthcheck(ctx context.Context, project *models.Project, containerID string) error {
	slog.Info("Starting advanced healthcheck probe", "projectId", project.ID, "containerId", containerID)

	// 1. Startup grace period
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	maxAttempts := 10
	currentInterval := 1 * time.Second
	isReady := false

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if s.IsContainerHealthy(containerID) {
			isReady = true
			break
		}

		slog.Debug("Container health check probe pending", "attempt", attempt, "containerId", containerID)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(currentInterval):
		}

		currentInterval *= 2
		if currentInterval > 8*time.Second {
			currentInterval = 8 * time.Second
		}
	}

	if !isReady {
		logs, _ := s.GetLogs(containerID, 50)
		return fmt.Errorf("container failed readiness probe after %d attempts. Logs:\n%s", maxAttempts, logs)
	}

	// 2. Stabilization window
	slog.Info("Container readiness verified, entering stabilization monitoring window (5s)", "containerId", containerID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	if !s.IsContainerHealthy(containerID) {
		logs, _ := s.GetLogs(containerID, 50)
		return fmt.Errorf("container destabilized during stabilization window. Logs:\n%s", logs)
	}

	slog.Info("Advanced healthcheck completed successfully", "containerId", containerID)
	return nil
}

// IsSystemOverloaded checks if host memory or CPU pressure is too high for safe builds.
func (s *DockerService) IsSystemOverloaded() (bool, string) {
	stats, err := s.GetSystemStats()
	if err != nil {
		return false, ""
	}
	if stats.MemoryTotal > 0 {
		memRatio := float64(stats.MemoryUsed) / float64(stats.MemoryTotal)
		if memRatio > 0.85 {
			return true, fmt.Sprintf("high memory pressure: %.1f%%", memRatio*100)
		}
	}
	if stats.CPUUsage > 90.0 {
		return true, fmt.Sprintf("high CPU pressure: %.1f%%", stats.CPUUsage)
	}
	return false, ""
}
