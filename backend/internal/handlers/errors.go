// ===========================================
// Error Handler
// ===========================================
// Global error handling for Fiber app
// ===========================================
package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// ErrorHandler is the global error handler
func ErrorHandler(c *fiber.Ctx, err error) error {
	// Default to 500 Internal Server Error
	code := fiber.StatusInternalServerError
	message := "Internal server error"

	// Check if it's a fiber error
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"error": message,
	})
}
