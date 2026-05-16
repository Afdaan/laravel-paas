// ===========================================
// Central Watchdog Service
// ===========================================
// Background monitoring daemon running in the backend API.
// Handles startup recovery, stale deployment checks,
// and scheduled garbage collection.
// ===========================================
package worker

import (
	"encoding/json"
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
	statuses := []models.ProjectStatus{
		models.StatusBuilding,
		models.StatusQueued,
		models.StatusPending,
	}

	for _, status := range statuses {
		projects, err := w.projectRepo.ListByStatus(status)
		if err != nil {
			slog.Error("Central watchdog: failed to query orphaned projects for recovery", "status", status, "error", err)
			continue
		}

		if len(projects) == 0 {
			continue
		}

		slog.Info("Central watchdog: recovering orphaned projects from previous session",
			"status", status,
			"count", len(projects))

		for i := range projects {
			project := projects[i]

			isQueued, _ := w.redisService.IsProjectQueued(project.ID)
			if isQueued {
				slog.Info("Central watchdog: project is already in queue, skipping recovery", "id", project.ID)
				continue
			}

			project.Status = models.StatusQueued
			recoveryLog := fmt.Sprintf("Recovered from unexpected shutdown (previous status: %s).", status)
			project.ErrorLog = &recoveryLog
			if err := w.projectRepo.Update(&project); err != nil {
				slog.Error("Central watchdog: failed to update project during recovery", "id", project.ID, "error", err)
			}

			if err := w.redisService.ForceReleaseDeploymentLock(project.ID); err != nil {
				slog.Warn("Central watchdog: failed to force release lock during recovery", "id", project.ID, "error", err)
			}

			if err := w.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
				slog.Error("Central watchdog: failed to re-queue project during recovery", "id", project.ID, "error", err)
			} else {
				slog.Info("Central watchdog: project automatically re-queued for reliability", "id", project.ID)
			}
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

// recordAuditEvent records a deployment event for watchdog actions
func (w *CentralWatchdog) recordAuditEvent(projectID uint, jobID, eventType, payload string) {
	event := &models.DeploymentEvent{
		ProjectID:  projectID,
		JobID:      jobID,
		WorkerID:   "central-watchdog",
		StateFrom:  models.StatusBuilding,
		StateTo:    models.StatusFailed,
		EventType:  eventType,
		Payload:    payload,
		CreatedAt:  time.Now(),
	}

	if err := w.projectRepo.RecordDeploymentEvent(event); err != nil {
		slog.Warn("Central watchdog: failed to record audit event to database", "projectId", projectID, "error", err)
	}

	eventJSON, err := json.Marshal(event)
	if err == nil {
		_ = w.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
	}
}

// StartStaleBuildWatchdog launches a supervisory loop that detects and cleans up stalled or orphaned deployments.
func (w *CentralWatchdog) StartStaleBuildWatchdog() {
	go func() {
		time.Sleep(2 * time.Minute)

		for w.running {
			staleThreshold := 15 * time.Minute

			inProgressStatuses := []models.ProjectStatus{
				models.StatusPreparing,
				models.StatusCloning,
				models.StatusBuilding,
				models.StatusProvisioning,
				models.StatusStarting,
				models.StatusHealthchecking,
				models.StatusMigrating,
				models.StatusPromoting,
			}
			projects, err := w.projectRepo.ListByStatuses(inProgressStatuses)
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
				} else if time.Since(project.UpdatedAt) > staleThreshold {
					reason = "stale deployment timeout (no active lock or lease)"
				}

				if reason != "" {
					slog.Warn("Central watchdog: detected orphaned deployment",
						"projectId", project.ID,
						"subdomain", project.Subdomain,
						"jobId", jobID,
						"reason", reason)

					errorMsg := fmt.Sprintf("Deployment failed: %s.", reason)
					project.Status = models.StatusFailed
					project.ErrorLog = &errorMsg
					if err := w.projectRepo.Update(&project); err != nil {
						slog.Error("Central watchdog: failed to update failed project", "id", project.ID, "error", err)
					}

					w.recordAuditEvent(project.ID, jobID, "orphan_recovered", fmt.Sprintf("Recovered orphaned deployment: %s", reason))

					if lockMeta != nil {
						if err := w.redisService.ForceReleaseDeploymentLock(project.ID); err != nil {
							slog.Warn("Central watchdog: failed to force release lock", "id", project.ID, "error", err)
						}
					}

					if project.ContainerID != nil && *project.ContainerID != "" {
						slog.Info("Central watchdog: removing orphaned container instance", "containerId", *project.ContainerID)
						_ = w.dockerService.RemoveContainer(*project.ContainerID, project.WorkerContainerID)
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
