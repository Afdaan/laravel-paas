package workers

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	"gorm.io/gorm"
)

func TestBillingRuntimeAction(t *testing.T) {
	for _, jobType := range []string{"deploy", "redeploy", "redeploy_clean", "rollback", "start", "restart", "update_env"} {
		if !billingRuntimeAction(jobType) {
			t.Fatalf("%q must pass through the billing runtime gate", jobType)
		}
	}
	for _, jobType := range []string{"stop", "delete", ""} {
		if billingRuntimeAction(jobType) {
			t.Fatalf("%q must not be blocked from stopping or deleting", jobType)
		}
	}
}

func TestBillingStopCompensationOnlyRestartsContainersStoppedByJob(t *testing.T) {
	mainContainerID := "main-running"
	workerContainerID := "worker-stopped"
	project := &models.Project{ContainerID: &mainContainerID, WorkerContainerID: &workerContainerID}
	running := map[string]bool{mainContainerID: true, workerContainerID: false}
	var stopped, started []string
	snapshot, err := stopProjectContainers(
		project,
		true,
		func(containerID string) (bool, error) { return running[containerID], nil },
		func(containerID string) error {
			stopped = append(stopped, containerID)
			return nil
		},
		func(string) {},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stopped, []string{mainContainerID}) {
		t.Fatalf("stopped=%v", stopped)
	}
	if !snapshot.mainWasRunning || snapshot.workerWasRunning {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if err := restoreBillingStoppedContainers(snapshot, func(containerID string) error {
		started = append(started, containerID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, []string{mainContainerID}) {
		t.Fatalf("stale billing compensation restarted user-stopped container: %v", started)
	}
}

func TestBillingStopDoesNotCompensateAfterInspectionFailure(t *testing.T) {
	mainContainerID := "main"
	project := &models.Project{ContainerID: &mainContainerID}
	called := false
	_, err := stopProjectContainers(
		project,
		true,
		func(string) (bool, error) { return false, errors.New("docker unavailable") },
		func(string) error {
			called = true
			return nil
		},
		func(string) {},
		nil,
	)
	if err == nil || called {
		t.Fatalf("err=%v stop-called=%t", err, called)
	}
}

func TestBillingStopCompensatesContainersStoppedBeforePartialFailure(t *testing.T) {
	mainContainerID, workerContainerID := "main", "worker"
	project := &models.Project{ContainerID: &mainContainerID, WorkerContainerID: &workerContainerID}
	var started []string
	snapshot, err := stopProjectContainers(
		project,
		true,
		func(string) (bool, error) { return true, nil },
		func(containerID string) error {
			if containerID == workerContainerID {
				return errors.New("worker stop failed")
			}
			return nil
		},
		func(string) {},
		func(snapshot billingStopSnapshot) error {
			if !snapshot.mainWasRunning || !snapshot.workerWasRunning {
				t.Fatalf("checkpoint=%#v", snapshot)
			}
			return nil
		},
	)
	if err == nil || !snapshot.mainStopped || snapshot.workerStopped {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	if err := restoreBillingStoppedContainers(snapshot, func(containerID string) error {
		started = append(started, containerID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started, []string{mainContainerID}) {
		t.Fatalf("partial failure compensation=%v", started)
	}
}

func TestBillingSuspensionStopRejectsPaidResource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: 1, Type: models.BillableTypeProject, ResourceID: 1, SpecID: 1, BillingStatus: models.BillableResourceStatusActive}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db)}
	if worker.billingSuspensionStillRequired(resource.ResourceID, 0) {
		t.Fatal("paid resource allowed billing-origin stop")
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	task := models.ProjectSuspensionTask{ProjectID: resource.ResourceID, BillableResourceID: resource.ID, UserID: resource.UserID}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if !worker.billingSuspensionStillRequired(resource.ResourceID, task.ID) {
		t.Fatal("suspended resource blocked billing-origin stop")
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	if worker.billingSuspensionStillRequired(resource.ResourceID, task.ID) {
		t.Fatal("paid resource allowed queued billing-origin stop")
	}
}

func TestBillingSuspensionStopCheckpointPersistsPreStopRuntimeState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Checkpoint", GithubURL: "https://github.com/example/checkpoint", Subdomain: "billing-checkpoint", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: project.UserID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: project.UserID}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db)}
	snapshot := billingStopSnapshot{mainContainerID: "main-before-stop", workerContainerID: "worker-before-stop", mainWasRunning: true, workerWasRunning: true}
	if err := worker.checkpointBillingSuspensionStop(project.ID, task.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.MainContainerID != snapshot.mainContainerID || task.WorkerContainerID != snapshot.workerContainerID || !task.MainWasRunning || !task.WorkerWasRunning || task.StopAttemptedAt == nil {
		t.Fatalf("task=%#v", task)
	}
	if err := worker.checkpointBillingSuspensionStop(project.ID, task.ID, billingStopSnapshot{mainContainerID: snapshot.mainContainerID, workerContainerID: snapshot.workerContainerID}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !task.MainWasRunning || !task.WorkerWasRunning || task.MainContainerID != snapshot.mainContainerID || task.WorkerContainerID != snapshot.workerContainerID {
		t.Fatalf("retry overwrote pre-stop snapshot: %#v", task)
	}
	canonical, found, err := worker.loadBillingSuspensionStopSnapshot(project.ID, task.ID)
	if err != nil || !found {
		t.Fatalf("load canonical snapshot found=%t err=%v", found, err)
	}
	if canonical.mainContainerID != snapshot.mainContainerID || canonical.workerContainerID != snapshot.workerContainerID || !canonical.mainWasRunning || !canonical.workerWasRunning {
		t.Fatalf("canonical=%#v", canonical)
	}
}

func TestBillingResumeFenceStopsRecordedContainersAfterPriorCrash(t *testing.T) {
	mainContainerID, workerContainerID := "main", "worker"
	var stopped []string
	err := stopResumeOwnedBillingContainers(
		billingResumePlan{mainContainerID: mainContainerID, workerContainerID: workerContainerID, mainWasRunning: true},
		func(containerID string) error {
			stopped = append(stopped, containerID)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stopped, []string{mainContainerID}) {
		t.Fatalf("stopped=%v", stopped)
	}
}

func TestBillingRuntimeGateRejectStopsInterruptedResumeContainers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Billing resume gate", GithubURL: "https://github.com/example/billing-resume-gate", Subdomain: "billing-resume-gate", Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: project.UserID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := models.ProjectSuspensionTask{
		ProjectID:          project.ID,
		BillableResourceID: resource.ID,
		UserID:             project.UserID,
		MainContainerID:    "main-resume-owned",
		WorkerContainerID:  "worker-resume-owned",
		MainWasRunning:     true,
		WorkerWasRunning:   true,
		StopAttemptedAt:    &now,
		CompletedAt:        &now,
		ResumeRequestedAt:  &now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "docker.log")
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte("#!/bin/sh\necho \"$*\" >> \"$DOCKER_TEST_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_LOG", logPath)

	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db), dockerService: &docker.DockerService{}}
	if err := worker.stopStaleBillingResume(project.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	stops, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stops) != "stop worker-resume-owned\nstop main-resume-owned\n" {
		t.Fatalf("runtime gate did not stop recorded resume containers: %q", stops)
	}
}

func TestBillingResumeRejectsRenewedSuspensionAndStopsInterruptedContainers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Billing resume retry", GithubURL: "https://github.com/example/billing-resume-retry", Subdomain: "billing-resume-retry", Status: models.StatusStopped}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: project.UserID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	task := models.ProjectSuspensionTask{
		ProjectID:          project.ID,
		BillableResourceID: resource.ID,
		UserID:             project.UserID,
		MainContainerID:    "main-retry-owned",
		WorkerContainerID:  "worker-retry-owned",
		MainWasRunning:     true,
		WorkerWasRunning:   true,
		StopAttemptedAt:    &now,
		CompletedAt:        &now,
		ResumeRequestedAt:  &now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "docker.log")
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte("#!/bin/sh\necho \"$*\" >> \"$DOCKER_TEST_LOG\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_LOG", logPath)

	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db), dockerService: &docker.DockerService{}}
	resumed, err := worker.resumeBillingSuspension(project.ID, task.ID)
	if err != nil || resumed {
		t.Fatalf("resumed=%t err=%v", resumed, err)
	}
	stops, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stops) != "stop worker-retry-owned\nstop main-retry-owned\n" {
		t.Fatalf("resume retry did not stop recorded containers: %q", stops)
	}
}

func TestFinalizeBillingSuspensionStopKeepsPaidProjectRunning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Billing fence", GithubURL: "https://github.com/example/billing-fence", Subdomain: "billing-fence", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: project.UserID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: project.UserID}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db)}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	finalized, err := worker.finalizeBillingSuspensionStop(project.ID, task.ID)
	if err != nil || finalized {
		t.Fatalf("paid finalization finalized=%t err=%v", finalized, err)
	}
	if err := db.First(&project, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if project.Status != models.StatusRunning {
		t.Fatalf("paid project status=%s", project.Status)
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusSuspended).Error; err != nil {
		t.Fatal(err)
	}
	finalized, err = worker.finalizeBillingSuspensionStop(project.ID, task.ID)
	if err != nil || !finalized {
		t.Fatalf("suspended finalization finalized=%t err=%v", finalized, err)
	}
	if err := db.First(&project, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if project.Status != models.StatusStopped {
		t.Fatalf("suspended project status=%s", project.Status)
	}
	if err := db.First(&task, task.ID).Error; err != nil || task.StopCompletedAt == nil || task.CompletedAt != nil {
		t.Fatalf("task=%#v err=%v", task, err)
	}
}

func TestBillingSuspensionPaymentBetweenPrecheckAndFinalizationKeepsProjectRunning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableResource{}, &models.ProjectSuspensionTask{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: 1, Name: "Billing race", GithubURL: "https://github.com/example/billing-race", Subdomain: "billing-race", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: project.UserID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	task := models.ProjectSuspensionTask{ProjectID: project.ID, BillableResourceID: resource.ID, UserID: project.UserID}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	worker := &DeploymentWorker{projectRepo: repositories.NewProjectRepository(db)}
	if !worker.billingSuspensionStillRequired(project.ID, task.ID) {
		t.Fatal("billing stop precheck rejected suspended resource")
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	finalized, err := worker.finalizeBillingSuspensionStop(project.ID, task.ID)
	if err != nil || finalized {
		t.Fatalf("finalized=%t err=%v", finalized, err)
	}
	if err := db.First(&project, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if project.Status != models.StatusRunning {
		t.Fatalf("stale billing stop persisted status=%s", project.Status)
	}
}
