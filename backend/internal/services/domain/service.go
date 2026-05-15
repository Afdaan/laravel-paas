package domain

import (
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
	"gorm.io/gorm"
)

type projectService interface {
	SyncProjectNginx(project *models.Project) error
}

// DomainService handles custom domain management and verification
type DomainService struct {
	cfg            *config.Config
	db             *gorm.DB
	redisService   *infrastructure.RedisService
	projectService projectService
	projectRepo    repositories.ProjectRepository
}

func NewDomainService(cfg *config.Config, db *gorm.DB, redisService *infrastructure.RedisService, projectService projectService, projectRepo repositories.ProjectRepository) *DomainService {
	return &DomainService{
		cfg:            cfg,
		db:             db,
		redisService:   redisService,
		projectService: projectService,
		projectRepo:    projectRepo,
	}
}
