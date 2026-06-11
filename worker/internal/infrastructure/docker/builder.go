package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
		cleanedErr := sanitizeDockerRunError(errMsg)
		cleanedErr = strings.TrimPrefix(cleanedErr, "failed to start container: ")
		return "", apperr.New(500, "DOCKER_RUN_FAILED", fmt.Sprintf("Failed to start container for %s: %s", project.Subdomain, cleanedErr))
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

// railpackBuild handles all other languages using railpacks-style auto-detection with cancellation context
func (s *DockerService) railpackBuild(ctx context.Context, project *models.Project, buildPath, imageName, logFilePath string, noCache bool, logCallback func(string)) error {
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
	}

	projectEnvPath := filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), ".env")
	if envVars, err := s.ParseProjectEnv(projectEnvPath); err == nil {
		for key, value := range envVars {
			envs[key] = value
		}
	}

	if noCache {
		envs["NO_CACHE"] = "1"
		envs["RAILPACK_DISABLE_CACHES"] = "*"
	}

	// Dynamically inject runtime version overrides based on project.Framework and LanguageVersion
	switch project.Framework {
	case "Go", "Golang":
		if project.LanguageVersion != "" {
			envs["RAILPACK_GO_VERSION"] = project.LanguageVersion
		}
	case "Python":
		if project.LanguageVersion != "" {
			envs["RAILPACK_PYTHON_VERSION"] = project.LanguageVersion
		}
	default:
		// Default / Node runtime version overrides
		if project.NodeVersion != "" {
			envs["RAILPACK_NODE_VERSION"] = project.NodeVersion
		} else if project.LanguageVersion != "" {
			envs["RAILPACK_NODE_VERSION"] = project.LanguageVersion
		}
	}

	// Fallback check if framework is empty but language version is provided
	if project.Framework == "" && project.LanguageVersion != "" {
		if _, err := os.Stat(filepath.Join(buildPath, "go.mod")); err == nil {
			envs["RAILPACK_GO_VERSION"] = project.LanguageVersion
		} else if _, err := os.Stat(filepath.Join(buildPath, "requirements.txt")); err == nil {
			envs["RAILPACK_PYTHON_VERSION"] = project.LanguageVersion
		} else if _, err := os.Stat(filepath.Join(buildPath, "package.json")); err == nil {
			envs["RAILPACK_NODE_VERSION"] = project.LanguageVersion
		}
	}

	// Auto-detect SPA / Static hosting recursively if start command is empty
	isStaticFramework := false
	staticFrameworks := []string{"Vite", "React", "Vue", "Svelte", "Static", "Angular"}
	for _, f := range staticFrameworks {
		if project.Framework == f {
			isStaticFramework = true
			break
		}
	}

	// Find all package.json files up to depth 3 to support monorepos
	packageJSONs := s.findNestedPackageJSONs(buildPath, 1, 3)
	var targetPackageJSON string
	if len(packageJSONs) == 1 {
		targetPackageJSON = packageJSONs[0]
	} else if len(packageJSONs) > 1 {
		// Monorepo: Prioritize directories named web, frontend, client, or spa
		for _, pPath := range packageJSONs {
			dirName := filepath.Base(filepath.Dir(pPath))
			if dirName == "web" || dirName == "frontend" || dirName == "client" || dirName == "spa" {
				targetPackageJSON = pPath
				break
			}
		}
		// Fallback to the first nested package.json if no specific match
		if targetPackageJSON == "" {
			for _, pPath := range packageJSONs {
				if filepath.Dir(pPath) != buildPath {
					targetPackageJSON = pPath
					break
				}
			}
		}
		// Fallback to root package.json
		if targetPackageJSON == "" {
			targetPackageJSON = packageJSONs[0]
		}
	}

	var staticDir string
	if targetPackageJSON != "" && project.StartCommand == "" {
		packageDir := filepath.Dir(targetPackageJSON)
		relDir, errRel := filepath.Rel(buildPath, packageDir)
		if errRel != nil {
			relDir = "."
		}

		hasViteConfig := false
		for _, vf := range []string{"vite.config.ts", "vite.config.js", "nuxt.config.js", "nuxt.config.ts", "svelte.config.js"} {
			if _, err := os.Stat(filepath.Join(packageDir, vf)); err == nil {
				hasViteConfig = true
				break
			}
		}

		isStaticNode := false
		isNextJS := false
		isNuxtJS := false
		hasStartScript := false
		data, err := os.ReadFile(targetPackageJSON)
		if err == nil {
			content := string(data)
			hasStartScript = strings.Contains(content, "\"start\"") || strings.Contains(content, "'start'")
			hasBuildScript := strings.Contains(content, "\"build\"") || strings.Contains(content, "'build'")
			isNextJS = strings.Contains(content, "\"next\"")
			isNuxtJS = strings.Contains(content, "\"nuxt\"")

			if !hasStartScript && (hasBuildScript || hasViteConfig) {
				isStaticNode = true
			}

			// Parse package.json start script to detect custom port configurations (SRE/Infrastructure Improvement)
			type PackageJSON struct {
				Scripts map[string]string `json:"scripts"`
			}
			var pkgJSON PackageJSON
			if json.Unmarshal(data, &pkgJSON) == nil {
				if startScript, exists := pkgJSON.Scripts["start"]; exists {
					portRegex := regexp.MustCompile(`(?i)(?:\bport\b|PORT\s*=\s*|\b-p\b)\s*=?\s*(\d+)`)
					if matches := portRegex.FindStringSubmatch(startScript); len(matches) > 1 {
						if pVal, errP := strconv.Atoi(matches[1]); errP == nil && pVal > 0 && pVal <= 65535 {
							project.Port = &pVal
							slog.Info("Parsed custom port override from package.json start script", "subdomain", project.Subdomain, "port", pVal)
						}
					}
				}
			}
		}

		// Resolve correct package manager based on lockfiles
		pkgManager := "npm"
		if _, errLock := os.Stat(filepath.Join(buildPath, "bun.lock")); errLock == nil {
			pkgManager = "bun"
		} else if _, errLock := os.Stat(filepath.Join(packageDir, "bun.lock")); errLock == nil {
			pkgManager = "bun"
		} else if _, errLock := os.Stat(filepath.Join(buildPath, "pnpm-lock.yaml")); errLock == nil {
			pkgManager = "pnpm"
		} else if _, errLock := os.Stat(filepath.Join(packageDir, "pnpm-lock.yaml")); errLock == nil {
			pkgManager = "pnpm"
		} else if _, errLock := os.Stat(filepath.Join(buildPath, "yarn.lock")); errLock == nil {
			pkgManager = "yarn"
		} else if _, errLock := os.Stat(filepath.Join(packageDir, "yarn.lock")); errLock == nil {
			pkgManager = "yarn"
		}

		if isStaticNode || isStaticFramework {
			isStaticFramework = true
			outputDir := s.detectStaticOutputDir(targetPackageJSON)
			finalOutputDir := outputDir
			if relDir != "." {
				finalOutputDir = filepath.Join(relDir, outputDir)
			}

			if logCallback != nil {
				logCallback(fmt.Sprintf(">> WARNING: No start command detected for frontend project. Automatically configuring static SPA hosting (serving '%s' directory).", finalOutputDir))
			}
			slog.Info("Auto-configuring static SPA hosting", "subdomain", project.Subdomain, "outputDir", finalOutputDir)
			envs["RAILPACK_SPA_OUTPUT_DIR"] = finalOutputDir
			staticDir = finalOutputDir

			if project.BuildCommand == "" {
				baseCmd := fmt.Sprintf("%s run build", pkgManager)
				if relDir != "." {
					envs["RAILPACK_BUILD_CMD"] = fmt.Sprintf("cd %s && %s", relDir, baseCmd)
				} else {
					envs["RAILPACK_BUILD_CMD"] = baseCmd
				}
			}
		} else if relDir != "." {
			// Monorepo dynamic app auto-detection (e.g. Next.js standalone or Express/Hono workspace)
			if isNextJS || isNuxtJS || hasStartScript {
				isStaticFramework = false

				if project.BuildCommand == "" {
					envs["RAILPACK_BUILD_CMD"] = fmt.Sprintf("cd %s && %s run build", relDir, pkgManager)
				}
				if project.StartCommand == "" {
					envs["RAILPACK_START_CMD"] = fmt.Sprintf("cd %s && %s run start", relDir, pkgManager)
				}
				if logCallback != nil {
					logCallback(fmt.Sprintf(">> INFO: Monorepo workspace app detected at '%s'. Automatically configuring build command ('cd %s && %s run build') and start command ('cd %s && %s run start').", relDir, relDir, pkgManager, relDir, pkgManager))
				}
				slog.Info("Auto-configuring monorepo workspace app", "subdomain", project.Subdomain, "relDir", relDir, "pkgManager", pkgManager)
			}
		}
	}

	if project.BuildCommand != "" {
		envs["RAILPACK_BUILD_CMD"] = project.BuildCommand
	}

	if project.StartCommand != "" {
		envs["RAILPACK_START_CMD"] = project.StartCommand
	}

	// Detect runtime technology stack based on files and project models
	stack := "nodejs" // default
	if project.Framework == "Go" || project.Framework == "Golang" {
		stack = "golang"
	} else if project.Framework == "Python" {
		stack = "python"
	} else if isStaticFramework {
		stack = "static"
	} else {
		// File-based auto-detection fallback
		if _, err := os.Stat(filepath.Join(buildPath, "go.mod")); err == nil {
			stack = "golang"
		} else if _, err := os.Stat(filepath.Join(buildPath, "requirements.txt")); err == nil {
			stack = "python"
		} else if _, err := os.Stat(filepath.Join(buildPath, "package.json")); err == nil {
			if isStaticFramework {
				stack = "static"
			} else {
				stack = "nodejs"
			}
		}
	}

	railpackConfigPath := filepath.Join(buildPath, "railpack.json")
	_, hasUserRailpack := os.Stat(railpackConfigPath)
	userRailpackExists := hasUserRailpack == nil

	// Defer cleanup of dynamically generated config files
	defer func() {
		if !userRailpackExists {
			_ = os.Remove(railpackConfigPath)
		}
		_ = os.Remove(filepath.Join(buildPath, "Caddyfile"))
	}()

	// Inject optimized railpack.json configuration using resolved commands and stack type
	s.injectDefaultRailpackConfig(buildPath, envs["RAILPACK_BUILD_CMD"], envs["RAILPACK_START_CMD"], stack, project, staticDir, noCache)

	// Finalize build command with path
	buildArgs = append(buildArgs, buildPath)

	// Collect env variables in KEY=VALUE format for secure OS-level injection
	var envSlice []string
	for key, value := range envs {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, value))
	}

	// Securely run build by passing secrets through OS process environment rather than command args
	res, err := utils.RunWithRefinedLogAndEnvCtx(ctx, 30*time.Minute, envSlice, logFilePath, logCallback, "railpack", buildArgs...)

	if err != nil {
		// Cleanup: Remove failed image and prune cache if it looks like a corruption issue.
		slog.Warn("Railpack build failed, performing cleanup", "subdomain", project.Subdomain, "error", err)

		_ = utils.RunSilent(time.Minute, "docker", "rmi", imageName)

		if strings.Contains(res.Stderr, "unrecognized image format") || strings.Contains(res.Stderr, "failed to solve") {
			slog.Info("Detected build kit corruption, pruning builder cache and disabling cache for next run", "subdomain", project.Subdomain)
			_ = utils.RunSilent(time.Minute, "docker", "builder", "prune", "-f", "--filter", "until=0s")
		}

		userMessage := sanitizeBuildError(res.Stderr)
		return apperr.New(500, "BUILD_FAILED", userMessage)
	}

	// Verify if build output indicates missing start command for a non-static project (Smart Fail-Fast)
	combinedOutput := res.Stdout + "\n" + res.Stderr
	if strings.Contains(combinedOutput, "No start command detected") && project.StartCommand == "" && envs["RAILPACK_START_CMD"] == "" && !isStaticFramework {
		slog.Warn("Railpack build finished but no start command was configured, failing early", "subdomain", project.Subdomain)
		_ = utils.RunSilent(time.Minute, "docker", "rmi", imageName)
		return apperr.New(400, "NO_START_COMMAND", "Build succeeded, but no start command or static/SPA output directory was detected. Non-static projects require a start command. Please configure a 'start' script in your package.json or specify a 'Start Command' in project settings.")
	}

	return nil
}

// injectDefaultRailpackConfig writes an optimized railpack.json for the project.
func (s *DockerService) injectDefaultRailpackConfig(buildPath string, buildCmd, startCmd string, stack string, project *models.Project, staticDir string, noCache bool) {
	slog.Info("Injecting optimized Railpack configuration", "buildPath", buildPath, "stack", stack, "buildCmd", buildCmd, "startCmd", startCmd)
	railpackConfigPath := filepath.Join(buildPath, "railpack.json")

	// 1. Check if railpack.json already exists in user's repo. If yes, skip config injection completely.
	if _, err := os.Stat(railpackConfigPath); err == nil {
		slog.Info("User-provided railpack.json detected, skipping template injection", "path", railpackConfigPath)
		return
	}

	// 2. Load the corresponding railpack.json from the stack directory under cfg.RailpacksPath
	templatePath := filepath.Join(s.cfg.RailpacksPath, stack, "railpack.json")
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		slog.Error("Failed to read railpack template, using minimal fallback", "path", templatePath, "error", err)
		templateData = []byte(`{"deploy":{"base":{"image":"debian:bookworm-slim"}}}`)
	}

	// 3. Perform version substitution
	version := "20" // default Node
	switch stack {
	case "golang":
		version = "latest"
	case "python":
		version = "3.11"
	}

	// Use project's specified version if provided
	if project.LanguageVersion != "" {
		version = project.LanguageVersion
	} else if stack == "nodejs" && project.NodeVersion != "" {
		version = project.NodeVersion
	}

	configStr := string(templateData)
	configStr = strings.ReplaceAll(configStr, "{{VERSION}}", version)
	if stack == "static" {
		configStr = strings.ReplaceAll(configStr, "{{STATIC_DIR}}", staticDir)
	}
	templateData = []byte(configStr)

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &config); err == nil {
		modified := false

		// 4. Overwrite steps.build.commands and deploy.startCommand with user custom inputs if configured
		if buildCmd != "" {
			if _, exists := config["steps"]; !exists {
				config["steps"] = make(map[string]interface{})
			}
			if steps, ok := config["steps"].(map[string]interface{}); ok {
				if _, exists := steps["build"]; !exists {
					steps["build"] = make(map[string]interface{})
				}
				if buildStep, ok := steps["build"].(map[string]interface{}); ok {
					buildStep["commands"] = []interface{}{buildCmd}
					modified = true
				}
			}
		}

		if startCmd != "" {
			if _, exists := config["deploy"]; !exists {
				config["deploy"] = make(map[string]interface{})
			}
			if deploy, ok := config["deploy"].(map[string]interface{}); ok {
				deploy["startCommand"] = startCmd
				modified = true
			}
		}

		// 5. Cache Busting: Inject a unique build ID only when noCache = true
		if noCache {
			if _, exists := config["variables"]; !exists {
				config["variables"] = make(map[string]interface{})
			}
			if vars, ok := config["variables"].(map[string]interface{}); ok {
				vars["PAAS_BUILD_ID"] = fmt.Sprintf("%d", time.Now().Unix())
				modified = true
			}
		}

		if modified {
			if newData, err := json.MarshalIndent(config, "", "  "); err == nil {
				templateData = newData
			}
		}
	}

	// 6. Write the refined railpack.json atomically to avoid race conditions
	if err := utils.WriteFileAtomic(railpackConfigPath, templateData, 0644); err != nil {
		slog.Error("Failed to write railpack.json atomically", "path", railpackConfigPath, "error", err)
	} else {
		slog.Info("Successfully injected optimized railpack.json", "path", railpackConfigPath)
	}

	// 7. If stack is static, write the Caddyfile dynamically to buildPath
	if stack == "static" {
		caddyfilePath := filepath.Join(s.cfg.RailpacksPath, "static", "Caddyfile")
		caddyfileData, err := os.ReadFile(caddyfilePath)
		if err != nil {
			slog.Error("Failed to read Caddyfile template", "path", caddyfilePath, "error", err)
		} else {
			caddyfileStr := string(caddyfileData)
			caddyfileStr = strings.ReplaceAll(caddyfileStr, "{{STATIC_DIR}}", staticDir)
			err = utils.WriteFileAtomic(filepath.Join(buildPath, "Caddyfile"), []byte(caddyfileStr), 0644)
			if err != nil {
				slog.Error("Failed to write Caddyfile dynamically", "error", err)
			} else {
				slog.Info("Successfully injected Caddyfile", "path", filepath.Join(buildPath, "Caddyfile"))
			}
		}
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

	// Write the default .dockerignore atomically to prevent half-written files from being read.
	if err := utils.WriteFileAtomic(ignorePath, data, 0644); err != nil {
		slog.Warn("Failed to write .dockerignore atomically to project", "path", ignorePath, "error", err)
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

// detectStaticOutputDir inspects the package.json to decide if the build output dir is "build" or "dist"
func (s *DockerService) detectStaticOutputDir(packageJSONPath string) string {
	data, err := os.ReadFile(packageJSONPath)
	if err == nil {
		content := string(data)
		// Classic React (create-react-app) uses "build" as output directory by default
		if strings.Contains(content, "react-scripts build") {
			return "build"
		}
	}
	return "dist"
}

// FindFileRecursively searches for a file in the directory up to maxDepth, ignoring common build and dependency folders
func (s *DockerService) FindFileRecursively(dir string, filename string, currentDepth, maxDepth int) string {
	if currentDepth > maxDepth {
		return ""
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// First pass: check files in current directory
	for _, f := range files {
		if !f.IsDir() && f.Name() == filename {
			return filepath.Join(dir, f.Name())
		}
	}

	// Second pass: traverse subdirectories
	for _, f := range files {
		if f.IsDir() {
			name := f.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" || name == ".next" || name == "vendor" {
				continue
			}
			found := s.FindFileRecursively(filepath.Join(dir, name), filename, currentDepth+1, maxDepth)
			if found != "" {
				return found
			}
		}
	}
	return ""
}

// findNestedPackageJSONs searches for all package.json files up to maxDepth, ignoring common build and dependency folders
func (s *DockerService) findNestedPackageJSONs(dir string, currentDepth, maxDepth int) []string {
	if currentDepth > maxDepth {
		return nil
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	// First pass: collect package.json in current directory
	for _, f := range files {
		if !f.IsDir() && f.Name() == "package.json" {
			results = append(results, filepath.Join(dir, f.Name()))
		}
	}

	// Second pass: recursively walk subdirectories
	for _, f := range files {
		if f.IsDir() {
			name := f.Name()
			if name == "node_modules" || name == ".git" || name == "dist" || name == "build" || name == ".next" || name == "vendor" {
				continue
			}
			subResults := s.findNestedPackageJSONs(filepath.Join(dir, name), currentDepth+1, maxDepth)
			results = append(results, subResults...)
		}
	}
	return results
}
