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
