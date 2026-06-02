package project

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/infrastructure/nginx"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/pkg/utils"
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
	slog.Info("Enqueueing project deletion job",
		"id", project.ID,
		"name", project.Name,
		"subdomain", project.Subdomain)

	project.Status = models.StatusDeleting
	if err := s.projectRepo.UpdateStatus(project.ID, project.Status); err != nil {
		return err
	}

	_, err := s.redisService.EnqueueDeployment(project.ID, project.UserID, "delete")
	return err
}

// SyncProjectNginx triggers a sync to the remote Nginx proxy and stores the resulting config hash.
func (s *ProjectService) SyncProjectNginx(project *models.Project) (string, error) {
	return s.SyncProjectNginxFrom(project, "unspecified")
}

// SyncProjectNginxFrom renders edge config from a fresh database snapshot, never caller-held partial state.
func (s *ProjectService) SyncProjectNginxFrom(project *models.Project, triggerSource string) (string, error) {
	start := time.Now()
	defer func() { metrics.GetCollector().ObserveNginxReloadDuration(time.Since(start)) }()

	if project == nil || project.ID == 0 {
		return "", fmt.Errorf("cannot sync nginx for empty project")
	}

	freshProject, err := s.projectRepo.GetByIDForNginx(project.ID)
	if err != nil {
		metrics.GetCollector().IncrNginxReloadFailedTotal()
		return "", fmt.Errorf("failed to load authoritative nginx project state: %w", err)
	}

	loadedDomains := make([]string, 0, len(freshProject.CustomDomains))
	verifiedDomains := make([]string, 0, len(freshProject.CustomDomains))
	for _, cd := range freshProject.CustomDomains {
		loadedDomains = append(loadedDomains, fmt.Sprintf("%s:%s", cd.Domain, cd.Status))
		if cd.Domain != "" && models.IsNginxRoutableCustomDomainStatus(cd.Status) {
			verifiedDomains = append(verifiedDomains, cd.Domain)
		}
	}

	slog.Info("Loaded authoritative Nginx reconciliation state",
		"triggerSource", triggerSource,
		"projectID", freshProject.ID,
		"subdomain", freshProject.Subdomain,
		"activeContainerID", freshProject.ContainerID,
		"rolloutContainerID", freshProject.RolloutContainerID,
		"port", freshProject.Port,
		"loadedCustomDomains", loadedDomains,
		"verifiedCustomDomains", verifiedDomains)

	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)
	serverNames := append([]string{freshProject.GetFullDomain(projectDomain)}, verifiedDomains...)
	if len(freshProject.CustomDomains) > 0 && len(verifiedDomains) == 0 {
		metrics.GetCollector().IncrNginxReloadFailedTotal()
		return "", fmt.Errorf("refusing to render nginx config without verified custom domains for project %s", freshProject.Subdomain)
	}

	slog.Info("Rendering Nginx server names",
		"triggerSource", triggerSource,
		"projectID", freshProject.ID,
		"subdomain", freshProject.Subdomain,
		"serverNames", serverNames)

	// Do not debounce here: fresh DB state must always be allowed to repair stale edge config.
	hash, err := s.nginxService.SyncProject(freshProject, projectDomain)
	if err != nil {
		metrics.GetCollector().IncrNginxReloadFailedTotal()
		return "", err
	}
	metrics.GetCollector().IncrNginxReloadTotal()

	if hash != "" && hash == freshProject.ConfigHash {
		metrics.GetCollector().IncrNginxReloadSkippedTotal()
	} else if hash != "" && hash != freshProject.ConfigHash {
		oldHash := freshProject.ConfigHash
		if err := s.projectRepo.UpdateConfigHash(freshProject.ID, hash, oldHash); err != nil {
			slog.Warn("Concurrent config hash update detected", "subdomain", freshProject.Subdomain, "triggerSource", triggerSource, "error", err)
			if latest, err := s.projectRepo.GetByID(freshProject.ID); err == nil {
				project.ConfigHash = latest.ConfigHash
			}
			return hash, nil
		}
		slog.Info("Project Nginx config hash updated", "subdomain", freshProject.Subdomain, "triggerSource", triggerSource, "old", oldHash, "new", hash)
		project.ConfigHash = hash
	}
	return hash, nil
}

// GetSSLStatus queries the remote Nginx VM for SSL certificate status
func (s *ProjectService) GetSSLStatus(domain string) (*nginx.SSLStatusResponse, error) {
	return s.nginxService.GetSSLStatus(domain)
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
func (s *ProjectService) CreateProject(userID uint, role models.Role, name, githubURL, branch, databaseName, baseDirectory, buildCommand, startCommand string, queueEnabled bool, enableDatabase bool, databaseEngine string, githubInstallationID *int64, githubRepoOwner, githubRepoName string) (*models.Project, error) {
	// Enforce per-user project limit (bypass for admins and superadmins)
	if role != models.RoleAdmin && role != models.RoleSuperAdmin {
		maxProjects, _ := strconv.Atoi(s.GetSetting(models.SettingMaxProjects, models.DefaultMaxProjects))
		count, _ := s.projectRepo.CountByUserID(userID)

		if int(count) >= maxProjects {
			return nil, apperr.New(403, "LIMIT_REACHED", fmt.Sprintf("You have reached the maximum allowed number of projects (%d)", maxProjects))
		}
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
		UserID:               userID,
		Name:                 name,
		GithubURL:            githubURL,
		Branch:               branch,
		Subdomain:            subdomain,
		DatabaseName:         dbName,
		DatabasePassword:     dbPassword,
		BaseDirectory:        baseDirectory,
		BuildCommand:         sanitizeCommand(buildCommand),
		StartCommand:         strings.TrimSpace(startCommand),
		QueueEnabled:         queueEnabled,
		Status:               models.StatusPending,
		DeploymentStatus:     models.DepStatusQueued,
		ExpiresAt:            expiresAt,
		UID:                  utils.GenerateRandomUID(),
		GithubInstallationID: githubInstallationID,
		GithubRepoOwner:      githubRepoOwner,
		GithubRepoName:       githubRepoName,
	}

	if enableDatabase {
		engine := "mysql"
		if databaseEngine == "postgresql" {
			engine = "postgresql"
		}

		host := "paas-mysql"
		port := 3306
		if engine == "postgresql" {
			host = "paas-user-postgres"
			port = 5432
		}

		instance := &models.DatabaseInstance{
			Engine:            engine,
			Status:            models.DBStatusActive,
			Name:              dbName,
			Username:          dbName,
			Password:          dbPassword,
			Host:              host,
			Port:              port,
			StorageAllocation: 1073741824, // 1GB
		}
		project.DatabaseInstance = instance
	}

	if err := s.projectRepo.Create(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *ProjectService) UpdateProject(id uint, userID uint, role models.Role, name, branch, phpVersion, baseDirectory string, queueEnabled bool, workerCommand, buildCommand, startCommand, nodeVersion, languageVersion string, githubURL string, githubInstallationID *int64, githubRepoOwner, githubRepoName string) (*models.Project, error) {
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

	// Update Git connection fields if provided (or allow resetting/changing)
	project.GithubURL = githubURL
	project.GithubInstallationID = githubInstallationID
	project.GithubRepoOwner = githubRepoOwner
	project.GithubRepoName = githubRepoName

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

// TransitionDeploymentState delegates status updates to the centralized atomic transition manager
func (s *ProjectService) TransitionDeploymentState(ctx context.Context, projectID uint, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, error) {
	if s.transitionManager == nil {
		return nil, fmt.Errorf("transition manager not initialized")
	}
	project, err := s.transitionManager.TransitionState(ctx, projectID, jobID, nextState, progress, eventType, payload)
	if err == nil && project != nil {
		if cacheErr := s.InvalidateSubdomainCache(project.Subdomain); cacheErr != nil {
			slog.Warn("Failed to invalidate subdomain cache after atomic deployment state transition", "subdomain", project.Subdomain, "error", cacheErr)
		}
	}
	return project, err
}

// UpdateDeploymentStatus updates the deployment execution status of a project and clears cache
func (s *ProjectService) UpdateDeploymentStatus(id uint, status models.DeploymentStatus, message string, progress int, jobID string) error {
	_, err := s.TransitionDeploymentState(context.Background(), id, jobID, status, progress, "system_update", message)
	return err
}

// PromoteRolloutContainer promotes the in-flight container to active and clears cache
func (s *ProjectService) PromoteRolloutContainer(id uint, newContainerID string) error {
	project, err := s.projectRepo.GetByID(id)
	if err == nil {
		if err := s.InvalidateSubdomainCache(project.Subdomain); err != nil {
			slog.Warn("Failed to invalidate subdomain cache after promotion", "subdomain", project.Subdomain, "error", err)
		}
	}
	return s.projectRepo.PromoteRolloutContainer(id, newContainerID)
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

// GetDeploymentEvents returns the timeline of deployment events for a project
func (s *ProjectService) GetDeploymentEvents(projectID uint) ([]models.DeploymentEvent, error) {
	return s.projectRepo.ListDeploymentEventsByProjectID(projectID)
}

// GetAllDeploymentEvents returns the complete unfiltered timeline of deployment events for a project
func (s *ProjectService) GetAllDeploymentEvents(projectID uint) ([]models.DeploymentEvent, error) {
	return s.projectRepo.ListAllDeploymentEventsByProjectID(projectID)
}
