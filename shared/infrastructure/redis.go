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
	ProjectID  uint       `json:"project_id"`
	UserID     uint       `json:"user_id"`
	Type       string     `json:"type"` // "deploy" or "redeploy"
	EnqueuedAt time.Time  `json:"enqueued_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	JobID      string     `json:"job_id"`
	RetryCount int        `json:"retry_count"`
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
	deploymentQueueKey        = "deployment:queue"
	deploymentDelayedQueueKey = "deployment:delayed_queue"
	deploymentLockKey         = "deployment:lock"
	deploymentStatsKey        = "deployment:stats"
	deploymentLeaseKeyPrefix  = "deployment:lease"
)

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

// DequeueDeployment removes and returns the next job from the queue
func (r *RedisService) DequeueDeployment(timeout time.Duration) (*DeploymentJob, error) {
	// Blocking left pop with timeout
	result, err := r.client.BLPop(r.ctx, timeout, deploymentQueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No jobs available
		}
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid queue response")
	}

	var job DeploymentJob
	if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	now := time.Now()
	job.StartedAt = &now

	return &job, nil
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

// AddToBlacklist adds a token to the blacklist
func (r *RedisService) AddToBlacklist(token string, expiration time.Duration) error {
	key := fmt.Sprintf("blacklist:%s", token)
	return r.client.Set(r.ctx, key, true, expiration).Err()
}

// IsBlacklisted checks if a token is blacklisted
func (r *RedisService) IsBlacklisted(token string) bool {
	key := fmt.Sprintf("blacklist:%s", token)
	exists, err := r.client.Exists(r.ctx, key).Result()
	return err == nil && exists > 0
}

// RateLimit checks and increments a rate limit counter
func (r *RedisService) RateLimit(key string, limit int, duration time.Duration) (bool, error) {
	count, err := r.client.Get(r.ctx, key).Int()
	if err != nil && err != redis.Nil {
		return false, err
	}

	if count >= limit {
		return false, nil // Limit exceeded
	}

	r.client.Incr(r.ctx, key)
	if count == 0 {
		r.client.Expire(r.ctx, key, duration)
	}

	return true, nil
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

// PublishBuildLog streams a build log line to a Redis Pub/Sub channel
func (r *RedisService) PublishBuildLog(projectID uint, msg string) error {
	channel := fmt.Sprintf("channel:build_logs:%d", projectID)
	return r.client.Publish(r.ctx, channel, msg).Err()
}

// PublishDeploymentEvent streams a deployment lifecycle event to a Redis Pub/Sub channel
func (r *RedisService) PublishDeploymentEvent(projectID uint, eventJSON string) error {
	channel := fmt.Sprintf("channel:deployment_events:%d", projectID)
	return r.client.Publish(r.ctx, channel, eventJSON).Err()
}

// SubscribeBuildLogs subscribes to a build log channel and returns a Go channel of messages
func (r *RedisService) SubscribeBuildLogs(ctx context.Context, projectID uint) (<-chan string, error) {
	channel := fmt.Sprintf("channel:build_logs:%d", projectID)
	sub := r.client.Subscribe(r.ctx, channel)

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
	sub := r.client.Subscribe(r.ctx, channel)

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
	_ = r.RemoveFromQueue(job.ProjectID)

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
	return r.client.Set(r.ctx, key, string(data), ttl).Err()
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

// ReleaseDomainLock securely releases a domain lock verifying the unique fencing token.
func (r *RedisService) ReleaseDomainLock(domainID uint, token string) error {
	lockKey := fmt.Sprintf("%s:%d", domainLockKeyPrefix, domainID)
	_, err := releaseLockScript.Run(r.ctx, r.client, []string{lockKey}, token).Result()
	if err != nil {
		return fmt.Errorf("failed to release domain lock: %w", err)
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
