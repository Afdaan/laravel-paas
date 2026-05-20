package project

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
)

// CreateProjectRequest represents project creation payload
type CreateProjectRequest struct {
	Name          string `json:"name"`
	GithubURL     string `json:"github_url"`
	Branch        string `json:"branch"`
	DatabaseName  string `json:"database_name"`
	BaseDirectory string `json:"base_directory"`
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

// UpdateRequest represents project update payload
type UpdateRequest struct {
	Name            string `json:"name"`
	Branch          string `json:"branch"`
	PHPVersion      string `json:"php_version"`
	BaseDirectory   string `json:"base_directory"`
	QueueEnabled    bool   `json:"queue_enabled"`
	WorkerCommand   string `json:"worker_command"`
	BuildCommand    string `json:"build_command"`
	StartCommand    string `json:"start_command"`
	NodeVersion     string `json:"node_version"`
	LanguageVersion string `json:"language_version"`
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

	project, err = h.projectService.UpdateProject(project.ID, project.UserID, project.User.Role, req.Name, req.Branch, req.PHPVersion, req.BaseDirectory, req.QueueEnabled, req.WorkerCommand, req.BuildCommand, req.StartCommand, req.NodeVersion, req.LanguageVersion)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update project"})
	}

	return c.JSON(project)
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
	if project.DeploymentStatus == models.DepStatusQueued {
		return c.JSON(fiber.Map{"logs": "Deployment is queued. Waiting for worker to start..."})
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

// StreamBuildLogs streams live build log output using Server-Sent Events (SSE)
func (h *ProjectHandler) StreamBuildLogs(c *fiber.Ctx) error {
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
		// 1. Read existing log file if it exists and write it as a single initial_logs event
		logPath := filepath.Join(h.cfg.ProjectsPath, project.Subdomain, "build.log")
		if logBytes, err := os.ReadFile(logPath); err == nil && len(logBytes) > 0 {
			// Limit to a reasonable size to avoid giant SSE messages, e.g. last 1MB
			const maxInitialBytes = 1 * 1024 * 1024
			if len(logBytes) > maxInitialBytes {
				logBytes = logBytes[len(logBytes)-maxInitialBytes:]
			}
			dataBytes, _ := json.Marshal(string(logBytes))
			_, err = w.WriteString(fmt.Sprintf("event: initial_logs\ndata: %s\n\n", string(dataBytes)))
			if err != nil {
				return
			}
			_ = w.Flush()
		}

		// 2. Subscribe to Redis build logs Pub/Sub for new logs
		msgChan, err := h.redisService.SubscribeBuildLogs(ctx, project.ID)
		if err != nil {
			return
		}

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-msgChan:
				if !ok {
					return
				}
				dataBytes, _ := json.Marshal(line)
				_, err := w.WriteString(fmt.Sprintf("event: log\ndata: %s\n\n", string(dataBytes)))
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

// StreamLogs streams live container logs using Server-Sent Events (SSE)
func (h *ProjectHandler) StreamLogs(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if project.ContainerID == nil || *project.ContainerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Container not running"})
	}

	containerID := *project.ContainerID
	logType := c.Query("type", "web")
	if logType == "worker" {
		if project.WorkerContainerID == nil || *project.WorkerContainerID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Worker container not running"})
		}
		containerID = *project.WorkerContainerID
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	h.projectService.UpdateActivity(project.ID)

	ctx := c.Context()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		cmdCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "docker", "logs", "-f", "--tail", "100", containerID)
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		cmd.Stderr = cmd.Stdout // Merge stderr into stdout pipe

		if err := cmd.Start(); err != nil {
			return
		}
		defer func() {
			// Context cancellation safely terminates the process via SIGKILL.
			// Reap process in background to prevent zombies without blocking closure.
			go func() {
				_ = cmd.Wait()
			}()
		}()

		linesChan := make(chan string, 100)
		go func() {
			scanner := bufio.NewScanner(stdoutPipe)
			// Expand scanner token size to 1MB to prevent "token too long" for large logs
			scanner.Buffer(make([]byte, 1024), 1024*1024)
			for scanner.Scan() {
				select {
				case linesChan <- scanner.Text():
				case <-cmdCtx.Done():
					return
				}
			}
			if err := scanner.Err(); err != nil {
				slog.Warn("Scanner error during log streaming", "error", err)
			}
			close(linesChan)
		}()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done(): // Detect client disconnect
				return
			case line, ok := <-linesChan:
				if !ok {
					return
				}
				dataBytes, _ := json.Marshal(line)
				_, err := w.WriteString(fmt.Sprintf("event: log\ndata: %s\n\n", string(dataBytes)))
				if err != nil {
					return
				}
			case <-ticker.C: // Periodic flush interval
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
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
	if err == nil && project.Status == models.StatusRunning && project.Port != nil && project.ContainerID != nil {
		// Cache hit! Forward with internal Docker routing
		// We use the container ID as the hostname because they are in the same paas-network
		targetURL := fmt.Sprintf("http://%s:%d", *project.ContainerID, *project.Port)

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
	if err := h.projectService.CacheSubdomainMapping(project_db); err != nil {
		slog.Warn("Failed to cache subdomain mapping during proxy fallback", "subdomain", subdomain, "error", err)
	}
	h.projectService.UpdateActivity(project_db.ID)

	if project_db.Port == nil || project_db.ContainerID == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Project container or port not configured"})
	}

	// 4. Forward with internal Docker routing
	targetURL := fmt.Sprintf("http://%s:%d", *project_db.ContainerID, *project_db.Port)
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

	projectStats := make(map[uint]docker.ContainerStats)
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
	idParam := c.Params("id")

	var project *models.Project
	var err error

	// 1. Try to fetch by UID column first (Standard)
	project, err = h.projectService.GetProjectByUID(idParam)
	if err != nil {
		// 2. Fallback: Check if it's a numeric ID (for admins or transition)
		id, errConv := strconv.Atoi(idParam)
		if errConv == nil {
			project, err = h.projectService.GetProjectByID(uint(id))
		}
	}

	if err != nil || project == nil {
		return nil, fmt.Errorf("project not found")
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

	// Permission checks

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, fmt.Errorf("project not found")
	}

	return project, nil
}

// CancelQueueJob cancels a queued or building deployment (Admin only)
func (h *ProjectHandler) CancelQueueJob(c *fiber.Ctx) error {
	idVal := c.Params("id")
	projectID, _ := strconv.Atoi(idVal)

	_, err := h.projectService.GetProjectByID(uint(projectID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	// 1. Broadcast cancellation across distributed worker cluster
	_ = h.redisService.PublishCancellation(c.Context(), uint(projectID))

	// 2. Remove from Redis Queue
	_ = h.redisService.RemoveFromQueue(uint(projectID))

	// 3. Release Redis Lock
	_ = h.redisService.ForceReleaseDeploymentLock(uint(projectID), "User cancelled deployment")

	// 4. Update project status to Failed
	_ = h.projectService.UpdateProjectStatus(uint(projectID), models.StatusFailed)

	return c.JSON(fiber.Map{"message": "Deployment cancelled successfully"})
}

// RequeueJob forcefully re-enqueues a stuck deployment (Admin only)
func (h *ProjectHandler) RequeueJob(c *fiber.Ctx) error {
	idVal := c.Params("id")
	projectID, _ := strconv.Atoi(idVal)

	project, err := h.projectService.GetProjectByID(uint(projectID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	// 1. Remove from queue if duplicate
	_ = h.redisService.RemoveFromQueue(uint(projectID))

	// 2. Release Redis Lock
	_ = h.redisService.ForceReleaseDeploymentLock(uint(projectID), "Admin manual requeue")

	// 3. Update project status to Queued
	_ = h.projectService.UpdateProjectStatus(uint(projectID), models.StatusQueued)

	// 4. Re-enqueue
	jobID, err := h.redisService.EnqueueDeployment(uint(projectID), project.UserID, "redeploy")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to re-enqueue job"})
	}

	_ = h.projectService.UpdateDeploymentStatus(uint(projectID), models.DepStatusQueued, "Admin manual requeue", 0, jobID)

	return c.JSON(fiber.Map{"message": "Job re-enqueued successfully"})
}
