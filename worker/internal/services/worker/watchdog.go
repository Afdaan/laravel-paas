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
	w.StartExpiryJanitor()
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
		models.DepStatusProvisioning,
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
		project := projects[i]

		isQueued, _ := w.redisService.IsProjectQueued(project.ID)
		if isQueued {
			slog.Info("Central watchdog: project is already in queue, skipping recovery", "id", project.ID)
			continue
		}

		recoveryLog := "Recovered from unexpected shutdown (re-queued)."
		jobID := ""
		if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
			jobID = *project.DeploymentJobID
		}
		if _, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, jobID, models.DepStatusQueued, 0, "watchdog_recovery", recoveryLog); err != nil {
			slog.Error("Central watchdog: failed to transition project deployment state during recovery", "id", project.ID, "error", err)
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"error_log": recoveryLog,
		})

		if err := w.redisService.ForceReleaseDeploymentLock(project.ID, "Watchdog recovering orphaned build from unexpected shutdown"); err != nil {
			slog.Warn("Central watchdog: failed to force release lock during recovery", "id", project.ID, "error", err)
		}

		if jobID, err := w.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
			slog.Error("Central watchdog: failed to re-queue project during recovery", "id", project.ID, "error", err)
		} else {
			slog.Info("Central watchdog: project automatically re-queued for reliability", "id", project.ID)
			_ = w.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Watchdog recovery re-queue", 0, jobID)
		}
	}
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
			staleThreshold := 15 * time.Minute

			inProgressStatuses := []models.DeploymentStatus{
				models.DepStatusPreparing,
				models.DepStatusCloning,
				models.DepStatusBuilding,
				models.DepStatusProvisioning,
				models.DepStatusStarting,
				models.DepStatusHealthchecking,
				models.DepStatusMigrating,
				models.DepStatusPromoting,
			}
			projects, err := w.projectRepo.ListByDeploymentStatuses(inProgressStatuses)
			if err != nil {
				slog.Error("Central watchdog: failed to query in-progress projects", "error", err)
				time.Sleep(5 * time.Minute)
				continue
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

				lockMeta, err := w.redisService.GetLockMetadata(project.ID)
				if err != nil {
					slog.Warn("Central watchdog: failed to fetch lock metadata from Redis", "projectId", project.ID, "error", err)
					continue
				}
				if lockMeta != nil {
					jobID = lockMeta.DeploymentID
					leaseMeta, _ := w.redisService.GetDeploymentLease(jobID)
					if leaseMeta == nil {
						reason = "deployment job lease missing (worker terminated abruptly)"
					} else {
						lastHeartbeat, err := time.Parse(time.RFC3339, leaseMeta.LastHeartbeat)
						if err == nil && time.Since(lastHeartbeat) > 3*time.Minute {
							reason = fmt.Sprintf("deployment job lease heartbeat expired (last heartbeat %s)", time.Since(lastHeartbeat).Round(time.Second))
						}
					}
				} else if project.DeploymentHeartbeatAt != nil && time.Since(*project.DeploymentHeartbeatAt) > staleThreshold {
					reason = fmt.Sprintf("stale deployment timeout (heartbeat %s ago)", time.Since(*project.DeploymentHeartbeatAt).Round(time.Second))
				} else if project.DeploymentHeartbeatAt == nil && time.Since(project.UpdatedAt) > staleThreshold {
					reason = "stale deployment timeout (no active lock or lease)"
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
					if _, err := w.projectService.TransitionDeploymentState(ctx, project.ID, jobID, models.DepStatusFailed, project.DeploymentProgress, "orphan_recovered", sanitizedMsg); err != nil {
						slog.Error("Central watchdog: failed atomic state transition for failed project deployment", "id", project.ID, "error", err)
					}
					cancel()
					_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
						"error_log": sanitizedMsg,
					})
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

			time.Sleep(1 * time.Minute)
		}
	}()
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

func (w *CentralWatchdog) StartExpiryJanitor() {
	go func() {
		time.Sleep(1 * time.Minute)

		for w.running {
			slog.Info("Central watchdog: running project expiry janitor")
			w.cleanupExpiredProjects()

			select {
			case <-w.stopChan:
				return
			case <-time.After(1 * time.Hour):
			}
		}
	}()
}

func (w *CentralWatchdog) cleanupExpiredProjects() {
	expiredProjects, err := w.projectRepo.ListExpired()
	if err != nil {
		slog.Error("Central watchdog: failed to query expired projects", "error", err)
		return
	}

	if len(expiredProjects) == 0 {
		return
	}

	slog.Info("Central watchdog: auto-deleting expired projects", "count", len(expiredProjects))

	for i := range expiredProjects {
		project := expiredProjects[i]
		slog.Info("Central watchdog: auto-deleting expired project via service", "name", project.Name, "id", project.ID)

		if err := w.projectService.DeleteProject(&project); err != nil {
			slog.Error("Central watchdog: failed to auto-delete expired project", "id", project.ID, "error", err)
		}
	}

	go func() {
		if err := w.dockerService.PruneImages(); err != nil {
			slog.Error("Central watchdog: background image prune failed", "error", err)
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
	projectPath := project.GetProjectPath(w.cfg.ProjectsPath)
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
					errorMsg := "Your application was terminated by the system because it exceeded its RAM limit. Please optimize your memory consumption or contact the administrator."
					_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
						"error_log": errorMsg,
					})
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
					errorMsg := "Your background worker was terminated by the system because it exceeded its RAM limit."
					_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
						"error_log": errorMsg,
					})
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
