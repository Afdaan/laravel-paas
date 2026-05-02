// ===========================================
// Docker Service
// ===========================================
// Manages Docker containers for student projects
// ===========================================
package infrastructure

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

// DockerService handles all Docker operations
type DockerService struct {
	cfg     *config.Config
	storage *StorageService
}

// ResolveBuildPath picks a safe build root under a project's folder.
// If baseDirectory is invalid or escapes the project path, it falls back to auto-detection.
func (s *DockerService) ResolveBuildPath(projectPath string, baseDirectory string) string {
	if strings.TrimSpace(baseDirectory) == "" {
		return s.GetBuildPath(projectPath)
	}

	clean := filepath.Clean(baseDirectory)
	// Disallow absolute paths and traversal.
	if filepath.IsAbs(clean) || clean == "." || clean == string(os.PathSeparator) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		slog.Warn("Invalid base_directory; falling back to auto-detection", "base_directory", baseDirectory)
		return s.GetBuildPath(projectPath)
	}

	candidate := filepath.Join(projectPath, clean)
	if !utils.IsPathWithinRoot(projectPath, candidate) {
		slog.Warn("base_directory escapes project root; falling back to auto-detection", "base_directory", baseDirectory)
		return s.GetBuildPath(projectPath)
	}

	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}

	return s.GetBuildPath(projectPath)
}

// NewDockerService creates a new Docker service
func NewDockerService(cfg *config.Config, storage *StorageService) *DockerService {
	return &DockerService{
		cfg:     cfg,
		storage: storage,
	}
}

// ===========================================
// Container Operations
// ===========================================

// BuildAndRun builds and starts a container for a project using Railpack
func (s *DockerService) BuildAndRun(project *models.Project, phpVersion, projectDomain string, cpuLimit float64, memoryLimit string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)

	// 1. Determine Build Path (Monorepo Support + Path Traversal Guard)
	buildPath := s.ResolveBuildPath(projectPath, project.BaseDirectory)

	// 2. Prepare Environment
	if err := s.CreateEnvFile(project, projectDomain); err != nil {
		return "", fmt.Errorf("failed to create .env: %w", err)
	}

	imageName := fmt.Sprintf("paas-%s", project.Subdomain)
	logFilePath := filepath.Join(projectPath, "build.log")

	// Set BuildKit host to use the local docker socket
	os.Setenv("BUILDKIT_HOST", "unix:///var/run/docker.sock")

	var err error
	var internalPort string

	// 3. Branching Build Strategy
	if project.Framework == "Laravel" {
		slog.Info("Using legacy Laravel build strategy", "subdomain", project.Subdomain)
		// Default Laravel port is 80 (nginx)
		if internalPort == "" {
			internalPort = "80"
		}
		err = s.legacyLaravelBuild(project, buildPath, imageName, phpVersion, logFilePath)
	} else {
		slog.Info("Using Railpack build strategy", "subdomain", project.Subdomain)
		err = s.railpackBuild(project, buildPath, imageName, logFilePath)
	}

	if err != nil {
		return "", err
	}

	// 3.5. NEW: Dynamic Port Detection from Image Metadata
	// If port hasn't been manually set by user, try to detect it from image metadata
	detectedPort, detectErr := s.DetectExposedPort(imageName)
	if detectErr == nil && detectedPort > 0 {
		slog.Info("Automatically detected exposed port from image", "subdomain", project.Subdomain, "port", detectedPort)
		p := detectedPort
		project.Port = &p
		internalPort = fmt.Sprintf("%d", p)
	}

	// 4. Determine Final Internal Port for Traefik
	if project.Port != nil {
		internalPort = fmt.Sprintf("%d", *project.Port)
	} else if internalPort == "" {
		// Fallback defaults if everything else fails
		if project.Framework == "Laravel" {
			internalPort = "80"
		} else {
			internalPort = "8080" // Safety default
		}
	}

	// 5. Start Web Container
	s.storage.EnsurePersistentPath(project)
	hostPersistentPath := s.storage.GetPersistentHostPath(project)

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	routerName := fmt.Sprintf("%s-%d", project.Subdomain, timestamp)
	serviceName := project.Subdomain

	finalCPUs := models.DefaultDockerCPULimit
	if cpuLimit > 0 {
		finalCPUs = fmt.Sprintf("%.1f", cpuLimit)
	}

	finalMemory := models.DefaultDockerMemoryLimit
	if memoryLimit != "" {
		finalMemory = memoryLimit
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
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)",
			routerName, project.Subdomain, projectDomain),
		"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", serviceName, internalPort),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=/health", serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=2s", serviceName),

		// Standard Volume Mapping
		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),

		imageName,
	}

	// 4.5. Append custom start command if provided
	if project.StartCommand != "" {
		parts := strings.Fields(project.StartCommand)
		runArgs = append(runArgs, parts...)
	}

	res, runErr := utils.Run(1*time.Minute, "docker", runArgs...)
	if runErr != nil {
		return "", apperr.New(500, "DOCKER_RUN_FAILED", fmt.Sprintf("Failed to start container for %s: %s", project.Subdomain, res.Stderr))
	}

	mainContainerID := strings.TrimSpace(res.Stdout)

	// 5. Start Worker Container if needed (for non-Laravel)
	if project.Framework != "Laravel" && project.WorkerCommand != "" {
		slog.Info("Starting background worker container", "subdomain", project.Subdomain, "command", project.WorkerCommand)
		workerID, err := s.StartWorkerContainer(project, imageName, finalCPUs, finalMemory)
		if err != nil {
			slog.Error("Failed to start worker container", "subdomain", project.Subdomain, "error", err)
		} else {
			project.WorkerContainerID = &workerID
		}
	}

	return mainContainerID, nil
}

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

// legacyLaravelBuild handles the proven Dockerfile + Nginx approach
func (s *DockerService) legacyLaravelBuild(project *models.Project, buildPath, imageName, phpVersion, logFilePath string) error {
	// 1. Prepare Dockerfile
	phpSlug := strings.ReplaceAll(phpVersion, ".", "")
	dockerfile := fmt.Sprintf("Dockerfile.php%s.dynamic", phpSlug)
	srcDockerfile := filepath.Join(s.cfg.TemplatesPath, dockerfile)

	if _, err := os.Stat(srcDockerfile); os.IsNotExist(err) {
		dockerfile = fmt.Sprintf("Dockerfile.php%s", phpSlug)
		srcDockerfile = filepath.Join(s.cfg.TemplatesPath, dockerfile)
	}

	dstDockerfile := filepath.Join(buildPath, "Dockerfile")
	if err := s.storage.CopyFile(srcDockerfile, dstDockerfile); err != nil {
		return fmt.Errorf("failed to copy Dockerfile: %w", err)
	}

	// 2. Prepare Nginx and Supervisor Configs
	dockerDir := filepath.Join(buildPath, "docker")
	os.MkdirAll(dockerDir, 0755)

	s.storage.CopyFile(filepath.Join(s.cfg.TemplatesPath, "nginx.conf"), filepath.Join(dockerDir, "nginx.conf"))
	s.storage.CopyFile(filepath.Join(s.cfg.TemplatesPath, "supervisord.conf"), filepath.Join(dockerDir, "supervisord.conf"))

	// 3. Handle Queue Worker if enabled
	if project.QueueEnabled {
		workerConfig := `
[program:laravel-worker]
process_name=%(program_name)s_%(process_num)02d
command=/usr/local/bin/php /var/www/html/artisan queue:work --sleep=3 --tries=3 --max-time=3600
autostart=true
autorestart=true
user=www-data
numprocs=1
redirect_stderr=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
`
		f, _ := os.OpenFile(filepath.Join(dockerDir, "supervisord.conf"), os.O_APPEND|os.O_WRONLY, 0644)
		if f != nil {
			f.WriteString(workerConfig)
			f.Close()
		}
	}

	// 4. Build using Docker Buildx
	res, err := utils.RunWithLog(30*time.Minute, logFilePath, "docker", "buildx", "build", "--load",
		"--label", models.LabelProjectManaged,
		"-t", imageName, s.storage.GetProjectsHostPath(project.Subdomain))

	if err != nil {
		return apperr.New(500, "DOCKER_BUILD_FAILED", fmt.Sprintf("Docker build failed for %s: %s", project.Subdomain, res.Stderr))
	}

	return nil
}

// railpackBuild handles all other languages using Nixpacks-style auto-detection
func (s *DockerService) railpackBuild(project *models.Project, buildPath, imageName, logFilePath string) error {
	s.injectDefaultRailpackConfig(buildPath, project.RuntimeImage)

	// Optimization: Inject env vars to limit parallelism and use caching
	// Use project ID as cache key for better isolation but still allowing layer reuse
	cacheKey := fmt.Sprintf("project-%d", project.ID)

	buildArgs := []string{
		"build",
		"--name", imageName,
		"--cache-key", cacheKey,
		"--env", "NPM_CONFIG_JOBS=2",
		"--env", "CI=true",
	}

	// Inject Node Version if specified
	if project.NodeVersion != "" {
		buildArgs = append(buildArgs, "--env", fmt.Sprintf("NIXPACKS_NODE_VERSION=%s", project.NodeVersion))
	}

	// Inject custom Build Command if specified
	if project.BuildCommand != "" {
		buildArgs = append(buildArgs, "--env", fmt.Sprintf("NIXPACKS_BUILD_CMD=%s", project.BuildCommand))
	}

	// Finalize build command with path
	buildArgs = append(buildArgs, buildPath)

	res, err := utils.RunWithLog(30*time.Minute, logFilePath, "railpack", buildArgs...)

	if err != nil {
		return apperr.New(500, "RAILPACK_BUILD_FAILED", fmt.Sprintf("Railpack build failed for %s: %s", project.Subdomain, res.Stderr))
	}

	return nil
}

// StartExistingImage starts a container from an already built image.
func (s *DockerService) StartExistingImage(project *models.Project, projectDomain string) (string, error) {
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)

	s.storage.EnsurePersistentPath(project)
	hostPersistentPath := s.storage.GetPersistentHostPath(project)

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	routerName := fmt.Sprintf("%s-%d", project.Subdomain, timestamp)
	serviceName := project.Subdomain

	internalPort := "8080"
	if project.Framework == "Laravel" {
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
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)",
			routerName, project.Subdomain, projectDomain),
		"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", serviceName, internalPort),

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

// RunMigrations executes artisan migrate inside the container
func (s *DockerService) RunMigrations(containerID string) (string, error) {
	// Wait a bit longer for the container and its internal services to fully initialize
	time.Sleep(5 * time.Second)

	// Try running migration. We remove -u www-data to use the image's default user
	// which is safer across different base images (Alpine vs Debian vs Custom)
	res, err := utils.Run(2*time.Minute, "docker", "exec", containerID, "php", "artisan", "migrate", "--force")

	if err != nil {
		output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
		if output == "" {
			output = "(No output from migration command)"
		}
		return output, fmt.Errorf("migration failed: %s", strings.TrimSpace(res.Stderr))
	}

	return res.Stdout, nil
}

// injectDefaultRailpackConfig writes a default railpack.json if one doesn't exist.
// This uses template files from /app/docker/templates/
func (s *DockerService) injectDefaultRailpackConfig(buildPath string, runtimeImage string) {
	railpackConfigPath := filepath.Join(buildPath, "railpack.json")

	// Don't override if user already has one
	if _, err := os.Stat(railpackConfigPath); err == nil {
		slog.Info("User railpack.json found, skipping default injection", "path", railpackConfigPath)
		return
	}

	// Default to alpine if not specified. Only allow known templates.
	if runtimeImage == "" {
		runtimeImage = "alpine"
	}
	switch runtimeImage {
	case "alpine", "debian":
		// ok
	default:
		slog.Warn("Invalid runtime image; falling back to alpine", "runtime", runtimeImage)
		runtimeImage = "alpine"
	}

	// Load template from file system
	templatePath := filepath.Join("/app/docker/templates", fmt.Sprintf("railpack.%s.json", runtimeImage))

	// Fallback to local path for dev
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		templatePath = filepath.Join("docker/templates", fmt.Sprintf("railpack.%s.json", runtimeImage))
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		slog.Warn("Failed to read railpack template, using fallback", "path", templatePath, "error", err)

		// Ultimate fallback if files are missing
		fallback := `{
  "deploy": {
    "base": {
      "image": "alpine:3.20"
    }
  }
}`
		data = []byte(fallback)
	}

	if err := os.WriteFile(railpackConfigPath, data, 0644); err != nil {
		slog.Warn("Failed to inject default railpack.json", "error", err)
		return
	}

	slog.Info("Injected default railpack.json", "runtime", runtimeImage, "path", buildPath)
}

// DetectExposedPort inspects a docker image to find the first EXPOSEd port.
// Returns the port number and nil if found, or 0 and error if not.
func (s *DockerService) DetectExposedPort(imageName string) (int, error) {
	// docker image inspect <image> --format '{{json .Config.ExposedPorts}}'
	res, err := utils.Run(10*time.Second, "docker", "image", "inspect", imageName, "--format", "{{json .Config.ExposedPorts}}")
	if err != nil {
		return 0, err
	}

	output := strings.TrimSpace(res.Stdout)
	if output == "null" || output == "" || output == "{}" {
		return 0, fmt.Errorf("no exposed ports found in image metadata")
	}

	// Output format: {"3000/tcp":{},"80/tcp":{}}
	var exposed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exposed); err != nil {
		return 0, err
	}

	// Pick the first port we find
	for portKey := range exposed {
		// portKey is something like "3000/tcp"
		parts := strings.Split(portKey, "/")
		if len(parts) > 0 {
			var port int
			fmt.Sscanf(parts[0], "%d", &port)
			if port > 0 {
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("could not parse port from metadata")
}

func (s *DockerService) CreateEnvFile(project *models.Project, projectDomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)
	examplePath := filepath.Join(projectPath, ".env.example")
	envPath := filepath.Join(projectPath, ".env")

	// 1. Load mandatory variables from template
	mandatory, err := s.loadMandatoryEnv(project, projectDomain)
	if err != nil {
		slog.Error("Failed to load mandatory env template, falling back to basic config", "error", err)
		// Basic fallback if template is missing
		mandatory = map[string]string{
			"APP_NAME":     fmt.Sprintf("\"%s\"", project.Name),
			"DATABASE_URL": fmt.Sprintf("mysql://%s:%s@paas-mysql:3306/%s", project.DatabaseName, project.DatabasePassword, project.DatabaseName),
		}
	}

	var finalLines []string
	seen := make(map[string]bool)

	// 2. Load from .env.example if it exists
	if data, err := os.ReadFile(examplePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				finalLines = append(finalLines, line)
				continue
			}

			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				if val, ok := mandatory[key]; ok {
					finalLines = append(finalLines, fmt.Sprintf("%s=%s", key, val))
					seen[key] = true
				} else {
					finalLines = append(finalLines, line)
				}
			} else {
				finalLines = append(finalLines, line)
			}
		}
	}

	// 3. Add any missing mandatory variables
	for key, val := range mandatory {
		if !seen[key] {
			finalLines = append(finalLines, fmt.Sprintf("%s=%s", key, val))
		}
	}

	return os.WriteFile(envPath, []byte(strings.Join(finalLines, "\n")), 0644)
}

// StopContainer stops a running container
func (s *DockerService) StopContainer(containerID string) error {
	return utils.RunSilent(30*time.Second, "docker", "stop", containerID)
}

// StartContainer starts a stopped container
func (s *DockerService) StartContainer(containerID string) error {
	return utils.RunSilent(30*time.Second, "docker", "start", containerID)
}

// RemoveContainer stops and removes a container, including associated workers
func (s *DockerService) RemoveContainer(containerID string, workerContainerID *string) error {
	if workerContainerID != nil && *workerContainerID != "" {
		slog.Debug("Removing worker container", "workerID", *workerContainerID)
		_ = exec.Command("docker", "stop", *workerContainerID).Run()
		_ = exec.Command("docker", "rm", *workerContainerID).Run()
	}

	if err := exec.Command("docker", "stop", containerID).Run(); err != nil {
		slog.Warn("Failed to stop container during removal", "containerID", containerID, "error", err)
	}
	if err := exec.Command("docker", "rm", containerID).Run(); err != nil {
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
	// We accept both "healthy" and "starting" (give it time)
	return status == "healthy" || status == "starting"
}

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	// Try both with and without the paas- prefix in case naming varies
	if err := exec.Command("docker", "rmi", imageName).Run(); err != nil {
		slog.Warn("Failed to remove image", "image", imageName, "error", err)
	}
	return nil
}

// PruneImages removes dangling images and unused project images
func (s *DockerService) PruneImages() error {
	slog.Info("Starting Docker image pruning")

	// 1. Remove dangling images (<none>)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f"); err != nil {
		slog.Warn("Failed to prune dangling images", "error", err)
	}

	// 2. Also remove unused project images (those with our label)
	filter := fmt.Sprintf("label=%s=true", models.LabelProjectManaged)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-a", "-f", "--filter", filter); err != nil {
		slog.Warn("Failed to prune project images", "error", err)
	}

	return nil
}

// CleanupProject removes project files
func (s *DockerService) CleanupProject(subdomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.RemoveAll(projectPath)
}

// GetBuildPath recursively finds the first directory containing project markers
func (s *DockerService) GetBuildPath(root string) string {
	markers := []string{
		"artisan",
		"composer.json",
		"package.json",
		"go.mod",
		"requirements.txt",
		"Gemfile",
		"Cargo.toml",
		"mix.exs",
	}

	// 1. Check root first
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return root
		}
	}

	// 2. If not at root, check first-level subdirectories (dynamic monorepo)
	// We only check 1 level deep to avoid accidental detection of nested vendor/node_modules
	entries, err := os.ReadDir(root)
	if err != nil {
		return root
	}

	// Priority subdirectories for monorepos
	priorityDirs := []string{"backend", "app", "server", "api"}

	for _, pDir := range priorityDirs {
		pPath := filepath.Join(root, pDir)
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(pPath, marker)); err == nil {
				return pPath
			}
		}
	}

	// Fallback to searching all non-hidden directories
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirPath := filepath.Join(root, entry.Name())
			for _, marker := range markers {
				if _, err := os.Stat(filepath.Join(dirPath, marker)); err == nil {
					return dirPath
				}
			}
		}
	}

	return root
}

// ===========================================
// Logs & Stats
// ===========================================

// GetLogs retrieves logs from a specific container ID
func (s *DockerService) GetLogs(containerID string, lines int) (string, error) {
	res, err := utils.Run(15*time.Second, "docker", "logs", "--tail", strconv.Itoa(lines), containerID)
	if err != nil {
		return "", fmt.Errorf("failed to get logs for %s: %s", containerID, res.Stderr)
	}

	return res.Stdout + res.Stderr, nil
}

// ContainerStats represents resource usage
type ContainerStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	MemoryMax  float64 `json:"memory_max_mb"`
}

// DockerStatsJSON represents the raw JSON output from docker stats
type DockerStatsJSON struct {
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
}

// GetContainerStats retrieves container resource usage
func (s *DockerService) GetContainerStats(containerID string) (*ContainerStats, error) {
	res, err := utils.Run(10*time.Second, "docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)

	if err != nil {
		slog.Error("Docker stats request failed", "containerId", containerID, "error", err, "stderr", res.Stderr)
		return nil, fmt.Errorf("docker stats failed: %s", res.Stderr)
	}

	// Output might contain multiple lines if multiple containers match (unlikely here)
	// or just one JSON object.
	output := strings.TrimSpace(res.Stdout)

	if output == "" {
		slog.Warn("Docker stats returned empty output", "containerId", containerID)
		return nil, fmt.Errorf("container not found or not running")
	}

	var rawStats DockerStatsJSON
	if err := json.Unmarshal([]byte(output), &rawStats); err != nil {
		slog.Error("Failed to unmarshal docker stats", "containerId", containerID, "output", output, "error", err)
		return nil, fmt.Errorf("failed to parse docker stats: %v", err)
	}

	stats := &ContainerStats{}

	// 1. Parse CPU (remove % and trim)
	cpuStr := strings.ReplaceAll(rawStats.CPUPerc, "%", "")
	cpuVal, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
	if err != nil {
		slog.Warn("Failed to parse CPU percentage from docker stats", "cpuStr", cpuStr, "error", err)
		stats.CPUPercent = 0
	} else {
		stats.CPUPercent = cpuVal
	}

	// 2. Parse Memory (format: USAGE / LIMIT)
	// Example: "12.5MiB / 1.94GiB"
	parts := strings.Split(rawStats.MemUsage, "/")
	if len(parts) >= 2 {
		stats.MemoryMB = parseMemoryBytes(strings.TrimSpace(parts[0]))
		stats.MemoryMax = parseMemoryBytes(strings.TrimSpace(parts[1]))
	} else {
		slog.Warn("Unexpected memory format in docker stats", "memUsage", rawStats.MemUsage)
	}

	slog.Debug("Parsed container stats", "containerId", containerID, "cpu", stats.CPUPercent, "memoryMB", stats.MemoryMB)

	return stats, nil
}

// GetAllContainerStats retrieves resource usage for all containers
func (s *DockerService) GetAllContainerStats() (map[string]ContainerStats, error) {
	res, err := utils.Run(15*time.Second, "docker", "stats", "--no-stream", "--format", "{{.ID}}|{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}")

	if err != nil {
		slog.Error("Global docker stats request failed", "error", err, "stderr", res.Stderr)
		return nil, fmt.Errorf("docker stats failed: %s", res.Stderr)
	}

	result := make(map[string]ContainerStats)
	output := res.Stdout
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		containerID := parts[0]
		// containerName := parts[1]
		cpuPerc := parts[2]
		memUsage := parts[3]

		stats := ContainerStats{}

		// Parse CPU
		cpuStr := strings.ReplaceAll(cpuPerc, "%", "")
		if val, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64); err == nil {
			stats.CPUPercent = val
		}

		// Parse Memory (Usage / Limit)
		memParts := strings.Split(memUsage, "/")
		if len(memParts) >= 2 {
			stats.MemoryMB = parseMemoryBytes(strings.TrimSpace(memParts[0]))
			stats.MemoryMax = parseMemoryBytes(strings.TrimSpace(memParts[1]))
		}

		result[containerID] = stats
	}

	return result, nil
}

// ===========================================
// System & Global Docker Info
// ===========================================

// GetSystemStats retrieves host machine resource usage with robust detection
func (s *DockerService) GetSystemStats() (*models.SystemStats, error) {
	stats := &models.SystemStats{
		DiskPath: s.cfg.ProjectsPath,
		OS:       "Linux",
		CPUCores: 1,
	}

	// 1. Hostname
	if hostname, err := os.Hostname(); err == nil {
		stats.Hostname = hostname
	}

	// 2. OS Distribution Detection
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				stats.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}

	// 3. Docker Mode Detection
	if _, err := os.Stat("/.dockerenv"); err == nil {
		stats.IsDocker = true
	} else if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd") {
			stats.IsDocker = true
		}
	}

	// 4. CPU Usage (Using /proc/stat - two samples for accuracy)
	readCpuStat := func() (uint64, uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) < 5 {
					return 0, 0
				}
				user, _ := strconv.ParseUint(fields[1], 10, 64)
				nice, _ := strconv.ParseUint(fields[2], 10, 64)
				system, _ := strconv.ParseUint(fields[3], 10, 64)
				idle, _ := strconv.ParseUint(fields[4], 10, 64)
				iowait, _ := strconv.ParseUint(fields[5], 10, 64)
				irq, _ := strconv.ParseUint(fields[6], 10, 64)
				softirq, _ := strconv.ParseUint(fields[7], 10, 64)
				total := user + nice + system + idle + iowait + irq + softirq
				return total, idle
			}
		}
		return 0, 0
	}

	t1, i1 := readCpuStat()
	time.Sleep(200 * time.Millisecond)
	t2, i2 := readCpuStat()

	if t2 > t1 {
		totalDiff := float64(t2 - t1)
		idleDiff := float64(i2 - i1)
		stats.CPUUsage = (totalDiff - idleDiff) * 100.0 / totalDiff
	}

	// 5. CPU Cores
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		stats.CPUCores = strings.Count(string(data), "processor")
	}

	// 6. Memory Usage (Using /proc/meminfo)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available, free, buffers, cached uint64
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimSuffix(fields[0], ":")
			val, _ := strconv.ParseUint(fields[1], 10, 64)
			switch key {
			case "MemTotal":
				total = val * 1024
			case "MemFree":
				free = val * 1024
			case "MemAvailable":
				available = val * 1024
			case "Buffers":
				buffers = val * 1024
			case "Cached":
				cached = val * 1024
			}
		}
		if total > 0 {
			if available == 0 {
				available = free + buffers + cached
			}
			stats.MemoryTotal = total
			stats.MemoryUsed = total - available
		}
	}

	// 7. Disk Usage
	if res, err := utils.Run(5*time.Second, "df", "-b", s.cfg.ProjectsPath); err == nil {
		lines := strings.Split(res.Stdout, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 3 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				used, _ := strconv.ParseUint(fields[2], 10, 64)
				stats.DiskTotal = total
				stats.DiskUsed = used
			}
		}
	}

	// 8. Docker Version
	if res, err := utils.Run(5*time.Second, "docker", "version", "--format", "{{.Server.Version}}"); err == nil {
		stats.DockerVersion = strings.TrimSpace(res.Stdout)
	}

	return stats, nil
}

func (s *DockerService) ListAllContainers() ([]models.DockerContainer, error) {
	// 1. Get stats for info merging in parallel (or just call it once as it's already one call)
	statsMap, _ := s.GetAllContainerStats()

	// 2. Get all container IDs first
	cmdIDs := exec.Command("docker", "ps", "-a", "-q")
	idsOut, err := cmdIDs.Output()
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(idsOut))
	if len(ids) == 0 {
		return []models.DockerContainer{}, nil
	}

	// 3. Batch inspect all containers
	// Format: ID|Names|Image|State|Status|CreatedAt|IPAddress|Ports
	// Note: We use .State.Status for State, and formatted Status string for Status
	// We'll get IP and Ports in a single call
	format := "{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.Status}}|{{.State.Status}}|{{.Created}}|{{range .NetworkSettings.Networks}}{{.IPAddress}},{{end}}|{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{.HostIp}}:{{.HostPort}}->{{$p}},{{end}}{{end}}"
	args := append([]string{"inspect", "--format", format}, ids...)
	cmdInspect := exec.Command("docker", args...)
	output, err := cmdInspect.Output()
	if err != nil {
		return nil, err
	}

	var result []models.DockerContainer
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 8 {
			continue
		}

		id := parts[0]
		// docker inspect names start with /
		name := strings.TrimPrefix(parts[1], "/")

		// Parse creation time (Docker uses ISO8601 for inspect)
		created, _ := time.Parse(time.RFC3339Nano, parts[5])

		container := models.DockerContainer{
			ID:        id[:12], // Shorten ID for consistency with ps
			Names:     []string{name},
			Image:     parts[2],
			State:     parts[3],
			Status:    parts[4],
			CreatedAt: created,
			IPAddress: strings.TrimSuffix(parts[6], ","),
			Ports:     parsePorts(strings.TrimSuffix(parts[7], ",")),
		}

		// Merge stats if available
		shortID := id[:12]
		if stats, ok := statsMap[shortID]; ok {
			container.CPUPercent = stats.CPUPercent
			container.MemoryUsage = stats.MemoryMB
		} else if stats, ok := statsMap[id]; ok {
			container.CPUPercent = stats.CPUPercent
			container.MemoryUsage = stats.MemoryMB
		} else {
			// Try by name
			if stats, ok := statsMap[name]; ok {
				container.CPUPercent = stats.CPUPercent
				container.MemoryUsage = stats.MemoryMB
			}
		}

		result = append(result, container)
	}

	return result, nil
}

// parsePorts converts docker port string to slice
func parsePorts(portStr string) []string {
	if portStr == "" {
		return []string{}
	}
	// Example: "0.0.0.0:80->80/tcp, :::80->80/tcp" or "80/tcp"
	return strings.Split(portStr, ", ")
}

// ListAllImages returns all images on the host
func (s *DockerService) ListAllImages() ([]models.DockerImage, error) {
	// Format: ID|Repo|Tag|Size|CreatedAt
	res, err := utils.Run(10*time.Second, "docker", "images", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}

	// Get used images to mark status
	usedImages := make(map[string]bool)
	cmdUsed := exec.Command("docker", "ps", "-a", "--format", "{{.Image}}")
	outUsed, _ := cmdUsed.Output()
	for _, img := range strings.Split(string(outUsed), "\n") {
		img = strings.TrimSpace(img)
		if img != "" {
			usedImages[img] = true
		}
	}

	var result []models.DockerImage
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		repo := parts[1]
		tag := parts[2]

		status := "Unused"
		// Match against full name or ID
		if usedImages[repo] || usedImages[repo+":"+tag] || usedImages[parts[0]] {
			status = "In Use"
		}

		result = append(result, models.DockerImage{
			ID:         parts[0],
			Repository: repo,
			Tag:        tag,
			SizeHuman:  parts[3],
			Status:     status,
		})
	}

	return result, nil
}

// ListAllNetworks returns all networks on the host
func (s *DockerService) ListAllNetworks() ([]models.DockerNetwork, error) {
	// Format: ID|Name|Driver|Scope
	res, err := utils.Run(10*time.Second, "docker", "network", "ls", "--format", "{{.ID}}|{{.Name}}|{{.Driver}}|{{.Scope}}")
	if err != nil {
		return nil, err
	}

	// Get used networks
	usedNets := make(map[string]bool)
	cmdUsed := exec.Command("docker", "ps", "-a", "--format", "{{.Networks}}")
	outUsed, _ := cmdUsed.Output()
	for _, netLine := range strings.Split(string(outUsed), "\n") {
		for _, net := range strings.Split(netLine, ",") {
			usedNets[strings.TrimSpace(net)] = true
		}
	}

	var result []models.DockerNetwork
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		name := parts[1]
		status := "Unused"
		if usedNets[name] {
			status = "In Use"
		}

		result = append(result, models.DockerNetwork{
			ID:     parts[0],
			Name:   name,
			Driver: parts[2],
			Scope:  parts[3],
			Status: status,
		})
	}

	return result, nil
}

// ListAllVolumes returns all volumes on the host
func (s *DockerService) ListAllVolumes() ([]models.DockerVolume, error) {
	// Format: Name|Driver|Mountpoint
	res, err := utils.Run(10*time.Second, "docker", "volume", "ls", "--format", "{{.Name}}|{{.Driver}}|{{.Mountpoint}}")
	if err != nil {
		return nil, err
	}

	var result []models.DockerVolume
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		result = append(result, models.DockerVolume{
			Name:       parts[0],
			Driver:     parts[1],
			Mountpoint: parts[2],
			Status:     "Active", // Volume status is harder to determine simply
		})
	}

	return result, nil
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
func (s *DockerService) ExecLaravelCommand(containerID, command string) (string, error) {
	// Split command string into args to avoiding shell injection
	// This assumes the command is a space-separated list of args for artisan
	// e.g. "migrate --force" -> ["migrate", "--force"]
	args := strings.Fields(command)

	fullArgs := append([]string{"exec", containerID, "php", "artisan"}, args...)
	res, err := utils.Run(2*time.Minute, "docker", fullArgs...)

	if err != nil {
		output := res.Stdout + "\n" + res.Stderr
		return output, fmt.Errorf("command failed: %s", output)
	}

	return res.Stdout, nil
}

// GetEnvFile reads the .env file for a project
func (s *DockerService) GetEnvFile(subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project
func (s *DockerService) SaveEnvFile(subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(content), 0644)
}

// loadMandatoryEnv renders the default.env template and returns a map of key-values.
func (s *DockerService) loadMandatoryEnv(project *models.Project, projectDomain string) (map[string]string, error) {
	templatePath := filepath.Join("/app/docker/templates/default.env")
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		// Local dev fallback
		templatePath = filepath.Join("docker/templates/default.env")
	}

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	// Prepare template data
	key := make([]byte, 32)
	rand.Read(key)
	appKey := base64.StdEncoding.EncodeToString(key)

	queueConn := "sync"
	if project.QueueEnabled {
		queueConn = "database"
	}

	data := struct {
		ProjectName      string
		Subdomain        string
		Domain           string
		AppKey           string
		DatabaseName     string
		DatabasePassword string
		QueueConnection  string
	}{
		ProjectName:      project.Name,
		Subdomain:        project.Subdomain,
		Domain:           projectDomain,
		AppKey:           "base64:" + appKey,
		DatabaseName:     project.DatabaseName,
		DatabasePassword: project.DatabasePassword,
		QueueConnection:  queueConn,
	}

	tmpl, err := template.New("env").Parse(string(content))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	// Parse rendered template into map
	result := make(map[string]string)
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return result, nil
}
