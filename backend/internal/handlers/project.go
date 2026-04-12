// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
)

// ProjectHandler handles project endpoints
type ProjectHandler struct {
	cfg            *config.Config
	projectService *services.ProjectService
	userService    *services.UserService
	redisService   *services.RedisService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(cfg *config.Config, redisService *services.RedisService, projectService *services.ProjectService, userService *services.UserService) *ProjectHandler {
	return &ProjectHandler{
		cfg:            cfg,
		projectService: projectService,
		userService:    userService,
		redisService:   redisService,
	}
}

// CreateProjectRequest represents project creation payload
type CreateProjectRequest struct {
	Name         string `json:"name"`
	GithubURL    string `json:"github_url"`
	Branch       string `json:"branch"`
	DatabaseName string `json:"database_name"`
	QueueEnabled bool   `json:"queue_enabled"`
}

// ListOwn returns user's own projects
func (h *ProjectHandler) ListOwn(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	projects, _, err := h.projectService.ListProjects(1, 100, userID, "", "")
	if err != nil {
		return apperr.New(500, "PROJECT_FETCH_FAILED", "Failed to fetch your projects")
	}

	return c.JSON(fiber.Map{
		"data": projects,
	})
}

// ListAll returns all projects (admin only)
func (h *ProjectHandler) ListAll(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	status := c.Query("status", "")
	search := c.Query("search", "")

	projects, total, err := h.projectService.ListProjects(page, limit, 0, status, search)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
	}

	return c.JSON(fiber.Map{
		"total": total,
		"page":  page,
		"limit": limit,
		"data":  projects,
	})
}

// Get returns single project details
func (h *ProjectHandler) Get(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return apperr.NewNotFound("Project", c.Params("id"))
	}

	h.projectService.UpdateActivity(project.ID)
	h.projectService.PopulateURL(project)

	return c.JSON(project)
}

// Create handles project creation
func (h *ProjectHandler) Create(c *fiber.Ctx) error {
	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	userID := c.Locals("user_id").(uint)

	// Basic validation
	if req.Name == "" || req.GithubURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name and Github URL are required",
		})
	}

	project, err := h.projectService.CreateProject(userID, req.Name, req.GithubURL, req.Branch, req.DatabaseName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Enqueue deployment job to Redis
	if err := h.redisService.EnqueueDeployment(project.ID, userID, "deploy"); err != nil {
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"project": project,
			"warning": "Failed to queue deployment: " + err.Error(),
		})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	h.projectService.UpdateProjectStatus(project.ID, models.StatusQueued)
	h.projectService.UpdateActivity(project.ID)

	// Enqueue redeployment job to Redis
	if err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue redeployment: " + err.Error(),
		})
	}

	// Get queue position
	queueLength, _ := h.redisService.GetQueueLength()

	return c.JSON(fiber.Map{
		"message":        "Redeployment queued successfully",
		"queue_position": queueLength,
	})
}

// UpdateRequest represents project update payload
type UpdateRequest struct {
	Name         string `json:"name"`
	Branch       string `json:"branch"`
	QueueEnabled bool   `json:"queue_enabled"`
}

// Update updates project details
func (h *ProjectHandler) Update(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	project, err = h.projectService.UpdateProject(project.ID, project.UserID, project.User.Role, req.Name, req.Branch, req.QueueEnabled)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update project"})
	}

	return c.JSON(project)
}

// Delete removes a project
func (h *ProjectHandler) Delete(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
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

// Logs streams container logs
func (h *ProjectHandler) Logs(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	lines, _ := strconv.Atoi(c.Query("lines", "100"))
	logs, err := h.projectService.GetLogs(*project.ContainerID, lines)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get logs"})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"logs": logs})
}

// Stats returns project resource usage
func (h *ProjectHandler) Stats(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	stats, err := h.projectService.GetStats(*project.ContainerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get stats"})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(stats)
}

// RunArtisanRequest represents artisan command payload
type RunArtisanRequest struct {
	Command string `json:"command"`
}

// RunArtisan executes an artisan command
func (h *ProjectHandler) RunArtisan(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	var req RunArtisanRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	output, err := h.projectService.ExecArtisan(*project.ContainerID, req.Command)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Command failed",
			"output": err.Error(),
		})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"output": output})
}

// GetEnv returns the .env file content
func (h *ProjectHandler) GetEnv(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var req UpdateEnvRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := h.projectService.SaveEnv(project.Subdomain, req.Content); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save .env file"})
	}

	h.projectService.UpdateActivity(project.ID)

	if project.ContainerID != nil {
		go h.projectService.StopContainer(*project.ContainerID)
	}

	return c.JSON(fiber.Map{
		"message": "Environment variables updated. Please redeploy to apply changes.",
	})
}

// AdminStats returns overview statistics
func (h *ProjectHandler) AdminStats(c *fiber.Ctx) error {
	totalProjects, _ := h.projectService.GetTotalCount()
	runningProjects, _ := h.projectService.GetRunningCount()
	totalStudents, _ := h.userService.GetStudentCount()

	return c.JSON(fiber.Map{
		"total_projects":   totalProjects,
		"running_projects": runningProjects,
		"total_students":   totalStudents,
	})
}

// ProxyToProject forwards requests to the correct project container
func (h *ProjectHandler) ProxyToProject(c *fiber.Ctx) error {
	host := c.Hostname()
	subdomain := strings.Split(host, ".")[0]

	var project models.Project
	cacheKey := fmt.Sprintf("proxy:subdomain:%s", subdomain)

	// 1. Try Cache First
	err := h.redisService.GetCache(cacheKey, &project)
	if err == nil && project.Status == models.StatusRunning && project.Port != nil {
		// Cache hit! Forward immediately
		targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project.Port)
		h.projectService.UpdateActivity(project.ID)
		return proxy.Forward(targetURL)(c)
	}

	// 2. Cache Miss: Fallback to Database
	// We need a way to get project by subdomain from service
	project_db, err := h.projectService.GetBySubdomain(subdomain)
	if err != nil || project_db.Status != models.StatusRunning {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found or not running"})
	}

	// 3. Populate Cache for the next request
	h.projectService.CacheSubdomainMapping(project_db)
	h.projectService.UpdateActivity(project_db.ID)

	if project_db.Port == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Project port not configured"})
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project_db.Port)
	return proxy.Forward(targetURL)(c)
}

// GetQueueStats returns deployment queue statistics
func (h *ProjectHandler) GetQueueStats(c *fiber.Ctx) error {
	stats, err := h.redisService.GetDeploymentStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get queue stats"})
	}
	return c.JSON(fiber.Map{"stats": stats})
}

// GetProjectsStats returns real-time resource usage for all running projects
func (h *ProjectHandler) GetProjectsStats(c *fiber.Ctx) error {
	statsMap, err := h.projectService.GetAllStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	projects, _ := h.projectService.GetRunningProjectsWithContainers()

	projectStats := make(map[uint]services.ContainerStats)
	for _, p := range projects {
		if p.ContainerID != nil && len(*p.ContainerID) >= 12 {
			shortID := (*p.ContainerID)[:12]
			if stat, exists := statsMap[shortID]; exists {
				projectStats[p.ID] = stat
			}
		}
	}

	return c.JSON(fiber.Map{"stats": projectStats})
}

// Helper methods

func (h *ProjectHandler) getProject(c *fiber.Ctx) (*models.Project, error) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return nil, fmt.Errorf("invalid project ID")
	}

	userID := c.Locals("user_id").(uint)
	role := models.Role(c.Locals("role").(string))

	project, err := h.projectService.GetProjectByID(uint(id))
	if err != nil {
		return nil, fmt.Errorf("project not found")
	}

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, fmt.Errorf("project not found")
	}

	return project, nil
}
