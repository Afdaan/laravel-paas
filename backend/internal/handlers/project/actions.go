package project

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// Create handles project creation
func (h *ProjectHandler) Create(c *fiber.Ctx) error {
	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return apperr.ErrUnauthorized
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return apperr.New(500, "AUTH_INTERNAL_ERROR", "Invalid user context")
	}

	// Basic validation
	if req.Name == "" || req.GithubURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name and Github URL are required",
		})
	}

	project, err := h.projectService.CreateProject(userID, req.Name, req.GithubURL, req.Branch, req.DatabaseName, req.BaseDirectory, req.BuildCommand, req.StartCommand, req.QueueEnabled)
	if err != nil {
		slog.Warn("Project creation failed", "user_id", userID, "error", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to create project",
		})
	}

	// Enqueue deployment job to Redis
	jobID, err := h.redisService.EnqueueDeployment(project.ID, userID, "deploy")
	if err != nil {
		slog.Error("Failed to enqueue deployment", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"project": project,
			"warning": "Project created but deployment queue failed. Please redeploy.",
		})
	}

	if err := h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Deployment enqueued", 0, jobID); err != nil {
		slog.Warn("Failed to update project deployment status on create", "id", project.ID, "error", err)
	}

	// Get queue position
	queueLength, _ := h.redisService.GetQueueLength()

	h.projectService.PopulateURL(project)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"project":        project,
		"queue_position": queueLength,
	})
}

// Redeploy rebuilds and restarts a project
func (h *ProjectHandler) Redeploy(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	// Check if already in queue to avoid duplicates
	isQueued, _ := h.redisService.IsProjectQueued(project.ID)
	if isQueued {
		queueLength, _ := h.redisService.GetQueueLength()
		return c.JSON(fiber.Map{
			"message":        "Project is already in queue",
			"queue_position": queueLength,
		})
	}

	// Enqueue redeployment job to Redis
	jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy")
	if err != nil {
		slog.Error("Failed to enqueue redeployment", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue redeployment",
		})
	}

	// Truncate the build log immediately to prevent old logs from displaying
	projectPath := filepath.Join(h.cfg.ProjectsPath, project.Subdomain)
	buildLogPath := filepath.Join(projectPath, "build.log")
	_ = os.MkdirAll(projectPath, 0755)
	_ = os.WriteFile(buildLogPath, []byte(""), 0644)

	if err := h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Redeployment requested by user", 0, jobID); err != nil {
		slog.Warn("Failed to update project deployment status to queued", "id", project.ID, "error", err)
	}
	h.projectService.UpdateActivity(project.ID)

	// Get queue position
	queueLength, _ := h.redisService.GetQueueLength()

	return c.JSON(fiber.Map{
		"message":        "Redeployment queued successfully",
		"queue_position": queueLength,
	})
}

// Delete removes a project
func (h *ProjectHandler) Delete(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if err := h.projectService.DeleteProject(project); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete project resources",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Project deleted successfully",
	})
}

// RunArtisanRequest represents artisan command payload
type RunArtisanRequest struct {
	Command string `json:"command"`
}

// RunArtisan executes an artisan command
func (h *ProjectHandler) RunArtisan(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	var req RunArtisanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Command == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Command is required"})
	}

	framework := project.Framework
	if framework == "" {
		framework = "Laravel" // Default fallback
	}

	if err := utils.ValidateCommand(framework, req.Command); err != nil {
		slog.Warn("Blocked command attempt",
			"project_id", project.ID,
			"framework", framework,
			"command", req.Command,
			"reason", err.Error(),
		)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	output, err := h.projectService.ExecCommand(project, req.Command)
	if err != nil {
		slog.Warn("Artisan command returned error",
			"project_id", project.ID,
			"command", req.Command,
			"error", err.Error(),
		)
		// We return 200 even if the command failed, because the "execution" was successful.
		// The frontend will display the output which contains the error message from Artisan.
		return c.JSON(fiber.Map{
			"output": output,
			"error":  "Command failed (non-zero exit code)",
		})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"output": output})
}

// Stop stops a project
func (h *ProjectHandler) Stop(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if err := h.projectService.StopProject(project); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to stop project"})
	}

	return c.JSON(fiber.Map{"message": "Project stopped successfully"})
}

// Start starts a stopped project
func (h *ProjectHandler) Start(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if err := h.projectService.StartProject(project); err != nil {
		slog.Error("Failed to start project", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to start project"})
	}

	return c.JSON(fiber.Map{"message": "Project started successfully"})
}

// Restart restarts a project
func (h *ProjectHandler) Restart(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if err := h.projectService.RestartProject(project); err != nil {
		slog.Error("Failed to restart project", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to restart project"})
	}

	return c.JSON(fiber.Map{"message": "Project restarted successfully"})
}
