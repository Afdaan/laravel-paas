// ===========================================
// Feedback Handler
// ===========================================
// Handles user feedback submission and management
// ===========================================
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/models"
)

type FeedbackHandler struct {
	service *services.FeedbackService
}

func NewFeedbackHandler(service *services.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{service: service}
}

type CreateFeedbackRequest struct {
	Title   string              `json:"title" validate:"required"`
	Content string              `json:"content" validate:"required"`
	Type    models.FeedbackType `json:"type" validate:"required"`
}

type UpdateFeedbackStatusRequest struct {
	Status models.FeedbackStatus `json:"status" validate:"required"`
}

// Create handles feedback submission from users
func (h *FeedbackHandler) Create(c *fiber.Ctx) error {
	var req CreateFeedbackRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	userID := c.Locals("user_id").(uint)
	feedback, err := h.service.SubmitFeedback(userID, req.Title, req.Content, req.Type)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to submit feedback",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(feedback)
}

// ListAll returns all feedback for admin
func (h *FeedbackHandler) ListAll(c *fiber.Ctx) error {
	feedback, err := h.service.GetAllFeedback(c.Query("type"), c.Query("status"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch feedback",
		})
	}

	return c.JSON(feedback)
}

// ListOwn returns feedback submitted by the current user
func (h *FeedbackHandler) ListOwn(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	feedback, err := h.service.GetUserFeedback(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch your feedback",
		})
	}

	return c.JSON(feedback)
}

// UpdateStatus updates the status of a feedback entry (Admin)
func (h *FeedbackHandler) UpdateStatus(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid feedback ID",
		})
	}

	var req UpdateFeedbackStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if err := h.service.UpdateStatus(uint(id), req.Status); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update feedback status",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Feedback status updated successfully",
	})
}

// Delete removes a feedback entry (Admin)
func (h *FeedbackHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid feedback ID",
		})
	}

	if err := h.service.DeleteFeedback(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete feedback",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Feedback deleted successfully",
	})
}
