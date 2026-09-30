package workers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	"github.com/redis/go-redis/v9"
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

func TestBillingQuotaReconciliationAction(t *testing.T) {
	for _, jobType := range []string{"deploy", "redeploy", "redeploy_clean", "rollback", "restart"} {
		if !billingQuotaReconciliationAction(jobType) {
			t.Fatalf("%q must recreate containers with current billing quota", jobType)
		}
	}
	for _, jobType := range []string{"start", "stop", "update_env", "delete", ""} {
		if billingQuotaReconciliationAction(jobType) {
			t.Fatalf("%q must not claim to apply new Docker limits", jobType)
		}
	}
}

func TestReconcileProjectBillingQuotaUsesAssignedSpec(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableSpec{}, &models.BillableResource{}); err != nil {
		t.Fatal(err)
	}
	legacyCPU := 0.5
	legacyMemory := "512m"
	project := models.Project{
		UserID:      1,
		Name:        "Quota sync",
		GithubURL:   "https://github.com/example/quota-sync",
		Subdomain:   "quota-sync",
		CPULimit:    &legacyCPU,
		MemoryLimit: &legacyMemory,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{
		Type:           models.BillableTypeProject,
		Name:           "Large",
		Slug:           "large",
		CPUMillicores:  2000,
		MemoryMB:       4096,
		StorageGB:      20,
		MonthlyCredits: 400,
		Version:        1,
		IsActive:       false,
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{
		UserID:        project.UserID,
		Type:          models.BillableTypeProject,
		ResourceID:    project.ID,
		SpecID:        spec.ID,
		BillingStatus: models.BillableResourceStatusActive,
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	worker := &DeploymentWorker{
		cfg:         &config.Config{BillingEnabled: true},
		projectRepo: repositories.NewProjectRepository(db),
	}
	changed, err := worker.reconcileProjectBillingQuota(context.Background(), &project)
	if err != nil || !changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	if project.CPULimit == nil || *project.CPULimit != 2 || project.MemoryLimit == nil || *project.MemoryLimit != "4096m" {
		t.Fatalf("project quota=%#v/%#v", project.CPULimit, project.MemoryLimit)
	}

	var persisted models.Project
	if err := db.First(&persisted, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.CPULimit == nil || *persisted.CPULimit != 2 || persisted.MemoryLimit == nil || *persisted.MemoryLimit != "4096m" {
		t.Fatalf("persisted quota=%#v/%#v", persisted.CPULimit, persisted.MemoryLimit)
	}
}

func TestReconcileProjectBillingQuotaDoesNotAssignLegacyProject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.BillableSpec{}, &models.BillableResource{}); err != nil {
		t.Fatal(err)
	}
	project := models.Project{
		UserID:    1,
		Name:      "Legacy project",
		GithubURL: "https://github.com/example/legacy-project",
		Subdomain: "legacy-project",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	worker := &DeploymentWorker{
		cfg:         &config.Config{BillingEnabled: true},
		projectRepo: repositories.NewProjectRepository(db),
	}
	changed, err := worker.reconcileProjectBillingQuota(context.Background(), &project)
	if err != nil || changed {
		t.Fatalf("changed=%t err=%v", changed, err)
	}
	if project.CPULimit != nil || project.MemoryLimit != nil {
		t.Fatalf("legacy project gained unbilled quota=%#v/%#v", project.CPULimit, project.MemoryLimit)
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
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

func TestUpdateProjectErrorSkippedWhenStateTransitionRejected(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DeploymentEvent{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	currentJobID := "newer-job"
	project := models.Project{
		UserID:           1,
		Name:             "Worker race",
		Subdomain:        "worker-race",
		DeploymentStatus: models.DepStatusQueued,
		DeploymentJobID:  &currentJobID,
		Status:           models.StatusQueued,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	repo := repositories.NewProjectRepository(db)
	transMgr := deployment.NewTransitionManager(db, nil)
	projService := projectServicePkg.NewProjectService(&config.Config{}, repo, nil, nil, nil, nil, nil, transMgr)
	worker := &DeploymentWorker{
		cfg:            &config.Config{ProjectsPath: t.TempDir()},
		projectRepo:    repo,
		projectService: projService,
	}

	// Stale worker running old job "stale-job" fails and calls updateProjectError
	worker.updateProjectError(&project, "stale-job", "fatal compilation error")

	// Project in DB must remain completely unmodified (not failed, error_log nil, still newer-job)
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status queued, got %s", current.DeploymentStatus)
	}
	if current.Status != models.StatusQueued {
		t.Fatalf("expected project status queued, got %s", current.Status)
	}
	if current.ErrorLog != nil {
		t.Fatalf("expected ErrorLog to be nil, got %s", *current.ErrorLog)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != currentJobID {
		t.Fatalf("expected deployment_job_id to remain %s, got %v", currentJobID, current.DeploymentJobID)
	}
}

func TestDeploymentWorker_PendingEnvRefresh_InterleavingDoesNotClearNewerMarker(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	t.Cleanup(func() { _ = client.Close() })
	redisService := infrastructure.NewRedisServiceWithClient(client)
	_ = client.FlushDB(context.Background()).Err()

	projectID := uint(time.Now().UnixNano() % 1000000000)

	// Step 1: Initial env update sets marker -> generation 1
	if err := redisService.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatalf("failed to set pending env refresh: %v", err)
	}
	gen1, err := redisService.GetPendingEnvRefresh(projectID)
	if err != nil || gen1 != 1 {
		t.Fatalf("expected generation 1, got gen=%d err=%v", gen1, err)
	}

	// Step 2: Worker A completes and detects pending marker for generation 1
	// Step 3: During worker A's quiet enqueue/processing, update B commits another change -> generation 2
	if err := redisService.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatalf("failed to set second pending env refresh: %v", err)
	}
	gen2, err := redisService.GetPendingEnvRefresh(projectID)
	if err != nil || gen2 != 2 {
		t.Fatalf("expected generation 2, got gen=%d err=%v", gen2, err)
	}

	// Step 4: Worker A finishes its quiet enqueue and attempts to clear marker up to generation 1
	cleared, err := redisService.ClearPendingEnvRefresh(projectID, gen1)
	if err != nil {
		t.Fatalf("failed to clear pending env refresh: %v", err)
	}
	if cleared {
		t.Fatalf("expected ClearPendingEnvRefresh(gen1=1) to return false because current is gen2=2")
	}

	// Step 5: Marker for generation 2 MUST still be active
	hasPending, err := redisService.HasPendingEnvRefresh(projectID)
	if err != nil || !hasPending {
		t.Fatalf("expected pending env refresh marker to remain active for generation 2, hasPending=%v err=%v", hasPending, err)
	}

	// Step 6: Worker B picks up generation 2 and clears up to generation 2
	clearedB, err := redisService.ClearPendingEnvRefresh(projectID, gen2)
	if err != nil {
		t.Fatalf("failed to clear pending env refresh for gen 2: %v", err)
	}
	if !clearedB {
		t.Fatalf("expected ClearPendingEnvRefresh(gen2=2) to return true")
	}

	// Step 7: Marker should now be completely cleared
	hasPendingFinal, err := redisService.HasPendingEnvRefresh(projectID)
	if err != nil || hasPendingFinal {
		t.Fatalf("expected pending marker to be fully cleared, hasPending=%v err=%v", hasPendingFinal, err)
	}
	if err := redisService.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatal(err)
	}
	gen3, err := redisService.GetPendingEnvRefresh(projectID)
	if err != nil || gen3 <= gen2 {
		t.Fatalf("third env update reused old generation: current=%d previous=%d err=%v", gen3, gen2, err)
	}
	if cleared, err := redisService.ClearPendingEnvRefresh(projectID, gen2); err != nil || cleared {
		t.Fatalf("late worker cleared third update: cleared=%v err=%v", cleared, err)
	}
	jobID, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, 1)
	if err != nil || jobID == "" {
		t.Fatalf("third update must still enqueue: job_id=%q err=%v", jobID, err)
	}
	if err := redisService.RemoveDeploymentJob(jobID); err != nil {
		t.Fatal(err)
	}
}
