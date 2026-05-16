// ===========================================
// Central Watchdog Service
// ===========================================
// Background monitoring daemon running in the backend API.
// Handles startup recovery, stale deployment checks,
// and scheduled garbage collection.
// ===========================================
package worker

import (
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

			if err := w.redisService.ReleaseDeploymentLock(project.ID); err != nil {
				slog.Warn("Central watchdog: failed to release lock during recovery", "id", project.ID, "error", err)
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

func (w *CentralWatchdog) StartStaleBuildWatchdog() {
	go func() {
		time.Sleep(2 * time.Minute)

		for w.running {
			staleThreshold := 15 * time.Minute

			projects, err := w.projectRepo.ListByStatus(models.StatusBuilding)
			if err != nil {
				slog.Error("Central watchdog: failed to query building projects", "error", err)
				time.Sleep(5 * time.Minute)
				continue
			}

			for i := range projects {
				project := projects[i]

				isLocked := w.redisService.IsDeploymentLocked(project.ID)
				isStale := time.Since(project.UpdatedAt) > staleThreshold

				if !isLocked || isStale {
					reason := "stale timeout"
					if !isLocked {
						reason = "worker process terminated unexpectedly (lock lost)"
					}
					slog.Warn("Central watchdog: detected failed deployment",
						"projectId", project.ID,
						"subdomain", project.Subdomain,
						"reason", reason)

					errorMsg := fmt.Sprintf("Deployment failed: %s.", reason)
					project.Status = models.StatusFailed
					project.ErrorLog = &errorMsg
					if err := w.projectRepo.Update(&project); err != nil {
						slog.Error("Central watchdog: failed to reset project", "id", project.ID, "error", err)
					}

					if isLocked {
						if err := w.redisService.ReleaseDeploymentLock(project.ID); err != nil {
							slog.Warn("Central watchdog: failed to release lock", "id", project.ID, "error", err)
						}
					}
				}
			}

			time.Sleep(1 * time.Minute)
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
