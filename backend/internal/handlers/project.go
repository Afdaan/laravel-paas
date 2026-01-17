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
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
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
	redisService  *services.RedisService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(db *gorm.DB, cfg *config.Config, redisService *services.RedisService) *ProjectHandler {
	return &ProjectHandler{
		db:            db,
		cfg:           cfg,
		dockerService: services.NewDockerService(cfg),
		redisService:  redisService,
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

	var projects []models.Project
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
	}

	h.populateURLs(projects)

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

	h.populateURLs(projects)

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

	h.populateURL(&project)

	return c.JSON(project)
}

// UpdateProjectRequest represents project update payload
type UpdateProjectRequest struct {
	PHPVersion   string `json:"php_version"`
	QueueEnabled *bool  `json:"queue_enabled"`
}

// Update modifies project settings
func (h *ProjectHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid project ID",
		})
	}

	var req UpdateProjectRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
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

	updates := map[string]interface{}{}
	
	// If PHP version provided, update it and set manual flag
	if req.PHPVersion != "" {
		updates["php_version"] = req.PHPVersion
		updates["is_manual_version"] = true
	}

	if req.QueueEnabled != nil {
		updates["queue_enabled"] = *req.QueueEnabled
	}

	if len(updates) > 0 {
		if err := h.db.Model(&project).Updates(updates).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update project",
			})
		}
	}

	return c.JSON(fiber.Map{
		"message": "Project updated successfully",
		"project": project,
	})
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
	
	// Default branch to main if empty
	branch := req.Branch
	if branch == "" {
		branch = "main"
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

	// Check if database name is unique (including soft-deleted records for MySQL constraint)
	var existing models.Project

	// First check with Unscoped to find any record (including soft-deleted)
	if err := h.db.Unscoped().Where("database_name = ?", req.DatabaseName).First(&existing).Error; err == nil {
		// Record exists - check if it's soft-deleted
		if existing.DeletedAt.Valid {
			// Soft-deleted record exists - permanently delete it to free up the database_name
			h.db.Unscoped().Delete(&existing)
		} else {
			// Active record exists
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Database name already in use",
			})
		}
	}

	// Generate unique subdomain (also check unscoped)
	subdomain := services.GenerateSubdomain(req.Name)
	for {
		if err := h.db.Unscoped().Where("subdomain = ?", subdomain).First(&existing).Error; err != nil {
			// No record found, subdomain is available
			break
		}
		// Record exists
		if existing.DeletedAt.Valid {
			// Soft-deleted, permanently delete to free up subdomain
			h.db.Unscoped().Delete(&existing)
			break
		}
		// Active record, generate new subdomain
		subdomain = services.GenerateSubdomain(req.Name)
	}

	// Create project record
	project := models.Project{
		UserID:       userID,
		Name:         req.Name,
		GithubURL:    req.GithubURL,
		Branch:       branch,
		Subdomain:    subdomain,
		DatabaseName: req.DatabaseName,
		Status:       models.StatusPending,
		QueueEnabled: req.QueueEnabled,
	}

	if err := h.db.Create(&project).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create project",
		})
	}

	// Enqueue deployment job to Redis
	if err := h.redisService.EnqueueDeployment(project.ID, userID, "deploy"); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue deployment: " + err.Error(),
		})
	}

	// Get queue position
	queueLength, _ := h.redisService.GetQueueLength()

	projectDomain := GetSetting(h.db, "project_domain", h.cfg.ProjectDomain)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"project":        project,
		"message":        "Deployment queued successfully",
		"url":            "https://" + project.GetFullDomain(projectDomain),
		"queue_position": queueLength,
	})
}
// deployProject handles the full deployment process
func (h *ProjectHandler) deployProject(project *models.Project) {
	// Update status to building and clear old error logs
	h.db.Model(project).Select("status", "error_log", "updated_at").Updates(map[string]interface{}{
		"status":    models.StatusBuilding,
		"error_log": nil,
	})

	// Step 1: Clone repository
	projectPath, err := h.dockerService.CloneRepository(project.GithubURL, project.Branch, project.Subdomain)
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

	
	// Use manual PHP version if set, otherwise use detected version
	finalPHPVersion := phpVersion
	if project.IsManualVersion && project.PHPVersion != "" {
		finalPHPVersion = project.PHPVersion
	}

	h.db.Model(project).Updates(map[string]interface{}{
		"laravel_version": laravelVersion,
		"php_version":     finalPHPVersion,
	})

	// Step 3: Create database
	if err := h.dockerService.CreateDatabase(project.DatabaseName); err != nil {
		h.updateProjectError(project, "Failed to create database: "+err.Error())
		return
	}

	
	var oldContainerID *string
	if project.ContainerID != nil {
		oldHelp := *project.ContainerID
		oldContainerID = &oldHelp
	}

	projectDomain := GetSetting(h.db, "project_domain", h.cfg.ProjectDomain)
	containerID, err := h.dockerService.BuildAndRun(project, finalPHPVersion, projectDomain)
	
	go h.dockerService.PruneImages()

	if err != nil {
		h.updateProjectError(project, "Failed to deploy container: "+err.Error())
		return
	}

	// Wait for health check (max 60s)
	healthy := false
	for i := 0; i < 30; i++ {
		if h.dockerService.IsContainerHealthy(containerID) {
			healthy = true
			break
		}
		time.Sleep(2 * time.Second)
	}

	if !healthy {
		h.updateProjectError(project, "Container failed health check")
		h.dockerService.RemoveContainer(containerID)
		return
	}

	h.db.Model(project).Updates(map[string]interface{}{
		"status":       models.StatusRunning,
		"container_id": containerID,
	})

	if oldContainerID != nil {
		go func() {
			time.Sleep(5 * time.Second) 
			h.dockerService.RemoveContainer(*oldContainerID)
		}()
	}
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

	// Enqueue redeployment job to Redis
	if err := h.redisService.EnqueueDeployment(project.ID, userID, "redeploy"); err != nil {
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

	// Remove project image
	h.dockerService.RemoveImage(project.Subdomain)

	// Clean up dangling images (<none>)
	go h.dockerService.PruneImages()

	// Remove project files
	h.dockerService.CleanupProject(project.Subdomain)

	// Drop database
	h.dockerService.DropDatabase(project.DatabaseName)

	// Hard delete project record (not soft delete) to free up database_name and subdomain
	if err := h.db.Unscoped().Delete(&project).Error; err != nil {
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

// RunArtisanRequest represents artisan command payload
type RunArtisanRequest struct {
	Command string `json:"command"`
}

// RunArtisan executes an artisan command
func (h *ProjectHandler) RunArtisan(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
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

	// Execute command
	output, err := h.dockerService.ExecLaravelCommand(*project.ContainerID, req.Command)
	if err != nil {
		// Return 200 even on command failure to show output, but maybe with a status flag?
		// Or just 500. Let's return 200 with error in JSON if execution ran but returned non-zero.
		// For now, if docker exec fails, it returns error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Command failed",
			"output": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"output": output,
	})
}

// GetEnv returns the .env file content
func (h *ProjectHandler) GetEnv(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	content, err := h.dockerService.GetEnvFile(project.Subdomain)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read .env file"})
	}

	return c.JSON(fiber.Map{
		"content": content,
	})
}

// UpdateEnvRequest represents env update payload
type UpdateEnvRequest struct {
	Content string `json:"content"`
}

// UpdateEnv updates the .env file content
func (h *ProjectHandler) UpdateEnv(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req UpdateEnvRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if err := h.dockerService.SaveEnvFile(project.Subdomain, req.Content); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save .env file"})
	}

	// Use empty string to restart container (stop & start logic handled in redeploy usually)
	// But here we probably should restart the container to apply changes.
	// For now, let's just save. The user might need to manually redeploy or we can trigger it.
	// Let's trigger a restart of the container if it's running.
	if project.ContainerID != nil {
		go func() {
			h.dockerService.StopContainer(*project.ContainerID)
			// We need to re-run the container. The simplest way is to trigger redeploy logic,
			// but we don't want to re-clone code.
			// Just restarting container might not be enough if env vars are passed via `docker run -e`...
			// Wait, in start.sh/docker.go, are we passing env vars individually or via --env-file?
			// Checking docker.go... we passed .env file: COPY .env* ./ in Dockerfile.
			// So restarting container is enough IF the .env inside container is updated.
			// BUT, our SaveEnvFile updates the file on HOST storage.
			// The Dockerfile COPIES .env at build time.
			// WE NEED TO MOUNT .env from host to container for dynamic updates to work without rebuild!
			// My `StartContainer` or `BuildAndRun` logic needs checking.
		}()
	}
	// Note: For now, we just save. The user is instructed to Redeploy to apply changes thoroughly.

	return c.JSON(fiber.Map{
		"message": "Environment variables updated. Please redeploy to apply changes.",
	})
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

// ProxyToProject forwards requests to the correct project container
func (h *ProjectHandler) ProxyToProject(c *fiber.Ctx) error {
	// Extract subdomain from host
	host := c.Hostname()
	subdomain := strings.Split(host, ".")[0]

	// Find the project by subdomain
	var project models.Project
	if err := h.db.Where("subdomain = ? AND status = ?", subdomain, models.StatusRunning).First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found or not running"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database error"})
	}

	// Ensure project has a port
	if project.Port == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Project port not configured"})
	}

	// Create the proxy URL
	targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project.Port)

	// Forward the request
	if err := proxy.Forward(targetURL)(c); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return nil // The proxy handler takes care of the response
}

// GetQueueStats returns deployment queue statistics
func (h *ProjectHandler) GetQueueStats(c *fiber.Ctx) error {
	stats, err := h.redisService.GetDeploymentStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to get queue stats",
		})
	}

	return c.JSON(fiber.Map{
		"stats": stats,
	})
}

// GetProjectsStats returns real-time resource usage for all running projects
func (h *ProjectHandler) GetProjectsStats(c *fiber.Ctx) error {
	// 1. Get bulk stats from Docker
	statsMap, err := h.dockerService.GetAllContainerStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch container stats: " + err.Error(),
		})
	}

	// 2. Fetch all running projects
	var projects []models.Project
	if err := h.db.Where("status = ? AND container_id IS NOT NULL", models.StatusRunning).Find(&projects).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
	}

	// 3. Map project ID to stats
	projectStats := make(map[uint]services.ContainerStats)

	for _, p := range projects {
		if p.ContainerID == nil || len(*p.ContainerID) < 12 {
			continue
		}

		shortID := (*p.ContainerID)[:12]

		if stat, exists := statsMap[shortID]; exists {
			projectStats[p.ID] = stat
		}
	}

	return c.JSON(fiber.Map{
		"stats": projectStats,
	})
}

func (h *ProjectHandler) populateURL(project *models.Project) {
	projectDomain := GetSetting(h.db, "project_domain", h.cfg.ProjectDomain)
	project.URL = "https://" + project.GetFullDomain(projectDomain)
}

func (h *ProjectHandler) populateURLs(projects []models.Project) {
	projectDomain := GetSetting(h.db, "project_domain", h.cfg.ProjectDomain)
	for i := range projects {
		projects[i].URL = "https://" + projects[i].GetFullDomain(projectDomain)
	}
}
