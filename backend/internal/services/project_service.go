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
	"gorm.io/gorm"
)

type ProjectService struct {
	db            *gorm.DB
	cfg           *config.Config
	dockerService *DockerService
	nginxService  *NginxWebhookService
	redisService  *RedisService
}

func NewProjectService(db *gorm.DB, cfg *config.Config, redisService *RedisService) *ProjectService {
	return &ProjectService{
		db:            db,
		cfg:           cfg,
		dockerService: NewDockerService(cfg),
		nginxService:  NewNginxWebhookService(cfg),
		redisService:  redisService,
	}
}

// GetProjectByID fetches a project with preloaded associations
func (s *ProjectService) GetProjectByID(id uint) (*models.Project, error) {
	var project models.Project
	if err := s.db.Preload("User").First(&project, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("project not found")
		}
		return nil, err
	}
	return &project, nil
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

	// 3. Cleanup Filesystem (Storage/Projects)
	s.dockerService.CleanupProject(project.Subdomain)

	// 4. Drop Student Database
	if project.DatabaseName != "" {
		s.dockerService.DropDatabase(project.DatabaseName)
	}

	// 5. Sync Deletion to Nginx Proxy
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	s.nginxService.DeleteProject(project, projectDomain)

	// 6. Hard Delete from Database
	if err := s.db.Unscoped().Delete(project).Error; err != nil {
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
	var setting models.Setting
	if err := s.db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}

// UpdateActivity updates the last_accessed_at and expires_at fields
func (s *ProjectService) UpdateActivity(projectID uint) {
	go func() {
		now := time.Now()
		expiryDays, _ := strconv.Atoi(s.GetSetting(models.SettingProjectExpiry, models.DefaultProjectExpiry))

		updates := map[string]interface{}{
			"last_accessed_at": &now,
		}

		if expiryDays > 0 {
			expire := now.AddDate(0, 0, expiryDays)
			updates["expires_at"] = &expire
		} else {
			updates["expires_at"] = nil
		}

		if err := s.db.Model(&models.Project{}).Where("id = ?", projectID).Updates(updates).Error; err != nil {
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

