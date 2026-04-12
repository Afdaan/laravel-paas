// ===========================================
// Git Service
// ===========================================
// Handles repository cloning and source code
// management for student projects
// ===========================================
package services

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
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

	args := []string{"clone", "--depth=1", "-b", branch, githubURL, tempPath}
	cmd := exec.Command("git", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", apperr.New(500, "GIT_CLONE_FAILED", fmt.Sprintf("Failed to clone repository from %s: %s", githubURL, stderr.String()))
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
