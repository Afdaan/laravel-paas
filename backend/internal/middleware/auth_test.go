package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestCookieNamesAndOrigin(t *testing.T) {
	production := &config.Config{AppEnv: "production"}
	session, admin, csrf := CookieNames(production)
	if session != "__Host-paas_session" || admin != "__Host-paas_admin_session" || csrf != "__Host-paas_csrf" {
		t.Fatalf("production cookies = %q %q %q", session, admin, csrf)
	}
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		if !validOriginURL(c, "https://console.example.com", "production") {
			return c.SendStatus(http.StatusForbidden)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	for _, origin := range []string{"", "https://apps.example.com", "https://console.example.com/path"} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q status = %d", origin, resp.StatusCode)
		}
	}
}

func TestRequireRecentBillingAuthentication(t *testing.T) {
	cfg := &config.Config{}
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		c.Locals("claims", &models.JWTClaims{AuthTime: jwt.NewNumericDate(time.Now().UTC())})
		c.Locals("token", "session-token")
		return RequireRecentBillingAuthentication(cfg)(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Cookie", "paas_session=session-token")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("fresh browser session status = %d", resp.StatusCode)
	}

	stale := fiber.New()
	stale.Post("/", func(c *fiber.Ctx) error {
		c.Locals("claims", &models.JWTClaims{AuthTime: jwt.NewNumericDate(time.Now().UTC().Add(-billingRecentAuthWindow - time.Second))})
		c.Locals("token", "session-token")
		if err := RequireRecentBillingAuthentication(cfg)(c); err != nil {
			return c.SendStatus(http.StatusForbidden)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err = stale.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stale browser session status = %d", resp.StatusCode)
	}

	// Legacy session with nil AuthTime
	legacy := fiber.New()
	legacy.Post("/", func(c *fiber.Ctx) error {
		c.Locals("claims", &models.JWTClaims{AuthTime: nil})
		c.Locals("token", "session-token")
		if err := RequireRecentBillingAuthentication(cfg)(c); err != nil {
			return c.SendStatus(http.StatusForbidden)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err = legacy.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("legacy session without auth_time status = %d", resp.StatusCode)
	}

	// Impersonated session is blocked
	impersonated := fiber.New()
	impersonated.Post("/", func(c *fiber.Ctx) error {
		c.Locals("impersonating", true)
		c.Locals("claims", &models.JWTClaims{AuthTime: jwt.NewNumericDate(time.Now().UTC())})
		c.Locals("token", "session-token")
		if err := RequireRecentBillingAuthentication(cfg)(c); err != nil {
			return c.SendStatus(http.StatusForbidden)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	resp, err = impersonated.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("impersonated session status = %d", resp.StatusCode)
	}
}
