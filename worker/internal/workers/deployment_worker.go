// ===========================================
// Deployment Worker
// ===========================================
// Background worker for processing deployment queue
// ===========================================
package workers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	sharedDocker "github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/billinggate"
	"github.com/laravel-paas/shared/services/setting"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

const migrationPartialChangesWarning = "WARNING: Application rollout stopped, but database changes completed before this failure were not automatically rolled back."

type managedDatabaseConnection struct {
	driverName     string
	dsn            string
	versionQuery   string
	defaultVersion string
}

func ensureManagedDatabase(verify func() (string, error), provision func() error) (string, error) {
	version, verifyErr := verify()
	if verifyErr == nil {
		return version, nil
	}

	provisionErr := provision()
	version, verifyErr = verify()
	if verifyErr == nil {
		return version, nil
	}
	if provisionErr != nil {
		return "", provisionErr
	}

	return "", fmt.Errorf("database provisioning verification failed: %w", verifyErr)
}

func buildManagedDatabaseConnection(engine, host string, port int, dbName, username, password string) (managedDatabaseConnection, error) {
	switch engine {
	case "mysql":
		config := mysqlDriver.NewConfig()
		config.User = username
		config.Passwd = password
		config.Net = "tcp"
		config.Addr = net.JoinHostPort(host, strconv.Itoa(port))
		config.DBName = dbName

		return managedDatabaseConnection{
			driverName:     "mysql",
			dsn:            config.FormatDSN(),
			versionQuery:   "SELECT @@version",
			defaultVersion: "MySQL 8.0",
		}, nil
	case "postgresql":
		connectionURL := &url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(username, password),
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
			Path:   "/" + dbName,
		}
		query := connectionURL.Query()
		query.Set("sslmode", "disable")
		connectionURL.RawQuery = query.Encode()

		return managedDatabaseConnection{
			driverName:     "pgx",
			dsn:            connectionURL.String(),
			versionQuery:   "SELECT version()",
			defaultVersion: "PostgreSQL 15",
		}, nil
	default:
		return managedDatabaseConnection{}, fmt.Errorf("unsupported managed database engine %q", engine)
	}
}

func inspectManagedDatabase(ctx context.Context, engine, host string, port int, dbName, username, password string) (string, error) {
	connection, err := buildManagedDatabaseConnection(engine, host, port, dbName, username, password)
	if err != nil {
		return "", err
	}

	dbConn, err := sql.Open(connection.driverName, connection.dsn)
	if err != nil {
		return "", err
	}
	defer dbConn.Close()

	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := dbConn.PingContext(verifyCtx); err != nil {
		return "", err
	}

	var version string
	if err := dbConn.QueryRowContext(verifyCtx, connection.versionQuery).Scan(&version); err != nil || version == "" {
		return connection.defaultVersion, nil
	}
	if engine == "postgresql" {
		version = strings.Split(version, " on ")[0]
	}

	return version, nil
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

	lastProcessingRecovery := time.Time{}
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

		if time.Since(lastProcessingRecovery) >= time.Minute {
			if err := w.redisService.RequeueExpiredDeploymentJobs(10 * time.Minute); err != nil {
				slog.Error("Failed to reclaim expired deployment jobs", "error", err)
			}
			lastProcessingRecovery = time.Now()
		}

		// Wait for next job with 5 second timeout
		job, err := w.redisService.DequeueDeployment(5 * time.Second)

		// If we're stopping, don't start new jobs
		if !w.running {
			// If we got a job but we're stopping, return its durable claim to the queue.
			if job != nil {
				if err := w.redisService.RequeueDeploymentJob(job); err != nil {
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
		isLightweight := job.Type == "restart" || job.Type == "update_env" || job.Type == "stop" || job.Type == "start"

		if isLightweight {
			w.wg.Add(1)
			go func(j *infrastructure.DeploymentJob) {
				defer w.wg.Done()
				defer w.acknowledgeDeploymentJob(j)
				w.processDeployment(j)
			}(job)
		} else {
			localSem := sem
			heartbeat := time.NewTicker(30 * time.Second)
			for {
				select {
				case localSem <- struct{}{}:
					heartbeat.Stop()
					goto heavyJobReady
				case <-heartbeat.C:
					if err := w.redisService.RenewDeploymentProcessingHeartbeat(job); err != nil {
						heartbeat.Stop()
						slog.Error("Lost queued deployment processing claim", "jobId", job.JobID, "error", err)
						goto nextJob
					}
				}
			}
		heavyJobReady:
			w.wg.Add(1)
			go func(j *infrastructure.DeploymentJob, sema chan struct{}) {
				defer w.wg.Done()
				defer func() { <-sema }()
				defer w.acknowledgeDeploymentJob(j)
				w.processDeployment(j)
			}(job, localSem)
		}
	nextJob:
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
		slog.Error("Failed to acquire deployment job lease", "jobId", job.JobID, "workerId", workerID, "error", err)
		if requeueErr := w.redisService.RequeueDeploymentJob(job); requeueErr != nil {
			slog.Error("Failed to return uncertain deployment lease claim", "jobId", job.JobID, "error", requeueErr)
		}
		return
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
		if project, errRepo := w.projectRepo.GetByID(job.ProjectID); errRepo == nil {
			w.updateProjectError(project, job.JobID, "Deployment failed: worker failed to acquire project build lock: "+err.Error())
		}
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
		if err := w.redisService.EnqueueDelayedDeploymentJob(job, delay); err != nil {
			slog.Error("Failed to enqueue delayed deployment retry; returning claim to queue", "projectId", job.ProjectID, "error", err)
			if requeueErr := w.redisService.RequeueDeploymentJob(job); requeueErr != nil {
				slog.Error("Failed to return deployment claim after delayed enqueue failure", "projectId", job.ProjectID, "error", requeueErr)
			}
		}
		return
	}

	// 1. Establish overall deployment watchdog timeout (default 30 mins)
	buildTimeoutSec, err := strconv.Atoi(w.getSetting(models.SettingBuildTimeout, models.DefaultBuildTimeout))
	if err != nil || buildTimeoutSec <= 0 {
		buildTimeoutSec = 1800
	}
	// Give the overall deployment 1.5x the build timeout to cover cloning, migrations, and Nginx reloads
	overallTimeout := time.Duration(buildTimeoutSec) * 3 / 2 * time.Second
	if overallTimeout < 5*time.Minute {
		overallTimeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
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
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.redisService.RenewDeploymentLock(job.ProjectID, lockToken, 2*time.Minute); err != nil {
					slog.Error("Lost deployment lock; fencing deployment", "projectId", job.ProjectID, "error", err)
					cancel()
					return
				}
				if err := w.redisService.RenewDeploymentLease(job.JobID, workerID, 2*time.Minute); err != nil {
					slog.Error("Lost deployment lease; fencing deployment", "jobId", job.JobID, "workerId", workerID, "error", err)
					cancel()
					return
				} else {
					if err := w.redisService.RenewDeploymentProcessingHeartbeat(job); err != nil {
						slog.Error("Lost deployment processing claim; fencing deployment", "jobId", job.JobID, "error", err)
						cancel()
						return
					}
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
		<-heartbeatDone
		if err := w.redisService.ReleaseDeploymentLock(job.ProjectID, lockToken); err != nil {
			slog.Warn("Failed to release lock for project", "id", job.ProjectID, "error", err)
		}

		// After lock is released, check if there is a pending env refresh marker
		if hasPending, err := w.redisService.HasPendingEnvRefresh(job.ProjectID); err == nil && hasPending {
			slog.Info("Detected pending env refresh marker after job completion, attempting quiet enqueue", "projectId", job.ProjectID)
			jobID, errQueue := w.redisService.EnqueueEnvUpdateIfQuiet(job.ProjectID, job.UserID)
			if errQueue != nil {
				slog.Error("Failed to enqueue pending env update job", "projectId", job.ProjectID, "error", errQueue)
			} else if jobID != "" {
				slog.Info("Successfully enqueued pending env update job, clearing marker", "projectId", job.ProjectID, "jobId", jobID)
				_, _ = w.redisService.ClearPendingEnvRefresh(job.ProjectID)
			} else {
				slog.Info("Project not quiet, keeping pending env refresh marker", "projectId", job.ProjectID)
			}
		}
	}()

	// Fetch project from database via repository
	project, err := w.projectRepo.GetByID(job.ProjectID)
	if err != nil {
		slog.Error("Failed to find project for deployment", "projectId", job.ProjectID, "error", err)
		w.redisService.IncrementDeploymentCounter("failed")
		return
	}
	if job.Type == "stop" && job.BillingSuspension {
		if !w.billingSuspensionStillRequired(project.ID, job.BillingSuspensionTaskID) {
			return
		}
	}
	if billingRuntimeAction(job.Type) {
		billingEnabled, blockDays := false, 0
		if w.cfg != nil {
			billingEnabled = w.cfg.BillingEnabled
			blockDays = w.cfg.BillingDeployBlockDays
		}
		gateErr := billinggate.NewProjectRuntimeGate(w.projectRepo.DB(), billingEnabled, blockDays).Check(context.Background(), project.ID, time.Now().UTC())
		if gateErr != nil {
			slog.Warn("Blocked project runtime action due to billing", "projectId", project.ID, "jobId", job.JobID, "type", job.Type, "error", gateErr)
			if job.BillingResume {
				if err := w.stopStaleBillingResume(project.ID, job.BillingSuspensionTaskID); err != nil {
					slog.Error("Failed to compensate stale billing resume", "projectId", project.ID, "taskId", job.BillingSuspensionTaskID, "error", err)
					w.updateProjectError(project, job.JobID, "Deployment blocked and billing-resume containers could not be stopped: "+err.Error())
					w.redisService.IncrementDeploymentCounter("failed")
					return
				}
			}
			w.updateProjectError(project, job.JobID, "Deployment blocked: resolve the overdue billing invoice before starting or deploying this project")
			w.redisService.IncrementDeploymentCounter("failed")
			return
		}
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

func (w *DeploymentWorker) billingSuspensionStillRequired(projectID, taskID uint) bool {
	if w == nil || w.projectRepo == nil || projectID == 0 || taskID == 0 {
		return false
	}
	allowed := false
	err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusSuspended {
			return nil
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			return err
		}
		allowed = true
		return nil
	})
	if err != nil {
		slog.Info("Discarding stale billing suspension stop", "project_id", projectID, "task_id", taskID, "error", err)
		return false
	}
	return allowed
}

func (w *DeploymentWorker) finalizeBillingSuspensionStop(projectID, taskID uint) (bool, error) {
	if w == nil || w.projectRepo == nil || projectID == 0 || taskID == 0 {
		return false, nil
	}
	finalized := false
	err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusSuspended {
			return nil
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return err
		}
		if err := tx.Model(&project).Update("status", models.StatusStopped).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&task).Update("stop_completed_at", &now).Error; err != nil {
			return err
		}
		finalized = true
		return nil
	})
	return finalized, err
}

type billingStopSnapshot struct {
	mainContainerID   string
	workerContainerID string
	mainWasRunning    bool
	workerWasRunning  bool
	mainStopped       bool
	workerStopped     bool
}

func (s billingStopSnapshot) anyWasRunning() bool {
	return s.mainWasRunning || s.workerWasRunning
}

func (s billingStopSnapshot) anyStopped() bool {
	return s.mainStopped || s.workerStopped
}

var errBillingSuspensionNoLongerRequired = errors.New("billing suspension no longer required")

func (w *DeploymentWorker) stopProjectContainers(project *models.Project, taskID uint, billingSuspension bool, appendLog func(string)) (billingStopSnapshot, error) {
	if billingSuspension {
		snapshot, found, err := w.loadBillingSuspensionStopSnapshot(project.ID, taskID)
		if err != nil {
			return snapshot, err
		}
		if !found {
			snapshot, err = inspectBillingStopSnapshot(project, w.dockerService.IsContainerRunning)
			if err != nil {
				return snapshot, err
			}
			if err := w.checkpointBillingSuspensionStop(project.ID, taskID, snapshot); err != nil {
				return snapshot, err
			}
			snapshot, found, err = w.loadBillingSuspensionStopSnapshot(project.ID, taskID)
			if err != nil {
				return snapshot, err
			}
			if !found {
				return snapshot, errors.New("billing suspension stop snapshot missing after checkpoint")
			}
		}
		return stopSnapshotContainers(snapshot, true, w.dockerService.StopContainer, appendLog)
	}
	return stopProjectContainers(
		project,
		false,
		w.dockerService.IsContainerRunning,
		w.dockerService.StopContainer,
		appendLog,
		nil,
	)
}

func stopProjectContainers(project *models.Project, billingSuspension bool, isRunning func(string) (bool, error), stop func(string) error, appendLog func(string), checkpoint func(billingStopSnapshot) error) (billingStopSnapshot, error) {
	if project == nil {
		return billingStopSnapshot{}, errors.New("project is required")
	}

	var snapshot billingStopSnapshot
	if project.ContainerID != nil {
		snapshot.mainContainerID = *project.ContainerID
	}
	if project.WorkerContainerID != nil {
		snapshot.workerContainerID = *project.WorkerContainerID
	}
	if billingSuspension {
		observedSnapshot, err := inspectBillingStopSnapshot(project, isRunning)
		if err != nil {
			return observedSnapshot, err
		}
		snapshot = observedSnapshot
		if checkpoint != nil {
			if err := checkpoint(snapshot); err != nil {
				return snapshot, err
			}
		}
	}
	return stopSnapshotContainers(snapshot, billingSuspension, stop, appendLog)
}

func inspectBillingStopSnapshot(project *models.Project, isRunning func(string) (bool, error)) (billingStopSnapshot, error) {
	if project == nil {
		return billingStopSnapshot{}, errors.New("project is required")
	}
	var snapshot billingStopSnapshot
	if project.ContainerID != nil {
		snapshot.mainContainerID = *project.ContainerID
	}
	if project.WorkerContainerID != nil {
		snapshot.workerContainerID = *project.WorkerContainerID
	}
	inspectContainer := func(kind, containerID string, setRunning func()) error {
		if containerID == "" {
			return nil
		}
		running, err := isRunning(containerID)
		if err != nil {
			return fmt.Errorf("inspect %s container: %w", kind, err)
		}
		if running {
			setRunning()
		}
		return nil
	}
	if err := inspectContainer("main", snapshot.mainContainerID, func() { snapshot.mainWasRunning = true }); err != nil {
		return snapshot, err
	}
	if err := inspectContainer("worker", snapshot.workerContainerID, func() { snapshot.workerWasRunning = true }); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func stopSnapshotContainers(snapshot billingStopSnapshot, billingSuspension bool, stop func(string) error, appendLog func(string)) (billingStopSnapshot, error) {
	stopContainer := func(kind, containerID string, wasRunning bool, setStopped func()) error {
		if containerID == "" {
			return nil
		}
		if billingSuspension && !wasRunning {
			return nil
		}
		appendLog(fmt.Sprintf("Stopping %s container: %s", kind, containerID))
		if err := stop(containerID); err != nil {
			return fmt.Errorf("stop %s container: %w", kind, err)
		}
		setStopped()
		return nil
	}

	if err := stopContainer("main", snapshot.mainContainerID, snapshot.mainWasRunning, func() { snapshot.mainStopped = true }); err != nil {
		return snapshot, err
	}
	if err := stopContainer("worker", snapshot.workerContainerID, snapshot.workerWasRunning, func() { snapshot.workerStopped = true }); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (w *DeploymentWorker) restoreBillingStoppedContainers(snapshot billingStopSnapshot) error {
	return restoreBillingStoppedContainers(snapshot, w.dockerService.StartContainer)
}

func restoreBillingStoppedContainers(snapshot billingStopSnapshot, start func(string) error) error {
	var restartErr error
	if snapshot.mainStopped {
		if err := start(snapshot.mainContainerID); err != nil {
			restartErr = errors.Join(restartErr, fmt.Errorf("restart main container after stale billing stop: %w", err))
		}
	}
	if snapshot.workerStopped {
		if err := start(snapshot.workerContainerID); err != nil {
			restartErr = errors.Join(restartErr, fmt.Errorf("restart worker container after stale billing stop: %w", err))
		}
	}
	return restartErr
}

func (w *DeploymentWorker) checkpointBillingSuspensionStop(projectID, taskID uint, snapshot billingStopSnapshot) error {
	if w == nil || w.projectRepo == nil || projectID == 0 || taskID == 0 {
		return errBillingSuspensionNoLongerRequired
	}
	now := time.Now().UTC()
	return w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingSuspensionNoLongerRequired
			}
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusSuspended {
			return errBillingSuspensionNoLongerRequired
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingSuspensionNoLongerRequired
			}
			return err
		}
		if task.StopAttemptedAt != nil {
			return nil
		}
		return tx.Model(&task).Updates(map[string]any{
			"main_container_id":   snapshot.mainContainerID,
			"worker_container_id": snapshot.workerContainerID,
			"main_was_running":    snapshot.mainWasRunning,
			"worker_was_running":  snapshot.workerWasRunning,
			"stop_attempted_at":   &now,
			"last_error":          "",
		}).Error
	})
}

func (w *DeploymentWorker) loadBillingSuspensionStopSnapshot(projectID, taskID uint) (billingStopSnapshot, bool, error) {
	if w == nil || w.projectRepo == nil || projectID == 0 || taskID == 0 {
		return billingStopSnapshot{}, false, errBillingSuspensionNoLongerRequired
	}
	var snapshot billingStopSnapshot
	found := false
	err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingSuspensionNoLongerRequired
			}
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusSuspended {
			return errBillingSuspensionNoLongerRequired
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errBillingSuspensionNoLongerRequired
			}
			return err
		}
		if task.StopAttemptedAt == nil {
			return nil
		}
		snapshot = billingStopSnapshot{
			mainContainerID:   task.MainContainerID,
			workerContainerID: task.WorkerContainerID,
			mainWasRunning:    task.MainWasRunning,
			workerWasRunning:  task.WorkerWasRunning,
		}
		found = true
		return nil
	})
	return snapshot, found, err
}

type billingResumePlan struct {
	mainContainerID   string
	workerContainerID string
	mainWasRunning    bool
	workerWasRunning  bool
}

func (w *DeploymentWorker) resumeBillingSuspension(projectID, taskID uint) (bool, error) {
	if w == nil || w.projectRepo == nil || w.dockerService == nil || projectID == 0 || taskID == 0 {
		return false, nil
	}
	var plan billingResumePlan
	required := false
	err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusActive {
			return nil
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND completed_at IS NOT NULL AND resume_requested_at IS NOT NULL AND resume_completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return err
		}
		mainContainerID, workerContainerID := "", ""
		if project.ContainerID != nil {
			mainContainerID = *project.ContainerID
		}
		if project.WorkerContainerID != nil {
			workerContainerID = *project.WorkerContainerID
		}
		if (task.MainWasRunning && mainContainerID != task.MainContainerID) || (task.WorkerWasRunning && workerContainerID != task.WorkerContainerID) {
			now := time.Now().UTC()
			return tx.Model(&task).Update("resume_completed_at", &now).Error
		}
		plan = billingResumePlan{
			mainContainerID:   task.MainContainerID,
			workerContainerID: task.WorkerContainerID,
			mainWasRunning:    task.MainWasRunning,
			workerWasRunning:  task.WorkerWasRunning,
		}
		required = plan.mainWasRunning || plan.workerWasRunning
		return nil
	})
	if err != nil {
		return false, err
	}
	if !required {
		if err := w.stopStaleBillingResume(projectID, taskID); err != nil {
			return false, err
		}
		return false, nil
	}

	startIfStopped := func(kind, containerID string, shouldResume bool) (bool, error) {
		if !shouldResume {
			return false, nil
		}
		running, err := w.dockerService.IsContainerRunning(containerID)
		if err != nil {
			return false, fmt.Errorf("inspect billing resume %s container: %w", kind, err)
		}
		if running {
			return false, nil
		}
		if err := w.dockerService.StartContainer(containerID); err != nil {
			return false, fmt.Errorf("start billing resume %s container: %w", kind, err)
		}
		return true, nil
	}
	started := billingStopSnapshot{mainContainerID: plan.mainContainerID, workerContainerID: plan.workerContainerID}
	mainStarted, err := startIfStopped("main", plan.mainContainerID, plan.mainWasRunning)
	if err != nil {
		return true, err
	}
	started.mainStopped = mainStarted
	workerStarted, err := startIfStopped("worker", plan.workerContainerID, plan.workerWasRunning)
	if err != nil {
		return true, errors.Join(err, stopStartedBillingResumeContainers(started, w.dockerService.StopContainer))
	}
	started.workerStopped = workerStarted

	completed := false
	err = w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if resource.BillingStatus != models.BillableResourceStatusActive {
			return nil
		}
		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND resume_completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&task).Update("resume_completed_at", &now).Error; err != nil {
			return err
		}
		if plan.mainWasRunning {
			if err := tx.Model(&models.Project{}).Where("id = ?", projectID).Update("status", models.StatusRunning).Error; err != nil {
				return err
			}
		}
		completed = true
		return nil
	})
	if err != nil {
		return false, errors.Join(err, stopStartedBillingResumeContainers(started, w.dockerService.StopContainer))
	}
	if !completed {
		if err := stopResumeOwnedBillingContainers(plan, w.dockerService.StopContainer); err != nil {
			return false, err
		}
	}
	return completed, nil
}

func stopStartedBillingResumeContainers(snapshot billingStopSnapshot, stop func(string) error) error {
	var stopErr error
	if snapshot.workerStopped {
		if err := stop(snapshot.workerContainerID); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop worker container after stale billing resume: %w", err))
		}
	}
	if snapshot.mainStopped {
		if err := stop(snapshot.mainContainerID); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("stop main container after stale billing resume: %w", err))
		}
	}
	return stopErr
}

func stopResumeOwnedBillingContainers(plan billingResumePlan, stop func(string) error) error {
	return stopStartedBillingResumeContainers(billingStopSnapshot{
		mainContainerID:   plan.mainContainerID,
		workerContainerID: plan.workerContainerID,
		mainStopped:       plan.mainWasRunning,
		workerStopped:     plan.workerWasRunning,
	}, stop)
}

// stopStaleBillingResume fences containers started by an interrupted billing resume
// after the resource becomes non-active again.
func (w *DeploymentWorker) stopStaleBillingResume(projectID, taskID uint) error {
	if w == nil || w.projectRepo == nil || w.dockerService == nil || projectID == 0 || taskID == 0 {
		return nil
	}

	var plan billingResumePlan
	shouldStop := false
	err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if resource.BillingStatus == models.BillableResourceStatusActive {
			return nil
		}

		var task models.ProjectSuspensionTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ? AND billable_resource_id = ? AND user_id = ? AND resume_requested_at IS NOT NULL AND resume_completed_at IS NULL", taskID, projectID, resource.ID, resource.UserID).First(&task).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		plan = billingResumePlan{
			mainContainerID:   task.MainContainerID,
			workerContainerID: task.WorkerContainerID,
			mainWasRunning:    task.MainWasRunning,
			workerWasRunning:  task.WorkerWasRunning,
		}
		shouldStop = plan.mainWasRunning || plan.workerWasRunning
		return nil
	})
	if err != nil || !shouldStop {
		return err
	}
	return stopResumeOwnedBillingContainers(plan, w.dockerService.StopContainer)
}

func billingRuntimeAction(jobType string) bool {
	switch jobType {
	case "deploy", "redeploy", "redeploy_clean", "rollback", "start", "restart", "update_env":
		return true
	default:
		return false
	}
}

func billingQuotaReconciliationAction(jobType string) bool {
	switch jobType {
	case "deploy", "redeploy", "redeploy_clean", "rollback", "restart":
		return true
	default:
		return false
	}
}

// reconcileProjectBillingQuota restores the persisted runtime quota from the
// plan linked to a project. Catalog repricing creates immutable spec versions,
// so the resource's SpecID remains the authoritative assignment.
func (w *DeploymentWorker) reconcileProjectBillingQuota(ctx context.Context, project *models.Project) (bool, error) {
	if w == nil || w.cfg == nil || !w.cfg.BillingEnabled || w.projectRepo == nil || project == nil || project.ID == 0 {
		return false, nil
	}

	changed := false
	err := w.projectRepo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var resource models.BillableResource
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("type = ? AND resource_id = ? AND billing_status <> ?", models.BillableTypeProject, project.ID, models.BillableResourceStatusDeleted).
			First(&resource).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("load project billing resource: %w", err)
		}

		var spec models.BillableSpec
		if err := tx.Where("id = ? AND type = ?", resource.SpecID, models.BillableTypeProject).First(&spec).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("assigned project billing specification is missing")
			}
			return fmt.Errorf("load assigned project billing specification: %w", err)
		}
		if spec.CPUMillicores <= 0 || spec.MemoryMB <= 0 {
			return fmt.Errorf("assigned project billing specification is invalid")
		}

		cpuLimit := float64(spec.CPUMillicores) / 1000
		memoryLimit := fmt.Sprintf("%dm", spec.MemoryMB)
		if project.CPULimit != nil && project.MemoryLimit != nil && *project.CPULimit == cpuLimit && *project.MemoryLimit == memoryLimit {
			return nil
		}
		if err := tx.Model(&models.Project{}).Where("id = ?", project.ID).Updates(map[string]any{
			"cpu_limit":    cpuLimit,
			"memory_limit": memoryLimit,
		}).Error; err != nil {
			return fmt.Errorf("persist project billing quota: %w", err)
		}
		project.CPULimit = &cpuLimit
		project.MemoryLimit = &memoryLimit
		changed = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if !changed && project.CPULimit == nil && project.MemoryLimit == nil {
		slog.Warn("Billing-enabled project has no billable plan; using legacy runtime defaults", "project_id", project.ID)
	}
	return changed, nil
}

// deployProject handles the full deployment process
func (w *DeploymentWorker) deployProject(ctx context.Context, project *models.Project, job *infrastructure.DeploymentJob) {
	if job.Type == "delete" {
		slog.Info("Performing project background deletion", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "delete_started", "Purging project from system")

		if err := w.projectService.DeleteProject(project); err != nil {
			slog.Error("Failed to purge project during background delete job", "projectId", project.ID, "error", err)
			return
		}
		w.acknowledgeProjectDeletion(project.ID)
		return
	}

	previousCommitHash := project.LastCommitHash
	projectPath := project.GetProjectPath(w.cfg.ProjectsPath)
	logsDir := filepath.Join(projectPath, "logs")
	_ = os.MkdirAll(logsDir, 0755)

	// 1. Register log pruning and size truncation defer block first.
	// Because of LIFO execution of defers, this runs LAST (after log files have been cleanly closed).
	defer func() {
		if err := utils.PruneJobLogs(logsDir, "build-*.log", 5); err != nil {
			slog.Warn("Failed to prune build logs", "projectId", project.ID, "error", err)
		}
		if err := utils.PruneJobLogs(logsDir, "infra-*.log", 5); err != nil {
			slog.Warn("Failed to prune infra logs", "projectId", project.ID, "error", err)
		}
		infraLogPath := filepath.Join(logsDir, "infra.log")
		if err := utils.TruncateFileIfNeeded(infraLogPath, 5*1024*1024); err != nil {
			slog.Warn("Failed to truncate infra.log", "projectId", project.ID, "error", err)
		}
	}()

	isDeployment := job.Type == "deploy" || job.Type == "redeploy" || job.Type == "redeploy_clean"

	// 2. Open persistent infra.log once at start to avoid repeated open/close I/O performance bottleneck
	var infraLogFile *os.File
	if !isDeployment {
		infraLogPath := filepath.Join(logsDir, "infra.log")
		var errOpen error
		infraLogFile, errOpen = os.OpenFile(infraLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if errOpen != nil {
			slog.Error("Failed to open persistent infra log file", "path", infraLogPath, "error", errOpen)
		} else {
			defer infraLogFile.Close()
		}
	}

	logFilePrefix := "infra"
	if isDeployment {
		logFilePrefix = "build"
	}
	buildLogPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", logFilePrefix, job.JobID))

	// 3. Open job-specific log file next
	logFile, logOpenErr := os.OpenFile(buildLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if logOpenErr != nil {
		slog.Error("Failed to open job log file", "path", buildLogPath, "error", logOpenErr)
	} else {
		defer logFile.Close()
	}

	var logFileMu sync.Mutex
	appendLog := func(msg string) {
		logFileMu.Lock()
		defer logFileMu.Unlock()

		if logFile != nil {
			_, _ = logFile.WriteString(msg + "\n")
		}
		_ = w.redisService.PublishBuildLogForJob(project.ID, job.JobID, msg)

		if !isDeployment && infraLogFile != nil {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			_, _ = infraLogFile.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, msg))
		}
	}
	appendLog = w.makeRedactingLogger(project, appendLog)

	if billingQuotaReconciliationAction(job.Type) {
		quotaUpdated, err := w.reconcileProjectBillingQuota(ctx, project)
		if err != nil {
			appendLog("ERROR: Failed to resolve runtime limits from assigned billing plan.")
			slog.Error("Failed to reconcile project billing quota", "project_id", project.ID, "error", err)
			w.updateProjectError(project, job.JobID, "[BILLING_QUOTA_FAILED] Failed to resolve runtime limits from assigned billing plan.")
			return
		}
		if quotaUpdated {
			appendLog(">> Applied runtime limits from assigned billing plan.")
		}
	}

	if job.Type == "update_env" {
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "env_update_started", "Applying environment configuration")
		if project.ContainerID == nil || *project.ContainerID == "" {
			appendLog(">> Project is stopped. Regenerating environment configuration on disk...")
			projectDomain := w.cfg.ProjectDomain
			if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
				appendLog("✗ Failed to regenerate environment configuration on disk: " + err.Error())
				slog.Error("Failed to create env file for stopped project", "subdomain", project.Subdomain, "error", err)
				w.updateProjectError(project, job.JobID, "[ENV_UPDATE_FAILED] Failed to regenerate environment configuration on disk: "+err.Error())
				return
			}

			appendLog("✓ Environment configuration updated successfully on disk.")
			slog.Info("Project container is stopped. Skipping container restart for env update.", "subdomain", project.Subdomain)
			w.recordAuditLog(project.ID, job.JobID, "deployment-worker", "env_update_skipped_stopped", "Container is stopped. Environment updated on disk.")
			_ = w.projectRepo.UpdateStatus(project.ID, models.StatusStopped)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "env_update_completed", "Environment updated on disk")
			w.acknowledgeProjectEnvironmentSync(project.ID, job.EnvSyncGeneration)
			return
		}

		appendLog(">> Preparing database credentials rotation...")
		slog.Info("Performing instant environment update", "subdomain", project.Subdomain)
		w.recordAuditLog(project.ID, job.JobID, "deployment-worker", "env_update_started", "Updating environment and restarting container")

		if err := w.instantUpdateEnv(project, appendLog); err != nil {
			appendLog("")
			appendLog("✗ Environment update failed: " + err.Error())
			slog.Error("Instant update failed", "subdomain", project.Subdomain, "error", err)
			w.updateProjectError(project, job.JobID, "[ENV_UPDATE_FAILED] Failed to update environment variables: "+err.Error())
		} else {
			appendLog("")
			appendLog("✓ Environment update completed successfully!")
			w.recordAuditLog(project.ID, job.JobID, "deployment-worker", "env_update_completed", "Environment variables updated successfully")
			_ = w.projectRepo.UpdateStatus(project.ID, models.StatusRunning)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "env_update_completed", "Environment variables updated successfully")
			w.acknowledgeProjectEnvironmentSync(project.ID, job.EnvSyncGeneration)
		}
		return
	}

	if job.Type == "rollback" {
		appendLog(fmt.Sprintf(">> Preparing rollback to commit %s...", project.LastCommitHash))
		slog.Info("Performing instant rollback", "subdomain", project.Subdomain, "commit", project.LastCommitHash)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "rollback_started", fmt.Sprintf("Rolling back to %s", project.LastCommitHash))
		if err := w.redeployExistingImage(project, appendLog); err == nil {
			appendLog("")
			appendLog("✓ Rollback completed successfully.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "rollback_completed", project.LastCommitHash)
			return
		}
		appendLog("")
		appendLog("✗ Rollback failed. Falling back to full rebuild.")
		slog.Warn("Instant rollback failed, falling back to full build deployment", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 30, "rollback_fallback", "Instant rollback failed, falling back to rebuild")
	}

	if job.Type == "stop" {
		slog.Info("Performing container stop action", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "stop_started", "Stopping application container(s)")
		appendLog(">> Stopping application container(s)...")

		stopSnapshot, stopErr := w.stopProjectContainers(project, job.BillingSuspensionTaskID, job.BillingSuspension, appendLog)
		if errors.Is(stopErr, errBillingSuspensionNoLongerRequired) {
			appendLog(">> Billing suspension stop became stale before containers changed.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "billing_stop_stale", "Payment completed before billing suspension stop began")
			return
		}
		if stopErr != nil {
			if job.BillingSuspension && stopSnapshot.anyStopped() {
				if compensationErr := w.restoreBillingStoppedContainers(stopSnapshot); compensationErr != nil {
					stopErr = errors.Join(stopErr, fmt.Errorf("restore containers after partial billing stop: %w", compensationErr))
				}
			}
			appendLog("✗ Failed to stop one or more application containers: " + stopErr.Error())
			w.updateProjectError(project, job.JobID, "Failed to stop project containers: "+stopErr.Error())
			return
		}
		if job.BillingSuspension {
			finalized, err := w.finalizeBillingSuspensionStop(project.ID, job.BillingSuspensionTaskID)
			if err != nil {
				appendLog("✗ Failed to finalize billing suspension: " + err.Error())
				w.updateProjectError(project, job.JobID, "Failed to finalize billing suspension: "+err.Error())
				return
			}
			if !finalized {
				appendLog(">> Billing payment completed during suspension. Restoring application container(s)...")
				restartErr := w.restoreBillingStoppedContainers(stopSnapshot)
				if restartErr != nil {
					appendLog("✗ Failed to restore container(s) after stale billing stop: " + restartErr.Error())
					w.updateProjectError(project, job.JobID, "Failed to restore containers after stale billing stop: "+restartErr.Error())
					return
				}
				if stopSnapshot.anyWasRunning() {
					if err := w.projectRepo.UpdateStatus(project.ID, models.StatusRunning); err != nil {
						slog.Error("Failed to restore project status after stale billing stop", "id", project.ID, "error", err)
					}
				}
				appendLog("✓ Billing suspension stop discarded after payment.")
				w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "billing_stop_reverted", "Payment completed before billing suspension stop finalized")
				return
			}
			project.Status = models.StatusStopped
		} else {
			project.Status = models.StatusStopped
			if err := w.projectRepo.UpdateStatus(project.ID, models.StatusStopped); err != nil {
				slog.Error("Failed to update project status to stopped", "id", project.ID, "error", err)
			}
		}

		if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
			slog.Warn("Failed to invalidate cache", "subdomain", project.Subdomain, "error", err)
		}

		appendLog("✓ Container(s) stopped successfully.")
		w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "stop_completed", "Container stopped successfully")
		return
	}

	if job.Type == "start" {
		if job.BillingResume {
			resumed, err := w.resumeBillingSuspension(project.ID, job.BillingSuspensionTaskID)
			if err != nil {
				appendLog("✗ Failed to resume billing-suspended container(s): " + err.Error())
				w.updateProjectError(project, job.JobID, "Failed to resume billing-suspended containers: "+err.Error())
				return
			}
			if !resumed {
				appendLog(">> Billing resume became stale before containers changed.")
				w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "billing_resume_stale", "Billing resume was no longer required")
				return
			}
			if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
				slog.Warn("Failed to invalidate cache after billing resume", "subdomain", project.Subdomain, "error", err)
			}
			appendLog("✓ Billing-suspended container(s) resumed successfully.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "billing_resume_completed", "Billing-suspended container(s) resumed successfully")
			return
		}
		slog.Info("Performing container start action", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "start_started", "Starting application container(s)")
		appendLog(">> Starting application container(s)...")

		if project.ContainerID != nil && *project.ContainerID != "" {
			appendLog(fmt.Sprintf("Starting main container: %s", *project.ContainerID))
			if err := w.dockerService.StartContainer(*project.ContainerID); err != nil {
				appendLog(fmt.Sprintf("Error starting main container: %v", err))
				w.updateProjectError(project, job.JobID, "Failed to start main container: "+err.Error())
				return
			}
		}
		if project.WorkerContainerID != nil && *project.WorkerContainerID != "" {
			appendLog(fmt.Sprintf("Starting worker container: %s", *project.WorkerContainerID))
			if err := w.dockerService.StartContainer(*project.WorkerContainerID); err != nil {
				appendLog(fmt.Sprintf("Warning: Failed to start worker container: %v", err))
			}
		}

		project.Status = models.StatusRunning
		if err := w.projectRepo.UpdateStatus(project.ID, models.StatusRunning); err != nil {
			slog.Error("Failed to update project status to running", "id", project.ID, "error", err)
		}

		if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
			slog.Warn("Failed to invalidate cache", "subdomain", project.Subdomain, "error", err)
		}

		appendLog("✓ Container(s) started successfully.")
		w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "start_completed", "Container started successfully")
		return
	}

	if job.Type == "restart" {
		slog.Info("Performing container restart action", "subdomain", project.Subdomain)
		w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 20, "restart_started", "Restarting application container(s)")
		w.recordAuditLog(project.ID, job.JobID, "deployment-worker", "restart_started", "Restarting application container(s)")
		appendLog(">> Restarting application container(s)...")

		if err := w.projectService.RecreateProjectZeroDowntime(project, appendLog); err != nil {
			appendLog("")
			appendLog("✗ Restart failed: " + err.Error())
			slog.Error("Restart failed", "subdomain", project.Subdomain, "error", err)
			w.updateProjectError(project, job.JobID, "[RESTART_FAILED] Failed to restart application container(s): "+err.Error())
		} else {
			appendLog("")
			appendLog("✓ Container(s) restarted successfully.")
			w.recordAuditLog(project.ID, job.JobID, "deployment-worker", "restart_completed", "Container restarted successfully")
			_ = w.projectRepo.UpdateStatus(project.ID, models.StatusRunning)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "restart_completed", "Container restarted successfully")
		}
		return
	}

	w.transitionDeploymentState(project, job.JobID, models.DepStatusPreparing, 10, "deployment_started", fmt.Sprintf("Triggered by %s", job.Type))
	project.ErrorLog = nil
	_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"error_log": nil,
	})

	w.checkDiskSpace()

	// Obtain GitHub App installation token for authenticating private repositories
	authURL := project.GithubURL
	var installationID int64

	owner := ""
	trimmed := strings.TrimPrefix(authURL, "https://github.com/")
	trimmed = strings.TrimPrefix(trimmed, "http://github.com/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) > 0 {
		owner = parts[0]
	}

	if project.GithubInstallationID != nil && *project.GithubInstallationID != 0 {
		// Verify if the stored installation matches the repository owner
		if matches, err := w.projectRepo.VerifyInstallationID(*project.GithubInstallationID, owner); err == nil && !matches {
			slog.Warn("GitHub Installation ID mismatch detected for project, forcing re-resolution", "projectId", project.ID, "storedID", *project.GithubInstallationID, "owner", owner)
			project.GithubInstallationID = nil
		}
	}

	if project.GithubInstallationID != nil && *project.GithubInstallationID != 0 {
		installationID = *project.GithubInstallationID
	} else {
		// Dynamic Self-Healing: Attempt to resolve missing installation ID from owner or user's account
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
		checkImg, err := exec.Command("docker", "image", "inspect", imageName).Output()
		if err == nil && len(checkImg) > 0 && strings.TrimSpace(string(checkImg)) != "[]" {
			slog.Info("Valid image found, skipping build", "subdomain", project.Subdomain)
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "deployment_skipped_existing_image", latestHash)
			if err := w.redeployExistingImage(project, appendLog); err == nil {
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
		projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(project.UserID, authURL, project.Branch, project.Subdomain)
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
				projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(project.UserID, retryURL, project.Branch, project.Subdomain)
			}
		}
	}()

	go func() {
		defer wg.Done()
		if project.DatabaseInstance == nil {
			return // no Runara database for this project, skip provisioning
		}
		if project.DatabaseOption != "new" {
			return // existing/attached databases are already provisioned
		}
		if project.DatabasePassword == "" {
			project.DatabasePassword = utils.GeneratePassword(16)
			if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"database_password": project.DatabasePassword,
			}); err != nil {
				slog.Warn("Failed to save database password", "id", project.ID, "error", err)
			}
		}

		engine := "mysql"
		dbName := project.GetDatabaseName()
		dbUsername := dbName
		dbPassword := project.DatabasePassword
		connectionLimit := infrastructure.DefaultManagedDatabaseConnectionLimit
		var dbInstance *models.DatabaseInstance
		if project.DatabaseInstance != nil {
			engine = project.DatabaseInstance.Engine
			dbInstance = project.DatabaseInstance
			dbName = dbInstance.Name
			dbUsername = dbInstance.Username
			dbPassword = dbInstance.Password
			if dbPassword == "" {
				dbPassword = project.DatabasePassword
			}
			if dbInstance.ConnectionLimit > 0 {
				connectionLimit = dbInstance.ConnectionLimit
			}
		}

		var host string
		var port int
		switch engine {
		case "mysql":
			host = infrastructure.MySQLContainerName()
			port = infrastructure.MySQLPort()
		case "postgresql":
			host = infrastructure.PostgreSQLContainerName()
			port = infrastructure.PostgreSQLPort()
		default:
			dbErr = fmt.Errorf("unsupported managed database engine %q", engine)
			return
		}

		verifyDatabase := func() (string, error) {
			return inspectManagedDatabase(ctx, engine, host, port, dbName, dbUsername, dbPassword)
		}
		provisionDatabase := func() error {
			switch engine {
			case "mysql":
				return w.mysqlService.CreateDatabaseCustomWithConnectionLimit(dbName, dbUsername, dbPassword, connectionLimit)
			case "postgresql":
				return infrastructure.NewPostgreSQLService().CreateDatabaseCustomWithConnectionLimit(dbName, dbUsername, dbPassword, connectionLimit)
			default:
				return fmt.Errorf("unsupported managed database engine %q", engine)
			}
		}

		var version string
		version, dbErr = ensureManagedDatabase(verifyDatabase, provisionDatabase)
		if dbErr == nil && dbInstance != nil {
			dbInstance.Host = host
			dbInstance.Port = port
			dbInstance.Password = dbPassword
			dbInstance.Status = models.DBStatusActive
			dbInstance.Version = version
			_ = w.projectRepo.SaveDatabaseInstance(dbInstance)
		}
	}()

	wg.Wait()

	if cloneErr != nil {
		w.updateProjectError(project, job.JobID, "[CLONE_FAILED] Failed to clone repository: "+cloneErr.Error())
		return
	}
	if dbErr != nil {
		w.updateProjectError(project, job.JobID, "[INFRASTRUCTURE_FAILED] Failed to create database: "+dbErr.Error())
		return
	}

	project.LastCommitHash = cloneHash
	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"last_commit_hash": cloneHash,
	}); err != nil {
		slog.Warn("Failed to update commit hash", "id", project.ID, "error", err)
	}

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

	buildPath, err := w.dockerService.ResolveBuildPath(projectPath, project.BaseDirectory)
	if err != nil {
		appendLog("ERROR: " + err.Error())
		w.updateProjectError(project, job.JobID, err.Error())
		return
	}

	oldFramework := project.Framework
	detection := w.detectProjectRuntime(ctx, project, buildPath)
	project.Framework = detection.Framework
	if project.Framework != oldFramework {
		project.NodeVersion = ""
		project.PHPVersion = ""
		project.LaravelVersion = ""
		project.LanguageVersion = ""
		project.IsManualVersion = false
	}

	if project.Framework == "Laravel" {
		laravelVersion, phpVersion, err := w.versionService.DetectVersions(buildPath)
		if err == nil {
			project.LaravelVersion = laravelVersion
			if !project.IsManualVersion {
				project.PHPVersion = phpVersion
			}
		} else if project.PHPVersion == "" {
			project.PHPVersion = "8.4"
		}
	} else if isJSFramework(project.Framework) {
		if !project.IsManualVersion {
			project.NodeVersion = detection.RuntimeVersion
			project.LanguageVersion = detection.RuntimeVersion
		}
	} else if !project.IsManualVersion {
		project.LanguageVersion = detection.RuntimeVersion
	}

	if detection.RuntimeVersion == "" && !project.IsManualVersion {
		if version, err := w.versionService.DetectRuntimeVersion(buildPath, project.Framework); err == nil {
			if isJSFramework(project.Framework) {
				project.NodeVersion = version
			}
			if project.Framework != "Laravel" {
				project.LanguageVersion = version
			}
		}
	}

	updates := map[string]interface{}{
		"framework":         project.Framework,
		"language_version":  project.LanguageVersion,
		"node_version":      project.NodeVersion,
		"php_version":       project.PHPVersion,
		"laravel_version":   project.LaravelVersion,
		"is_manual_version": project.IsManualVersion,
	}
	slog.Info("Detected project runtime",
		"subdomain", project.Subdomain,
		"framework", detection.Framework,
		"provider", detection.Provider,
		"runtime", detection.Runtime,
		"runtime_version", detection.RuntimeVersion,
		"source", detection.Source,
	)
	appendLog(fmt.Sprintf(">> Runtime detected: %s", detection.Framework))
	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{"detected_framework": detection.Framework}); err != nil {
		slog.Warn("Failed to persist detected runtime candidate", "id", project.ID, "error", err)
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
	// BuildAndRun only assigns this field when a new worker starts. Clearing the
	// stale value makes every pre-promotion cleanup target the rollout worker only.
	project.WorkerContainerID = nil

	projectDomain := w.cfg.ProjectDomain

	cpuPercentStr := w.getSetting(models.SettingCPULimit, models.DefaultCPULimit)
	cpuPercent, _ := strconv.ParseFloat(cpuPercentStr, 64)
	cpuLimit := cpuPercent / 100.0
	if project.CPULimit != nil && *project.CPULimit > 0 {
		cpuLimit = *project.CPULimit
	}

	memoryMB := w.getSetting(models.SettingMemoryLimit, models.DefaultMemoryLimit)
	memoryLimit := memoryMB + "m"
	if project.MemoryLimit != nil && *project.MemoryLimit != "" {
		memoryLimit = *project.MemoryLimit
	}

	buildTimeoutSec, err := strconv.Atoi(w.getSetting(models.SettingBuildTimeout, models.DefaultBuildTimeout))
	if err != nil || buildTimeoutSec <= 0 {
		buildTimeoutSec = 1800
	}
	buildCtx, buildCancel := context.WithTimeout(ctx, time.Duration(buildTimeoutSec)*time.Second)
	defer buildCancel()

	newContainerID, err := w.dockerService.BuildAndRun(buildCtx, project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit, job.Type == "deploy", job.Type == "redeploy_clean", appendLog)
	if err != nil {
		if newContainerID != "" {
			project.RolloutContainerID = &newContainerID
			project.RolloutWorkerContainerID = project.WorkerContainerID
			if checkpointErr := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"rollout_container_id":        newContainerID,
				"rollout_worker_container_id": project.WorkerContainerID,
			}); checkpointErr != nil {
				slog.Error("Failed to persist orphaned rollout recovery state", "projectId", project.ID, "containerId", newContainerID, "error", checkpointErr)
			}
		}
		sharedDocker.GetCircuitBreaker().RecordFailure()
		if previousCommitHash != "" {
			imageName := fmt.Sprintf("paas-%s", project.Subdomain)
			_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"last_commit_hash": previousCommitHash,
		})
		if cloneHash != "" {
			_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
		}

		if ctx.Err() == context.Canceled {
			appendLog("ERROR: Deployment cancelled by user request.")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusCancelled, project.DeploymentProgress, "deployment_cancelled", "User requested cancellation")
			w.updateProjectError(project, job.JobID, "[TIMEOUT_EXCEEDED] Deployment cancelled by user request.")
			return
		}
		if ctx.Err() == context.DeadlineExceeded {
			appendLog("ERROR: Deployment timed out (watchdog kill).")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusFailed, project.DeploymentProgress, "watchdog_timeout", "Watchdog timed out the overall deployment process")
			w.updateProjectError(project, job.JobID, "[TIMEOUT_EXCEEDED] Deployment aborted: Execution watchdog timeout exceeded.")
			return
		}
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			appendLog("ERROR: Deployment build phase timed out (build limit).")
			w.transitionDeploymentState(project, job.JobID, models.DepStatusFailed, project.DeploymentProgress, "watchdog_timeout", "Build log watchdog timed out build step")
			w.updateProjectError(project, job.JobID, "[BUILD_FAILED] Deployment failed: Build step exceeded maximum allowed time limit.")
			return
		}
		appendLog("ERROR: Failed to deploy container: " + err.Error())
		w.updateProjectError(project, job.JobID, "[BUILD_FAILED] Failed to deploy container: "+err.Error())
		return
	}
	sharedDocker.GetCircuitBreaker().RecordSuccess()

	w.transitionDeploymentState(project, job.JobID, models.DepStatusStarting, 50, "starting_container", "Launching new container instance")

	project.RolloutContainerID = &newContainerID
	project.RolloutWorkerContainerID = project.WorkerContainerID
	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"rollout_container_id":        newContainerID,
		"rollout_worker_container_id": project.WorkerContainerID,
		"port":                        project.Port,
	}); err != nil {
		if !w.cleanupRollout(project, newContainerID, project.WorkerContainerID) {
			if checkpointErr := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"rollout_container_id":        newContainerID,
				"rollout_worker_container_id": project.WorkerContainerID,
			}); checkpointErr != nil {
				slog.Error("Failed to persist rollout after cleanup failure", "projectId", project.ID, "containerId", newContainerID, "error", checkpointErr)
			}
		}
		w.updateProjectError(project, job.JobID, "[ROLLOUT_CHECKPOINT_FAILED] Failed to persist rollout recovery state: "+err.Error())
		return
	}

	// For SQLite Laravel projects, run migrations BEFORE healthcheck.
	// The app cannot pass healthcheck without database tables (SQLSTATE no such table).
	if project.Framework == "Laravel" && project.DatabaseOption == "sqlite" {
		if ctx.Err() == context.Canceled {
			slog.Info("Deployment cancelled before migrations, rolling back", "subdomain", project.Subdomain)
			appendLog("ERROR: Deployment cancelled by user request. Rolling back.")

			sharedDocker.GetCircuitBreaker().RecordFailure()
			if previousCommitHash != "" {
				imageName := fmt.Sprintf("paas-%s", project.Subdomain)
				_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
			}
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"last_commit_hash": previousCommitHash,
			})
			if cloneHash != "" {
				_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
			}

			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Cancelled before migration")
			w.cleanupRollout(project, newContainerID, project.WorkerContainerID)
			w.updateProjectError(project, job.JobID, "[TIMEOUT_EXCEEDED] Deployment cancelled by user before migrations. Old version is still running.")
			return
		}
		w.transitionDeploymentState(project, job.JobID, models.DepStatusMigrating, 55, "running_migrations", "Executing artisan migrate --force (pre-healthcheck for SQLite)")
		slog.Info("Running database migrations before healthcheck (SQLite)", "subdomain", project.Subdomain)
		appendLog(">> Running database migrations (SQLite: before healthcheck)...")
		if output, err := w.dockerService.RunMigrations(ctx, newContainerID); err != nil {
			migrationErrorCode := utils.MigrationErrorCode(output)
			slog.Error("Migrations failed", "subdomain", project.Subdomain, "errorCode", migrationErrorCode, "error", err)
			appendLog(fmt.Sprintf("ERROR [%s]: Migrations failed:\n%s", migrationErrorCode, utils.SanitizeLogOutput(output)))
			appendLog(migrationPartialChangesWarning)

			sharedDocker.GetCircuitBreaker().RecordFailure()
			if previousCommitHash != "" {
				imageName := fmt.Sprintf("paas-%s", project.Subdomain)
				_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
			}
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"last_commit_hash": previousCommitHash,
			})
			if cloneHash != "" {
				_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
			}

			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Migrations failed")
			w.cleanupRollout(project, newContainerID, project.WorkerContainerID)
			w.updateProjectError(project, job.JobID, fmt.Sprintf("[%s] Migrations failed: %s\n\nOutput:\n%s", migrationErrorCode, err.Error(), output))
			return
		} else {
			if strings.TrimSpace(output) != "" {
				appendLog(strings.TrimRight(output, "\r\n"))
			} else {
				appendLog("✓ Database migrations ran successfully (nothing to migrate).")
			}
		}
	}

	appendLog(">> Starting container readiness checks...")
	w.transitionDeploymentState(project, job.JobID, models.DepStatusHealthchecking, 65, "healthchecking_container", "Executing readiness probe and stabilization monitoring")

	if err := w.dockerService.AdvancedHealthcheck(ctx, project, newContainerID, appendLog, project.ResolveRuntimeExposure(0)); err != nil {
		slog.Error("New container failed advanced healthcheck, initiating rollback", "subdomain", project.Subdomain, "id", newContainerID, "error", err)
		appendLog("ERROR: Health check failed: " + err.Error() + ". Rolling back.")

		sharedDocker.GetCircuitBreaker().RecordFailure()
		if previousCommitHash != "" {
			imageName := fmt.Sprintf("paas-%s", project.Subdomain)
			_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"last_commit_hash": previousCommitHash,
		})
		if cloneHash != "" {
			_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
		}

		w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Healthcheck failed, keeping old version active")

		w.cleanupRollout(project, newContainerID, project.WorkerContainerID)

		w.updateProjectError(project, job.JobID, "[RUNTIME_FAILED] Deployment failed healthcheck: "+err.Error()+". Old version is still running.")
		return
	} else {
		appendLog("✓ Health check passed.")
	}

	// For non-SQLite Laravel projects, run migrations after healthcheck (original behavior)
	if project.Framework == "Laravel" && project.DatabaseOption != "sqlite" {
		if ctx.Err() == context.Canceled {
			slog.Info("Deployment cancelled before migrations, rolling back", "subdomain", project.Subdomain)
			appendLog("ERROR: Deployment cancelled by user request. Rolling back.")

			sharedDocker.GetCircuitBreaker().RecordFailure()
			if previousCommitHash != "" {
				imageName := fmt.Sprintf("paas-%s", project.Subdomain)
				_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
			}
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"last_commit_hash": previousCommitHash,
			})
			if cloneHash != "" {
				_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
			}

			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Cancelled before migration")
			w.cleanupRollout(project, newContainerID, project.WorkerContainerID)
			w.updateProjectError(project, job.JobID, "[TIMEOUT_EXCEEDED] Deployment cancelled by user before migrations. Old version is still running.")
			return
		}
		w.transitionDeploymentState(project, job.JobID, models.DepStatusMigrating, 75, "running_migrations", "Executing artisan migrate --force")
		slog.Info("Running database migrations", "subdomain", project.Subdomain)
		appendLog(">> Running database migrations...")
		if output, err := w.dockerService.RunMigrations(ctx, newContainerID); err != nil {
			migrationErrorCode := utils.MigrationErrorCode(output)
			slog.Error("Migrations failed", "subdomain", project.Subdomain, "errorCode", migrationErrorCode, "error", err)
			appendLog(fmt.Sprintf("ERROR [%s]: Migrations failed:\n%s", migrationErrorCode, utils.SanitizeLogOutput(output)))
			appendLog(migrationPartialChangesWarning)

			sharedDocker.GetCircuitBreaker().RecordFailure()
			if previousCommitHash != "" {
				imageName := fmt.Sprintf("paas-%s", project.Subdomain)
				_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
			}
			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
				"last_commit_hash": previousCommitHash,
			})
			if cloneHash != "" {
				_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
			}

			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Migrations failed")
			w.cleanupRollout(project, newContainerID, project.WorkerContainerID)
			w.updateProjectError(project, job.JobID, fmt.Sprintf("[%s] Migrations failed: %s\n\nOutput:\n%s", migrationErrorCode, err.Error(), output))
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
		slog.Error("Nginx sync failed during promote, rolling back", "subdomain", project.Subdomain, "error", err)
		appendLog("ERROR: Failed to update public routing: " + err.Error())

		if previousCommitHash != "" {
			imageName := fmt.Sprintf("paas-%s", project.Subdomain)
			_, _ = utils.Run(1*time.Minute, "docker", "tag", fmt.Sprintf("%s:%s", imageName, previousCommitHash), imageName+":latest")
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"last_commit_hash": previousCommitHash,
		})
		if cloneHash != "" {
			_ = utils.RunSilent(1*time.Minute, "docker", "rmi", fmt.Sprintf("paas-%s:%s", project.Subdomain, cloneHash))
		}

		w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Routing sync failed")

		w.cleanupRollout(project, newContainerID, project.WorkerContainerID)

		w.updateProjectError(project, job.JobID, "[ROUTING_FAILED] Failed to update public routing: "+err.Error())
		return
	}

	if err := w.projectService.PromoteRolloutContainerWithWorker(project.ID, newContainerID, project.WorkerContainerID); err != nil {
		slog.Error("Failed to promote rollout container", "id", project.ID, "error", err)
		appendLog("ERROR: Failed to promote deployment: " + err.Error())
		w.cleanupRollout(project, newContainerID, project.WorkerContainerID)
		w.updateProjectError(project, job.JobID, "[ROLLOUT_PROMOTION_FAILED] Failed to promote release: "+err.Error())
		return
	}
	if err := w.projectRepo.UpdateMetadata(project.ID, updates); err != nil {
		slog.Warn("Failed to promote detected runtime metadata", "id", project.ID, "error", err)
	}
	appendLog("✓ Release promoted successfully.")
	if cloneHash != "" {
		_ = w.redisService.SetIdempotency(project.ID, cloneHash, project.Subdomain, job.Type)
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

	appendLog("")
	appendLog("========================================================================")
	appendLog("✓ Deployment completed successfully! Application is live.")
	appendLog("========================================================================")

	if !w.transitionDeploymentState(project, job.JobID, models.DepStatusCompleted, 100, "deployment_completed", project.LastCommitHash) {
		w.forceDeploymentCompleted(project, job.JobID)
	}

	go func() {
		_ = utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f")
		_ = utils.RunSilent(5*time.Minute, "docker", "volume", "prune", "-f")
	}()
}

func (w *DeploymentWorker) acknowledgeProjectEnvironmentSync(projectID, generation uint) {
	if generation == 0 {
		return
	}
	if err := w.projectRepo.DB().Transaction(func(tx *gorm.DB) error {
		var task models.ProjectEnvSyncTask
		if err := tx.Where("project_id = ?", projectID).First(&task).Error; err != nil {
			return err
		}
		if task.AcknowledgedGeneration >= generation {
			return nil
		}
		return tx.Model(&task).Updates(map[string]any{"acknowledged_generation": generation, "last_error": ""}).Error
	}); err != nil {
		slog.Error("Failed to acknowledge durable project environment sync", "projectId", projectID, "generation", generation, "error", err)
	}
}

func (w *DeploymentWorker) acknowledgeProjectDeletion(projectID uint) {
	now := time.Now().UTC()
	if err := w.projectRepo.DB().Model(&models.ProjectDeletionTask{}).
		Where("project_id = ? AND completed_at IS NULL", projectID).
		Update("completed_at", now).Error; err != nil {
		slog.Error("Failed to acknowledge durable project deletion", "projectId", projectID, "error", err)
	}
}

func (w *DeploymentWorker) acknowledgeDeploymentJob(job *infrastructure.DeploymentJob) {
	if err := w.redisService.AcknowledgeDeployment(job); err != nil {
		slog.Error("Failed to acknowledge deployment job", "jobId", job.JobID, "projectId", job.ProjectID, "error", err)
	}
}

func detectJSFrameworkFromPackage(packagePath string) string {
	info, err := os.Lstat(packagePath)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return ""
	}

	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Scripts         map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}

	hasDependency := func(name string) bool {
		if _, ok := manifest.Dependencies[name]; ok {
			return true
		}
		_, ok := manifest.DevDependencies[name]
		return ok
	}

	switch {
	case hasDependency("next"):
		return "Next.js"
	case hasDependency("nuxt") || hasDependency("@nuxt/kit"):
		return "Nuxt.js"
	case hasDependency("express") || hasDependency("fastify") || hasDependency("koa") || hasDependency("hono") || hasDependency("@nestjs/core"):
		return "Node.js"
	case hasDependency("@angular/core") || hasDependency("@angular/cli"):
		return "Angular"
	case hasDependency("svelte") || hasDependency("@sveltejs/kit"):
		return "Svelte"
	case hasDependency("vue"):
		return "Vue"
	case hasDependency("react") || hasDependency("react-dom"):
		return "React"
	}

	return "Node.js"
}

type projectRuntimeDetection struct {
	Framework      string
	Provider       string
	Runtime        string
	RuntimeVersion string
	Source         string
}

type railpackProjectInfo struct {
	Success           bool                       `json:"success"`
	DetectedProviders []string                   `json:"detectedProviders"`
	Metadata          map[string]string          `json:"metadata"`
	ResolvedPackages  map[string]railpackPackage `json:"resolvedPackages"`
}

type railpackPackage struct {
	RequestedVersion string `json:"requestedVersion"`
	ResolvedVersion  string `json:"resolvedVersion"`
}

func (w *DeploymentWorker) detectProjectRuntime(ctx context.Context, project *models.Project, buildPath string) projectRuntimeDetection {
	args := []string{"info", "--format", "json"}
	if project.BuildCommand != "" {
		args = append(args, "--build-cmd", project.BuildCommand)
	}
	if project.StartCommand != "" {
		args = append(args, "--start-cmd", project.StartCommand)
	}
	args = append(args, buildPath)

	res, err := utils.RunCtx(ctx, 20*time.Second, "railpack", args...)
	if err == nil {
		var info railpackProjectInfo
		if json.Unmarshal([]byte(res.Stdout), &info) == nil && info.Success {
			if detection := runtimeDetectionFromRailpack(info, buildPath); detection.Framework != "Other" {
				return detection
			}
		}
	} else {
		slog.Warn("Railpack runtime inspection failed; using marker fallback", "path", buildPath, "error", err)
	}
	if isLaravelManifest(buildPath) {
		return projectRuntimeDetection{Framework: "Laravel", Provider: "php", Runtime: "laravel", Source: "manifest"}
	}

	detection := w.detectProjectRuntimeFallback(buildPath)
	detection.Source = "markers"
	return detection
}

func runtimeDetectionFromRailpack(info railpackProjectInfo, buildPath string) projectRuntimeDetection {
	provider := ""
	if strings.EqualFold(info.Metadata["phpLaravel"], "true") {
		provider = "php"
	} else if len(info.DetectedProviders) > 0 {
		provider = strings.ToLower(info.DetectedProviders[0])
	} else {
		provider = strings.ToLower(info.Metadata["providers"])
	}

	detection := projectRuntimeDetection{Provider: provider, Source: "railpack", Framework: "Other"}
	switch provider {
	case "php":
		if strings.EqualFold(info.Metadata["phpLaravel"], "true") {
			detection.Framework = "Laravel"
			detection.Runtime = "laravel"
		} else {
			detection.Framework = "PHP"
			detection.Runtime = "php"
		}
		detection.RuntimeVersion = railpackRuntimeVersion(info, "php", 2)
	case "node":
		detection.Runtime = strings.ToLower(info.Metadata["nodeRuntime"])
		switch detection.Runtime {
		case "next":
			detection.Framework = "Next.js"
		case "nuxt":
			detection.Framework = "Nuxt.js"
		case "vite":
			detection.Framework = "Vite"
		default:
			detection.Framework = detectJSFrameworkFromPackage(filepath.Join(buildPath, "package.json"))
			if detection.Framework == "" {
				detection.Framework = "Node.js"
			}
		}
		detection.RuntimeVersion = railpackRuntimeVersion(info, "node", 1)
	case "golang", "go":
		detection.Framework = "Go"
		detection.Runtime = "go"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "go", 2)
	case "python":
		detection.Framework = "Python"
		detection.Runtime = strings.ToLower(info.Metadata["pythonRuntime"])
		detection.RuntimeVersion = railpackRuntimeVersion(info, "python", 2)
	case "ruby":
		detection.Framework = "Ruby"
		detection.Runtime = "ruby"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "ruby", 2)
	case "java":
		detection.Framework = "Java"
		detection.Runtime = "java"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "java", 2)
	case "rust":
		detection.Framework = "Rust"
		detection.Runtime = "rust"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "rust", 2)
	case "elixir":
		detection.Framework = "Elixir"
		detection.Runtime = "elixir"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "elixir", 2)
	case "deno":
		detection.Framework = "Deno"
		detection.Runtime = "deno"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "deno", 2)
	case "dotnet":
		detection.Framework = ".NET"
		detection.Runtime = "dotnet"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "dotnet", 2)
	case "gleam":
		detection.Framework = "Gleam"
		detection.Runtime = "gleam"
		detection.RuntimeVersion = railpackRuntimeVersion(info, "gleam", 2)
	case "cpp":
		detection.Framework = "C/C++"
		detection.Runtime = "cpp"
	case "shell":
		detection.Framework = "Shell"
		detection.Runtime = "shell"
	case "staticfile":
		detection.Framework = "Static"
		detection.Runtime = "static"
	}
	return detection
}

func railpackRuntimeVersion(info railpackProjectInfo, packageName string, parts int) string {
	pkg, ok := info.ResolvedPackages[packageName]
	if !ok {
		return ""
	}
	version := pkg.RequestedVersion
	if version == "" || strings.EqualFold(version, "latest") {
		version = pkg.ResolvedVersion
	}
	matches := regexp.MustCompile(`\d+`).FindAllString(version, parts)
	return strings.Join(matches, ".")
}

func isLaravelManifest(buildPath string) bool {
	artisanInfo, err := os.Lstat(filepath.Join(buildPath, "artisan"))
	if err != nil || !artisanInfo.Mode().IsRegular() {
		return false
	}
	composerPath := filepath.Join(buildPath, "composer.json")
	composerInfo, err := os.Lstat(composerPath)
	if err != nil || !composerInfo.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return false
	}
	var composer struct {
		Require map[string]string `json:"require"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return false
	}
	_, ok := composer.Require["laravel/framework"]
	return ok
}

func (w *DeploymentWorker) detectProjectRuntimeFallback(buildPath string) projectRuntimeDetection {
	type marker struct {
		file string
		name string
	}
	markers := []marker{
		{"artisan", "Laravel"},
		{"go.mod", "Go"},
		{"requirements.txt", "Python"},
		{"main.py", "Python"},
		{"Gemfile", "Ruby"},
		{"Cargo.toml", "Rust"},
		{"pom.xml", "Java"},
		{"build.gradle", "Java"},
		{"build.gradle.kts", "Java"},
		{"composer.json", "PHP"},
		{"mix.exs", "Elixir"},
		{"deno.json", "Deno"},
		{"deno.jsonc", "Deno"},
		{"gleam.toml", "Gleam"},
		{"CMakeLists.txt", "C/C++"},
		{"meson.build", "C/C++"},
		{"start.sh", "Shell"},
		{"next.config.js", "Next.js"},
		{"next.config.mjs", "Next.js"},
		{"next.config.ts", "Next.js"},
		{"nuxt.config.js", "Nuxt.js"},
		{"nuxt.config.mjs", "Nuxt.js"},
		{"nuxt.config.ts", "Nuxt.js"},
		{"vite.config.js", "Vite"},
		{"vite.config.mjs", "Vite"},
		{"vite.config.ts", "Vite"},
		{"vite.config.mts", "Vite"},
		{"src/App.tsx", "React"},
		{"src/App.jsx", "React"},
		{"src/App.vue", "Vue"},
		{"src/main.js", "Node.js"},
		{"svelte.config.js", "Svelte"},
		{"angular.json", "Angular"},
		{"package.json", "Node.js"},
		{"tsconfig.json", "TypeScript"},
		{"index.html", "Static"},
	}

	detection := projectRuntimeDetection{Framework: "Other"}
	for _, marker := range markers {
		if info, err := os.Lstat(filepath.Join(buildPath, marker.file)); err == nil && info.Mode().IsRegular() {
			detection.Framework = marker.name
			break
		}
	}
	if !isJSFramework(detection.Framework) {
		return detection
	}
	if detected := detectJSFrameworkFromPackage(filepath.Join(buildPath, "package.json")); detected != "" {
		detection.Framework = detected
	}
	return detection
}

func isJSFramework(framework string) bool {
	switch framework {
	case "Node.js", "Next.js", "Vite", "React", "Vue", "Nuxt.js", "Svelte", "Angular", "TypeScript":
		return true
	default:
		return false
	}
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

func (w *DeploymentWorker) getActiveLogPath(project *models.Project) string {
	projectPath := project.GetProjectPath(w.cfg.ProjectsPath)
	if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
		jobID := *project.DeploymentJobID
		buildPath := filepath.Join(projectPath, "logs", fmt.Sprintf("build-%s.log", jobID))
		if _, err := os.Stat(buildPath); err == nil {
			return buildPath
		}
		infraPath := filepath.Join(projectPath, "logs", fmt.Sprintf("infra-%s.log", jobID))
		if _, err := os.Stat(infraPath); err == nil {
			return infraPath
		}
		return buildPath
	}
	return filepath.Join(projectPath, "build.log")
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
		desc = "Deploying container release onto system..."
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

	if description != "" {
		desc = description
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

	instID := *project.GithubInstallationID
	owner := project.GithubRepoOwner
	repo := project.GithubRepoName
	commitHash := project.LastCommitHash
	projectID := project.ID
	jobID := ""
	if project.DeploymentJobID != nil {
		jobID = *project.DeploymentJobID
	}

	// Save desired status to Redis first to enable background retry/reconciliation
	createdAt := time.Now().UnixNano()
	statusPayload := &infrastructure.GithubStatusPayload{
		InstallationID: instID,
		Owner:          owner,
		Repo:           repo,
		SHA:            commitHash,
		State:          ghState,
		TargetURL:      targetURL,
		Description:    desc,
		CreatedAt:      createdAt,
	}
	if err := w.redisService.SetDesiredCommitStatus(statusPayload); err != nil {
		slog.Warn("Failed to set desired commit status in Redis", "project_id", projectID, "error", err)
	}

	err := w.githubService.UpdateCommitStatus(instID, owner, repo, commitHash, ghState, targetURL, desc)
	if err == nil {
		_, _ = w.redisService.RemoveCommitStatusSyncIfMatched(commitHash, createdAt)
	} else {
		slog.Warn("Failed to update GitHub commit status, queued for reconciler", "project_id", projectID, "error", err)

		logMsg := fmt.Sprintf("[%s] System Warning: Failed to update GitHub commit status to %s: %s", time.Now().Format("2006-01-02 15:04:05"), ghState, err.Error())
		_ = w.redisService.PublishBuildLogForJob(projectID, jobID, logMsg)

		buildLogPath := w.getActiveLogPath(project)
		if f, errOpt := os.OpenFile(buildLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); errOpt == nil {
			_, _ = f.WriteString(logMsg + "\n")
			f.Close()
		}
	}
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
// cleanupRollout clears durable recovery state only after Docker confirms that
// every rollout container is gone. A failed cleanup remains checkpointed for
// watchdog reconciliation.
func (w *DeploymentWorker) cleanupRollout(project *models.Project, containerID string, workerContainerID *string) bool {
	if err := w.dockerService.RemoveContainer(containerID, workerContainerID); err != nil {
		slog.Error("Failed to cleanup rollout container", "projectId", project.ID, "containerId", containerID, "error", err)
		return false
	}
	if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"rollout_container_id":        nil,
		"rollout_worker_container_id": nil,
	}); err != nil {
		slog.Error("Failed to clear rollout checkpoint after cleanup", "projectId", project.ID, "containerId", containerID, "error", err)
		return false
	}
	project.RolloutContainerID = nil
	project.RolloutWorkerContainerID = nil
	return true
}

func (w *DeploymentWorker) updateProjectError(project *models.Project, jobID string, errorMsg string) {
	// Log the raw error for administrator diagnostics
	slog.Error("Deployment failure with raw diagnostic details", "projectId", project.ID, "jobId", jobID, "error", errorMsg)

	// Gather sensitive values dynamically from database config and .env variables
	var sensitiveValues []string
	if project.GetDatabaseName() != "" {
		sensitiveValues = append(sensitiveValues, project.GetDatabaseName())
	}
	if project.DatabasePassword != "" {
		sensitiveValues = append(sensitiveValues, project.DatabasePassword)
	}

	projectEnvPath := filepath.Join(project.GetProjectPath(w.cfg.ProjectsPath), ".env")
	if envVars, err := w.dockerService.ParseProjectEnv(projectEnvPath); err == nil {
		for _, val := range envVars {
			sensitiveValues = append(sensitiveValues, val)
		}
	}

	// Clean up internal infrastructure credentials and paths using centralized utility functions
	redactedErrorMsg := utils.RedactInfrastructureDetails(errorMsg, sensitiveValues)
	sanitizedMsg := utils.SanitizeError(redactedErrorMsg)

	// Get smart suggestion based on centralized utility classifiers
	suggestion := utils.GetSmartSuggestion(errorMsg)
	if suggestion != "" {
		sanitizedMsg = fmt.Sprintf("%s\n\nRunara Recommendation:\n- %s", sanitizedMsg, suggestion)
	}

	if !w.transitionDeploymentState(project, jobID, models.DepStatusFailed, project.DeploymentProgress, "deployment_failed", sanitizedMsg) {
		if err := w.projectRepo.UpdateDeploymentStatus(project.ID, models.DepStatusFailed, sanitizedMsg, project.DeploymentProgress, jobID); err != nil {
			slog.Error("Failed to force deployment failure status", "projectId", project.ID, "jobId", jobID, "error", err)
		}
	}
	msg := sanitizedMsg
	project.ErrorLog = &msg

	// Force the error message into the build log stream so the UI terminal displays it immediately
	terminalErrorMsg := fmt.Sprintf("\n>> [FAILED] DEPLOYMENT ERROR: %s\n", sanitizedMsg)
	_ = w.redisService.PublishBuildLogForJob(project.ID, jobID, terminalErrorMsg)

	// Also append to the physical log file
	buildLogPath := w.getActiveLogPath(project)
	if f, errOpt := os.OpenFile(buildLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); errOpt == nil {
		_, _ = f.WriteString(terminalErrorMsg)
		f.Close()
	}

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
		w.cleanupRollout(project, *project.RolloutContainerID, project.RolloutWorkerContainerID)
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

func (w *DeploymentWorker) instantUpdateEnv(project *models.Project, logFunc func(string)) error {
	if logFunc == nil {
		logFunc = func(string) {}
	}
	projectDomain := w.cfg.ProjectDomain

	logFunc("")
	logFunc(">> Regenerating environment configuration...")
	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		logFunc("✗ Failed to regenerate environment configuration: " + err.Error())
		return err
	}
	logFunc("✓ Environment configuration regenerated successfully.")

	logFunc("")
	logFunc(">> Updating database connection credentials...")
	if err := w.dockerService.UpdateDatabaseCredentialsInEnv(project); err != nil {
		logFunc("✗ Failed to update database connection credentials: " + err.Error())
		slog.Warn("Failed to update database credentials in .env file", "id", project.ID, "error", err)
	} else {
		logFunc("✓ Database connection credentials updated successfully.")
	}

	logFunc("")
	return w.projectService.RecreateProjectZeroDowntime(project, logFunc)
}

func (w *DeploymentWorker) redeployExistingImage(project *models.Project, logFunc func(string)) error {
	if logFunc == nil {
		logFunc = func(string) {}
	}
	projectDomain := w.cfg.ProjectDomain

	logFunc("")
	logFunc(">> Refreshing environment configuration...")
	if err := w.dockerService.CreateEnvFile(project, projectDomain, false); err != nil {
		logFunc("✗ Failed to refresh environment configuration: " + err.Error())
		slog.Warn("Failed to refresh environment file during redeploy", "id", project.ID, "error", err)
	} else {
		logFunc("✓ Environment configuration refreshed successfully.")
	}

	logFunc("")
	logFunc(">> Updating database connection credentials...")
	if err := w.dockerService.UpdateDatabaseCredentialsInEnv(project); err != nil {
		logFunc("✗ Failed to update database connection credentials: " + err.Error())
		slog.Warn("Failed to update database credentials in .env file", "id", project.ID, "error", err)
	} else {
		logFunc("✓ Database connection credentials updated successfully.")
	}

	logFunc("")
	return w.projectService.RecreateProjectZeroDowntime(project, logFunc)
}

// getSecretsToRedact compiles a list of decrypted secrets linked to the project.
// Any secret value returned here (if length > 4) will be redacted in the build logs.
func (w *DeploymentWorker) getSecretsToRedact(project *models.Project) []string {
	var secrets []string

	// 1. Add DB password
	if len(project.DatabasePassword) > 4 {
		secrets = append(secrets, project.DatabasePassword)
	}

	// 2. Fetch compile map
	envMap, err := w.dockerService.CompileEnvForProject(project.ID, project.UserID, project.Subdomain, project.GetDatabaseName(), project.DatabasePassword, project.Framework, "production")
	if err == nil {
		// Explicitly add DB_PASSWORD and DATABASE_URL
		if val, ok := envMap["DB_PASSWORD"]; ok && len(val) > 4 {
			secrets = append(secrets, val)
		}
		if val, ok := envMap["DATABASE_URL"]; ok && len(val) > 4 {
			secrets = append(secrets, val)
		}
	}

	// 3. Query all SecretStoreItemValue linked to the project's active bindings
	db := w.dockerService.GetDB()
	if db != nil {
		var bindings []models.SecretStoreBinding
		if err := db.Where("project_id = ?", project.ID).Find(&bindings).Error; err == nil {
			decryptionKeys := utils.CredentialDecryptionKeys(w.cfg.CredentialEncryptionKey, w.cfg.CredentialEncryptionPreviousKeys)
			for _, b := range bindings {
				var items []models.SecretStoreItem
				if err := db.Where("secret_store_id = ?", b.SecretStoreID).Preload("Values").Find(&items).Error; err == nil {
					for _, item := range items {
						var latestVal *models.SecretStoreItemValue
						for i := range item.Values {
							val := &item.Values[i]
							if val.Version == item.LatestSnapshotVersion {
								latestVal = val
								break
							}
						}
						if latestVal != nil {
							decrypted, err := utils.Decrypt(latestVal.EncryptedValue, decryptionKeys...)
							if err == nil && len(decrypted) > 4 {
								secrets = append(secrets, decrypted)
							}
						}
					}
				}
			}
		}
	}

	// De-duplicate secrets and remove empty/short strings, ignoring common non-sensitive environment values
	nonSensitiveBlocklist := map[string]bool{
		"production":  true,
		"development": true,
		"staging":     true,
		"testing":     true,
		"local":       true,
		"true":        true,
		"false":       true,
	}

	uniqueSecrets := make(map[string]bool)
	var filtered []string
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if len(s) > 4 && !uniqueSecrets[s] && !nonSensitiveBlocklist[strings.ToLower(s)] {
			uniqueSecrets[s] = true
			filtered = append(filtered, s)
		}
	}

	return filtered
}

// makeRedactingLogger wraps a log callback function to redact sensitive values
func (w *DeploymentWorker) makeRedactingLogger(project *models.Project, rawLogger func(string)) func(string) {
	if rawLogger == nil {
		return func(string) {}
	}
	secrets := w.getSecretsToRedact(project)
	if len(secrets) == 0 {
		return rawLogger
	}

	return func(msg string) {
		redacted := msg
		for _, secret := range secrets {
			redacted = strings.ReplaceAll(redacted, secret, "[REDACTED_SECRET]")
		}
		rawLogger(redacted)
	}
}
