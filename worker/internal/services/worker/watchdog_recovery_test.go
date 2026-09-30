package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	projectservice "github.com/laravel-paas/worker/internal/services/project"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func extractJobFromCmd(cmd redis.Cmder) *infrastructure.DeploymentJob {
	for _, arg := range cmd.Args() {
		var job infrastructure.DeploymentJob
		if err := json.Unmarshal([]byte(fmt.Sprint(arg)), &job); err == nil && job.JobID != "" {
			return &job
		}
	}
	return nil
}

type recoveryPublishHook struct {
	onPublish func(string)
}

func (hook recoveryPublishHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (hook recoveryPublishHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (hook recoveryPublishHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		isPublish := (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:stats")
		err := next(ctx, cmd)
		if err == nil && isPublish {
			if job := extractJobFromCmd(cmd); job != nil && hook.onPublish != nil {
				hook.onPublish(job.JobID)
			}
		}
		return err
	}
}

func recoveryFixture(t *testing.T) (*CentralWatchdog, models.Project, redismock.ClientMock, *gorm.DB, *redis.Client) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.CustomDomain{}, &models.DatabaseInstance{}, &models.DeploymentEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{ID: 1, Email: "recovery@example.com"}).Error; err != nil {
		t.Fatal(err)
	}
	oldJobID := "old-job"
	project := models.Project{
		ID:               uint(time.Now().UnixNano() % 1000000000),
		UID:              fmt.Sprintf("orphan-%d", time.Now().UnixNano()),
		Name:             "orphan",
		Subdomain:        fmt.Sprintf("orphan-%d", time.Now().UnixNano()),
		DeploymentStatus: models.DepStatusBuilding,
		DeploymentJobID:  &oldJobID,
		UserID:           1,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	client, mock := redismock.NewClientMock()
	redisService := infrastructure.NewRedisServiceWithClient(client)
	repo := repositories.NewProjectRepository(db)
	cfg := &config.Config{ProjectsPath: t.TempDir()}
	manager := deployment.NewTransitionManager(db, nil)
	service := projectservice.NewProjectService(cfg, repo, nil, nil, nil, nil, redisService, manager)
	return &CentralWatchdog{cfg: cfg, projectRepo: repo, redisService: redisService, projectService: service}, project, mock, db, client
}

func TestRecoverySkipsLiveLockAndClaimedJob(t *testing.T) {
	for _, owner := range []string{"live lock", "claimed waiting for slot"} {
		t.Run(owner, func(t *testing.T) {
			watchdog, project, mock, db, _ := recoveryFixture(t)
			mock.Regexp().ExpectEvalSha(".*", []string{
				"deployment:queue", "deployment:delayed_queue", "deployment:processing_queue", fmt.Sprintf("deployment:lock:%d", project.ID),
			}, fmt.Sprint(project.ID), ".*", "120000").SetVal(int64(0))
			watchdog.recoverOrphanedBuilds()
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.DeploymentStatus != models.DepStatusBuilding || *current.DeploymentJobID != "old-job" {
				t.Fatalf("live owner was requeued: status=%s job=%v", current.DeploymentStatus, current.DeploymentJobID)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecoveryPublishesAfterQueuedStateAndDoesNotRewindPickup(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)
	manager := deployment.NewTransitionManager(db, nil)
	client.AddHook(recoveryPublishHook{onPublish: func(jobID string) {
		var queued models.Project
		if err := db.First(&queued, project.ID).Error; err != nil {
			t.Fatal(err)
		}
		if queued.DeploymentStatus != models.DepStatusQueued || queued.DeploymentJobID == nil || *queued.DeploymentJobID != jobID {
			t.Fatalf("job published before queued identity: status=%s job=%v", queued.DeploymentStatus, queued.DeploymentJobID)
		}
		if _, err := manager.TransitionState(context.Background(), project.ID, jobID, models.DepStatusPreparing, 10, "picked_up", "worker picked up job"); err != nil {
			t.Fatal(err)
		}
	}})
	watchdog.recoverOrphanedBuilds()
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusPreparing || current.DeploymentStartedAt == nil {
		t.Fatalf("pickup rewound after recovery: status=%s started=%v", current.DeploymentStatus, current.DeploymentStartedAt)
	}
	if current.DeploymentJobID != nil {
		if err := watchdog.redisService.RemoveDeploymentJob(*current.DeploymentJobID); err != nil {
			t.Fatal(err)
		}
	}
}

type recoveryFaultHook struct {
	onPublish   func(string)
	injectErr   error
	injectOnCmd func(cmd redis.Cmder) bool
	skipServer  bool
	inProgress  *bool
}

func (hook recoveryFaultHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (hook recoveryFaultHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (hook recoveryFaultHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if hook.inProgress != nil && *hook.inProgress {
			return next(ctx, cmd)
		}
		isPublish := (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:stats")
		if hook.injectOnCmd != nil && hook.injectOnCmd(cmd) {
			if hook.skipServer {
				return hook.injectErr
			}
			_ = next(ctx, cmd)
			return hook.injectErr
		}
		if isPublish {
			if hook.skipServer {
				return hook.injectErr
			}
			if hook.inProgress != nil {
				*hook.inProgress = true
				defer func() { *hook.inProgress = false }()
			}
			err := next(ctx, cmd)
			if err == nil {
				if job := extractJobFromCmd(cmd); job != nil && hook.onPublish != nil {
					hook.onPublish(job.JobID)
				}
				if hook.injectErr != nil {
					return hook.injectErr
				}
			}
			return err
		}
		return next(ctx, cmd)
	}
}

func TestRecoverySucceedsWhenPublishSucceedsWithClientErrorAndPickup(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)
	manager := deployment.NewTransitionManager(db, nil)

	var publishedJobID string
	client.AddHook(recoveryFaultHook{
		injectErr: fmt.Errorf("transport timeout after eval"),
		onPublish: func(jobID string) {
			publishedJobID = jobID
			if _, err := manager.TransitionState(context.Background(), project.ID, jobID, models.DepStatusPreparing, 10, "picked_up", "worker picked up job"); err != nil {
				t.Fatal(err)
			}
		},
	})

	watchdog.recoverOrphanedBuilds()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify deployment state was NOT overwritten to failed; preparing state is preserved!
	if current.DeploymentStatus != models.DepStatusPreparing {
		t.Fatalf("expected deployment status to stay preparing, got %s", current.DeploymentStatus)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != publishedJobID {
		t.Fatalf("expected job ID %s, got %v", publishedJobID, current.DeploymentJobID)
	}
	if err := watchdog.redisService.RemoveDeploymentJob(publishedJobID); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryMarksFailedOnGenuineEnqueueFailure(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)

	client.AddHook(recoveryFaultHook{
		skipServer: true,
		injectErr:  fmt.Errorf("redis connection refused"),
	})

	watchdog.recoverOrphanedBuilds()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify genuine failure marks failed
	if current.DeploymentStatus != models.DepStatusFailed {
		t.Fatalf("expected deployment status to be failed, got %s", current.DeploymentStatus)
	}
}

func TestRecoveryPreservesQueuedWhenReconciliationFails(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)

	client.AddHook(recoveryFaultHook{
		injectOnCmd: func(cmd redis.Cmder) bool {
			// Fail both the enqueue eval and any subsequent check script, but allow initial reservation
			args := fmt.Sprint(cmd.Args())
			return (cmd.Name() == "evalsha" || cmd.Name() == "eval") && !strings.Contains(args, "deployment:lock")
		},
		skipServer: true,
		injectErr:  fmt.Errorf("simulated total redis partition"),
	})

	watchdog.recoverOrphanedBuilds()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify state is PRESERVED as queued, NOT failed
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to remain queued, got %s", current.DeploymentStatus)
	}
}

func TestRecoveryInterleavedReplacesDoNotOverwriteNewerJob(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)
	manager := deployment.NewTransitionManager(db, nil)

	jobBID := fmt.Sprintf("job-B-%d", time.Now().UnixNano())

	// When Watchdog's publish hook fires for its recovery job (Job A):
	// 1. An Admin concurrently intervenes: sets project to queued with Job B, and enqueues Job B into Redis.
	// 2. Watchdog's publish returns a transport error.
	var jobAID string
	var inProgress bool
	client.AddHook(recoveryFaultHook{
		inProgress: &inProgress,
		onPublish: func(publishedAID string) {
			jobAID = publishedAID

			// Concurrent Admin operation:
			if _, err := manager.TransitionState(context.Background(), project.ID, jobBID, models.DepStatusQueued, 0, "admin_override", "Admin override"); err != nil {
				t.Fatalf("admin transition: %v", err)
			}
			jobB := &infrastructure.DeploymentJob{
				ProjectID: project.ID, UserID: project.UserID, Type: "redeploy", JobID: jobBID, EnqueuedAt: time.Now(),
			}
			// EnqueueReplacingDeploymentJob replaces Job A in Redis!
			if err := watchdog.redisService.EnqueueReplacingDeploymentJob(jobB); err != nil {
				t.Fatalf("admin enqueue: %v", err)
			}
		},
		injectErr: fmt.Errorf("transport timeout on watchdog publish"),
	})

	watchdog.recoverOrphanedBuilds()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}

	// CRITICAL ASSERTION: Watchdog's stale compensation must NOT have overwritten Job B to failed!
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobBID {
		t.Fatalf("expected project to remain owned by Job B (%s), but got %v", jobBID, current.DeploymentJobID)
	}
	if current.DeploymentStatus == models.DepStatusFailed {
		t.Fatalf("Watchdog's stale compensation overwrote Job B to failed!")
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to be queued for Job B, got %s", current.DeploymentStatus)
	}

	// Verify Job B is in Redis
	hasJobB, err := watchdog.redisService.HasDeploymentJob(jobBID)
	if err != nil || !hasJobB {
		t.Fatalf("Job B should be present in Redis: has=%v err=%v", hasJobB, err)
	}
	// Verify Job A is not in Redis
	hasJobA, _ := watchdog.redisService.HasDeploymentJob(jobAID)
	if hasJobA {
		t.Fatalf("Job A should have been replaced in Redis")
	}

	_ = watchdog.redisService.RemoveDeploymentJob(jobBID)
}

func TestWatchdogRecoveryAdminInterleavedOverride(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)
	manager := deployment.NewTransitionManager(db, nil)

	jobBID := fmt.Sprintf("job-admin-%d", time.Now().UnixNano())

	// Interleaving hook:
	// When Watchdog is about to publish its recovery job (after reading DB and transitioning),
	// Admin intervenes: reserves admin requeue (overwriting the lock token in Redis),
	// transitions DB to queued with Job B, and publishes Job B to Redis.
	var inProgress bool
	client.AddHook(recoveryFaultHook{
		inProgress: &inProgress,
		onPublish: func(watchdogJobID string) {
			// Admin intervenes concurrently:
			tokenAdmin, err := watchdog.redisService.ReserveAdminRequeue(project.ID)
			if err != nil {
				t.Fatalf("admin reserve: %v", err)
			}
			if _, err := manager.RequeueDeployment(context.Background(), project.ID, jobBID, "Admin manual requeue"); err != nil {
				t.Fatalf("admin requeue DB: %v", err)
			}
			jobB := &infrastructure.DeploymentJob{
				ProjectID: project.ID, UserID: project.UserID, Type: "redeploy", JobID: jobBID, EnqueuedAt: time.Now(),
			}
			if err := watchdog.redisService.EnqueueReplacingDeploymentJob(jobB, tokenAdmin); err != nil {
				t.Fatalf("admin enqueue: %v", err)
			}
		},
		// NO transport error injected! Watchdog's publish proceeds to Redis with its old token,
		// and MUST be rejected by the Redis Lua script because Admin superseded the lock token!
	})

	watchdog.recoverOrphanedBuilds()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}

	// CRITICAL ASSERTION: Watchdog MUST NOT have overwritten Admin's Job B!
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobBID {
		t.Fatalf("expected project in DB to remain owned by Job B (%s), but got %v", jobBID, current.DeploymentJobID)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to be queued for Job B, got %s", current.DeploymentStatus)
	}
	if current.Status == models.StatusFailed {
		t.Fatalf("project was marked failed unexpectedly")
	}

	// Verify Job B is in Redis
	hasJobB, err := watchdog.redisService.HasDeploymentJob(jobBID)
	if err != nil || !hasJobB {
		t.Fatalf("Job B should be present in Redis: has=%v err=%v", hasJobB, err)
	}

	_ = watchdog.redisService.RemoveDeploymentJob(jobBID)
}

func TestWatchdogDoesNotFailJobWithReservationLock(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)

	jobID := fmt.Sprintf("job-res-%d", time.Now().UnixNano())
	if err := db.Model(&project).Updates(map[string]interface{}{
		"deployment_status": models.DepStatusQueued,
		"deployment_job_id": jobID,
		"status":            models.StatusQueued,
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Reserve admin requeue lock (which has empty DeploymentID and WorkerID="admin_requeue")
	token, err := watchdog.redisService.ReserveAdminRequeue(project.ID)
	if err != nil || token == "" {
		t.Fatalf("failed to reserve admin requeue: token=%s err=%v", token, err)
	}
	t.Cleanup(func() { _ = watchdog.redisService.ReleaseDeploymentLock(project.ID, token) })

	// Run stale build watchdog check
	watchdog.checkStaleBuilds()

	// Verify project in DB was NOT marked failed
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected project deployment status to remain queued, got %s", current.DeploymentStatus)
	}
	if current.Status == models.StatusFailed {
		t.Fatalf("project was marked failed unexpectedly while holding reservation lock")
	}
}

func TestWatchdogDoesNotFailJobClaimedInProcessingQueue(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)

	jobID := fmt.Sprintf("job-proc-%d", time.Now().UnixNano())
	staleTime := time.Now().Add(-20 * time.Minute)
	if err := db.Model(&project).Updates(map[string]interface{}{
		"deployment_status":       models.DepStatusQueued,
		"deployment_job_id":       jobID,
		"status":                  models.StatusQueued,
		"deployment_heartbeat_at": staleTime,
		"updated_at":              staleTime,
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := &infrastructure.DeploymentJob{
		ProjectID:  project.ID,
		UserID:     project.UserID,
		Type:       "redeploy",
		JobID:      jobID,
		EnqueuedAt: time.Now(),
	}
	if err := watchdog.redisService.EnqueueDeploymentJob(job); err != nil {
		t.Fatal(err)
	}

	// Dequeue job to move it into deployment:processing_queue (waiting for semaphore slot)
	claimed, err := watchdog.redisService.DequeueDeployment(time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}
	t.Cleanup(func() { _ = watchdog.redisService.AcknowledgeDeployment(claimed) })

	// HasDeploymentJob must be true
	hasJob, err := watchdog.redisService.HasDeploymentJob(jobID)
	if err != nil || !hasJob {
		t.Fatalf("expected HasDeploymentJob to be true for claimed job: %v", err)
	}

	// Run stale build watchdog check
	watchdog.checkStaleBuilds()

	// Verify project in DB was NOT marked failed
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected project deployment status to remain queued, got %s", current.DeploymentStatus)
	}
	if current.Status == models.StatusFailed {
		t.Fatalf("project was marked failed unexpectedly while waiting in processing queue")
	}
}

func TestWatchdogReapsStaleInFlightBuildEvenIfInProcessingQueue(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	watchdog, project, _, db, _ := recoveryFixture(t)
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	watchdog.redisService = infrastructure.NewRedisServiceWithClient(client)

	jobID := fmt.Sprintf("job-inflight-%d", time.Now().UnixNano())
	staleHeartbeat := time.Now().Add(-5 * time.Minute)
	if err := db.Model(&project).Updates(map[string]interface{}{
		"deployment_status":       models.DepStatusBuilding,
		"deployment_job_id":       jobID,
		"status":                  models.StatusBuilding,
		"deployment_heartbeat_at": staleHeartbeat,
		"updated_at":              staleHeartbeat,
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := &infrastructure.DeploymentJob{
		ProjectID:  project.ID,
		UserID:     project.UserID,
		Type:       "redeploy",
		JobID:      jobID,
		EnqueuedAt: time.Now(),
	}
	if err := watchdog.redisService.EnqueueDeploymentJob(job); err != nil {
		t.Fatal(err)
	}

	// Dequeue job to move it into deployment:processing_queue (in-flight worker execution)
	claimed, err := watchdog.redisService.DequeueDeployment(time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("failed to dequeue job: %v", err)
	}
	t.Cleanup(func() { _ = watchdog.redisService.AcknowledgeDeployment(claimed) })

	// HasDeploymentJob is true because the payload is in deployment:processing_queue
	hasJob, err := watchdog.redisService.HasDeploymentJob(jobID)
	if err != nil || !hasJob {
		t.Fatalf("expected HasDeploymentJob to be true: %v", err)
	}

	// Acquire lock with an expired lease
	token, lockErr := watchdog.redisService.AcquireDeploymentLock(project.ID, jobID, 2*time.Minute)
	if lockErr != nil || token == "" {
		t.Fatalf("failed to acquire deployment lock: %v", lockErr)
	}
	t.Cleanup(func() { _ = watchdog.redisService.ReleaseDeploymentLock(project.ID, token) })

	// Set lease heartbeat in the past (> 30s ago)
	workerID := "worker-stale"
	staleLeaseTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	leaseMeta := infrastructure.DeploymentLeaseMetadata{
		JobID:         jobID,
		ProjectID:     project.ID,
		WorkerID:      workerID,
		StartedAt:     staleLeaseTime,
		LastHeartbeat: staleLeaseTime,
	}
	leaseData, _ := json.Marshal(leaseMeta)
	client.Set(context.Background(), fmt.Sprintf("deployment:lease:%s", jobID), string(leaseData), 0)

	// Run stale build watchdog check
	watchdog.checkStaleBuilds()

	// Verify project in DB was reaped and marked failed because lease expired
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusFailed {
		t.Fatalf("expected project deployment status to be failed, got %s", current.DeploymentStatus)
	}
	if current.Status != models.StatusFailed {
		t.Fatalf("expected project status to be failed, got %s", current.Status)
	}
	if current.ErrorLog == nil || !strings.Contains(*current.ErrorLog, "lease heartbeat expired") {
		t.Fatalf("expected error log explaining expired lease, got %v", current.ErrorLog)
	}
}
