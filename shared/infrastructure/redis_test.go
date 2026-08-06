package infrastructure

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-redis/redismock/v9"
)

func TestPendingEnvRefresh(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	expectedKey := "project:pending_env_refresh:42"

	// 1. Test SetPendingEnvRefresh
	mock.ExpectSet(expectedKey, "true", 0).SetVal("OK")
	err := redisService.SetPendingEnvRefresh(projectID)
	if err != nil {
		t.Fatalf("SetPendingEnvRefresh failed: %v", err)
	}

	// 2. Test HasPendingEnvRefresh (exists/true)
	mock.ExpectGet(expectedKey).SetVal("true")
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
	mock.ExpectDel(expectedKey).SetVal(1)
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

func TestEnqueueEnvUpdateIfQuiet_BusyLocked(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Lock exists -> busy
	mock.ExpectExists(expectedLockKey).SetVal(1)

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

	// Lock does not exist, but there is a job for this project in the queue -> busy
	mock.ExpectExists(expectedLockKey).SetVal(0)
	mock.ExpectLRange("deployment:queue", 0, -1).SetVal([]string{
		`{"project_id":42,"user_id":1,"type":"deploy","job_id":"existing-job"}`,
	})

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

	// Lock does not exist, queue is empty/has other projects -> quiet, so enqueues
	mock.ExpectExists(expectedLockKey).SetVal(0)
	mock.ExpectLRange("deployment:queue", 0, -1).SetVal([]string{
		`{"project_id":99,"user_id":1,"type":"deploy","job_id":"other-job"}`,
	})

	// Push and HIncr in EnqueueDeploymentNonDestructive.
	mock.Regexp().ExpectRPush("deployment:queue", ".*").SetVal(1)
	mock.ExpectHIncrBy("deployment:stats", "enqueued", 1).SetVal(1)

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

func TestEnqueueEnvUpdateIfQuiet_LRangeError(t *testing.T) {
	redisClient, mock := redismock.NewClientMock()
	redisService := NewRedisServiceWithClient(redisClient)

	projectID := uint(42)
	userID := uint(1)
	expectedLockKey := "deployment:lock:42"

	// Exists is fine, but LRange fails
	mock.ExpectExists(expectedLockKey).SetVal(0)
	mock.ExpectLRange("deployment:queue", 0, -1).SetErr(fmt.Errorf("read error"))

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
