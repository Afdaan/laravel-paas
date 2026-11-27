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
