// ===========================================
// Global Error Handler
// ===========================================
package handlers

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/shared/apperr"
)

func ErrorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "An unexpected error occurred"
	var ae *apperr.AppError
	if errors.As(err, &ae) {
		status = ae.HTTPStatus
		code = ae.Code
		message = ae.Message
	} else if fe, ok := err.(*fiber.Error); ok {
		status = fe.Code
		if status < fiber.StatusInternalServerError {
			message = fe.Message
		}
	}

	requestID := middleware.RequestID(c)
	if status >= 500 {
		slog.Error("request failed", "request_id", requestID, "status", status, "path", c.Path(), "error", err)
	} else {
		slog.Warn("request rejected", "request_id", requestID, "status", status, "path", c.Path(), "error", err)
	}
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": requestID,
	})
}
