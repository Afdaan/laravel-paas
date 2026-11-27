package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
)

const requestIDLocal = "request_id"

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
