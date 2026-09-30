// ===========================================
// Central Watchdog Service
// ===========================================
// Background monitoring daemon running in the backend API.
// Handles startup recovery, stale deployment checks,
// and scheduled garbage collection.
// ===========================================
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/setting"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
)

// Staleness thresholds used by StartStaleBuildWatchdog. A deployment holding a
// Redis lease is governed by leaseHeartbeatStaleAfter; only deployments with no
// lock metadata at all fall back to the much longer dbHeartbeatStaleAfter.
const (
	// leaseHeartbeatStaleAfter bounds the gap between lease heartbeat renewals.
	// This is the threshold that governs in practice, because every running
	// deployment holds a lock.
	leaseHeartbeatStaleAfter = 3 * time.Minute
	// dbHeartbeatStaleAfter applies only when Redis has no lock metadata for the
	// project, so the database heartbeat is the sole liveness signal.
	dbHeartbeatStaleAfter = 15 * time.Minute
)

// CentralWatchdog oversees system consistency and maintenance tasks
type CentralWatchdog struct {
	cfg            *config.Config
	projectRepo    repositories.ProjectRepository
	redisService   *infrastructure.RedisService
	dockerService  *docker.DockerService
	projectService *projectServicePkg.ProjectService
	settingService *setting.SettingService
	githubService  *infrastructure.GithubService
	running        bool
	stopChan       chan struct{}
}

// NewCentralWatchdog creates a new CentralWatchdog instance
func NewCentralWatchdog(
	cfg *config.Config,
	projectRepo repositories.ProjectRepository,
	redisService *infrastructure.RedisService,
	dockerService *docker.DockerService,
	projectService *projectServicePkg.ProjectService,
	settingService *setting.SettingService,
	githubService *infrastructure.GithubService,
) *CentralWatchdog {
	return &CentralWatchdog{
		cfg:            cfg,
		projectRepo:    projectRepo,
		redisService:   redisService,
		dockerService:  dockerService,
		projectService: projectService,
		settingService: settingService,
		githubService:  githubService,
		running:        false,
		stopChan:       make(chan struct{}),
	}
}

// Start initiates the supervisory routines
func (w *CentralWatchdog) Start() {
	if w.running {
		return
	}
	w.running = true
	slog.Info("Central watchdog: starting background supervision system")

	w.recoverOrphanedDeletions()
	w.recoverOrphanedBuilds()

	w.StartPruneScheduler()
	w.StartStaleBuildWatchdog()
	w.StartDelayedJobScheduler()
	w.StartAutoHealingWatchdog()
	w.StartGitHubStatusReconciler()
}

// Stop cleanly terminates all supervisory routines
func (w *CentralWatchdog) Stop() {
	if !w.running {
		return
	}
	w.running = false
	close(w.stopChan)
}

func (w *CentralWatchdog) recoverOrphanedBuilds() {
	statuses := []models.DeploymentStatus{
		models.DepStatusQueued,
		models.DepStatusPreparing,
		models.DepStatusCloning,
		models.DepStatusBuilding,
		models.DepStatusStarting,
		models.DepStatusHealthchecking,
		models.DepStatusMigrating,
		models.DepStatusPromoting,
	}

	projects, err := w.projectRepo.ListByDeploymentStatuses(statuses)
	if err != nil {
		slog.Error("Central watchdog: failed to query orphaned projects for recovery", "error", err)
		return
	}

	if len(projects) == 0 {
		return
	}

	slog.Info("Central watchdog: recovering orphaned projects from previous session", "count", len(projects))

	for i := range projects {
		w.recoverOrphanedBuild(projects[i])
	}
}

func (w *CentralWatchdog) recoverOrphanedBuild(project models.Project) {
	token, err := w.redisService.ReserveOrphanRecovery(project.ID)
	if err != nil {
		slog.Warn("Central watchdog: failed to reserve orphan recovery", "id", project.ID, "error", err)
		return
	}
	if token == "" {
		return
	}
	defer func() {
		if err := w.redisService.ReleaseDeploymentLock(project.ID, token); err != nil {
			slog.Error("Central watchdog: failed to release recovery lock", "id", project.ID, "error", err)
		}
	}()

	current, err := w.projectRepo.GetByID(project.ID)
	if err != nil {
		slog.Error("Central watchdog: failed to reload project during recovery", "id", project.ID, "error", err)
		return
	}
	if current.DeploymentStatus != project.DeploymentStatus || !sameDeploymentJob(current.DeploymentJobID, project.DeploymentJobID) {
		return
	}

	job := &infrastructure.DeploymentJob{
		ProjectID: project.ID, UserID: project.UserID, Type: "redeploy",
		JobID: utils.GenerateRandomUID(), EnqueuedAt: time.Now(),
	}
	_, applied, transitionErr := w.projectService.TransitionStateIfMatch(
		context.Background(),
		project.ID,
		project.DeploymentStatus,
		project.DeploymentJobID,
		job.JobID,
		models.DepStatusQueued,
		0,
		"watchdog_recovery",
		"Recovered from unexpected shutdown (re-queued).",
	)
	if transitionErr != nil {
		slog.Error("Central watchdog: failed to transition project deployment state during recovery", "id", project.ID, "error", transitionErr)
		return
	}
	if !applied {
		slog.Info("Central watchdog: recovery aborted: project state modified by concurrent operation", "id", project.ID)
		return
	}

	if err := w.redisService.EnqueueReplacingDeploymentJob(job, token); err != nil {
		if infrastructure.IsStaleOwnerError(err) {
			slog.Info("Central watchdog: recovery superseded by newer operation", "id", project.ID, "job_id", job.JobID)
			return
		}
		slog.Error("Central watchdog: failed to re-queue project during recovery", "id", project.ID, "error", err)

		// Uncertain publish reconciliation:
		// An EVAL command may commit in Redis and successfully enqueue the job, but network disruption
		// or timeout can cause the client to receive an error. Before asserting terminal failure,
		// inspect whether the predetermined job was actually enqueued or already claimed by a worker.
		inQueue, checkErr := w.redisService.HasDeploymentJob(job.JobID)
		if checkErr != nil {
			// If Redis cannot be inspected, do NOT destroy state by asserting failure.
			// Preserve the recoverable queued state so future watchdog cycles or operators can reconcile.
			slog.Warn("Central watchdog: cannot verify recovery enqueue status after error; preserving queued state", "id", project.ID, "job_id", job.JobID, "check_error", checkErr)
			return
		}

		if inQueue {
			// The job exists in Redis (ready, delayed, or processing). The publish succeeded server-side.
			slog.Info("Central watchdog: recovery job was enqueued despite transport error", "id", project.ID, "job_id", job.JobID)
			return
		}

		// Check if a worker already claimed the job and progressed it past queued in the database.
		if current, getErr := w.projectRepo.GetByID(project.ID); getErr == nil && current != nil {
			if current.DeploymentJobID != nil && *current.DeploymentJobID == job.JobID && current.DeploymentStatus != models.DepStatusQueued {
				slog.Info("Central watchdog: recovery job was claimed and progressed by worker despite transport error", "id", project.ID, "job_id", job.JobID, "status", current.DeploymentStatus)
				return
			}
			if current.DeploymentJobID != nil && *current.DeploymentJobID != job.JobID {
				slog.Info("Central watchdog: recovery superseded by newer deployment job", "id", project.ID, "job_id", job.JobID, "current_job_id", *current.DeploymentJobID)
				return
			}
		}

		// Confirmed genuine failure: job is NOT in Redis and NOT claimed by any worker.
		// Failure compensation must be conditional on this job still owning the queued
		// database row inside the transaction, avoiding overwriting a newer concurrent job.
		applied, transitionErr := w.projectService.FailQueuedDeploymentIfUnchanged(context.Background(), project.ID, job.JobID, "Recovery queue unavailable", false)
		if transitionErr != nil {
			slog.Error("Central watchdog: failed to record recovery queue failure", "id", project.ID, "job_id", job.JobID, "error", transitionErr)
		} else if !applied {
			slog.Warn("Central watchdog: recovery failure compensation skipped: project state owned by newer job or no longer queued", "id", project.ID, "job_id", job.JobID)
		}
		return
	}
	slog.Info("Central watchdog: project automatically re-queued for reliability", "id", project.ID, "job_id", job.JobID)
}

func sameDeploymentJob(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (w *CentralWatchdog) recoverOrphanedDeletions() {
	deletingProjects, err := w.projectRepo.ListByStatus(models.StatusDeleting)
	if err != nil {
		slog.Error("Central watchdog: failed to query orphaned deletions", "error", err)
		return
	}

	if len(deletingProjects) > 0 {
		slog.Info("Central watchdog: recovering orphaned deletions from previous session", "count", len(deletingProjects))
		for i := range deletingProjects {
			project := deletingProjects[i]
			slog.Info("Central watchdog: re-triggering project deletion", "id", project.ID)
			go func(p models.Project) {
				if err := w.projectService.DeleteProject(&p); err != nil {
					slog.Error("Central watchdog: failed to background delete orphaned project", "id", p.ID, "error", err)
				}
			}(project)
		}
	}
}

// StartStaleBuildWatchdog launches a supervisory loop that detects and cleans up stalled or orphaned deployments.
func (w *CentralWatchdog) StartStaleBuildWatchdog() {
	go func() {
		time.Sleep(2 * time.Minute)

		for w.running {
			w.checkStaleBuilds()
			time.Sleep(1 * time.Minute)
		}
	}()
}

func (w *CentralWatchdog) checkStaleBuilds() {
	inProgressStatuses := []models.DeploymentStatus{
		models.DepStatusQueued,
		models.DepStatusPreparing,
		models.DepStatusCloning,
		models.DepStatusBuilding,
		models.DepStatusStarting,
		models.DepStatusHealthchecking,
		models.DepStatusMigrating,
		models.DepStatusPromoting,
	}
	projects, err := w.projectRepo.ListByDeploymentStatuses(inProgressStatuses)
	if err != nil {
		slog.Error("Central watchdog: failed to query in-progress projects", "error", err)
		return
	}

			for i := range projects {
				project := projects[i]
				jobID := ""
				if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
					jobID = *project.DeploymentJobID
				}
				if jobID == "" {
					jobID = "unknown"
				}
				var reason string

				// If the project is still in queued state, verify if the job is active in Redis (ready, delayed, or processing).
				// This prevents reaping jobs waiting in queue or claimed by a worker and waiting for a concurrency slot.
				if project.DeploymentStatus == models.DepStatusQueued {
					if jobID != "" && jobID != "unknown" {
						hasJob, err := w.redisService.HasDeploymentJob(jobID)
						if err == nil && hasJob {
							continue
						}
					}
					queued, queueErr := w.redisService.IsProjectQueued(project.ID)
					if queueErr != nil {
						slog.Warn("Central watchdog: failed to inspect deployment queue", "projectId", project.ID, "error", queueErr)
						continue
					}
					if queued {
						continue
					}
				}

				lockMeta, err := w.redisService.GetLockMetadata(project.ID)
				if err != nil {
					slog.Warn("Central watchdog: failed to fetch lock metadata from Redis", "projectId", project.ID, "error", err)
					continue
				}
				if lockMeta != nil {
					// Admission or recovery reservations have empty DeploymentID or reservation worker IDs;
					// they are actively preparing to queue and must not be treated as crashed workers.
					if lockMeta.DeploymentID == "" || lockMeta.WorkerID == "admin_requeue" || lockMeta.WorkerID == "redeploy" || lockMeta.WorkerID == "watchdog" {
						continue
					}
					jobID = lockMeta.DeploymentID
					leaseMeta, _ := w.redisService.GetDeploymentLease(jobID)
					if leaseMeta == nil {
						reason = "deployment job lease missing (worker terminated abruptly)"
					} else {
						lastHeartbeat, err := time.Parse(time.RFC3339, leaseMeta.LastHeartbeat)
						if err == nil && time.Since(lastHeartbeat) > leaseHeartbeatStaleAfter {
							reason = fmt.Sprintf("deployment job lease heartbeat expired (last heartbeat %s)", time.Since(lastHeartbeat).Round(time.Second))
						}
					}
				} else if project.DeploymentHeartbeatAt != nil && time.Since(*project.DeploymentHeartbeatAt) > dbHeartbeatStaleAfter {
					reason = fmt.Sprintf("stale deployment timeout (heartbeat %s ago)", time.Since(*project.DeploymentHeartbeatAt).Round(time.Second))
				} else if project.DeploymentHeartbeatAt == nil && time.Since(project.UpdatedAt) > dbHeartbeatStaleAfter {
					reason = "stale deployment timeout (no active lock or lease)"
				}

				if reason != "" && project.DeploymentStatus == models.DepStatusQueued {
					// A worker may have popped the job between the scan and now.
					// Re-read before failing so a deployment that just started is
					// never killed by this loop.
					if fresh, err := w.projectRepo.GetByID(project.ID); err != nil || fresh.DeploymentStatus != models.DepStatusQueued {
						continue
					}
				}

				if reason != "" {
					slog.Warn("Central watchdog: detected orphaned deployment",
						"projectId", project.ID,
						"subdomain", project.Subdomain,
						"jobId", jobID,
						"reason", reason)

					errorMsg := fmt.Sprintf("Deployment failed: %s.", reason)
					sanitizedMsg := utils.SanitizeError(errorMsg)

					// Use a timeout context instead of context.Background() to prevent indefinite hangs during DB failure
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					_, err := w.projectService.TransitionDeploymentState(ctx, project.ID, jobID, models.DepStatusFailed, project.DeploymentProgress, "orphan_recovered", sanitizedMsg)
					cancel()
					if err != nil {
						slog.Error("Central watchdog: failed atomic state transition for failed project deployment", "id", project.ID, "error", err)
						continue
					}
					w.updateGitHubCommitStatus(&project, models.DepStatusFailed, sanitizedMsg)

					// Force the timeout message into the build log stream so the UI terminal displays it immediately
					terminalErrorMsg := fmt.Sprintf("\n>> [TIMEOUT] DEPLOYMENT STALLED: %s\n", sanitizedMsg)
					_ = w.redisService.PublishBuildLogForJob(project.ID, jobID, terminalErrorMsg)

					// Also append to the physical log file so it persists on page refresh
					// Sanitize jobID to prevent path traversal risks
					safeJobID := filepath.Base(filepath.Clean(jobID))
					buildLogPath := w.getActiveLogPath(&project, safeJobID)

					// Ensure the logs directory exists; if the original worker crashed before creating it, os.OpenFile will fail
					if err := os.MkdirAll(filepath.Dir(buildLogPath), 0755); err != nil {
						slog.Error("Central watchdog: failed to create logs directory", "projectId", project.ID, "error", err)
					} else {
						if f, err := os.OpenFile(buildLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
							_, _ = f.WriteString(terminalErrorMsg)
							f.Close()
						} else {
							slog.Error("Central watchdog: failed to open log file", "projectId", project.ID, "path", buildLogPath, "error", err)
						}
					}

					if lockMeta != nil {
						if err := w.redisService.ForceReleaseDeploymentLock(project.ID, fmt.Sprintf("Watchdog cleaning up stale deployment: %s", reason)); err != nil {
							slog.Warn("Central watchdog: failed to force release lock", "id", project.ID, "error", err)
						}
					}

					if project.RolloutContainerID != nil && *project.RolloutContainerID != "" {
						slog.Info("Central watchdog: removing orphaned rollout container instance", "containerId", *project.RolloutContainerID)
						if err := w.dockerService.RemoveContainer(*project.RolloutContainerID, project.RolloutWorkerContainerID); err != nil {
							slog.Error("Central watchdog: failed to cleanup orphaned rollout", "projectId", project.ID, "error", err)
						} else if err := w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
							"rollout_container_id":        nil,
							"rollout_worker_container_id": nil,
						}); err != nil {
							slog.Error("Central watchdog: failed to clear rollout checkpoint", "projectId", project.ID, "error", err)
						}
					}
				}
			}
		}

func (w *CentralWatchdog) updateGitHubCommitStatus(project *models.Project, state models.DeploymentStatus, description string) {
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

	slog.Info("Watchdog: updating GitHub commit status", "project_id", project.ID, "sha", project.LastCommitHash, "state", ghState, "desc", desc)

	instID := *project.GithubInstallationID
	owner := project.GithubRepoOwner
	repo := project.GithubRepoName
	commitHash := project.LastCommitHash
	projectID := project.ID

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
		slog.Warn("Watchdog: failed to set desired commit status in Redis", "project_id", projectID, "error", err)
	}

	err := w.githubService.UpdateCommitStatus(instID, owner, repo, commitHash, ghState, targetURL, desc)
	if err == nil {
		_, _ = w.redisService.RemoveCommitStatusSyncIfMatched(commitHash, createdAt)
	} else {
		slog.Warn("Watchdog: failed to update GitHub commit status, queued for reconciler", "project_id", projectID, "error", err)

		logMsg := fmt.Sprintf("[%s] System Warning (Watchdog): Failed to update GitHub commit status to %s: %s", time.Now().Format("2006-01-02 15:04:05"), ghState, err.Error())
		jobID := ""
		if project.DeploymentJobID != nil {
			jobID = *project.DeploymentJobID
		}
		_ = w.redisService.PublishBuildLogForJob(projectID, jobID, logMsg)
	}
}

// StartDelayedJobScheduler launches a background daemon that migrates ready delayed jobs into the active deployment queue.
func (w *CentralWatchdog) StartDelayedJobScheduler() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for w.running {
			select {
			case <-ticker.C:
				count, err := w.redisService.MigrateDelayedJobs()
				if err != nil {
					slog.Warn("Central watchdog: failed to migrate delayed jobs", "error", err)
				} else if count > 0 {
					slog.Info("Central watchdog: migrated ready delayed deployment jobs to active queue", "count", count)
				}
			case <-w.stopChan:
				return
			}
		}
	}()
}

func (w *CentralWatchdog) StartPruneScheduler() {
	go func() {
		for w.running {
			now := time.Now()
			next3AM := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
			if now.After(next3AM) {
				next3AM = next3AM.Add(24 * time.Hour)
			}

			durationToWait := next3AM.Sub(now)
			slog.Info("Central watchdog: scheduled next Docker cache prune", "time", next3AM.Format("15:04:05"))

			select {
			case <-w.stopChan:
				return
			case <-time.After(durationToWait):
			}

			slog.Info("Central watchdog: executing 3 AM scheduled Docker cache prune")
			if err := w.dockerService.PruneImages(); err != nil {
				slog.Error("Central watchdog: scheduled image prune failed", "error", err)
			}

			if err := exec.Command("docker", "builder", "prune", "-f", "--filter", "until=48h").Run(); err != nil {
				slog.Warn("Central watchdog: failed to prune docker builder", "error", err)
			}
		}
	}()
}

// StartAutoHealingWatchdog launches the container health auto-healing checker loop.
func (w *CentralWatchdog) StartAutoHealingWatchdog() {
	go func() {
		time.Sleep(45 * time.Second)
		for w.running {
			w.autoHealingCheck()
			select {
			case <-w.stopChan:
				return
			case <-time.After(1 * time.Minute):
			}
		}
	}()
}

func (w *CentralWatchdog) inspectContainerState(containerID string) (running bool, oomKilled bool, err error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}::{{.State.OOMKilled}}", containerID)
	out, err := cmd.Output()
	if err != nil {
		return false, false, err
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "::")
	if len(parts) != 2 {
		return false, false, fmt.Errorf("unexpected inspect output: %s", string(out))
	}
	running = parts[0] == "true"
	oomKilled = parts[1] == "true"
	return running, oomKilled, nil
}

// recordHealthNotice stores a runtime health notice for a running project.
// These are not deployment failures, so they never touch error_log.
func (w *CentralWatchdog) recordHealthNotice(projectID uint, notice string) {
	now := time.Now()
	if err := w.projectRepo.UpdateMetadata(projectID, map[string]interface{}{
		"health_notice":    notice,
		"health_notice_at": &now,
	}); err != nil {
		slog.Warn("Central watchdog: failed to persist health notice", "projectId", projectID, "error", err)
	}
}

func (w *CentralWatchdog) recordAutoHealingEvent(projectID uint, eventType string, payload string, deploymentJobID *string) {
	jobID := "system"
	if deploymentJobID != nil && *deploymentJobID != "" {
		jobID = *deploymentJobID
	}
	event := &models.DeploymentEvent{
		ProjectID: projectID,
		JobID:     jobID,
		WorkerID:  "watchdog",
		StateFrom: string(models.StatusRunning),
		StateTo:   string(models.StatusRunning),
		EventType: eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
	if err := w.projectRepo.RecordDeploymentEvent(event); err != nil {
		slog.Error("Central watchdog: failed to record auto-healing event", "projectId", projectID, "error", err)
	}
	if eventJSON, err := json.Marshal(event); err == nil {
		_ = w.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
	}
}

func (w *CentralWatchdog) getActiveLogPath(project *models.Project, jobID string) string {
	projectsPath := ""
	if w.cfg != nil {
		projectsPath = w.cfg.ProjectsPath
	}
	projectPath := project.GetProjectPath(projectsPath)
	if jobID != "" && jobID != "unknown" {
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

func (w *CentralWatchdog) autoHealingCheck() {
	runningProjects, err := w.projectRepo.GetRunningWithContainers()
	if err != nil {
		slog.Error("Central watchdog: failed to fetch running projects for auto-healing check", "error", err)
		return
	}

	for i := range runningProjects {
		project := runningProjects[i]
		if !w.billingAllowsAutoHealing(project.ID) {
			continue
		}

		// Check Web Container
		if project.ContainerID != nil && *project.ContainerID != "" {
			running, oomKilled, err := w.inspectContainerState(*project.ContainerID)
			if err == nil && !running {
				if oomKilled {
					slog.Warn("Central watchdog: container OOM killed", "projectId", project.ID, "containerId", *project.ContainerID)
					w.recordAutoHealingEvent(project.ID, "oom_killed", "Container terminated: Out of Memory (OOM Killed)", project.DeploymentJobID)
					w.recordHealthNotice(project.ID, "Your application was terminated by the system because it exceeded its RAM limit. Please optimize your memory consumption or contact the administrator.")
				} else {
					slog.Warn("Central watchdog: container is not running", "projectId", project.ID, "containerId", *project.ContainerID)
					w.recordAutoHealingEvent(project.ID, "container_crashed", "Container is not running", project.DeploymentJobID)
				}

				slog.Info("Central watchdog: auto-healing: restarting web container", "projectId", project.ID)
				if err := w.dockerService.StartContainer(*project.ContainerID); err != nil {
					slog.Error("Central watchdog: auto-healing: failed to restart web container", "projectId", project.ID, "error", err)
				} else {
					w.recordAutoHealingEvent(project.ID, "auto_healing_restart", "Auto-healing: Restarted container", project.DeploymentJobID)
					w.compensateAutoHealingBillingRace(project.ID, *project.ContainerID)
				}
			}
		}

		// Check Worker Container
		if project.WorkerContainerID != nil && *project.WorkerContainerID != "" {
			running, oomKilled, err := w.inspectContainerState(*project.WorkerContainerID)
			if err == nil && !running {
				if oomKilled {
					slog.Warn("Central watchdog: worker container OOM killed", "projectId", project.ID, "containerId", *project.WorkerContainerID)
					w.recordAutoHealingEvent(project.ID, "worker_oom_killed", "Worker container terminated: Out of Memory (OOM Killed)", project.DeploymentJobID)
					w.recordHealthNotice(project.ID, "Your background worker was terminated by the system because it exceeded its RAM limit.")
				} else {
					slog.Warn("Central watchdog: worker container is not running", "projectId", project.ID, "containerId", *project.WorkerContainerID)
					w.recordAutoHealingEvent(project.ID, "worker_container_crashed", "Worker container is not running", project.DeploymentJobID)
				}

				slog.Info("Central watchdog: auto-healing: restarting worker container", "projectId", project.ID)
				if err := w.dockerService.StartContainer(*project.WorkerContainerID); err != nil {
					slog.Error("Central watchdog: auto-healing: failed to restart worker container", "projectId", project.ID, "error", err)
				} else {
					w.recordAutoHealingEvent(project.ID, "auto_healing_restart", "Auto-healing: Restarted worker container", project.DeploymentJobID)
					w.compensateAutoHealingBillingRace(project.ID, *project.WorkerContainerID)
				}
			}
		}
	}
}

func (w *CentralWatchdog) billingAllowsAutoHealing(projectID uint) bool {
	if w == nil || w.cfg == nil || !w.cfg.BillingEnabled {
		return true
	}
	var resource models.BillableResource
	if err := w.projectRepo.DB().Where("type = ? AND resource_id = ?", models.BillableTypeProject, projectID).First(&resource).Error; err != nil {
		slog.Error("Central watchdog: billing state unavailable; skipping auto-healing", "project_id", projectID, "error", err)
		return false
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		slog.Info("Central watchdog: skipping auto-healing for non-active billing resource", "project_id", projectID, "billing_status", resource.BillingStatus)
		return false
	}
	return true
}

func (w *CentralWatchdog) compensateAutoHealingBillingRace(projectID uint, containerID string) {
	if w.billingAllowsAutoHealing(projectID) {
		return
	}
	if err := w.dockerService.StopContainer(containerID); err != nil {
		slog.Error("Central watchdog: failed to stop container after billing suspension race", "project_id", projectID, "container_id", containerID, "error", err)
		return
	}
	w.recordAutoHealingEvent(projectID, "auto_healing_billing_reverted", "Auto-healing restart reverted because billing suspension became active", nil)
}

// StartGitHubStatusReconciler runs a background loop that periodically synchronizes any failed or pending GitHub commit statuses.
func (w *CentralWatchdog) StartGitHubStatusReconciler() {
	go func() {
		// Wait 10 seconds before starting
		time.Sleep(10 * time.Second)

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for w.running {
			select {
			case <-ticker.C:
				shas, err := w.redisService.GetPendingCommitStatusSHAs()
				if err != nil {
					continue
				}

				for _, sha := range shas {
					payload, err := w.redisService.GetDesiredCommitStatus(sha)
					if err != nil {
						// Desired status data missing, clean up sync set to prevent stuck entries
						_ = w.redisService.RemoveCommitStatusSync(sha)
						continue
					}

					slog.Info("Central watchdog: retrying failed GitHub commit status update", "sha", sha, "state", payload.State)
					err = w.githubService.UpdateCommitStatus(payload.InstallationID, payload.Owner, payload.Repo, payload.SHA, payload.State, payload.TargetURL, payload.Description)
					if err == nil {
						slog.Info("Central watchdog: successfully synchronized GitHub commit status", "sha", sha, "state", payload.State)
						_, _ = w.redisService.RemoveCommitStatusSyncIfMatched(sha, payload.CreatedAt)
					} else {
						slog.Warn("Central watchdog: failed to reconcile GitHub commit status, will retry", "sha", sha, "error", err)
					}
				}
			case <-w.stopChan:
				return
			}
		}
	}()
}
