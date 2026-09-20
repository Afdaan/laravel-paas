package docker

import (
	"context"
	"encoding/json"
	"errors"
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
	buildPath, resolveErr := s.ResolveBuildPath(projectPath, project.BaseDirectory)
	if resolveErr != nil {
		return "", resolveErr
	}

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
	s.injectDockerIgnore(projectPath, logCallback)

	imageName := fmt.Sprintf("paas-%s", project.Subdomain)
	logFilePath := filepath.Join(projectPath, "build.log")
	if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
		logsDir := filepath.Join(projectPath, "logs")
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			slog.Error("Failed to create logs directory", "path", logsDir, "error", err)
			return "", fmt.Errorf("failed to create logs directory: %w", err)
		}
		logFilePath = filepath.Join(logsDir, fmt.Sprintf("build-%s.log", *project.DeploymentJobID))
	}

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
		if err == nil {
			validationErr := s.validateImageStartupArtifacts(imageName)
			if validationErr != nil && !noCache {
				slog.Warn("Railpack image validation failed; retrying without build cache", "subdomain", project.Subdomain, "error", validationErr)
				if logCallback != nil {
					logCallback(">> WARNING: Built image is missing its required startup file. Retrying once without build cache...")
				}
				_ = utils.RunSilent(time.Minute, "docker", "rmi", "-f", imageName)
				err = s.railpackBuild(ctx, project, buildPath, imageName, logFilePath, true, logCallback)
				if err == nil {
					validationErr = s.validateImageStartupArtifacts(imageName)
				}
			}
			if err == nil && validationErr != nil {
				slog.Error("Railpack produced an invalid runtime image", "subdomain", project.Subdomain, "error", validationErr)
				return "", apperr.New(500, "INVALID_RUNTIME_IMAGE", "Build produced an incomplete runtime image: required startup file '/start-container.sh' is missing. Deployment stopped before replacing the running version.")
			}
		}
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
	exposure := project.ResolveRuntimeExposure(0)
	if detectErr == nil {
		exposure = project.ResolveRuntimeExposure(detectedPort)
	}
	if exposure.Reason == models.RuntimeExposureReasonImageExpose {
		slog.Info("Automatically detected exposed port from image", "subdomain", project.Subdomain, "port", detectedPort)
		p := detectedPort
		project.Port = &p
		if s.GetDB() != nil {
			s.GetDB().Model(project).UpdateColumn("port", detectedPort)
		}
	}

	// 4. Determine Final Internal Port for Traefik
	internalPort = fmt.Sprintf("%d", exposure.Port)

	// 5. Start Web Container
	s.storage.EnsurePersistentPath(project)
	hostPersistentPath := s.storage.GetPersistentHostPath(project)

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	finalCPUs := models.DefaultDockerCPULimit
	if cpuLimit > 0 {
		finalCPUs = strconv.FormatFloat(cpuLimit, 'f', -1, 64)
	}

	finalMemory := models.DefaultDockerMemoryLimit
	if memoryLimit != "" {
		finalMemory = memoryLimit
	}

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
		message := "Internal port could not be resolved. Set the project internal port to expose this app publicly, or set it to 0 for a private worker-only service."
		if logCallback != nil {
			logCallback("ERROR: " + message)
		}
		return "", apperr.New(400, "PORT_RESOLUTION_REQUIRED", message)
	}

	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--network", models.NetworkName,
		"--network-alias", fmt.Sprintf("project-%s", project.Subdomain),
		"--restart", "unless-stopped",
		"--cpus", finalCPUs,
		"--memory", finalMemory,
		"-e", "PYTHONUNBUFFERED=1",
		"-e", "TZ=Asia/Jakarta",
		"--env-file", filepath.Join(projectPath, ".env"),
		"-e", fmt.Sprintf("PORT=%s", internalPort),
	}

	runArgs = append(runArgs, TenantHardeningArgs(finalMemory)...)

	// Project ingress is resolved by the authenticated backend proxy after database
	// promotion. Docker-provider routing would expose an unvalidated rollout early.
	runArgs = append(runArgs, "--label", "traefik.enable=false")

	runArgs = append(runArgs,
		// Runara metadata labels
		"--label", fmt.Sprintf("paas.project_id=%d", project.ID),
		"--label", fmt.Sprintf("paas.project_subdomain=%s", project.Subdomain),
		"--label", fmt.Sprintf("paas.rollout_created_at=%d", timestamp),
		"--label", "paas.container_role=web",
	)

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
		// Mount only the sqlite file, not the directory, to preserve Laravel's database/migrations and database/seeders.
		hostSQLiteFile := filepath.Join(hostPersistentPath, "sqlite", "database.sqlite")
		volumes = append(volumes, "-v", fmt.Sprintf("%s:/var/www/html/database/database.sqlite", hostSQLiteFile))
	}
	runArgs = append(runArgs, volumes...)
	runArgs = append(runArgs, imageName)

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

	if project.DatabaseOption == "sqlite" {
		if err := s.EnsureSQLiteFile(mainContainerID); err != nil {
			cleanupErr := s.RemoveContainer(mainContainerID, nil)
			if cleanupErr != nil {
				return mainContainerID, errors.Join(
					fmt.Errorf("initialize SQLite database: %w", err),
					fmt.Errorf("cleanup main rollout container: %w", cleanupErr),
				)
			}
			return "", fmt.Errorf("failed to initialize SQLite database: %w", err)
		}
	}

	// 5. Start Worker Container if needed (for non-Laravel)
	if project.Framework != "Laravel" && project.WorkerCommand != "" {
		slog.Info("Starting background worker container", "subdomain", project.Subdomain, "command", project.WorkerCommand)
		workerID, err := s.StartWorkerContainer(project, imageName, finalCPUs, finalMemory)
		if err != nil {
			cleanupErr := s.RemoveContainer(mainContainerID, nil)
			if cleanupErr != nil {
				return mainContainerID, errors.Join(
					fmt.Errorf("start required worker container: %w", err),
					fmt.Errorf("cleanup main rollout container: %w", cleanupErr),
				)
			}
			return "", fmt.Errorf("start required worker container: %w", err)
		}
		project.WorkerContainerID = &workerID
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
stdout_logfile=/var/www/html/storage/logs/laravel-worker.log
stdout_logfile_maxbytes=5MB
stdout_logfile_backups=5
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
	if npmrcPath, cleanup, err := s.prepareNpmAuthConfig(buildPath, project); err != nil {
		return err
	} else if npmrcPath != "" {
		defer cleanup()
		buildArgs = append(buildArgs, "--secret", fmt.Sprintf("id=npmrc,src=%s", npmrcPath))
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
		"--error-missing-start",
	}
	envs := map[string]string{
		"NPM_CONFIG_JOBS":             "2",
		"NPM_CONFIG_LEGACY_PEER_DEPS": "true",
		"NPM_CONFIG_INCLUDE":          "dev",
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

	// Dynamically inject runtime version overrides using idiomatic files for Railpack/mise
	switch project.Framework {
	case "Go", "Golang":
		if project.LanguageVersion != "" {
			versionFile := filepath.Join(buildPath, ".go-version")
			if _, err := os.Stat(versionFile); os.IsNotExist(err) {
				if err := os.WriteFile(versionFile, []byte(project.LanguageVersion), 0644); err != nil {
					slog.Warn("Failed to inject .go-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected .go-version for Railpack", "version", project.LanguageVersion)
				}
			}
		}
	case "Python":
		if project.LanguageVersion != "" {
			versionFile := filepath.Join(buildPath, ".python-version")
			if _, err := os.Stat(versionFile); os.IsNotExist(err) {
				if err := os.WriteFile(versionFile, []byte(project.LanguageVersion), 0644); err != nil {
					slog.Warn("Failed to inject .python-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected .python-version for Railpack", "version", project.LanguageVersion)
				}
			}
		}
	default:
		// Default / Node runtime version overrides
		nodeVer := project.NodeVersion
		if nodeVer == "" {
			nodeVer = project.LanguageVersion
		}
		if nodeVer != "" {
			versionFile := filepath.Join(buildPath, ".node-version")
			if _, err := os.Stat(versionFile); os.IsNotExist(err) {
				if err := os.WriteFile(versionFile, []byte(nodeVer), 0644); err != nil {
					slog.Warn("Failed to inject .node-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected .node-version for Railpack", "version", nodeVer)
				}
			}
		}
	}

	// Fallback check if framework is empty but language version is provided
	if project.Framework == "" && project.LanguageVersion != "" {
		if _, err := os.Stat(filepath.Join(buildPath, "go.mod")); err == nil {
			versionFile := filepath.Join(buildPath, ".go-version")
			if _, e := os.Stat(versionFile); os.IsNotExist(e) {
				if err := os.WriteFile(versionFile, []byte(project.LanguageVersion), 0644); err != nil {
					slog.Warn("Failed to inject fallback .go-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected fallback .go-version for Railpack", "version", project.LanguageVersion)
				}
			}
		} else if _, err := os.Stat(filepath.Join(buildPath, "requirements.txt")); err == nil {
			versionFile := filepath.Join(buildPath, ".python-version")
			if _, e := os.Stat(versionFile); os.IsNotExist(e) {
				if err := os.WriteFile(versionFile, []byte(project.LanguageVersion), 0644); err != nil {
					slog.Warn("Failed to inject fallback .python-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected fallback .python-version for Railpack", "version", project.LanguageVersion)
				}
			}
		} else if _, err := os.Stat(filepath.Join(buildPath, "package.json")); err == nil {
			versionFile := filepath.Join(buildPath, ".node-version")
			if _, e := os.Stat(versionFile); os.IsNotExist(e) {
				if err := os.WriteFile(versionFile, []byte(project.LanguageVersion), 0644); err != nil {
					slog.Warn("Failed to inject fallback .node-version for Railpack", "error", err, "subdomain", project.Subdomain)
				} else {
					slog.Info("Injected fallback .node-version for Railpack", "version", project.LanguageVersion)
				}
			}
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
	packageJSONs := []string(nil)
	if usesNodeBuildTemplate(project.Framework) {
		packageJSONs = s.findNestedPackageJSONs(buildPath, 1, 3)
	}
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

		isNextJS := false
		isNuxtJS := false
		hasStartScript := false
		hasBuildScript := false
		data, err := os.ReadFile(targetPackageJSON)
		if err == nil {
			content := string(data)
			hasStartScript = strings.Contains(content, "\"start\"") || strings.Contains(content, "'start'")
			hasBuildScript = strings.Contains(content, "\"build\"") || strings.Contains(content, "'build'")
			isNextJS = strings.Contains(content, "\"next\"")
			isNuxtJS = strings.Contains(content, "\"nuxt\"")

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
							if project.Port == nil {
								project.Port = &pVal
								slog.Info("Parsed custom port override from package.json start script", "subdomain", project.Subdomain, "port", pVal)
							}

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

		isStaticFramework = shouldUseStaticNodeHosting(isStaticFramework, hasStartScript, hasBuildScript, hasViteConfig)
		if isStaticFramework {
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
	stack := ""
	if project.Framework == "Go" || project.Framework == "Golang" {
		stack = "golang"
	} else if project.Framework == "Python" {
		stack = "python"
	} else if isStaticFramework {
		stack = "static"
	} else if usesNodeBuildTemplate(project.Framework) {
		stack = "nodejs"
	} else if project.Framework == "" || project.Framework == "Other" {
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
	if _, cleanup, err := s.prepareNpmAuthConfig(buildPath, project); err != nil {
		return err
	} else if cleanup != nil {
		defer cleanup()
	}

	// Inject optimized railpack.json configuration using resolved commands and stack type
	if stack != "" {
		s.injectDefaultRailpackConfig(buildPath, envs["RAILPACK_BUILD_CMD"], envs["RAILPACK_START_CMD"], stack, project, staticDir, noCache)
	}
	if buildCommand := envs["RAILPACK_BUILD_CMD"]; buildCommand != "" {
		buildArgs = append(buildArgs, "--build-cmd", buildCommand)
	}
	if startCommand := envs["RAILPACK_START_CMD"]; startCommand != "" {
		buildArgs = append(buildArgs, "--start-cmd", startCommand)
	}

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

func usesNodeBuildTemplate(framework string) bool {
	switch framework {
	case "Node.js", "Next.js", "Vite", "React", "Vue", "Nuxt.js", "Svelte", "Angular", "TypeScript":
		return true
	default:
		return false
	}
}

func shouldUseStaticNodeHosting(frameworkHint, hasStartScript, hasBuildScript, hasViteConfig bool) bool {
	return !hasStartScript && (frameworkHint || hasBuildScript || hasViteConfig)
}

func (s *DockerService) validateImageStartupArtifacts(imageName string) error {
	res, err := utils.Run(30*time.Second, "docker", "image", "inspect", imageName)
	if err != nil {
		return fmt.Errorf("inspect runtime image: %w", err)
	}

	var images []struct {
		Config struct {
			Entrypoint []string `json:"Entrypoint"`
			Cmd        []string `json:"Cmd"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &images); err != nil || len(images) != 1 {
		return fmt.Errorf("decode runtime image metadata")
	}
	if !commandReferencesPath(images[0].Config.Entrypoint, images[0].Config.Cmd, "/start-container.sh") {
		return nil
	}

	_, err = utils.Run(30*time.Second, "docker", "run", "--rm",
		"--network", "none",
		"--read-only",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		"--pids-limit", "16",
		"--memory", "64m",
		"--entrypoint", "/bin/sh",
		imageName, "-c", "test -x /start-container.sh",
	)
	if err != nil {
		return fmt.Errorf("required startup file /start-container.sh is missing or not executable")
	}
	return nil
}

func commandReferencesPath(entrypoint, command []string, path string) bool {
	for _, argument := range append(append([]string{}, entrypoint...), command...) {
		for _, token := range strings.Fields(argument) {
			if strings.Trim(token, "'\"") == path {
				return true
			}
		}
	}
	return false
}

func (s *DockerService) prepareNpmAuthConfig(buildPath string, project *models.Project) (string, func(), error) {
	projectEnvPath := filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), ".env")
	envVars, err := s.ParseProjectEnv(projectEnvPath)
	if err != nil || envVars["NODE_AUTH_TOKEN"] == "" {
		return "", nil, nil
	}

	npmrcPath := filepath.Join(buildPath, ".npmrc")
	var previousContent []byte
	var previousMode os.FileMode = 0644
	if stat, err := os.Stat(npmrcPath); err == nil {
		previousMode = stat.Mode().Perm()
		previousContent, err = os.ReadFile(npmrcPath)
		if err != nil {
			return "", nil, fmt.Errorf("failed read existing npm config: %w", err)
		}
	}

	npmrcLines := []string{strings.TrimSpace(string(previousContent))}
	for _, scope := range s.detectGithubPackageScopes(buildPath, envVars) {
		npmrcLines = append(npmrcLines, fmt.Sprintf("@%s:registry=https://npm.pkg.github.com", scope))
	}
	npmrcLines = append(npmrcLines,
		fmt.Sprintf("//npm.pkg.github.com/:_authToken=%s", envVars["NODE_AUTH_TOKEN"]),
		"always-auth=true",
	)
	content := strings.TrimSpace(strings.Join(npmrcLines, "\n")) + "\n"
	if err := os.WriteFile(npmrcPath, []byte(content), 0600); err != nil {
		return "", nil, fmt.Errorf("failed create npm auth config: %w", err)
	}

	return npmrcPath, func() {
		if previousContent != nil {
			_ = os.WriteFile(npmrcPath, previousContent, previousMode)
			return
		}
		_ = os.Remove(npmrcPath)
	}, nil
}

func (s *DockerService) detectGithubPackageScopes(buildPath string, envVars map[string]string) []string {
	configuredScopes := strings.TrimSpace(envVars["NPM_GITHUB_SCOPES"])
	if configuredScopes != "" {
		return splitNpmScopes(configuredScopes)
	}

	scopes := make(map[string]struct{})
	for _, pkg := range detectGithubPackagesFromLockfile(filepath.Join(buildPath, "package-lock.json")) {
		if scope := npmScope(pkg); scope != "" {
			scopes[scope] = struct{}{}
		}
	}
	if len(scopes) == 0 {
		for _, scope := range detectScopedPackagesFromPackageJSON(filepath.Join(buildPath, "package.json")) {
			scopes[scope] = struct{}{}
		}
	}

	result := make([]string, 0, len(scopes))
	for scope := range scopes {
		result = append(result, scope)
	}
	return result
}

func detectGithubPackagesFromLockfile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "npm.pkg.github.com") {
		return nil
	}

	type lockPackage struct {
		Resolved string `json:"resolved"`
	}
	type lockfile struct {
		Packages     map[string]lockPackage `json:"packages"`
		Dependencies map[string]lockPackage `json:"dependencies"`
	}
	var lock lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil
	}

	packages := make([]string, 0)
	for path, pkg := range lock.Packages {
		if strings.Contains(pkg.Resolved, "npm.pkg.github.com") {
			packages = append(packages, strings.TrimPrefix(path, "node_modules/"))
		}
	}
	for name, pkg := range lock.Dependencies {
		if strings.Contains(pkg.Resolved, "npm.pkg.github.com") {
			packages = append(packages, name)
		}
	}
	return packages
}

func detectScopedPackagesFromPackageJSON(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	scopePattern := regexp.MustCompile(`"(@[A-Za-z0-9][A-Za-z0-9._-]*/[^"/]+)"\s*:`)
	packages := make([]string, 0)
	for _, match := range scopePattern.FindAllStringSubmatch(string(data), -1) {
		packages = append(packages, match[1])
	}
	return packages
}

func npmScope(packageName string) string {
	if !strings.HasPrefix(packageName, "@") {
		return ""
	}
	parts := strings.SplitN(strings.TrimPrefix(packageName, "@"), "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func splitNpmScopes(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		scope := strings.TrimPrefix(strings.TrimSpace(part), "@")
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes
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

	// 2. Skip configuration injection completely for Go/Golang to use Railpack's native builder directly.
	if stack == "golang" {
		slog.Info("Go/Golang stack detected, skipping config injection to use direct Railpack builder", "subdomain", project.Subdomain)
		return
	}

	// 3. Load the corresponding railpack.json from the stack directory under cfg.RailpacksPath
	templatePath := filepath.Join(s.cfg.RailpacksPath, stack, "railpack.json")
	templateData, err := os.ReadFile(templatePath)
	if err != nil {
		slog.Error("Failed to read railpack template, using minimal fallback", "path", templatePath, "error", err)
		templateData = []byte(`{"deploy":{"base":{"image":"debian:bookworm-slim"}}}`)
	}

	// 4. Perform version substitution
	version := "20" // default Node
	switch stack {
	case "golang":
		version = "1.22"
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

		// 5. Overwrite steps.build.commands and deploy.startCommand with user custom inputs if configured
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

		// Add support for tsc build cache if applicable
		if stack == "nodejs" {
			hasTsConfig := false
			tsconfigPath := filepath.Join(buildPath, "tsconfig.json")
			if _, err := os.Stat(tsconfigPath); err == nil {
				hasTsConfig = true
			}
			if hasTsConfig && buildCmd != "" {
				// Check if tsconfig.json has incremental set to true
				hasIncremental := false
				if data, err := os.ReadFile(tsconfigPath); err == nil {
					content := string(data)
					if strings.Contains(content, `"incremental": true`) || strings.Contains(content, `"incremental":true`) {
						hasIncremental = true
					}
				}

				if !hasIncremental {
					if _, exists := config["variables"]; !exists {
						config["variables"] = make(map[string]interface{})
					}
					if vars, ok := config["variables"].(map[string]interface{}); ok {
						vars["TSCONFIG_INCREMENTAL"] = "true"
						vars["TSCONFIG_TSBUILDINFO"] = ".tsbuildinfo"
						modified = true
					}
				}

				// Only augment real tsc invocations, not npm run build strings.
				cmds := strings.Split(buildCmd, "&&")
				var trimmedCmds []interface{}
				for _, c := range cmds {
					c = strings.TrimSpace(c)
					if strings.HasPrefix(c, "tsc") && !strings.Contains(c, "--incremental") {
						// Explicitly specify tsbuildinfo path to match railpack.json cache definition
						c += " --incremental --tsBuildInfoFile .tsbuildinfo"
					}
					trimmedCmds = append(trimmedCmds, c)
				}
				if steps, ok := config["steps"].(map[string]interface{}); ok {
					if buildStep, ok := steps["build"].(map[string]interface{}); ok {
						buildStep["commands"] = trimmedCmds
						modified = true
					}
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

		// 6. Cache Busting: Inject a unique build ID only when noCache = true
		if noCache {
			if _, exists := config["variables"]; !exists {
				config["variables"] = make(map[string]interface{})
			}
			if vars, ok := config["variables"].(map[string]interface{}); ok {
				vars["PAAS_BUILD_ID"] = fmt.Sprintf("%d", time.Now().Unix())
				modified = true
			}
		}

		// Inject PORT based on project configuration to ensure deploy phase uses the correct port
		if project.Port != nil {
			if _, exists := config["deploy"]; !exists {
				config["deploy"] = make(map[string]interface{})
			}
			if deploy, ok := config["deploy"].(map[string]interface{}); ok {
				if _, exists := deploy["variables"]; !exists {
					deploy["variables"] = make(map[string]interface{})
				}
				if vars, ok := deploy["variables"].(map[string]interface{}); ok {
					vars["PORT"] = fmt.Sprintf("%d", *project.Port)
					modified = true
				}
			}
		}

		if modified {
			if newData, err := json.MarshalIndent(config, "", "  "); err == nil {
				templateData = newData
			}
		}
	}

	// 7. Write the refined railpack.json atomically to avoid race conditions
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

func (s *DockerService) injectDockerIgnore(projectPath string, logCallback func(string)) {
	ignorePath := filepath.Join(projectPath, ".dockerignore")
	templatePath := filepath.Join("/app/docker/templates", ".dockerignore")
	// Fallback to local path for dev environment
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		templatePath = "docker/templates/.dockerignore"
	}

	shouldScan := false
	if currentData, err := os.ReadFile(ignorePath); err == nil {
		// User already has a .dockerignore. Check if it's our older template.
		// The old template had "/build/" and "/dist/" but the new one does not.
		// If they have our exact old signature header, or specifically the old build exclusions, we overwrite.
		content := string(currentData)
		if strings.Contains(content, "# Runara - Docker Ignore File") && strings.Contains(content, "/build/") && strings.Contains(content, "/dist/") {
			slog.Info("Found older generated .dockerignore template, updating to new version to improve caching", "path", ignorePath)
		} else {
			shouldScan = true
		}
	}

	if !shouldScan {
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
		shouldScan = true
	}

	if shouldScan && logCallback != nil {
		if data, err := os.ReadFile(ignorePath); err == nil {
			content := string(data)
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "dist/") || strings.HasPrefix(line, "/dist/") ||
					strings.HasPrefix(line, "build/") || strings.HasPrefix(line, "/build/") ||
					line == "dist" || line == "/dist" || line == "build" || line == "/build" {
					logCallback(">> WARNING: Your .dockerignore excludes 'dist/' or 'build/'. TypeScript compiled output will not be cached between builds, increasing build times. Consider removing these exclusions.")
					break
				}
			}
		}
	}
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
