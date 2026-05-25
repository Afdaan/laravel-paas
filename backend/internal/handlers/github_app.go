package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
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
	secret := os.Getenv("GITHUB_APP_WEBHOOK_SECRET")
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
		if err := h.db.Where("github_repo_owner = ? AND github_repo_name = ? AND branch = ? AND github_installation_id = ?",
			owner, repo, branch, payload.Installation.ID).Find(&projects).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		for _, p := range projects {
			isQueued, _ := h.redisService.IsProjectQueued(p.ID)
			if isQueued {
				slog.Info("Project already queued, skipping webhook trigger", "project_id", p.ID)
				continue
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
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "This GitHub installation is already linked to another account"})
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

	repos, err := h.githubService.ListRepositories(instID)
	if err != nil {
		if strings.Contains(err.Error(), "status=404") || strings.Contains(err.Error(), "status=401") {
			slog.Warn("GitHub installation was uninstalled or revoked on GitHub's side. Purging locally...", "installation_id", instID)
			h.db.Where("installation_id = ?", instID).Delete(&models.GithubAppInstallation{})
			h.db.Model(&models.Project{}).Where("github_installation_id = ?", instID).
				Updates(map[string]interface{}{
					"github_installation_id": nil,
					"github_repo_owner":      "",
					"github_repo_name":       "",
				})
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "This GitHub installation has been uninstalled or revoked. It has been unlinked from your account.",
				"code":  "INSTALLATION_REVOKED",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
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
	if err != nil {
		if strings.Contains(err.Error(), "status=404") || strings.Contains(err.Error(), "status=401") {
			slog.Warn("GitHub installation was uninstalled or revoked on GitHub's side during branch fetch. Purging locally...", "installation_id", inst.InstallationID)
			h.db.Where("installation_id = ?", inst.InstallationID).Delete(&models.GithubAppInstallation{})
			h.db.Model(&models.Project{}).Where("github_installation_id = ?", inst.InstallationID).
				Updates(map[string]interface{}{
					"github_installation_id": nil,
					"github_repo_owner":      "",
					"github_repo_name":       "",
				})
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "This GitHub installation has been uninstalled or revoked.",
				"code":  "INSTALLATION_REVOKED",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": branches})
}
