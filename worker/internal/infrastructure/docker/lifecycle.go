package docker

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

var (
	rxDockerDaemonErr     = regexp.MustCompile(`(?s)docker: Error response from daemon: `)
	rxDockerSeeHelp       = regexp.MustCompile(`(?s)See 'docker run --help'.*`)
	rxDockerRunHelp       = regexp.MustCompile(`(?s)Run 'docker run --help'.*`)
	rxPaasImageWithHash   = regexp.MustCompile(`paas-[a-zA-Z0-9-]+:[a-f0-9]+`)
	rxPaasImageWithLatest = regexp.MustCompile(`paas-[a-zA-Z0-9-]+:latest`)
	rxPaasImageBase       = regexp.MustCompile(`paas-[a-zA-Z0-9-]+`)
	rxStorageProjectsPath = regexp.MustCompile(`/[a-zA-Z0-9_-]+/storage/projects/[a-zA-Z0-9_-]+`)
	rxStorageAppPath      = regexp.MustCompile(`/[a-zA-Z0-9_-]+/storage/app/[a-zA-Z0-9_-]+`)
)

// StartWorkerContainer starts a secondary container for background tasks.
// Architectural Note on Multi-Tenant Isolation:
// To prevent noisy-neighbor attacks and privilege escalation across tenant containers,
// we enforce cgroups v2 resource ceilings (--cpus, --memory), disable swap thrashing (--memory-swap equal to limit),
// prevent SUID escalation (--security-opt=no-new-privileges:true), and restrict process table exhaustion (--pids-limit=250).
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
		"--env-file", filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), ".env"),

		// Standard Runara metadata labels for deterministic container reconciliation and cleanup
		"--label", fmt.Sprintf("paas.project_id=%d", project.ID),
		"--label", fmt.Sprintf("paas.project_subdomain=%s", project.Subdomain),
		"--label", fmt.Sprintf("paas.rollout_created_at=%d", timestamp),
		"--label", "paas.container_role=worker",
	}

	runArgs = append(runArgs, TenantHardeningArgs(memoryLimit)...)

	volumes := []string{
		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),
	}
	if project.DatabaseOption == "sqlite" {
		// Fail-fast: ensure host sqlite file exists before Docker attempts bind-mount.
		if err := s.storage.PrepareSQLiteHostFile(project); err != nil {
			return "", fmt.Errorf("failed to prepare SQLite database file: %w", err)
		}
		// Mount only the sqlite file to preserve Laravel's database/migrations and database/seeders.
		hostSQLiteFile := filepath.Join(hostPersistentPath, "sqlite", "database.sqlite")
		volumes = append(volumes, "-v", fmt.Sprintf("%s:/var/www/html/database/database.sqlite", hostSQLiteFile))
	}
	runArgs = append(runArgs, volumes...)
	runArgs = append(runArgs, imageName)

	// Append custom worker command
	runArgs = append(runArgs, "sh", "-c", project.WorkerCommand)

	res, err := utils.Run(3*time.Minute, "docker", runArgs...)
	if err != nil {
		errMsg := res.Stderr
		if errMsg == "" {
			errMsg = err.Error()
		}
		cleanedErr := sanitizeDockerRunError(errMsg)
		cleanedErr = strings.TrimPrefix(cleanedErr, "failed to start container: ")
		return "", fmt.Errorf("worker start failed: %s", cleanedErr)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// sanitizeBuildError strips internal build system noise from stderr output
// and returns only the lines that are actionable by the end user using regex error classifiers.
func sanitizeBuildError(stderr string) string {
	if msg := utils.NpmRegistryAuthErrorMessage(stderr); msg != "" {
		return msg
	}

	lines := strings.Split(stderr, "\n")

	// Define actionable error patterns
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\[vite\]:\s*Rollup\s*failed`),
		regexp.MustCompile(`(?i)ERROR\s*in\s+`),
		regexp.MustCompile(`(?i)Failed\s*to\s*compile`),
		regexp.MustCompile(`(?i)Your\s*requirements\s*could\s*not\s*be\s*resolved`),
		regexp.MustCompile(`(?i)npm\s+ERR!`),
		regexp.MustCompile(`(?i)npm\s+error`),
		regexp.MustCompile(`(?i)yarn\s+error`),
		regexp.MustCompile(`(?i)command\s+failed\s+with\s+exit\s+code`),
		regexp.MustCompile(`(?i)error\s+TS[0-9]+:`),
		regexp.MustCompile(`(?i)syntax\s+error`),
		regexp.MustCompile(`(?i)fatal:\s+`),
	}

	for i, line := range lines {
		for _, pat := range patterns {
			if pat.MatchString(line) {
				start := i - 1
				if start < 0 {
					start = 0
				}
				end := i + 2
				if end >= len(lines) {
					end = len(lines) - 1
				}

				var contextLines []string
				for idx := start; idx <= end; idx++ {
					trimmed := strings.TrimSpace(lines[idx])
					if trimmed != "" {
						contextLines = append(contextLines, trimmed)
					}
				}
				if len(contextLines) > 0 {
					return strings.Join(contextLines, "\n")
				}
			}
		}
	}

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
	for _, line := range lines {
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
	if len(kept) > 15 {
		kept = kept[len(kept)-15:]
	}
	return strings.Join(kept, "\n")
}

// StartExistingImage deploys a primary web container instance for a project.
// Architectural Note on Multi-Tenant Isolation:
// Web instances are strictly isolated using cgroups v2 resource quotas (--cpus, --memory),
// swap disablement (--memory-swap), process table limits (--pids-limit=250),
// and privilege escalation prevention (--security-opt=no-new-privileges:true). Traefik routing rules
// enforce strict network ingress isolation.
func (s *DockerService) StartExistingImage(project *models.Project, projectDomain string) (string, error) {
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)
	if project.LastCommitHash != "" {
		tagToCheck := fmt.Sprintf("%s:%s", imageName, project.LastCommitHash)
		checkImg, err := exec.Command("docker", "image", "inspect", tagToCheck).Output()
		if err == nil && len(checkImg) > 0 && strings.TrimSpace(string(checkImg)) != "[]" {
			imageName = tagToCheck
			slog.Info("Using specific commit tag image for startup", "tag", tagToCheck)
		}
	}

	s.storage.EnsurePersistentPath(project)
	hostPersistentPath := s.storage.GetPersistentHostPath(project)

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	routerName := fmt.Sprintf("%s-%d", project.Subdomain, timestamp)
	serviceName := project.Subdomain

	detectedPort, detectErr := s.DetectExposedPort(imageName)
	exposure := project.ResolveRuntimeExposure(0)
	if detectErr == nil {
		exposure = project.ResolveRuntimeExposure(detectedPort)
	}
	if exposure.Reason == models.RuntimeExposureReasonImageExpose {
		p := detectedPort
		project.Port = &p
		if s.GetDB() != nil {
			s.GetDB().Model(project).UpdateColumn("port", detectedPort)
		}
	}
	internalPort := fmt.Sprintf("%d", exposure.Port)
	slog.Info("Resolved runtime exposure",
		"subdomain", project.Subdomain,
		"framework", project.Framework,
		"web_facing", exposure.WebFacing,
		"resolved_port", exposure.Port,
		"resolution_reason", exposure.Reason,
		"detected_exposed_port", detectedPort,
		"manual_port_set", project.Port != nil,
	)
	if exposure.Reason == models.RuntimeExposureReasonUnknownFramework {
		return "", fmt.Errorf("internal port could not be resolved; set project internal port to expose this app publicly, or set it to 0 for a private worker-only service")
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
		"--network-alias", "project-" + project.Subdomain,
		"--restart", "unless-stopped",
		"--cpus", finalCPUs,
		"--memory", finalMemory,
		"-e", fmt.Sprintf("PORT=%s", internalPort),
		"-e", "PYTHONUNBUFFERED=1",
		"--env-file", filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), ".env"),
	}

	runArgs = append(runArgs, TenantHardeningArgs(finalMemory)...)
	if exposure.WebFacing {
		runArgs = append(runArgs,
			"--label", "traefik.enable=true",
			"--label", fmt.Sprintf("traefik.http.routers.%s.rule=%s",
				routerName, fmt.Sprintf("Host(`%s.%s`)", project.Subdomain, projectDomain)),
			"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", serviceName, internalPort),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=%s", serviceName, project.GetHealthCheckPath()),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=2s", serviceName),
		)
	} else {
		runArgs = append(runArgs, "--label", "traefik.enable=false")
	}

	volumes := []string{
		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),
	}
	if project.DatabaseOption == "sqlite" {
		// Fail-fast: ensure host sqlite file exists before Docker attempts bind-mount.
		if err := s.storage.PrepareSQLiteHostFile(project); err != nil {
			return "", fmt.Errorf("failed to prepare SQLite database file: %w", err)
		}
		// Mount only the sqlite file to preserve Laravel's database/migrations and database/seeders.
		hostSQLiteFile := filepath.Join(hostPersistentPath, "sqlite", "database.sqlite")
		volumes = append(volumes, "-v", fmt.Sprintf("%s:/var/www/html/database/database.sqlite", hostSQLiteFile))
	}
	runArgs = append(runArgs, volumes...)
	runArgs = append(runArgs, imageName)

	res, err := utils.Run(3*time.Minute, "docker", runArgs...)
	if err != nil {
		errMsg := res.Stderr
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("%s", sanitizeDockerRunError(errMsg))
	}

	mainContainerID := strings.TrimSpace(res.Stdout)

	if project.DatabaseOption == "sqlite" {
		if err := s.EnsureSQLiteFile(mainContainerID); err != nil {
			_ = utils.RunSilent(30*time.Second, "docker", "rm", "-f", mainContainerID)
			return "", fmt.Errorf("failed to initialize SQLite database: %w", err)
		}
	}

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

// CleanupLegacyContainers finds and removes any running or stopped containers for a project that do NOT match the current active container IDs.
// It inspects container metadata labels to enforce a 10-minute grace period protecting in-flight zero-downtime rollouts.
func (s *DockerService) CleanupLegacyContainers(subdomain string, currentWebID string, currentWorkerID *string) {
	s.ReconcileContainers(subdomain, currentWebID, currentWorkerID, nil)
}

// IsContainerHealthy checks if a container is running and healthy
func (s *DockerService) IsContainerHealthy(containerID string) bool {
	res, err := utils.Run(5*time.Second, "docker", "inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}no_health{{end}}", containerID)
	if err != nil {
		return false
	}
	status := strings.TrimSpace(res.Stdout)
	if status == "no_health" || status == "" || strings.Contains(status, "no value") {
		resRun, errRun := utils.Run(5*time.Second, "docker", "inspect", "--format", "{{.State.Running}}::{{.State.Restarting}}", containerID)
		if errRun != nil {
			return false
		}
		parts := strings.Split(strings.TrimSpace(resRun.Stdout), "::")
		if len(parts) == 2 {
			return parts[0] == "true" && parts[1] == "false"
		}
		return false
	}
	return status == "healthy"
}

// parseArtisanMigrationCommand deterministically inspects an artisan command string.
// If the command is an operational database migration or seed command, it enforces non-interactive and force execution flags.
func parseArtisanMigrationCommand(cmdStr string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return cmdStr
	}

	cmdName := fields[0]
	isMigration := false
	switch cmdName {
	case "migrate", "migrate:fresh", "migrate:refresh", "migrate:rollback", "migrate:reset", "db:seed":
		isMigration = true
	}

	if !isMigration {
		return cmdStr
	}

	result := cmdStr
	if !strings.Contains(result, "--no-interaction") && !strings.Contains(result, "-n") {
		result += " --no-interaction"
	}
	if !strings.Contains(result, "--force") {
		result += " --force"
	}
	return result
}

// RunMigrations runs `php artisan migrate --force --no-interaction` inside a specific container.
// Unlike ExecProjectCommand which uses the project's current ContainerID, this method targets
// a specific container (typically a rollout container that hasn't been promoted yet).
// Returns the combined stdout+stderr output and any error encountered.
func (s *DockerService) RunMigrations(containerID string) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("container ID is empty")
	}

	// Detect the correct user to run artisan commands as
	user := "root"
	if res, err := utils.Run(5*time.Second, "docker", "exec", containerID, "id", "-u", "www-data"); err == nil && strings.TrimSpace(res.Stdout) != "" {
		user = "www-data"

		// Fix permissions before running as www-data to prevent storage/cache write errors
		if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "-R", "www-data:www-data", "storage", "bootstrap/cache"); err != nil {
			slog.Warn("Failed to fix permissions before migration", "containerID", containerID, "error", err, "stderr", res.Stderr)
		}
	}

	// Check if --isolated flag is supported by artisan (Laravel 9+)
	useIsolated := false
	if helpRes, errHelp := utils.Run(5*time.Second, "docker", "exec", containerID, "php", "artisan", "migrate", "--help"); errHelp == nil {
		if strings.Contains(helpRes.Stdout, "--isolated") {
			useIsolated = true
			slog.Info("Detected Laravel 9+ with migration isolation support. Appending --isolated flag.", "containerID", containerID)
		}
	}

	// Build the migration command arguments
	migrationArgs := []string{"exec", "-u", user, containerID, "php", "artisan", "migrate", "--force", "--no-interaction"}
	if useIsolated {
		migrationArgs = append(migrationArgs, "--isolated")
	}

	// Run artisan migrate
	// 5-minute timeout accommodates large migrations but prevents indefinite hangs
	res, err := utils.Run(5*time.Minute, "docker", migrationArgs...)

	output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
	if err != nil {
		if output == "" {
			output = "Migration command failed with no output"
		}
		return output, fmt.Errorf("migration failed: %w", err)
	}

	return output, nil
}

// ExecProjectCommand runs commands inside container (artisan for Laravel, direct binary execution for others)
// Tokenized arguments bypass intermediate shells (sh -c) to eliminate command injection vulnerabilities.
func (s *DockerService) ExecProjectCommand(project *models.Project, command string) (string, error) {
	containerID := ""
	if project.ContainerID != nil {
		containerID = *project.ContainerID
	}
	if containerID == "" {
		return "", fmt.Errorf("container not running")
	}

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

	parser := utils.NewSecureCommandParser(true)

	// Prepare execution args based on framework
	var fullArgs []string
	if project.Framework == "Laravel" {
		cmdStr := parseArtisanMigrationCommand(command)
		tokens, err := parser.Tokenize(cmdStr)
		if err != nil {
			return "", fmt.Errorf("command security validation failed: %w", err)
		}

		// Fix permissions as root before running as www-data to avoid "Permission denied" errors in logs/cache
		if user != "root" {
			if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "-R", user+":"+user, "storage", "bootstrap/cache"); err != nil {
				slog.Warn("Failed to fix permissions before command execution", "error", err, "stderr", res.Stderr)
			}
		}

		fullArgs = []string{"exec", "-u", user, containerID, "php", "artisan"}
		fullArgs = append(fullArgs, tokens...)
	} else {
		// For other frameworks (Node.js, Go, etc.), tokenize and run command directly
		tokens, err := parser.Tokenize(command)
		if err != nil {
			return "", fmt.Errorf("command security validation failed: %w", err)
		}
		fullArgs = []string{"exec", "-u", user, containerID}
		fullArgs = append(fullArgs, tokens...)
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

// sanitizeDockerRunError refines raw Docker daemon execution errors to prevent infrastructure leakage.
func sanitizeDockerRunError(stderr string) string {
	if stderr == "" {
		return "failed to start application container"
	}

	// 1. Image not found / pull denied (broken / missing image tag)
	if strings.Contains(stderr, "Unable to find image") || strings.Contains(stderr, "pull access denied") || strings.Contains(stderr, "repository does not exist") {
		return "failed to start container: application build image not found. Please trigger a new clean build first."
	}

	// 2. Port collision
	if strings.Contains(stderr, "port is already allocated") || strings.Contains(stderr, "address already in use") {
		return "failed to start container: network port collision detected."
	}

	// 3. Clean up generic docker prefix and help suffix
	cleaned := stderr
	cleaned = rxDockerDaemonErr.ReplaceAllString(cleaned, "")
	cleaned = rxDockerSeeHelp.ReplaceAllString(cleaned, "")
	cleaned = rxDockerRunHelp.ReplaceAllString(cleaned, "")

	// Strip absolute storage paths to prevent internal host directory disclosure
	cleaned = rxStorageProjectsPath.ReplaceAllString(cleaned, "project-storage")
	cleaned = rxStorageAppPath.ReplaceAllString(cleaned, "project-storage")

	// Strip internal image name tag leaks if any (e.g. paas-porttofolio-hcn5qa:latest)
	cleaned = rxPaasImageWithHash.ReplaceAllString(cleaned, "application-image")
	cleaned = rxPaasImageWithLatest.ReplaceAllString(cleaned, "application-image")
	cleaned = rxPaasImageBase.ReplaceAllString(cleaned, "application-image")

	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return "failed to start container: internal daemon execution error"
	}
	return "failed to start container: " + cleaned
}

// EnsureSQLiteFile ensures the SQLite database file exists inside the container with correct ownership.
// Only touches the sqlite file itself — does NOT recursively chown/chmod /var/www/html/database
// to preserve Laravel's migrations/ and seeders/ directories.
func (s *DockerService) EnsureSQLiteFile(containerID string) error {
	// Ensure the sqlite file exists (it's bind-mounted, but touch is idempotent)
	if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "touch", "/var/www/html/database/database.sqlite"); err != nil {
		return fmt.Errorf("failed to touch sqlite file: %w (stderr: %s)", err, res.Stderr)
	}

	// Only chown/chmod the sqlite file, not the entire database directory
	if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "www-data:www-data", "/var/www/html/database/database.sqlite"); err != nil {
		return fmt.Errorf("failed to chown sqlite file: %w (stderr: %s)", err, res.Stderr)
	}
	if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chmod", "664", "/var/www/html/database/database.sqlite"); err != nil {
		return fmt.Errorf("failed to chmod sqlite file: %w (stderr: %s)", err, res.Stderr)
	}

	return nil
}
