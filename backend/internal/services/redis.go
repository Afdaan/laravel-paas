// ===========================================
// Redis Service
// ===========================================
// Handles Redis connections and operations
// ===========================================
package services

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
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       0, // use default DB
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
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "total_enqueued", 1)

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

	// Increment processed counter
	r.client.HIncrBy(r.ctx, deploymentStatsKey, "total_processed", 1)

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
