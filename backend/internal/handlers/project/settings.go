package project

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// GetEnv returns the compiled dotenv environment variables for the project from SecretStore
func (h *ProjectHandler) GetEnv(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	envMap, err := h.secretStoreService.CompileEnvForProject(project.ID, "production")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to compile environment variables"})
	}

	envContent := utils.FormatEnvMap(envMap)

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"content": envContent})
}

// UpdateEnvRequest represents env update payload
type UpdateEnvRequest struct {
	Content string `json:"content"`
}

// UpdateEnv updates variables inside the project's SecretStore and triggers deployment
func (h *ProjectHandler) UpdateEnv(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req UpdateEnvRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Parse the raw dotenv content into key-value map.
	lines := strings.Split(req.Content, "\n")
	parsedSecrets := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
				v = v[1 : len(v)-1]
			} else {
				// Strip unquoted trailing comments
				if idx := strings.Index(v, "#"); idx != -1 {
					v = strings.TrimSpace(v[:idx])
				}
			}
			parsedSecrets[k] = v
		}
	}

	// Locate or create the project's SecretStore container.
	var binding models.SecretStoreBinding
	errBinding := h.db.Where("project_id = ?", project.ID).First(&binding).Error
	var storeID uint
	if errBinding != nil {
		store, errStore := h.secretStoreService.CreateSecretStore(project.UserID, fmt.Sprintf("Environment Secrets (%s)", project.Name), "Managed variables for project "+project.Name, c.IP(), c.Get("User-Agent"))
		if errStore != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create secret store container"})
		}
		storeID = store.ID

		_, errBind := h.secretStoreService.BindSecretStore(project.UserID, storeID, project.ID, "production", c.IP(), c.Get("User-Agent"))
		if errBind != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to bind secret store container"})
		}
	} else {
		storeID = binding.SecretStoreID
	}

	// Set or update secret variables without individual propagation triggers.
	for k, v := range parsedSecrets {
		if k == "" {
			continue
		}
		if _, errSet := h.secretStoreService.SetSecretValueNoPropagate(project.UserID, storeID, k, v, c.IP(), c.Get("User-Agent")); errSet != nil {
			slog.Error("Failed to set secret value from env update", "key", k, "error", errSet)
		}
	}

	// Safely clean up variables removed from the dotenv text area.
	var items []models.SecretStoreItem
	if errItems := h.db.Where("secret_store_id = ?", storeID).Find(&items).Error; errItems == nil {
		for _, item := range items {
			if _, exists := parsedSecrets[item.Key]; !exists {
				h.db.Delete(&item)
				h.secretStoreService.LogActivity(project.UserID, &storeID, &item.ID, &project.ID, "delete_secret_key", "Removed secret key: "+item.Key, c.IP(), c.Get("User-Agent"))
			}
		}
	}

	// Propagate updates to all other projects bound to this store, skipping the current project
	// since we explicitly queue a redeployment for it at the end of this handler.
	go h.secretStoreService.PropagateSecretStoreUpdatesExcept(storeID, project.ID)

	h.projectService.UpdateActivity(project.ID)

	if err := h.projectService.UpdateProjectStatus(project.ID, models.StatusQueued); err != nil {
		slog.Warn("Failed to update project status after env update", "id", project.ID, "error", err)
	}

	jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "update_env")
	if err != nil {
		slog.Error("Failed to enqueue redeployment after env update", "project_id", project.ID, "error", err)
		return c.JSON(fiber.Map{
			"message": "Environment variables saved, but failed to queue auto-redeploy. Please redeploy manually.",
		})
	}

	if err := h.projectService.UpdateDeploymentStatus(project.ID, models.DepStatusQueued, "Auto-redeploy triggered by environment update", 0, jobID); err != nil {
		slog.Warn("Failed to update deployment status on env update", "id", project.ID, "error", err)
	}

	return c.JSON(fiber.Map{
		"message": "Environment variables updated. A new build has been queued to apply changes.",
	})
}

// ListBranches returns all branches for the project's repository
func (h *ProjectHandler) ListBranches(c *fiber.Ctx) error {
	project, err := h.getProject(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var branches []string

	// If it's a GitHub App-connected project, fetch via GitHub App Service
	if project.GithubInstallationID != nil && *project.GithubInstallationID != 0 && project.GithubRepoOwner != "" && project.GithubRepoName != "" {
		githubService := infrastructure.NewGithubService(h.cfg, h.redisService)
		instID := *project.GithubInstallationID
		
		ghBranches, err := githubService.ListBranches(instID, project.GithubRepoOwner, project.GithubRepoName)
		if err != nil && (strings.Contains(err.Error(), "status=404") || strings.Contains(err.Error(), "status=401")) {
			githubService.InvalidateInstallationToken(instID)
			ghBranches, err = githubService.ListBranches(instID, project.GithubRepoOwner, project.GithubRepoName)
		}
		
		if err != nil {
			slog.Error("Failed to list branches via GitHub App", "project_id", project.ID, "installation_id", instID, "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch branches from GitHub"})
		}
		
		for _, b := range ghBranches {
			branches = append(branches, b.Name)
		}
	} else if project.GithubURL != "" {
		// Manual Git connected project
		gitService := infrastructure.NewGitService(h.cfg, h.db)
		remoteBranches, err := gitService.GetRemoteBranches(project.GithubURL)
		if err != nil {
			slog.Error("Failed to list remote branches via git ls-remote", "project_id", project.ID, "url", project.GithubURL, "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch remote branches"})
		}
		branches = remoteBranches
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Project has no Git source configured"})
	}

	h.projectService.UpdateActivity(project.ID)

	return c.JSON(fiber.Map{"data": branches})
}

