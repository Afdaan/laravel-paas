package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/metrics"
)

// AddDomain adds a new custom domain for a project
func (s *DomainService) AddDomain(projectID uint, domainName string) (*models.CustomDomain, error) {
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" || strings.Contains(domainName, "/") || strings.Contains(domainName, " ") {
		return nil, apperr.New(400, "INVALID_DOMAIN", "Invalid domain format")
	}

	var count int64
	s.db.Model(&models.CustomDomain{}).Where("domain = ?", domainName).Count(&count)
	if count > 0 {
		return nil, apperr.New(409, "DOMAIN_EXISTS", "Domain is already registered to another project")
	}

	domain := &models.CustomDomain{
		ProjectID:    projectID,
		Domain:       domainName,
		Status:       models.DomainStatusPending,
		HealthStatus: models.DomainHealthUnknown,
	}

	if err := s.db.Create(domain).Error; err != nil {
		return nil, err
	}

	_ = s.RecordEvent(domain, models.DomainStatusPending, models.DomainStatusPending, "domain_registered", fmt.Sprintf("Custom domain %s registered to project %d", domainName, projectID), "")

	return domain, nil
}

// RemoveDomain safely removes a custom domain with distributed locking and Nginx config cleanup.
func (s *DomainService) RemoveDomain(domainID uint, projectID uint) error {
	var domain models.CustomDomain
	if err := s.db.Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		return apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	token, err := s.redisService.AcquireDomainLock(domain.ID, 30*time.Second)
	if err != nil || token == "" {
		metrics.GetCollector().IncrLockContention()
		return apperr.New(423, "LOCKED", "Domain is currently locked by an active provisioning operation. Please try again in a few moments.")
	}
	defer s.redisService.ReleaseDomainLock(domain.ID, token)

	_ = s.db.Model(&domain).Update("cleanup_checkpoint", "init")
	_ = s.TransitionState(&domain, models.DomainStatusPendingCleanup, models.ErrNone, "Domain scheduled for deletion")

	project, err := s.projectRepo.GetByID(projectID)
	if err == nil {
		_, _ = s.projectService.SyncProjectNginx(project)
	}

	return nil
}

// ListDomains returns all active and provisioning domains for a project
func (s *DomainService) ListDomains(projectID uint) ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	if err := s.db.Where("project_id = ? AND status NOT IN (?)", projectID, []string{string(models.DomainStatusPendingCleanup), string(models.DomainStatusDisabled)}).Find(&domains).Error; err != nil {
		return nil, err
	}
	return domains, nil
}

// ListUserDomains returns all domains across all projects of a user
func (s *DomainService) ListUserDomains(userID uint) ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	subQuery := s.db.Model(&models.Project{}).Select("id").Where("user_id = ?", userID)
	
	err := s.db.Where("project_id IN (?) AND status NOT IN (?)", subQuery, []string{string(models.DomainStatusPendingCleanup), string(models.DomainStatusDisabled)}).
		Order("created_at DESC").
		Preload("Project").
		Find(&domains).Error
		
	return domains, err
}

// ListAllDomains returns all domains in the system (Admin)
func (s *DomainService) ListAllDomains() ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	err := s.db.Where("status NOT IN (?)", []string{string(models.DomainStatusPendingCleanup), string(models.DomainStatusDisabled)}).Preload("Project.User").Order("created_at DESC").Find(&domains).Error
	return domains, err
}

// TransferDomain moves a domain from one project to another (same user only)
func (s *DomainService) TransferDomain(userID uint, domainID uint, targetProjectID uint) error {
	var domain models.CustomDomain
	if err := s.db.Preload("Project").First(&domain, domainID).Error; err != nil {
		return apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	if domain.Project.UserID != userID {
		return apperr.New(403, "FORBIDDEN", "You do not own this domain")
	}

	targetProject, err := s.projectRepo.GetByID(targetProjectID)
	if err != nil {
		return apperr.New(404, "TARGET_NOT_FOUND", "Target project not found")
	}
	if targetProject.UserID != userID {
		return apperr.New(403, "FORBIDDEN", "You do not own the target project")
	}

	sourceProjectID := domain.ProjectID
	domain.ProjectID = targetProjectID
	if err := s.db.Model(&domain).Update("project_id", targetProjectID).Error; err != nil {
		return err
	}

	_ = s.RecordEvent(&domain, domain.Status, domain.Status, "domain_transferred", fmt.Sprintf("Domain transferred from project %d to project %d", sourceProjectID, targetProjectID), "")

	sourceProject, err := s.projectRepo.GetByID(sourceProjectID)
	if err == nil {
		_, _ = s.projectService.SyncProjectNginx(sourceProject)
	}
	_, _ = s.projectService.SyncProjectNginx(targetProject)

	return nil
}

// ListEvents returns all lifecycle transition and audit events for a custom domain.
func (s *DomainService) ListEvents(domainID uint) ([]models.DomainEvent, error) {
	var events []models.DomainEvent
	if err := s.db.Where("domain_id = ?", domainID).Order("created_at DESC").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}
