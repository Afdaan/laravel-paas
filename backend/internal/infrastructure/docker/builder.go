package docker

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

// ===========================================
// Container Operations
// ===========================================
// BuildAndRun builds and starts a container for a project using Railpack
func (s *DockerService) BuildAndRun(project *models.Project, phpVersion, projectDomain string, cpuLimit float64, memoryLimit string, isInitial bool, noCache bool) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)

	// 1. Determine Build Path (Monorepo Support + Path Traversal Guard)
	buildPath := s.ResolveBuildPath(projectPath, project.BaseDirectory)

	// 2. Prepare Environment
	if err := s.CreateEnvFile(project, projectDomain, isInitial); err != nil {
		return "", fmt.Errorf("failed to create .env: %w", err)
	}

	// 2.1 Sync .env to buildPath for monorepo support.
	// This ensures that build-time environment variables are available to frameworks
	// (like Vite or Next.js) when the source code is in a subdirectory.
	if buildPath != projectPath {
		sourceEnv := filepath.Join(projectPath, ".env")
		targetEnv := filepath.Join(buildPath, ".env")

		if err := s.storage.CopyFile(sourceEnv, targetEnv); err != nil {
			slog.Warn("Failed to sync .env to build directory",
				"subdomain", project.Subdomain,
				"target", buildPath,
				"error", err)
		}
	}

	// 2.2 Inject .dockerignore if missing to optimize build context size
	s.injectDockerIgnore(projectPath)

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
		err = s.legacyLaravelBuild(project, buildPath, imageName, phpVersion, logFilePath, noCache)
	} else {
		slog.Info("Using Railpack build strategy", "subdomain", project.Subdomain)
		err = s.railpackBuild(project, buildPath, imageName, logFilePath, noCache)
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
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=%s",
			routerName, project.GetTraefikHostRule(projectDomain)),
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

// legacyLaravelBuild handles the proven Dockerfile + Nginx approach
func (s *DockerService) legacyLaravelBuild(project *models.Project, buildPath, imageName, phpVersion, logFilePath string, noCache bool) error {
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
	if err := os.MkdirAll(dockerDir, 0755); err != nil {
		return fmt.Errorf("failed to create docker config directory: %w", err)
	}

	if err := s.storage.CopyFile(filepath.Join(s.cfg.TemplatesPath, "nginx.conf"), filepath.Join(dockerDir, "nginx.conf")); err != nil {
		return fmt.Errorf("failed to copy nginx.conf: %w", err)
	}
	if err := s.storage.CopyFile(filepath.Join(s.cfg.TemplatesPath, "supervisord.conf"), filepath.Join(dockerDir, "supervisord.conf")); err != nil {
		return fmt.Errorf("failed to copy supervisord.conf: %w", err)
	}

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
			if _, err := f.WriteString(workerConfig); err != nil {
				slog.Warn("Failed to write worker config to supervisord.conf", "error", err)
			}
			f.Close()
		}
	}

	// 4. Build using Docker Buildx
	buildArgs := []string{"buildx", "build", "--load"}
	if noCache {
		buildArgs = append(buildArgs, "--no-cache")
	}
	if project.BuildCommand != "" {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("BUILD_COMMAND=%s", project.BuildCommand))
	}
	if project.NodeVersion != "" {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("NODE_VERSION=%s", project.NodeVersion))
	}
	buildArgs = append(buildArgs, "--label", models.LabelProjectManaged, "-t", imageName, s.storage.GetProjectsHostPath(project.Subdomain))

	res, err := utils.RunWithRefinedLog(30*time.Minute, logFilePath, "docker", buildArgs...)

	if err != nil {
		return apperr.New(500, "DOCKER_BUILD_FAILED", fmt.Sprintf("Docker build failed for %s: %s", project.Subdomain, res.Stderr))
	}

	return nil
}

// railpackBuild handles all other languages using Nixpacks-style auto-detection
func (s *DockerService) railpackBuild(project *models.Project, buildPath, imageName, logFilePath string, noCache bool) error {
	s.injectDefaultRailpackConfig(buildPath)

	// Strip strict lockfiles before Railpack runs.
	// Railpack hardcodes --frozen-lockfile when it detects bun.lock or yarn.lock,
	// causing builds to fail when the user's lockfile is out of sync with package.json.
	// A pre-sync approach requires the package manager on the host, which is not guaranteed.
	// Deleting the lockfile forces a fresh resolve bounded by package.json semver constraints.
	for _, lockfile := range []string{"bun.lock", "yarn.lock"} {
		if err := os.Remove(filepath.Join(buildPath, lockfile)); err == nil {
			slog.Info("Stripped lockfile to allow non-frozen install", "file", lockfile, "subdomain", project.Subdomain)
		}
	}

	// Use project ID as cache key for better isolation but still allowing layer reuse
	cacheKey := fmt.Sprintf("project-%d", project.ID)
	if noCache {
		cacheKey = fmt.Sprintf("project-%d-%d", project.ID, time.Now().Unix())
	}

	buildArgs := []string{
		"build",
		"--name", imageName,
		"--cache-key", cacheKey,
	}
	// Collect all environment variables in a map to avoid duplicates
	envs := map[string]string{
		"NPM_CONFIG_JOBS":             "2",
		"NPM_CONFIG_LEGACY_PEER_DEPS": "true",
		"PAAS_BUILD_ID":               fmt.Sprintf("%d", time.Now().Unix()),
	}

	// Load environment variables from .env
	projectEnvPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain, ".env")
	if envVars, err := s.parseProjectEnv(projectEnvPath); err == nil {
		for key, value := range envVars {
			envs[key] = value
		}
	}

	// Inject Node Version if specified
	if project.NodeVersion != "" {
		envs["NIXPACKS_NODE_VERSION"] = project.NodeVersion
	}

	// Inject custom Build Command if specified
	if project.BuildCommand != "" {
		envs["NIXPACKS_BUILD_CMD"] = project.BuildCommand
	}

	// Append all envs to buildArgs
	for key, value := range envs {
		buildArgs = append(buildArgs, "--env", fmt.Sprintf("%s=%s", key, value))
	}

	// Finalize build command with path
	buildArgs = append(buildArgs, buildPath)

	res, err := utils.RunWithRefinedLog(30*time.Minute, logFilePath, "railpack", buildArgs...)

	if err != nil {
		// Cleanup: Remove failed image and prune cache if it looks like a corruption issue.
		// We use RunSilent for cleanup to prevent cluttering the main logic with non-fatal errors.
		slog.Warn("Railpack build failed, performing cleanup", "subdomain", project.Subdomain, "error", err)

		_ = utils.RunSilent(time.Minute, "docker", "rmi", imageName)

		if strings.Contains(res.Stderr, "unrecognized image format") || strings.Contains(res.Stderr, "failed to solve") {
			slog.Info("Detected build kit corruption, pruning builder cache and disabling cache for next run", "subdomain", project.Subdomain)
			_ = utils.RunSilent(time.Minute, "docker", "builder", "prune", "-f", "--filter", "until=0s")
		}

		// Extract only the actionable error lines from stderr, discarding internal
		// Railpack/BuildKit noise that is not meaningful to the end user.
		userMessage := sanitizeBuildError(res.Stderr)
		return apperr.New(500, "BUILD_FAILED", userMessage)
	}

	return nil
}

// RunMigrations executes artisan migrate inside the container
func (s *DockerService) RunMigrations(containerID string) (string, error) {
	// Infrastructure buffer: Wait for the container network stack and internal services
	// (like PHP-FPM or the application server) to fully initialize before executing
	// migration commands. This prevents "connection refused" or "socket not found" errors.
	time.Sleep(5 * time.Second)

	// Determine the best user to run the command (prefer www-data, fallback to root)
	user := "root"
	if res, err := utils.Run(5*time.Second, "docker", "exec", containerID, "id", "-u", "www-data"); err == nil && strings.TrimSpace(res.Stdout) != "" {
		user = "www-data"
	}

	// Ensure permissions for the web user before migration
	if user != "root" {
		if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "-R", user+":"+user, "storage", "bootstrap/cache"); err != nil {
			slog.Warn("Failed to fix permissions before migration", "error", err, "stderr", res.Stderr)
		}
	}

	// We use the detected user to ensure that any log files or cache files created during migration
	// are owned by the web user, preventing permission issues later.
	res, err := utils.Run(2*time.Minute, "docker", "exec", "-u", user, containerID, "php", "artisan", "migrate", "--force", "--no-interaction")

	if err != nil {
		output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
		if output == "" {
			output = "(No output from migration command)"
		}
		return output, fmt.Errorf("migration failed: %s", output)
	}

	return res.Stdout, nil
}

// injectDefaultRailpackConfig writes an optimized railpack.json for the project.
// It uses a single unified template and dynamically adjusts phases.
func (s *DockerService) injectDefaultRailpackConfig(buildPath string) {
	slog.Info("Injecting optimized Railpack configuration", "buildPath", buildPath)
	railpackConfigPath := filepath.Join(buildPath, "railpack.json")

	// 1. Resolve template path (we now use a single unified template)
	possiblePaths := []string{
		filepath.Join("/app/docker/templates", "railpack.json"),
		filepath.Join("docker/templates", "railpack.json"),
		filepath.Join("../docker/templates", "railpack.json"),
	}

	var templateData []byte
	var finalTemplatePath string

	for _, p := range possiblePaths {
		if data, err := os.ReadFile(p); err == nil {
			templateData = data
			finalTemplatePath = p
			break
		}
	}

	if templateData == nil {
		slog.Error("CRITICAL: Failed to find railpack templates in any location")
		templateData = []byte(`{"deploy":{"base":{"image":"debian:bookworm-slim"}}}`)
	} else {
		slog.Info("Loaded railpack template", "path", finalTemplatePath)
	}

	// 2. Refine configuration based on project contents
	var config map[string]interface{}
	if err := json.Unmarshal(templateData, &config); err == nil {
		modified := false

		hasPackageJson := false
		if _, err := os.Stat(filepath.Join(buildPath, "package.json")); err == nil {
			hasPackageJson = true
		}

		// 2.1 Framework-Specific Optimizations
		if !hasPackageJson {
			// Remove Node-specific phases and variables for non-Node projects
			if phases, ok := config["phases"].(map[string]interface{}); ok {
				if _, exists := phases["install"]; exists {
					delete(phases, "install")
					modified = true
				}
				if _, exists := phases["build"]; exists {
					delete(phases, "build")
					modified = true
				}
			}

			if variables, ok := config["variables"].(map[string]interface{}); ok {
				nodeVars := []string{"NPM_CONFIG_LEGACY_PEER_DEPS", "BUN_INSTALL_FROZEN_LOCKFILE", "NODE_ENV"}
				for _, v := range nodeVars {
					if _, exists := variables[v]; exists {
						delete(variables, v)
						modified = true
					}
				}
			}
			slog.Debug("Applied non-Node optimizations", "path", buildPath)
		}

		// 2.2 Cache Busting: Inject a unique build ID
		if _, exists := config["variables"]; !exists {
			config["variables"] = make(map[string]interface{})
		}

		if vars, ok := config["variables"].(map[string]interface{}); ok {
			vars["PAAS_BUILD_ID"] = fmt.Sprintf("%d", time.Now().Unix())
			modified = true
		}

		if modified {
			if newData, err := json.MarshalIndent(config, "", "  "); err == nil {
				templateData = newData
			}
		}
	}

	// 3. Finalize: Write the refined railpack.json to the build directory
	if err := os.WriteFile(railpackConfigPath, templateData, 0644); err != nil {
		slog.Error("Failed to write railpack.json", "path", railpackConfigPath, "error", err)
	} else {
		slog.Info("Successfully injected optimized railpack.json", "path", railpackConfigPath)
	}
}

func (s *DockerService) injectDockerIgnore(projectPath string) {
	ignorePath := filepath.Join(projectPath, ".dockerignore")
	if _, err := os.Stat(ignorePath); err == nil {
		// User already has a .dockerignore, respect it
		return
	}

	templatePath := filepath.Join("/app/docker/templates", ".dockerignore")
	// Fallback to local path for dev environment
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		templatePath = "docker/templates/.dockerignore"
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		slog.Warn("Failed to read .dockerignore template", "path", templatePath, "error", err)
		return
	}

	if err := os.WriteFile(ignorePath, data, 0644); err != nil {
		slog.Warn("Failed to write .dockerignore to project", "path", ignorePath, "error", err)
		return
	}

	slog.Debug("Injected default .dockerignore", "path", ignorePath)
}
