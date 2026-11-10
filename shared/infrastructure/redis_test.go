package infrastructure

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
)

func TestReserveOrphanRecoveryWithRedis(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	service := NewRedisServiceWithClient(client)
	_ = client.FlushDB(service.ctx).Err()
	projectID := uint(time.Now().UnixNano() % 1000000000)

	lock, err := service.AcquireDeploymentLock(projectID, "live-job", time.Minute)
	if err != nil || lock == "" {
		t.Fatalf("acquire live lock: token=%q err=%v", lock, err)
	}
	if token, err := service.ReserveOrphanRecovery(projectID); err != nil || token != "" {
		t.Fatalf("live lock must block recovery: token=%q err=%v", token, err)
	}
	if err := service.ReleaseDeploymentLock(projectID, lock); err != nil {
		t.Fatal(err)
	}

	job := &DeploymentJob{ProjectID: projectID, UserID: 1, Type: "redeploy", JobID: fmt.Sprintf("claimed-%d", projectID), EnqueuedAt: time.Now()}
	if err := service.EnqueueDeploymentJob(job); err != nil {
		t.Fatal(err)
	}
	claimed, err := service.DequeueDeployment(time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("claim deployment: job=%v err=%v", claimed, err)
	}
	t.Cleanup(func() { _ = service.AcknowledgeDeployment(claimed) })
	if token, err := service.ReserveOrphanRecovery(projectID); err != nil || token != "" {
		t.Fatalf("claimed job waiting for slot must block recovery: token=%q err=%v", token, err)
	}
	if err := service.AcknowledgeDeployment(claimed); err != nil {
		t.Fatal(err)
	}
	token, err := service.ReserveOrphanRecovery(projectID)
	if err != nil || token == "" {
		t.Fatalf("orphan must reserve recovery: token=%q err=%v", token, err)
	}
	if competing, err := service.AcquireDeploymentLock(projectID, "new-job", time.Minute); err != nil || competing != "" {
		t.Fatalf("recovery lock must fence new worker: token=%q err=%v", competing, err)
	}
	if err := service.ReleaseDeploymentLock(projectID, token); err != nil {
		t.Fatal(err)
	}
}

func TestPendingEnvRefresh(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	expectedKey := "project:pending_env_refresh:42"

	// 1. Test SetPendingEnvRefresh
	mock.ExpectEvalSha(setPendingEnvRefreshScript.Hash(), []string{expectedKey, "project:pending_env_refresh_sequence:42"}).SetVal(int64(1))
	err := redisService.SetPendingEnvRefresh(projectID)
	if err != nil {
		t.Fatalf("SetPendingEnvRefresh failed: %v", err)
	}

	// 2. Test HasPendingEnvRefresh (exists/true)
	mock.ExpectGet(expectedKey).SetVal("1")
	hasPending, err := redisService.HasPendingEnvRefresh(projectID)
	if err != nil {
		t.Fatalf("HasPendingEnvRefresh failed: %v", err)
	}
	if !hasPending {
		t.Error("Expected HasPendingEnvRefresh to return true, got false")
	}

	// 3. Test HasPendingEnvRefresh (not exists)
	mock.ExpectGet(expectedKey).RedisNil()
	hasPending, err = redisService.HasPendingEnvRefresh(projectID)
	if err != nil {
		t.Fatalf("HasPendingEnvRefresh failed on nil: %v", err)
	}
	if hasPending {
		t.Error("Expected HasPendingEnvRefresh to return false on nil, got true")
	}

	// 4. Test ClearPendingEnvRefresh
	mock.ExpectEvalSha(clearPendingEnvRefreshScript.Hash(), []string{expectedKey}, int64(0)).SetVal(int64(1))
	cleared, err := redisService.ClearPendingEnvRefresh(projectID)
	if err != nil {
		t.Fatalf("ClearPendingEnvRefresh failed: %v", err)
	}
	if !cleared {
		t.Error("Expected ClearPendingEnvRefresh to return true, got false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestPendingEnvRefresh_MonotonicGenerationAndClearInterleaving(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	service := NewRedisServiceWithClient(client)
	_ = client.FlushDB(service.ctx).Err()
	projectID := uint(time.Now().UnixNano() % 1000000000)

	// Step 1: Initial env update sets marker -> generation 1
	if err := service.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatalf("set initial pending env refresh: %v", err)
	}
	gen1, err := service.GetPendingEnvRefresh(projectID)
	if err != nil || gen1 != 1 {
		t.Fatalf("expected generation 1, got gen=%d err=%v", gen1, err)
	}

	// Step 2: Second concurrent env update occurs while first is being processed -> generation 2
	if err := service.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatalf("set concurrent pending env refresh: %v", err)
	}
	gen2, err := service.GetPendingEnvRefresh(projectID)
	if err != nil || gen2 != 2 {
		t.Fatalf("expected generation 2, got gen=%d err=%v", gen2, err)
	}

	// Step 3: First worker tries to clear up to generation 1
	clearedOld, err := service.ClearPendingEnvRefresh(projectID, gen1)
	if err != nil {
		t.Fatalf("clear pending env refresh for gen1: %v", err)
	}
	if clearedOld {
		t.Fatalf("expected ClearPendingEnvRefresh(gen1=1) to return false because current is gen2=2")
	}

	// Step 4: Marker MUST still be present for generation 2
	hasPending, err := service.HasPendingEnvRefresh(projectID)
	if err != nil || !hasPending {
		t.Fatalf("expected pending marker to persist for gen2, hasPending=%v err=%v", hasPending, err)
	}

	// Step 5: Second worker consumes and clears up to generation 2
	clearedNew, err := service.ClearPendingEnvRefresh(projectID, gen2)
	if err != nil {
		t.Fatalf("clear pending env refresh for gen2: %v", err)
	}
	if !clearedNew {
		t.Fatalf("expected ClearPendingEnvRefresh(gen2=2) to return true")
	}

	// Step 6: Marker should now be gone
	hasPendingFinal, err := service.HasPendingEnvRefresh(projectID)
	if err != nil || hasPendingFinal {
		t.Fatalf("expected pending marker to be fully cleared, hasPending=%v err=%v", hasPendingFinal, err)
	}

	if err := service.SetPendingEnvRefresh(projectID); err != nil {
		t.Fatal(err)
	}
	gen3, err := service.GetPendingEnvRefresh(projectID)
	if err != nil || gen3 <= gen2 {
		t.Fatalf("generation reset after clear: previous=%d current=%d err=%v", gen2, gen3, err)
	}
	staleClear, err := service.ClearPendingEnvRefresh(projectID, gen2)
	if err != nil || staleClear {
		t.Fatalf("stale worker cleared new marker: cleared=%v err=%v", staleClear, err)
	}
	if current, err := service.GetPendingEnvRefresh(projectID); err != nil || current != gen3 {
		t.Fatalf("new marker lost: current=%d want=%d err=%v", current, gen3, err)
	}
}

func TestEnqueueEnvUpdateIfQuiet_BusyLocked(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Lock exists -> busy
	mock.ExpectExists(expectedLockKey).SetVal(int64(1))

	jobID, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if jobID != "" {
		t.Errorf("Expected empty jobID for locked project, got %q", jobID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestEnqueueEnvUpdateIfQuiet_BusyQueued(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Lock does not exist, but the atomic queue index reports pending work.
	mock.ExpectExists(expectedLockKey).SetVal(int64(0))
	mock.ExpectEvalSha(isProjectQueuedScript.Hash(), []string{
		deploymentQueueKey,
		deploymentDelayedQueueKey,
		deploymentQueuedProjectsKey,
	}, "42").SetVal(int64(1))

	jobID, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if jobID != "" {
		t.Errorf("Expected empty jobID for project with queued job, got %q", jobID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestEnqueueEnvUpdateIfQuiet_QuietSuccess(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Lock does not exist and the atomic queue index is empty, so enqueue.
	mock.ExpectExists(expectedLockKey).SetVal(int64(0))
	mock.ExpectEvalSha(isProjectQueuedScript.Hash(), []string{
		deploymentQueueKey,
		deploymentDelayedQueueKey,
		deploymentQueuedProjectsKey,
	}, "42").SetVal(int64(0))
	mock.Regexp().ExpectEvalSha(enqueueDeploymentJobScript.Hash(), []string{
		deploymentQueueKey,
		deploymentQueuedProjectsKey,
		deploymentStatsKey,
	}, ".*", "42").SetVal(int64(1))

	jobID, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if jobID == "" {
		t.Error("Expected non-empty jobID for quiet project")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestEnqueueEnvUpdateIfQuiet_Error(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Exists fails with Redis error
	mock.ExpectExists(expectedLockKey).SetErr(fmt.Errorf("connection refused"))

	_, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, userID)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("Expected connection refused error, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestEnqueueEnvUpdateIfQuiet_QueueIndexError(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Exists is fine, but the atomic membership lookup fails.
	mock.ExpectExists(expectedLockKey).SetVal(int64(0))
	mock.ExpectEvalSha(isProjectQueuedScript.Hash(), []string{
		deploymentQueueKey,
		deploymentDelayedQueueKey,
		deploymentQueuedProjectsKey,
	}, "42").SetErr(fmt.Errorf("read error"))

	_, err := redisService.EnqueueEnvUpdateIfQuiet(projectID, userID)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("Expected read error, got: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestIsProjectQueuedUsesAtomicIndexedLookup(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	mock.ExpectEvalSha(isProjectQueuedScript.Hash(), []string{
		deploymentQueueKey,
		deploymentDelayedQueueKey,
		deploymentQueuedProjectsKey,
	}, "42").SetVal(int64(1))

	queued, err := redisService.IsProjectQueued(42)
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("expected project to be queued")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDelayedMigrationKeepsQueuedMembershipIndex(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	mock.MatchExpectationsInOrder(false)
	redisService := NewRedisServiceWithClient(redisClient)

	mock.Regexp().ExpectEvalSha(migrateDelayedJobsScript.Hash(), []string{
		deploymentDelayedQueueKey,
		deploymentQueueKey,
	}, ".*").SetVal(int64(1))
	mock.ExpectEvalSha(isProjectQueuedScript.Hash(), []string{
		deploymentQueueKey,
		deploymentDelayedQueueKey,
		deploymentQueuedProjectsKey,
	}, "42").SetVal(int64(1))

	errCh := make(chan error, 2)
	queuedCh := make(chan bool, 1)
	go func() {
		_, err := redisService.MigrateDelayedJobs()
		errCh <- err
	}()
	go func() {
		queued, err := redisService.IsProjectQueued(42)
		queuedCh <- queued
		errCh <- err
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if !<-queuedCh {
		t.Fatal("delayed-to-ready migration must preserve queued membership")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRateLimitScriptAllowedAndExceeded(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	key := "test:rate:limit:key"
	limit := 10
	duration := 60 * time.Second
	durationMs := duration.Milliseconds()

	// 1. Allowed request
	mock.ExpectEvalSha(rateLimitScript.Hash(), []string{key}, durationMs, limit).
		SetVal([]interface{}{int64(1), int64(0)})

	allowed, ttl, err := redisService.RateLimit(key, limit, duration)
	if err != nil {
		t.Fatalf("RateLimit failed: %v", err)
	}
	if !allowed || ttl != 0 {
		t.Errorf("Expected allowed=true and ttl=0, got allowed=%v, ttl=%v", allowed, ttl)
	}

	// 2. Exceeded request (self-healed with remaining TTL in ms)
	mock.ExpectEvalSha(rateLimitScript.Hash(), []string{key}, durationMs, limit).
		SetVal([]interface{}{int64(0), int64(45000)})

	allowed, ttl, err = redisService.RateLimit(key, limit, duration)
	if err != nil {
		t.Fatalf("RateLimit failed: %v", err)
	}
	if allowed {
		t.Error("Expected allowed=false, got true")
	}
	if ttl != 45*time.Second {
		t.Errorf("Expected ttl=45s, got %v", ttl)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectations were not met: %v", err)
	}
}

func TestHasDeploymentJobAcrossQueues(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	service := NewRedisServiceWithClient(client)
	_ = client.FlushDB(service.ctx).Err()

	projectID := uint(time.Now().UnixNano() % 1000000000)
	readyJobID := fmt.Sprintf("job-ready-%d", projectID)
	delayedJobID := fmt.Sprintf("job-delayed-%d", projectID)

	// Non-existent job
	exists, err := service.HasDeploymentJob("non-existent-job")
	if err != nil {
		t.Fatalf("check non-existent job: %v", err)
	}
	if exists {
		t.Fatal("expected non-existent job to return false")
	}

	// 1. Ready queue
	readyJob := &DeploymentJob{ProjectID: projectID, UserID: 1, Type: "redeploy", JobID: readyJobID, EnqueuedAt: time.Now()}
	if err := service.EnqueueReplacingDeploymentJob(readyJob); err != nil {
		t.Fatalf("enqueue ready job: %v", err)
	}
	t.Cleanup(func() { _ = service.RemoveDeploymentJob(readyJobID) })

	exists, err = service.HasDeploymentJob(readyJobID)
	if err != nil {
		t.Fatalf("check ready job: %v", err)
	}
	if !exists {
		t.Fatal("expected ready job to exist")
	}

	// 2. Delayed queue
	delayedJob := &DeploymentJob{ProjectID: projectID + 1, UserID: 1, Type: "redeploy", JobID: delayedJobID, EnqueuedAt: time.Now()}
	if err := service.EnqueueDelayedDeploymentJob(delayedJob, time.Hour); err != nil {
		t.Fatalf("enqueue delayed job: %v", err)
	}
	t.Cleanup(func() { _ = service.RemoveDeploymentJob(delayedJobID) })

	exists, err = service.HasDeploymentJob(delayedJobID)
	if err != nil {
		t.Fatalf("check delayed job: %v", err)
	}
	if !exists {
		t.Fatal("expected delayed job to exist")
	}

	// 3. Processing queue (after claim)
	claimed, err := service.DequeueDeployment(time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("claim deployment: %v", err)
	}
	if claimed.JobID != readyJobID {
		t.Fatalf("claimed unexpected job: got %s, want %s", claimed.JobID, readyJobID)
	}
	exists, err = service.HasDeploymentJob(readyJobID)
	if err != nil {
		t.Fatalf("check claimed job: %v", err)
	}
	if !exists {
		t.Fatal("expected claimed job in processing queue to exist")
	}

	// 4. Acknowledged (removed from processing queue)
	if err := service.AcknowledgeDeployment(claimed); err != nil {
		t.Fatalf("acknowledge deployment: %v", err)
	}
	exists, err = service.HasDeploymentJob(readyJobID)
	if err != nil {
		t.Fatalf("check acknowledged job: %v", err)
	}
	if exists {
		t.Fatal("expected acknowledged job to no longer exist")
	}
}
