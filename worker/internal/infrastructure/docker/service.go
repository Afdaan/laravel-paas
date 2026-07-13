package docker

import (
	"log/slog"
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
