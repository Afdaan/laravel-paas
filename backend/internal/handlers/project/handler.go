package project

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
)

// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
// ProjectHandler handles project endpoints
type ProjectHandler struct {
	cfg            *config.Config
	redisService   *infrastructure.RedisService
	projectService *projectServicePkg.ProjectService
	userService    *services.UserService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(cfg *config.Config, redisService *infrastructure.RedisService, projectService *projectServicePkg.ProjectService, userService *services.UserService) *ProjectHandler {
	return &ProjectHandler{
		cfg:            cfg,
		projectService: projectService,
		userService:    userService,
		redisService:   redisService,
	}
}

// GetDeploymentEvents returns the append-only timeline of deployment events for a project
func (h *ProjectHandler) GetDeploymentEvents(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	events, err := h.projectService.GetDeploymentEvents(project.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve deployment events"})
	}

	return c.JSON(events)
}
