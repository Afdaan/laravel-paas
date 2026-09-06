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
	if resp.Header.Get("X-Request-ID") == "" || resp.Header.Get("X-Frame-Options") != "DENY" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing headers: %#v", resp.Header)
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
