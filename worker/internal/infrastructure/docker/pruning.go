package docker

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	res, err := utils.Run(30*time.Second, "docker", "images", "--format", "{{.Tag}}", imageName)
	if err != nil {
		slog.Warn("Failed to list project images for deletion", "image", imageName, "error", err)
		return nil
	}
	tags := strings.Split(strings.TrimSpace(res.Stdout), "\n")
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		imgToDel := fmt.Sprintf("%s:%s", imageName, tag)
		if delRes, delErr := utils.Run(30*time.Second, "docker", "rmi", "-f", imgToDel); delErr != nil {
			slog.Warn("Failed to remove project image tag", "image", imgToDel, "error", delErr, "stderr", delRes.Stderr)
		} else {
			slog.Info("Successfully removed project image tag", "image", imgToDel)
		}
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
	filter := fmt.Sprintf("label=%s", models.LabelProjectManaged)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f", "--filter", filter); err != nil {
		slog.Warn("Failed to prune project images", "error", err)
	}

	return nil
}

// CleanupProject removes project files
func (s *DockerService) CleanupProject(userID uint, subdomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, fmt.Sprintf("user-%d", userID), subdomain)
	return os.RemoveAll(projectPath)
}
