package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
)

const requestIDLocal = "request_id"

// Inline styles remain allowed for dynamic React layouts. Scripts stay
// same-origin only so injected inline or third-party JavaScript is reported.
const contentSecurityPolicy = "default-src 'self'; " +
	"base-uri 'self'; " +
	"object-src 'none'; " +
	"frame-ancestors 'none'; " +
	"form-action 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"manifest-src 'self'"

func RequestSecurity() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return err
		}
		requestID := hex.EncodeToString(bytes[:])
		c.Locals(requestIDLocal, requestID)
		c.Set("X-Request-ID", requestID)
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderReferrerPolicy, "strict-origin-when-cross-origin")
		c.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Set("Content-Security-Policy-Report-Only", contentSecurityPolicy)
		return c.Next()
	}
}

func RequestID(c *fiber.Ctx) string {
	requestID, _ := c.Locals(requestIDLocal).(string)
	return requestID
}

func NoStore() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Next()
	}
}

func MaxBody(bytes int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(c.Body()) > bytes {
			return fiber.ErrRequestEntityTooLarge
		}
		return c.Next()
	}
}
