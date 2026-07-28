package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
)

func TestCookieNamesAndOrigin(t *testing.T) {
	production := &config.Config{AppEnv: "production"}
	session, admin, csrf := CookieNames(production)
	if session != "__Host-paas_session" || admin != "__Host-paas_admin_session" || csrf != "__Host-paas_csrf" {
		t.Fatalf("production cookies = %q %q %q", session, admin, csrf)
	}
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		if !validOrigin(c, "https://console.example.com") {
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
