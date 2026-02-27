// ===========================================
// Feedback Handler
// ===========================================
// Handles user feedback submission and management
// ===========================================
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

type FeedbackHandler struct {
	db *gorm.DB
}

func NewFeedbackHandler(db *gorm.DB) *FeedbackHandler {
	return &FeedbackHandler{db: db}
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

	feedback := models.Feedback{
		UserID:  userID,
		Title:   req.Title,
		Content: req.Content,
		Type:    req.Type,
		Status:  models.FeedbackStatusPending,
	}

	if err := h.db.Create(&feedback).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to submit feedback",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(feedback)
}

// ListAll returns all feedback for admin
func (h *FeedbackHandler) ListAll(c *fiber.Ctx) error {
	var feedback []models.Feedback
	
	query := h.db.Preload("User").Order("created_at DESC")
	
	// Filtering
	if feedbackType := c.Query("type"); feedbackType != "" {
		query = query.Where("type = ?", feedbackType)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Find(&feedback).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch feedback",
		})
	}

	return c.JSON(feedback)
}

// ListOwn returns feedback submitted by the current user
func (h *FeedbackHandler) ListOwn(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	var feedback []models.Feedback

	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&feedback).Error; err != nil {
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

	if err := h.db.Model(&models.Feedback{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
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

	if err := h.db.Delete(&models.Feedback{}, id).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete feedback",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Feedback deleted successfully",
	})
}
