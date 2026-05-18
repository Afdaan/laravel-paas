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

	// 1. Prepare backup of .env to a persistent temporary location outside the projectPath
	// to ensure it survives os.RemoveAll(projectPath)
	envPath := filepath.Join(projectPath, ".env")
	envBackupPath := filepath.Join(s.cfg.ProjectsPath, subdomain+".env.bak")

	hasEnv := false
	if _, err := os.Stat(envPath); err == nil {
		if err := utils.RunSilent(time.Minute, "cp", envPath, envBackupPath); err == nil {
			hasEnv = true
			slog.Info("Backed up .env file before sync", "subdomain", subdomain)
		}
	}

	// 2. Check if repository already exists and has a valid .git directory
	gitDir := filepath.Join(projectPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		slog.Info("Existing repository found, performing git pull / sync to avoid conflicts", "subdomain", subdomain, "branch", safeBranch)

		// Ensure remote origin URL is up to date
		_, _ = utils.Run(30*time.Second, "git", "-C", projectPath, "remote", "set-url", "origin", githubURL)

		// Fetch the latest commit from the remote branch
		fetchRes, err := utils.Run(3*time.Minute, "git", "-C", projectPath, "fetch", "--depth=1", "origin", safeBranch)
		if err != nil {
			slog.Warn("Git fetch failed, falling back to full fresh clone", "subdomain", subdomain, "error", fetchRes.Stderr)
		} else {
			// Reset hard to FETCH_HEAD to eliminate any merge conflicts
			_, resetErr := utils.Run(1*time.Minute, "git", "-C", projectPath, "reset", "--hard", "FETCH_HEAD")
			if resetErr == nil {
				// Clean untracked files
				_, _ = utils.Run(1*time.Minute, "git", "-C", projectPath, "clean", "-fd")

				// Get new commit hash
				hashRes, _ := utils.Run(10*time.Second, "git", "-C", projectPath, "rev-parse", "HEAD")
				commitHash := strings.TrimSpace(hashRes.Stdout)

				// Restore .env from backup
				if hasEnv {
					if err := utils.RunSilent(time.Minute, "cp", envBackupPath, envPath); err != nil {
						slog.Warn("Failed to restore .env file after sync", "subdomain", subdomain, "error", err)
					} else {
						slog.Info("Restored .env file after git pull", "subdomain", subdomain)
					}
				}

				slog.Info("Successfully updated repository via git pull", "subdomain", subdomain, "commitHash", commitHash)
				return projectPath, commitHash, nil
			}
			slog.Warn("Git reset failed, falling back to full fresh clone", "subdomain", subdomain)
		}
	}

	// 3. Use a truly unique temporary directory to avoid race conditions
	tempPath, err := os.MkdirTemp(s.cfg.ProjectsPath, subdomain+"_temp_*")
	if err != nil {
		return "", "", apperr.New(500, "TEMP_DIR_FAILED", "Failed to create temporary build directory: "+err.Error())
	}
	defer os.RemoveAll(tempPath)

	slog.Info("Cloning repository", "url", githubURL, "branch", safeBranch, "tempPath", tempPath)

	// 3. Clone the repository
	res, err := utils.Run(5*time.Minute, "git", "clone", "--depth=1", "-b", safeBranch, "--", githubURL, tempPath)
	if err != nil {
		return "", "", apperr.New(500, "GIT_CLONE_FAILED", fmt.Sprintf("Failed to clone repository: %s", res.Stderr))
	}

	// Get the commit hash
	hashRes, _ := utils.Run(10*time.Second, "git", "-C", tempPath, "rev-parse", "HEAD")
	commitHash := strings.TrimSpace(hashRes.Stdout)

	// 4. Swap temp clone into final project path
	if err := os.MkdirAll(s.cfg.ProjectsPath, 0755); err != nil {
		return "", "", apperr.New(500, "PROJECT_FS_ERROR", "Failed to prepare projects directory")
	}

	os.RemoveAll(projectPath)
	if err := os.Rename(tempPath, projectPath); err != nil {
		return "", "", apperr.New(500, "PROJECT_FS_ERROR", "Failed to finalize project directory: "+err.Error())
	}

	// 5. Restore .env from backup
	if hasEnv {
		if err := utils.RunSilent(time.Minute, "mv", envBackupPath, envPath); err != nil {
			slog.Warn("Failed to restore .env file after sync", "subdomain", subdomain, "error", err)
		} else {
			slog.Info("Restored .env file after sync", "subdomain", subdomain)
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
