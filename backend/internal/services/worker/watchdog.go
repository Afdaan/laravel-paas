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
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/infrastructure/docker"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
)

// CentralWatchdog oversees system consistency and maintenance tasks
type CentralWatchdog struct {
	projectRepo    repositories.ProjectRepository
	redisService   *infrastructure.RedisService
	dockerService  *docker.DockerService
	projectService *projectServicePkg.ProjectService
	running        bool
	stopChan       chan struct{}
}

// NewCentralWatchdog creates a new CentralWatchdog instance
func NewCentralWatchdog(
	projectRepo repositories.ProjectRepository,
	redisService *infrastructure.RedisService,
	dockerService *docker.DockerService,
	projectService *projectServicePkg.ProjectService,
) *CentralWatchdog {
	return &CentralWatchdog{
		projectRepo:    projectRepo,
		redisService:   redisService,
		dockerService:  dockerService,
		projectService: projectService,
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
		if _, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, "", models.DepStatusQueued, 0, "watchdog_recovery", recoveryLog); err != nil {
			slog.Error("Central watchdog: failed to transition project deployment state during recovery", "id", project.ID, "error", err)
		}
		_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"error_log": recoveryLog,
		})

		if err := w.redisService.ForceReleaseDeploymentLock(project.ID, "Watchdog recovering orphaned build from unexpected shutdown"); err != nil {
			slog.Warn("Central watchdog: failed to force release lock during recovery", "id", project.ID, "error", err)
		}

		if err := w.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
			slog.Error("Central watchdog: failed to re-queue project during recovery", "id", project.ID, "error", err)
		} else {
			slog.Info("Central watchdog: project automatically re-queued for reliability", "id", project.ID)
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
				jobID := "unknown"
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
