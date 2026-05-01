// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
package handlers

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
)

// ProjectHandler handles project endpoints
type ProjectHandler struct {
	cfg            *config.Config
	redisService   *infrastructure.RedisService
	projectService *services.ProjectService
	userService    *services.UserService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(cfg *config.Config, redisService *infrastructure.RedisService, projectService *services.ProjectService, userService *services.UserService) *ProjectHandler {
	return &ProjectHandler{
		cfg:            cfg,
		projectService: projectService,
		userService:    userService,
		redisService:   redisService,
	}
}

// CreateProjectRequest represents project creation payload
type CreateProjectRequest struct {
	Name          string `json:"name"`
	GithubURL     string `json:"github_url"`
	Branch        string `json:"branch"`
	DatabaseName  string `json:"database_name"`
	BaseDirectory string `json:"base_directory"`
	RuntimeImage  string `json:"runtime_image"`
	BuildCommand  string `json:"build_command"`
	StartCommand  string `json:"start_command"`
	QueueEnabled  bool   `json:"queue_enabled"`
}

// ListOwn returns user's own projects
func (h *ProjectHandler) ListOwn(c *fiber.Ctx) error {
	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return apperr.ErrUnauthorized
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return apperr.New(500, "AUTH_INTERNAL_ERROR", "Invalid user context")
	}

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

	project, err := h.projectService.CreateProject(userID, req.Name, req.GithubURL, req.Branch, req.DatabaseName, req.BaseDirectory, req.RuntimeImage, req.BuildCommand, req.StartCommand, req.QueueEnabled)
	if err != nil {
		slog.Warn("Project creation failed", "user_id", userID, "error", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to create project",
		})
	}

	// Enqueue deployment job to Redis
	if err := h.redisService.EnqueueDeployment(project.ID, userID, "deploy"); err != nil {
		slog.Error("Failed to enqueue deployment", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"project": project,
			"warning": "Project created but deployment queue failed. Please redeploy.",
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	h.projectService.UpdateProjectStatus(project.ID, models.StatusQueued)
	h.projectService.UpdateActivity(project.ID)

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
	if err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy"); err != nil {
		slog.Error("Failed to enqueue redeployment", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue redeployment",
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
	Name          string `json:"name"`
	Branch        string `json:"branch"`
	PHPVersion    string `json:"php_version"`
	RuntimeImage  string `json:"runtime_image"`
	BaseDirectory string `json:"base_directory"`
	QueueEnabled  bool   `json:"queue_enabled"`
	WorkerCommand string `json:"worker_command"`
	BuildCommand  string `json:"build_command"`
	StartCommand  string `json:"start_command"`
	NodeVersion   string `json:"node_version"`
}

// Update updates project details
func (h *ProjectHandler) Update(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req UpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	project, err = h.projectService.UpdateProject(project.ID, project.UserID, project.User.Role, req.Name, req.Branch, req.PHPVersion, req.RuntimeImage, req.BaseDirectory, req.QueueEnabled, req.WorkerCommand, req.BuildCommand, req.StartCommand, req.NodeVersion)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update project"})
	}

	return c.JSON(project)
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

// Logs streams container logs
func (h *ProjectHandler) Logs(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if project.ContainerID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	lines, _ := strconv.Atoi(c.Query("lines", "100"))
	logType := c.Query("type", "web")
	logs, err := h.projectService.GetLogs(project, logType, lines)
	if err != nil {
		slog.Warn("Failed to get project logs", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve logs"})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"logs": logs})
}

// BuildLogs returns the railpack build log output
func (h *ProjectHandler) BuildLogs(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	logPath := filepath.Join(h.cfg.ProjectsPath, project.Subdomain, "build.log")
	f, err := os.Open(logPath)
	if err != nil {
		// Log not available yet or project not building
		return c.JSON(fiber.Map{"logs": "Initializing build environment..."})
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return c.JSON(fiber.Map{"logs": "Initializing build environment..."})
	}

	// Cap response size to avoid UI polling turning into a memory/CPU DoS.
	const maxBytes = 256 * 1024
	size := st.Size()
	readSize := int64(maxBytes)
	if size < readSize {
		readSize = size
	}

	buf := make([]byte, readSize)
	off := size - readSize
	if off < 0 {
		off = 0
	}
	_, _ = f.ReadAt(buf, off)

	return c.JSON(fiber.Map{"logs": string(buf)})
}

// Stats returns project resource usage
func (h *ProjectHandler) Stats(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
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

// allowedArtisanCommands is the list of safe artisan commands that students can execute
var allowedArtisanCommands = map[string]bool{
	"cache:clear":    true,
	"cache:forget":   true,
	"config:cache":   true,
	"config:clear":   true,
	"route:cache":    true,
	"route:clear":    true,
	"route:list":     true,
	"view:cache":     true,
	"view:clear":     true,
	"optimize":       true,
	"optimize:clear": true,
	"list":           true,
	"about":          true,
	"env":            true,
	"storage:link":   true,
}

// blockedArtisanPatterns contains command prefixes that are never allowed
var blockedArtisanPatterns = []string{
	"migrate:fresh",
	"migrate:reset",
	"migrate:rollback",
	"db:seed",
	"tinker",
	"make:",
	"key:generate",
	"down",
	"up",
	"serve",
	"schedule:run",
	"schedule:work",
	"queue:work",
	"queue:listen",
	"queue:restart",
	"queue:retry",
	"queue:forget",
	"queue:flush",
	"queue:prune-batches",
	"optimize:v2",
	"stub:publish",
	"vendor:publish",
	"install",
	"test",
	"pest",
	"clear-compiled",
}

func validateArtisanCommand(command string) error {
	// Extract base command (first word)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	baseCommand := parts[0]

	// Check against blocklist first
	for _, pattern := range blockedArtisanPatterns {
		if baseCommand == pattern || strings.HasPrefix(baseCommand, pattern) {
			return fmt.Errorf("command '%s' is not allowed", baseCommand)
		}
	}

	// Check against allowlist (if not in allowlist, reject)
	if !allowedArtisanCommands[baseCommand] {
		return fmt.Errorf("command '%s' is not in the allowed list", baseCommand)
	}

	return nil
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

	if err := validateArtisanCommand(req.Command); err != nil {
		slog.Warn("Blocked artisan command attempt",
			"project_id", project.ID,
			"command", req.Command,
			"reason", err.Error(),
		)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}

	output, err := h.projectService.ExecArtisan(*project.ContainerID, req.Command)
	if err != nil {
		slog.Error("Artisan command execution failed",
			"project_id", project.ID,
			"command", req.Command,
			"error", err.Error(),
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Command execution failed",
		})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"output": output})
}

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

	if project.ContainerID != nil {
		go h.projectService.StopContainer(*project.ContainerID)
	}

	return c.JSON(fiber.Map{
		"message": "Environment variables updated. Please redeploy to apply changes.",
	})
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
		// Cache hit! Forward with path stripping
		targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project.Port)

		// Map the path correctly by stripping the /proxy prefix
		// Fiber's wildcard parameter (*) holds the rest of the path
		path := c.Params("*")
		target := targetURL + "/" + path

		h.projectService.UpdateActivity(project.ID)
		return proxy.Forward(target)(c)
	}

	// 2. Cache Miss: Fallback to Database
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

	// 4. Forward with path stripping
	targetURL := fmt.Sprintf("http://127.0.0.1:%d", *project_db.Port)
	path := c.Params("*")
	target := targetURL + "/" + path

	return proxy.Forward(target)(c)
}

// GetQueueStats returns deployment queue statistics and job lists
func (h *ProjectHandler) GetQueueStats(c *fiber.Ctx) error {
	stats, err := h.redisService.GetDeploymentStats()
	if err != nil {
		stats = make(map[string]string)
	}

	// 1. Fetch active builds (currently BUILDING) from DB
	active, err := h.projectService.GetProjectsByStatus(models.StatusBuilding)
	if err != nil {
		active = []models.Project{}
	}

	// 2. Fetch all projects that SHOULD be waiting (QUEUED or PENDING) from DB
	waitingProjs, _ := h.projectService.GetProjectsByStatuses([]models.ProjectStatus{models.StatusQueued, models.StatusPending})

	// 3. Fetch actual jobs from Redis
	redisJobs, _ := h.redisService.ListDeploymentJobs()

	// 4. Enrich and Merge
	type EnrichedJob struct {
		infrastructure.DeploymentJob
		ProjectName string `json:"project_name"`
		Email       string `json:"email"`
	}

	finalWaitList := make([]EnrichedJob, 0)
	seenProjectIDs := make(map[uint]bool)

	// Add Redis jobs first (they preserve queue order)
	for _, job := range redisJobs {
		eJob := EnrichedJob{DeploymentJob: job}
		p, err := h.projectService.GetProjectByID(job.ProjectID)
		if err == nil {
			eJob.ProjectName = p.Name
			eJob.Email = p.User.Email
		}
		finalWaitList = append(finalWaitList, eJob)
		seenProjectIDs[job.ProjectID] = true
	}

	// Add DB projects that are missing from Redis (but marked as Queued/Pending)
	for _, p := range waitingProjs {
		if !seenProjectIDs[p.ID] {
			finalWaitList = append(finalWaitList, EnrichedJob{
				DeploymentJob: infrastructure.DeploymentJob{
					ProjectID:  p.ID,
					UserID:     p.UserID,
					Type:       "waiting",
					EnqueuedAt: p.CreatedAt, // Fallback to creation time
				},
				ProjectName: p.Name,
				Email:       p.User.Email,
			})
		}
	}

	return c.JSON(fiber.Map{
		"stats":  stats,
		"active": active,
		"queued": finalWaitList,
	})
}

// GetProjectsStats returns real-time resource usage for all running projects
func (h *ProjectHandler) GetProjectsStats(c *fiber.Ctx) error {
	statsMap, err := h.projectService.GetAllStats()
	if err != nil {
		slog.Error("Failed to get all project stats", "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve stats"})
	}

	projects, _ := h.projectService.GetRunningProjectsWithContainers()

	projectStats := make(map[uint]infrastructure.ContainerStats)
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

	uidVal := c.Locals("user_id")
	roleVal := c.Locals("role")

	if uidVal == nil || roleVal == nil {
		return nil, fmt.Errorf("unauthorized: missing user context")
	}

	userID, okUID := uidVal.(uint)
	roleStr, okRole := roleVal.(string)

	if !okUID || !okRole {
		return nil, fmt.Errorf("internal server error: invalid user context format")
	}

	role := models.Role(roleStr)

	project, err := h.projectService.GetProjectByID(uint(id))
	if err != nil {
		return nil, fmt.Errorf("project not found")
	}

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, fmt.Errorf("project not found")
	}

	return project, nil
}
