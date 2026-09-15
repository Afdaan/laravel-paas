package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
)

func TestInternalOnlyRequiresCapability(t *testing.T) {
	cfg := &config.Config{InternalAPIToken: "abcdefghijklmnopqrstuvwxyz123456"}
	app := fiber.New()
	app.Get("/internal", InternalOnly(cfg), func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"missing", "", http.StatusForbidden},
		{"wrong", "wrong", http.StatusForbidden},
		{"trusted proxy external client", "", http.StatusForbidden},
		{"valid", cfg.InternalAPIToken, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal", nil)
			req.RemoteAddr = "172.18.0.2:1234"
			req.Header.Set("X-Forwarded-For", "203.0.113.9")
			if tc.token != "" {
				req.Header.Set(internalAPIHeader, tc.token)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == http.StatusNoContent && resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			if tc.want == http.StatusForbidden && resp.StatusCode == http.StatusNoContent {
				t.Fatal("unauthorized internal request accepted")
			}
		})
	}
}
