package project

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm/clause"
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

	roleVal := c.Locals("role")
	roleStr, _ := roleVal.(string)
	role := models.Role(roleStr)

	// Basic validation
	if req.Name == "" || req.GithubURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Name and Github URL are required",
		})
	}

	if req.GithubInstallationID != nil && *req.GithubInstallationID != 0 {
		if req.GithubRepoOwner == "" || req.GithubRepoName == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "GitHub repository owner and name are required when installation ID is provided",
			})
		}
	}

	if req.GithubInstallationID != nil && *req.GithubInstallationID != 0 {
		var localInst models.GithubAppInstallation
		if err := h.db.Where("installation_id = ? AND user_id = ?", *req.GithubInstallationID, userID).First(&localInst).Error; err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "The specified GitHub installation does not belong to your account",
			})
		}

		githubService := infrastructure.NewGithubService(h.cfg, h.redisService)
		repos, err := githubService.ListRepositories(*req.GithubInstallationID)
		if err != nil {
			// Retry with fresh token — handles stale cache after GitHub App reinstall
			slog.Warn("GitHub API error during repo validation, retrying with fresh token", "installation_id", *req.GithubInstallationID, "error", err)
			githubService.InvalidateInstallationToken(*req.GithubInstallationID)
			repos, err = githubService.ListRepositories(*req.GithubInstallationID)
		}
		if err != nil {
			slog.Warn("Failed to list repositories for validation after retry", "installation_id", *req.GithubInstallationID, "error", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to verify repository access with GitHub. Please check your GitHub App configuration.",
			})
		}

		expectedFullName := fmt.Sprintf("%s/%s", req.GithubRepoOwner, req.GithubRepoName)
		found := false
		for _, r := range repos {
			if strings.EqualFold(r.FullName, expectedFullName) {
				found = true
				break
			}
		}

		if !found {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "The repository is not authorized or does not exist under the specified GitHub App installation.",
			})
		}
	}

	// Fallback mapping for DatabaseOption
	dbOption := req.DatabaseOption
	if dbOption == "" {
		if req.EnableDatabase {
			dbOption = "new"
		} else {
			dbOption = "none"
		}
	}

	switch dbOption {
	case "none", "sqlite", "new", "existing", "external":
		// Valid options
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid database_option. Must be one of: none, sqlite, new, existing, external",
		})
	}

	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Validate and lock existing database if option is "existing"
	var existingDb *models.DatabaseInstance
	if dbOption == "existing" {
		if req.ExistingDatabaseUID == "" {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "existing_database_uid is required when database_option is existing",
			})
		}
		var dbInst models.DatabaseInstance
		// Acquire row lock to prevent race conditions on concurrent attachment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", req.ExistingDatabaseUID).First(&dbInst).Error; err != nil {
			tx.Rollback()
			slog.Warn("Existing database not found during locking", "uid", req.ExistingDatabaseUID, "error", err)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Database not found",
			})
		}
		if dbInst.UserID != userID {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Forbidden: You do not own this database",
			})
		}
		if dbInst.Status != models.DBStatusActive {
			tx.Rollback()
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Database is not active",
			})
		}
		if dbInst.ProjectID != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Database is already attached to another project",
			})
		}
		existingDb = &dbInst
	}

	// Create project record inside the transaction
	project, err := h.projectService.CreateProjectTx(tx, userID, role, req.Name, req.GithubURL, req.Branch, dbOption, req.DatabaseName, req.DatabaseUsername, req.DatabasePassword, req.BaseDirectory, req.BuildCommand, req.StartCommand, req.Port, req.QueueEnabled, req.DatabaseEngine, req.GithubInstallationID, req.GithubRepoOwner, req.GithubRepoName)
	if err != nil {
		tx.Rollback()
		slog.Warn("Project creation failed", "user_id", userID, "error", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Handle attachment for "existing" database inside transaction
	if dbOption == "existing" && existingDb != nil {
		projID := project.ID
		existingDb.ProjectID = &projID
		if err := tx.Save(existingDb).Error; err != nil {
			tx.Rollback()
			slog.Error("Failed to attach existing database", "project_id", projID, "database_uid", req.ExistingDatabaseUID, "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to attach existing database",
			})
		}

		project.DatabaseName = &existingDb.Name
		project.DatabasePassword = existingDb.Password
		project.DatabaseOption = "existing"
		if err := tx.Model(project).Updates(map[string]interface{}{
			"database_name":     project.DatabaseName,
			"database_password": project.DatabasePassword,
			"database_option":   project.DatabaseOption,
		}).Error; err != nil {
			tx.Rollback()
			slog.Error("Failed to update project database details", "project_id", projID, "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update project database details",
			})
		}
	}

	// Handle environment injection for SQLite inside transaction
	if dbOption == "sqlite" {
		store, errStore := h.secretStoreService.CreateSecretStoreTx(tx, userID, fmt.Sprintf("Environment Secrets (%s)", project.Name), "Managed variables for project "+project.Name, c.IP(), c.Get("User-Agent"))
		if errStore != nil {
			tx.Rollback()
			slog.Error("Failed to create secret store container for SQLite", "project_id", project.ID, "error", errStore)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create secret store container for SQLite",
			})
		}
		_, errBind := h.secretStoreService.BindSecretStoreTx(tx, userID, store.ID, project.ID, "production", c.IP(), c.Get("User-Agent"))
		if errBind != nil {
			tx.Rollback()
			slog.Error("Failed to bind secret store container for SQLite", "project_id", project.ID, "store_id", store.ID, "error", errBind)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to bind secret store container for SQLite",
			})
		}
		if _, errSet := h.secretStoreService.SetSecretValueNoPropagateTx(tx, userID, store.ID, "DB_CONNECTION", "sqlite", c.IP(), c.Get("User-Agent")); errSet != nil {
			tx.Rollback()
			slog.Error("Failed to set SQLite DB_CONNECTION", "project_id", project.ID, "store_id", store.ID, "error", errSet)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to set SQLite database connection settings",
			})
		}
		if _, errSet := h.secretStoreService.SetSecretValueNoPropagateTx(tx, userID, store.ID, "DB_DATABASE", "/var/www/html/database/database.sqlite", c.IP(), c.Get("User-Agent")); errSet != nil {
			tx.Rollback()
			slog.Error("Failed to set SQLite DB_DATABASE", "project_id", project.ID, "store_id", store.ID, "error", errSet)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to set SQLite database path settings",
			})
		}
		if _, errSet := h.secretStoreService.SetSecretValueNoPropagateTx(tx, userID, store.ID, "DB_FOREIGN_KEYS", "true", c.IP(), c.Get("User-Agent")); errSet != nil {
			tx.Rollback()
			slog.Error("Failed to set SQLite DB_FOREIGN_KEYS", "project_id", project.ID, "store_id", store.ID, "error", errSet)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to set SQLite database foreign key settings",
			})
		}
	}

	if err := tx.Commit().Error; err != nil {
		slog.Error("Transaction commit failed during project creation", "user_id", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Transaction commit failed",
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
		jobID := ""
		if project.DeploymentJobID != nil {
			jobID = *project.DeploymentJobID
		}
		return c.JSON(fiber.Map{
			"message":           "Project is already in queue",
			"job_id":            jobID,
			"deployment_status": project.DeploymentStatus,
			"queue_position":    queueLength,
		})
	}

	clean := c.Query("clean")
	jobType := "redeploy"
	statusMsg := "Redeployment requested by user"
	if clean == "true" {
		jobType = "redeploy_clean"
		statusMsg = "Clean rebuild requested by user"
	}

	// Enqueue redeployment job to Redis
	jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, jobType)
	if err != nil {
		slog.Error("Failed to enqueue redeployment", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue redeployment",
		})
	}

	// Truncate the build log immediately to prevent old logs from displaying
	projectPath := project.GetProjectPath(h.cfg.ProjectsPath)
	buildLogPath := filepath.Join(projectPath, "build.log")
	_ = os.MkdirAll(projectPath, 0755)
	_ = os.WriteFile(buildLogPath, []byte(""), 0644)

	if err := h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, statusMsg, 0, jobID); err != nil {
		if cleanupErr := h.redisService.RemoveDeploymentJob(jobID); cleanupErr != nil {
			slog.Error("Failed to remove queued redeployment after state transition failure",
				"project_id", project.ID,
				"job_id", jobID,
				"transition_error", err,
				"cleanup_error", cleanupErr,
			)
		}
		slog.Error("Failed to update project deployment status to queued",
			"project_id", project.ID,
			"job_id", jobID,
			"error", err,
		)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to queue redeployment",
		})
	}
	h.projectService.UpdateActivity(project.ID)

	// Get queue position
	queueLength, _ := h.redisService.GetQueueLength()

	return c.JSON(fiber.Map{
		"message":           "Redeployment queued successfully",
		"job_id":            jobID,
		"deployment_status": models.DepStatusQueued,
		"queue_position":    queueLength,
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

// Rollback restores a previous deployment instantly if cached locally or falls back to rebuild
func (h *ProjectHandler) Rollback(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req struct {
		CommitSHA string `json:"commit_sha"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	if req.CommitSHA == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "commit_sha is required"})
	}

	imageTag := fmt.Sprintf("paas-%s:%s", project.Subdomain, req.CommitSHA)
	inspectRes, inspectErr := utils.Run(10*time.Second, "docker", "image", "inspect", imageTag)
	imageExists := inspectErr == nil && strings.TrimSpace(inspectRes.Stdout) != ""

	if imageExists {
		slog.Info("Performing instant rollback using existing local image", "project", project.Subdomain, "commit", req.CommitSHA)

		project.LastCommitHash = req.CommitSHA
		h.db.Model(project).Update("last_commit_hash", req.CommitSHA)

		jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "rollback")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to enqueue rollback"})
		}

		_ = h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Instant rollback to "+req.CommitSHA, 0, jobID)

		return c.JSON(fiber.Map{
			"message": "Instant rollback initiated successfully",
			"type":    "instant",
		})
	} else {
		slog.Info("Image not found locally, performing rebuild/redeploy fallback", "project", project.Subdomain, "commit", req.CommitSHA)

		project.LastCommitHash = req.CommitSHA
		h.db.Model(project).Update("last_commit_hash", req.CommitSHA)

		jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to enqueue redeploy fallback"})
		}

		projectPath := project.GetProjectPath(h.cfg.ProjectsPath)
		buildLogPath := filepath.Join(projectPath, "build.log")
		_ = os.MkdirAll(projectPath, 0755)
		_ = os.WriteFile(buildLogPath, []byte(""), 0644)

		_ = h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Rebuild fallback for rollback to "+req.CommitSHA, 0, jobID)

		return c.JSON(fiber.Map{
			"message": "Rebuild rollback initiated successfully",
			"type":    "rebuild",
		})
	}
}
