package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequestSecurityHeaders(t *testing.T) {
	app := fiber.New()
	app.Use(RequestSecurity())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}

	expectedHeaders := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "geolocation=(), microphone=(), camera=()",
	}
	for name, expected := range expectedHeaders {
		if actual := resp.Header.Get(name); actual != expected {
			t.Errorf("%s = %q, want %q", name, actual, expected)
		}
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("X-Request-ID is missing")
	}

	policy := resp.Header.Get("Content-Security-Policy-Report-Only")
	if policy == "" {
		t.Fatal("Content-Security-Policy-Report-Only is missing")
	}
	if enforced := resp.Header.Get("Content-Security-Policy"); enforced != "" {
		t.Fatalf("Content-Security-Policy must remain disabled during report-only rollout, got %q", enforced)
	}

	expectedDirectives := map[string]string{
		"default-src":     "default-src 'self'",
		"base-uri":        "base-uri 'self'",
		"object-src":      "object-src 'none'",
		"frame-ancestors": "frame-ancestors 'none'",
		"form-action":     "form-action 'self'",
		"script-src":      "script-src 'self'",
		"style-src":       "style-src 'self' 'unsafe-inline'",
		"img-src":         "img-src 'self' data: blob:",
		"font-src":        "font-src 'self' data:",
		"connect-src":     "connect-src 'self'",
		"manifest-src":    "manifest-src 'self'",
	}
	for name, expected := range expectedDirectives {
		if actual := cspDirective(policy, name); actual != expected {
			t.Errorf("CSP directive %s = %q, want %q", name, actual, expected)
		}
	}

	scriptPolicy := cspDirective(policy, "script-src")
	for _, forbidden := range []string{"'unsafe-inline'", "'unsafe-eval'", "data:", "*"} {
		if strings.Contains(scriptPolicy, forbidden) {
			t.Errorf("script-src must not contain %q: %q", forbidden, scriptPolicy)
		}
	}
}

func cspDirective(policy, name string) string {
	for _, directive := range strings.Split(policy, ";") {
		directive = strings.TrimSpace(directive)
		if directive == name || strings.HasPrefix(directive, name+" ") {
			return directive
		}
	}
	return ""
}

func TestMaxBody(t *testing.T) {
	app := fiber.New()
	app.Post("/auth", MaxBody(8), func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader("123456789")))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
