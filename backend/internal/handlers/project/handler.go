package project

import (
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
)

// ===========================================
// Project Handler
// ===========================================
// Handles project deployment and management
// ===========================================
// ProjectHandler handles project endpoints
type ProjectHandler struct {
	cfg            *config.Config
	redisService   *infrastructure.RedisService
	projectService *projectServicePkg.ProjectService
	userService    *services.UserService
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(cfg *config.Config, redisService *infrastructure.RedisService, projectService *projectServicePkg.ProjectService, userService *services.UserService) *ProjectHandler {
	return &ProjectHandler{
		cfg:            cfg,
		projectService: projectService,
		userService:    userService,
		redisService:   redisService,
	}
}
