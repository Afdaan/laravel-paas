package domain

import (
	"strings"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
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
		ProjectID: projectID,
		Domain:    domainName,
		Status:    models.DomainStatusPending,
	}

	if err := s.db.Create(domain).Error; err != nil {
		return nil, err
	}

	return domain, nil
}

// RemoveDomain removes a custom domain
func (s *DomainService) RemoveDomain(domainID uint, projectID uint) error {
	result := s.db.Where("id = ? AND project_id = ?", domainID, projectID).Delete(&models.CustomDomain{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperr.New(404, "NOT_FOUND", "Domain not found")
	}
	return nil
}

// ListDomains returns all domains for a project
func (s *DomainService) ListDomains(projectID uint) ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	if err := s.db.Where("project_id = ?", projectID).Find(&domains).Error; err != nil {
		return nil, err
	}
	return domains, nil
}

// ListUserDomains returns all domains across all projects of a user
func (s *DomainService) ListUserDomains(userID uint) ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	subQuery := s.db.Model(&models.Project{}).Select("id").Where("user_id = ?", userID)
	
	err := s.db.Where("project_id IN (?)", subQuery).
		Order("created_at DESC").
		Preload("Project").
		Find(&domains).Error
		
	return domains, err
}

// ListAllDomains returns all domains in the system (Admin)
func (s *DomainService) ListAllDomains() ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	err := s.db.Preload("Project.User").Order("created_at DESC").Find(&domains).Error
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
	if err := s.db.Save(&domain).Error; err != nil {
		return err
	}

	sourceProject, err := s.projectRepo.GetByID(sourceProjectID)
	if err == nil {
		s.projectService.SyncProjectNginx(sourceProject)
	}
	s.projectService.SyncProjectNginx(targetProject)

	return nil
}
