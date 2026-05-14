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
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
	"github.com/laravel-paas/backend/internal/repositories"
	"github.com/laravel-paas/backend/internal/services"
)

// DeploymentWorker processes deployment jobs from the queue
type DeploymentWorker struct {
	cfg            *config.Config
	projectService *services.ProjectService
	dockerService  *infrastructure.DockerService
	gitService     *infrastructure.GitService
	versionService *infrastructure.VersionService
	mysqlService   *infrastructure.MySQLService
	redisService   *infrastructure.RedisService
	nginxService   *infrastructure.NginxWebhookService
	projectRepo    repositories.ProjectRepository
	settingService *services.SettingService

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewDeploymentWorker creates a new deployment worker
func NewDeploymentWorker(
	cfg *config.Config,
	projectRepo repositories.ProjectRepository,
	settingService *services.SettingService,
	redisService *infrastructure.RedisService,
	dockerService *infrastructure.DockerService,
	gitService *infrastructure.GitService,
	versionService *infrastructure.VersionService,
	mysqlService *infrastructure.MySQLService,
	projectService *services.ProjectService,
) *DeploymentWorker {
	return &DeploymentWorker{
		cfg:            cfg,
		projectService: projectService,
		dockerService:  dockerService,
		gitService:     gitService,
		versionService: versionService,
		mysqlService:   mysqlService,
		redisService:   redisService,
		nginxService:   infrastructure.NewNginxWebhookService(cfg),
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
	slog.Info("Starting deployment worker system...")

	// Recover orphaned Deletions
	w.recoverOrphanedDeletions()

	// Recover orphaned jobs on startup
	w.recoverOrphanedBuilds()

	// Start threads
	w.StartPruneScheduler()
	w.StartExpiryJanitor()

	go w.processJobs()
}

// Stop signals the worker to shut down gracefully
func (w *DeploymentWorker) Stop() {
	if !w.running {
		return
	}
	slog.Info("Worker stopping: waiting for active jobs to finish...")

	// Signal stop and wait for all jobs in WG to finish
	w.running = false
	close(w.stopChan)
	w.wg.Wait()

	slog.Info("Worker stopped gracefully.")
}

// StartPruneScheduler starts a daily cron job to prune Docker images at 3 AM
func (w *DeploymentWorker) StartPruneScheduler() {
	go func() {
		for {
			now := time.Now()
			// Calculate time until 3 AM (local time)
			next3AM := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(next3AM) {
				next3AM = next3AM.Add(24 * time.Hour)
			}

			durationToWait := next3AM.Sub(now)
			slog.Info("Scheduled next Docker cache prune", "time", next3AM.Format("15:04:05"))

			time.Sleep(durationToWait)

			slog.Info("Executing 3 AM scheduled Docker cache prune")
			if err := w.dockerService.PruneImages(); err != nil {
				slog.Error("Scheduled image prune failed", "error", err)
			}

			// Optional: Aggressive BuildKit cache cleanup for absolute zero-cache state
			if err := exec.Command("docker", "builder", "prune", "-a", "-f").Run(); err != nil {
				slog.Warn("Failed to prune docker builder", "error", err)
			}
		}
	}()
}

// StartExpiryJanitor starts a periodic job to cleanup expired (inactive) projects
func (w *DeploymentWorker) StartExpiryJanitor() {
	go func() {
		// Wait a bit before first run to let system settle
		time.Sleep(1 * time.Minute)

		for w.running {
			slog.Info("Running project expiry janitor")
			w.cleanupExpiredProjects()

			// Run every hour
			time.Sleep(1 * time.Hour)
		}
	}()
}

// cleanupExpiredProjects finds and thoroughly deletes projects that have passed their inactivity limit
func (w *DeploymentWorker) cleanupExpiredProjects() {
	expiredProjects, err := w.projectRepo.ListExpired()
	if err != nil {
		slog.Error("Failed to query expired projects", "error", err)
		return
	}

	if len(expiredProjects) == 0 {
		return
	}

	slog.Info("Auto-deleting expired projects", "count", len(expiredProjects))

	for i := range expiredProjects {
		project := expiredProjects[i]
		slog.Info("Auto-deleting expired project via service", "name", project.Name, "id", project.ID)

		if err := w.projectService.DeleteProject(&project); err != nil {
			slog.Error("Failed to auto-delete expired project", "id", project.ID, "error", err)
		}
	}

	// Global prune to cleanup any leftover layers
	go func() {
		if err := w.dockerService.PruneImages(); err != nil {
			slog.Error("Background image prune failed", "error", err)
		}
	}()
}

// recoverOrphanedBuilds finds projects left in an inconsistent state due to unexpected shutdown
func (w *DeploymentWorker) recoverOrphanedBuilds() {
	statuses := []models.ProjectStatus{
		models.StatusBuilding,
		models.StatusQueued,
		models.StatusPending,
	}

	for _, status := range statuses {
		projects, err := w.projectRepo.ListByStatus(status)
		if err != nil {
			slog.Error("Failed to query orphaned projects for recovery", "status", status, "error", err)
			continue
		}

		if len(projects) == 0 {
			continue
		}

		slog.Info("Recovering orphaned projects from previous session",
			"status", status,
			"count", len(projects))

		for i := range projects {
			project := projects[i]

			// Check if already in queue to avoid duplicates
			isQueued, _ := w.redisService.IsProjectQueued(project.ID)
			if isQueued {
				slog.Info("Project is already in queue, skipping recovery", "id", project.ID)
				continue
			}

			// Reset status to queued
			project.Status = models.StatusQueued
			recoveryLog := fmt.Sprintf("Recovered from unexpected shutdown (previous status: %s).", status)
			project.ErrorLog = &recoveryLog
			if err := w.projectRepo.Update(&project); err != nil {
				slog.Error("Failed to update project during recovery", "id", project.ID, "error", err)
			}

			// Clear existing lock if any
			if err := w.redisService.ReleaseDeploymentLock(project.ID); err != nil {
				slog.Warn("Failed to release lock during recovery", "id", project.ID, "error", err)
			}

			// Re-enqueue
			if err := w.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
				slog.Error("Failed to re-queue project during recovery", "id", project.ID, "error", err)
			} else {
				slog.Info("Project automatically re-queued for reliability", "id", project.ID)
			}
		}
	}
}

// recoverOrphanedDeletions finds projects stuck in deleting state
func (w *DeploymentWorker) recoverOrphanedDeletions() {
	deletingProjects, err := w.projectRepo.ListByStatus(models.StatusDeleting)
	if err != nil {
		slog.Error("Failed to query orphaned deletions", "error", err)
		return
	}

	if len(deletingProjects) > 0 {
		slog.Info("Recovering orphaned deletions from previous session", "count", len(deletingProjects))
		for i := range deletingProjects {
			project := deletingProjects[i]
			slog.Info("Re-triggering project deletion", "id", project.ID)
			go func(p models.Project) {
				if err := w.projectService.DeleteProject(&p); err != nil {
					slog.Error("Failed to background delete orphaned project", "id", p.ID, "error", err)
				}
			}(project)
		}
	}
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

	// Try to acquire lock for this project
	// Build can take > 30m (clone + railpack build + docker run + migrations),
	// keep lock long enough to avoid concurrent deployments of same project.
	locked, err := w.redisService.AcquireDeploymentLock(job.ProjectID, 2*time.Hour)
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

	// Ensure lock is released after deployment
	defer func() {
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
	// ---------------------------------------------------------
	// OPTIMIZATION 1: Instant Updates (Skip Build for Env Changes)
	// ---------------------------------------------------------
	if job.Type == "update_env" && project.ContainerID != nil {
		slog.Info("Performing instant environment update (skipping build)", "subdomain", project.Subdomain)
		if err := w.instantUpdateEnv(project); err == nil {
			return // Success
		}
		slog.Warn("Instant update failed, falling back to full deployment", "subdomain", project.Subdomain)
	}

	// Update status to building and clear old error logs
	project.Status = models.StatusBuilding
	project.ErrorLog = nil
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project status to building", "id", project.ID, "error", err)
	}

	// Thoroughly clear the previous build log to ensure a fresh UI experience
	projectPath := filepath.Join(w.cfg.ProjectsPath, project.Subdomain)
	buildLogPath := filepath.Join(projectPath, "build.log")
	if err := os.MkdirAll(projectPath, 0755); err == nil {
		if err := os.WriteFile(buildLogPath, []byte(""), 0644); err != nil {
			slog.Warn("Failed to clear build log", "path", buildLogPath, "error", err)
		}
	}

	// Step 0: Check disk space (Pre-build guard)
	w.checkDiskSpace()

	// ---------------------------------------------------------
	// OPTIMIZATION 2: Commit-Hash Caching (Skip Build if same)
	// ---------------------------------------------------------
	latestHash, hashErr := w.gitService.GetRemoteCommitHash(project.GithubURL, project.Branch)
	if job.Type == "deploy" && hashErr == nil && latestHash != "" && project.LastCommitHash == latestHash && project.ContainerID != nil {
		slog.Info("Commit hash hasn't changed, checking if we can skip build", "subdomain", project.Subdomain, "hash", latestHash)
		// Check if image still exists
		imageName := fmt.Sprintf("paas-%s", project.Subdomain)
		checkImg, _ := exec.Command("docker", "image", "inspect", imageName).Output()
		if len(checkImg) > 0 {
			slog.Info("Image found, skipping build and redeploying existing image", "subdomain", project.Subdomain)
			if err := w.redeployExistingImage(project); err == nil {
				return // Success
			}
		}
	}

	// ---------------------------------------------------------
	// OPTIMIZATION 3: Parallel Task Execution (Clone & DB)
	// ---------------------------------------------------------
	var cloneHash string
	var cloneErr error
	var dbErr error
	var wg sync.WaitGroup

	wg.Add(2)
	// Task A: Clone Repository
	go func() {
		defer wg.Done()
		projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(project.GithubURL, project.Branch, project.Subdomain)
	}()

	// Task B: Create Student Database
	go func() {
		defer wg.Done()
		if project.DatabasePassword == "" {
			project.DatabasePassword = utils.GeneratePassword(16)
			if err := w.projectRepo.Update(project); err != nil {
				slog.Warn("Failed to save generated database password", "id", project.ID, "error", err)
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
		slog.Warn("Failed to update project commit hash", "id", project.ID, "error", err)
	}

	// Step 2: Detect Framework and Versions
	// We must detect from the build path (monorepo support + path traversal guard)
	buildPath := w.dockerService.ResolveBuildPath(projectPath, project.BaseDirectory)

	// Ordered list of marker files to framework names (Priority matters!)
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

	// Dynamic framework detection: First match wins
	project.Framework = "Other"
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(buildPath, m.file)); err == nil {
			project.Framework = m.name
			break
		}
	}

	// Runtime version detection (Generic for the future)
	langVersion, _ := w.versionService.DetectRuntimeVersion(buildPath, project.Framework)
	project.LanguageVersion = langVersion

	// Backward compatibility and specific field syncing
	if project.Framework == "Laravel" {
		laravelVersion, phpVersion, err := w.versionService.DetectVersions(buildPath)
		if err == nil {
			project.LaravelVersion = laravelVersion
			project.PHPVersion = phpVersion
			// Use manual PHP version if set
			if project.IsManualVersion && project.PHPVersion != "" {
				slog.Debug("Using manual PHP version", "subdomain", project.Subdomain, "version", project.PHPVersion)
			}
		} else {
			project.PHPVersion = "8.4" // Fallback
		}
	} else {
		// Sync NodeVersion if it's a JS project for backward compatibility in UI
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
		slog.Warn("Failed to update project framework/versions", "id", project.ID, "error", err)
	}

	finalPHPVersion := project.PHPVersion
	if finalPHPVersion == "" {
		finalPHPVersion = "8.4"
	}

	// Capture old container ID for cleanup after successful deployment
	var oldContainerID *string
	if project.ContainerID != nil {
		oldHelp := *project.ContainerID
		oldContainerID = &oldHelp
	}

	// Step 4: Build and run container
	// Project Domain (Global setting)
	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	// Resource Limits (Global settings)
	cpuPercentStr := w.getSetting(models.SettingCPULimit, models.DefaultCPULimit)
	cpuPercent, _ := strconv.ParseFloat(cpuPercentStr, 64)
	cpuLimit := cpuPercent / 100.0

	memoryMB := w.getSetting(models.SettingMemoryLimit, models.DefaultMemoryLimit)
	memoryLimit := memoryMB + "m"

	// Start deployment process
	newContainerID, err := w.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit, job.Type == "deploy", job.Type == "redeploy")

	if err != nil {
		w.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Step 5: Run database migrations (Only if artisan exists)
	if project.Framework == "Laravel" {
		slog.Info("Running database migrations", "subdomain", project.Subdomain)
		if output, err := w.dockerService.RunMigrations(newContainerID); err != nil {
			slog.Error("Migrations failed", "subdomain", project.Subdomain, "error", err)
			// Cleanup failed new container
			if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
				slog.Warn("Failed to cleanup failed container after migration failure", "id", newContainerID, "error", err)
			}
			w.updateProjectError(project, "Migrations failed: "+err.Error()+"\n\nOutput:\n"+output)
			return
		}
	} else {
		slog.Info("Skipping database migrations for non-Laravel framework", "subdomain", project.Subdomain, "framework", project.Framework)
	}

	// Sync Redis Proxy Cache
	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to cache subdomain mapping", "subdomain", project.Subdomain, "error", err)
	}

	// Step 6: Sync config to remote Nginx
	if err := w.nginxService.SyncProject(project, projectDomain); err != nil {
		slog.Error("Nginx sync failed", "subdomain", project.Subdomain, "error", err)
		// Cleanup failed new container
		if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to cleanup failed container after nginx sync failure", "id", newContainerID, "error", err)
		}
		w.updateProjectError(project, "Failed to sync Nginx configuration: "+err.Error())
		return
	}

	// Step 7: SUCCESS! Finalize and Cleanup Old version
	project.Status = models.StatusRunning
	project.ContainerID = &newContainerID
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project to running", "id", project.ID, "error", err)
	}
	if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate cache after success", "subdomain", project.Subdomain, "error", err)
	}

	// Sync Redis Proxy Cache (again with new ID/Status)
	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to update cached subdomain mapping", "subdomain", project.Subdomain, "error", err)
	}

	// Cleanup old container after successful switch
	if oldContainerID != nil {
		go func() {
			// Wait for the new container to become healthy before removing the old one
			maxWait := 30
			for i := 0; i < maxWait; i++ {
				if w.dockerService.IsContainerHealthy(newContainerID) {
					slog.Info("New container is healthy, switching traffic and cleaning up old container", "subdomain", project.Subdomain)
					time.Sleep(2 * time.Second) // Extra buffer for Traefik synchronization
					break
				}
				time.Sleep(1 * time.Second)
			}
			if err := w.dockerService.RemoveContainer(*oldContainerID, nil); err != nil {
				slog.Warn("Failed to remove old container after switch", "id", *oldContainerID, "error", err)
			}
		}()
	}

	// Step 8: Post-deployment cleanup (Real-time pruning)
	go func() {
		slog.Info("Performing post-deployment cleanup", "subdomain", project.Subdomain)
		if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f"); err != nil {
			slog.Warn("Background image prune failed", "error", err)
		}
		if err := utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f"); err != nil {
			slog.Warn("Background volume prune failed", "error", err)
		}
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

// instantUpdateEnv restarts the container with a new environment file without rebuilding.
func (w *DeploymentWorker) instantUpdateEnv(project *models.Project) error {
	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	// Update .env file
	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		return err
	}

	// Stop and remove current container
	if project.ContainerID != nil {
		if err := w.dockerService.RemoveContainer(*project.ContainerID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to remove container during instant update", "id", *project.ContainerID, "error", err)
		}
	}

	// Just start it again with the same image
	newID, err := w.dockerService.StartExistingImage(project, projectDomain)
	if err != nil {
		return err
	}

	project.ContainerID = &newID
	project.Status = models.StatusRunning
	return w.projectRepo.Update(project)
}

// redeployExistingImage restarts a project using its already built image.
func (w *DeploymentWorker) redeployExistingImage(project *models.Project) error {
	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	// Ensure .env is fresh
	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		slog.Warn("Failed to create env file for redeploy", "id", project.ID, "error", err)
	}

	// Step 4.5: Capture old ID
	var oldID *string
	if project.ContainerID != nil {
		tmp := *project.ContainerID
		oldID = &tmp
	}

	// Start new container from existing image
	newID, err := w.dockerService.StartExistingImage(project, projectDomain)
	if err != nil {
		return err
	}

	project.ContainerID = &newID
	project.Status = models.StatusRunning
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project status after redeploy", "id", project.ID, "error", err)
	}

	// Cleanup old if exists
	if oldID != nil {
		go func() {
			time.Sleep(5 * time.Second)
			if err := w.dockerService.RemoveContainer(*oldID, nil); err != nil {
				slog.Warn("Failed to remove old container after redeploy", "id", *oldID, "error", err)
			}
		}()
	}

	return nil
}
