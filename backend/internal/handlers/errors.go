// ===========================================
// Global Error Handler
// ===========================================
// Orchestrates centralized error responses
// ===========================================
package handlers

import (
	"errors"
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
	var ae *apperr.AppError
	if errors.As(err, &ae) {
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
		// Log full error for 4xx as well so cryptographic and logical failures aren't blindly swallowed
		slog.Warn("Request Error", "status", status, "message", message, "path", c.Path(), "error", err)
	}

	// Standardize the response
	response := fiber.Map{
		"error": message,
		"code":  code,
	}
	
	// Include structured details if present for API consumers
	if ae != nil && ae.Details != nil {
		response["details"] = ae.Details
	}

	return c.Status(status).JSON(response)
}
