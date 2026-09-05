// ===========================================
// Rate Limiting Middleware
// ===========================================
// Protects endpoints from brute-force and abuse
// ===========================================
package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
)

type distributedRateLimiter interface {
	RateLimit(key string, limit int, duration time.Duration) (bool, time.Duration, error)
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

func (rl *RateLimiter) Allow(ip string) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.clients[ip]

	if !exists || now.Sub(record.windowStart) > rl.window {
		rl.clients[ip] = &clientRecord{
			count:       1,
			windowStart: now,
		}
		return true, 0
	}

	if record.count >= rl.max {
		remaining := rl.window - now.Sub(record.windowStart)
		sec := int(math.Ceil(remaining.Seconds()))
		if sec < 1 {
			sec = 1
		}
		return false, sec
	}

	record.count++
	return true, 0
}

func formatRateLimitMsg(baseMsg string, sec int) string {
	if sec <= 1 {
		return fmt.Sprintf("%s. Please try again in 1 second.", baseMsg)
	}

	return fmt.Sprintf("%s. Please try again in %d seconds.", baseMsg, sec)
}

// Global rate limiters for different endpoint categories
var (
	loginIPLimiter            = NewRateLimiter(30, 1*time.Minute)  // 30 req/min per IP
	loginEmailLimiter         = NewRateLimiter(5, 1*time.Minute)   // 5 req/min per email
	reauthUserLimiter         = NewRateLimiter(5, 1*time.Minute)   // 5 req/min per user
	reauthIPLimiter           = NewRateLimiter(15, 1*time.Minute)  // 15 req/min per IP
	queryLimiter              = NewRateLimiter(60, 1*time.Minute)  // 60 req/min per user
	publicCatalogLimiter      = NewRateLimiter(120, 1*time.Minute) // 120 req/min per IP
	proxyLimiter              = NewRateLimiter(120, 1*time.Minute) // 120 req/min per IP
	consoleLimiter            = NewRateLimiter(30, 1*time.Minute)  // 30 req/min per project/user
	importLimiter             = NewRateLimiter(10, 5*time.Minute)  // 10 req/5min per user
	autoRenewLimiter          = NewRateLimiter(30, 1*time.Minute)  // 30 req/min per user
	topupCreateUserLimiter    = NewRateLimiter(10, 1*time.Minute)  // 10 req/min per user
	topupCreateIPLimiter      = NewRateLimiter(30, 1*time.Minute)  // 30 req/min per IP
	topupReconcileUserLimiter = NewRateLimiter(30, 1*time.Minute)  // 30 req/min per user
	topupReconcileIPLimiter   = NewRateLimiter(60, 1*time.Minute)  // 60 req/min per IP
	midtransWebhookLimiter    = NewRateLimiter(300, 1*time.Minute) // 300 req/min per IP
	pakasirWebhookLimiter     = NewRateLimiter(300, 1*time.Minute) // 300 req/min per IP
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

		type limitRule struct {
			key      string
			limit    int
			localKey string
			limiter  *RateLimiter
			errMsg   string
		}

		rules := []limitRule{
			{
				key:      "auth:login:ip:" + ip,
				limit:    30,
				localKey: "ip:" + ip,
				limiter:  loginIPLimiter,
				errMsg:   "Too many login attempts from this IP",
			},
		}
		if emailHash != "" {
			rules = append(rules,
				limitRule{
					key:      "auth:login:email:" + emailHash,
					limit:    5,
					localKey: "email:" + emailHash,
					limiter:  loginEmailLimiter,
					errMsg:   "Too many login attempts for this account",
				},
				limitRule{
					key:      "auth:login:ip-email:" + ip + ":" + emailHash,
					limit:    5,
					localKey: "ip-email:" + ip + ":" + emailHash,
					limiter:  loginEmailLimiter,
					errMsg:   "Too many login attempts for this account",
				},
			)
		}

		if redis != nil {
			for _, rule := range rules {
				allowed, ttl, err := redis.RateLimit(rule.key, rule.limit, time.Minute)
				if err != nil {
					slog.Warn("Redis login rate limit failed; falling back to local limiter", "error", err, "key", rule.key)
					allowedLocal, sec := rule.limiter.Allow(rule.localKey)
					if !allowedLocal {
						c.Set("Retry-After", strconv.Itoa(sec))
						return apperr.NewRateLimited(formatRateLimitMsg(rule.errMsg, sec), sec)
					}
					continue
				}
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg(rule.errMsg, sec), sec)
				}
			}
			return c.Next()
		}

		for _, rule := range rules {
			allowed, sec := rule.limiter.Allow(rule.localKey)
			if !allowed {
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg(rule.errMsg, sec), sec)
			}
		}

		return c.Next()
	}
}

// RateLimitReauth applies rate limiting specifically to the re-authentication endpoint by user_id and IP.
func RateLimitReauth(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		ip := c.IP()
		userKey := fmt.Sprintf("auth:reauth:user:%v", uidVal)
		ipKey := fmt.Sprintf("auth:reauth:ip:%s", ip)
		localIPKey := "ip:" + ip

		if redis != nil {
			allowedUser, ttlUser, errUser := redis.RateLimit(userKey, 5, time.Minute)
			if errUser != nil {
				slog.Warn("Redis reauth rate limit failed; falling back to local limiter", "error", errUser)
				allowedLocal, sec := reauthUserLimiter.Allow(userKey)
				if !allowedLocal {
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts", sec), sec)
				}
			} else if !allowedUser {
				sec := int(math.Ceil(ttlUser.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts", sec), sec)
			}

			allowedIP, ttlIP, errIP := redis.RateLimit(ipKey, 15, time.Minute)
			if errIP != nil {
				slog.Warn("Redis reauth IP rate limit failed; falling back to local limiter", "error", errIP)
				allowedLocalIP, secIP := reauthIPLimiter.Allow(localIPKey)
				if !allowedLocalIP {
					c.Set("Retry-After", strconv.Itoa(secIP))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts from this IP", secIP), secIP)
				}
			} else if !allowedIP {
				sec := int(math.Ceil(ttlIP.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts from this IP", sec), sec)
			}
			return c.Next()
		}

		allowedUser, secUser := reauthUserLimiter.Allow(userKey)
		if !allowedUser {
			c.Set("Retry-After", strconv.Itoa(secUser))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts", secUser), secUser)
		}

		allowedIP, secIP := reauthIPLimiter.Allow(localIPKey)
		if !allowedIP {
			c.Set("Retry-After", strconv.Itoa(secIP))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many re-authentication attempts from this IP", secIP), secIP)
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
func RateLimitQuery(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := fmt.Sprintf("query:user:%v", uidVal)

		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 60, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many database queries", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis query rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := queryLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many database queries", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitPublicCatalog limits anonymous catalog reads by client IP.
func RateLimitPublicCatalog(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		key := "catalog:public:ip:" + c.IP()
		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 120, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many catalog requests", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis public catalog rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := publicCatalogLimiter.Allow(c.IP())
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many catalog requests", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitProxy applies rate limiting to proxy endpoint
func RateLimitProxy(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		key := "proxy:ip:" + ip
		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 120, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Rate limit exceeded for this project", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis proxy rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := proxyLimiter.Allow(ip)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Rate limit exceeded for this project", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitConsole applies rate limiting to project console command execution.
func RateLimitConsole(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		projectID := c.Params("id")
		uidVal := c.Locals("user_id")
		var key string
		if uidVal != nil {
			key = fmt.Sprintf("console:user:%v:proj:%s", uidVal, projectID)
		} else {
			key = fmt.Sprintf("console:ip:%s:proj:%s", c.IP(), projectID)
		}

		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 30, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many command executions", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis console rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := consoleLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many command executions", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitImport applies rate limiting to import endpoints
func RateLimitImport(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := fmt.Sprintf("import:user:%v", uidVal)

		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 10, 5*time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many import attempts", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis import rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := importLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many import attempts", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitAutoRenew applies rate limiting to auto-renew toggle endpoint
func RateLimitAutoRenew(redis ...distributedRateLimiter) fiber.Handler {
	var r distributedRateLimiter
	if len(redis) > 0 {
		r = redis[0]
	}
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		key := fmt.Sprintf("autorenew:user:%v", uidVal)

		if r != nil {
			allowed, ttl, err := r.RateLimit(key, 30, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many auto-renew toggle requests", sec), sec)
				}
				return c.Next()
			}
			slog.Warn("Redis autorenew rate limit failed; falling back to local limiter", "error", err)
		}

		allowed, sec := autoRenewLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many auto-renew toggle requests", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitMidtransWebhook applies provider-aware rate limiting for Midtrans webhooks
func RateLimitMidtransWebhook(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		key := "webhook:midtrans:ip:" + ip
		if redis != nil {
			allowed, ttl, err := redis.RateLimit(key, 300, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many webhook requests", sec), sec)
				}
				return c.Next()
			}
		}

		allowed, sec := midtransWebhookLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many webhook requests", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitPakasirWebhook applies provider-aware rate limiting for Pakasir webhooks
func RateLimitPakasirWebhook(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ip := c.IP()
		key := "webhook:pakasir:ip:" + ip
		if redis != nil {
			allowed, ttl, err := redis.RateLimit(key, 300, time.Minute)
			if err == nil {
				if !allowed {
					sec := int(math.Ceil(ttl.Seconds()))
					if sec < 1 {
						sec = 1
					}
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many webhook requests", sec), sec)
				}
				return c.Next()
			}
		}

		allowed, sec := pakasirWebhookLimiter.Allow(key)
		if !allowed {
			c.Set("Retry-After", strconv.Itoa(sec))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many webhook requests", sec), sec)
		}
		return c.Next()
	}
}

// RateLimitTopupCreate applies distributed and local rate limiting to topup creation
func RateLimitTopupCreate(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		ip := c.IP()
		userKey := fmt.Sprintf("topup:create:user:%v", uidVal)
		ipKey := fmt.Sprintf("topup:create:ip:%s", ip)

		if redis != nil {
			allowedUser, ttlUser, errUser := redis.RateLimit(userKey, 10, time.Minute)
			if errUser != nil {
				slog.Warn("Redis topup create user rate limit failed; falling back to local limiter", "error", errUser)
				allowedLocal, sec := topupCreateUserLimiter.Allow(userKey)
				if !allowedLocal {
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests", sec), sec)
				}
			} else if !allowedUser {
				sec := int(math.Ceil(ttlUser.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests", sec), sec)
			}

			allowedIP, ttlIP, errIP := redis.RateLimit(ipKey, 30, time.Minute)
			if errIP != nil {
				slog.Warn("Redis topup create IP rate limit failed; falling back to local limiter", "error", errIP)
				allowedLocal, sec := topupCreateIPLimiter.Allow(ipKey)
				if !allowedLocal {
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests from this IP", sec), sec)
				}
			} else if !allowedIP {
				sec := int(math.Ceil(ttlIP.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests from this IP", sec), sec)
			}
			return c.Next()
		}

		allowedUser, secUser := topupCreateUserLimiter.Allow(userKey)
		if !allowedUser {
			c.Set("Retry-After", strconv.Itoa(secUser))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests", secUser), secUser)
		}
		allowedIP, secIP := topupCreateIPLimiter.Allow(ipKey)
		if !allowedIP {
			c.Set("Retry-After", strconv.Itoa(secIP))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many top-up requests from this IP", secIP), secIP)
		}

		return c.Next()
	}
}

// RateLimitTopupReconcile applies distributed and local rate limiting to topup reconciliation
func RateLimitTopupReconcile(redis distributedRateLimiter) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidVal := c.Locals("user_id")
		if uidVal == nil {
			return apperr.ErrUnauthorized
		}
		ip := c.IP()
		userKey := fmt.Sprintf("topup:reconcile:user:%v", uidVal)
		ipKey := fmt.Sprintf("topup:reconcile:ip:%s", ip)

		if redis != nil {
			allowedUser, ttlUser, errUser := redis.RateLimit(userKey, 30, time.Minute)
			if errUser != nil {
				slog.Warn("Redis topup reconcile user rate limit failed; falling back to local limiter", "error", errUser)
				allowedLocal, sec := topupReconcileUserLimiter.Allow(userKey)
				if !allowedLocal {
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests", sec), sec)
				}
			} else if !allowedUser {
				sec := int(math.Ceil(ttlUser.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests", sec), sec)
			}

			allowedIP, ttlIP, errIP := redis.RateLimit(ipKey, 60, time.Minute)
			if errIP != nil {
				slog.Warn("Redis topup reconcile IP rate limit failed; falling back to local limiter", "error", errIP)
				allowedLocal, sec := topupReconcileIPLimiter.Allow(ipKey)
				if !allowedLocal {
					c.Set("Retry-After", strconv.Itoa(sec))
					return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests from this IP", sec), sec)
				}
			} else if !allowedIP {
				sec := int(math.Ceil(ttlIP.Seconds()))
				if sec < 1 {
					sec = 1
				}
				c.Set("Retry-After", strconv.Itoa(sec))
				return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests from this IP", sec), sec)
			}
			return c.Next()
		}

		allowedUser, secUser := topupReconcileUserLimiter.Allow(userKey)
		if !allowedUser {
			c.Set("Retry-After", strconv.Itoa(secUser))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests", secUser), secUser)
		}
		allowedIP, secIP := topupReconcileIPLimiter.Allow(ipKey)
		if !allowedIP {
			c.Set("Retry-After", strconv.Itoa(secIP))
			return apperr.NewRateLimited(formatRateLimitMsg("Too many reconciliation requests from this IP", secIP), secIP)
		}

		return c.Next()
	}
}
