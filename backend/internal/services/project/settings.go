package project

import (
	"github.com/laravel-paas/backend/internal/models"
)

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
