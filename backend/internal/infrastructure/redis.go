// ===========================================
// Redis Service
// ===========================================
// Handles Redis connections and operations
// ===========================================
package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/laravel-paas/backend/internal/config"
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
		DB:           0, // use default DB
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
	ProjectID   uint      `json:"project_id"`
	UserID      uint      `json:"user_id"`
	Type        string    `json:"type"` // "deploy" or "redeploy"
	EnqueuedAt  time.Time `json:"enqueued_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
}

const (
	deploymentQueueKey = "deployment:queue"
	deploymentLockKey  = "deployment:lock"
	deploymentStatsKey = "deployment:stats"
)

// EnqueueDeployment adds a deployment job to the queue
func (r *RedisService) EnqueueDeployment(projectID, userID uint, deployType string) error {
	job := DeploymentJob{
		ProjectID:  projectID,
		UserID:     userID,
		Type:       deployType,
		EnqueuedAt: time.Now(),
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Add to the queue
	if err := r.client.RPush(r.ctx, deploymentQueueKey, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	// Increment enqueued counter
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

// AcquireDeploymentLock tries to acquire a distributed lock for deployment
func (r *RedisService) AcquireDeploymentLock(projectID uint, ttl time.Duration) (bool, error) {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	
	// Try to set the lock with NX (only if not exists) and expiration
	ok, err := r.client.SetNX(r.ctx, lockKey, time.Now().Unix(), ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	
	return ok, nil
}

// ReleaseDeploymentLock releases the deployment lock
func (r *RedisService) ReleaseDeploymentLock(projectID uint) error {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	
	if err := r.client.Del(r.ctx, lockKey).Err(); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	
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

// RemoveFromQueue removes a specific project from the deployment queue
func (r *RedisService) RemoveFromQueue(projectID uint) error {
	results, err := r.client.LRange(r.ctx, deploymentQueueKey, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to read queue: %w", err)
	}

	for _, res := range results {
		var job DeploymentJob
		if err := json.Unmarshal([]byte(res), &job); err == nil {
			if job.ProjectID == projectID {
				if err := r.client.LRem(r.ctx, deploymentQueueKey, 1, res).Err(); err != nil {
					return fmt.Errorf("failed to remove from redis list: %w", err)
				}
				break
			}
		}
	}
	return nil
}

// RenewDeploymentLock resets the TTL of an active deployment lock
func (r *RedisService) RenewDeploymentLock(projectID uint, ttl time.Duration) error {
	lockKey := fmt.Sprintf("%s:%d", deploymentLockKey, projectID)
	return r.client.Expire(r.ctx, lockKey, ttl).Err()
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

