package project

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
)

// GetEnv returns the .env file content
func (h *ProjectHandler) GetEnv(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	content, err := h.projectService.GetEnv(project.Subdomain)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read .env file"})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"content": content})
}

// UpdateEnvRequest represents env update payload
type UpdateEnvRequest struct {
	Content string `json:"content"`
}

// UpdateEnv updates the .env file content
func (h *ProjectHandler) UpdateEnv(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req UpdateEnvRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := h.projectService.SaveEnv(project.Subdomain, req.Content); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save .env file"})
	}

	h.projectService.UpdateActivity(project.ID)

	// Set status to Queued so the UI shows immediate feedback
	if err := h.projectService.UpdateProjectStatus(project.ID, models.StatusQueued); err != nil {
		slog.Warn("Failed to update project status after env update", "id", project.ID, "error", err)
	}

	// Automatically trigger a redeploy to apply changes.
	// This is essential for frontend frameworks (Vite, Next.js) that need these
	// variables during the build phase.
	if err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
		slog.Error("Failed to auto-enqueue redeployment after env update",
			"project_id", project.ID,
			"error", err)

		return c.JSON(fiber.Map{
			"message": "Environment variables saved, but failed to queue auto-redeploy. Please redeploy manually.",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Environment variables updated. A new build has been queued to apply changes.",
	})
}
