package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// ===========================================
// Container Operations
// ===========================================
// BuildAndRun builds and starts a container for a project using Railpack with cancellation context
func (s *DockerService) BuildAndRun(ctx context.Context, project *models.Project, phpVersion, projectDomain string, cpuLimit float64, memoryLimit string, isInitial bool, noCache bool, logCallback func(string)) (string, error) {
	projectPath := project.GetProjectPath(s.cfg.ProjectsPath)

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

	// Set BuildKit host to use the remote BuildKit container
	os.Setenv("BUILDKIT_HOST", "tcp://paas-buildkit:1234")

	var err error
	var internalPort string

	// 3. Branching Build Strategy
	if project.Framework == "Laravel" {
		slog.Info("Using legacy Laravel build strategy", "subdomain", project.Subdomain)
		err = s.legacyLaravelBuild(ctx, project, buildPath, imageName, phpVersion, logFilePath, noCache, logCallback)
	} else {
		slog.Info("Using Railpack build strategy", "subdomain", project.Subdomain)
		err = s.railpackBuild(ctx, project, buildPath, imageName, logFilePath, noCache, logCallback)
	}

	if err != nil {
		return "", err
	}

	// Apply commit SHA tag for rollbacks/image retention
	if project.LastCommitHash != "" {
		commitTag := fmt.Sprintf("%s:%s", imageName, project.LastCommitHash)
		slog.Info("Applying commit tag to built image", "tag", commitTag)
		if tagRes, tagErr := utils.Run(1*time.Minute, "docker", "tag", imageName+":latest", commitTag); tagErr != nil {
			slog.Warn("Failed to tag image with commit hash", "error", tagErr, "stderr", tagRes.Stderr)
		}
	}

	// 3.5. NEW: Dynamic Port Detection from Image Metadata
	// If port hasn't been manually set by user, try to detect it from image metadata
	detectedPort, detectErr := s.DetectExposedPort(imageName)
	if detectErr == nil && detectedPort > 0 {
		slog.Info("Automatically detected exposed port from image", "subdomain", project.Subdomain, "port", detectedPort)
		p := detectedPort
		project.Port = &p
	}

	// 4. Determine Final Internal Port for Traefik
	internalPort = project.GetInternalPort()

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

	// Web-facing by default unless port is explicitly set to <= 0.
	isWebFacing := project.Port == nil || *project.Port > 0
	if !isWebFacing {
		slog.Info("Project classified as non-web", "subdomain", project.Subdomain, "framework", project.Framework)
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
		"-e", "TZ=Asia/Jakarta",
		"--env-file", filepath.Join(projectPath, ".env"),
	}

	if isWebFacing {
		runArgs = append(runArgs,
			"--label", "traefik.enable=true",
			"--label", fmt.Sprintf("traefik.http.routers.%s.rule=%s",
				routerName, fmt.Sprintf("Host(`%s.%s`)", project.Subdomain, projectDomain)),
			"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=%s", serviceName, internalPort),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=%s", serviceName, project.GetHealthCheckPath()),
			"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=10s", serviceName),
		)
	} else {
		runArgs = append(runArgs,
			"--label", "traefik.enable=false",
		)
	}

	runArgs = append(runArgs,
		// PaaS metadata labels
		"--label", fmt.Sprintf("paas.project_id=%d", project.ID),
		"--label", fmt.Sprintf("paas.project_subdomain=%s", project.Subdomain),
		"--label", fmt.Sprintf("paas.rollout_created_at=%d", timestamp),
		"--label", "paas.container_role=web",

		// Standard Volume Mapping
		"-v", fmt.Sprintf("%s:/var/www/html/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/storage/app", hostPersistentPath),
		"-v", fmt.Sprintf("%s:/app/data", hostPersistentPath),

		imageName,
	)

	// 4.5. Append custom start command if provided
	if project.StartCommand != "" {
		cmdStr := strings.TrimSpace(project.StartCommand)
		if project.Framework == "Laravel" && !strings.HasPrefix(cmdStr, "php ") && !strings.HasPrefix(cmdStr, "/usr/bin/") && !strings.HasPrefix(cmdStr, "sh ") && !strings.HasPrefix(cmdStr, "bash ") && !strings.HasPrefix(cmdStr, "npm ") && !strings.HasPrefix(cmdStr, "bun ") {
			cmdStr = "php artisan " + cmdStr
		}
		runArgs = append(runArgs, "sh", "-c", cmdStr)
	}

	res, runErr := utils.RunCtx(ctx, 3*time.Minute, "docker", runArgs...)
	if runErr != nil {
		errMsg := res.Stderr
		if errMsg == "" {
			errMsg = runErr.Error()
		}
		return "", apperr.New(500, "DOCKER_RUN_FAILED", fmt.Sprintf("Failed to start container for %s: %s", project.Subdomain, errMsg))
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

// legacyLaravelBuild handles the proven Dockerfile + Nginx approach with cancellation context
func (s *DockerService) legacyLaravelBuild(ctx context.Context, project *models.Project, buildPath, imageName, phpVersion, logFilePath string, noCache bool, logCallback func(string)) error {
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
	buildArgs := []string{"buildx", "build", "--builder", "paas-builder", "--load"}
	if noCache {
		buildArgs = append(buildArgs, "--no-cache")
	}
	if project.BuildCommand != "" {
		cmdStr := strings.TrimSpace(project.BuildCommand)
		if project.Framework == "Laravel" && !strings.HasPrefix(cmdStr, "php ") && !strings.HasPrefix(cmdStr, "composer ") && !strings.HasPrefix(cmdStr, "npm ") && !strings.HasPrefix(cmdStr, "bun ") && !strings.HasPrefix(cmdStr, "yarn ") && !strings.HasPrefix(cmdStr, "pnpm ") {
			cmdStr = "php artisan " + cmdStr
		}
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("BUILD_COMMAND=%s", cmdStr))
	}
	if project.NodeVersion != "" {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("NODE_VERSION=%s", project.NodeVersion))
	}
	buildArgs = append(buildArgs, "--label", models.LabelProjectManaged, "-t", imageName, buildPath)

	res, err := utils.RunWithRefinedLogCtx(ctx, 30*time.Minute, logFilePath, logCallback, "docker", buildArgs...)

	if err != nil {
		return apperr.New(500, "DOCKER_BUILD_FAILED", fmt.Sprintf("Docker build failed for %s: %s", project.Subdomain, res.Stderr))
	}

	return nil
}

// railpackBuild handles all other languages using Nixpacks-style auto-detection with cancellation context
func (s *DockerService) railpackBuild(ctx context.Context, project *models.Project, buildPath, imageName, logFilePath string, noCache bool, logCallback func(string)) error {
	s.injectDefaultRailpackConfig(buildPath)

	// Strip strict lockfiles before Railpack runs.
	for _, lockfile := range []string{"bun.lock", "yarn.lock"} {
		if err := os.Remove(filepath.Join(buildPath, lockfile)); err == nil {
			slog.Info("Stripped lockfile to allow non-frozen install", "file", lockfile, "subdomain", project.Subdomain)
		}
	}

	cacheKey := fmt.Sprintf("project-%d", project.ID)
	if noCache {
		cacheKey = fmt.Sprintf("project-%d-%d", project.ID, time.Now().Unix())
	}

	buildArgs := []string{
		"build",
		"--name", imageName,
		"--cache-key", cacheKey,
	}
	envs := map[string]string{
		"NPM_CONFIG_JOBS":             "2",
		"NPM_CONFIG_LEGACY_PEER_DEPS": "true",
		"PYTHONUNBUFFERED":            "1",
		"PAAS_BUILD_ID":               fmt.Sprintf("%d", time.Now().Unix()),
	}

	projectEnvPath := filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), ".env")
	if envVars, err := s.parseProjectEnv(projectEnvPath); err == nil {
		for key, value := range envVars {
			envs[key] = value
		}
	}

	if project.NodeVersion != "" {
		envs["NIXPACKS_NODE_VERSION"] = project.NodeVersion
	}

	if project.BuildCommand != "" {
		envs["NIXPACKS_BUILD_CMD"] = project.BuildCommand
	}

	if project.StartCommand != "" {
		envs["NIXPACKS_START_CMD"] = project.StartCommand
	}

	// Append all envs to buildArgs
	for key, value := range envs {
		buildArgs = append(buildArgs, "--env", fmt.Sprintf("%s=%s", key, value))
	}

	// Finalize build command with path
	buildArgs = append(buildArgs, buildPath)

	res, err := utils.RunWithRefinedLogCtx(ctx, 30*time.Minute, logFilePath, logCallback, "railpack", buildArgs...)

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

// PruneProjectImages removes older images for a project keeping only the latest maxRetention unique versions
func (s *DockerService) PruneProjectImages(subdomain string, maxRetention int) {
	if maxRetention <= 0 {
		maxRetention = 2 // default fallback
	}

	slog.Info("Running image retention pruning", "subdomain", subdomain, "max_retention", maxRetention)

	// List images with their tags and image IDs
	res, err := utils.Run(30*time.Second, "docker", "images", "--format", "{{.Tag}}::{{.ID}}", "paas-"+subdomain)
	if err != nil {
		slog.Warn("Failed to list project images for pruning", "subdomain", subdomain, "error", err)
		return
	}

	lines := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	if len(lines) <= maxRetention {
		return // Not enough images to prune
	}

	type imageInfo struct {
		tag string
		id  string
	}
	var images []imageInfo
	for _, line := range lines {
		parts := strings.Split(line, "::")
		if len(parts) == 2 && parts[0] != "" {
			images = append(images, imageInfo{tag: parts[0], id: parts[1]})
		}
	}

	// Find the unique images by ID, preserving order (newest first)
	var uniqueImageIDs []string
	seenIDs := make(map[string]bool)
	for _, img := range images {
		if !seenIDs[img.id] {
			seenIDs[img.id] = true
			uniqueImageIDs = append(uniqueImageIDs, img.id)
		}
	}

	// If the number of unique images is within the retention limit, nothing to prune
	if len(uniqueImageIDs) <= maxRetention {
		return
	}

	// The images we want to keep are the ones corresponding to the top `maxRetention` unique IDs
	keepIDs := make(map[string]bool)
	for i := 0; i < maxRetention && i < len(uniqueImageIDs); i++ {
		keepIDs[uniqueImageIDs[i]] = true
	}

	// Now find all tags that belong to the images we want to delete
	for _, img := range images {
		if !keepIDs[img.id] && img.tag != "latest" {
			imageToDel := fmt.Sprintf("paas-%s:%s", subdomain, img.tag)
			slog.Info("Pruning old project image", "image", imageToDel)
			if delRes, delErr := utils.Run(30*time.Second, "docker", "rmi", imageToDel); delErr != nil {
				slog.Warn("Failed to delete old project image", "image", imageToDel, "error", delErr, "stderr", delRes.Stderr)
			}
		}
	}
}
