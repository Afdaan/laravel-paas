package docker

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

// StartWorkerContainer starts a secondary container for background tasks
func (s *DockerService) StartWorkerContainer(project *models.Project, imageName, cpuLimit, memoryLimit string) (string, error) {
	hostPersistentPath := s.storage.GetPersistentHostPath(project)
	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-worker-%s-%d", project.Subdomain, timestamp)

	// Worker containers don't need Traefik labels as they don't handle HTTP traffic
	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--network", models.NetworkName,
		"--restart", "unless-stopped",
		"--cpus", cpuLimit,
		"--memory", memoryLimit,
		"--env-file", filepath.Join(s.storage.GetProjectsHostPath(project.Subdomain), ".env"),
		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),
		imageName,
	}

	// Append custom worker command
	parts := strings.Fields(project.WorkerCommand)
	runArgs = append(runArgs, parts...)

	res, err := utils.Run(1*time.Minute, "docker", runArgs...)
	if err != nil {
		return "", fmt.Errorf("worker start failed: %s", res.Stderr)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// sanitizeBuildError strips internal build system noise from stderr output
// and returns only the lines that are actionable by the end user.
func sanitizeBuildError(stderr string) string {
	noisePatterns := []string{
		"ERRO failed to solve",
		"failed to solve:",
		"unrecognized image format",
		"INFO No package manager",
		"Railpack build failed",
		" ERRO ",
		" INFO ",
		" WARN ",
	}
	var kept []string
	for _, line := range strings.Split(stderr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isNoise := false
		for _, pattern := range noisePatterns {
			if strings.Contains(trimmed, pattern) {
				isNoise = true
				break
			}
		}
		if !isNoise {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) == 0 {
		return "Build failed. Check the build logs for details."
	}
	return strings.Join(kept, "\n")
}
func (s *DockerService) StartExistingImage(project *models.Project, projectDomain string) (string, error) {
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)

	s.storage.EnsurePersistentPath(project)
	hostPersistentPath := s.storage.GetPersistentHostPath(project)

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	routerName := fmt.Sprintf("%s-%d", project.Subdomain, timestamp)
	serviceName := project.Subdomain

	internalPort := "8080"
	if project.Port != nil {
		internalPort = fmt.Sprintf("%d", *project.Port)
	} else if project.Framework == "Laravel" {
		internalPort = "80"
	}

	finalCPUs := models.DefaultDockerCPULimit
	if project.CPULimit != nil {
		finalCPUs = fmt.Sprintf("%.1f", *project.CPULimit)
	}

	finalMemory := models.DefaultDockerMemoryLimit
	if project.MemoryLimit != nil {
		finalMemory = *project.MemoryLimit
	}

	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--network", models.NetworkName,
		"--restart", "unless-stopped",
		"--cpus", finalCPUs,
		"--memory", finalMemory,
		"-e", fmt.Sprintf("PORT=%s", internalPort),
		"--env-file", filepath.Join(s.storage.GetProjectsHostPath(project.Subdomain), ".env"),

		"--label", "traefik.enable=true",
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=%s",
			routerName, project.GetTraefikHostRule(projectDomain)),
		"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", serviceName, internalPort),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=/health", serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=2s", serviceName),

		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),

		imageName,
	}

	res, err := utils.Run(1*time.Minute, "docker", runArgs...)
	if err != nil {
		return "", fmt.Errorf("failed to start container: %s", res.Stderr)
	}

	mainContainerID := strings.TrimSpace(res.Stdout)

	// Restart Worker if needed
	if project.Framework != "Laravel" && project.WorkerCommand != "" {
		slog.Info("Restarting background worker container for existing image", "subdomain", project.Subdomain)
		workerID, err := s.StartWorkerContainer(project, imageName, finalCPUs, finalMemory)
		if err != nil {
			slog.Error("Failed to restart worker container", "subdomain", project.Subdomain, "error", err)
		} else {
			project.WorkerContainerID = &workerID
		}
	}

	return mainContainerID, nil
}

// StopContainer stops a running container
func (s *DockerService) StopContainer(containerID string) error {
	return utils.RunSilent(30*time.Second, "docker", "stop", containerID)
}

// StartContainer starts a stopped container
func (s *DockerService) StartContainer(containerID string) error {
	return utils.RunSilent(30*time.Second, "docker", "start", containerID)
}

// RestartContainer restarts a container
func (s *DockerService) RestartContainer(containerID string) error {
	return utils.RunSilent(30*time.Second, "docker", "restart", containerID)
}

// RemoveContainer stops and removes a container, including associated workers
func (s *DockerService) RemoveContainer(containerID string, workerContainerID *string) error {
	if workerContainerID != nil && *workerContainerID != "" {
		slog.Debug("Removing worker container", "workerID", *workerContainerID)
		_ = utils.RunSilent(30*time.Second, "docker", "stop", *workerContainerID)
		_ = utils.RunSilent(30*time.Second, "docker", "rm", "-f", *workerContainerID)
	}

	if err := utils.RunSilent(30*time.Second, "docker", "stop", containerID); err != nil {
		slog.Warn("Failed to stop container during removal", "containerID", containerID, "error", err)
	}
	if err := utils.RunSilent(30*time.Second, "docker", "rm", "-f", containerID); err != nil {
		slog.Warn("Failed to remove container", "containerID", containerID, "error", err)
	}
	return nil
}

// IsContainerHealthy checks if a container is running and healthy
func (s *DockerService) IsContainerHealthy(containerID string) bool {
	// Check container status via docker inspect
	res, err := utils.Run(5*time.Second, "docker", "inspect", "--format", "{{.State.Health.Status}}", containerID)

	if err != nil {
		// Container doesn't have health check or not running, check if it's at least running
		res, err = utils.Run(5*time.Second, "docker", "inspect", "--format", "{{.State.Running}}", containerID)
		if err != nil {
			return false
		}
		return strings.TrimSpace(res.Stdout) == "true"
	}

	status := strings.TrimSpace(res.Stdout)
	// Docker health status can be: starting, healthy, unhealthy
	// We only return true when it has fully transitioned to "healthy"
	return status == "healthy"
}

// Helper to convert docker memory headers (GiB, MiB, kiB, B) to MB
func parseMemoryBytes(memStr string) float64 {
	// Remove non-alphanumeric chars (except .)
	input := strings.TrimSpace(memStr)
	valueStr := ""
	unit := ""
	// Separate number and unit
	for i, r := range input {
		if (r < '0' || r > '9') && r != '.' {
			valueStr = input[:i]
			unit = strings.TrimSpace(input[i:])
			break
		}
	}
	if valueStr == "" {
		return 0
	}
	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(unit) {
	case "gib", "gb":
		return val * 1024
	case "mib", "mb":
		return val
	case "kib", "kb":
		return val / 1024
	case "b":
		return val / 1024 / 1024
	default:
		return val // Assume already MB or unknown
	}
}

// ExecLaravelCommand runs artisan commands inside container
// ExecProjectCommand runs commands inside container (artisan for Laravel, shell for others)
func (s *DockerService) ExecProjectCommand(project *models.Project, command string) (string, error) {
	containerID := ""
	if project.ContainerID != nil {
		containerID = *project.ContainerID
	}
	if containerID == "" {
		return "", fmt.Errorf("container not running")
	}

	// Split command string into args to avoiding shell injection
	args := strings.Fields(command)

	// Determine the best user to run the command
	// For Laravel (legacy) it's usually www-data
	// For Node.js (railpack) it's often 'node' (UID 1000) or root
	user := "root"
	potentialUsers := []string{"www-data", "node"}
	for _, u := range potentialUsers {
		if res, err := utils.Run(5*time.Second, "docker", "exec", containerID, "id", "-u", u); err == nil && strings.TrimSpace(res.Stdout) != "" {
			user = u
			break
		}
	}

	// Prepare execution args based on framework
	var fullArgs []string
	if project.Framework == "Laravel" {
		// Determine if we should add --force (for migrate/db:seed)
		// and always add --no-interaction to prevent hanging
		baseCmd := ""
		if len(args) > 0 {
			baseCmd = args[0]
		}

		hasNoInteraction := false
		hasForce := false
		for _, arg := range args {
			if arg == "--no-interaction" || arg == "-n" {
				hasNoInteraction = true
			}
			if arg == "--force" {
				hasForce = true
			}
		}

		if !hasNoInteraction {
			args = append(args, "--no-interaction")
		}

		// Auto-add --force for commands that usually require it in production
		if !hasForce && (baseCmd == "migrate" || baseCmd == "db:seed" || strings.HasPrefix(baseCmd, "migrate:")) {
			args = append(args, "--force")
		}

		// Fix permissions as root before running as www-data to avoid "Permission denied" errors in logs/cache
		if user != "root" {
			if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "-R", user+":"+user, "storage", "bootstrap/cache"); err != nil {
				slog.Warn("Failed to fix permissions before command execution", "error", err, "stderr", res.Stderr)
			}
		}

		fullArgs = append([]string{"exec", "-u", user, containerID, "php", "artisan"}, args...)
	} else {
		// For other frameworks (Node.js, Go, etc.), run command directly
		fullArgs = append([]string{"exec", "-u", user, containerID}, args...)
	}

	res, err := utils.Run(2*time.Minute, "docker", fullArgs...)

	output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
	if err != nil {
		if output == "" {
			output = "Command failed with no output"
		}
		return output, fmt.Errorf("command failed: %w", err)
	}

	return output, nil
}
