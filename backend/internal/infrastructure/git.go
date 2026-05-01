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
	"strings"
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
// It returns the project path and the current commit hash.
func (s *GitService) CloneRepository(githubURL, branch, subdomain string) (string, string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)

	// 1. Sanitize branch to prevent any shell or path-traversal tricks
	safeBranch := filepath.Clean(branch)
	if strings.Contains(safeBranch, "..") || strings.HasPrefix(safeBranch, "/") {
		return "", "", apperr.New(400, "INVALID_BRANCH", "Invalid branch name provided")
	}

	// Backup existing .env before sync
	var envBackup []byte
	envPath := filepath.Join(projectPath, ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		envBackup = data
	}

	// 2. Use a truly unique temporary directory to avoid race conditions and collisions.
	// os.MkdirTemp ensures the folder is created with 0700 permissions by default on many systems.
	tempPath, err := os.MkdirTemp(s.cfg.ProjectsPath, subdomain+"_temp_*")
	if err != nil {
		return "", "", apperr.New(500, "TEMP_DIR_FAILED", "Failed to create temporary build directory: "+err.Error())
	}
	defer os.RemoveAll(tempPath) // Cleanup temp even on failure

	slog.Info("Cloning repository", "url", githubURL, "branch", safeBranch, "tempPath", tempPath)

	// 3. Use "--" to stop option parsing, preventing malicious URLs from injecting git flags.
	// We also use -c advice.detachedHead=false to keep logs clean.
	res, err := utils.Run(5*time.Minute, "git", "clone", "--depth=1", "-b", safeBranch, "--", githubURL, tempPath)

	if err != nil {
		return "", "", apperr.New(500, "GIT_CLONE_FAILED", fmt.Sprintf("Failed to clone repository: %s", res.Stderr))
	}

	// Get the commit hash of the cloned repo
	hashRes, _ := utils.Run(10*time.Second, "git", "-C", tempPath, "rev-parse", "HEAD")
	commitHash := strings.TrimSpace(hashRes.Stdout)

	// 4. Swap temp clone into final project path
	// Ensure the parent directory exists
	if err := os.MkdirAll(s.cfg.ProjectsPath, 0755); err != nil {
		return "", "", apperr.New(500, "PROJECT_FS_ERROR", "Failed to prepare projects directory")
	}

	os.RemoveAll(projectPath)
	if err := os.Rename(tempPath, projectPath); err != nil {
		return "", "", apperr.New(500, "PROJECT_FS_ERROR", "Failed to finalize project directory: "+err.Error())
	}

	// Restore .env if it existed before sync
	if envBackup != nil {
		if err := os.WriteFile(envPath, envBackup, 0644); err != nil {
			slog.Warn("Failed to restore .env file after sync", "subdomain", subdomain, "error", err)
		}
	}

	return projectPath, commitHash, nil
}

// GetRemoteCommitHash retrieves the latest commit hash of a remote branch without cloning.
func (s *GitService) GetRemoteCommitHash(githubURL, branch string) (string, error) {
	res, err := utils.Run(30*time.Second, "git", "ls-remote", githubURL, branch)
	if err != nil {
		return "", err
	}

	// Output format: <hash> \t <ref>
	parts := strings.Fields(res.Stdout)
	if len(parts) > 0 {
		return parts[0], nil
	}

	return "", fmt.Errorf("no commit hash found for branch %s", branch)
}
