// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package services

import (
	"log"
	"os/exec"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

// DeploymentWorker processes deployment jobs from the queue
type DeploymentWorker struct {
	db            *gorm.DB
	cfg           *config.Config
	dockerService *DockerService
	redisService  *RedisService
	running       bool
}

// NewDeploymentWorker creates a new deployment worker
func NewDeploymentWorker(db *gorm.DB, cfg *config.Config, redisService *RedisService) *DeploymentWorker {
	return &DeploymentWorker{
		db:            db,
		cfg:           cfg,
		dockerService: NewDockerService(cfg),
		redisService:  redisService,
		running:       false,
	}
}

// Start begins processing jobs from the queue
func (w *DeploymentWorker) Start() {
	if w.running {
		log.Println("[WARN] Worker already running")
		return
	}

	w.running = true
	log.Println("[INFO] Deployment worker started")
	log.Println("[INFO] Waiting for deployment jobs...")

	// Start the daily 3 AM Docker cache cleanup
	w.StartPruneScheduler()

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
			log.Printf("[INFO] Scheduled next Docker cache prune at: %v", next3AM.Format("15:04:05"))
			
			time.Sleep(durationToWait)
			
			log.Println("[INFO] Executing 3 AM scheduled Docker cache prune...")
			w.dockerService.PruneImages()
			
			// Optional: Aggressive BuildKit cache cleanup for absolute zero-cache state
			exec.Command("docker", "builder", "prune", "-a", "-f").Run()
		}
	}()
}

// Stop stops the worker
func (w *DeploymentWorker) Stop() {
	w.running = false
	log.Println("[INFO] Deployment worker stopped")
}

// processJobs continuously processes jobs from the queue
func (w *DeploymentWorker) processJobs() {
	for w.running {
		// Wait for next job with 5 second timeout
		job, err := w.redisService.DequeueDeployment(5 * time.Second)
		if err != nil {
			log.Printf("[ERROR] Error dequeuing job: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// No job available, continue waiting
		if job == nil {
			continue
		}

		// Process the job
		w.processDeployment(job)
	}
}

// processDeployment handles a single deployment job
func (w *DeploymentWorker) processDeployment(job *DeploymentJob) {
	log.Printf("[INFO] Processing %s for project #%d (queued: %v ago)",
		job.Type,
		job.ProjectID,
		time.Since(job.EnqueuedAt).Round(time.Second))

	// Try to acquire lock for this project
	locked, err := w.redisService.AcquireDeploymentLock(job.ProjectID, 30*time.Minute)
	if err != nil {
		log.Printf("[ERROR] Failed to acquire lock for project #%d: %v", job.ProjectID, err)
		w.redisService.IncrementDeploymentCounter("failed_lock")
		return
	}

	if !locked {
		log.Printf("[WARN] Project #%d is already being deployed, skipping", job.ProjectID)
		w.redisService.IncrementDeploymentCounter("skipped_locked")
		return
	}

	// Ensure lock is released after deployment
	defer func() {
		if err := w.redisService.ReleaseDeploymentLock(job.ProjectID); err != nil {
			log.Printf("[WARN] Failed to release lock for project #%d: %v", job.ProjectID, err)
		}
	}()

	// Fetch project from database
	var project models.Project
	if err := w.db.First(&project, job.ProjectID).Error; err != nil {
		log.Printf("[ERROR] Failed to find project #%d: %v", job.ProjectID, err)
		w.redisService.IncrementDeploymentCounter("failed_not_found")
		return
	}

	// Execute deployment
	startTime := time.Now()
	w.deployProject(&project)
	duration := time.Since(startTime)

	log.Printf("[INFO] Completed %s for project #%d '%s' in %v",
		job.Type,
		project.ID,
		project.Name,
		duration.Round(time.Second))

	w.redisService.IncrementDeploymentCounter("completed")
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(project *models.Project) {
	// Update status to building and clear old error logs
	w.db.Model(project).Select("status", "error_log", "updated_at").Updates(map[string]interface{}{
		"status":    models.StatusBuilding,
		"error_log": nil,
	})

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

	w.db.Model(project).Updates(map[string]interface{}{
		"laravel_version": laravelVersion,
		"php_version":     finalPHPVersion,
	})

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
	// Start building new container
	projectDomain := w.getProjectDomain()
	containerID, err := w.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain)

	if err != nil {
		w.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Update project as running with new container ID
	w.db.Model(project).Updates(map[string]interface{}{
		"status":       models.StatusRunning,
		"container_id": containerID,
	})

	// Cleanup old container after successful switch
	if oldContainerID != nil {
		go func() {
			w.dockerService.RemoveContainer(*oldContainerID)
		}()
	}
}

// updateProjectError sets project status to failed
func (w *DeploymentWorker) updateProjectError(project *models.Project, errorMsg string) {
	w.db.Model(project).Updates(map[string]interface{}{
		"status":    models.StatusFailed,
		"error_log": errorMsg,
	})
	w.redisService.IncrementDeploymentCounter("failed_deployment")
}

// getProjectDomain gets project domain from settings
func (w *DeploymentWorker) getProjectDomain() string {
	var setting models.Setting
	if err := w.db.Where("setting_key = ?", "project_domain").First(&setting).Error; err != nil {
		return w.cfg.ProjectDomain
	}
	return setting.Value
}
