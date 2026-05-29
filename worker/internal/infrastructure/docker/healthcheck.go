// ===========================================
// Advanced Healthcheck Orchestration
// ===========================================
package docker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// GetContainerIP extracts the private IP of a container on the Docker network
func (s *DockerService) GetContainerIP(containerID string) (string, error) {
	format := fmt.Sprintf("{{(index .NetworkSettings.Networks %q).IPAddress}}", models.NetworkName)
	res, err := utils.Run(5*time.Second, "docker", "inspect", "--format", format, containerID)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(res.Stdout)
	if ip == "" {
		return "", fmt.Errorf("container IP is empty")
	}
	return ip, nil
}

// probeHTTP sends an HTTP GET request to the specified URL to verify status code < 500
func (s *DockerService) probeHTTP(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error status code: %d", resp.StatusCode)
	}
	
	return nil
}

// AdvancedHealthcheck runs a production readiness probe with exponential backoff and a stabilization monitoring window.
func (s *DockerService) AdvancedHealthcheck(ctx context.Context, project *models.Project, containerID string) error {
	slog.Info("Starting advanced healthcheck probe", "projectId", project.ID, "containerId", containerID)

	// Determine if container exposes a web port
	isWebFacing := project.Port == nil || *project.Port > 0

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
			if isWebFacing {
				ip, ipErr := s.GetContainerIP(containerID)
				if ipErr == nil {
					url := fmt.Sprintf("http://%s:%s%s", ip, project.GetInternalPort(), project.GetHealthCheckPath())
					if err := s.probeHTTP(ctx, url); err == nil {
						isReady = true
						break
					} else {
						slog.Warn("Container running but HTTP probe failed", "url", url, "error", err, "attempt", attempt)
					}
				} else {
					slog.Warn("Failed to resolve container IP for HTTP probe", "error", ipErr, "attempt", attempt)
				}
			} else {
				isReady = true
				break
			}
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
		logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
		return fmt.Errorf("[RUNTIME_FAILED] Container process is running but did not respond to HTTP readiness checks. Last logs:\n%s", logs)
	}

	// 2. Stabilization window
	slog.Info("Container readiness verified, entering stabilization monitoring window (5s)", "containerId", containerID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	if !s.IsContainerHealthy(containerID) {
		logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
		return fmt.Errorf("[RUNTIME_FAILED] Container crashed during stabilization window. Last logs:\n%s", logs)
	}

	if isWebFacing {
		ip, _ := s.GetContainerIP(containerID)
		url := fmt.Sprintf("http://%s:%s%s", ip, project.GetInternalPort(), project.GetHealthCheckPath())
		if err := s.probeHTTP(ctx, url); err != nil {
			logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
			return fmt.Errorf("[RUNTIME_FAILED] HTTP probe failed during stabilization window. Error: %w. Last logs:\n%s", err, logs)
		}
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
