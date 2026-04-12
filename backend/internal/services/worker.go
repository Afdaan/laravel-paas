// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package services

import (
	"log/slog"
	"os/exec"
	"strconv"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	"gorm.io/gorm"
)

// DeploymentWorker processes deployment jobs from the queue
type DeploymentWorker struct {
	db             *gorm.DB
	cfg            *config.Config
	projectService *ProjectService
	dockerService  *DockerService
	redisService   *RedisService
	nginxService   *NginxWebhookService
	projectRepo    repositories.ProjectRepository
	settingService *SettingService
	running        bool
}

// NewDeploymentWorker creates a new deployment worker
func NewDeploymentWorker(db *gorm.DB, cfg *config.Config, redisService *RedisService) *DeploymentWorker {
	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	settingService := NewSettingService(settingRepo)
	
	return &DeploymentWorker{
		db:             db,
		cfg:            cfg,
		projectService: NewProjectService(cfg, projectRepo, settingService, redisService),
		dockerService:  NewDockerService(cfg),
		redisService:   redisService,
		nginxService:   NewNginxWebhookService(cfg),
		projectRepo:    projectRepo,
		settingService: settingService,
		running:        false,
	}
}

// Start begins processing jobs from the queue
func (w *DeploymentWorker) Start() {
	if w.running {
		slog.Warn("Worker already running")
		return
	}

	w.running = true
	slog.Info("Deployment worker started")

	// Run recovery for any interrupted builds
	w.recoverOrphanedBuilds()

	slog.Info("Waiting for deployment jobs...")

	// Start the daily 3 AM Docker cache cleanup
	w.StartPruneScheduler()

	// Start hourly project expiry janitor
	w.StartExpiryJanitor()

	go w.processJobs()
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
			w.dockerService.PruneImages()
			
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
	var expiredProjects []models.Project
	now := time.Now()
	
	// Find projects where ExpiresAt is in the past
	expiredProjects, err := w.projectRepo.ListAll() // Simplified: ListAll and filter or add specific repo method
	if err != nil {
		slog.Error("Failed to query expired projects", "error", err)
		return
	}
	
	if len(expiredProjects) == 0 {
		return
	}
	
	slog.Info("Checking Expired projects for automated cleanup", "count", len(expiredProjects))
	
	for i := range expiredProjects {
		project := expiredProjects[i]
		if project.ExpiresAt != nil && project.ExpiresAt.Before(now) {
			slog.Info("Auto-deleting expired project via service", "name", project.Name, "id", project.ID)
			
			if err := w.projectService.DeleteProject(&project); err != nil {
				slog.Error("Failed to auto-delete expired project", "id", project.ID, "error", err)
			}
		}
	}
	
	// Global prune to cleanup any leftover layers
	go w.dockerService.PruneImages()
}

// Stop stops the worker
func (w *DeploymentWorker) Stop() {
	w.running = false
	slog.Info("Deployment worker stopped")
}

// recoverOrphanedBuilds finds projects left in "building" state due to unexpected shutdown
// and pushes them back into the redis queue as "queued"
func (w *DeploymentWorker) recoverOrphanedBuilds() {
	orphanedProjects, err := w.projectRepo.ListAll() // Filter for StatusBuilding
	if err != nil {
		slog.Error("Failed to query orphaned builds", "error", err)
		return
	}

	if len(orphanedProjects) > 0 {
		slog.Info("Recovering orphaned builds from previous session", "count", len(orphanedProjects))
		
		for i := range orphanedProjects {
			project := orphanedProjects[i]
			if project.Status == models.StatusBuilding {
				// 1. Reset Status in DB to Queued
				project.Status = models.StatusQueued
				recoveryLog := "Recovered from unexpected server shutdown."
				project.ErrorLog = &recoveryLog
				w.projectRepo.Update(&project)

			// 2. Clear existing lock if any
			w.redisService.ReleaseDeploymentLock(project.ID)

				// 3. Re-enqueue to Redis (Assume 'redeploy' for full clean spin-up)
				if err := w.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
					slog.Error("Failed to re-queue project", "id", project.ID, "error", err)
				} else {
					slog.Info("Project automatically re-queued", "id", project.ID)
				}
			}
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
		if err != nil {
			slog.Error("Error dequeuing job", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// No job available, continue waiting
		if job == nil {
			continue
		}

		// Acquire semaphore token
		sem <- struct{}{}

		// Process the job in a goroutine
		go func(j *DeploymentJob) {
			defer func() { <-sem }() // Release semaphore token
			w.processDeployment(j)
		}(job)
	}
}

// processDeployment handles a single deployment job
func (w *DeploymentWorker) processDeployment(job *DeploymentJob) {
	slog.Info("Processing deployment job", 
		"type", job.Type, 
		"projectId", job.ProjectID, 
		"queuedDuration", time.Since(job.EnqueuedAt).Round(time.Second))

	// Try to acquire lock for this project
	locked, err := w.redisService.AcquireDeploymentLock(job.ProjectID, 30*time.Minute)
	if err != nil {
		slog.Error("Failed to acquire lock for project", "id", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed_lock")
		return
	}

	if !locked {
		slog.Warn("Project is already being deployed, skipping", "id", job.ProjectID)
		w.redisService.IncrementDeploymentCounter("skipped_locked")
		return
	}

	// Ensure lock is released after deployment
	defer func() {
		if err := w.redisService.ReleaseDeploymentLock(job.ProjectID); err != nil {
			slog.Warn("Failed to release lock for project", "id", job.ProjectID, "error", err)
		}
	}()

	// Fetch project from database with User preloaded
	var project models.Project
	if err := w.db.Preload("User").First(&project, job.ProjectID).Error; err != nil {
		slog.Error("Failed to find project", "id", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed_not_found")
		return
	}

	// Execute deployment
	startTime := time.Now()
	w.deployProject(&project)
	duration := time.Since(startTime)

	slog.Info("Completed deployment job", 
		"type", job.Type, 
		"projectId", project.ID, 
		"projectName", project.Name, 
		"duration", duration.Round(time.Second))

	w.redisService.IncrementDeploymentCounter("completed")
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(project *models.Project) {
	// Update status to building and clear old error logs
	project.Status = models.StatusBuilding
	project.ErrorLog = nil
	w.projectRepo.Update(project)

	// Step 1: Clone repository
	projectPath, err := w.dockerService.CloneRepository(project.GithubURL, project.Branch, project.Subdomain)
	if err != nil {
		w.updateProjectError(project, "Failed to clone repository: "+err.Error())
		return
	}

	// Step 2: Detect Laravel version
	laravelVersion, phpVersion, err := w.dockerService.DetectVersions(projectPath)
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
	w.projectRepo.Update(project)

	// Step 3: Create database
	if err := w.dockerService.CreateDatabase(project.DatabaseName); err != nil {
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
	containerID, err := w.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit)

	if err != nil {
		w.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Update project as running with new container ID
	project.Status = models.StatusRunning
	project.ContainerID = &containerID
	w.projectRepo.Update(project)

	// Sync Redis Proxy Cache
	w.projectService.CacheSubdomainMapping(project)

	// Sync config to remote Nginx
	w.nginxService.SyncProject(project, projectDomain)

	// Cleanup old container after successful switch
	if oldContainerID != nil {
		go func() {
			w.dockerService.RemoveContainer(*oldContainerID)
		}()
	}
}

// updateProjectError sets project status to failed
func (w *DeploymentWorker) updateProjectError(project *models.Project, errorMsg string) {
	project.Status = models.StatusFailed
	msg := errorMsg
	project.ErrorLog = &msg
	w.projectRepo.Update(project)
	w.redisService.IncrementDeploymentCounter("failed_deployment")
}

// getSetting helper to get a setting value from service
func (w *DeploymentWorker) getSetting(key string, defaultValue string) string {
	return w.projectService.GetSetting(key, defaultValue)
}

