package docker

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	sharedDocker "github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
)

// ===========================================
// Docker Service
// ===========================================
// Manages Docker containers for user projects
// ===========================================
// DockerService handles all Docker operations
type DockerService struct {
	*sharedDocker.DockerService
	cfg     *config.Config
	storage *infrastructure.StorageService
}

// ResolveBuildPath picks a safe build root under a project's folder.
// If baseDirectory is invalid or escapes the project path, it falls back to auto-detection.
func (s *DockerService) ResolveBuildPath(projectPath string, baseDirectory string) string {
	if strings.TrimSpace(baseDirectory) == "" {
		return s.GetBuildPath(projectPath)
	}

	clean := filepath.Clean(baseDirectory)
	// Disallow absolute paths and traversal.
	if filepath.IsAbs(clean) || clean == "." || clean == string(os.PathSeparator) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		slog.Warn("Invalid base_directory; falling back to auto-detection", "base_directory", baseDirectory)
		return s.GetBuildPath(projectPath)
	}

	candidate := filepath.Join(projectPath, clean)
	if !utils.IsPathWithinRoot(projectPath, candidate) {
		slog.Warn("base_directory escapes project root; falling back to auto-detection", "base_directory", baseDirectory)
		return s.GetBuildPath(projectPath)
	}

	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}

	return s.GetBuildPath(projectPath)
}

// NewDockerService creates a new Docker service
func NewDockerService(cfg *config.Config, storage *infrastructure.StorageService, db *gorm.DB) *DockerService {
	s := &DockerService{
		DockerService: sharedDocker.NewDockerService(cfg, storage, db),
		cfg:           cfg,
		storage:       storage,
	}
	s.initializeBuildxBuilder()
	return s
}

func (s *DockerService) initializeBuildxBuilder() {
	slog.Info("Initializing BuildKit Buildx remote driver...")
	// Check if paas-builder already exists
	_, err := utils.Run(10*time.Second, "docker", "buildx", "inspect", "paas-builder")
	if err != nil {
		slog.Info("Creating paas-builder buildx remote driver targeting tcp://paas-buildkit:1234...")
		res, err := utils.Run(15*time.Second, "docker", "buildx", "create",
			"--name", "paas-builder",
			"--driver", "remote",
			"tcp://paas-buildkit:1234",
			"--use",
		)
		if err != nil {
			slog.Error("Failed to create buildx remote driver", "error", err, "stderr", res.Stderr)
		} else {
			slog.Info("Successfully created paas-builder remote buildx driver")
		}
	} else {
		slog.Info("paas-builder remote driver already exists")
	}
}

// GetBuildPath recursively finds the first directory containing project markers
func (s *DockerService) GetBuildPath(root string) string {
	markers := []string{
		"artisan",
		"composer.json",
		"package.json",
		"go.mod",
		"requirements.txt",
		"Gemfile",
		"Cargo.toml",
		"mix.exs",
		"index.html",
		"Staticfile",
	}

	// 1. Check root first
	for _, marker := range markers {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			return root
		}
	}

	// 2. If not at root, check first-level subdirectories (dynamic monorepo)
	// We only check 1 level deep to avoid accidental detection of nested vendor/node_modules
	entries, err := os.ReadDir(root)
	if err != nil {
		return root
	}

	// Priority subdirectories for monorepos and common project structures
	priorityDirs := []string{"backend", "app", "server", "api", "frontend", "web", "ui", "public", "src"}

	for _, pDir := range priorityDirs {
		pPath := filepath.Join(root, pDir)
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(pPath, marker)); err == nil {
				return pPath
			}
		}
	}

	// Fallback to searching all non-hidden directories
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirPath := filepath.Join(root, entry.Name())
			for _, marker := range markers {
				if _, err := os.Stat(filepath.Join(dirPath, marker)); err == nil {
					return dirPath
				}
			}
		}
	}

	return root
}
