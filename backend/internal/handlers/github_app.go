package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"gorm.io/gorm"
)

type GithubAppHandler struct {
	db             *gorm.DB
	cfg            *config.Config
	githubService  *infrastructure.GithubService
	redisService   *infrastructure.RedisService
	projectService *projectServicePkg.ProjectService
}

func NewGithubAppHandler(db *gorm.DB, cfg *config.Config, githubService *infrastructure.GithubService, redisService *infrastructure.RedisService, projectService *projectServicePkg.ProjectService) *GithubAppHandler {
	return &GithubAppHandler{
		db:             db,
		cfg:            cfg,
		githubService:  githubService,
		redisService:   redisService,
		projectService: projectService,
	}
}

func (h *GithubAppHandler) Webhook(c *fiber.Ctx) error {
	metrics.GetCollector().IncrGithubWebhooksReceived()

	deliveryID := c.Get("X-GitHub-Delivery")
	if deliveryID != "" {
		key := fmt.Sprintf("github:webhook:processed:%s", deliveryID)
		ok, err := h.redisService.SetNX(key, true, 24*time.Hour)
		if err != nil {
			slog.Warn("Failed to check webhook delivery cache", "delivery_id", deliveryID, "error", err)
		} else if !ok {
			slog.Info("Duplicate webhook delivery detected, ignoring", "delivery_id", deliveryID)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Duplicate event ignored"})
		}
	}

	secret := h.cfg.GithubAppWebhookSecret
	if secret == "" {
		secret = os.Getenv("GITHUB_APP_WEBHOOK_SECRET")
	}
	body := c.Body()

	if secret == "" && h.cfg.AppEnv == "production" {
		slog.Error("Webhook signature validation bypassed: GITHUB_APP_WEBHOOK_SECRET is not configured in production")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal security misconfiguration"})
	}

	if secret != "" {
		signature := c.Get("X-Hub-Signature-256")
		if signature == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing signature"})
		}

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)
		expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC)

		if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid signature"})
		}
	}

	event := c.Get("X-GitHub-Event")
	if event == "push" {
		var payload struct {
			Ref        string `json:"ref"`
			Repository struct {
				Name  string `json:"name"`
				Owner struct {
					Login string `json:"login"`
				} `json:"owner"`
			} `json:"repository"`
			HeadCommit struct {
				ID      string `json:"id"`
				Message string `json:"message"`
			} `json:"head_commit"`
			Installation struct {
				ID int64 `json:"id"`
			} `json:"installation"`
		}

		if err := json.Unmarshal(body, &payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload"})
		}

		branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
		owner := payload.Repository.Owner.Login
		repo := payload.Repository.Name
		commitSHA := payload.HeadCommit.ID

		slog.Info("Received push webhook from GitHub", "repo", owner+"/"+repo, "branch", branch, "commit", commitSHA)

		var projects []models.Project
		if err := h.db.Where("LOWER(github_repo_owner) = LOWER(?) AND LOWER(github_repo_name) = LOWER(?) AND branch = ? AND github_installation_id = ?",
			owner, repo, branch, payload.Installation.ID).Find(&projects).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		for _, p := range projects {
			isQueued, _ := h.redisService.IsProjectQueued(p.ID)
			if isQueued {
				slog.Info("Project already queued, skipping webhook trigger", "project_id", p.ID)
				continue
			}

			// Update GitHub commit status to pending immediately (synchronous to prevent race conditions with the worker)
			if p.GithubInstallationID != nil && *p.GithubInstallationID != 0 && p.GithubRepoOwner != "" && p.GithubRepoName != "" && commitSHA != "" {
				projectUID := p.UID
				if projectUID == "" {
					projectUID = fmt.Sprintf("%d", p.ID)
				}
				targetURL := fmt.Sprintf("%s/projects/%s?tab=build", h.cfg.FrontendURL, projectUID)
				desc := "Build queued. Waiting for an available worker slot..."
				err := h.githubService.UpdateCommitStatus(*p.GithubInstallationID, p.GithubRepoOwner, p.GithubRepoName, commitSHA, "pending", targetURL, desc)
				if err != nil {
					slog.Warn("Failed to update initial GitHub commit status to pending", "project_id", p.ID, "error", err)
				}
			}

			// Update commit hash in DB
			h.db.Model(&p).Update("last_commit_hash", commitSHA)

			jobID, err := h.redisService.EnqueueDeployment(p.ID, p.UserID, "redeploy")
			if err != nil {
				slog.Error("Failed to enqueue push deployment from webhook", "project_id", p.ID, "error", err)
				continue
			}

			projectPath := filepath.Join(h.cfg.ProjectsPath, p.Subdomain)
			buildLogPath := filepath.Join(projectPath, "build.log")
			_ = os.MkdirAll(projectPath, 0755)
			_ = os.WriteFile(buildLogPath, []byte(""), 0644)

			if err := h.projectService.UpdateDeploymentStatus(p.ID, models.DepStatusQueued, "GitHub Push trigger: "+payload.HeadCommit.Message, 0, jobID); err != nil {
				slog.Warn("Failed to update status", "id", p.ID, "error", err)
			}
			h.projectService.UpdateActivity(p.ID)
		}

	} else if event == "installation" {
		var payload struct {
			Action       string `json:"action"`
			Installation struct {
				ID      int64 `json:"id"`
				Account struct {
					Login     string `json:"login"`
					AvatarURL string `json:"avatar_url"`
				} `json:"account"`
			} `json:"installation"`
		}

		if err := json.Unmarshal(body, &payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid payload"})
		}

		if payload.Action == "deleted" {
			slog.Info("GitHub App uninstalled", "installation_id", payload.Installation.ID)
			h.db.Where("installation_id = ?", payload.Installation.ID).Delete(&models.GithubAppInstallation{})
			h.db.Model(&models.Project{}).Where("github_installation_id = ?", payload.Installation.ID).
				Updates(map[string]interface{}{
					"github_installation_id": nil,
					"github_repo_owner":      "",
					"github_repo_name":       "",
				})
		}
	}

	metrics.GetCollector().IncrGithubWebhooksProcessed()
	return c.SendStatus(fiber.StatusOK)
}

func (h *GithubAppHandler) ListInstallations(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	var installations []models.GithubAppInstallation
	if err := h.db.Where("user_id = ?", userID).Find(&installations).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": installations})
}

func (h *GithubAppHandler) LinkInstallation(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	var req struct {
		InstallationID int64 `json:"installation_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	ghInst, err := h.githubService.GetInstallationDetails(req.InstallationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to fetch installation details from GitHub"})
	}

	var existing models.GithubAppInstallation
	if err := h.db.Where("installation_id = ?", req.InstallationID).First(&existing).Error; err == nil {
		if existing.UserID != userID {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "This GitHub account is already connected to another user profile. Please disconnect it from the other profile first or use a different GitHub account.",
			})
		}
	}

	inst := models.GithubAppInstallation{
		UserID:         userID,
		InstallationID: req.InstallationID,
		AccountName:    ghInst.Account.Login,
		AvatarURL:      ghInst.Account.AvatarURL,
	}

	if err := h.db.Where(models.GithubAppInstallation{InstallationID: req.InstallationID}).
		Assign(inst).FirstOrCreate(&inst).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save installation mapping"})
	}

	return c.JSON(fiber.Map{"message": "GitHub App connected successfully", "data": inst})
}

func (h *GithubAppHandler) ListRepositories(c *fiber.Ctx) error {
	instID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid installation ID"})
	}

	// Verify the installation belongs to the authenticated user before touching GitHub
	userID := c.Locals("user_id").(uint)
	var localInst models.GithubAppInstallation
	if err := h.db.Where("installation_id = ? AND user_id = ?", instID, userID).First(&localInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No GitHub App installation found"})
	}

	repos, err := h.githubService.ListRepositories(instID)
	if err != nil && isGitHubAuthError(err) {
		// Stale cached token is the most likely cause — bust it and retry once
		slog.Warn("GitHub API auth error, retrying with fresh token", "installation_id", instID)
		h.githubService.InvalidateInstallationToken(instID)
		repos, err = h.githubService.ListRepositories(instID)
	}
	if err != nil {
		if isGitHubAuthError(err) {
			slog.Warn("GitHub installation confirmed revoked after retry", "installation_id", instID)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "This GitHub installation is unauthorized or has been uninstalled. Please reconnect your GitHub App.",
				"code":  "INSTALLATION_REVOKED",
			})
		}
		slog.Error("Failed to list repositories", "installation_id", instID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch repositories from GitHub"})
	}

	return c.JSON(fiber.Map{"data": repos})
}

func (h *GithubAppHandler) ListBranches(c *fiber.Ctx) error {
	owner := c.Params("owner")
	repo := c.Params("repo")
	userID := c.Locals("user_id").(uint)

	var inst models.GithubAppInstallation
	if err := h.db.Where("user_id = ? AND account_name = ?", userID, owner).First(&inst).Error; err != nil {
		if err := h.db.Where("user_id = ?", userID).First(&inst).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "No GitHub App installation found"})
		}
	}

	branches, err := h.githubService.ListBranches(inst.InstallationID, owner, repo)
	if err != nil && isGitHubAuthError(err) {
		slog.Warn("GitHub API auth error on branch fetch, retrying with fresh token", "installation_id", inst.InstallationID)
		h.githubService.InvalidateInstallationToken(inst.InstallationID)
		branches, err = h.githubService.ListBranches(inst.InstallationID, owner, repo)
	}
	if err != nil {
		if isGitHubAuthError(err) {
			slog.Warn("GitHub installation confirmed revoked after retry (branches)", "installation_id", inst.InstallationID)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "This GitHub installation is unauthorized or has been uninstalled.",
				"code":  "INSTALLATION_REVOKED",
			})
		}
		slog.Error("Failed to list branches", "installation_id", inst.InstallationID, "owner", owner, "repo", repo, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch branches from GitHub"})
	}

	return c.JSON(fiber.Map{"data": branches})
}

// isGitHubAuthError checks whether a GitHub API error indicates a stale/revoked credential.
func isGitHubAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "status=404") || strings.Contains(msg, "status=401")
}
