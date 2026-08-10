// ===========================================
// Redis Service
// ===========================================
// Handles Redis connections and operations
// ===========================================
package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"os"
	"strconv"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// RedisService handles Redis operations
type RedisService struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisService creates a new Redis service
func NewRedisService(cfg *config.Config) (*RedisService, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password:     cfg.RedisPassword,
		DB:           0,   // use default DB
		PoolSize:     200, // optimize for ~200 real users concurrency
		MinIdleConns: 50,
	})

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisService{
		client: client,
		ctx:    ctx,
	}, nil
}

// NewRedisServiceWithClient creates a Redis service wrapping an existing client
func NewRedisServiceWithClient(client *redis.Client) *RedisService {
	return &RedisService{
		client: client,
		ctx:    context.Background(),
	}
}

// Close closes the Redis connection
func (r *RedisService) Close() error {
	return r.client.Close()
}

// Ping tests the Redis connection
func (r *RedisService) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// DeploymentJob represents a deployment job in the queue
type DeploymentJob struct {
	ProjectID               uint       `json:"project_id"`
	UserID                  uint       `json:"user_id"`
	Type                    string     `json:"type"` // "deploy" or "redeploy"
	BillingSuspension       bool       `json:"billing_suspension,omitempty"`
	BillingResume           bool       `json:"billing_resume,omitempty"`
	BillingSuspensionTaskID uint       `json:"billing_suspension_task_id,omitempty"`
	EnvSyncGeneration       uint       `json:"env_sync_generation,omitempty"`
	EnqueuedAt              time.Time  `json:"enqueued_at"`
	StartedAt               *time.Time `json:"started_at,omitempty"`
	HeartbeatAt             *time.Time `json:"heartbeat_at,omitempty"`
	JobID                   string     `json:"job_id"`
	RetryCount              int        `json:"retry_count"`
	queuePayload            string
}

// DeploymentLeaseMetadata stores structured JSON metadata for a deployment job-scoped lease
type DeploymentLeaseMetadata struct {
	JobID          string `json:"job_id"`
	ProjectID      uint   `json:"project_id"`
	WorkerID       string `json:"worker_id"`
	Hostname       string `json:"hostname"`
	StartedAt      string `json:"started_at"`
	LastHeartbeat  string `json:"last_heartbeat"`
	DeploymentType string `json:"deployment_type"`
}

const (
	deploymentQueueKey           = "deployment:queue"
	deploymentProcessingQueueKey = "deployment:processing_queue"
	deploymentDelayedQueueKey    = "deployment:delayed_queue"
	deploymentLockKey            = "deployment:lock"
	deploymentStatsKey           = "deployment:stats"
	deploymentLeaseKeyPrefix     = "deployment:lease"
)

var (
	claimDeploymentJobScript = redis.NewScript(`
local raw = redis.call("lpop", KEYS[1])
if not raw then return false end
local job = cjson.decode(raw)
job.started_at = ARGV[1]
job.heartbeat_at = ARGV[1]
local claimed = cjson.encode(job)
redis.call("rpush", KEYS[2], claimed)
return claimed
`)
	requeueClaimedDeploymentJobScript = redis.NewScript(`
if redis.call("lrem", KEYS[1], 1, ARGV[1]) == 1 then
    redis.call("rpush", KEYS[2], ARGV[2])
    return 1
end
return 0
`)
)

var replaceClaimedDeploymentJobScript = redis.NewScript(`
if redis.call("lrem", KEYS[1], 1, ARGV[1]) == 1 then
    redis.call("rpush", KEYS[1], ARGV[2])
    return 1
end
return 0
`)

// EnqueueDeployment adds a deployment job to the queue with deduplication
func (r *RedisService) EnqueueDeployment(projectID, userID uint, deployType string) (string, error) {
	// Deduplicate: Remove any existing queued jobs for this project ID so latest request wins
	_ = r.RemoveFromQueue(projectID)

	jobID := utils.GenerateRandomUID()
	job := DeploymentJob{
		ProjectID:  projectID,
		UserID:     userID,
		Type:       deployType,
		EnqueuedAt: time.Now(),
		JobID:      jobID,
		RetryCount: 0,
	}

	data, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}

	// Add to the queue
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return "", fmt.Errorf("failed to enqueue job: %w", err)
	}

	// Increment enqueued counter
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)

	return jobID, nil
}

func (r *RedisService) EnqueueDeploymentNonDestructive(projectID, userID uint, deployType string) (string, error) {
	jobID := utils.GenerateRandomUID()
	job := DeploymentJob{ProjectID: projectID, UserID: userID, Type: deployType, EnqueuedAt: time.Now(), JobID: jobID}
	data, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return "", fmt.Errorf("failed to enqueue job: %w", err)
	}
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)
	return jobID, nil
}

// EnqueueBillingSuspensionStop preserves the origin so stale stop jobs can be fenced at execution.
func (r *RedisService) EnqueueBillingSuspensionStop(projectID, userID, taskID uint) (string, error) {
	if taskID == 0 {
		return "", fmt.Errorf("billing suspension task is required")
	}
	jobID := utils.GenerateRandomUID()
	job := DeploymentJob{ProjectID: projectID, UserID: userID, Type: "stop", BillingSuspension: true, BillingSuspensionTaskID: taskID, EnqueuedAt: time.Now(), JobID: jobID}
	data, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal billing suspension job: %w", err)
	}
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return "", fmt.Errorf("failed to enqueue billing suspension job: %w", err)
	}
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)
	return jobID, nil
}

// EnqueueBillingSuspensionResume starts only runtime identities recorded by the billing stop task.
func (r *RedisService) EnqueueBillingSuspensionResume(projectID, userID, taskID uint) (string, error) {
	if taskID == 0 {
		return "", fmt.Errorf("billing suspension task is required")
	}
	jobID := utils.GenerateRandomUID()
	job := DeploymentJob{ProjectID: projectID, UserID: userID, Type: "start", BillingResume: true, BillingSuspensionTaskID: taskID, EnqueuedAt: time.Now(), JobID: jobID}
	data, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal billing resume job: %w", err)
	}
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return "", fmt.Errorf("failed to enqueue billing resume job: %w", err)
	}
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)
	return jobID, nil
}

// EnqueueDeploymentEnvSync queues one durable environment-generation acknowledgement.
func (r *RedisService) EnqueueDeploymentEnvSync(projectID, userID, generation uint) (string, error) {
	if generation == 0 {
		return "", fmt.Errorf("environment sync generation is required")
	}
	jobID := utils.GenerateRandomUID()
	job := DeploymentJob{ProjectID: projectID, UserID: userID, Type: "update_env", EnvSyncGeneration: generation, EnqueuedAt: time.Now(), JobID: jobID}
	data, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal environment sync job: %w", err)
	}
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return "", fmt.Errorf("failed to enqueue environment sync job: %w", err)
	}
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)
	return jobID, nil
}

// EnqueueEnvUpdateIfQuiet enqueues an update_env job only if no other job for this project is queued or running
func (r *RedisService) EnqueueEnvUpdateIfQuiet(projectID, userID uint) (string, error) {
	// Check if already locked (running)
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	locked, err := r.client.Exists(r.ctx, lockKey).Result()
	if err != nil {
		return "", fmt.Errorf("failed to check lock status: %w", err)
	}
	if locked > 0 {
		slog.Info("Project has active deployment lock, skipping env update enqueue", "projectId", projectID)
		return "", nil
	}

	// Check if already in queue
	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return "", fmt.Errorf("failed to read deployment queue: %w", err)
	}
	for _, res := range results {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil && job.ProjectID == projectID {
			slog.Info("Project already has a queued job, skipping env update enqueue", "projectId", projectID, "jobType", job.Type)
			return "", nil
		}
	}

	return r.EnqueueDeploymentNonDestructive(projectID, userID, "update_env")
}

// SetPendingEnvRefresh sets a marker indicating that the project needs an env refresh
func (r *RedisService) SetPendingEnvRefresh(projectID uint) error {
	key := fmt.Sprintf("project:pending_env_refresh:%d", projectID)
	return r.client.Set(r.ctx, key, "true", 0).Err()
}

// HasPendingEnvRefresh checks if the project has a pending env refresh marker set
func (r *RedisService) HasPendingEnvRefresh(projectID uint) (bool, error) {
	key := fmt.Sprintf("project:pending_env_refresh:%d", projectID)
	val, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "true", nil
}

// ClearPendingEnvRefresh removes the pending env refresh marker for the project
func (r *RedisService) ClearPendingEnvRefresh(projectID uint) (bool, error) {
	key := fmt.Sprintf("project:pending_env_refresh:%d", projectID)
	deleted, err := r.client.Del(r.ctx, key).Result()
	return deleted > 0, err
}

// EnqueueDeploymentJob enqueues an existing DeploymentJob struct (used for retries)
func (r *RedisService) EnqueueDeploymentJob(job *DeploymentJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	r.client.HIncrBy(r.ctx, deploymentStatsKey, "enqueued", 1)
	return nil
}

// DequeueDeployment atomically claims the next job into durable processing storage.
func (r *RedisService) DequeueDeployment(timeout time.Duration) (*DeploymentJob, error) {
	deadline := time.Now().Add(timeout)
	for {
		now := time.Now().UTC()
		result, err := claimDeploymentJobScript.Run(r.ctx, r.client, []string{deploymentQueueKey, deploymentProcessingQueueKey}, now.Format(time.RFC3339Nano)).Result()
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("claim deployment job: %w", err)
		}
		if raw, ok := result.(string); ok && raw != "" {
			var job DeploymentJob
			if err := json.Unmarshal([]byte(raw), &job); err != nil {
				_ = r.client.LRem(r.ctx, deploymentProcessingQueueKey, 1, raw).Err()
				return nil, fmt.Errorf("unmarshal claimed deployment job: %w", err)
			}
			job.queuePayload = raw
			return &job, nil
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// AcknowledgeDeployment removes a successfully handled claimed job.
func (r *RedisService) AcknowledgeDeployment(job *DeploymentJob) error {
	if job == nil || job.queuePayload == "" {
		return nil
	}
	if err := r.client.LRem(r.ctx, deploymentProcessingQueueKey, 1, job.queuePayload).Err(); err != nil {
		return fmt.Errorf("acknowledge deployment job: %w", err)
	}
	return nil
}

// RequeueDeploymentJob returns a claimed job to the active queue without affecting other jobs.
func (r *RedisService) RequeueDeploymentJob(job *DeploymentJob) error {
	if job == nil || job.queuePayload == "" {
		return fmt.Errorf("claimed deployment job payload is required")
	}
	claimedPayload := job.queuePayload
	job.StartedAt = nil
	job.HeartbeatAt = nil
	job.queuePayload = ""
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal deployment job for requeue: %w", err)
	}
	if _, err := requeueClaimedDeploymentJobScript.Run(r.ctx, r.client, []string{deploymentProcessingQueueKey, deploymentQueueKey}, claimedPayload, payload).Result(); err != nil {
		return fmt.Errorf("requeue claimed deployment job: %w", err)
	}
	return nil
}

// RequeueExpiredDeploymentJobs returns abandoned processing jobs to the active queue.
func (r *RedisService) RequeueExpiredDeploymentJobs(lease time.Duration) error {
	if lease <= 0 {
		return fmt.Errorf("deployment processing lease must be positive")
	}
	entries, err := r.client.LRange(r.ctx, deploymentProcessingQueueKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("list claimed deployment jobs: %w", err)
	}
	cutoff := time.Now().UTC().Add(-lease)
	for _, entry := range entries {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(entry), &job); err != nil {
			continue
		}
		lastHeartbeat := job.HeartbeatAt
		if lastHeartbeat == nil {
			lastHeartbeat = job.StartedAt
		}
		if lastHeartbeat == nil || lastHeartbeat.After(cutoff) {
			continue
		}
		job.StartedAt = nil
		job.HeartbeatAt = nil
		payload, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("marshal expired deployment job: %w", err)
		}
		if _, err := requeueClaimedDeploymentJobScript.Run(r.ctx, r.client, []string{deploymentProcessingQueueKey, deploymentQueueKey}, entry, payload).Result(); err != nil {
			return fmt.Errorf("requeue expired deployment job: %w", err)
		}
	}
	return nil
}

func (r *RedisService) RenewDeploymentProcessingHeartbeat(job *DeploymentJob) error {
	if job == nil || job.queuePayload == "" {
		return fmt.Errorf("claimed deployment job payload is required")
	}
	previousPayload := job.queuePayload
	now := time.Now().UTC()
	job.HeartbeatAt = &now
	job.queuePayload = ""
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal deployment processing heartbeat: %w", err)
	}
	result, err := replaceClaimedDeploymentJobScript.Run(r.ctx, r.client, []string{deploymentProcessingQueueKey}, previousPayload, payload).Result()
	if err != nil {
		return fmt.Errorf("renew deployment processing heartbeat: %w", err)
	}
	if changed, ok := result.(int64); !ok || changed != 1 {
		return fmt.Errorf("deployment processing claim missing")
	}
	job.queuePayload = string(payload)
	return nil
}

// GetQueueLength returns the number of jobs in the queue
func (r *RedisService) GetQueueLength() (int64, error) {
	length, err := r.client.LLen(r.ctx, deploymentQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue length: %w", err)
	}
	return length, nil
}

// LockMetadata holds ownership metadata for a distributed deployment lock
type LockMetadata struct {
	Token         string    `json:"token"`
	WorkerID      string    `json:"worker_id"`
	DeploymentID  string    `json:"deployment_id"`
	Hostname      string    `json:"hostname"`
	StartedAt     time.Time `json:"started_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

var (
	releaseLockScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 0 end
local decoded = cjson.decode(val)
if decoded.token == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`)

	releasePlainLockScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 0 end
if val == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`)

	renewLockScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 0 end
local decoded = cjson.decode(val)
if decoded.token == ARGV[1] then
    decoded.last_heartbeat = ARGV[3]
    redis.call("set", KEYS[1], cjson.encode(decoded), "PX", ARGV[2])
    return 1
else
    return 0
end
`)

	renewLeaseScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 0 end
local decoded = cjson.decode(val)
if decoded.worker_id == ARGV[1] then
    decoded.last_heartbeat = ARGV[3]
    redis.call("set", KEYS[1], cjson.encode(decoded), "PX", ARGV[2])
    return 1
else
    return 0
end
`)

	releaseLeaseScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 1 end
local decoded = cjson.decode(val)
if decoded.worker_id == ARGV[1] then
    redis.call("del", KEYS[1])
    return 1
else
    return 0
end
`)

	migrateDelayedJobsScript = redis.NewScript(`
local delayedKey = KEYS[1]
local activeKey = KEYS[2]
local maxScore = ARGV[1]

local items = redis.call("ZRANGEBYSCORE", delayedKey, "-inf", maxScore)
if #items == 0 then
    return 0
end

for _, item in ipairs(items) do
    redis.call("ZREM", delayedKey, item)
    redis.call("RPUSH", activeKey, item)
    redis.call("HINCRBY", "deployment:stats", "enqueued", 1)
end

return #items
`)

	acquireOrRenewLeaderScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then
    redis.call("set", KEYS[1], ARGV[1], "PX", ARGV[2])
    return 1
elseif val == ARGV[1] then
    redis.call("pexpire", KEYS[1], ARGV[2])
    return 1
else
    return 0
end
`)

	renewDomainLockScript = redis.NewScript(`
local val = redis.call("get", KEYS[1])
if not val then return 0 end
local decoded = cjson.decode(val)
if decoded.token == ARGV[1] then
    redis.call("pexpire", KEYS[1], ARGV[2])
    return 1
else
    return 0
end
`)
)

func generateLockToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AcquireDeploymentLock tries to acquire a distributed lock for deployment, returning a unique lock token
func (r *RedisService) AcquireDeploymentLock(projectID uint, deploymentID string, ttl time.Duration) (string, error) {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	token := generateLockToken()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker-node"
	}

	meta := LockMetadata{
		Token:         token,
		WorkerID:      fmt.Sprintf("worker-%s", hostname),
		DeploymentID:  deploymentID,
		Hostname:      hostname,
		StartedAt:     time.Now(),
		LastHeartbeat: time.Now(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal lock metadata: %w", err)
	}

	// Try to set the lock with NX (only if not exists) and expiration
	ok, err := r.client.SetNX(r.ctx, lockKey, string(data), ttl).Result()
	if err != nil {
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !ok {
		return "", nil // Lock already held
	}

	return token, nil
}

// ReleaseDeploymentLock securely releases the deployment lock verifying the unique token
func (r *RedisService) ReleaseDeploymentLock(projectID uint, token string) error {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)

	_, err := releaseLockScript.Run(r.ctx, r.client, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to execute release lock script: %w", err)
	}

	return nil
}

// GetLockMetadata returns the metadata of an active deployment lock
func (r *RedisService) GetLockMetadata(projectID uint) (*LockMetadata, error) {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	val, err := r.client.Get(r.ctx, lockKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No active lock
		}
		return nil, fmt.Errorf("failed to get lock: %w", err)
	}

	var meta LockMetadata
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lock metadata: %w", err)
	}

	return &meta, nil
}

// ForceReleaseDeploymentLock unconditionally removes the lock (used by admin and watchdog recovery)
func (r *RedisService) ForceReleaseDeploymentLock(projectID uint, reason string) error {
	if reason == "" {
		reason = "System recovery / manual override"
	}
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	val, _ := r.client.Get(r.ctx, lockKey).Result()
	if err := r.client.Del(r.ctx, lockKey).Err(); err != nil {
		return fmt.Errorf("failed to force release lock: %w", err)
	}
	slog.Warn("Deployment lock forcibly released", "projectID", projectID, "reason", reason, "previousToken", val)
	return nil
}

// GetDeploymentStats returns statistics about the deployment queue
func (r *RedisService) GetDeploymentStats() (map[string]string, error) {
	stats, err := r.client.HGetAll(r.ctx, deploymentStatsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	// Add current queue length
	queueLen, _ := r.GetQueueLength()
	stats["queue_length"] = fmt.Sprintf("%d", queueLen)

	return stats, nil
}

// IncrementDeploymentCounter increments a specific deployment counter
func (r *RedisService) IncrementDeploymentCounter(counter string) {
	r.client.HIncrBy(r.ctx, deploymentStatsKey, counter, 1)
}

// ListDeploymentJobs returns all jobs currently in the queue
func (r *RedisService) ListDeploymentJobs() ([]DeploymentJob, error) {
	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	jobs := make([]DeploymentJob, 0, len(results))
	for _, res := range results {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// IsProjectQueued checks if a project is already in the deployment queue
func (r *RedisService) IsProjectQueued(projectID uint) (bool, error) {
	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return false, err
	}

	for _, res := range results {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil {
			if job.ProjectID == projectID {
				return true, nil
			}
		}
	}

	return false, nil
}

// SetCache sets a value in cache with expiration
func (r *RedisService) SetCache(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	if err := r.client.Set(r.ctx, key, data, expiration).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// SetNX sets a cache key only if it does not exist, returning true if successful.
func (r *RedisService) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return r.client.SetNX(r.ctx, key, data, expiration).Result()
}

// GetCache gets a value from cache
func (r *RedisService) GetCache(key string, dest interface{}) error {
	data, err := r.client.Get(r.ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key not found")
		}
		return fmt.Errorf("failed to get cache: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// GetString gets a raw string value from Redis
func (r *RedisService) GetString(key string) (string, error) {
	val, err := r.client.Get(r.ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("key not found")
		}
		return "", fmt.Errorf("failed to get string: %w", err)
	}
	return val, nil
}

// DeleteCache deletes a key from cache
func (r *RedisService) DeleteCache(key string) error {
	return r.client.Del(r.ctx, key).Err()
}

const revokedJTIPrefix = "auth:revoked:"

// RevokeJTI blocks one session until its exact JWT expiry.
func (r *RedisService) RevokeJTI(jti string, expiration time.Duration) error {
	if expiration <= 0 {
		return nil
	}
	return r.client.Set(r.ctx, revokedJTIPrefix+jti, true, expiration).Err()
}

// IsJTIRevoked returns Redis failure so authentication can fail closed.
func (r *RedisService) IsJTIRevoked(jti string) (bool, error) {
	exists, err := r.client.Exists(r.ctx, revokedJTIPrefix+jti).Result()
	return exists > 0, err
}

// RateLimit checks and increments a rate limit counter
func (r *RedisService) RateLimit(key string, limit int, duration time.Duration) (bool, time.Duration, error) {
	count, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return false, 0, err
	}

	if count == 1 {
		r.client.Expire(r.ctx, key, duration)
	}

	if count > int64(limit) {
		ttl, _ := r.client.TTL(r.ctx, key).Result()
		if ttl < time.Second {
			ttl = time.Second
		}
		return false, ttl, nil // Limit exceeded
	}

	return true, 0, nil
}

// IncrDriftFailure increments the consecutive drift check failure count for a domain and returns the current count.
// Keeps track of failures across distributed workers to enforce a grace period before marking a domain degraded.
func (r *RedisService) IncrDriftFailure(domainID uint) (int, error) {
	key := fmt.Sprintf("drift:failures:%d", domainID)
	count, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		// Set 1-hour expiration so stale failure counters are eventually cleaned up automatically
		r.client.Expire(r.ctx, key, 1*time.Hour)
	}
	return int(count), nil
}

// ClearDriftFailure resets the consecutive drift check failure count when the check succeeds.
func (r *RedisService) ClearDriftFailure(domainID uint) error {
	key := fmt.Sprintf("drift:failures:%d", domainID)
	return r.client.Del(r.ctx, key).Err()
}

// IncrHealthFailure increments the consecutive health check failure count for a domain and returns the current count.
// Shared across workers to enforce a grace period before marking a domain unhealthy.
func (r *RedisService) IncrHealthFailure(domainID uint) (int, error) {
	key := fmt.Sprintf("health:failures:%d", domainID)
	count, err := r.client.Incr(r.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		// Set 1-hour expiration to prevent stale metrics leaks
		r.client.Expire(r.ctx, key, 1*time.Hour)
	}
	return int(count), nil
}

// ClearHealthFailure resets the consecutive health check failure count when the check succeeds.
func (r *RedisService) ClearHealthFailure(domainID uint) error {
	key := fmt.Sprintf("health:failures:%d", domainID)
	return r.client.Del(r.ctx, key).Err()
}

// RemoveFromQueue removes all queued instances of a specific project from the deployment queue and delayed queue
func (r *RedisService) RemoveFromQueue(projectID uint) error {
	// 1. Remove from active queue
	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err == nil {
		for _, res := range results {
			var job DeploymentJob
			if err := json.Unmarshal([]byte(res), &job); err == nil && job.ProjectID == projectID {
				_ = r.client.LRem(r.ctx, deploymentQueueKey, 0, res).Err()
			}
		}
	}

	// 2. Remove from delayed queue
	delayedResults, err := r.client.ZRange(r.ctx, deploymentDelayedQueueKey, 0, -1).Result()
	if err == nil {
		for _, res := range delayedResults {
			var job DeploymentJob
			if err := json.Unmarshal([]byte(res), &job); err == nil && job.ProjectID == projectID {
				_ = r.client.ZRem(r.ctx, deploymentDelayedQueueKey, res).Err()
			}
		}
	}
	return nil
}

// RemoveDeploymentJob removes a specific queued deployment job without touching newer jobs for the same project.
func (r *RedisService) RemoveDeploymentJob(jobID string) error {
	if jobID == "" {
		return nil
	}

	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to inspect deployment queue: %w", err)
	}
	for _, res := range results {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil && job.JobID == jobID {
			if err := r.client.LRem(r.ctx, deploymentQueueKey, 1, res).Err(); err != nil {
				return fmt.Errorf("failed to remove queued deployment job: %w", err)
			}
			return nil
		}
	}

	delayedResults, err := r.client.ZRange(r.ctx, deploymentDelayedQueueKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to inspect delayed deployment queue: %w", err)
	}
	for _, res := range delayedResults {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil && job.JobID == jobID {
			if err := r.client.ZRem(r.ctx, deploymentDelayedQueueKey, res).Err(); err != nil {
				return fmt.Errorf("failed to remove delayed deployment job: %w", err)
			}
			return nil
		}
	}

	return nil
}

// RenewDeploymentLock resets the TTL of an active deployment lock verifying the unique token and updating heartbeat metadata
func (r *RedisService) RenewDeploymentLock(projectID uint, token string, ttl time.Duration) error {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	nowStr := time.Now().Format(time.RFC3339)
	res, err := renewLockScript.Run(r.ctx, r.client, []string{lockKey}, token, int64(ttl.Milliseconds()), nowStr).Result()
	if err != nil {
		return fmt.Errorf("failed to execute renew lock script: %w", err)
	}
	if val, ok := res.(int64); ok && val == 0 {
		return fmt.Errorf("lock expired or token mismatch")
	}
	return nil
}

// IsDeploymentLocked checks if a deployment lock still exists
func (r *RedisService) IsDeploymentLocked(projectID uint) bool {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	exists, err := r.client.Exists(r.ctx, lockKey).Result()
	return err == nil && exists > 0
}

// BuildLogMessage carries a log line with optional JobID for client-side filtering.
type BuildLogMessage struct {
	JobID string `json:"job_id,omitempty"`
	Line  string `json:"line"`
}

// PublishBuildLog streams a build log line to a Redis Pub/Sub channel.
func (r *RedisService) PublishBuildLog(projectID uint, msg string) error {
	channel := fmt.Sprintf("channel:build_logs:%d", projectID)
	return r.client.Publish(r.ctx, channel, msg).Err()
}

// PublishBuildLogForJob streams a log line with JobID to filter stale events.
func (r *RedisService) PublishBuildLogForJob(projectID uint, jobID, msg string) error {
	if jobID == "" {
		return r.PublishBuildLog(projectID, msg)
	}

	payload, err := json.Marshal(BuildLogMessage{
		JobID: jobID,
		Line:  msg,
	})
	if err != nil {
		return err
	}

	channel := fmt.Sprintf("channel:build_logs:%d", projectID)
	return r.client.Publish(r.ctx, channel, string(payload)).Err()
}

// PublishDeploymentEvent streams a deployment lifecycle event to a Redis Pub/Sub channel
func (r *RedisService) PublishDeploymentEvent(projectID uint, eventJSON string) error {
	channel := fmt.Sprintf("channel:deployment_events:%d", projectID)
	return r.client.Publish(r.ctx, channel, eventJSON).Err()
}

// SubscribeBuildLogs subscribes to a build log channel and returns a Go channel of messages
func (r *RedisService) SubscribeBuildLogs(ctx context.Context, projectID uint) (<-chan string, error) {
	channel := fmt.Sprintf("channel:build_logs:%d", projectID)
	sub := r.client.Subscribe(ctx, channel)

	msgChan := make(chan string, 100)
	go func() {
		defer sub.Close()
		defer close(msgChan)
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case msgChan <- msg.Payload:
				default:
					// buffer full, skip
				}
			}
		}
	}()
	return msgChan, nil
}

// SubscribeDeploymentEvents subscribes to a deployment lifecycle events channel and returns a Go channel of messages
func (r *RedisService) SubscribeDeploymentEvents(ctx context.Context, projectID uint) (<-chan string, error) {
	channel := fmt.Sprintf("channel:deployment_events:%d", projectID)
	sub := r.client.Subscribe(ctx, channel)

	msgChan := make(chan string, 100)
	go func() {
		defer sub.Close()
		defer close(msgChan)
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case msgChan <- msg.Payload:
				default:
					// buffer full, skip
				}
			}
		}
	}()
	return msgChan, nil
}

// PublishCancellation broadcasts a cancellation signal for a specific project
func (r *RedisService) PublishCancellation(ctx context.Context, projectID uint) error {
	channel := fmt.Sprintf("channel:cancel_deployment:%d", projectID)
	return r.client.Publish(r.ctx, channel, "cancel").Err()
}

// SubscribeCancellation subscribes to the cancellation channel for a project
func (r *RedisService) SubscribeCancellation(ctx context.Context, projectID uint) (<-chan string, error) {
	channel := fmt.Sprintf("channel:cancel_deployment:%d", projectID)
	sub := r.client.Subscribe(r.ctx, channel)

	msgChan := make(chan string, 10)
	go func() {
		defer sub.Close()
		defer close(msgChan)
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				select {
				case msgChan <- msg.Payload:
				default:
				}
			}
		}
	}()
	return msgChan, nil
}

// EnqueueDelayedDeploymentJob adds a deployment job to the durable delayed ZSET queue
func (r *RedisService) EnqueueDelayedDeploymentJob(job *DeploymentJob, delay time.Duration) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	executeAt := time.Now().Add(delay).UnixMilli()
	if err := r.client.ZAdd(r.ctx, deploymentDelayedQueueKey, redis.Z{Score: float64(executeAt), Member: string(data)}).Err(); err != nil {
		return fmt.Errorf("failed to enqueue delayed job: %w", err)
	}

	r.client.HIncrBy(r.ctx, deploymentStatsKey, "delayed", 1)
	return nil
}

// MigrateDelayedJobs atomically moves ready delayed jobs to the active queue
func (r *RedisService) MigrateDelayedJobs() (int64, error) {
	nowMilli := strconv.FormatInt(time.Now().UnixMilli(), 10)
	res, err := migrateDelayedJobsScript.Run(r.ctx, r.client, []string{deploymentDelayedQueueKey, deploymentQueueKey}, nowMilli).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to run migrate script: %w", err)
	}
	val, _ := res.(int64)
	return val, nil
}

// CalculateIdempotencyHash generates a unique fingerprint for a deployment request
func CalculateIdempotencyHash(projectID uint, commitHash, envHash, trigger string) string {
	raw := fmt.Sprintf("%d:%s:%s:%s", projectID, commitHash, envHash, trigger)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// CheckIdempotency verifies if an identical deployment fingerprint already exists
func (r *RedisService) CheckIdempotency(projectID uint, commitHash, envHash, trigger string) (bool, error) {
	key := fmt.Sprintf("deployment:idempotency:%d", projectID)
	expected := CalculateIdempotencyHash(projectID, commitHash, envHash, trigger)

	val, err := r.client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == expected, nil
}

// SetIdempotency stores a deployment fingerprint with 24h expiration
func (r *RedisService) SetIdempotency(projectID uint, commitHash, envHash, trigger string) error {
	key := fmt.Sprintf("deployment:idempotency:%d", projectID)
	fingerprint := CalculateIdempotencyHash(projectID, commitHash, envHash, trigger)
	return r.client.Set(r.ctx, key, fingerprint, 24*time.Hour).Err()
}

// AcquireDeploymentLease creates an independent job-scoped lease for an active deployment.
func (r *RedisService) AcquireDeploymentLease(jobID string, metadata *DeploymentLeaseMetadata, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", deploymentLeaseKeyPrefix, jobID)
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment lease metadata: %w", err)
	}
	acquired, err := r.client.SetNX(r.ctx, key, string(data), ttl).Result()
	if err != nil {
		return fmt.Errorf("create deployment lease: %w", err)
	}
	if !acquired {
		return fmt.Errorf("deployment lease already held for job %s", jobID)
	}
	return nil
}

// RenewDeploymentLease safely renews a deployment lease verifying worker ownership via Lua script.
func (r *RedisService) RenewDeploymentLease(jobID string, workerID string, ttl time.Duration) error {
	key := fmt.Sprintf("%s:%s", deploymentLeaseKeyPrefix, jobID)
	ttlMs := fmt.Sprintf("%d", ttl.Milliseconds())
	nowStr := time.Now().Format(time.RFC3339)

	res, err := renewLeaseScript.Run(r.ctx, r.client, []string{key}, workerID, ttlMs, nowStr).Result()
	if err != nil {
		return fmt.Errorf("failed to run renew lease script: %w", err)
	}

	if val, ok := res.(int64); !ok || val == 0 {
		return fmt.Errorf("lease renewal rejected: lease missing or ownership mismatch for worker %s", workerID)
	}
	return nil
}

// ReleaseDeploymentLease cleanly removes a deployment lease verifying worker ownership via Lua script.
func (r *RedisService) ReleaseDeploymentLease(jobID string, workerID string) error {
	key := fmt.Sprintf("%s:%s", deploymentLeaseKeyPrefix, jobID)

	_, err := releaseLeaseScript.Run(r.ctx, r.client, []string{key}, workerID).Result()
	if err != nil {
		return fmt.Errorf("failed to run release lease script: %w", err)
	}
	return nil
}

// GetDeploymentLease retrieves the active lease metadata for a deployment job.
func (r *RedisService) GetDeploymentLease(jobID string) (*DeploymentLeaseMetadata, error) {
	key := fmt.Sprintf("%s:%s", deploymentLeaseKeyPrefix, jobID)
	val, err := r.client.Get(r.ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No active lease
		}
		return nil, fmt.Errorf("failed to get deployment lease: %w", err)
	}

	var meta DeploymentLeaseMetadata
	if err := json.Unmarshal([]byte(val), &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal deployment lease metadata: %w", err)
	}
	return &meta, nil
}

// ListActiveDeploymentLeases scans Redis for all active deployment leases without using KEYS.
func (r *RedisService) ListActiveDeploymentLeases() ([]DeploymentLeaseMetadata, error) {
	var cursor uint64
	var allLeases []DeploymentLeaseMetadata
	match := fmt.Sprintf("%s:*", deploymentLeaseKeyPrefix)

	for {
		var keys []string
		var err error
		keys, cursor, err = r.client.Scan(r.ctx, cursor, match, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		for _, key := range keys {
			val, err := r.client.Get(r.ctx, key).Result()
			if err == nil {
				var meta DeploymentLeaseMetadata
				if json.Unmarshal([]byte(val), &meta) == nil {
					allLeases = append(allLeases, meta)
				}
			}
		}

		if cursor == 0 {
			break
		}
	}
	return allLeases, nil
}

// ===========================================
// Domain Management & Locking Operations
// ===========================================

const (
	domainLockKeyPrefix      = "domain:lock"
	reconcilerLockKey        = "reconciler:leader:lock"
	domainEventChannelPrefix = "channel:domain_events"
)

// DomainLockMetadata holds ownership metadata for a distributed domain lock to ensure concurrency safety during verification and SSL provisioning.
type DomainLockMetadata struct {
	Token     string    `json:"token"`
	WorkerID  string    `json:"worker_id"`
	DomainID  uint      `json:"domain_id"`
	StartedAt time.Time `json:"started_at"`
}

// AcquireDomainLock tries to acquire a distributed lock for a specific domain ID, returning a unique fencing token.
func (r *RedisService) AcquireDomainLock(domainID uint, ttl time.Duration) (string, error) {
	lockKey := fmt.Sprintf("%s:%d", domainLockKeyPrefix, domainID)
	token := generateLockToken()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker-node"
	}

	meta := DomainLockMetadata{
		Token:     token,
		WorkerID:  fmt.Sprintf("worker-%s", hostname),
		DomainID:  domainID,
		StartedAt: time.Now(),
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("failed to marshal domain lock metadata: %w", err)
	}

	ok, err := r.client.SetNX(r.ctx, lockKey, string(data), ttl).Result()
	if err != nil {
		return "", fmt.Errorf("failed to acquire domain lock: %w", err)
	}

	if !ok {
		return "", nil // Lock already held by another process
	}

	return token, nil
}

// ReleaseDomainLock securely releases the domain lock verifying the unique fencing token.
func (r *RedisService) ReleaseDomainLock(domainID uint, token string) error {
	lockKey := fmt.Sprintf("%s:%d", domainLockKeyPrefix, domainID)
	_, err := releaseLockScript.Run(r.ctx, r.client, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to release domain lock: %w", err)
	}
	return nil
}

// AcquireProjectDomainLock tries to acquire a distributed lock for project domain registration, returning a unique token.
func (r *RedisService) AcquireProjectDomainLock(projectID uint, ttl time.Duration) (string, error) {
	lockKey := fmt.Sprintf("project:%d:domain_lock", projectID)
	token := generateLockToken()
	ok, err := r.client.SetNX(r.ctx, lockKey, token, ttl).Result()
	if err != nil {
		return "", fmt.Errorf("failed to acquire project domain lock: %w", err)
	}
	if !ok {
		return "", nil // Lock already held
	}
	return token, nil
}

// ReleaseProjectDomainLock securely releases the project domain lock verifying the unique token.
func (r *RedisService) ReleaseProjectDomainLock(projectID uint, token string) error {
	lockKey := fmt.Sprintf("project:%d:domain_lock", projectID)
	_, err := releasePlainLockScript.Run(r.ctx, r.client, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to release project domain lock: %w", err)
	}
	return nil
}

// RenewDomainLock safely renews an active domain lock checking token match.
func (r *RedisService) RenewDomainLock(domainID uint, token string, ttl time.Duration) error {
	lockKey := fmt.Sprintf("%s:%d", domainLockKeyPrefix, domainID)
	res, err := renewDomainLockScript.Run(r.ctx, r.client, []string{lockKey}, token, int64(ttl.Milliseconds())).Result()
	if err != nil {
		return fmt.Errorf("failed to renew domain lock: %w", err)
	}
	if val, ok := res.(int64); !ok || val == 0 {
		return fmt.Errorf("domain lock renewal rejected: expired or token mismatch")
	}
	return nil
}

// ForceReleaseDomainLock unconditionally removes a domain lock (used for emergency recovery).
func (r *RedisService) ForceReleaseDomainLock(domainID uint, reason string, operator string) error {
	if reason == "" {
		return fmt.Errorf("force release of domain lock rejected: mandatory reason is required")
	}
	if operator == "" {
		operator = "system_watchdog"
	}
	lockKey := fmt.Sprintf("%s:%d", domainLockKeyPrefix, domainID)
	val, _ := r.client.Get(r.ctx, lockKey).Result()
	err := r.client.Del(r.ctx, lockKey).Err()
	if err == nil {
		slog.Warn("Domain lock forcibly released", "domainID", domainID, "operator", operator, "reason", reason, "previousToken", val)
		r.IncrDomainMetric("lock_force_releases", 1)
	}
	return err
}

// AcquireReconcilerLock acquires the global leadership lease lock for the domain reconciliation worker to prevent multi-worker collisions.
func (r *RedisService) AcquireReconcilerLock(workerID string, ttl time.Duration) (bool, error) {
	ok, err := r.client.SetNX(r.ctx, reconcilerLockKey, workerID, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire reconciler leadership lock: %w", err)
	}
	return ok, nil
}

// RenewReconcilerLock safely renews the reconciler leadership lease lock verifying worker ownership.
func (r *RedisService) RenewReconcilerLock(workerID string, ttl time.Duration) (bool, error) {
	val, err := r.client.Get(r.ctx, reconcilerLockKey).Result()
	if err == redis.Nil {
		return r.AcquireReconcilerLock(workerID, ttl)
	}
	if err != nil {
		return false, fmt.Errorf("failed to check reconciler leadership lock: %w", err)
	}
	if val != workerID {
		return false, nil // Another worker is leader
	}
	ok, err := r.client.Expire(r.ctx, reconcilerLockKey, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to renew reconciler leadership lock: %w", err)
	}
	return ok, nil
}

// AcquireOrRenewReconcilerLock atomically acquires leadership if unowned, or renews if owned by current worker.
func (r *RedisService) AcquireOrRenewReconcilerLock(workerID string, ttl time.Duration) (bool, error) {
	res, err := acquireOrRenewLeaderScript.Run(r.ctx, r.client, []string{reconcilerLockKey}, workerID, int64(ttl.Milliseconds())).Result()
	if err != nil {
		return false, fmt.Errorf("failed to run leader script: %w", err)
	}
	if val, ok := res.(int64); ok && val == 1 {
		return true, nil
	}
	return false, nil
}

// PublishDomainEvent streams a domain lifecycle audit event to a Redis Pub/Sub channel for realtime frontend delivery.
func (r *RedisService) PublishDomainEvent(domainID uint, projectID uint, eventJSON string) error {
	channel := fmt.Sprintf("%s:%d", domainEventChannelPrefix, domainID)
	if projectID > 0 {
		projChannel := fmt.Sprintf("project:domains:events:%d", projectID)
		_ = r.client.Publish(r.ctx, projChannel, eventJSON).Err()
	}
	return r.client.Publish(r.ctx, channel, eventJSON).Err()
}

// runPubSubLoop handles resilient Redis Pub/Sub resubscription with exponential backoff and jitter
func (r *RedisService) runPubSubLoop(ctx context.Context, channel string, msgChan chan string) {
	defer close(msgChan)
	backoff := 500 * time.Millisecond
	maxBackoff := 10 * time.Second
	attempts := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if attempts > 0 {
			if err := r.client.Ping(ctx).Err(); err != nil {
				attempts++
				jitter := time.Duration(mathrand.Int64N(int64(backoff/4) + 1))
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff + jitter):
				}
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}
		}

		sub := r.client.Subscribe(ctx, channel)
		ch := sub.Channel()
		if attempts > 0 {
			slog.Info("Redis SSE subscription successfully recovered after reconnect.", "channel", channel, "reconnectAttempts", attempts)
			reconnectMsg := `{"event_type":"redis_reconnected","message":"Redis connection recovered"}`
			select {
			case msgChan <- reconnectMsg:
			default:
			}
		}
		attempts = 0
		backoff = 500 * time.Millisecond

	receiveLoop:
		for {
			select {
			case <-ctx.Done():
				_ = sub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					_ = sub.Close()
					attempts++
					slog.Warn("Redis connection lost during SSE subscription. Initiating reconnect loop with exponential backoff.", "channel", channel, "attempt", attempts)
					break receiveLoop
				}
				select {
				case msgChan <- msg.Payload:
				default:
					r.IncrDomainMetric("sse_subscriber_overflow", 1)
					metrics.GetCollector().IncrSSEOverflowTotal()
					slog.Warn("SSE subscriber buffer overflow, emitting overflow signal", "channel", channel)
					overflowMsg := `{"event_type":"overflow","message":"Subscriber buffer overflow, forcing reconnect and replay","error_code":"overflow"}`
					select {
					case <-msgChan:
					default:
					}
					select {
					case msgChan <- overflowMsg:
					default:
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		slog.Info("Attempting Redis SSE subscription recovery...", "channel", channel, "backoff", backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// SubscribeDomainEvents subscribes to a domain audit event channel and returns a Go channel of JSON messages.
func (r *RedisService) SubscribeDomainEvents(ctx context.Context, domainID uint) (<-chan string, error) {
	channel := fmt.Sprintf("%s:%d", domainEventChannelPrefix, domainID)
	msgChan := make(chan string, 100)
	go r.runPubSubLoop(ctx, channel, msgChan)
	return msgChan, nil
}

// SubscribeProjectEvents subscribes to all domain audit events within a specific project.
func (r *RedisService) SubscribeProjectEvents(ctx context.Context, projectID uint) (<-chan string, error) {
	channel := fmt.Sprintf("project:domains:events:%d", projectID)
	msgChan := make(chan string, 100)
	go r.runPubSubLoop(ctx, channel, msgChan)
	return msgChan, nil
}

// IncrDomainMetric increments an operational metric counter in Redis.
func (r *RedisService) IncrDomainMetric(field string, incr int64) {
	_ = r.client.HIncrBy(r.ctx, "domain:metrics", field, incr).Err()
}

// RecordDomainMetricDuration records the average or total duration of an operational phase.
func (r *RedisService) RecordDomainMetricDuration(field string, d time.Duration) {
	_ = r.client.HIncrBy(r.ctx, "domain:metrics:duration", field, int64(d.Milliseconds())).Err()
	_ = r.client.HIncrBy(r.ctx, "domain:metrics:count", field, 1).Err()
}

// GetDomainMetrics retrieves all operational metrics.
func (r *RedisService) GetDomainMetrics() (map[string]interface{}, error) {
	counts, err := r.client.HGetAll(r.ctx, "domain:metrics").Result()
	if err != nil {
		return nil, err
	}
	durations, _ := r.client.HGetAll(r.ctx, "domain:metrics:duration").Result()
	calls, _ := r.client.HGetAll(r.ctx, "domain:metrics:count").Result()

	res := make(map[string]interface{})
	for k, v := range counts {
		val, _ := strconv.ParseInt(v, 10, 64)
		res[k] = val
	}
	for k, vD := range durations {
		vC := calls[k]
		totalMs, _ := strconv.ParseInt(vD, 10, 64)
		totalCalls, _ := strconv.ParseInt(vC, 10, 64)
		if totalCalls > 0 {
			res[k+"_avg_ms"] = totalMs / totalCalls
		}
	}
	return res, nil
}

// GithubStatusPayload represents the desired commit status payload stored in Redis for eventual consistency reconciliation.
type GithubStatusPayload struct {
	InstallationID int64  `json:"installation_id"`
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	SHA            string `json:"sha"`
	State          string `json:"state"`
	TargetURL      string `json:"target_url"`
	Description    string `json:"description"`
	CreatedAt      int64  `json:"created_at"`
}

// SetDesiredCommitStatus stores the latest desired GitHub commit status for a commit hash.
func (r *RedisService) SetDesiredCommitStatus(payload *GithubStatusPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.HSet(r.ctx, "github:status:desired", payload.SHA, data)
	pipe.SAdd(r.ctx, "github:status:sync_set", payload.SHA)
	_, err = pipe.Exec(r.ctx)
	return err
}

// GetPendingCommitStatusSHAs returns all commit hashes that need their status synchronized.
func (r *RedisService) GetPendingCommitStatusSHAs() ([]string, error) {
	return r.client.SMembers(r.ctx, "github:status:sync_set").Result()
}

// GetDesiredCommitStatus retrieves the desired status for a specific commit hash.
func (r *RedisService) GetDesiredCommitStatus(sha string) (*GithubStatusPayload, error) {
	data, err := r.client.HGet(r.ctx, "github:status:desired", sha).Result()
	if err != nil {
		return nil, err
	}

	var payload GithubStatusPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// RemoveCommitStatusSync removes a commit hash from the sync set and deletes its desired status.
func (r *RedisService) RemoveCommitStatusSync(sha string) error {
	pipe := r.client.Pipeline()
	pipe.HDel(r.ctx, "github:status:desired", sha)
	pipe.SRem(r.ctx, "github:status:sync_set", sha)
	_, err := pipe.Exec(r.ctx)
	return err
}

// RemoveCommitStatusSyncIfMatched transactionally removes a commit hash from the sync set only if the stored payload's CreatedAt timestamp matches.
func (r *RedisService) RemoveCommitStatusSyncIfMatched(sha string, createdAt int64) (bool, error) {
	keyDesired := "github:status:desired"
	keySet := "github:status:sync_set"

	txf := func(tx *redis.Tx) error {
		data, err := tx.HGet(r.ctx, keyDesired, sha).Result()
		if err == redis.Nil {
			_, err = tx.TxPipelined(r.ctx, func(pipe redis.Pipeliner) error {
				pipe.SRem(r.ctx, keySet, sha)
				return nil
			})
			return err
		} else if err != nil {
			return err
		}

		var payload GithubStatusPayload
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			_, err = tx.TxPipelined(r.ctx, func(pipe redis.Pipeliner) error {
				pipe.HDel(r.ctx, keyDesired, sha)
				pipe.SRem(r.ctx, keySet, sha)
				return nil
			})
			return err
		}

		if payload.CreatedAt == createdAt {
			_, err = tx.TxPipelined(r.ctx, func(pipe redis.Pipeliner) error {
				pipe.HDel(r.ctx, keyDesired, sha)
				pipe.SRem(r.ctx, keySet, sha)
				return nil
			})
			return err
		}

		return nil
	}

	err := r.client.Watch(r.ctx, txf, keyDesired)
	if err != nil {
		return false, err
	}
	return true, nil
}
