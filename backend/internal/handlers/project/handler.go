package project

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"gorm.io/gorm"
)

// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
// ProjectHandler handles project endpoints
type ProjectHandler struct {
	cfg            *config.Config
	db             *gorm.DB
	redisService   *infrastructure.RedisService
	projectService *projectServicePkg.ProjectService
	userService    *services.UserService
	dockerService  *docker.DockerService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(cfg *config.Config, db *gorm.DB, redisService *infrastructure.RedisService, projectService *projectServicePkg.ProjectService, userService *services.UserService, dockerService *docker.DockerService) *ProjectHandler {
	return &ProjectHandler{
		cfg:            cfg,
		db:             db,
		projectService: projectService,
		userService:    userService,
		redisService:   redisService,
		dockerService:  dockerService,
	}
}

// GetDeploymentEvents returns the append-only timeline of deployment events for a project
func (h *ProjectHandler) GetDeploymentEvents(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var events []models.DeploymentEvent
	var fetchErr error

	if c.Query("all") == "true" {
		events, fetchErr = h.projectService.GetAllDeploymentEvents(project.ID)
	} else {
		events, fetchErr = h.projectService.GetDeploymentEvents(project.ID)
	}

	if fetchErr != nil {
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
	c.Set("X-Accel-Buffering", "no")

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

		keepAliveTicker := time.NewTicker(15 * time.Second)
		defer keepAliveTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case payload, ok := <-msgChan:
				if !ok {
					return
				}
				// Filter out lease-related events to keep the timeline clean
				var ev struct {
					EventType string `json:"event_type"`
				}
				if err := json.Unmarshal([]byte(payload), &ev); err == nil {
					if strings.Contains(strings.ToLower(ev.EventType), "lease") {
						continue
					}
				}
				_, err := w.WriteString(fmt.Sprintf("event: deployment_event\ndata: %s\n\n", payload))
				if err != nil {
					return
				}
			case <-keepAliveTicker.C:
				_, err := w.WriteString(":\n\n")
				if err != nil {
					return
				}
				_ = w.Flush()
			case <-ticker.C:
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}

