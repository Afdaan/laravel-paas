package deployment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"gorm.io/gorm"
)

func newTestManager(t *testing.T) (*transitionManager, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DeploymentEvent{}); err != nil {
		t.Fatal(err)
	}
	return &transitionManager{db: db, hostname: "test"}, db
}

func reload(t *testing.T, db *gorm.DB, id uint) models.Project {
	t.Helper()
	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		t.Fatal(err)
	}
	return project
}

// Queue admission and worker pickup must be distinguishable, otherwise queue
// wait time is unmeasurable.
func TestTransitionSeparatesEnqueueFromPickup(t *testing.T) {
	manager, db := newTestManager(t)
	project := models.Project{Name: "p", Subdomain: "p", DeploymentStatus: models.DepStatusCompleted}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := manager.TransitionState(ctx, project.ID, "job-1", models.DepStatusQueued, 0, "enqueued", "queued"); err != nil {
		t.Fatal(err)
	}
	queued := reload(t, db, project.ID)
	if queued.DeploymentEnqueuedAt == nil {
		t.Fatal("queued must record deployment_enqueued_at")
	}
	if queued.DeploymentStartedAt != nil {
		t.Fatal("queued must not record deployment_started_at")
	}

	if _, err := manager.TransitionState(ctx, project.ID, "job-1", models.DepStatusPreparing, 10, "picked_up", "preparing"); err != nil {
		t.Fatal(err)
	}
	preparing := reload(t, db, project.ID)
	if preparing.DeploymentStartedAt == nil {
		t.Fatal("preparing must record deployment_started_at")
	}
	if preparing.DeploymentEnqueuedAt == nil || !preparing.DeploymentEnqueuedAt.Equal(*queued.DeploymentEnqueuedAt) {
		t.Fatal("worker pickup must not overwrite the enqueue timestamp")
	}
	if preparing.DeploymentFinishedAt != nil {
		t.Fatal("an in-flight deployment must have no finish timestamp")
	}
}

// A rollback ends the job, so its duration must be computable.
func TestTransitionMarksRollbackFinished(t *testing.T) {
	manager, db := newTestManager(t)
	project := models.Project{Name: "p", Subdomain: "p2", DeploymentStatus: models.DepStatusHealthchecking}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := manager.TransitionState(context.Background(), project.ID, "job-2", models.DepStatusRollback, 60, "rollback", "rolled back"); err != nil {
		t.Fatal(err)
	}
	if reload(t, db, project.ID).DeploymentFinishedAt == nil {
		t.Fatal("rollback is terminal and must set deployment_finished_at")
	}
}

func TestTransitionRejectsInvalidProgression(t *testing.T) {
	manager, db := newTestManager(t)
	project := models.Project{Name: "p", Subdomain: "p3", DeploymentStatus: models.DepStatusQueued}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := manager.TransitionState(context.Background(), project.ID, "job-3", models.DepStatusPromoting, 80, "bogus", "nope"); err == nil {
		t.Fatal("queued -> promoting must be rejected")
	}
	if reload(t, db, project.ID).DeploymentStatus != models.DepStatusQueued {
		t.Fatal("a rejected transition must not mutate the project")
	}
}

func TestTransitionRequeuesInFlightDeployment(t *testing.T) {
	manager, db := newTestManager(t)
	oldEnqueuedAt := time.Now().Add(-2 * time.Minute)
	oldStartedAt := time.Now().Add(-time.Minute)
	project := models.Project{
		Name:                 "recovery",
		Subdomain:            "recovery",
		DeploymentStatus:     models.DepStatusBuilding,
		DeploymentEnqueuedAt: &oldEnqueuedAt,
		DeploymentStartedAt:  &oldStartedAt,
		DeploymentFinishedAt: &oldStartedAt,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := manager.TransitionState(context.Background(), project.ID, "recovery-job", models.DepStatusQueued, 0, "watchdog_recovery", "requeued"); err != nil {
		t.Fatal(err)
	}

	requeued := reload(t, db, project.ID)
	if requeued.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("status = %s, want queued", requeued.DeploymentStatus)
	}
	if requeued.DeploymentEnqueuedAt == nil || !requeued.DeploymentEnqueuedAt.After(oldEnqueuedAt) {
		t.Fatal("requeue must refresh deployment_enqueued_at")
	}
	if requeued.DeploymentStartedAt != nil {
		t.Fatal("requeue must clear deployment_started_at")
	}
	if requeued.DeploymentFinishedAt != nil {
		t.Fatal("requeue must clear deployment_finished_at")
	}

	var event models.DeploymentEvent
	if err := db.Where("project_id = ? AND job_id = ?", project.ID, "recovery-job").First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.SequenceNumber != 1 || event.StateFrom != string(models.DepStatusBuilding) || event.StateTo != string(models.DepStatusQueued) {
		t.Fatalf("unexpected recovery event: sequence=%d from=%s to=%s", event.SequenceNumber, event.StateFrom, event.StateTo)
	}
}

func TestTransitionAcceptsFullSQLiteDeploymentSequence(t *testing.T) {
	manager, db := newTestManager(t)
	project := models.Project{Name: "sqlite", Subdomain: "sqlite", DeploymentStatus: models.DepStatusBuilding}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	sequence := []models.DeploymentStatus{
		models.DepStatusStarting,
		models.DepStatusMigrating,
		models.DepStatusHealthchecking,
		models.DepStatusPromoting,
		models.DepStatusCompleted,
	}
	for index, status := range sequence {
		if _, err := manager.TransitionState(context.Background(), project.ID, "sqlite-job", status, (index+1)*20, "sqlite_step", string(status)); err != nil {
			t.Fatalf("transition to %s failed: %v", status, err)
		}
	}

	var events []models.DeploymentEvent
	if err := db.Where("project_id = ? AND job_id = ?", project.ID, "sqlite-job").Order("sequence_number ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != len(sequence) {
		t.Fatalf("got %d events, want %d", len(events), len(sequence))
	}
	for index, event := range events {
		if event.SequenceNumber != index+1 {
			t.Fatalf("event %d has sequence %d", index, event.SequenceNumber)
		}
		if event.StateTo != string(sequence[index]) {
			t.Fatalf("event %d transitioned to %s, want %s", index, event.StateTo, sequence[index])
		}
	}
}

func TestFailQueuedDeploymentIfUnchanged(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	t.Run("applies failure when job and state match queued", func(t *testing.T) {
		jobID := "job-match"
		project := models.Project{
			UID: "p1-uid", Name: "p1", Subdomain: "p1",
			DeploymentStatus: models.DepStatusQueued,
			DeploymentJobID:  &jobID,
			Status:           models.StatusQueued,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		applied, err := manager.FailQueuedDeploymentIfUnchanged(ctx, project.ID, jobID, "queue failure", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !applied {
			t.Fatal("expected failure compensation to be applied")
		}

		updated := reload(t, db, project.ID)
		if updated.DeploymentStatus != models.DepStatusFailed {
			t.Fatalf("expected deployment status failed, got %s", updated.DeploymentStatus)
		}
		if updated.Status != models.StatusFailed {
			t.Fatalf("expected project status failed, got %s", updated.Status)
		}
		if updated.DeploymentFinishedAt == nil {
			t.Fatal("expected deployment_finished_at to be recorded")
		}
	})

	t.Run("skips failure when job ID does not match", func(t *testing.T) {
		currentJobID := "newer-job"
		project := models.Project{
			UID: "p2-uid", Name: "p2", Subdomain: "p2",
			DeploymentStatus: models.DepStatusQueued,
			DeploymentJobID:  &currentJobID,
			Status:           models.StatusQueued,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		staleJobID := "stale-job"
		applied, err := manager.FailQueuedDeploymentIfUnchanged(ctx, project.ID, staleJobID, "stale error", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if applied {
			t.Fatal("expected failure compensation to be skipped for stale job ID")
		}

		updated := reload(t, db, project.ID)
		if updated.DeploymentStatus != models.DepStatusQueued {
			t.Fatalf("expected deployment status to stay queued, got %s", updated.DeploymentStatus)
		}
		if *updated.DeploymentJobID != currentJobID {
			t.Fatalf("expected deployment job ID %s, got %s", currentJobID, *updated.DeploymentJobID)
		}
		if updated.Status != models.StatusQueued {
			t.Fatalf("expected project status to stay queued, got %s", updated.Status)
		}
	})

	t.Run("skips failure when state is no longer queued", func(t *testing.T) {
		jobID := "claimed-job"
		project := models.Project{
			UID: "p3-uid", Name: "p3", Subdomain: "p3",
			DeploymentStatus: models.DepStatusPreparing,
			DeploymentJobID:  &jobID,
			Status:           models.StatusQueued,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		applied, err := manager.FailQueuedDeploymentIfUnchanged(ctx, project.ID, jobID, "too late", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if applied {
			t.Fatal("expected failure compensation to be skipped when state is preparing")
		}

		updated := reload(t, db, project.ID)
		if updated.DeploymentStatus != models.DepStatusPreparing {
			t.Fatalf("expected deployment status to stay preparing, got %s", updated.DeploymentStatus)
		}
	})
}

func TestTransitionStateIfMatch(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	t.Run("applies when expected status and job ID match", func(t *testing.T) {
		oldJobID := "old-job-1"
		project := models.Project{
			UID: "match-p1", Name: "p1", Subdomain: "match-p1",
			DeploymentStatus: models.DepStatusBuilding,
			DeploymentJobID:  &oldJobID,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		proj, applied, err := manager.TransitionStateIfMatch(ctx, project.ID, models.DepStatusBuilding, &oldJobID, "new-job-1", models.DepStatusQueued, 0, "recovery", "requeued")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !applied {
			t.Fatal("expected transition to be applied")
		}
		if proj.DeploymentStatus != models.DepStatusQueued || *proj.DeploymentJobID != "new-job-1" {
			t.Fatalf("unexpected project state: status=%s job=%v", proj.DeploymentStatus, proj.DeploymentJobID)
		}
	})

	t.Run("skips when expected status does not match", func(t *testing.T) {
		oldJobID := "old-job-2"
		project := models.Project{
			UID: "match-p2", Name: "p2", Subdomain: "match-p2",
			DeploymentStatus: models.DepStatusStarting,
			DeploymentJobID:  &oldJobID,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		proj, applied, err := manager.TransitionStateIfMatch(ctx, project.ID, models.DepStatusBuilding, &oldJobID, "new-job-2", models.DepStatusQueued, 0, "recovery", "requeued")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if applied || proj != nil {
			t.Fatal("expected transition to be skipped when status mismatches")
		}
		updated := reload(t, db, project.ID)
		if updated.DeploymentStatus != models.DepStatusStarting || *updated.DeploymentJobID != oldJobID {
			t.Fatalf("project should remain untouched, got status=%s job=%v", updated.DeploymentStatus, updated.DeploymentJobID)
		}
	})

	t.Run("skips when expected job ID does not match", func(t *testing.T) {
		currentJobID := "newer-job"
		staleJobID := "stale-job"
		project := models.Project{
			UID: "match-p3", Name: "p3", Subdomain: "match-p3",
			DeploymentStatus: models.DepStatusBuilding,
			DeploymentJobID:  &currentJobID,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		proj, applied, err := manager.TransitionStateIfMatch(ctx, project.ID, models.DepStatusBuilding, &staleJobID, "new-job-3", models.DepStatusQueued, 0, "recovery", "requeued")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if applied || proj != nil {
			t.Fatal("expected transition to be skipped when job ID mismatches")
		}
		updated := reload(t, db, project.ID)
		if *updated.DeploymentJobID != currentJobID {
			t.Fatalf("project should remain owned by newer job %s, got %s", currentJobID, *updated.DeploymentJobID)
		}
	})
}

func TestRequeueDeployment(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	oldJobID := "hung-job"
	project := models.Project{
		UID: "requeue-p1", Name: "p1", Subdomain: "requeue-p1",
		DeploymentStatus: models.DepStatusBuilding,
		DeploymentJobID:  &oldJobID,
		Status:           models.StatusBuilding,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	proj, err := manager.RequeueDeployment(ctx, project.ID, "admin-job-1", "Admin requeue")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proj.DeploymentStatus != models.DepStatusQueued || proj.Status != models.StatusQueued || *proj.DeploymentJobID != "admin-job-1" {
		t.Fatalf("unexpected project state after requeue: dep_status=%s status=%s job=%v", proj.DeploymentStatus, proj.Status, proj.DeploymentJobID)
	}

	updated := reload(t, db, project.ID)
	if updated.DeploymentStatus != models.DepStatusQueued || updated.Status != models.StatusQueued || *updated.DeploymentJobID != "admin-job-1" {
		t.Fatalf("reloaded project mismatch: dep_status=%s status=%s job=%v", updated.DeploymentStatus, updated.Status, updated.DeploymentJobID)
	}
}

func TestRequeueKeepsServingContainerEligibleForAutoHealing(t *testing.T) {
	manager, db := newTestManager(t)
	containerID := "serving-container"
	project := models.Project{
		Name: "serving", Subdomain: "serving", Status: models.StatusRunning,
		ContainerID: &containerID, DeploymentStatus: models.DepStatusCompleted,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := manager.RequeueDeploymentIfMatch(context.Background(), &project, "redeploy-job", "Replacing serving container")
	if err != nil || queued.Status != models.StatusRunning || queued.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("queued replacement changed runtime: project=%+v err=%v", queued, err)
	}
	for _, state := range []models.DeploymentStatus{
		models.DepStatusPreparing, models.DepStatusCloning, models.DepStatusBuilding,
		models.DepStatusStarting, models.DepStatusHealthchecking, models.DepStatusPromoting,
	} {
		if _, err := manager.TransitionState(context.Background(), project.ID, "redeploy-job", state, 40, "progress", "building replacement"); err != nil {
			t.Fatal(err)
		}
		running, err := repositories.NewProjectRepository(db).GetRunningWithContainers()
		if err != nil || len(running) != 1 || running[0].ID != project.ID || running[0].ContainerID == nil || *running[0].ContainerID != containerID {
			t.Fatalf("watchdog lost serving container at %s: projects=%+v err=%v", state, running, err)
		}
	}
}

func TestFailedPublishPreservesServingRuntime(t *testing.T) {
	for _, source := range []string{"redeploy", "webhook", "admin"} {
		t.Run(source, func(t *testing.T) {
			manager, db := newTestManager(t)
			containerID := "serving-container"
			project := models.Project{Name: source, Subdomain: source, Status: models.StatusRunning,
				ContainerID: &containerID, DeploymentStatus: models.DepStatusCompleted}
			if err := db.Create(&project).Error; err != nil {
				t.Fatal(err)
			}
			jobID := source + "-job"
			if source == "admin" {
				_, err := manager.RequeueDeployment(context.Background(), project.ID, jobID, "admin override")
				if err != nil {
					t.Fatal(err)
				}
			} else if _, err := manager.RequeueDeploymentIfMatch(context.Background(), &project, jobID, source); err != nil {
				t.Fatal(err)
			}
			applied, err := manager.FailQueuedDeploymentIfUnchanged(context.Background(), project.ID, jobID, "Redis publish failed", true)
			if err != nil || !applied {
				t.Fatalf("failure compensation: applied=%v err=%v", applied, err)
			}
			current := reload(t, db, project.ID)
			if current.Status != models.StatusRunning || current.DeploymentStatus != models.DepStatusFailed || current.ContainerID == nil || *current.ContainerID != containerID {
				t.Fatalf("publish failure changed serving runtime: %+v", current)
			}
			running, err := repositories.NewProjectRepository(db).GetRunningWithContainers()
			if err != nil || len(running) != 1 || running[0].ID != project.ID {
				t.Fatalf("watchdog lost app after failed publish: %+v %v", running, err)
			}
		})
	}
}

func TestFailedAdminRequeueDoesNotStartStoppedContainer(t *testing.T) {
	manager, db := newTestManager(t)
	containerID := "stopped-container"
	project := models.Project{Name: "stopped", Subdomain: "stopped", Status: models.StatusStopped,
		ContainerID: &containerID, DeploymentStatus: models.DepStatusCompleted}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RequeueDeployment(context.Background(), project.ID, "admin-job", "admin override"); err != nil {
		t.Fatal(err)
	}
	applied, err := manager.FailQueuedDeploymentIfUnchanged(context.Background(), project.ID, "admin-job", "publish failed", true)
	if err != nil || !applied {
		t.Fatalf("compensation: applied=%v err=%v", applied, err)
	}
	current := reload(t, db, project.ID)
	if current.Status != models.StatusStopped || current.DeploymentStatus != models.DepStatusFailed {
		t.Fatalf("failed admin requeue changed stopped runtime: %+v", current)
	}
}

func TestTransitionStateRejectsStaleJobOwner(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	activeJobID := "job-B"
	project := models.Project{
		UID:              "stale-owner-p1",
		Name:             "p1",
		Subdomain:        "stale-owner-p1",
		DeploymentStatus: models.DepStatusQueued,
		DeploymentJobID:  &activeJobID,
		Status:           models.StatusQueued,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	// Stale worker running job-A attempts to transition to building
	_, err := manager.TransitionState(ctx, project.ID, "job-A", models.DepStatusBuilding, 20, "build_started", "Building...")
	if err == nil {
		t.Fatal("expected TransitionState to reject transition from stale worker job-A")
	}
	if !errors.Is(err, ErrStaleDeploymentOwner) {
		t.Fatalf("expected ErrStaleDeploymentOwner, got %v", err)
	}

	// Stale worker running job-A attempts to transition to failed
	_, err = manager.TransitionState(ctx, project.ID, "job-A", models.DepStatusFailed, 20, "deployment_failed", "Failed")
	if err == nil {
		t.Fatal("expected TransitionState to reject failure transition from stale worker job-A")
	}
	if !errors.Is(err, ErrStaleDeploymentOwner) {
		t.Fatalf("expected ErrStaleDeploymentOwner, got %v", err)
	}

	// Project in DB must remain completely unmodified
	updated := reload(t, db, project.ID)
	if updated.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected status to remain queued, got %s", updated.DeploymentStatus)
	}
	if updated.DeploymentJobID == nil || *updated.DeploymentJobID != activeJobID {
		t.Fatalf("expected deployment_job_id to remain %s, got %v", activeJobID, updated.DeploymentJobID)
	}
}

func TestRequeueDeploymentClearsStaleErrorLog(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	oldError := "Fatal: npm build failed with exit code 1"
	oldNotice := "High CPU load detected"
	now := time.Now()
	oldJobID := "old-failed-job"

	project := models.Project{
		UID:              "clear-error-p1",
		Name:             "p1",
		Subdomain:        "clear-error-p1",
		DeploymentStatus: models.DepStatusFailed,
		DeploymentJobID:  &oldJobID,
		Status:           models.StatusFailed,
		ErrorLog:         &oldError,
		HealthNotice:     &oldNotice,
		HealthNoticeAt:   &now,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	newJobID := "fresh-requeue-job"
	if _, err := manager.RequeueDeployment(ctx, project.ID, newJobID, "Requeued by admin"); err != nil {
		t.Fatalf("requeue failed: %v", err)
	}

	updated := reload(t, db, project.ID)
	if updated.ErrorLog != nil {
		t.Fatalf("expected ErrorLog to be cleared (nil), got %s", *updated.ErrorLog)
	}
	if updated.HealthNotice != nil {
		t.Fatalf("expected HealthNotice to be cleared (nil), got %s", *updated.HealthNotice)
	}
	if updated.HealthNoticeAt != nil {
		t.Fatalf("expected HealthNoticeAt to be cleared (nil), got %v", updated.HealthNoticeAt)
	}
	if *updated.DeploymentJobID != newJobID {
		t.Fatalf("expected job ID %s, got %s", newJobID, *updated.DeploymentJobID)
	}
	if updated.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status queued, got %s", updated.DeploymentStatus)
	}
}

func TestTransitionStateFailedUpdatesStatusAndErrorLog(t *testing.T) {
	manager, db := newTestManager(t)
	ctx := context.Background()

	t.Run("updates status to failed and sets error_log when no container running", func(t *testing.T) {
		jobID := "fail-job-1"
		project := models.Project{
			UID:              "fail-p1",
			Name:             "p1",
			Subdomain:        "fail-p1",
			DeploymentStatus: models.DepStatusBuilding,
			DeploymentJobID:  &jobID,
			Status:           models.StatusBuilding,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		failMsg := "Build failed: syntax error in main.go"
		updatedProj, err := manager.TransitionState(ctx, project.ID, jobID, models.DepStatusFailed, 50, "deployment_failed", failMsg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updatedProj.DeploymentStatus != models.DepStatusFailed {
			t.Fatalf("expected DepStatusFailed, got %s", updatedProj.DeploymentStatus)
		}
		if updatedProj.Status != models.StatusFailed {
			t.Fatalf("expected StatusFailed, got %s", updatedProj.Status)
		}
		if updatedProj.ErrorLog == nil || *updatedProj.ErrorLog != failMsg {
			t.Fatalf("expected error log %q, got %v", failMsg, updatedProj.ErrorLog)
		}

		reloaded := reload(t, db, project.ID)
		if reloaded.Status != models.StatusFailed {
			t.Fatalf("expected DB StatusFailed, got %s", reloaded.Status)
		}
		if reloaded.ErrorLog == nil || *reloaded.ErrorLog != failMsg {
			t.Fatalf("expected DB error log %q, got %v", failMsg, reloaded.ErrorLog)
		}
	})

	t.Run("preserves status as running and sets error_log when existing container is active", func(t *testing.T) {
		jobID := "fail-job-2"
		containerID := "existing-container-123"
		project := models.Project{
			UID:              "fail-p2",
			Name:             "p2",
			Subdomain:        "fail-p2",
			DeploymentStatus: models.DepStatusBuilding,
			DeploymentJobID:  &jobID,
			Status:           models.StatusRunning,
			ContainerID:      &containerID,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		failMsg := "Redeployment failed: migrations error"
		updatedProj, err := manager.TransitionState(ctx, project.ID, jobID, models.DepStatusFailed, 50, "deployment_failed", failMsg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updatedProj.DeploymentStatus != models.DepStatusFailed {
			t.Fatalf("expected DepStatusFailed, got %s", updatedProj.DeploymentStatus)
		}
		if updatedProj.Status != models.StatusRunning {
			t.Fatalf("expected StatusRunning to be preserved, got %s", updatedProj.Status)
		}
		if updatedProj.ErrorLog == nil || *updatedProj.ErrorLog != failMsg {
			t.Fatalf("expected error log %q, got %v", failMsg, updatedProj.ErrorLog)
		}

		reloaded := reload(t, db, project.ID)
		if reloaded.Status != models.StatusRunning {
			t.Fatalf("expected DB StatusRunning, got %s", reloaded.Status)
		}
		if reloaded.ErrorLog == nil || *reloaded.ErrorLog != failMsg {
			t.Fatalf("expected DB error log %q, got %v", failMsg, reloaded.ErrorLog)
		}
	})

	t.Run("stale job owner failure transition rejected without mutating state", func(t *testing.T) {
		activeJobID := "new-admin-job"
		project := models.Project{
			UID:              "fail-p3",
			Name:             "p3",
			Subdomain:        "fail-p3",
			DeploymentStatus: models.DepStatusQueued,
			DeploymentJobID:  &activeJobID,
			Status:           models.StatusQueued,
			ErrorLog:         nil,
		}
		if err := db.Create(&project).Error; err != nil {
			t.Fatal(err)
		}

		staleJobID := "old-stale-worker-job"
		_, err := manager.TransitionState(ctx, project.ID, staleJobID, models.DepStatusFailed, 50, "deployment_failed", "stale fail message")
		if err == nil || !errors.Is(err, ErrStaleDeploymentOwner) {
			t.Fatalf("expected ErrStaleDeploymentOwner, got %v", err)
		}

		reloaded := reload(t, db, project.ID)
		if reloaded.DeploymentStatus != models.DepStatusQueued {
			t.Fatalf("expected deployment status queued, got %s", reloaded.DeploymentStatus)
		}
		if reloaded.Status != models.StatusQueued {
			t.Fatalf("expected project status queued, got %s", reloaded.Status)
		}
		if reloaded.ErrorLog != nil {
			t.Fatalf("expected error log to remain nil, got %v", reloaded.ErrorLog)
		}
	})
}
