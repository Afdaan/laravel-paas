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
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

type ProjectService struct {
	cfg            *config.Config
	projectRepo    repositories.ProjectRepository
	settingService *SettingService
	dockerService  *infrastructure.DockerService
	storageService *infrastructure.StorageService
	mysqlService   *infrastructure.MySQLService
	nginxService   *infrastructure.NginxWebhookService
	redisService   *infrastructure.RedisService
}

func NewProjectService(
	cfg *config.Config, 
	projectRepo repositories.ProjectRepository,
	settingService *SettingService,
	dockerService *infrastructure.DockerService,
	storageService *infrastructure.StorageService,
	mysqlService *infrastructure.MySQLService,
	redisService *infrastructure.RedisService,
) *ProjectService {
	return &ProjectService{
		cfg:            cfg,
		projectRepo:    projectRepo,
		settingService: settingService,
		dockerService:  dockerService,
		storageService: storageService,
		mysqlService:   mysqlService,
		nginxService:   infrastructure.NewNginxWebhookService(cfg),
		redisService:   redisService,
	}
}

// GetProjectByID fetches a project with preloaded associations
func (s *ProjectService) GetProjectByID(id uint) (*models.Project, error) {
	return s.projectRepo.GetByID(id)
}

// GetBySubdomain fetches a project by its subdomain with Redis caching
func (s *ProjectService) GetBySubdomain(subdomain string) (*models.Project, error) {
	// 1. Try to fetch from Redis Cache first
	var project models.Project
	cacheKey := fmt.Sprintf("project:subdomain:%s", subdomain)
	
	if err := s.redisService.GetCache(cacheKey, &project); err == nil {
		slog.Debug("Subdomain cache hit", "subdomain", subdomain)
		return &project, nil
	}

	// 2. Cache miss: fetch from DB
	p, err := s.projectRepo.GetBySubdomain(subdomain)
	if err != nil {
		return nil, err
	}

	// 3. Store in Redis for next time (expires in 1 hour)
	s.redisService.SetCache(cacheKey, p, 1*time.Hour)
	
	return p, nil
}

// DeleteProject performs a thorough cleanup of all project resources
func (s *ProjectService) DeleteProject(project *models.Project) error {
	slog.Info("Executing thorough project deletion", 
		"id", project.ID, 
		"name", project.Name, 
		"subdomain", project.Subdomain)

	// 1. Sync Deletion to Nginx Proxy (Do this first to stop traffic)
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	s.nginxService.DeleteProject(project, projectDomain)

	// Invalidate Redis Proxy Cache
	s.InvalidateSubdomainCache(project.Subdomain)

	// 2. Remove Docker Container & Image
	if project.ContainerID != nil {
		slog.Debug("Removing container", "id", *project.ContainerID)
		s.dockerService.RemoveContainer(*project.ContainerID)
	}
	s.dockerService.RemoveImage(project.Subdomain)

	// 3. Drop Student Database
	if project.DatabaseName != "" {
		slog.Debug("Dropping database", "db", project.DatabaseName)
		if err := s.mysqlService.DropDatabase(project.DatabaseName); err != nil {
			slog.Warn("Failed to drop database, might be already gone", "db", project.DatabaseName, "error", err)
		}
	}

	// 4. Cleanup Filesystem (Source Code & Persistent Data)
	s.dockerService.CleanupProject(project.Subdomain)
	s.storageService.CleanupPersistentData(project)

	// 5. Hard Delete from Database
	if err := s.projectRepo.Delete(project.ID); err != nil {
		slog.Error("CRITICAL: Failed to delete project record", "id", project.ID, "error", err)
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

// ListProjects returns paginated projects with filtering
func (s *ProjectService) ListProjects(page, limit int, userID uint, status string, search string) ([]models.Project, int64, error) {
	projects, total, err := s.projectRepo.List(page, limit, userID, status, search)
	if err != nil {
		return nil, 0, err
	}
	s.PopulateURLs(projects)
	return projects, total, nil
}

// ListByUserID returns all projects for a specific user without pagination
func (s *ProjectService) ListByUserID(userID uint) ([]models.Project, error) {
	return s.projectRepo.ListByUserID(userID)
}

// CreateProject handles the initial creation of a project record
func (s *ProjectService) CreateProject(userID uint, name, githubURL, branch, databaseName string) (*models.Project, error) {
	// Enforce per-user project limit
	maxProjects, _ := strconv.Atoi(s.GetSetting(models.SettingMaxProjects, models.DefaultMaxProjects))
	count, _ := s.projectRepo.CountByUserID(userID)

	if int(count) >= maxProjects {
		return nil, apperr.New(403, "LIMIT_REACHED", fmt.Sprintf("You have reached the maximum allowed number of projects (%d)", maxProjects))
	}

	// 5. Generate unique subdomain using the refactored string utility
	subdomain := utils.GenerateSubdomain(name)

	dbName := databaseName
	if dbName == "" {
		dbName = strings.ReplaceAll(subdomain, "-", "_")
	}

	expiryDays, _ := strconv.Atoi(s.GetSetting(models.SettingProjectExpiry, models.DefaultProjectExpiry))
	var expiresAt *time.Time
	if expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	project := &models.Project{
		UserID:       userID,
		Name:         name,
		GithubURL:    githubURL,
		Branch:       branch,
		Subdomain:    subdomain,
		DatabaseName: dbName,
		Status:       models.StatusQueued,
		ExpiresAt:    expiresAt,
	}

	if err := s.projectRepo.Create(project); err != nil {
		return nil, err
	}

	return project, nil
}

// UpdateProject updates project details
func (s *ProjectService) UpdateProject(id uint, userID uint, role models.Role, name, branch string, queueEnabled bool) (*models.Project, error) {
	project, err := s.projectRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, apperr.ErrForbidden
	}

	if name != "" {
		project.Name = name
	}
	if branch != "" {
		project.Branch = branch
	}
	project.QueueEnabled = queueEnabled

	if err := s.projectRepo.Update(project); err != nil {
		return nil, err
	}

	// Invalidate Metadata Cache
	s.InvalidateSubdomainCache(project.Subdomain)

	return project, nil
}

// UpdateProjectStatus updates the status of a project and clears cache
func (s *ProjectService) UpdateProjectStatus(id uint, status models.ProjectStatus) error {
	project, err := s.projectRepo.GetByID(id)
	if err == nil {
		s.InvalidateSubdomainCache(project.Subdomain)
	}
	return s.projectRepo.UpdateStatus(id, status)
}


// GetTotalCount returns total number of projects
func (s *ProjectService) GetTotalCount() (int64, error) {
	return s.projectRepo.CountTotal()
}

// GetRunningCount returns number of running projects
func (s *ProjectService) GetRunningCount() (int64, error) {
	return s.projectRepo.CountRunning()
}

// GetRunningProjectsWithContainers returns projects that have containers
func (s *ProjectService) GetRunningProjectsWithContainers() ([]models.Project, error) {
	return s.projectRepo.GetRunningWithContainers()
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
func (s *ProjectService) GetStats(containerID string) (*infrastructure.ContainerStats, error) {
	return s.dockerService.GetContainerStats(containerID)
}

// GetAllStats returns resource usage for all containers
func (s *ProjectService) GetAllStats() (map[string]infrastructure.ContainerStats, error) {
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
