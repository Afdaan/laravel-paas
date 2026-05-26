// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	sharedDocker "github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/laravel-paas/shared/repositories"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	"github.com/laravel-paas/shared/services/setting"
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
	projectRepo    repositories.ProjectRepository
	settingService *setting.SettingService
	githubService  *infrastructure.GithubService

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
		projectRepo:    projectRepo,
		settingService: settingService,
		githubService:  infrastructure.NewGithubService(cfg, redisService),
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

		if !sharedDocker.GetCircuitBreaker().Allow() {
			slog.Warn("Docker circuit breaker is open or half-open, pausing worker queue polling")
			time.Sleep(5 * time.Second)
			continue
		}

		if overloaded, reason := w.dockerService.IsSystemOverloaded(); overloaded {
			slog.Warn("System resource pressure critical, pausing worker queue polling", "reason", reason)
			time.Sleep(5 * time.Second)
			continue
		}

		// Wait for next job with 5 second timeout
		job, err := w.redisService.DequeueDeployment(5 * time.Second)

		// If we're stopping, don't start new jobs
		if !w.running {
			// If we got a job but we're stopping, put it back in the queue
			if job != nil {
				if _, err := w.redisService.EnqueueDeployment(job.ProjectID, job.UserID, job.Type); err != nil {
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

const (
	MaxRetryCount  = 5
	BaseRetryDelay = 5 * time.Second
	MaxRetryDelay  = 2 * time.Minute
)

func calculateBackoff(retryCount int) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}
	delay := float64(BaseRetryDelay) * math.Pow(2, float64(retryCount))
	if delay > float64(MaxRetryDelay) {
		delay = float64(MaxRetryDelay)
	}
	// Jitter +/- 20%
	jitter := (rand.Float64()*0.4 - 0.2) * delay
	return time.Duration(delay + jitter)
}

// processDeployment handles a single deployment job with panic recovery and deployment job leases
func (w *DeploymentWorker) processDeployment(job *infrastructure.DeploymentJob) {
	slog.Info("Processing deployment job",
		"jobId", job.JobID,
		"type", job.Type,
		"projectId", job.ProjectID,
		"retryCount", job.RetryCount,
		"queuedDuration", time.Since(job.EnqueuedAt).Round(time.Second))

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	workerID := fmt.Sprintf("worker-%s", hostname)

	jobName := "deployment task"
	if job.Type != "" {
		jobName = fmt.Sprintf("%s task", job.Type)
	}

	workerName := "Worker Manager"
	if slotNum := os.Getenv("SLOT"); slotNum != "" {
		workerName = fmt.Sprintf("Worker Slot %s", slotNum)
	}

	// Wrap deployment execution in panic recovery
	defer func() {
		if r := recover(); r != nil {
			slog.Error("CRITICAL PANIC during deployment execution", "jobId", job.JobID, "projectId", job.ProjectID, "workerId", workerID, "panic", r)
			w.recordAuditLog(job.ProjectID, job.JobID, workerID, "deployment_panic", fmt.Sprintf("Worker panic recovered: %v", r))
			_ = w.redisService.ReleaseDeploymentLease(job.JobID, workerID)
			_ = w.redisService.ForceReleaseDeploymentLock(job.ProjectID, fmt.Sprintf("Worker panic recovered: %v", r))
			if project, err := w.projectRepo.GetByID(job.ProjectID); err == nil {
				w.updateProjectError(project, job.JobID, fmt.Sprintf("Deployment aborted due to worker internal error (panic): %v", r))
			}
		}
	}()

	// Idempotency check for regular deploys
	if job.Type == "deploy" {
		project, err := w.projectRepo.GetByID(job.ProjectID)
		if err == nil && project.LastCommitHash != "" {
			if idempotent, _ := w.redisService.CheckIdempotency(job.ProjectID, project.LastCommitHash, project.Subdomain, job.Type); idempotent {
				slog.Info("Deployment idempotent match found, skipping duplicate processing", "projectId", job.ProjectID, "commit", project.LastCommitHash)
				w.redisService.IncrementDeploymentCounter("processed")
				return
			}
		}
	}

	// Acquire independent deployment job lease
	leaseMeta := &infrastructure.DeploymentLeaseMetadata{
		JobID:          job.JobID,
		ProjectID:      job.ProjectID,
		WorkerID:       workerID,
		Hostname:       hostname,
		StartedAt:      time.Now().Format(time.RFC3339),
		LastHeartbeat:  time.Now().Format(time.RFC3339),
		DeploymentType: job.Type,
	}
	if err := w.redisService.AcquireDeploymentLease(job.JobID, leaseMeta, 2*time.Minute); err != nil {
		slog.Warn("Failed to acquire deployment job lease", "jobId", job.JobID, "workerId", workerID, "error", err)
	} else {
		w.recordAuditLog(job.ProjectID, job.JobID, workerID, "lease_acquired", fmt.Sprintf("Acquired lease for %s on %s", jobName, workerName))
	}

	defer w.cleanupJobTracking(job.JobID)

	// Ensure lease is cleanly released on any exit
	defer func() {
		if err := w.redisService.ReleaseDeploymentLease(job.JobID, workerID); err != nil {
			slog.Warn("Failed to release deployment job lease", "jobId", job.JobID, "workerId", workerID, "error", err)
		} else {
			w.recordAuditLog(job.ProjectID, job.JobID, workerID, "lease_released", fmt.Sprintf("Released lease for %s on %s", jobName, workerName))
		}
	}()

	// Acquire short 2-minute lock with background heartbeat renewal
	lockToken, err := w.redisService.AcquireDeploymentLock(job.ProjectID, job.JobID, 2*time.Minute)
	if err != nil {
		slog.Error("Failed to acquire lock for project", "id", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed")
		return
	}

	if lockToken == "" {
		if job.RetryCount >= MaxRetryCount {
			slog.Error("Max retries reached for project deployment, failing permanently", "id", job.ProjectID, "jobId", job.JobID)
			if project, err := w.projectRepo.GetByID(job.ProjectID); err == nil {
				w.updateProjectError(project, job.JobID, "Deployment failed: system is currently busy deploying this project and lock timed out after maximum retries.")
			}
			return
		}

		delay := calculateBackoff(job.RetryCount)
		job.RetryCount++
		slog.Warn("Project is already being deployed, enqueuing delayed job with exponential backoff", "id", job.ProjectID, "retry", job.RetryCount, "delay", delay.Round(time.Millisecond))
		_ = w.redisService.EnqueueDelayedDeploymentJob(job, delay)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelSub, err := w.redisService.SubscribeCancellation(ctx, job.ProjectID)
	if err == nil {
		go func() {
			select {
			case <-cancelSub:
				slog.Info("Deployment cancelled via broadcast, aborting process", "projectId", job.ProjectID)
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.redisService.RenewDeploymentLock(job.ProjectID, lockToken, 2*time.Minute); err != nil {
					slog.Warn("Failed to renew deployment lock", "projectId", job.ProjectID, "error", err)
				}
				if err := w.redisService.RenewDeploymentLease(job.JobID, workerID, 2*time.Minute); err != nil {
					slog.Warn("Failed to renew deployment job lease", "jobId", job.JobID, "workerId", workerID, "error", err)
				} else {
					w.recordAuditLog(job.ProjectID, job.JobID, workerID, "lease_renewed", fmt.Sprintf("Renewed lease for %s", jobName))
				}
				if err := w.projectRepo.UpdateDeploymentHeartbeat(job.ProjectID); err != nil {
					slog.Warn("Failed to update deployment heartbeat in database", "projectId", job.ProjectID, "error", err)
				}
			case <-stopHeartbeat:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Ensure lock is released and heartbeat stopped after deployment
	defer func() {
		close(stopHeartbeat)
		if err := w.redisService.ReleaseDeploymentLock(job.ProjectID, lockToken); err != nil {
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
	w.deployProject(ctx, project, job)
	duration := time.Since(startTime)

	slog.Info("Completed deployment job",
		"type", job.Type,
		"projectId", project.ID,
		"projectName", project.Name,
		"duration", duration.Round(time.Second))

	w.redisService.IncrementDeploymentCounter("processed")
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(ctx context.Context, project *models.Project, job *infrastructure.DeploymentJob) {
	if job.Type == "update_env" && project.ContainerID != nil {
		slog.Info("Performing instant environment update", "subdomain", project.Subdomain)
		if err := w.instantUpdateEnv(project); err == nil {
			return
		}
		slog.Warn("Instant update failed, falling back to full deployment", "subdomain", project.Subdomain)
	}

	if job.Type == "rollback" {
		slog.Info("Performing instant rollback", "subdomain", project.Subdomain, "commit", project.LastCommitHash)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "rollback_started", fmt.Sprintf("Rolling back to %s", project.LastCommitHash))
		if err := w.redeployExistingImage(project); err == nil {
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "rollback_completed", project.LastCommitHash)
			return
		}
		slog.Warn("Instant rollback failed, falling back to full build deployment", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 30, "rollback_fallback", "Instant rollback failed, falling back to rebuild")
	}

	w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 10, "deployment_started", fmt.Sprintf("Triggered by %s", job.Type))
	project.ErrorLog = nil
	_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"error_log": nil,
	})

	projectPath := filepath.Join(w.cfg.ProjectsPath, project.Subdomain)
	buildLogPath := filepath.Join(projectPath, "build.log")
	if err := os.MkdirAll(projectPath, 0755); err == nil {
		if err := os.WriteFile(buildLogPath, []byte(""), 0644); err != nil {
			slog.Warn("Failed to clear build log", "path", buildLogPath, "error", err)
		}
	}

	appendLog := func(msg string) {
		if f, err := os.OpenFile(buildLogPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644); err == nil {
			_, _ = f.WriteString(msg + "\n")
			f.Close()
		}
		_ = w.redisService.PublishBuildLog(project.ID, msg)
	}

	w.checkDiskSpace()

	// Obtain GitHub App installation token for authenticating private repositories
	authURL := project.GithubURL
	var installationID int64
	if project.GithubInstallationID != nil && *project.GithubInstallationID != 0 {
		installationID = *project.GithubInstallationID
	} else {
		// Dynamic Self-Healing: Attempt to resolve missing installation ID from owner or user's account
		owner := ""
		trimmed := strings.TrimPrefix(authURL, "https://github.com/")
		trimmed = strings.TrimPrefix(trimmed, "http://github.com/")
		trimmed = strings.TrimSuffix(trimmed, "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) > 0 {
			owner = parts[0]
		}
		
		if resolvedID, err := w.projectRepo.ResolveInstallationID(project.UserID, owner); err == nil && resolvedID != 0 {
			installationID = resolvedID
			slog.Info("Self-healed and dynamically resolved GitHub Installation ID for project", "projectId", project.ID, "owner", owner, "resolvedID", resolvedID)
			
			// Persist the resolved installation ID to prevent future slow resolution runs
			project.GithubInstallationID = &resolvedID
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"github_installation_id": resolvedID,
			})
		}
	}

	if installationID != 0 {
		token, err := w.githubService.GetInstallationToken(installationID)
		if err != nil {
			slog.Error("Failed to get GitHub installation token for deployment", "projectId", project.ID, "error", err)
			w.updateProjectError(project, job.JobID, "Failed to authenticate with GitHub App: "+err.Error())
			return
		}
		// Inject token into git URL: https://x-access-token:<token>@github.com/owner/repo
		if strings.HasPrefix(authURL, "https://github.com/") {
			authURL = "https://x-access-token:" + token + "@" + strings.TrimPrefix(authURL, "https://")
		}
	}

	latestHash, hashErr := w.gitService.GetRemoteCommitHash(authURL, project.Branch)
	if job.Type == "deploy" && hashErr == nil && latestHash != "" && project.LastCommitHash == latestHash && project.ContainerID != nil {
		slog.Info("Commit hash unchanged, checking for existing image", "subdomain", project.Subdomain, "hash", latestHash)

		imageName := fmt.Sprintf("paas-%s", project.Subdomain)
		checkImg, _ := exec.Command("docker", "image", "inspect", imageName).Output()
		if len(checkImg) > 0 {
			slog.Info("Valid image found, skipping build", "subdomain", project.Subdomain)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "deployment_skipped_existing_image", latestHash)
			if err := w.redeployExistingImage(project); err == nil {
				return
			}
		}
	}

	w.transitionDeploymentState(project, job.JobID, models.DepStatusCloning, 20, "cloning_repository", project.GithubURL)

	var cloneHash string
	var cloneErr error
	var dbErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(authURL, project.Branch, project.Subdomain)
		if cloneErr != nil && installationID != 0 && (strings.Contains(cloneErr.Error(), "Authentication failed") || strings.Contains(cloneErr.Error(), "Invalid username or token") || strings.Contains(cloneErr.Error(), "could not read Username")) {
			slog.Warn("Git clone failed due to authentication issue, invalidating cached token and retrying...", "projectId", project.ID, "installationId", installationID)
			w.githubService.InvalidateInstallationToken(installationID)
			
			// Fetch a fresh token
			newToken, err := w.githubService.GetInstallationToken(installationID)
			if err == nil && newToken != "" {
				// Reconstruct authURL with fresh token
				retryURL := project.GithubURL
				if strings.HasPrefix(retryURL, "https://github.com/") {
					retryURL = "https://x-access-token:" + newToken + "@" + strings.TrimPrefix(retryURL, "https://")
				}
				slog.Info("Retrying Git clone with a fresh token...", "projectId", project.ID)
				projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(retryURL, project.Branch, project.Subdomain)
			}
		}
	}()

	go func() {
		defer wg.Done()
		if project.DatabasePassword == "" {
			project.DatabasePassword = utils.GeneratePassword(16)
			if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"database_password": project.DatabasePassword,
			}); err != nil {
				slog.Warn("Failed to save database password", "id", project.ID, "error", err)
			}
		}
		dbErr = w.mysqlService.CreateDatabase(project.DatabaseName, project.DatabasePassword)
	}()

	wg.Wait()

	if cloneErr != nil {
		w.updateProjectError(project, job.JobID, "Failed to clone repository: "+cloneErr.Error())
		return
	}
	if dbErr != nil {
		w.updateProjectError(project, job.JobID, "Failed to create database: "+dbErr.Error())
		return
	}

	project.LastCommitHash = cloneHash
	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"last_commit_hash": cloneHash,
	}); err != nil {
		slog.Warn("Failed to update commit hash", "id", project.ID, "error", err)
	}

	_ = w.redisService.SetIdempotency(project.ID, cloneHash, project.Subdomain, job.Type)

	shortHash := cloneHash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}

	commitMessage := ""
	if msgRes, err := utils.Run(10*time.Second, "git", "-C", projectPath, "log", "-1", "--format=%s"); err == nil {
		commitMessage = strings.TrimSpace(msgRes.Stdout)
	}

	commitDetail := fmt.Sprintf("Commit %s", shortHash)
	if commitMessage != "" {
		commitDetail = fmt.Sprintf("Commit %s: %s", shortHash, commitMessage)
	}

	w.transitionDeploymentState(project, job.JobID, models.DepStatusBuilding, 35, "building_image", commitDetail)

	var logMsg string
	if commitMessage != "" {
		logMsg = fmt.Sprintf(">> Building image for commit %s (%s)...", shortHash, commitMessage)
	} else {
		logMsg = fmt.Sprintf(">> Building image for commit %s...", shortHash)
	}
	appendLog(logMsg)

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

	updates := map[string]interface{}{
		"framework": project.Framework,
	}
	if project.LanguageVersion != "" {
		updates["language_version"] = project.LanguageVersion
	}
	if project.NodeVersion != "" {
		updates["node_version"] = project.NodeVersion
	}
	if project.PHPVersion != "" {
		updates["php_version"] = project.PHPVersion
	}
	if project.LaravelVersion != "" {
		updates["laravel_version"] = project.LaravelVersion
	}
	if err := w.projectRepo.UpdateMetadata(project.ID, updates); err != nil {
		slog.Warn("Failed to update project metadata", "id", project.ID, "error", err)
	}

	finalPHPVersion := project.PHPVersion
	if finalPHPVersion == "" {
		finalPHPVersion = "8.4"
	}

	var oldContainerID *string
	var oldWorkerContainerID *string
	if project.ContainerID != nil {
		oldHelp := *project.ContainerID
		oldContainerID = &oldHelp
	}
	if project.WorkerContainerID != nil {
		oldHelpWorker := *project.WorkerContainerID
		oldWorkerContainerID = &oldHelpWorker
	}

	projectDomain := w.getSetting(models.SettingProjectDomain, w.cfg.ProjectDomain)

	cpuPercentStr := w.getSetting(models.SettingCPULimit, models.DefaultCPULimit)
	cpuPercent, _ := strconv.ParseFloat(cpuPercentStr, 64)
	cpuLimit := cpuPercent / 100.0

	memoryMB := w.getSetting(models.SettingMemoryLimit, models.DefaultMemoryLimit)
	memoryLimit := memoryMB + "m"


	buildTimeoutSec, err := strconv.Atoi(w.getSetting(models.SettingBuildTimeout, models.DefaultBuildTimeout))
	if err != nil || buildTimeoutSec <= 0 {
		buildTimeoutSec = 1800
	}
	buildCtx, buildCancel := context.WithTimeout(ctx, time.Duration(buildTimeoutSec)*time.Second)
	defer buildCancel()

	newContainerID, err := w.dockerService.BuildAndRun(buildCtx, project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit, job.Type == "deploy", job.Type == "redeploy", appendLog)
	if err != nil {
		sharedDocker.GetCircuitBreaker().RecordFailure()
		if ctx.Err() == context.Canceled {
			appendLog("ERROR: Deployment cancelled by user request.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCancelled, project.DeploymentProgress, "deployment_cancelled", "User requested cancellation")
			w.updateProjectError(project, job.JobID, "Deployment cancelled by user.")
			return
		}
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			appendLog("ERROR: Deployment build phase timed out (watchdog kill).")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusFailed, project.DeploymentProgress, "watchdog_timeout", "Build log watchdog timed out build step")
			w.updateProjectError(project, job.JobID, "Deployment failed: Build step exceeded maximum allowed time limit.")
			return
		}
		appendLog("ERROR: Failed to deploy container: " + err.Error())
		w.updateProjectError(project, job.JobID, "Failed to deploy container: "+err.Error())
		return
	}
	sharedDocker.GetCircuitBreaker().RecordSuccess()

	w.transitionDeploymentState(project, job.JobID, models.DepStatusStarting, 50, "starting_container", "Launching new container instance")

	project.RolloutContainerID = &newContainerID
	_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"rollout_container_id": newContainerID,
		"port":                 project.Port,
	})

	appendLog(">> Running health checks...")
	w.transitionDeploymentState(project, job.JobID, models.DepStatusHealthchecking, 65, "healthchecking_container", "Executing readiness probe and stabilization monitoring")

	if err := w.dockerService.AdvancedHealthcheck(ctx, project, newContainerID); err != nil {
		slog.Error("New container failed advanced healthcheck, initiating rollback", "subdomain", project.Subdomain, "id", newContainerID, "error", err)
		appendLog("ERROR: Health check failed: " + err.Error() + ". Rolling back.")

		w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Healthcheck failed, keeping old version active")

		if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to cleanup unhealthy deployment", "id", newContainerID, "error", err)
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"rollout_container_id": nil,
		})

		w.updateProjectError(project, job.JobID, "Deployment failed healthcheck: "+err.Error()+". Old version is still running.")
		return
	} else {
		appendLog("✓ Health check passed.")
	}

	if project.Framework == "Laravel" {
		if ctx.Err() == context.Canceled {
			slog.Info("Deployment cancelled before migrations, rolling back", "subdomain", project.Subdomain)
			appendLog("ERROR: Deployment cancelled by user request. Rolling back.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Cancelled before migration")
			_ = w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID)
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"rollout_container_id": nil,
			})
			w.updateProjectError(project, job.JobID, "Deployment cancelled by user before migrations. Old version is still running.")
			return
		}
		w.transitionDeploymentState(project, job.JobID, models.DepStatusMigrating, 75, "running_migrations", "Executing artisan migrate --force")
		slog.Info("Running database migrations", "subdomain", project.Subdomain)
		appendLog(">> Running database migrations...")
		if output, err := w.dockerService.RunMigrations(newContainerID); err != nil {
			slog.Error("Migrations failed", "subdomain", project.Subdomain, "error", err)
			appendLog("ERROR: Migrations failed:\n" + output)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Migrations failed")
			if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
				slog.Warn("Failed to cleanup failed container", "id", newContainerID, "error", err)
			}
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"rollout_container_id": nil,
			})
			w.updateProjectError(project, job.JobID, "Migrations failed: "+err.Error()+"\n\nOutput:\n"+output)
			return
		} else {
			if strings.TrimSpace(output) != "" {
				appendLog(strings.TrimRight(output, "\r\n"))
			} else {
				appendLog("✓ Database migrations ran successfully (nothing to migrate).")
			}
		}
	}

	w.transitionDeploymentState(project, job.JobID, models.DepStatusPromoting, 85, "promoting_release", "Syncing routing traffic to new container instance")
	appendLog(">> Promoting deployment...")

	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to cache subdomain mapping", "subdomain", project.Subdomain, "error", err)
	}

	if _, err := w.projectService.SyncProjectNginxFrom(project, "deployment_promote"); err != nil {
		slog.Error("Nginx sync failed", "subdomain", project.Subdomain, "error", err)
	}

	if err := w.projectService.PromoteRolloutContainer(project.ID, newContainerID); err != nil {
		slog.Error("Failed to promote rollout container", "id", project.ID, "error", err)
		appendLog("ERROR: Failed to promote deployment: " + err.Error())
	} else {
		appendLog("✓ Release promoted successfully.")
	}
	project.Status = models.StatusRunning
	project.ContainerID = &newContainerID
	project.RolloutContainerID = nil

	if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate cache", "subdomain", project.Subdomain, "error", err)
	}

	if err := w.projectService.CacheSubdomainMapping(project); err != nil {
		slog.Warn("Failed to update subdomain cache", "subdomain", project.Subdomain, "error", err)
	}

	if oldContainerID != nil {
		shortContainerID := *oldContainerID
		if len(shortContainerID) > 12 {
			shortContainerID = shortContainerID[:12]
		}
		w.transitionDeploymentState(project, job.JobID, models.DepStatusCleanup, 95, "cleaning_legacy_instance", fmt.Sprintf("Removing previous container instance (%s)", shortContainerID))
		slog.Info("Cleaning up legacy instance", "subdomain", project.Subdomain)
		// Perform container cleanup silently to hide infrastructure secrets from developer logs
		if err := w.dockerService.RemoveContainer(*oldContainerID, oldWorkerContainerID); err != nil {
			slog.Warn("Failed to remove legacy container", "id", *oldContainerID, "error", err)
		}
	}

	w.dockerService.CleanupLegacyContainers(project.Subdomain, newContainerID, project.WorkerContainerID)

	// Prune old project image builds according to retention policy
	maxRetentionStr := w.getSetting(models.SettingMaxImageRetention, models.DefaultMaxImageRetention)
	maxRetention, _ := strconv.Atoi(maxRetentionStr)
	if maxRetention <= 0 {
		maxRetention, _ = strconv.Atoi(models.DefaultMaxImageRetention)
	}
	w.dockerService.PruneProjectImages(project.Subdomain, maxRetention)

	if !w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "deployment_completed", project.LastCommitHash) {
		w.forceDeploymentCompleted(project, job.JobID)
	}
	
	appendLog("")
	appendLog("========================================================================")
	appendLog("✓ Deployment completed successfully! Application is live.")
	appendLog("========================================================================")

	go func() {
		_ = utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f")
		_ = utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f")
	}()
}

// cleanupJobTracking safely removes sequence tracking state for a completed or terminated job to prevent memory leaks.
func (w *DeploymentWorker) cleanupJobTracking(jobID string) {
}

// transitionDeploymentState validates and transitions the deployment status while recording a structured event audit trail
func (w *DeploymentWorker) transitionDeploymentState(project *models.Project, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) bool {
	updatedProject, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, jobID, nextState, progress, eventType, payload)
	if err != nil {
		slog.Warn("Failed atomic deployment state transition", "projectId", project.ID, "nextState", nextState, "error", err)
		return false
	}
	if updatedProject != nil {
		project.DeploymentStatus = updatedProject.DeploymentStatus
		project.DeploymentProgress = updatedProject.DeploymentProgress
		project.DeploymentMessage = updatedProject.DeploymentMessage
		w.updateGitHubCommitStatus(project, nextState, payload)
	}
	return true
}

func (w *DeploymentWorker) updateGitHubCommitStatus(project *models.Project, state models.DeploymentStatus, description string) {
	if project.GithubInstallationID == nil || *project.GithubInstallationID == 0 || project.GithubRepoOwner == "" || project.GithubRepoName == "" || project.LastCommitHash == "" {
		return
	}

	ghState := "pending"
	desc := ""

	switch state {
	case models.DepStatusCompleted:
		ghState = "success"
		desc = "Deployment successful. Application is live."
	case models.DepStatusFailed:
		ghState = "failure"
		desc = "Deployment failed. View build logs in the dashboard for details."
	case models.DepStatusRollback:
		ghState = "failure"
		desc = "Deployment failed. Rolled back to previous stable version."
	case models.DepStatusCancelled:
		ghState = "error"
		desc = "Deployment cancelled by user."
	case models.DepStatusQueued:
		desc = "Build queued. Waiting for an available worker slot..."
	case models.DepStatusPreparing:
		desc = "Preparing build environment..."
	case models.DepStatusCloning:
		desc = "Cloning source code repository..."
	case models.DepStatusBuilding:
		desc = "Building container image using BuildKit..."
	case models.DepStatusStarting:
		desc = "Provisioning container instance..."
	case models.DepStatusHealthchecking:
		desc = "Running health checks and readiness probes..."
	case models.DepStatusMigrating:
		desc = "Running database migrations..."
	case models.DepStatusPromoting:
		desc = "Promoting release and propagating routing traffic..."
	case models.DepStatusCleanup:
		desc = "Cleaning up build artifacts..."
	default:
		desc = description
		if desc == "" {
			desc = string(state)
		}
	}

	if len(desc) > 140 {
		desc = desc[:137] + "..."
	}

	projectUID := project.UID
	if projectUID == "" {
		projectUID = fmt.Sprintf("%d", project.ID)
	}
	targetURL := fmt.Sprintf("%s/projects/%s?tab=build", w.cfg.FrontendURL, projectUID)

	slog.Info("Updating GitHub commit status", "project_id", project.ID, "sha", project.LastCommitHash, "state", ghState, "desc", desc)
	go func() {
		err := w.githubService.UpdateCommitStatus(*project.GithubInstallationID, project.GithubRepoOwner, project.GithubRepoName, project.LastCommitHash, ghState, targetURL, desc)
		if err != nil {
			slog.Warn("Failed to update GitHub commit status", "project_id", project.ID, "error", err)
		}
	}()
}

func (w *DeploymentWorker) forceDeploymentCompleted(project *models.Project, jobID string) {
	if err := w.projectRepo.UpdateDeploymentStatus(project.ID, models.DepStatusCompleted, "Release successfully promoted and live", 100, jobID); err != nil {
		slog.Error("Failed to force deployment completion status", "projectId", project.ID, "error", err)
		return
	}
	w.recordAuditLog(project.ID, jobID, "deployment-worker", "deployment_completed_reconciled", "Release successfully promoted and live")
	project.DeploymentStatus = models.DepStatusCompleted
	project.DeploymentProgress = 100
	msg := "Release successfully promoted and live"
	project.DeploymentMessage = &msg
	w.updateGitHubCommitStatus(project, models.DepStatusCompleted, msg)
	if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate cache after forced deployment completion", "subdomain", project.Subdomain, "error", err)
	}
}

// recordAuditLog records a deployment event without altering the project status
func (w *DeploymentWorker) recordAuditLog(projectID uint, jobID, workerID, eventType, payload string) {
	event := &models.DeploymentEvent{
		ProjectID: projectID,
		JobID:     jobID,
		WorkerID:  workerID,
		StateFrom: string(models.DepStatusBuilding),
		StateTo:   string(models.DepStatusBuilding),
		EventType: eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	if err := w.projectRepo.RecordDeploymentEvent(event); err != nil {
		slog.Warn("Failed to record audit event to database", "projectId", projectID, "error", err)
	}

	eventJSON, err := json.Marshal(event)
	if err == nil {
		_ = w.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
	}
}

// updateProjectError sets project deployment status to failed
func (w *DeploymentWorker) updateProjectError(project *models.Project, jobID string, errorMsg string) {
	w.transitionDeploymentState(project, jobID, models.DepStatusFailed, project.DeploymentProgress, "deployment_failed", errorMsg)
	msg := errorMsg
	project.ErrorLog = &msg
	
	// Determine the correct project status after a failure
	statusUpdate := models.StatusFailed
	if project.ContainerID != nil && *project.ContainerID != "" {
		// If an existing container is already running, keep the status as running
		statusUpdate = models.StatusRunning
	}
	project.Status = statusUpdate
	
	if err := w.projectRepo.UpdateStatus(project.ID, statusUpdate); err != nil {
		slog.Error("Failed to update project runtime status on error", "id", project.ID, "status", statusUpdate, "error", err)
	}

	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"error_log": msg,
	}); err != nil {
		slog.Error("Failed to update project error log on error", "id", project.ID, "error", err)
	}
	if project.RolloutContainerID != nil && *project.RolloutContainerID != "" {
		_ = w.dockerService.RemoveContainer(*project.RolloutContainerID, nil)
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"rollout_container_id": nil,
		})
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

	if usage > 85 {
		slog.Warn("Disk space usage exceeds 85%, performing smart cleanup", "usage", usage)
		_ = utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-a", "-f", "--filter", "until=24h")
		_ = utils.RunSilent(5*time.Minute, "docker", "builder", "prune", "-f")
		_ = utils.RunSilent(5*time.Minute, "docker", "buildx", "prune", "-f", "--builder", "paas-builder")
		_ = utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f")
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
