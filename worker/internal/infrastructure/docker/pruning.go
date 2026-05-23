package docker

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	// Try both with and without the paas- prefix in case naming varies
	if err := exec.Command("docker", "rmi", imageName).Run(); err != nil {
		slog.Warn("Failed to remove image", "image", imageName, "error", err)
	}
	return nil
}

// PruneImages removes dangling images and unused project images
func (s *DockerService) PruneImages() error {
	slog.Info("Starting Docker image pruning")

	// 1. Remove dangling images (<none>)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f"); err != nil {
		slog.Warn("Failed to prune dangling images", "error", err)
	}

	// 2. Also remove unused project images (those with our label)
	filter := fmt.Sprintf("label=%s=true", models.LabelProjectManaged)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-a", "-f", "--filter", filter); err != nil {
		slog.Warn("Failed to prune project images", "error", err)
	}

	return nil
}

// CleanupProject removes project files
func (s *DockerService) CleanupProject(subdomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.RemoveAll(projectPath)
}
