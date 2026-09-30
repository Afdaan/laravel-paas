package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/services/deployment"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type envSyncPublishHook struct {
	onPublish func(*infrastructure.DeploymentJob) error
}

func (hook envSyncPublishHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (hook envSyncPublishHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (hook envSyncPublishHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err != nil || (cmd.Name() != "evalsha" && cmd.Name() != "eval") || !strings.Contains(fmt.Sprint(cmd.Args()), "deployment:stats") {
			return err
		}
		for _, arg := range cmd.Args() {
			var job infrastructure.DeploymentJob
			if json.Unmarshal([]byte(fmt.Sprint(arg)), &job) == nil && job.JobID != "" {
				return hook.onPublish(&job)
			}
		}
		return err
	}
}

func envSyncFixture(t *testing.T) (*gorm.DB, models.Project, *redis.Client, *redis.Client) {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 13})
	workerClient := redis.NewClient(&redis.Options{Addr: addr, DB: 13})
	t.Cleanup(func() { _ = client.Close(); _ = workerClient.Close() })
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DeploymentEvent{}, &models.ProjectEnvSyncTask{}); err != nil {
		t.Fatal(err)
	}
	containerID := "serving-container"
	project := models.Project{Name: "Env sync", Subdomain: "env-sync", UserID: 1,
		Status: models.StatusRunning, DeploymentStatus: models.DepStatusCompleted, ContainerID: &containerID}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	return db, project, client, workerClient
}

func TestEnvSyncAdmissionBeforeFastWorkerPickup(t *testing.T) {
	db, project, client, workerClient := envSyncFixture(t)
	service := NewProjectEnvSyncService(db, infrastructure.NewRedisServiceWithClient(client))
	workerRedis := infrastructure.NewRedisServiceWithClient(workerClient)
	manager := deployment.NewTransitionManager(db, nil)
	client.AddHook(envSyncPublishHook{onPublish: func(published *infrastructure.DeploymentJob) error {
		var staged models.Project
		if err := db.First(&staged, project.ID).Error; err != nil {
			return err
		}
		if staged.DeploymentStatus != models.DepStatusQueued || staged.DeploymentJobID == nil || *staged.DeploymentJobID != published.JobID || staged.Status != models.StatusRunning || published.EnvSyncGeneration != 3 {
			return fmt.Errorf("published before DB admission: project=%+v job=%+v", staged, published)
		}
		claimed, err := workerRedis.DequeueDeployment(time.Second)
		if err != nil || claimed == nil || claimed.JobID != published.JobID {
			return fmt.Errorf("worker pickup: job=%+v err=%w", claimed, err)
		}
		defer workerRedis.AcknowledgeDeployment(claimed)
		_, err = manager.TransitionState(context.Background(), project.ID, claimed.JobID, models.DepStatusPreparing, 20, "env_started", "running")
		return err
	}})
	jobID, err := service.Enqueue(context.Background(), project.ID, 3)
	if err != nil || jobID == "" {
		t.Fatalf("admission: job=%q err=%v", jobID, err)
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobID || current.DeploymentStatus != models.DepStatusPreparing {
		t.Fatalf("fast pickup was rewound: %+v", current)
	}
}

func TestEnvSyncDefersBehindActiveDeployment(t *testing.T) {
	db, project, client, workerClient := envSyncFixture(t)
	activeJobID := "active-build"
	if err := db.Model(&project).Updates(map[string]interface{}{
		"deployment_job_id": activeJobID, "deployment_status": models.DepStatusBuilding,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProjectEnvSyncTask{ProjectID: project.ID, DesiredGeneration: 3}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewProjectEnvSyncService(db, infrastructure.NewRedisServiceWithClient(client))
	jobID, err := service.Enqueue(context.Background(), project.ID, 3)
	if err != nil || jobID != "" {
		t.Fatalf("active deployment must defer env sync: job=%q err=%v", jobID, err)
	}
	service.dispatch(context.Background())
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusBuilding || current.DeploymentJobID == nil || *current.DeploymentJobID != activeJobID {
		t.Fatalf("env sync replaced active deployment: %+v", current)
	}
	queued, err := infrastructure.NewRedisServiceWithClient(workerClient).IsProjectQueued(project.ID)
	if err != nil || queued {
		t.Fatalf("env sync published while project busy: queued=%v err=%v", queued, err)
	}
	var task models.ProjectEnvSyncTask
	if err := db.Where("project_id = ?", project.ID).First(&task).Error; err != nil || task.DesiredGeneration != 3 || task.AcknowledgedGeneration != 0 {
		t.Fatalf("durable intent lost: task=%+v err=%v", task, err)
	}
}
