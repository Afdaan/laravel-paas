// ===========================================
// Rate Limiting Middleware
// ===========================================
// Protects endpoints from brute-force and abuse
// ===========================================
package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
)

type distributedRateLimiter interface {
	RateLimit(key string, limit int, duration time.Duration) (bool, error)
}

// RateLimiter implements a sliding window rate limiter
type RateLimiter struct {
	clients map[string]*clientRecord
	mu      sync.Mutex
	max     int
	window  time.Duration
}

type clientRecord struct {
	count       int
	windowStart time.Time
}

// NewRateLimiter creates a rate limiter with the specified max requests per window
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientRecord),
		max:     max,
		window:  window,
	}

	// Cleanup goroutine
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, record := range rl.clients {
			if now.Sub(record.windowStart) > rl.window {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.clients[ip]

	if !exists || now.Sub(record.windowStart) > rl.window {
		rl.clients[ip] = &clientRecord{
			count:       1,
			windowStart: now,
		}
		return true
	}

	if record.count >= rl.max {
		return false
	}

	record.count++
	return true
}

// Global rate limiters for different endpoint categories
var (
	loginLimiter     = NewRateLimiter(5, 1*time.Minute)  // 5 req/min per IP
	queryLimiter     = NewRateLimiter(10, 1*time.Minute) // 10 req/min per user
	proxyLimiter     = NewRateLimiter(60, 1*time.Minute) // 60 req/min per IP
	consoleLimiter   = NewRateLimiter(5, 1*time.Minute)  // 5 req/min per project
	importLimiter    = NewRateLimiter(3, 5*time.Minute)  // 3 req/5min per user
	autoRenewLimiter = NewRateLimiter(10, 1*time.Minute) // 10 req/min per user
)

// RateLimitLogin applies rate limiting to login endpoint
func RateLimitLogin(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()

		var req struct {
			Email string `json:"email"`
		}
		_ = c.BodyParser(&req)

		emailHash := hashRateLimitPart(req.Email)
		keys := []string{
			"auth:login:ip:" + ip,
		}
		if emailHash != "" {
			keys = append(keys, "auth:login:email:"+emailHash, "auth:login:ip-email:"+ip+":"+emailHash)
		}

		if redis != nil {
			for _, key := range keys {
				allowed, err := redis.RateLimit(key, 5, time.Minute)
				if err != nil {
					slog.Warn("Redis login rate limit failed; falling back to local limiter", "error", err)
					break
				}
				if !allowed {
					return apperr.New(429, "RATE_LIMITED", "Too many login attempts. Please try again later")
				}
			}
			return c.Next()
		}

		if !loginLimiter.Allow(ip + ":" + emailHash) {
			return apperr.New(429, "RATE_LIMITED", "Too many login attempts. Please try again later")
		}
		return c.Next()
	}
}

func hashRateLimitPart(value string) string {
	if value == "" {
		return ""
	}

	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// RateLimitQuery applies rate limiting to database query endpoint
func RateLimitQuery() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := c.IP()
		if !queryLimiter.Allow(key) {
			return apperr.New(429, "RATE_LIMITED", "Too many database queries. Please slow down")
		}
		return c.Next()
	}
}

// RateLimitProxy applies rate limiting to proxy endpoint
func RateLimitProxy() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		if !proxyLimiter.Allow(ip) {
			return apperr.New(429, "RATE_LIMITED", "Rate limit exceeded for this project")
		}
		return c.Next()
	}
}

// RateLimitConsole applies rate limiting to project console command execution.
func RateLimitConsole() fiber.Handler {
	return func(c *fiber.Ctx) error {
		projectID := c.Params("id")
		key := "console:" + c.IP() + ":" + projectID
		if !consoleLimiter.Allow(key) {
			return apperr.New(429, "RATE_LIMITED", "Too many command executions. Please wait")
		}
		return c.Next()
	}
}

// RateLimitImport applies rate limiting to import endpoints
func RateLimitImport() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := c.IP()
		if !importLimiter.Allow(key) {
			return apperr.New(429, "RATE_LIMITED", "Too many import attempts. Please try again later")
		}
		return c.Next()
	}
}

// RateLimitAutoRenew applies rate limiting to auto-renew toggle endpoint
func RateLimitAutoRenew() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := "autorenew:" + c.IP()
		if !autoRenewLimiter.Allow(key) {
			return apperr.New(429, "RATE_LIMITED", "Too many auto-renew toggle requests. Please wait a moment")
		}
		return c.Next()
	}
}
