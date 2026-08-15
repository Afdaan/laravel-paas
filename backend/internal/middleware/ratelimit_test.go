package middleware

import (
	"net/http/httptest"
	"strings"
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
