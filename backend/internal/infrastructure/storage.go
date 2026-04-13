package infrastructure

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
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
		// We use -n (no-clobber) to avoid overwriting files the student/app has already created
		// We copy content of source/storage/app/* into path/ (which is the volume root)
		exec.Command("cp", "-an", projectSourceStorage+"/.", path).Run()
	}

	// Always ensure public exists as it's required for storage:link
	publicPath := filepath.Join(path, "public")
	os.MkdirAll(publicPath, 0777)

	fullUserPath := filepath.Join(s.cfg.DataPath, fmt.Sprintf("user-%d", project.UserID))
	if err := s.ChmodRecursive(fullUserPath, 0777); err != nil {
		slog.Error("Failed to apply storage permissions", "userId", project.UserID, "path", fullUserPath, "error", err)
	}

	return path
}

// ChmodRecursive applies permissions recursively to a path
func (s *StorageService) ChmodRecursive(path string, mode os.FileMode) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err == nil {
			return os.Chmod(name, mode)
		}
		return err
	})
}

// GetPersistentHostPath returns the path as seen by the Host OS
func (s *StorageService) GetPersistentHostPath(project *models.Project) string {
	s.EnsurePersistentPath(project)
	return filepath.Join(s.cfg.HostDataPath, fmt.Sprintf("user-%d", project.UserID), project.Subdomain, "storage")
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

	os.MkdirAll(filepath.Dir(dst), 0755)

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
