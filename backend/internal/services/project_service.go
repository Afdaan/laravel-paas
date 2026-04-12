// ===========================================
// Project Service
// ===========================================
// Centralized business logic for project management
// ===========================================
package services

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
)

type ProjectService struct {
	cfg           *config.Config
	projectRepo   repositories.ProjectRepository
	settingService *SettingService
	dockerService *DockerService
	nginxService  *NginxWebhookService
	redisService  *RedisService
}

func NewProjectService(
	cfg *config.Config, 
	projectRepo repositories.ProjectRepository,
	settingService *SettingService,
	redisService *RedisService,
) *ProjectService {
	return &ProjectService{
		cfg:           cfg,
		projectRepo:   projectRepo,
		settingService: settingService,
		dockerService: NewDockerService(cfg),
		nginxService:  NewNginxWebhookService(cfg),
		redisService:  redisService,
	}
}

// GetProjectByID fetches a project with preloaded associations
func (s *ProjectService) GetProjectByID(id uint) (*models.Project, error) {
	return s.projectRepo.GetByID(id)
}

// DeleteProject performs a thorough cleanup of all project resources
func (s *ProjectService) DeleteProject(project *models.Project) error {
	slog.Info("Executing thorough project deletion", 
		"id", project.ID, 
		"name", project.Name, 
		"subdomain", project.Subdomain)

	// 1. Remove Docker Container
	if project.ContainerID != nil {
		s.dockerService.RemoveContainer(*project.ContainerID)
	}

	// 2. Remove Docker Image
	s.dockerService.RemoveImage(project.Subdomain)

	// 3. Cleanup Filesystem (Source Code & Persistent Data)
	s.dockerService.CleanupProject(project.Subdomain)
	s.dockerService.CleanupPersistentData(project)

	// 4. Drop Student Database
	if project.DatabaseName != "" {
		s.dockerService.DropDatabase(project.DatabaseName)
	}

	// 5. Sync Deletion to Nginx Proxy
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	s.nginxService.DeleteProject(project, projectDomain)

	// Invalidate Redis Proxy Cache
	s.InvalidateSubdomainCache(project.Subdomain)

	// 6. Hard Delete from Database
	if err := s.projectRepo.Delete(project.ID); err != nil {
		slog.Error("Failed to delete project record from database", "id", project.ID, "error", err)
		return err
	}

	// 7. Cleanup dangling images after deletion
	go s.dockerService.PruneImages()

	slog.Info("Project thoroughly purged from system", "name", project.Name)
	return nil
}

// GetSetting fetches a platform setting with a fallback
func (s *ProjectService) GetSetting(key, defaultValue string) string {
	return s.settingService.Get(key, defaultValue)
}

// UpdateActivity updates the last_accessed_at and expires_at fields
func (s *ProjectService) UpdateActivity(projectID uint) {
	go func() {
		now := time.Now()
		expiryDays, _ := strconv.Atoi(s.GetSetting(models.SettingProjectExpiry, models.DefaultProjectExpiry))

		project, err := s.projectRepo.GetByID(projectID)
		if err != nil {
			slog.Error("Failed to fetch project for activity update", "id", projectID, "error", err)
			return
		}

		project.LastAccessedAt = &now
		if expiryDays > 0 {
			expire := now.AddDate(0, 0, expiryDays)
			project.ExpiresAt = &expire
		} else {
			project.ExpiresAt = nil
		}

		if err := s.projectRepo.Update(project); err != nil {
			slog.Error("Failed to update project activity", "id", projectID, "error", err)
		}
	}()
}

// PopulateURL sets the URL field on a project model
func (s *ProjectService) PopulateURL(project *models.Project) {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	project.URL = "https://" + project.GetFullDomain(projectDomain)
}

// PopulateURLs sets the URL field on a slice of project models
func (s *ProjectService) PopulateURLs(projects []models.Project) {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	for i := range projects {
		projects[i].URL = "https://" + projects[i].GetFullDomain(projectDomain)
	}
}

// GetLogs returns container logs
func (s *ProjectService) GetLogs(containerID string, lines int) (string, error) {
	return s.dockerService.GetContainerLogs(containerID, lines)
}

// GetStats returns container resource usage
func (s *ProjectService) GetStats(containerID string) (*ContainerStats, error) {
	return s.dockerService.GetContainerStats(containerID)
}

// GetAllStats returns resource usage for all containers
func (s *ProjectService) GetAllStats() (map[string]ContainerStats, error) {
	return s.dockerService.GetAllContainerStats()
}

// ExecArtisan executes an artisan command in the container
func (s *ProjectService) ExecArtisan(containerID string, command string) (string, error) {
	return s.dockerService.ExecLaravelCommand(containerID, command)
}

// GetEnv reads the .env file from the project storage
func (s *ProjectService) GetEnv(subdomain string) (string, error) {
	return s.dockerService.GetEnvFile(subdomain)
}

// SaveEnv saves the .env file to the project storage
func (s *ProjectService) SaveEnv(subdomain string, content string) error {
	return s.dockerService.SaveEnvFile(subdomain, content)
}

// StopContainer stops a container
func (s *ProjectService) StopContainer(containerID string) error {
	return s.dockerService.StopContainer(containerID)
}

// CacheSubdomainMapping syncs project lookup data to Redis
func (s *ProjectService) CacheSubdomainMapping(project *models.Project) error {
	if project.Port == nil {
		return nil
	}
	key := fmt.Sprintf("proxy:subdomain:%s", project.Subdomain)
	// Cache for 1 hour, auto-refresh on access
	return s.redisService.SetCache(key, project, 1*time.Hour)
}

// InvalidateSubdomainCache removes project mapping from Redis
func (s *ProjectService) InvalidateSubdomainCache(subdomain string) error {
	key := fmt.Sprintf("proxy:subdomain:%s", subdomain)
	return s.redisService.DeleteCache(key)
}
