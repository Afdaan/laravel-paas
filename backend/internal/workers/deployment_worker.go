// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package workers

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/pkg/utils"
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
			exec.Command("docker", "builder", "prune", "-a", "-f").Run()
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

		// Process the job
		w.wg.Add(1)
		sem <- struct{}{}

		go func(j *infrastructure.DeploymentJob) {
			defer w.wg.Done()
			defer func() { <-sem }()
			w.processDeployment(j)
		}(job)
	}
}

// processDeployment handles a single deployment job
func (w *DeploymentWorker) processDeployment(job *infrastructure.DeploymentJob) {
	slog.Info("Processing deployment job",
		"type", job.Type,
		"projectId", job.ProjectID,
		"queuedDuration", time.Since(job.EnqueuedAt).Round(time.Second))

	// Try to acquire lock for this project
	locked, err := w.redisService.AcquireDeploymentLock(job.ProjectID, 30*time.Minute)
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
	w.deployProject(project)
	duration := time.Since(startTime)

	slog.Info("Completed deployment job",
		"type", job.Type,
		"projectId", project.ID,
		"projectName", project.Name,
		"duration", duration.Round(time.Second))

	w.redisService.IncrementDeploymentCounter("processed")
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(project *models.Project) {
	// Update status to building and clear old error logs
	project.Status = models.StatusBuilding
	project.ErrorLog = nil
	if err := w.projectRepo.Update(project); err != nil {
		slog.Error("Failed to update project status to building", "id", project.ID, "error", err)
	}

	// Step 1: Clone repository
	projectPath, err := w.gitService.CloneRepository(project.GithubURL, project.Branch, project.Subdomain)
	if err != nil {
		w.updateProjectError(project, "Failed to clone repository: "+err.Error())
		return
	}

	// Step 2: Detect Laravel version
	laravelVersion, phpVersion, err := w.versionService.DetectVersions(projectPath)
	if err != nil {
		w.updateProjectError(project, "Failed to detect Laravel version: "+err.Error())
		return
	}

	// Use manual PHP version if set, otherwise use detected version
	finalPHPVersion := phpVersion
	if project.IsManualVersion && project.PHPVersion != "" {
		finalPHPVersion = project.PHPVersion
	}

	project.LaravelVersion = laravelVersion
	project.PHPVersion = finalPHPVersion
	if err := w.projectRepo.Update(project); err != nil {
		slog.Warn("Failed to update project versions", "id", project.ID, "error", err)
	}

	// Step 3: Create student database
	// Generate a unique crypto-random password if not already set
	if project.DatabasePassword == "" {
		project.DatabasePassword = utils.GeneratePassword(16)
		w.projectRepo.Update(project)
	}

	if err := w.mysqlService.CreateDatabase(project.DatabaseName, project.DatabasePassword); err != nil {
		w.updateProjectError(project, "Failed to create database: "+err.Error())
		return
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
	newContainerID, err := w.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit)

	if err != nil {
		w.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Step 5: Run database migrations
	slog.Info("Running database migrations", "subdomain", project.Subdomain)
	if output, err := w.dockerService.RunMigrations(newContainerID); err != nil {
		slog.Error("Migrations failed", "subdomain", project.Subdomain, "error", err)
		// Cleanup failed new container
		if err := w.dockerService.RemoveContainer(newContainerID); err != nil {
			slog.Warn("Failed to cleanup failed container after migration failure", "id", newContainerID, "error", err)
		}
		w.updateProjectError(project, "Migrations failed: "+err.Error()+"\n\nOutput:\n"+output)
		return
	}

	// Sync Redis Proxy Cache
	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to cache subdomain mapping", "subdomain", project.Subdomain, "error", err)
	}

	// Step 6: Sync config to remote Nginx
	if err := w.nginxService.SyncProject(project, projectDomain); err != nil {
		slog.Error("Nginx sync failed", "subdomain", project.Subdomain, "error", err)
		// Cleanup failed new container
		if err := w.dockerService.RemoveContainer(newContainerID); err != nil {
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
			if err := w.dockerService.RemoveContainer(*oldContainerID); err != nil {
				slog.Warn("Failed to remove old container after switch", "id", *oldContainerID, "error", err)
			}
		}()
	}
}

// updateProjectError sets project status to failed
func (w *DeploymentWorker) updateProjectError(project *models.Project, errorMsg string) {
	project.Status = models.StatusFailed
	msg := errorMsg
	project.ErrorLog = &msg
	w.projectRepo.Update(project)
	w.projectService.InvalidateSubdomainCache(project.Subdomain)
	w.redisService.IncrementDeploymentCounter("failed")
}

// getSetting helper to get a setting value from service
func (w *DeploymentWorker) getSetting(key string, defaultValue string) string {
	return w.settingService.Get(key, defaultValue)
}
