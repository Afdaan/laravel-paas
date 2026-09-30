package workers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestBuiltRedeployFencedAcrossAdminOverride(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	for _, origin := range []string{"rollback_fallback", "webhook_redeploy"} {
		for _, step := range []string{"before_hash", "before_promotion", "before_cleanup", "after_promotion_before_legacy_cleanup", "promotion_db_failure", "success"} {
			t.Run(origin+"/"+step, func(t *testing.T) {
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
				oldContainer, jobAID := "container-old", "job-A"
				oldHash, targetHash := strings.Repeat("1", 40), strings.Repeat("2", 40)
				project := models.Project{UserID: 1, Name: "Build fence", Subdomain: "build-fence", Status: models.StatusRunning,
					DeploymentStatus: models.DepStatusHealthchecking, DeploymentJobID: &jobAID,
					ContainerID: &oldContainer, LastCommitHash: oldHash}
				if err := db.Create(&project).Error; err != nil {
					t.Fatal(err)
				}
				repo := repositories.NewProjectRepository(db)
				manager := deployment.NewTransitionManager(db, redisService)
				service := projectServicePkg.NewProjectService(&config.Config{}, repo, nil, nil, nil, nil, redisService, manager)
				dockerLog := filepath.Join(t.TempDir(), "docker-calls")
				binDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_DOCKER_LOG\"\n"), 0755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
				t.Setenv("TEST_DOCKER_LOG", dockerLog)
				worker := &DeploymentWorker{projectService: service, projectRepo: repo, dockerService: &docker.DockerService{}}
				jobA := &infrastructure.DeploymentJob{ProjectID: project.ID, UserID: project.UserID, JobID: jobAID,
					Type: "redeploy", TargetCommitHash: targetHash, PreviousCommitHash: oldHash, EnqueuedAt: time.Now()}
				if jobA.Type != "redeploy" || jobA.TargetCommitHash == "" {
					t.Fatal("rebuild job lost target commit")
				}
				project.LastCommitHash = jobA.TargetCommitHash
				if step != "before_hash" {
					if err := repo.UpdateMetadataForJob(project.ID, jobAID, map[string]interface{}{"rollout_container_id": "container-A"}); err != nil {
						t.Fatal(err)
					}
				}
				if step == "promotion_db_failure" {
					if err := db.Callback().Update().Before("gorm:update").Register("fail_built_promotion", func(tx *gorm.DB) {
						tx.AddError(errors.New("promotion write failed"))
					}); err != nil {
						t.Fatal(err)
					}
					if err := worker.commitBuiltRollout(&project, jobAID, "container-A", map[string]interface{}{"framework": "Laravel"}); err == nil {
						t.Fatal("promotion must fail atomically")
					}
				} else if step == "success" || step == "after_promotion_before_legacy_cleanup" {
					if step == "after_promotion_before_legacy_cleanup" && !worker.transitionDeploymentState(&project, jobAID, models.DepStatusPromoting, 85, "promoting_release", "promoting") {
						t.Fatal("worker A could not enter promotion")
					}
					if err := worker.commitBuiltRollout(&project, jobAID, "container-A", map[string]interface{}{"framework": "Laravel"}); err != nil {
						t.Fatal(err)
					}
				}
				if step == "before_hash" || step == "before_promotion" || step == "before_cleanup" || step == "after_promotion_before_legacy_cleanup" {
					if _, err := redisService.AcquireDeploymentLock(project.ID, jobAID, time.Minute); err != nil {
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
					if step == "before_hash" {
						if worker.transitionDeploymentState(&project, jobAID, models.DepStatusBuilding, 35, "building_image", "starting build") {
							t.Fatal("superseded build accepted")
						}
					} else {
						if err := repo.UpdateMetadataForJob(project.ID, jobB.JobID, map[string]interface{}{"rollout_container_id": "container-B"}); err != nil {
							t.Fatal(err)
						}
						if step == "before_promotion" {
							if worker.transitionDeploymentState(&project, jobAID, models.DepStatusPromoting, 85, "promoting_release", "promoting") {
								t.Fatal("superseded promotion accepted")
							}
							if err := worker.commitBuiltRollout(&project, jobAID, "container-A", nil); !errors.Is(err, infrastructure.ErrStaleDeploymentOwner) {
								t.Fatalf("superseded worker promoted: %v", err)
							}
						}
						if step == "after_promotion_before_legacy_cleanup" {
							if worker.transitionDeploymentState(&project, jobAID, models.DepStatusCleanup, 95, "cleaning_legacy_instance", "cleaning") {
								t.Fatal("superseded worker entered legacy cleanup")
							}
						} else if !worker.cleanupRollout(&project, jobAID, "container-A", nil) {
							t.Fatal("stale worker could not clean its own container")
						}
					}
					queued, err := redisService.DequeueDeployment(time.Second)
					if err != nil || queued == nil || queued.JobID != jobB.JobID {
						t.Fatalf("admin job lost: job=%v err=%v", queued, err)
					}
					if err := redisService.AcknowledgeDeployment(queued); err != nil {
						t.Fatal(err)
					}
				}
				var current models.Project
				if err := db.First(&current, project.ID).Error; err != nil {
					t.Fatal(err)
				}
				if step == "success" || step == "after_promotion_before_legacy_cleanup" {
					if current.ContainerID == nil || *current.ContainerID != "container-A" || current.LastCommitHash != targetHash || current.Framework != "Laravel" {
						t.Fatalf("built release not committed atomically: %+v", current)
					}
				} else {
					if current.ContainerID == nil || *current.ContainerID != oldContainer || current.LastCommitHash != oldHash {
						t.Fatalf("stale/failed build changed running image: container=%v hash=%s", current.ContainerID, current.LastCommitHash)
					}
					if step == "promotion_db_failure" && (current.RolloutContainerID == nil || *current.RolloutContainerID != "container-A") {
						t.Fatalf("DB failure cleared A checkpoint: %v", current.RolloutContainerID)
					}
					if step != "promotion_db_failure" && step != "before_hash" && (current.RolloutContainerID == nil || *current.RolloutContainerID != "container-B") {
						t.Fatalf("stale cleanup cleared B checkpoint: %v", current.RolloutContainerID)
					}
				}
				if step != "promotion_db_failure" && step != "success" && (current.DeploymentJobID == nil || *current.DeploymentJobID != "admin-B") {
					t.Fatalf("admin lost ownership: %v", current.DeploymentJobID)
				}
				if step == "after_promotion_before_legacy_cleanup" && (current.RolloutContainerID == nil || *current.RolloutContainerID != "container-B") {
					t.Fatalf("admin B checkpoint lost: %v", current.RolloutContainerID)
				}
				if removed, err := os.ReadFile(dockerLog); err == nil && (strings.Contains(string(removed), oldContainer) || strings.Contains(string(removed), "container-B")) {
					t.Fatalf("stale cleanup removed active/B container: %s", removed)
				}
			})
		}
	}
}
