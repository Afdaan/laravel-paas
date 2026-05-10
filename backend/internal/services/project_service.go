// ===========================================
// Project Service
// ===========================================
// Centralized business logic for project management
// ===========================================
package services

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
	"github.com/laravel-paas/backend/internal/repositories"
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

func validateBaseDirectory(baseDirectory string) error {
	bd := strings.TrimSpace(baseDirectory)
	if bd == "" {
		return nil
	}
	clean := filepath.Clean(bd)
	if filepath.IsAbs(clean) || clean == "." || clean == string(filepath.Separator) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid base_directory")
	}
	// Avoid Windows-style separators sneaking in.
	if strings.ContainsRune(bd, '\\') {
		return fmt.Errorf("invalid base_directory")
	}
	return nil
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

// GetProjectByUID fetches a project by its secure UID
func (s *ProjectService) GetProjectByUID(uid string) (*models.Project, error) {
	return s.projectRepo.GetByUID(uid)
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
	if err := s.redisService.SetCache(cacheKey, p, 1*time.Hour); err != nil {
		slog.Warn("Failed to cache project in Redis", "subdomain", subdomain, "error", err)
	}

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
	if err := s.nginxService.DeleteProject(project, projectDomain); err != nil {
		slog.Warn("Failed to delete project from Nginx proxy", "subdomain", project.Subdomain, "error", err)
	}

	// Invalidate Redis Proxy Cache
	if err := s.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate subdomain cache", "subdomain", project.Subdomain, "error", err)
	}

	// 2. Remove Docker Container & Image
	if project.ContainerID != nil {
		slog.Debug("Removing containers", "mainID", *project.ContainerID, "workerID", project.WorkerContainerID)
		if err := s.dockerService.RemoveContainer(*project.ContainerID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to remove container", "id", *project.ContainerID, "error", err)
		}
	}
	if err := s.dockerService.RemoveImage(project.Subdomain); err != nil {
		slog.Warn("Failed to remove docker image", "subdomain", project.Subdomain, "error", err)
	}

	// 3. Drop Student Database
	if project.DatabaseName != "" {
		slog.Debug("Dropping database", "db", project.DatabaseName)
		if err := s.mysqlService.DropDatabase(project.DatabaseName); err != nil {
			slog.Warn("Failed to drop database, might be already gone", "db", project.DatabaseName, "error", err)
		}
	}

	// 4. Cleanup Filesystem (Source Code & Persistent Data)
	if err := s.dockerService.CleanupProject(project.Subdomain); err != nil {
		slog.Warn("Failed to cleanup project filesystem", "subdomain", project.Subdomain, "error", err)
	}
	s.storageService.CleanupPersistentData(project)

	// 5. Hard Delete from Database
	if err := s.projectRepo.Delete(project.ID); err != nil {
		slog.Error("CRITICAL: Failed to delete project record", "id", project.ID, "error", err)
		return err
	}

	// 7. Cleanup dangling images after deletion
	go func() {
		if err := s.dockerService.PruneImages(); err != nil {
			slog.Error("Failed to prune images after project deletion", "error", err)
		}
	}()

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
func (s *ProjectService) CreateProject(userID uint, name, githubURL, branch, databaseName, baseDirectory, buildCommand, startCommand string, queueEnabled bool) (*models.Project, error) {
	// Enforce per-user project limit
	maxProjects, _ := strconv.Atoi(s.GetSetting(models.SettingMaxProjects, models.DefaultMaxProjects))
	count, _ := s.projectRepo.CountByUserID(userID)

	if int(count) >= maxProjects {
		return nil, apperr.New(403, "LIMIT_REACHED", fmt.Sprintf("You have reached the maximum allowed number of projects (%d)", maxProjects))
	}


	if err := validateBaseDirectory(baseDirectory); err != nil {
		return nil, apperr.New(400, "INVALID_BASE_DIRECTORY", err.Error())
	}

	// 5. Generate unique subdomain using the refactored string utility
	subdomain := utils.GenerateSubdomain(name)

	dbName := databaseName
	if dbName == "" {
		dbName = strings.ReplaceAll(subdomain, "-", "_")
	}

	// Always generate a random password if not provided to ensure successful MySQL user creation
	dbPassword := utils.GeneratePassword(16)

	expiryDays, _ := strconv.Atoi(s.GetSetting(models.SettingProjectExpiry, models.DefaultProjectExpiry))
	var expiresAt *time.Time
	if expiryDays > 0 {
		t := time.Now().AddDate(0, 0, expiryDays)
		expiresAt = &t
	}

	project := &models.Project{
		UserID:           userID,
		Name:             name,
		GithubURL:        githubURL,
		Branch:           branch,
		Subdomain:        subdomain,
		DatabaseName:     dbName,
		DatabasePassword: dbPassword,
		BaseDirectory:    baseDirectory,
		BuildCommand:     buildCommand,
		StartCommand:     startCommand,
		QueueEnabled:     queueEnabled,
		Status:           models.StatusQueued,
		ExpiresAt:        expiresAt,
		UID:              utils.GenerateRandomUID(),
	}

	if err := s.projectRepo.Create(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *ProjectService) UpdateProject(id uint, userID uint, role models.Role, name, branch, phpVersion, baseDirectory string, queueEnabled bool, workerCommand, buildCommand, startCommand, nodeVersion, languageVersion string) (*models.Project, error) {
	project, err := s.projectRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Permission check (Only owner or admin can update)
	if project.UserID != userID && role != models.RoleSuperAdmin && role != models.RoleAdmin {
		return nil, apperr.New(403, "FORBIDDEN", "You do not have permission to update this project")
	}

	if name != "" {
		project.Name = name
	}
	project.Branch = branch
	project.PHPVersion = phpVersion
	project.BaseDirectory = baseDirectory
	project.QueueEnabled = queueEnabled
	project.WorkerCommand = workerCommand
	project.BuildCommand = buildCommand
	project.StartCommand = startCommand
	project.NodeVersion = nodeVersion
	project.LanguageVersion = languageVersion

	if err := s.projectRepo.Update(project); err != nil {
		return nil, err
	}

	// Invalidate Metadata Cache
	if err := s.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate subdomain cache after update", "subdomain", project.Subdomain, "error", err)
	}

	return project, nil
}

// UpdateProjectStatus updates the status of a project and clears cache
func (s *ProjectService) UpdateProjectStatus(id uint, status models.ProjectStatus) error {
	project, err := s.projectRepo.GetByID(id)
	if err == nil {
		if err := s.InvalidateSubdomainCache(project.Subdomain); err != nil {
			slog.Warn("Failed to invalidate subdomain cache after status update", "subdomain", project.Subdomain, "error", err)
		}
	}
	return s.projectRepo.UpdateStatus(id, status)
}

// GetProjectsByStatus returns projects matching a specific status
func (s *ProjectService) GetProjectsByStatus(status models.ProjectStatus) ([]models.Project, error) {
	projects, err := s.projectRepo.ListByStatus(status)
	if err != nil {
		return nil, err
	}
	s.PopulateURLs(projects)
	return projects, nil
}

// GetProjectsByStatuses returns projects matching any of the specific statuses
func (s *ProjectService) GetProjectsByStatuses(statuses []models.ProjectStatus) ([]models.Project, error) {
	projects, err := s.projectRepo.ListByStatuses(statuses)
	if err != nil {
		return nil, err
	}
	s.PopulateURLs(projects)
	return projects, nil
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

// PopulateURL sets the URL and UID fields on a project model
func (s *ProjectService) PopulateURL(project *models.Project) {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	project.URL = "https://" + project.GetFullDomain(projectDomain)
}

// PopulateURLs sets the URL and UID fields on a slice of project models
func (s *ProjectService) PopulateURLs(projects []models.Project) {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	for i := range projects {
		projects[i].URL = "https://" + projects[i].GetFullDomain(projectDomain)
	}
}

// GetLogs returns container logs (either web or worker)
func (s *ProjectService) GetLogs(project *models.Project, logType string, lines int) (string, error) {
	containerID := project.ContainerID
	if logType == "worker" {
		containerID = project.WorkerContainerID
	}

	if containerID == nil || *containerID == "" {
		return "", fmt.Errorf("no container running for type %s", logType)
	}

	return s.dockerService.GetLogs(*containerID, lines)
}

// GetStats returns container resource usage
func (s *ProjectService) GetStats(containerID string) (*infrastructure.ContainerStats, error) {
	return s.dockerService.GetContainerStats(containerID)
}

// GetAllStats returns resource usage for all containers
func (s *ProjectService) GetAllStats() (map[string]infrastructure.ContainerStats, error) {
	return s.dockerService.GetAllContainerStats()
}

// ExecCommand executes a command in the container (automatically handles artisan for Laravel)
func (s *ProjectService) ExecCommand(project *models.Project, command string) (string, error) {
	return s.dockerService.ExecProjectCommand(project, command)
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

// StopProject stops both web and worker containers and updates status
func (s *ProjectService) StopProject(project *models.Project) error {
	if project.ContainerID != nil {
		if err := s.dockerService.StopContainer(*project.ContainerID); err != nil {
			slog.Warn("Failed to stop web container", "id", *project.ContainerID, "error", err)
		}
	}
	if project.WorkerContainerID != nil {
		if err := s.dockerService.StopContainer(*project.WorkerContainerID); err != nil {
			slog.Warn("Failed to stop worker container", "id", *project.WorkerContainerID, "error", err)
		}
	}

	project.Status = models.StatusStopped
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
}

// StartProject starts both web and worker containers and updates status
func (s *ProjectService) StartProject(project *models.Project) error {
	if project.ContainerID != nil {
		if err := s.dockerService.StartContainer(*project.ContainerID); err != nil {
			return fmt.Errorf("failed to start web container: %w", err)
		}
	}
	if project.WorkerContainerID != nil {
		if err := s.dockerService.StartContainer(*project.WorkerContainerID); err != nil {
			slog.Warn("Failed to start worker container", "id", *project.WorkerContainerID, "error", err)
		}
	}

	project.Status = models.StatusRunning
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
}

// RestartProject restarts both web and worker containers
func (s *ProjectService) RestartProject(project *models.Project) error {
	// Set status to restarting
	project.Status = models.StatusRestarting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		slog.Error("Failed to update status to restarting", "id", project.ID, "error", err)
	}

	if project.ContainerID != nil {
		if err := s.dockerService.RestartContainer(*project.ContainerID); err != nil {
			return fmt.Errorf("failed to restart web container: %w", err)
		}
	}
	if project.WorkerContainerID != nil {
		if err := s.dockerService.RestartContainer(*project.WorkerContainerID); err != nil {
			slog.Warn("Failed to restart worker container", "id", *project.WorkerContainerID, "error", err)
		}
	}

	project.Status = models.StatusRunning
	return s.projectRepo.UpdateStatus(project.ID, project.Status)
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
