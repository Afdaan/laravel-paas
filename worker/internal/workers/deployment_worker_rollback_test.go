package workers

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestRollbackPromotionFencedAndAtomic(t *testing.T) {
	const previousHash = "1111111111111111111111111111111111111111"
	const targetHash = "2222222222222222222222222222222222222222"
	for _, scenario := range []string{"admin_before_start", "admin_before_promotion", "write_failure", "success"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.Project{}, &models.DeploymentEvent{}, &models.User{}); err != nil {
				t.Fatal(err)
			}
			jobID, oldContainer := "rollback-A", "container-old"
			project := models.Project{UserID: 1, Name: "Rollback race", Subdomain: "rollback-race", Status: models.StatusRunning,
				DeploymentStatus: models.DepStatusPreparing, DeploymentJobID: &jobID, ContainerID: &oldContainer, LastCommitHash: previousHash}
			if err := db.Create(&project).Error; err != nil {
				t.Fatal(err)
			}
			repo := repositories.NewProjectRepository(db)
			manager := deployment.NewTransitionManager(db, nil)
			service := projectServicePkg.NewProjectService(&config.Config{}, repo, nil, nil, nil, nil, nil, manager)
			worker := &DeploymentWorker{projectService: service}
			if scenario == "admin_before_start" {
				if _, err := manager.RequeueDeployment(context.Background(), project.ID, "admin-B", "admin override"); err != nil {
					t.Fatal(err)
				}
				if worker.transitionDeploymentState(&project, jobID, models.DepStatusPreparing, 20, "rollback_started", "starting") {
					t.Fatal("superseded worker accepted before image start")
				}
			}
			if scenario != "admin_before_start" {
				if err := repo.UpdateMetadataForJob(project.ID, jobID, map[string]interface{}{"rollout_container_id": "container-new"}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "admin_before_promotion" {
				if _, err := manager.RequeueDeployment(context.Background(), project.ID, "admin-B", "admin override"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "write_failure" {
				if err := db.Callback().Update().Before("gorm:update").Register("rollback_promotion_fault", func(tx *gorm.DB) {
					tx.AddError(errors.New("simulated DB write failure"))
				}); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "admin_before_start" {
				rollbackProject := project
				rollbackProject.LastCommitHash = targetHash
				err = service.PromoteRolloutForJob(&rollbackProject, jobID, "container-new", nil)
				if scenario == "admin_before_promotion" && !errors.Is(err, infrastructure.ErrStaleDeploymentOwner) {
					t.Fatalf("expected stale owner, got %v", err)
				}
				if scenario == "write_failure" && err == nil {
					t.Fatal("expected DB failure")
				}
				if scenario == "success" && err != nil {
					t.Fatal(err)
				}
			}
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "success" {
				if current.ContainerID == nil || *current.ContainerID != "container-new" || current.LastCommitHash != targetHash {
					t.Fatalf("promotion not atomic: container=%v hash=%s", current.ContainerID, current.LastCommitHash)
				}
			} else if current.ContainerID == nil || *current.ContainerID != oldContainer || current.LastCommitHash != previousHash {
				t.Fatalf("failed/stale promotion mutated runtime: container=%v hash=%s", current.ContainerID, current.LastCommitHash)
			}
			if scenario == "admin_before_promotion" || scenario == "admin_before_start" {
				if current.DeploymentJobID == nil || *current.DeploymentJobID != "admin-B" {
					t.Fatalf("admin lost ownership: %v", current.DeploymentJobID)
				}
			}
		})
	}
}

func TestRollbackWithoutRunningContainerCannotChangeCommit(t *testing.T) {
	service := projectServicePkg.NewProjectService(&config.Config{}, nil, nil, nil, nil, nil, nil, nil)
	if err := service.RecreateProjectZeroDowntime(context.Background(), &models.Project{}, nil, "rollback-A"); err == nil {
		t.Fatal("rollback without running image must not report success")
	}
}

func TestRollbackAdminPublishBeforePromotion(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	redisService := infrastructure.NewRedisServiceWithClient(client)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Project{}, &models.DeploymentEvent{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	jobA, oldContainer, previousHash := "rollback-A", "container-old", "1111111111111111111111111111111111111111"
	project := models.Project{UserID: 1, Name: "Rollback publish race", Subdomain: "rollback-publish-race",
		Status: models.StatusRunning, DeploymentStatus: models.DepStatusPreparing,
		DeploymentJobID: &jobA, ContainerID: &oldContainer, LastCommitHash: previousHash}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	repo := repositories.NewProjectRepository(db)
	manager := deployment.NewTransitionManager(db, redisService)
	service := projectServicePkg.NewProjectService(&config.Config{}, repo, nil, nil, nil, nil, redisService, manager)
	worker := &DeploymentWorker{projectService: service}
	if !worker.transitionDeploymentState(&project, jobA, models.DepStatusPreparing, 20, "rollback_started", "starting") {
		t.Fatal("worker A failed before admin override")
	}
	if err := repo.UpdateMetadataForJob(project.ID, jobA, map[string]interface{}{"rollout_container_id": "container-A"}); err != nil {
		t.Fatal(err)
	}
	token, err := redisService.ReserveAdminRequeue(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	jobB := &infrastructure.DeploymentJob{ProjectID: project.ID, UserID: project.UserID, JobID: "admin-B", Type: "redeploy", EnqueuedAt: time.Now()}
	if _, err := manager.RequeueDeployment(context.Background(), project.ID, jobB.JobID, "admin override"); err != nil {
		t.Fatal(err)
	}
	if err := redisService.EnqueueReplacingDeploymentJob(jobB, token); err != nil {
		t.Fatal(err)
	}
	project.LastCommitHash = "2222222222222222222222222222222222222222"
	if err := service.PromoteRolloutForJob(&project, jobA, "container-A", nil); !errors.Is(err, infrastructure.ErrStaleDeploymentOwner) {
		t.Fatalf("worker A promoted after B publish: %v", err)
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.ContainerID == nil || *current.ContainerID != oldContainer || current.LastCommitHash != previousHash || current.DeploymentJobID == nil || *current.DeploymentJobID != jobB.JobID {
		t.Fatalf("admin B lost runtime/ownership: container=%v hash=%s job=%v", current.ContainerID, current.LastCommitHash, current.DeploymentJobID)
	}
	queued, err := redisService.DequeueDeployment(time.Second)
	if err != nil || queued == nil || queued.JobID != jobB.JobID {
		t.Fatalf("admin B lost published job: job=%v err=%v", queued, err)
	}
	if err := redisService.AcknowledgeDeployment(queued); err != nil {
		t.Fatal(err)
	}
}
