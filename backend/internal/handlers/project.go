// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	"github.com/laravel-paas/backend/internal/services"
	"gorm.io/gorm"
)

// ProjectHandler handles project endpoints
type ProjectHandler struct {
	db             *gorm.DB
	cfg            *config.Config
	projectService *services.ProjectService
	redisService   *services.RedisService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(db *gorm.DB, cfg *config.Config, redisService *services.RedisService) *ProjectHandler {
	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	settingService := services.NewSettingService(settingRepo)
	
	return &ProjectHandler{
		db:             db,
		cfg:            cfg,
		projectService: services.NewProjectService(cfg, projectRepo, settingService, redisService),
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

	var projects []models.Project
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
		return apperr.New(500, "PROJECT_FETCH_FAILED", "Failed to fetch your projects")
	}

	h.projectService.PopulateURLs(projects)

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
		query = query.Where("name ILIKE ? OR subdomain ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&projects).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch projects",
		})
	}

	h.projectService.PopulateURLs(projects)

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

	// Check project limit
	maxProjects, _ := strconv.Atoi(h.projectService.GetSetting(models.SettingMaxProjects, models.DefaultMaxProjects))
	var projectCount int64
	h.db.Model(&models.Project{}).Where("user_id = ?", userID).Count(&projectCount)
	if int(projectCount) >= maxProjects {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "You have reached the maximum project limit",
		})
	}

	// Basic validation
	if req.Name == "" || req.GithubURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name and Github URL are required",
		})
	}

	// Generate subdomain and database name
	subdomain := h.generateSubdomain(req.Name)
	dbName := req.DatabaseName
	if dbName == "" {
		dbName = strings.ReplaceAll(subdomain, "-", "_")
	}

	// Calculate expiration date
	expiryDays, _ := strconv.Atoi(h.projectService.GetSetting(models.SettingProjectExpiry, models.DefaultProjectExpiry))
	var expiresAt *time.Time
	if expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	project := models.Project{
		UserID:       userID,
		Name:         req.Name,
		GithubURL:    req.GithubURL,
		Branch:       req.Branch,
		Subdomain:    subdomain,
		DatabaseName: dbName,
		Status:       models.StatusQueued,
		ExpiresAt:    expiresAt,
	}

	if err := h.db.Create(&project).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create project",
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

	h.projectService.PopulateURL(&project)

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

	h.db.Model(project).Updates(map[string]interface{}{
		"status": models.StatusQueued,
	})

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

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Branch != "" {
		updates["branch"] = req.Branch
	}
	updates["queue_enabled"] = req.QueueEnabled

	if err := h.db.Model(project).Updates(updates).Error; err != nil {
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

// AdminStats returns overview statistics (Internal logic kept simple for now)
func (h *ProjectHandler) AdminStats(c *fiber.Ctx) error {
	var totalProjects, runningProjects, totalStudents int64
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
	if err := h.db.Where("subdomain = ? AND status = ?", subdomain, models.StatusRunning).First(&project).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found or not running"})
	}

	// 3. Populate Cache for the next request
	h.projectService.CacheSubdomainMapping(&project)
	h.projectService.UpdateActivity(project.ID)

	if project.Port == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Project port not configured"})
	}

	targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project.Port)
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

	var projects []models.Project
	h.db.Where("status = ? AND container_id IS NOT NULL", models.StatusRunning).Find(&projects)

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

	var project models.Project
	query := h.db.Where("id = ?", id)
	if role != models.RoleAdmin && role != models.RoleSuperAdmin {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project).Error; err != nil {
		return nil, fmt.Errorf("project not found")
	}

	return &project, nil
}

func (h *ProjectHandler) generateSubdomain(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	reg := ""
	for _, char := range s {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			reg += string(char)
		}
	}
	return fmt.Sprintf("%s-%s", reg, h.randomString(6))
}

func (h *ProjectHandler) randomString(n int) string {
	rand.Seed(time.Now().UnixNano())
	var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}
