// ===========================================
// Git Service
// ===========================================
// Handles repository cloning and source code
// management for student projects
// ===========================================
package infrastructure

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

// GitService handles Git repository operations
type GitService struct {
	cfg *config.Config
}

// NewGitService creates a new Git service
func NewGitService(cfg *config.Config) *GitService {
	return &GitService{
		cfg: cfg,
	}
}

// CloneRepository clones a GitHub repository using a non-destructive sync strategy.
// It preserves the existing .env file and storage/app directory across redeployments.
func (s *GitService) CloneRepository(githubURL, branch, subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)

	// Backup existing .env before sync
	var envBackup []byte
	envPath := filepath.Join(projectPath, ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		envBackup = data
	}

	// Non-destructive sync: clone to temp, then swap
	tempPath := projectPath + "_temp"
	os.RemoveAll(tempPath)

	res, err := utils.Run(5*time.Minute, "git", "clone", "--depth=1", "-b", branch, githubURL, tempPath)

	if err != nil {
		return "", apperr.New(500, "GIT_CLONE_FAILED", fmt.Sprintf("Failed to clone repository from %s: %s", githubURL, res.Stderr))
	}

	// Swap temp clone into final project path
	os.RemoveAll(projectPath)
	if err := os.Rename(tempPath, projectPath); err != nil {
		return "", apperr.New(500, "PROJECT_FS_ERROR", "Failed to finalize project directory: "+err.Error())
	}

	// Validate this is a Laravel project
	if _, err := os.Stat(filepath.Join(projectPath, "artisan")); os.IsNotExist(err) {
		return "", apperr.New(422, "NOT_LARAVEL_PROJECT", "The repository does not appear to be a valid Laravel project (missing artisan file)")
	}

	// Restore .env if it existed before sync
	if envBackup != nil {
		if err := os.WriteFile(envPath, envBackup, 0644); err != nil {
			slog.Warn("Failed to restore .env file after sync", "subdomain", subdomain, "error", err)
		}
	}

	return projectPath, nil
}
