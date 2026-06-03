package project

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/nginx"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/services/deployment"
	"github.com/laravel-paas/shared/services/setting"
	"github.com/laravel-paas/shared/pkg/traefik"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
)

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

func (s *ProjectService) GetSetting(key, defaultValue string) string {
	return s.settingService.Get(key, defaultValue)
}

func (s *ProjectService) GetConfig() *config.Config {
	return s.cfg
}

func (s *ProjectService) CacheSubdomainMapping(project *models.Project) error {
	if project.Subdomain == "" || project.ContainerID == nil {
		return nil
	}
	key := fmt.Sprintf("proxy:subdomain:%s", project.Subdomain)
	return s.redisService.SetCache(key, project, 1*time.Hour)
}

func (s *ProjectService) InvalidateSubdomainCache(subdomain string) error {
	key := fmt.Sprintf("proxy:subdomain:%s", subdomain)
	return s.redisService.DeleteCache(key)
}

func (s *ProjectService) PromoteRolloutContainer(id uint, newContainerID string) error {
	return s.projectRepo.PromoteRolloutContainer(id, newContainerID)
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

	// Clean up Traefik dynamic routing file
	if err := traefik.DeleteProjectDynamicFile(s.cfg, project.UserID, project.ID, project.Subdomain); err != nil {
		slog.Warn("Failed to delete project Traefik config", "id", project.ID, "error", err)
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

	// Drop User Database
	if project.DatabaseName != "" {
		slog.Debug("Dropping database", "db", project.DatabaseName)
		if err := s.mysqlService.DropDatabase(project.DatabaseName); err != nil {
			slog.Warn("Failed to drop database, might be already gone", "db", project.DatabaseName, "error", err)
		}
	}

	// Cleanup Filesystem (Source Code & Persistent Data)
	if err := s.dockerService.CleanupProject(project.Subdomain); err != nil {
		slog.Warn("Failed to cleanup project filesystem", "subdomain", project.Subdomain, "error", err)
	}
	s.storageService.CleanupPersistentData(project)

	// Hard Delete from Database
	if err := s.projectRepo.Delete(project.ID); err != nil {
		slog.Error("CRITICAL: Failed to delete project record", "id", project.ID, "error", err)
		return err
	}

	// Cleanup dangling images after deletion
	go func() {
		if err := s.dockerService.PruneImages(); err != nil {
			slog.Error("Failed to prune images after project deletion", "error", err)
		}
	}()

	slog.Info("Project thoroughly purged from system", "name", project.Name)
	return nil
}

func (s *ProjectService) SyncProjectNginx(project *models.Project) (string, error) {
	return s.SyncProjectNginxFrom(project, "unspecified")
}

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

func (s *ProjectService) RecreateProjectZeroDowntime(project *models.Project, logFunc func(string)) error {
	if logFunc == nil {
		logFunc = func(string) {}
	}
	projectDomain := s.GetSetting(models.SettingProjectDomain, s.cfg.ProjectDomain)

	if project.ContainerID == nil || *project.ContainerID == "" {
		return nil
	}

	logFunc(">> Initiating application hot-swap...")
	slog.Info("Executing zero-downtime container recreation with health guard",
		"subdomain", project.Subdomain,
		"projectId", project.ID)

	oldWebID := *project.ContainerID
	oldWorkerID := project.WorkerContainerID

	logFunc(">> Starting new application instance...")
	newID, err := s.dockerService.StartExistingImage(project, projectDomain)
	if err != nil {
		logFunc("✗ Failed to start new application instance: " + err.Error())
		slog.Error("Failed to start new container during recreation", "subdomain", project.Subdomain, "error", err)
		return err
	}
	logFunc("✓ New application instance started.")

	_ = s.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
		"rollout_container_id": newID,
	})

	logFunc(">> Running application health checks...")
	// Run Advanced 2-step Healthcheck with timeout context
	hcCtx, hcCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer hcCancel()

	if err := s.dockerService.AdvancedHealthcheck(hcCtx, project, newID); err != nil {
		logFunc("✗ Health checks failed. Rolling back configuration changes...")
		slog.Error("New container failed advanced healthcheck, rolling back", "subdomain", project.Subdomain, "newID", newID, "error", err)

		logFunc(">> Rolling back release...")
		if err := s.dockerService.RemoveContainer(newID, project.WorkerContainerID); err != nil {
			slog.Warn("Failed to cleanup unhealthy new container", "id", newID, "error", err)
		}
		_ = s.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
			"rollout_container_id": nil,
		})

		return fmt.Errorf("recreation failed: %w", err)
	}
	logFunc("✓ Health checks passed successfully.")

	logFunc(">> Swapping application routing...")
	if err := s.PromoteRolloutContainer(project.ID, newID); err != nil {
		logFunc("✗ Failed to route traffic to the new instance: " + err.Error())
		slog.Error("Failed to promote rollout container during recreation", "id", project.ID, "error", err)
	} else {
		logFunc("✓ Application routing updated successfully.")
	}
	project.ContainerID = &newID
	project.RolloutContainerID = nil

	time.Sleep(2 * time.Second)

	logFunc(">> Cleaning up legacy resources...")
	slog.Info("Cleaning up legacy containers",
		"subdomain", project.Subdomain,
		"oldWebID", oldWebID)

	if err := s.dockerService.RemoveContainer(oldWebID, oldWorkerID); err != nil {
		logFunc("✗ Failed to prune legacy resources: " + err.Error())
		slog.Warn("Failed to remove old containers after successful swap", "error", err)
	} else {
		logFunc("✓ Legacy resources cleaned up.")
	}

	s.dockerService.CleanupLegacyContainers(project.Subdomain, newID, project.WorkerContainerID)
	logFunc("✓ Hot-swap completed.")

	return nil
}

func (s *ProjectService) TransitionDeploymentState(ctx context.Context, projectID uint, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, error) {
	return s.transitionManager.TransitionState(ctx, projectID, jobID, nextState, progress, eventType, payload)
}

func (s *ProjectService) UpdateDeploymentStatus(id uint, status models.DeploymentStatus, message string, progress int, jobID string) error {
	return s.projectRepo.UpdateDeploymentStatus(id, status, message, progress, jobID)
}

func (s *ProjectService) GetProjectByID(id uint) (*models.Project, error) {
	return s.projectRepo.GetByID(id)
}

func (s *ProjectService) GetSSLStatus(domain string) (*nginx.SSLStatusResponse, error) {
	return s.nginxService.GetSSLStatus(domain)
}


