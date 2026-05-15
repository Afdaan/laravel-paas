package project

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
)

func sanitizeCommand(cmd string) string {
	if cmd == "" {
		return ""
	}
	lines := strings.Split(cmd, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			// If line already ends with && or ;, don't double it up (basic check)
			if strings.HasSuffix(trimmed, "&&") || strings.HasSuffix(trimmed, ";") {
				cleanLines = append(cleanLines, trimmed)
			} else {
				cleanLines = append(cleanLines, trimmed)
			}
		}
	}
	if len(cleanLines) == 0 {
		return ""
	}
	// Join with && to ensure sequential execution
	return strings.Join(cleanLines, " && ")
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
	var project models.Project
	cacheKey := fmt.Sprintf("project:subdomain:%s", subdomain)

	if err := s.redisService.GetCache(cacheKey, &project); err == nil {
		slog.Debug("Subdomain cache hit", "subdomain", subdomain)
		return &project, nil
	}

	p, err := s.projectRepo.GetBySubdomain(subdomain)
	if err != nil {
		return nil, err
	}

	if err := s.redisService.SetCache(cacheKey, p, 1*time.Hour); err != nil {
		slog.Warn("Failed to cache project in Redis", "subdomain", subdomain, "error", err)
	}

	return p, nil
}

func (s *ProjectService) DeleteProject(project *models.Project) error {
	slog.Info("Executing thorough project deletion",
		"id", project.ID,
		"name", project.Name,
		"subdomain", project.Subdomain)

	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	if err := s.nginxService.DeleteProject(project, projectDomain); err != nil {
		slog.Warn("Failed to delete project from Nginx proxy", "subdomain", project.Subdomain, "error", err)
	}

	if err := s.InvalidateSubdomainCache(project.Subdomain); err != nil {
		slog.Warn("Failed to invalidate subdomain cache", "subdomain", project.Subdomain, "error", err)
	}

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

// SyncProjectNginx triggers a sync to the remote Nginx proxy
func (s *ProjectService) SyncProjectNginx(project *models.Project) error {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	return s.nginxService.SyncProject(project, projectDomain)
}

func (s *ProjectService) RecreateProjectZeroDowntime(project *models.Project) error {
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)

	if project.ContainerID == nil || *project.ContainerID == "" {
		return nil
	}

	slog.Info("Executing zero-downtime container recreation with health guard",
		"subdomain", project.Subdomain,
		"projectId", project.ID)

	oldWebID := *project.ContainerID
	oldWorkerID := project.WorkerContainerID

	newID, err := s.dockerService.StartExistingImage(project, projectDomain)
	if err != nil {
		slog.Error("Failed to start new container during recreation", "subdomain", project.Subdomain, "error", err)
		return err
	}

	isHealthy := false
	maxWait := 30
	for i := 0; i < maxWait; i++ {
		if s.dockerService.IsContainerHealthy(newID) {
			isHealthy = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !isHealthy {
		slog.Error("New container failed health check, rolling back", "subdomain", project.Subdomain, "newID", newID)
		
		if err := s.dockerService.RemoveContainer(newID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to cleanup unhealthy new container", "id", newID, "error", err)
		}

		return fmt.Errorf("recreation failed: new container is unhealthy")
	}

	project.ContainerID = &newID
	if err := s.projectRepo.Update(project); err != nil {
		slog.Warn("Failed to update container metadata", "id", project.ID, "error", err)
	}

	time.Sleep(2 * time.Second)

	slog.Info("Cleaning up legacy containers",
		"subdomain", project.Subdomain,
		"oldWebID", oldWebID)

	if err := s.dockerService.RemoveContainer(oldWebID, oldWorkerID); err != nil {
		slog.Warn("Failed to remove old containers after successful swap", "error", err)
	}

	return nil
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
	// GenerateSubdomain already appends a random 6-character suffix
	subdomain := utils.GenerateSubdomain(name)

	// Extract the random suffix to ensure database name is also unique
	// Subdomain format: "name-suffix"
	parts := strings.Split(subdomain, "-")
	suffix := parts[len(parts)-1]

	dbName := databaseName
	if dbName == "" {
		dbName = strings.ReplaceAll(subdomain, "-", "_")
	} else {
		// Even if user provides a database name, we sanitize it and append the
		// same unique suffix to prevent collisions across users.
		dbName = fmt.Sprintf("%s_%s",
			strings.Trim(strings.ReplaceAll(strings.ToLower(dbName), "-", "_"), "_"),
			suffix)
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
		BuildCommand:     sanitizeCommand(buildCommand),
		StartCommand:     strings.TrimSpace(startCommand),
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
	project.BuildCommand = sanitizeCommand(buildCommand)
	project.StartCommand = strings.TrimSpace(startCommand)
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
