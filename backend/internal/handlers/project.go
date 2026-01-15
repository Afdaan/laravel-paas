// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
	"gorm.io/gorm"
)

// ProjectHandler handles project endpoints
type ProjectHandler struct {
	db            *gorm.DB
	cfg           *config.Config
	dockerService *services.DockerService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(db *gorm.DB, cfg *config.Config) *ProjectHandler {
	return &ProjectHandler{
		db:            db,
		cfg:           cfg,
		dockerService: services.NewDockerService(cfg),
	}
}

// CreateProjectRequest represents project creation payload
type CreateProjectRequest struct {
	Name        string `json:"name"`
	GithubURL   string `json:"github_url"`
	DatabaseName string `json:"database_name"`
}

// ListOwn returns user's own projects
func (h *ProjectHandler) ListOwn(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var projects []models.Project
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
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

	offset := (page - 1) * limit

	var projects []models.Project
	var total int64

	query := h.db.Model(&models.Project{}).Preload("User")

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		query = query.Where("name LIKE ? OR subdomain LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&projects).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
	}

	return c.JSON(fiber.Map{
		"data":  projects,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Get returns a single project
func (h *ProjectHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db.Preload("User")

	// Students can only see their own projects
	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Project not found",
		})
	}

	return c.JSON(project)
}

// Create deploys a new project
func (h *ProjectHandler) Create(c *fiber.Ctx) error {
	var req CreateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Name == "" || req.GithubURL == "" || req.DatabaseName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name, GitHub URL, and database name are required",
		})
	}

	userID := c.Locals("user_id").(uint)

	// Check project limit
	maxProjects, _ := strconv.Atoi(GetSetting(h.db, "max_projects_per_user", "3"))
	var projectCount int64
	h.db.Model(&models.Project{}).Where("user_id = ?", userID).Count(&projectCount)
	if int(projectCount) >= maxProjects {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Project limit reached",
		})
	}

	// Check if database name is unique
	var existing models.Project
	if h.db.Where("database_name = ?", req.DatabaseName).First(&existing).Error == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Database name already in use",
		})
	}

	// Generate unique subdomain
	subdomain := services.GenerateSubdomain(req.Name)
	for {
		if h.db.Where("subdomain = ?", subdomain).First(&existing).Error != nil {
			break
		}
		subdomain = services.GenerateSubdomain(req.Name)
	}

	// Create project record
	project := models.Project{
		UserID:       userID,
		Name:         req.Name,
		GithubURL:    req.GithubURL,
		Subdomain:    subdomain,
		DatabaseName: req.DatabaseName,
		Status:       models.StatusPending,
	}

	if err := h.db.Create(&project).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create project",
		})
	}

	// Start deployment in background
	go h.deployProject(&project)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"project": project,
		"message": "Deployment started",
		"url":     "https://" + project.GetFullDomain(h.cfg.BaseDomain),
	})
}
// deployProject handles the full deployment process
func (h *ProjectHandler) deployProject(project *models.Project) {
	// Update status to building
	h.db.Model(project).Update("status", models.StatusBuilding)

	// Step 1: Clone repository
	projectPath, err := h.dockerService.CloneRepository(project.GithubURL, project.Subdomain)
	if err != nil {
		h.updateProjectError(project, "Failed to clone repository: "+err.Error())
		return
	}

	// Step 2: Detect Laravel version
	laravelVersion, phpVersion, err := h.dockerService.DetectVersions(projectPath)
	if err != nil {
		h.updateProjectError(project, "Failed to detect Laravel version: "+err.Error())
		return
	}

	h.db.Model(project).Updates(map[string]interface{}{
		"laravel_version": laravelVersion,
		"php_version":     phpVersion,
	})

	// Step 3: Create database
	if err := h.dockerService.CreateDatabase(project.DatabaseName); err != nil {
		h.updateProjectError(project, "Failed to create database: "+err.Error())
		return
	}

	// Step 4: Build and run container
	containerID, err := h.dockerService.BuildAndRun(project, phpVersion)
	if err != nil {
		h.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Update project as running
	h.db.Model(project).Updates(map[string]interface{}{
		"status":       models.StatusRunning,
		"container_id": containerID,
	})
}

// updateProjectError sets project status to failed
func (h *ProjectHandler) updateProjectError(project *models.Project, errorMsg string) {
	h.db.Model(project).Updates(map[string]interface{}{
		"status":    models.StatusFailed,
		"error_log": errorMsg,
	})
}

// Redeploy rebuilds and restarts a project
func (h *ProjectHandler) Redeploy(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Project not found",
		})
	}

	// Stop existing container if running
	if project.ContainerID != nil {
		h.dockerService.StopContainer(*project.ContainerID)
	}

	// Redeploy in background
	go h.deployProject(&project)

	return c.JSON(fiber.Map{
		"message": "Redeployment started",
	})
}

// Delete removes a project
func (h *ProjectHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Project not found",
		})
	}

	// Stop and remove container
	if project.ContainerID != nil {
		h.dockerService.RemoveContainer(*project.ContainerID)
	}

	// Remove project files
	h.dockerService.CleanupProject(project.Subdomain)

	// Drop database
	h.dockerService.DropDatabase(project.DatabaseName)

	// Delete project record
	if err := h.db.Delete(&project).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete project",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Project deleted successfully",
	})
}

// Logs streams container logs
func (h *ProjectHandler) Logs(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Project not found",
		})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Container not running",
		})
	}

	lines, _ := strconv.Atoi(c.Query("lines", "100"))
	logs, err := h.dockerService.GetContainerLogs(*project.ContainerID, lines)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get logs",
		})
	}

	return c.JSON(fiber.Map{
		"logs": logs,
	})
}

// Stats returns project resource usage
func (h *ProjectHandler) Stats(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Project not found",
		})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Container not running",
		})
	}

	stats, err := h.dockerService.GetContainerStats(*project.ContainerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get stats",
		})
	}

	return c.JSON(stats)
}

// AdminStats returns overview statistics
func (h *ProjectHandler) AdminStats(c *fiber.Ctx) error {
	var totalProjects int64
	var runningProjects int64
	var totalStudents int64

	h.db.Model(&models.Project{}).Count(&totalProjects)
	h.db.Model(&models.Project{}).Where("status = ?", models.StatusRunning).Count(&runningProjects)
	h.db.Model(&models.User{}).Where("role = ?", models.RoleStudent).Count(&totalStudents)

	return c.JSON(fiber.Map{
		"total_projects":   totalProjects,
		"running_projects": runningProjects,
		"total_students":   totalStudents,
	})
}
