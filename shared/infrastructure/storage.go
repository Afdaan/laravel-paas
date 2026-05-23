package infrastructure

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

type StorageService struct {
	cfg *config.Config
}

func NewStorageService(cfg *config.Config) *StorageService {
	return &StorageService{cfg: cfg}
}

// EnsurePersistentPath ensures the hierarchical data path exists on host
func (s *StorageService) EnsurePersistentPath(project *models.Project) string {
	path := filepath.Join(s.cfg.DataPath, fmt.Sprintf("user-%d", project.UserID), project.Subdomain, "storage")

	if err := os.MkdirAll(path, 0777); err != nil {
		slog.Error("Failed to create storage path", "subdomain", project.Subdomain, "path", path, "error", err)
	}
	// Logic: Sync new files from Git Source to Persistent Storage
	// We use 'cp -an' to copy non-existing files and preserve attributes
	// projectSourceStorage = /home/afdaan/projects/subdomain/storage/app/
	// path = /home/afdaan/data/user-1/subdomain/storage/
	projectSourceStorage := filepath.Join(s.cfg.ProjectsPath, project.Subdomain, "storage", "app")

	if _, err := os.Stat(projectSourceStorage); err == nil {
		slog.Info("Syncing source assets to persistent storage", "subdomain", project.Subdomain)
		// We use -n (no-clobber) to avoid overwriting files the user/app has already created.
		// We use -d to ensure symlinks are copied as symlinks and not followed.
		// This prevents users from symlinking /etc/shadow into their storage and having us copy it.
		if err := exec.Command("cp", "-and", projectSourceStorage+"/.", path).Run(); err != nil {
			slog.Warn("Failed to sync source assets", "subdomain", project.Subdomain, "error", err)
		}
	}

	// Always ensure public exists as it's required for storage:link
	publicPath := filepath.Join(path, "public")
	if err := os.MkdirAll(publicPath, 0777); err != nil {
		slog.Warn("Failed to create public storage path", "path", publicPath, "error", err)
	}

	fullUserPath := filepath.Join(s.cfg.DataPath, fmt.Sprintf("user-%d", project.UserID))
	if err := s.ChmodRecursive(fullUserPath, 0777); err != nil {
		slog.Error("Failed to apply storage permissions", "userId", project.UserID, "path", fullUserPath, "error", err)
	}

	return path
}

// ChmodRecursive applies permissions recursively to a path, but safely skips symbolic links
// to prevent following them to sensitive system files.
func (s *StorageService) ChmodRecursive(path string, mode os.FileMode) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Use Lstat to check the type of the entry without following it.
		// Standard Walk provides info, but we re-verify with Lstat for maximum safety.
		lInfo, err := os.Lstat(name)
		if err != nil {
			return err
		}

		// Skip symbolic links to avoid privilege escalation via symlink-to-system-file attacks.
		// On Linux, chmod on a symlink follows it to the target.
		if lInfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		return os.Chmod(name, mode)
	})
}

// GetPersistentHostPath returns the path as seen by the Host OS
func (s *StorageService) GetPersistentHostPath(project *models.Project) string {
	s.EnsurePersistentPath(project)
	return filepath.Join(s.cfg.HostDataPath, fmt.Sprintf("user-%d", project.UserID), project.Subdomain, "storage")
}

// GetProjectsHostPath returns the project path as seen by the Host OS
func (s *StorageService) GetProjectsHostPath(subdomain string) string {
	return filepath.Join(s.cfg.HostProjectsPath, subdomain)
}

// CleanupPersistentData removes project storage folders
func (s *StorageService) CleanupPersistentData(project *models.Project) {
	path := filepath.Join(s.cfg.DataPath, fmt.Sprintf("user-%d", project.UserID), project.Subdomain)
	slog.Info("Cleaning up persistent data", "path", path)
	os.RemoveAll(path)
}

// CopyFile helper for general file copying
func (s *StorageService) CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// GetEnvFile reads the .env file for a project
func (s *StorageService) GetEnvFile(subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project
func (s *StorageService) SaveEnvFile(subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(content), 0644)
}

