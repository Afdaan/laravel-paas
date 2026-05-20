package project

import (
	"bufio"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
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

// StreamDeploymentEvents streams live deployment timeline events using Server-Sent Events (SSE)
func (h *ProjectHandler) StreamDeploymentEvents(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	h.projectService.UpdateActivity(project.ID)

	ctx := c.Context()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// 1. Fetch existing events from DB and stream them as a single initial_events event
		events, err := h.projectService.GetDeploymentEvents(project.ID)
		if err == nil {
			eventsJSON, err := json.Marshal(events)
			if err == nil {
				_, _ = w.WriteString(fmt.Sprintf("event: initial_events\ndata: %s\n\n", string(eventsJSON)))
				_ = w.Flush()
			}
		}

		// 2. Subscribe to Redis for new events
		msgChan, err := h.redisService.SubscribeDeploymentEvents(ctx, project.ID)
		if err != nil {
			return
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-msgChan:
				if !ok {
					return
				}
				_, err := w.WriteString(fmt.Sprintf("event: deployment_event\ndata: %s\n\n", payload))
				if err != nil {
					return
				}
			case <-ticker.C:
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}

