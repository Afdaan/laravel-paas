package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/laravel-paas/shared/models"
)

func TestProjectSuspensionDispatcherRetainsIntentUntilStopped(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	task := models.ProjectSuspensionTask{ProjectID: fixture.projectID, BillableResourceID: fixture.resource.ID, UserID: fixture.resource.UserID}
	if err := fixture.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&fixture.resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}

	dispatcher := newProjectSuspensionDispatcher(fixture.db, func(uint, uint, uint) (string, error) {
		return "", errors.New("redis unavailable")
	})
	if err := dispatcher.DispatchPending(context.Background()); err == nil {
		t.Fatal("expected enqueue failure")
	}
	if err := fixture.db.First(&task, task.ID).Error; err != nil || task.CompletedAt != nil || task.RetryCount != 1 {
		t.Fatalf("task=%#v err=%v", task, err)
	}

	var calls int
	dispatcher = newProjectSuspensionDispatcher(fixture.db, func(projectID, userID, taskID uint) (string, error) {
		calls++
		if projectID != fixture.projectID || userID != fixture.resource.UserID || taskID != task.ID {
			t.Fatalf("enqueue project=%d user=%d task=%d", projectID, userID, taskID)
		}
		return "job", nil
	})
	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("enqueue calls=%d", calls)
	}
	if err := fixture.db.First(&task, task.ID).Error; err != nil || task.CompletedAt != nil {
		t.Fatalf("task was completed before worker acknowledgment: %#v err=%v", task, err)
	}
	if err := fixture.db.Model(&models.Project{}).Where("id = ?", fixture.projectID).Update("status", models.StatusStopped).Error; err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("logical stopped status skipped physical stop: enqueue calls=%d", calls)
	}
	if err := fixture.db.First(&task, task.ID).Error; err != nil || task.CompletedAt != nil || task.StopCompletedAt != nil {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}

func TestProjectSuspensionDispatcherCompletesStalePaidTask(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	task := models.ProjectSuspensionTask{ProjectID: fixture.projectID, BillableResourceID: fixture.resource.ID, UserID: fixture.resource.UserID}
	if err := fixture.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	dispatcher := newProjectSuspensionDispatcher(fixture.db, func(uint, uint, uint) (string, error) {
		calls++
		return "job", nil
	})
	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("stale paid suspension enqueued %d stop jobs", calls)
	}
	if err := fixture.db.First(&task, task.ID).Error; err != nil || task.CompletedAt == nil {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}

func TestProjectSuspensionDispatcherEnqueuesDurableResumeForPaidStoppedProject(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	if err := fixture.db.Model(&fixture.resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := models.ProjectSuspensionTask{
		ProjectID:          fixture.projectID,
		BillableResourceID: fixture.resource.ID,
		UserID:             fixture.resource.UserID,
		MainContainerID:    "main-suspended",
		MainWasRunning:     true,
		StopAttemptedAt:    &now,
		CompletedAt:        &now,
		ResumeRequestedAt:  &now,
	}
	if err := fixture.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	stopCalls, resumeCalls := 0, 0
	dispatcher := newProjectSuspensionDispatcher(
		fixture.db,
		func(uint, uint, uint) (string, error) {
			stopCalls++
			return "stop", nil
		},
		func(projectID, userID, taskID uint) (string, error) {
			resumeCalls++
			if projectID != fixture.projectID || userID != fixture.resource.UserID || taskID != task.ID {
				t.Fatalf("resume project=%d user=%d task=%d", projectID, userID, taskID)
			}
			return "resume", nil
		},
	)
	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stopCalls != 0 || resumeCalls != 1 {
		t.Fatalf("stop=%d resume=%d", stopCalls, resumeCalls)
	}
}

func TestProjectSuspensionDispatcherReopensRenewedSuspensionBeforePhysicalStop(t *testing.T) {
	fixture := suspensionFixture(t, models.BillableTypeProject)
	if err := fixture.db.Model(&fixture.resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.Project{}).Where("id = ?", fixture.projectID).Update("status", models.StatusStopped).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := models.ProjectSuspensionTask{
		ProjectID:          fixture.projectID,
		BillableResourceID: fixture.resource.ID,
		UserID:             fixture.resource.UserID,
		MainContainerID:    "main-resume-owned",
		WorkerContainerID:  "worker-resume-owned",
		MainWasRunning:     true,
		WorkerWasRunning:   true,
		StopAttemptedAt:    &now,
		StopCompletedAt:    &now,
		CompletedAt:        &now,
		ResumeRequestedAt:  &now,
	}
	if err := fixture.db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	stopCalls, resumeCalls := 0, 0
	dispatcher := newProjectSuspensionDispatcher(
		fixture.db,
		func(projectID, userID, taskID uint) (string, error) {
			stopCalls++
			if projectID != fixture.projectID || userID != fixture.resource.UserID || taskID != task.ID {
				t.Fatalf("stop project=%d user=%d task=%d", projectID, userID, taskID)
			}
			return "stop", nil
		},
		func(uint, uint, uint) (string, error) {
			resumeCalls++
			return "resume", nil
		},
	)
	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stopCalls != 1 || resumeCalls != 0 {
		t.Fatalf("stop=%d resume=%d", stopCalls, resumeCalls)
	}
	var persisted models.ProjectSuspensionTask
	if err := fixture.db.First(&persisted, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.CompletedAt != nil || persisted.StopCompletedAt != nil || persisted.ResumeRequestedAt != nil || persisted.ResumeCompletedAt != nil {
		t.Fatalf("renewed suspension was not reopened: %#v", persisted)
	}
}
