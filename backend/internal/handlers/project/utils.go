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
	"github.com/laravel-paas/shared/pkg/utils"
)

// CreateProjectRequest represents project creation payload
type CreateProjectRequest struct {
	Name                   string `json:"name"`
	GithubURL              string `json:"github_url"`
	Branch                 string `json:"branch"`
	DatabaseOption         string `json:"database_option"`
	DatabaseName           string `json:"database_name"`
	BaseDirectory          string `json:"base_directory"`
	BuildCommand           string `json:"build_command"`
	StartCommand           string `json:"start_command"`
	QueueEnabled           bool   `json:"queue_enabled"`
	EnableDatabase         bool   `json:"enable_database"`
	DatabaseEngine         string `json:"database_engine"`
	DatabaseUsername       string `json:"database_username"`
	DatabasePassword       string `json:"database_password"`
	ExistingDatabaseUID    string `json:"existing_database_uid"`
	BillableSpecID         uint   `json:"billable_spec_id"`
	DatabaseBillableSpecID uint   `json:"database_billable_spec_id"`
	GithubInstallationID   *int64 `json:"github_installation_id,omitempty"`
	GithubRepoOwner        string `json:"github_repo_owner,omitempty"`
	GithubRepoName         string `json:"github_repo_name,omitempty"`
	Port                   *int   `json:"port,omitempty"`
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

	h.projectService.PopulateURL(project)

	c.Set("Cache-Control", "no-store")
	return c.JSON(struct {
		*models.Project
		ServerTime time.Time `json:"server_time"`
	}{Project: project, ServerTime: time.Now().UTC()})
}

// UpdateRequest represents project update payload
type UpdateRequest struct {
	Name                 string  `json:"name"`
	Branch               string  `json:"branch"`
	PHPVersion           *string `json:"php_version,omitempty"`
	BaseDirectory        string  `json:"base_directory"`
	BuildCommand         string  `json:"build_command"`
	StartCommand         string  `json:"start_command"`
	NodeVersion          *string `json:"node_version,omitempty"`
	LanguageVersion      *string `json:"language_version,omitempty"`
	WorkerCommand        *string `json:"worker_command,omitempty"`
	QueueEnabled         *bool   `json:"queue_enabled,omitempty"`
	Port                 *int    `json:"port,omitempty"`
	GithubURL            string  `json:"github_url"`
	GithubInstallationID *int64  `json:"github_installation_id,omitempty"`
	GithubRepoOwner      *string `json:"github_repo_owner,omitempty"`
	GithubRepoName       *string `json:"github_repo_name,omitempty"`
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

	project, err = h.projectService.UpdateProject(
		project.ID,
		project.UserID,
		project.User.Role,
		req.Name,
		req.Branch,
		req.PHPVersion,
		req.NodeVersion,
		req.BaseDirectory,
		req.QueueEnabled,
		req.WorkerCommand,
		req.BuildCommand,
		req.StartCommand,
		req.LanguageVersion,
		req.Port,
		req.GithubURL,
		req.GithubInstallationID,
		req.GithubRepoOwner,
		req.GithubRepoName,
	)
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

	return c.JSON(fiber.Map{"logs": logs})
}

func getActiveOrLatestLogPath(projectPath string, jobID string) string {
	logsDir := filepath.Join(projectPath, "logs")
	if jobID != "" {
		buildPath := filepath.Join(logsDir, fmt.Sprintf("build-%s.log", jobID))
		if utils.IsPathWithinRoot(projectPath, buildPath) {
			if _, err := os.Stat(buildPath); err == nil {
				return buildPath
			}
		}
		infraPath := filepath.Join(logsDir, fmt.Sprintf("infra-%s.log", jobID))
		if utils.IsPathWithinRoot(projectPath, infraPath) {
			if _, err := os.Stat(infraPath); err == nil {
				return infraPath
			}
		}
		// SRE Guard: Prevent fallback to stale log files when the requested job log does not exist yet.
		return buildPath
	}

	// Fallback to most recent file in logsDir
	if matches, err := filepath.Glob(filepath.Join(logsDir, "build-*.log")); err == nil && len(matches) > 0 {
		var newestFile string
		var newestTime time.Time
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil {
				if info.ModTime().After(newestTime) {
					newestTime = info.ModTime()
					newestFile = m
				}
			}
		}
		if infraMatches, err := filepath.Glob(filepath.Join(logsDir, "infra-*.log")); err == nil {
			for _, m := range infraMatches {
				if info, err := os.Stat(m); err == nil {
					if info.ModTime().After(newestTime) {
						newestTime = info.ModTime()
						newestFile = m
					}
				}
			}
		}
		if newestFile != "" {
			return newestFile
		}
	}

	// Final fallback to legacy build.log
	return filepath.Join(projectPath, "build.log")
}

// resolveLogPath searches multiple path options (with/without user folder, host/container configurations)
// to resolve the authoritative build log location.
func (h *ProjectHandler) resolveLogPath(project *models.Project, jobID string) string {
	// 1. Primary path: using configured ProjectsPath with user folder (multi-tenant layout)
	primaryPath := project.GetProjectPath(h.cfg.ProjectsPath)
	logPath := getActiveOrLatestLogPath(primaryPath, jobID)
	if utils.IsPathWithinRoot(h.cfg.ProjectsPath, primaryPath) {
		if _, err := os.Stat(logPath); err == nil {
			return logPath
		}
	}

	// 2. Fallback: try without user folder (legacy direct layout)
	legacyPath := filepath.Join(h.cfg.ProjectsPath, project.Subdomain)
	if utils.IsPathWithinRoot(h.cfg.ProjectsPath, legacyPath) {
		logPathFallback := getActiveOrLatestLogPath(legacyPath, jobID)
		if _, err := os.Stat(logPathFallback); err == nil {
			return logPathFallback
		}
	}

	// 3. Fallback: if PROJECTS_PATH is set to host path in .env, try standard container mount /app/storage/projects
	const defaultContainerPath = "/app/storage/projects"
	if h.cfg.ProjectsPath != defaultContainerPath {
		// With user folder
		containerPath := project.GetProjectPath(defaultContainerPath)
		if utils.IsPathWithinRoot(defaultContainerPath, containerPath) {
			logPathFallback := getActiveOrLatestLogPath(containerPath, jobID)
			if _, err := os.Stat(logPathFallback); err == nil {
				return logPathFallback
			}
		}

		// Without user folder
		containerLegacyPath := filepath.Join(defaultContainerPath, project.Subdomain)
		if utils.IsPathWithinRoot(defaultContainerPath, containerLegacyPath) {
			logPathFallback := getActiveOrLatestLogPath(containerLegacyPath, jobID)
			if _, err := os.Stat(logPathFallback); err == nil {
				return logPathFallback
			}
		}
	}

	// 4. Default: return primary path (so standard file-open handling propagates any errors naturally)
	return logPath
}

// BuildLogs returns the railpack build log output
const (
	// maxTailLines bounds ?tail=N so a caller cannot ask for the whole file
	// one line at a time.
	maxTailLines = 200
	// bytesPerTailLine is a generous per-line budget (real build log lines run
	// 60-150 bytes) used to size the read window for a tail request.
	bytesPerTailLine = 512
	// tailReadFloor covers the partial leading line plus short files.
	tailReadFloor = 2048
)

// tailLines returns the last n lines of s. partialFirst reports whether s starts
// mid-file, in which case its first line is a fragment and is dropped.
func tailLines(s string, n int, partialFirst bool) string {
	if n <= 0 {
		return s
	}
	if partialFirst {
		// A window that starts mid-file almost certainly starts mid-line. With no
		// line break at all the window is the tail of one very long line; keep it,
		// matching how the uncapped response truncates at maxBytes.
		if cut := strings.IndexByte(s, '\n'); cut >= 0 {
			s = s[cut+1:]
		}
	}
	trimmed := strings.TrimRight(s, "\n")
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (h *ProjectHandler) BuildLogs(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	if project.DeploymentStatus == models.DepStatusQueued {
		jobID := ""
		if project.DeploymentJobID != nil {
			jobID = *project.DeploymentJobID
		}
		return c.JSON(fiber.Map{
			"logs":        "Deployment is queued. Waiting for worker to start...",
			"job_id":      jobID,
			"available":   false,
			"placeholder": true,
		})
	}

	jobID := ""
	if project.DeploymentJobID != nil {
		jobID = *project.DeploymentJobID
	}
	logPath := h.resolveLogPath(project, jobID)
	f, err := os.Open(logPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("Failed to open build log file", "projectId", project.ID, "subdomain", project.Subdomain, "jobID", jobID, "path", logPath, "error", err.Error())
		}
		// Log not available yet or project not building
		return c.JSON(fiber.Map{
			"logs":        "Initializing build environment...",
			"job_id":      jobID,
			"available":   false,
			"placeholder": true,
		})
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return c.JSON(fiber.Map{
			"logs":        "Initializing build environment...",
			"job_id":      jobID,
			"available":   false,
			"placeholder": true,
		})
	}

	// Cap response size to avoid UI polling turning into a memory/CPU DoS.
	const maxBytes = 256 * 1024

	// A tail request wants the last N lines, not the whole buffer, so size the read
	// window for them instead. Bytes rather than a line-wise reverse scan: a single
	// log line can be enormous (stack traces, base64 blobs), and a byte window
	// bounds the read regardless of what is in the file.
	tail, _ := strconv.Atoi(c.Query("tail", "0"))
	if tail > maxTailLines {
		tail = maxTailLines
	}

	readSize := int64(maxBytes)
	if tail > 0 {
		readSize = int64(tail)*bytesPerTailLine + tailReadFloor
		if readSize > maxBytes {
			readSize = maxBytes
		}
	}

	size := st.Size()
	if size < readSize {
		readSize = size
	}

	buf := make([]byte, readSize)
	off := size - readSize
	if off < 0 {
		off = 0
	}
	_, _ = f.ReadAt(buf, off)

	logs := string(buf)
	if tail > 0 {
		logs = tailLines(logs, tail, off > 0)
	}

	return c.JSON(fiber.Map{
		"logs":        logs,
		"job_id":      jobID,
		"available":   size > 0,
		"placeholder": false,
	})
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
	c.Set("X-Accel-Buffering", "no")

	ctx := c.Context()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Immediately flush an initial keep-alive to force HTTP headers to be sent
		// preventing the browser/proxy from hanging and dropping the connection.
		_, _ = w.WriteString(":\n\n")
		_ = w.Flush()

		// 1. Read existing log file if it exists and write it as a single initial_logs event
		jobID := ""
		if project.DeploymentJobID != nil {
			jobID = *project.DeploymentJobID
		}
		logPath := h.resolveLogPath(project, jobID)
		if f, err := os.Open(logPath); err == nil {
			st, statErr := f.Stat()
			if statErr == nil && st.Size() > 0 {
				const maxInitialBytes = 1 * 1024 * 1024 // Limit to last 1MB
				size := st.Size()
				readSize := int64(maxInitialBytes)
				if size < readSize {
					readSize = size
				}
				buf := make([]byte, readSize)
				off := size - readSize
				if off < 0 {
					off = 0
				}
				n, _ := f.ReadAt(buf, off)
				f.Close() // Close immediately to release the file descriptor

				if n > 0 {
					logBytes := buf[:n]
					dataBytes, _ := json.Marshal(string(logBytes))
					_, err = w.WriteString(fmt.Sprintf("event: initial_logs\ndata: %s\n\n", string(dataBytes)))
					if err != nil {
						return
					}
					_ = w.Flush()
				}
			} else {
				f.Close()
			}
		}

		// 2. Subscribe to Redis build logs Pub/Sub for new logs
		msgChan, err := h.redisService.SubscribeBuildLogs(ctx, project.ID)
		if err != nil {
			return
		}

		keepAliveTicker := time.NewTicker(15 * time.Second)
		defer keepAliveTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-msgChan:
				if !ok {
					return
				}
				var liveLog infrastructure.BuildLogMessage
				if err := json.Unmarshal([]byte(line), &liveLog); err == nil && (liveLog.Line != "" || liveLog.JobID != "") {
					if liveLog.JobID != "" && jobID != "" && liveLog.JobID != jobID {
						continue
					}
					line = liveLog.Line
				}
				dataBytes, _ := json.Marshal(line)
				_, err := w.WriteString(fmt.Sprintf("event: log\ndata: %s\n\n", string(dataBytes)))
				if err != nil {
					return
				}
				_ = w.Flush()
			case <-keepAliveTicker.C:
				_, err := w.WriteString(":\n\n")
				if err != nil {
					return
				}
				_ = w.Flush()
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
	var workerLogPath string
	workerUsesContainer := false
	if logType == "worker" {
		if project.WorkerContainerID != nil && *project.WorkerContainerID != "" {
			containerID = *project.WorkerContainerID
			workerUsesContainer = true
		} else {
			res, err := utils.Run(15*time.Second, "docker", "exec", *project.ContainerID, "sh", "-c",
				`for f in /var/www/html/storage/logs/laravel-worker.log /var/www/html/storage/logs/worker.log /app/storage/logs/worker.log /app/worker.log /var/log/worker.log; do if [ -f "$f" ]; then echo "$f"; break; fi; done`)
			if err == nil {
				workerLogPath = strings.TrimSpace(res.Stdout)
			}
		}
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ctx := c.Context()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Immediately flush an initial keep-alive to force HTTP headers to be sent
		// preventing the browser/proxy from hanging and dropping the connection.
		_, _ = w.WriteString(":\n\n")
		_ = w.Flush()

		cmdCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var cmd *exec.Cmd
		if logType == "worker" && workerLogPath != "" {
			cmd = exec.CommandContext(cmdCtx, "docker", "exec", *project.ContainerID, "tail", "-f", "-n", "100", workerLogPath)
		} else if logType == "worker" && workerUsesContainer {
			cmd = exec.CommandContext(cmdCtx, "docker", "logs", "-f", "--tail", "100", containerID)
		} else if logType == "worker" {
			dataBytes, _ := json.Marshal("No worker logs found yet.")
			_, _ = w.WriteString(fmt.Sprintf("event: log\ndata: %s\n\n", string(dataBytes)))
			_ = w.Flush()
			return
		} else {
			cmd = exec.CommandContext(cmdCtx, "docker", "logs", "-f", "--tail", "100", containerID)
		}

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

		keepAliveTicker := time.NewTicker(15 * time.Second)
		defer keepAliveTicker.Stop()

		for {
			select {
			case <-ctx.Done(): // Detect client disconnect
				return
			case line, ok := <-linesChan:
				if !ok {
					return
				}
				line = utils.StripLogControlSequences(line)
				if logType == "web" && isWorkerRuntimeLogLine(line) {
					continue
				}
				dataBytes, _ := json.Marshal(line)
				_, err := w.WriteString(fmt.Sprintf("event: log\ndata: %s\n\n", string(dataBytes)))
				if err != nil {
					return
				}
			case <-keepAliveTicker.C:
				_, err := w.WriteString(":\n\n")
				if err != nil {
					return
				}
				_ = w.Flush()
			case <-ticker.C: // Periodic flush interval
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})

	return nil
}

func isWorkerRuntimeLogLine(line string) bool {
	normalized := strings.ToLower(line)
	return strings.Contains(normalized, "laravel-worker") ||
		strings.Contains(normalized, "queue:work") ||
		strings.Contains(normalized, "storage/logs/worker.log") ||
		strings.Contains(normalized, "storage/logs/laravel-worker.log") ||
		strings.Contains(normalized, "/app/worker.log") ||
		strings.Contains(normalized, "/var/log/worker.log")
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

	return c.JSON(stats)
}

// AdminStats returns overview statistics
func (h *ProjectHandler) AdminStats(c *fiber.Ctx) error {
	totalProjects, _ := h.projectService.GetTotalCount()
	runningProjects, _ := h.projectService.GetRunningCount()
	totalUsers, _ := h.userService.GetRegularUserCount()

	return c.JSON(fiber.Map{
		"total_projects":   totalProjects,
		"running_projects": runningProjects,
		"total_users":      totalUsers,
	})
}

// ProxyToProject forwards requests to the correct project container
func (h *ProjectHandler) ProxyToProject(c *fiber.Ctx) error {
	host := c.Hostname()
	subdomain := strings.Split(host, ".")[0]

	var project models.Project
	cacheKey := fmt.Sprintf("proxy:subdomain:%s", subdomain)

	// Helper to build target URL and preserve original request path + forwarded headers
	buildProxyTargetAndHeaders := func(proj *models.Project) string {
		targetURL := fmt.Sprintf("http://%s:%s", proj.GetTargetHostname(), proj.GetInternalPort())
		reqURL := c.OriginalURL()
		if reqURL == "" || reqURL == "/" {
			reqURL = "/"
		} else if !strings.HasPrefix(reqURL, "/") {
			reqURL = "/" + reqURL
		}

		// Preserve original Host and X-Forwarded headers so PHP/Laravel/Nginx gets the real host
		c.Request().Header.SetHost(host)
		c.Request().Header.Set("Host", host)
		c.Request().Header.Set("X-Forwarded-Host", host)

		proto := c.Protocol()
		if fwdProto := c.Get("X-Forwarded-Proto"); fwdProto != "" {
			proto = fwdProto
		}
		c.Request().Header.Set("X-Forwarded-Proto", proto)
		if proto == "https" {
			c.Request().Header.Set("X-Forwarded-Port", "443")
		} else {
			c.Request().Header.Set("X-Forwarded-Port", "80")
		}

		return targetURL + reqURL
	}

	// 1. Try Cache First
	err := h.redisService.GetCache(cacheKey, &project)
	if err == nil && project.Status == models.StatusRunning && project.ContainerID != nil {
		target := buildProxyTargetAndHeaders(&project)

		return proxy.Forward(target)(c)
	}

	// 2. Cache Miss: Fallback to Database
	project_db, err := h.projectService.GetBySubdomain(subdomain)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	if project_db.Status != models.StatusRunning {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found or not running"})
	}

	// 3. Populate Cache for the next request
	if err := h.projectService.CacheSubdomainMapping(project_db); err != nil {
		slog.Warn("Failed to cache subdomain mapping during proxy fallback", "subdomain", subdomain, "error", err)
	}
	if project_db.ContainerID == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Project container not configured"})
	}

	// 4. Forward with internal Docker routing
	target := buildProxyTargetAndHeaders(project_db)

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

	// Query project strictly by string UID.
	project, err := h.projectService.GetProjectByUID(idParam)
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
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	projectID := project.ID

	// 1. Broadcast cancellation across distributed worker cluster
	_ = h.redisService.PublishCancellation(c.Context(), uint(projectID))

	// 2. Remove from Redis Queue
	_ = h.redisService.RemoveFromQueue(uint(projectID))

	// 3. Release Redis Lock
	_ = h.redisService.ForceReleaseDeploymentLock(uint(projectID), "User cancelled deployment")

	// 4. Update project deployment status to Cancelled
	jobID := ""
	if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
		jobID = *project.DeploymentJobID
	}
	if _, errTransition := h.projectService.TransitionDeploymentState(c.Context(), uint(projectID), jobID, models.DepStatusCancelled, project.DeploymentProgress, "user_cancelled", "Deployment cancelled by user request"); errTransition != nil {
		slog.Warn("Failed to transition deployment state to cancelled", "project_id", projectID, "error", errTransition)
	}

	// 5. Update project status to Failed
	_ = h.projectService.UpdateProjectStatus(uint(projectID), models.StatusFailed)

	// 6. Update GitHub commit status to error/failure immediately so it doesn't get stuck
	if project.GithubInstallationID != nil && *project.GithubInstallationID != 0 && project.GithubRepoOwner != "" && project.GithubRepoName != "" && project.LastCommitHash != "" {
		githubService := infrastructure.NewGithubService(h.cfg, h.redisService)
		projectUID := project.UID
		if projectUID == "" {
			projectUID = fmt.Sprintf("%d", project.ID)
		}
		targetURL := fmt.Sprintf("%s/projects/%s?tab=build", h.cfg.FrontendURL, projectUID)

		instID := *project.GithubInstallationID
		owner := project.GithubRepoOwner
		repo := project.GithubRepoName
		commitHash := project.LastCommitHash

		createdAt := time.Now().UnixNano()
		statusPayload := &infrastructure.GithubStatusPayload{
			InstallationID: instID,
			Owner:          owner,
			Repo:           repo,
			SHA:            commitHash,
			State:          "error",
			TargetURL:      targetURL,
			Description:    "Deployment cancelled by user.",
			CreatedAt:      createdAt,
		}
		if err := h.redisService.SetDesiredCommitStatus(statusPayload); err != nil {
			slog.Warn("Failed to set desired commit status in Redis on cancellation", "project_id", projectID, "error", err)
		}

		go func() {
			errStatus := githubService.UpdateCommitStatus(instID, owner, repo, commitHash, "error", targetURL, "Deployment cancelled by user.")
			if errStatus == nil {
				_, _ = h.redisService.RemoveCommitStatusSyncIfMatched(commitHash, createdAt)
			} else {
				slog.Warn("Failed to update GitHub commit status on cancellation, queued for reconciler", "project_id", projectID, "error", errStatus)
			}
		}()
	}

	return c.JSON(fiber.Map{"message": "Deployment cancelled successfully"})
}

// RequeueJob forcefully re-enqueues a stuck deployment (Admin only)
func (h *ProjectHandler) RequeueJob(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	projectID := project.ID

	lockToken, err := h.redisService.ReserveAdminRequeue(projectID)
	if err != nil {
		slog.Error("Failed to reserve deployment lock for admin requeue", "project_id", projectID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to re-enqueue job"})
	}

	job := &infrastructure.DeploymentJob{
		ProjectID: projectID, UserID: project.UserID, Type: "redeploy",
		JobID: utils.GenerateRandomUID(), EnqueuedAt: time.Now(),
	}
	if err := h.projectService.RequeueDeployment(c.Context(), projectID, job.JobID, "Admin manual requeue"); err != nil {
		slog.Error("Failed to transition admin requeue in DB", "project_id", projectID, "job_id", job.JobID, "error", err)
		_ = h.redisService.ReleaseDeploymentLock(projectID, lockToken)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to re-enqueue job"})
	}
	if err := h.redisService.EnqueueReplacingDeploymentJob(job, lockToken); err != nil {
		if infrastructure.IsStaleOwnerError(err) {
			slog.Info("Admin requeue stale owner / lock lost", "project_id", projectID, "job_id", job.JobID)
			current, getErr := h.projectService.GetProjectByID(projectID)
			if getErr != nil || current == nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error": "Failed to verify project state after stale requeue",
				})
			}

			// Case 1: Another newer job superseded this one
			if current.DeploymentJobID != nil && *current.DeploymentJobID != job.JobID {
				newerJobID := *current.DeploymentJobID
				hasNewerJob, _ := h.redisService.HasDeploymentJob(newerJobID)
				isNewerActiveInDB := !models.IsTerminalDeploymentStatus(current.DeploymentStatus)

				if hasNewerJob || isNewerActiveInDB {
					return c.JSON(fiber.Map{
						"message":           "Deployment requeue superseded by newer active job",
						"superseded":        true,
						"superseded_by":     newerJobID,
						"deployment_status": current.DeploymentStatus,
					})
				}

				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":             "Deployment requeue superseded by another job that is no longer active",
					"superseded_by":     newerJobID,
					"deployment_status": current.DeploymentStatus,
				})
			}

			// Case 2: Cancellation intervened
			if current.DeploymentStatus == models.DepStatusCancelled {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":             "Deployment was cancelled before publication",
					"deployment_status": current.DeploymentStatus,
				})
			}

			// Case 3: Failed status
			if current.DeploymentStatus == models.DepStatusFailed || current.Status == models.StatusFailed {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":             "Deployment failed before publication",
					"deployment_status": current.DeploymentStatus,
				})
			}

			// Case 4: Lock lost or expired (DB was set to queued with job.JobID, but lock disappeared)
			_, _ = h.projectService.FailQueuedDeploymentIfUnchanged(
				c.Context(),
				projectID,
				job.JobID,
				"Deployment lock lost or expired before publication",
				true,
			)
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Deployment lock lost or expired before publication",
			})
		}
		slog.Error("Failed to enqueue admin requeue job", "project_id", projectID, "job_id", job.JobID, "error", err)

		// Uncertain publish reconciliation:
		// An EVAL command may commit in Redis and successfully enqueue the job, but network disruption
		// or timeout can cause the client to receive an error. Before asserting terminal failure,
		// inspect whether the predetermined job was actually enqueued or already claimed by a worker.
		inQueue, checkErr := h.redisService.HasDeploymentJob(job.JobID)
		if checkErr != nil {
			// If Redis cannot be inspected, do NOT destroy state by asserting failure.
			// Preserve the recoverable queued state so watchdog or operators can reconcile.
			slog.Warn("Cannot verify enqueue status after error; preserving queued state", "project_id", projectID, "job_id", job.JobID, "check_error", checkErr)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Failed to verify queue status; project remains queued"})
		}

		if inQueue {
			// The job exists in Redis (ready, delayed, or processing). The publish succeeded server-side.
			slog.Info("Admin requeue job was enqueued despite transport error", "project_id", projectID, "job_id", job.JobID)
			return c.JSON(fiber.Map{"message": "Job re-enqueued successfully"})
		}

		// Check if a worker already claimed the job and progressed it past queued in the database.
		if current, getErr := h.projectService.GetProjectByID(projectID); getErr == nil && current != nil {
			if current.DeploymentJobID != nil && *current.DeploymentJobID == job.JobID && current.DeploymentStatus != models.DepStatusQueued {
				slog.Info("Admin requeue job was claimed and progressed by worker despite transport error", "project_id", projectID, "job_id", job.JobID, "status", current.DeploymentStatus)
				return c.JSON(fiber.Map{"message": "Job re-enqueued successfully"})
			}
			if current.DeploymentJobID != nil && *current.DeploymentJobID != job.JobID {
				slog.Info("Admin requeue superseded by newer deployment job", "project_id", projectID, "job_id", job.JobID, "current_job_id", *current.DeploymentJobID)
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"error":         "Deployment requeue superseded by newer job",
					"superseded_by": *current.DeploymentJobID,
				})
			}
		}

		// Confirmed genuine failure: job is NOT in Redis and NOT claimed by any worker.
		// Failure compensation must be conditional on this job still owning the queued
		// database row inside the transaction, avoiding overwriting a newer concurrent job.
		applied, transitionErr := h.projectService.FailQueuedDeploymentIfUnchanged(c.Context(), projectID, job.JobID, "Admin requeue failed: queue unavailable", true)
		if transitionErr != nil {
			slog.Error("Failed to record admin requeue failure", "project_id", projectID, "job_id", job.JobID, "error", transitionErr)
		} else if !applied {
			slog.Warn("Admin requeue failure compensation skipped: project state owned by newer job or no longer queued", "project_id", projectID, "job_id", job.JobID)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to re-enqueue job"})
	}

	return c.JSON(fiber.Map{"message": "Job re-enqueued successfully"})
}
