package project

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
)

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

	// Set status to Queued so the UI shows immediate feedback
	if err := h.projectService.UpdateProjectStatus(project.ID, models.StatusQueued); err != nil {
		slog.Warn("Failed to update project status after env update", "id", project.ID, "error", err)
	}

	// Automatically trigger a redeploy to apply changes.
	// This is essential for frontend frameworks (Vite, Next.js) that need these
	// variables during the build phase.
	jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "redeploy")
	if err != nil {
		slog.Error("Failed to auto-enqueue redeployment after env update",
			"project_id", project.ID,
			"error", err)

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
		gitService := infrastructure.NewGitService(h.cfg)
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

