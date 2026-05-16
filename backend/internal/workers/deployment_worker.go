// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package workers

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/infrastructure/docker"
	"github.com/laravel-paas/backend/internal/infrastructure/nginx"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
	"github.com/laravel-paas/backend/internal/repositories"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/backend/internal/services/setting"
)

// DeploymentWorker processes deployment jobs from the queue
type DeploymentWorker struct {
	cfg            *config.Config
	projectService *projectServicePkg.ProjectService
	dockerService  *docker.DockerService
	gitService     *infrastructure.GitService
	versionService *infrastructure.VersionService
	mysqlService   *infrastructure.MySQLService
	redisService   *infrastructure.RedisService
	nginxService   *nginx.NginxWebhookService
	projectRepo    repositories.ProjectRepository
	settingService *setting.SettingService

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewDeploymentWorker creates a new deployment worker
func NewDeploymentWorker(
	cfg *config.Config,
	projectRepo repositories.ProjectRepository,
	settingService *setting.SettingService,
	redisService *infrastructure.RedisService,
	dockerService *docker.DockerService,
	gitService *infrastructure.GitService,
	versionService *infrastructure.VersionService,
	mysqlService *infrastructure.MySQLService,
	projectService *projectServicePkg.ProjectService,
) *DeploymentWorker {
	return &DeploymentWorker{
		cfg:            cfg,
		projectService: projectService,
		dockerService:  dockerService,
		gitService:     gitService,
		versionService: versionService,
		mysqlService:   mysqlService,
		redisService:   redisService,
		nginxService:   nginx.NewNginxWebhookService(cfg),
		projectRepo:    projectRepo,
		settingService: settingService,
		running:        false,
		stopChan:       make(chan struct{}),
	}
}

// Start begins processing jobs in the background
func (w *DeploymentWorker) Start() {
	if w.running {
		return
	}
	w.running = true
	slog.Info("Starting deployment worker daemon...")

	go w.processJobs()
}

// Stop signals the worker to shut down gracefully (SIGTERM drain mode)
func (w *DeploymentWorker) Stop() {
	if !w.running {
		return
	}
	slog.Info("Worker draining: waiting for active jobs to finish before shutdown...")

	w.running = false
	close(w.stopChan)
	w.wg.Wait()

	slog.Info("Worker stopped gracefully.")
}

// processJobs continuously processes jobs from the queue
func (w *DeploymentWorker) processJobs() {
	// Semaphore channel for concurrent builds
	var currentMax int
	var sem chan struct{}

	updateSemaphore := func() {
		// Fetch max concurrent builds from settings
		maxStr := w.getSetting(models.SettingMaxConcurrent, models.DefaultMaxConcurrent)

		maxWorkers, err := strconv.Atoi(maxStr)
		if err != nil {
			slog.Warn("Failed to parse max_concurrent_builds, defaulting to 3",
				"value", maxStr,
				"error", err)
			maxWorkers = 3
		}

		if maxWorkers < 1 {
			maxWorkers = 1
		}

		if maxWorkers != currentMax {
			currentMax = maxWorkers
			sem = make(chan struct{}, currentMax)
		}
	}

	updateSemaphore()

	for w.running {
		// Update semaphore config dynamically just in case it changes
		updateSemaphore()

		// Wait for next job with 5 second timeout
		job, err := w.redisService.DequeueDeployment(5 * time.Second)

		// If we're stopping, don't start new jobs
		if !w.running {
			// If we got a job but we're stopping, put it back in the queue
			if job != nil {
				if err := w.redisService.EnqueueDeployment(job.ProjectID, job.UserID, job.Type); err != nil {
					slog.Error("Failed to re-enqueue job during shutdown", "projectId", job.ProjectID, "error", err)
				}
			}
			break
		}

		if err != nil {
			slog.Error("Error dequeuing job", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if job == nil {
			continue
		}

		slog.Info("Worker dequeued new deployment job",
			"projectId", job.ProjectID,
			"type", job.Type)

		// Process the job
		w.wg.Add(1)
		localSem := sem
		localSem <- struct{}{}

		go func(j *infrastructure.DeploymentJob, sema chan struct{}) {
			defer w.wg.Done()
			defer func() { <-sema }()
			w.processDeployment(j)
		}(job, localSem)
	}
}

// processDeployment handles a single deployment job
func (w *DeploymentWorker) processDeployment(job *infrastructure.DeploymentJob) {
	slog.Info("Processing deployment job",
		"type", job.Type,
		"projectId", job.ProjectID,
		"queuedDuration", time.Since(job.EnqueuedAt).Round(time.Second))

	// Acquire short 2-minute lock with background heartbeat renewal
	locked, err := w.redisService.AcquireDeploymentLock(job.ProjectID, 2*time.Minute)
	if err != nil {
		slog.Error("Failed to acquire lock for project", "id", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed")
		return
	}

	if !locked {
		slog.Warn("Project is already being deployed, skipping", "id", job.ProjectID)
		w.redisService.IncrementDeploymentCounter("failed")
		return
	}

	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.redisService.RenewDeploymentLock(job.ProjectID, 2*time.Minute); err != nil {
					slog.Warn("Failed to renew deployment lock", "projectId", job.ProjectID, "error", err)
				}
			case <-stopHeartbeat:
				return
			}
		}
	}()

	// Ensure lock is released and heartbeat stopped after deployment
	defer func() {
		close(stopHeartbeat)
		if err := w.redisService.ReleaseDeploymentLock(job.ProjectID); err != nil {
			slog.Warn("Failed to release lock for project", "id", job.ProjectID, "error", err)
		}
	}()

	// Fetch project from database via repository
	project, err := w.projectRepo.GetByID(job.ProjectID)
	if err != nil {
		slog.Error("Failed to find project for deployment", "projectId", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed")
		return
	}

	// Execute deployment
	startTime := time.Now()
	w.deployProject(project, job)
	duration := time.Since(startTime)

	slog.Info("Completed deployment job",
		"type", job.Type,
		"projectId", project.ID,
		"projectName", project.Name,
		"duration", duration.Round(time.Second))

	w.redisService.IncrementDeploymentCounter("processed")
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(project *models.Project, job *infrastructure.DeploymentJob) {
	if job.Type == "update_env" && project.ContainerID != nil {
		slog.Info("Performing instant environment update", "subdomain", project.Subdomain)
		if err := w.instantUpdateEnv(project); err == nil {
			return
		}
		slog.Warn("Instant update failed, falling back to full deployment", "subdomain", project.Subdomain)
	}

	project.Status = models.StatusBuilding
	project.ErrorLog = nil
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update status to building", "id", project.ID, "error", err)
	}

	projectPath := filepath.Join(w.cfg.ProjectsPath, project.Subdomain)
	buildLogPath := filepath.Join(projectPath, "build.log")
	if err := os.MkdirAll(projectPath, 0755); err == nil {
		if err := os.WriteFile(buildLogPath, []byte(""), 0644); err != nil {
			slog.Warn("Failed to clear build log", "path", buildLogPath, "error", err)
		}
	}

	w.checkDiskSpace()

	latestHash, hashErr := w.gitService.GetRemoteCommitHash(project.GithubURL, project.Branch)
	if job.Type == "deploy" && hashErr == nil && latestHash != "" && project.LastCommitHash == latestHash && project.ContainerID != nil {
		slog.Info("Commit hash unchanged, checking for existing image", "subdomain", project.Subdomain, "hash", latestHash)

		imageName := fmt.Sprintf("paas-%s", project.Subdomain)
		checkImg, _ := exec.Command("docker", "image", "inspect", imageName).Output()
		if len(checkImg) > 0 {
			slog.Info("Valid image found, skipping build", "subdomain", project.Subdomain)
			if err := w.redeployExistingImage(project); err == nil {
				return
			}
		}
	}

	var cloneHash string
	var cloneErr error
	var dbErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(project.GithubURL, project.Branch, project.Subdomain)
	}()

	go func() {
		defer wg.Done()
		if project.DatabasePassword == "" {
			project.DatabasePassword = utils.GeneratePassword(16)
			if err := w.projectRepo.Update(project); err != nil {
				slog.Warn("Failed to save database password", "id", project.ID, "error", err)
			}
		}
		dbErr = w.mysqlService.CreateDatabase(project.DatabaseName, project.DatabasePassword)
	}()

	wg.Wait()

	if cloneErr != nil {
		w.updateProjectError(project, "Failed to clone repository: "+cloneErr.Error())
		return
	}
	if dbErr != nil {
		w.updateProjectError(project, "Failed to create database: "+dbErr.Error())
		return
	}

	project.LastCommitHash = cloneHash
	if err := w.projectRepo.Update(project); err != nil {
		slog.Warn("Failed to update commit hash", "id", project.ID, "error", err)
	}

	buildPath := w.dockerService.ResolveBuildPath(projectPath, project.BaseDirectory)

	type marker struct {
		file string
		name string
	}
	markers := []marker{
		{"artisan", "Laravel"},
		{"next.config.js", "Next.js"},
		{"next.config.mjs", "Next.js"},
		{"nuxt.config.js", "Nuxt.js"},
		{"nuxt.config.ts", "Nuxt.js"},
		{"vite.config.js", "Vite"},
		{"vite.config.ts", "Vite"},
		{"src/App.tsx", "React"},
		{"src/App.jsx", "React"},
		{"src/App.vue", "Vue"},
		{"src/main.js", "Node.js"},
		{"svelte.config.js", "Svelte"},
		{"angular.json", "Angular"},
		{"package.json", "Node.js"},
		{"tsconfig.json", "TypeScript"},
		{"go.mod", "Go"},
		{"requirements.txt", "Python"},
		{"main.py", "Python"},
		{"Gemfile", "Ruby"},
		{"Cargo.toml", "Rust"},
		{"pom.xml", "Java"},
		{"build.gradle", "Java"},
		{"composer.json", "PHP"},
		{"index.html", "Static"},
	}

	project.Framework = "Other"
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(buildPath, m.file)); err == nil {
			project.Framework = m.name
			break
		}
	}

	langVersion, _ := w.versionService.DetectRuntimeVersion(buildPath, project.Framework)
	project.LanguageVersion = langVersion

	if project.Framework == "Laravel" {
		laravelVersion, phpVersion, err := w.versionService.DetectVersions(buildPath)
		if err == nil {
			project.LaravelVersion = laravelVersion
			project.PHPVersion = phpVersion
		} else {
			project.PHPVersion = "8.4"
		}
	} else {
		isJS := false
		jsFrameworks := []string{"Node.js", "Next.js", "Vite", "React", "Vue", "Nuxt.js", "Svelte", "Angular", "TypeScript"}
		for _, f := range jsFrameworks {
			if project.Framework == f {
				isJS = true
				break
			}
		}
		if isJS {
			project.NodeVersion = langVersion
		}
	}

	if err := w.projectRepo.Update(project); err != nil {
		slog.Warn("Failed to update project metadata", "id", project.ID, "error", err)
	}

	finalPHPVersion := project.PHPVersion
	if finalPHPVersion == "" {
		finalPHPVersion = "8.4"
	}

	var oldContainerID *string
	if project.ContainerID != nil {
		oldHelp := *project.ContainerID
		oldContainerID = &oldHelp
	}

	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	cpuPercentStr := w.getSetting(models.SettingCPULimit, models.DefaultCPULimit)
	cpuPercent, _ := strconv.ParseFloat(cpuPercentStr, 64)
	cpuLimit := cpuPercent / 100.0

	memoryMB := w.getSetting(models.SettingMemoryLimit, models.DefaultMemoryLimit)
	memoryLimit := memoryMB + "m"

	appendLog := func(msg string) {
		if f, err := os.OpenFile(buildLogPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644); err == nil {
			_, _ = f.WriteString(msg + "\n")
			f.Close()
		}
		_ = w.redisService.PublishBuildLog(project.ID, msg)
	}

	newContainerID, err := w.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit, job.Type == "deploy", job.Type == "redeploy")
	if err != nil {
		appendLog("ERROR: Failed to deploy container: " + err.Error())
		w.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	appendLog("Starting application container and verifying health check...")

	// Wait for the new container to become healthy before proceeding
	isHealthy := false
	maxWait := 30
	for i := 0; i < maxWait; i++ {
		if w.dockerService.IsContainerHealthy(newContainerID) {
			isHealthy = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !isHealthy {
		slog.Error("New container failed health check, rolling back", "subdomain", project.Subdomain, "id", newContainerID)
		appendLog("ERROR: Deployment failed: new container is unhealthy or crashing. Rolling back.")
		
		if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to cleanup unhealthy deployment", "id", newContainerID, "error", err)
		}

		w.updateProjectError(project, "Deployment failed: new container is unhealthy or crashing. Old version is still running.")
		return
	}

	if project.Framework == "Laravel" {
		slog.Info("Running database migrations", "subdomain", project.Subdomain)
		appendLog("Running database migrations...")
		if output, err := w.dockerService.RunMigrations(newContainerID); err != nil {
			slog.Error("Migrations failed", "subdomain", project.Subdomain, "error", err)
			appendLog("ERROR: Migrations failed:\n" + output)
			if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
				slog.Warn("Failed to cleanup failed container", "id", newContainerID, "error", err)
			}
			w.updateProjectError(project, "Migrations failed: "+err.Error()+"\n\nOutput:\n"+output)
			return
		}
	}

	appendLog("Deployment completed successfully! System is now live.")

	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to cache subdomain mapping", "subdomain", project.Subdomain, "error", err)
	}

	if err := w.nginxService.SyncProject(project, projectDomain); err != nil {
		slog.Error("Nginx sync failed", "subdomain", project.Subdomain, "error", err)
	}

	project.Status = models.StatusRunning
	project.ContainerID = &newContainerID
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project status", "id", project.ID, "error", err)
	}

	if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate cache", "subdomain", project.Subdomain, "error", err)
	}

	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to update subdomain cache", "subdomain", project.Subdomain, "error", err)
	}

	if oldContainerID != nil {
		slog.Info("Cleaning up legacy instance", "subdomain", project.Subdomain)
		time.Sleep(2 * time.Second)
		if err := w.dockerService.RemoveContainer(*oldContainerID, nil); err != nil {
			slog.Warn("Failed to remove legacy container", "id", *oldContainerID, "error", err)
		}
	}

	go func() {
		utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f")
		utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f")
	}()
}


// updateProjectError sets project status to failed
func (w *DeploymentWorker) updateProjectError(project *models.Project, errorMsg string) {
	project.Status = models.StatusFailed
	msg := errorMsg
	project.ErrorLog = &msg
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project status on error", "id", project.ID, "error", err)
	}
	if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate cache on error", "subdomain", project.Subdomain, "error", err)
	}
	w.redisService.IncrementDeploymentCounter("failed")
}

// getSetting helper to get a setting value from service
func (w *DeploymentWorker) getSetting(key string, defaultValue string) string {
	return w.settingService.Get(key, defaultValue)
}

// checkDiskSpace performs a pre-build check to ensure the server has enough space.
func (w *DeploymentWorker) checkDiskSpace() {
	res, err := utils.Run(5*time.Second, "df", "-h", w.cfg.ProjectsPath)
	if err != nil {
		return
	}

	lines := strings.Split(res.Stdout, "\n")
	if len(lines) < 2 {
		return
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return
	}

	usageStr := strings.TrimSuffix(fields[4], "%")
	usage, _ := strconv.Atoi(usageStr)

	if usage > 90 {
		slog.Warn("Disk space critical, performing emergency prune", "usage", usage)
		// Immediate cleanup
		if err := w.dockerService.PruneImages(); err != nil {
			slog.Warn("Emergency image prune failed", "error", err)
		}
		if err := utils.RunSilent(5*time.Minute, "docker", "builder", "prune", "-a", "-f"); err != nil {
			slog.Warn("Emergency builder prune failed", "error", err)
		}
		if err := utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f"); err != nil {
			slog.Warn("Emergency volume prune failed", "error", err)
		}
	}
}

func (w *DeploymentWorker) instantUpdateEnv(project *models.Project) error {
	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		return err
	}

	return w.projectService.RecreateProjectZeroDowntime(project)
}

func (w *DeploymentWorker) redeployExistingImage(project *models.Project) error {
	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	// Refresh .env before restart
	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		slog.Warn("Failed to refresh environment file during redeploy", "id", project.ID, "error", err)
	}

	return w.projectService.RecreateProjectZeroDowntime(project)
}

