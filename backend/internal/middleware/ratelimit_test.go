package middleware

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiterRemainingSeconds(t *testing.T) {
	rl := NewRateLimiter(2, 60*time.Second)

	allowed, sec := rl.Allow("user1")
	assert.True(t, allowed)
	assert.Equal(t, 0, sec)

	allowed, sec = rl.Allow("user1")
	assert.True(t, allowed)
	assert.Equal(t, 0, sec)

	allowed, sec = rl.Allow("user1")
	assert.False(t, allowed)
	assert.GreaterOrEqual(t, sec, 58)
	assert.LessOrEqual(t, sec, 60)
}

func TestRateLimitLoginMiddlewareHeaderAndErrorMsg(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{
					"error": ae.Message,
					"code":  ae.Code,
				})
			}
			return c.SendStatus(500)
		},
	})
	app.Post("/login", RateLimitLogin(nil), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"test@example.com"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}

	// 6th request should fail with 429
	req := httptest.NewRequest("POST", "/login", strings.NewReader(`{"email":"test@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
}

type mockLimiter struct {
	allowed bool
	ttl     time.Duration
	err     error
}

func (m *mockLimiter) RateLimit(key string, limit int, duration time.Duration) (bool, time.Duration, error) {
	return m.allowed, m.ttl, m.err
}

func TestRateLimitMidtransWebhookDistributedAndFallback(t *testing.T) {
	// 1. Distributed allowed path
	appAllowed := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appAllowed.Post("/webhook/midtrans", RateLimitMidtransWebhook(&mockLimiter{allowed: true, ttl: 0, err: nil}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/webhook/midtrans", nil)
	resp, err := appAllowed.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Distributed blocked path (returns 429 and sets Retry-After)
	appDenied := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appDenied.Post("/webhook/midtrans", RateLimitMidtransWebhook(&mockLimiter{allowed: false, ttl: 45 * time.Second, err: nil}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/webhook/midtrans", nil)
	resp, err = appDenied.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "45", resp.Header.Get("Retry-After"))

	// 3. Distributed Redis error falls back gracefully to local limiter
	appFallback := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appFallback.Post("/webhook/midtrans", RateLimitMidtransWebhook(&mockLimiter{err: assert.AnError}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/webhook/midtrans", nil)
	resp, err = appFallback.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRateLimitPakasirWebhookDistributedAndFallback(t *testing.T) {
	// 1. Distributed allowed path
	appAllowed := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appAllowed.Post("/webhook/pakasir", RateLimitPakasirWebhook(&mockLimiter{allowed: true, ttl: 0, err: nil}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/webhook/pakasir", nil)
	resp, err := appAllowed.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Distributed blocked path (returns 429 and sets Retry-After)
	appDenied := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appDenied.Post("/webhook/pakasir", RateLimitPakasirWebhook(&mockLimiter{allowed: false, ttl: 30 * time.Second, err: nil}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/webhook/pakasir", nil)
	resp, err = appDenied.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "30", resp.Header.Get("Retry-After"))

	// 3. Distributed Redis error falls back gracefully to local limiter
	appFallback := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appFallback.Post("/webhook/pakasir", RateLimitPakasirWebhook(&mockLimiter{err: assert.AnError}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/webhook/pakasir", nil)
	resp, err = appFallback.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRateLimitLocalWebhookLimiterThresholdAndDeny(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)

	allowed, sec := limiter.Allow("test-ip-1")
	assert.True(t, allowed)
	assert.Equal(t, 0, sec)

	allowed, sec = limiter.Allow("test-ip-1")
	assert.True(t, allowed)
	assert.Equal(t, 0, sec)

	// Exceeded limit: returns false and non-zero retry seconds
	allowed, sec = limiter.Allow("test-ip-1")
	assert.False(t, allowed)
	assert.Greater(t, sec, 0)
	assert.LessOrEqual(t, sec, 60)
}

func TestRateLimitTopupCreateMiddleware(t *testing.T) {
	// 1. Allowed path
	appAllowed := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appAllowed.Post("/billing/topup", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(42))
		return c.Next()
	}, RateLimitTopupCreate(&mockLimiter{allowed: true}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/billing/topup", nil)
	resp, err := appAllowed.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Denied path
	appDenied := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appDenied.Post("/billing/topup", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(42))
		return c.Next()
	}, RateLimitTopupCreate(&mockLimiter{allowed: false, ttl: 45 * time.Second}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/billing/topup", nil)
	resp, err = appDenied.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "45", resp.Header.Get("Retry-After"))
}

func TestRateLimitTopupReconcileMiddleware(t *testing.T) {
	// 1. Allowed path
	appAllowed := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appAllowed.Post("/billing/topup/:id/reconcile", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(42))
		return c.Next()
	}, RateLimitTopupReconcile(&mockLimiter{allowed: true}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/billing/topup/10/reconcile", nil)
	resp, err := appAllowed.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Denied path
	appDenied := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appDenied.Post("/billing/topup/:id/reconcile", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(42))
		return c.Next()
	}, RateLimitTopupReconcile(&mockLimiter{allowed: false, ttl: 20 * time.Second}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req = httptest.NewRequest("POST", "/billing/topup/10/reconcile", nil)
	resp, err = appDenied.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 429, resp.StatusCode)
	assert.Equal(t, "20", resp.Header.Get("Retry-After"))
}

type conditionalMockLimiter struct {
	userAllowed bool
	userErr     error
	ipAllowed   bool
	ipErr       error
}

func (c *conditionalMockLimiter) RateLimit(key string, limit int, duration time.Duration) (bool, time.Duration, error) {
	if strings.Contains(key, ":user:") {
		return c.userAllowed, 30 * time.Second, c.userErr
	}
	return c.ipAllowed, 15 * time.Second, c.ipErr
}

func TestRateLimitTopupCreateRedisPartialAndFullFallback(t *testing.T) {
	// 1. Redis user check succeeds, but Redis IP check errors -> falls back to local IP limiter
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	app.Post("/billing/topup", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(88))
		return c.Next()
	}, RateLimitTopupCreate(&conditionalMockLimiter{userAllowed: true, userErr: nil, ipAllowed: true, ipErr: assert.AnError}), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/billing/topup", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 2. Full Redis nil fallback -> local user & local IP limiters are applied
	appLocal := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appLocal.Post("/billing/topup", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(99))
		return c.Next()
	}, RateLimitTopupCreate(nil), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	reqLocal := httptest.NewRequest("POST", "/billing/topup", nil)
	respLocal, errLocal := appLocal.Test(reqLocal)
	assert.NoError(t, errLocal)
	assert.Equal(t, 200, respLocal.StatusCode)

	// 3. User threshold exhaustion with nil Redis
	appUserExhaust := fiber.New(fiber.Config{
		ProxyHeader: fiber.HeaderXForwardedFor,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	appUserExhaust.Post("/billing/topup", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(777))
		return c.Next()
	}, RateLimitTopupCreate(nil), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 1; i <= 5; i++ {
		req := httptest.NewRequest("POST", "/billing/topup", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i)) // Different IPs so only user threshold hits
		resp, err := appUserExhaust.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "expected request %d to be allowed", i)
	}
	// 6th request for user 777 exceeds topupCreateLocalLimiter (limit = 5)
	reqExhaust := httptest.NewRequest("POST", "/billing/topup", nil)
	reqExhaust.Header.Set("X-Forwarded-For", "198.51.100.99")
	respExhaust, err := appUserExhaust.Test(reqExhaust)
	assert.NoError(t, err)
	assert.Equal(t, 429, respExhaust.StatusCode)

	// 4. IP threshold exhaustion with nil Redis
	appIPExhaust := fiber.New(fiber.Config{
		ProxyHeader: fiber.HeaderXForwardedFor,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	targetIP := "203.0.113.50"
	var reqUserCounter uint32
	appIPExhaust.Post("/billing/topup", func(c *fiber.Ctx) error {
		uid := atomic.AddUint32(&reqUserCounter, 1)
		c.Locals("user_id", uint(50000+uid))
		return c.Next()
	}, RateLimitTopupCreate(nil), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 1; i <= 15; i++ {
		req := httptest.NewRequest("POST", "/billing/topup", nil)
		req.Header.Set("X-Forwarded-For", targetIP)
		resp, err := appIPExhaust.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "expected IP request %d to be allowed", i)
	}
	// 16th request from same IP exceeds topupCreateIPLimiter (limit = 15)
	reqIPExhaust := httptest.NewRequest("POST", "/billing/topup", nil)
	reqIPExhaust.Header.Set("X-Forwarded-For", targetIP)
	respIPExhaust, err := appIPExhaust.Test(reqIPExhaust)
	assert.NoError(t, err)
	assert.Equal(t, 429, respIPExhaust.StatusCode)
}

func TestRateLimitTopupReconcileRedisFallbackExhaustion(t *testing.T) {
	app := fiber.New(fiber.Config{
		ProxyHeader: fiber.HeaderXForwardedFor,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			if ae, ok := err.(*apperr.AppError); ok {
				return c.Status(ae.HTTPStatus).JSON(fiber.Map{"error": ae.Message})
			}
			return c.SendStatus(500)
		},
	})
	app.Post("/billing/reconcile", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(888))
		return c.Next()
	}, RateLimitTopupReconcile(nil), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// User threshold (limit = 10)
	for i := 1; i <= 10; i++ {
		req := httptest.NewRequest("POST", "/billing/reconcile", nil)
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i))
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode, "expected reconcile request %d to be allowed", i)
	}
	reqExhaust := httptest.NewRequest("POST", "/billing/reconcile", nil)
	reqExhaust.Header.Set("X-Forwarded-For", "198.51.100.99")
	respExhaust, err := app.Test(reqExhaust)
	assert.NoError(t, err)
	assert.Equal(t, 429, respExhaust.StatusCode)
}
