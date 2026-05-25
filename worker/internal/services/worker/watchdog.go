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
	"os/exec"
	"strings"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	setting "github.com/laravel-paas/shared/services/setting"
)

// CentralWatchdog oversees system consistency and maintenance tasks
type CentralWatchdog struct {
	projectRepo    repositories.ProjectRepository
	redisService   *infrastructure.RedisService
	dockerService  *docker.DockerService
	projectService *projectServicePkg.ProjectService
	settingService *setting.SettingService
	running        bool
	stopChan       chan struct{}
}

// NewCentralWatchdog creates a new CentralWatchdog instance
func NewCentralWatchdog(
	projectRepo repositories.ProjectRepository,
	redisService *infrastructure.RedisService,
	dockerService *docker.DockerService,
	projectService *projectServicePkg.ProjectService,
	settingService *setting.SettingService,
) *CentralWatchdog {
	return &CentralWatchdog{
		projectRepo:    projectRepo,
		redisService:   redisService,
		dockerService:  dockerService,
		projectService: projectService,
		settingService: settingService,
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
	w.StartScaleToZeroJanitor()
	w.StartAutoHealingWatchdog()
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

				lockMeta, _ := w.redisService.GetLockMetadata(project.ID)
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
					if _, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, jobID, models.DepStatusFailed, project.DeploymentProgress, "orphan_recovered", errorMsg); err != nil {
						slog.Error("Central watchdog: failed atomic state transition for failed project deployment", "id", project.ID, "error", err)
					}
					_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
						"error_log": errorMsg,
					})

					if lockMeta != nil {
						if err := w.redisService.ForceReleaseDeploymentLock(project.ID, fmt.Sprintf("Watchdog cleaning up stale deployment: %s", reason)); err != nil {
							slog.Warn("Central watchdog: failed to force release lock", "id", project.ID, "error", err)
						}
					}

					if project.RolloutContainerID != nil && *project.RolloutContainerID != "" {
						slog.Info("Central watchdog: removing orphaned rollout container instance", "containerId", *project.RolloutContainerID)
						_ = w.dockerService.RemoveContainer(*project.RolloutContainerID, nil)
						_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
							"rollout_container_id": nil,
						})
					}
				}
			}

			time.Sleep(1 * time.Minute)
		}
	}()
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

			if err := exec.Command("docker", "builder", "prune", "-a", "-f").Run(); err != nil {
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

// StartScaleToZeroJanitor launches the scale-to-zero supervision loop.
func (w *CentralWatchdog) StartScaleToZeroJanitor() {
	go func() {
		time.Sleep(30 * time.Second)
		for w.running {
			w.scaleToZeroCheck()
			select {
			case <-w.stopChan:
				return
			case <-time.After(1 * time.Minute):
			}
		}
	}()
}

func (w *CentralWatchdog) scaleToZeroCheck() {
	idleMinStr := w.settingService.Get(models.SettingScaleToZeroIdleMin, models.DefaultScaleToZeroIdleMin)
	var idleMin int
	if _, err := fmt.Sscanf(idleMinStr, "%d", &idleMin); err != nil {
		idleMin = 1440 // fallback to 24h
	}

	if idleMin <= 0 {
		return
	}

	runningProjects, err := w.projectRepo.GetRunningWithContainers()
	if err != nil {
		slog.Error("Central watchdog: failed to fetch running projects for scale-to-zero check", "error", err)
		return
	}

	for i := range runningProjects {
		project := runningProjects[i]
		if project.LastAccessedAt == nil {
			continue
		}

		if time.Since(*project.LastAccessedAt) > time.Duration(idleMin)*time.Minute {
			slog.Info("Central watchdog: scaling project to zero due to inactivity", "projectId", project.ID, "subdomain", project.Subdomain, "lastAccessed", project.LastAccessedAt)

			if project.ContainerID != nil && *project.ContainerID != "" {
				slog.Info("Central watchdog: scale-to-zero: stopping main container", "containerId", *project.ContainerID)
				if err := w.dockerService.StopContainer(*project.ContainerID); err != nil {
					slog.Warn("Central watchdog: scale-to-zero: failed to stop web container", "projectId", project.ID, "error", err)
				}
			}

			if project.WorkerContainerID != nil && *project.WorkerContainerID != "" {
				slog.Info("Central watchdog: scale-to-zero: stopping worker container", "containerId", *project.WorkerContainerID)
				if err := w.dockerService.StopContainer(*project.WorkerContainerID); err != nil {
					slog.Warn("Central watchdog: scale-to-zero: failed to stop worker container", "projectId", project.ID, "error", err)
				}
			}

			if err := w.projectRepo.UpdateStatus(project.ID, models.StatusSleeping); err != nil {
				slog.Error("Central watchdog: scale-to-zero: failed to update status to sleeping", "projectId", project.ID, "error", err)
				continue
			}

			if err := w.projectService.InvalidateSubdomainCache(project.Subdomain); err != nil {
				slog.Warn("Central watchdog: scale-to-zero: failed to invalidate subdomain cache", "subdomain", project.Subdomain, "error", err)
			}

			jobID := "system"
			if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
				jobID = *project.DeploymentJobID
			}

			event := &models.DeploymentEvent{
				ProjectID: project.ID,
				JobID:     jobID,
				WorkerID:  "watchdog",
				StateFrom: string(models.StatusRunning),
				StateTo:   string(models.StatusSleeping),
				EventType: "scale_to_zero",
				Payload:   "Scale to Zero: Project went idle, sleeping container",
				CreatedAt: time.Now(),
			}

			if err := w.projectRepo.RecordDeploymentEvent(event); err != nil {
				slog.Error("Central watchdog: scale-to-zero: failed to record event", "projectId", project.ID, "error", err)
			}

			if eventJSON, err := json.Marshal(event); err == nil {
				_ = w.redisService.PublishDeploymentEvent(project.ID, string(eventJSON))
			}
		}
	}
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

func (w *CentralWatchdog) autoHealingCheck() {
	runningProjects, err := w.projectRepo.GetRunningWithContainers()
	if err != nil {
		slog.Error("Central watchdog: failed to fetch running projects for auto-healing check", "error", err)
		return
	}

	for i := range runningProjects {
		project := runningProjects[i]

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
				}
			}
		}
	}
}
