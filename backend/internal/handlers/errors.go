// ===========================================
// Global Error Handler
// ===========================================
// Orchestrates centralized error responses
// ===========================================
package handlers

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
)

// ErrorHandler is the global error handler for the Fiber application
func ErrorHandler(c *fiber.Ctx, err error) error {
	// Default values
	status := fiber.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "An unexpected error occurred"
	// 1. Check for custom AppError
	if ae, ok := err.(*apperr.AppError); ok {
		status = ae.HTTPStatus
		code = ae.Code
		message = ae.Message
	} else if fe, ok := err.(*fiber.Error); ok {
		// 2. Check for Fiber's built-in errors
		status = fe.Code
		message = fe.Message
	}

	// Log the error (Only 500s are critical)
	if status >= 500 {
		slog.Error("Critical System Error", "error", err, "path", c.Path())
	} else {
		slog.Warn("Request Error", "status", status, "message", message, "path", c.Path())
	}

	// Standardize the response
	return c.Status(status).JSON(fiber.Map{
		"error": message,
		"code":  code,
	})
}
