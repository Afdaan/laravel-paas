package project

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/infrastructure/nginx"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	"github.com/laravel-paas/shared/services/setting"
)

// ===========================================
// Project Service
// ===========================================
// Centralized business logic for project management
// ===========================================
type ProjectService struct {
	cfg               *config.Config
	projectRepo       repositories.ProjectRepository
	settingService    *setting.SettingService
	dockerService     *docker.DockerService
	storageService    *infrastructure.StorageService
	mysqlService      *infrastructure.MySQLService
	nginxService      *nginx.NginxWebhookService
	redisService      *infrastructure.RedisService
	transitionManager deployment.TransitionManager
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
	settingService *setting.SettingService,
	dockerService *docker.DockerService,
	storageService *infrastructure.StorageService,
	mysqlService *infrastructure.MySQLService,
	redisService *infrastructure.RedisService,
	transitionManager deployment.TransitionManager,
) *ProjectService {
	return &ProjectService{
		cfg:               cfg,
		projectRepo:       projectRepo,
		settingService:    settingService,
		dockerService:     dockerService,
		storageService:    storageService,
		mysqlService:      mysqlService,
		nginxService:      nginx.NewNginxWebhookService(cfg),
		redisService:      redisService,
		transitionManager: transitionManager,
	}
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

		updates := map[string]interface{}{
			"last_accessed_at": now,
		}
		if expiryDays > 0 {
			updates["expires_at"] = now.AddDate(0, 0, expiryDays)
		} else {
			updates["expires_at"] = nil
		}

		if err := s.projectRepo.UpdateMetadata(projectID, updates); err != nil {
			slog.Error("Failed to update project activity", "id", projectID, "error", err)
		}
	}()
}

// PopulateURL sets the URL and UID fields on a project model
func (s *ProjectService) PopulateURL(project *models.Project) {
	project.URL = "https://" + project.GetFullDomain(s.cfg.ProjectDomain)
}

// PopulateURLs sets the URL and UID fields on a slice of project models
func (s *ProjectService) PopulateURLs(projects []models.Project) {
	for i := range projects {
		projects[i].URL = "https://" + projects[i].GetFullDomain(s.cfg.ProjectDomain)
	}
}

// CacheSubdomainMapping syncs project lookup data to Redis
func (s *ProjectService) CacheSubdomainMapping(project *models.Project) error {
	if project.Subdomain == "" || project.ContainerID == nil {
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
