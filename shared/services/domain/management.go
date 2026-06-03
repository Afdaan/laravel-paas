package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
)

// AddDomain adds a new custom domain for a project
func (s *DomainService) AddDomain(projectID uint, domainName string) (*models.CustomDomain, error) {
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" || strings.Contains(domainName, "/") || strings.Contains(domainName, " ") || strings.Contains(domainName, ":") {
		return nil, apperr.New(400, "INVALID_DOMAIN", "Invalid domain format. Domain cannot contain ports or colons.")
	}

	token, err := s.redisService.AcquireProjectDomainLock(projectID, 15*time.Second)
	if err != nil || token == "" {
		return nil, apperr.New(423, "LOCKED", "Another domain operation is in progress on this project. Please try again.")
	}
	defer func() {
		_ = s.redisService.ReleaseProjectDomainLock(projectID, token)
	}()

	// Enforce custom domains limit per project
	var currentDomainsCount int64
	s.db.Model(&models.CustomDomain{}).Where("project_id = ? AND status NOT IN (?)", projectID, []string{string(models.DomainStatusPendingCleanup), string(models.DomainStatusDisabled)}).Count(&currentDomainsCount)

	maxDomains := 3
	var setting models.Setting
	if err := s.db.Where("setting_key = ?", models.SettingMaxDomainsPerProject).First(&setting).Error; err == nil {
		if val, err := strconv.Atoi(setting.Value); err == nil {
			maxDomains = val
		}
	}

	if currentDomainsCount >= int64(maxDomains) {
		return nil, apperr.New(400, "DOMAIN_LIMIT_REACHED", fmt.Sprintf("You have reached the maximum limit of %d domains for this project", maxDomains))
	}

	var count int64
	s.db.Model(&models.CustomDomain{}).Where("domain = ? AND status != ?", domainName, string(models.DomainStatusPendingCleanup)).Count(&count)
	if count > 0 {
		return nil, apperr.New(409, "DNS_CONFLICT", "Domain is already registered to another project")
	}

	// Safely hard-delete any legacy/soft-deleted or pending_cleanup rows with this domain to avoid Postgres unique constraint violations on create
	if err := s.db.Unscoped().Where("domain = ?", domainName).Delete(&models.CustomDomain{}).Error; err != nil {
		return nil, err
	}

	domain := &models.CustomDomain{
		ProjectID:    projectID,
		Domain:       domainName,
		Status:       models.DomainStatusPending,
		HealthStatus: models.DomainHealthUnknown,
	}

	if err := s.db.Create(domain).Error; err != nil {
		// Catch unique key constraint violation error as a fallback safety check
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "uni_custom_domains_domain") {
			return nil, apperr.New(409, "DNS_CONFLICT", "Domain is already registered to another project")
		}
		return nil, err
	}

	var projectName string
	project, err := s.projectRepo.GetByID(projectID)
	if err == nil && project != nil {
		projectName = project.Name
	} else {
		projectName = fmt.Sprintf("%d", projectID)
	}

	_ = s.RecordEvent(domain, models.DomainStatusPending, models.DomainStatusPending, "domain_registered", fmt.Sprintf("Custom domain %s registered to project %s", domainName, projectName), "")

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
	defer func() {
		_ = s.redisService.ReleaseDomainLock(domain.ID, token)
	}()

	_ = s.db.Model(&domain).Update("cleanup_checkpoint", "init")
	_ = s.TransitionState(&domain, models.DomainStatusPendingCleanup, models.ErrNone, "Domain scheduled for deletion")

	project, err := s.projectRepo.GetByID(projectID)
	if err == nil {
		_, _ = s.projectService.SyncProjectNginxFrom(project, "domain_remove")
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
	// 1. Lock the domain first to serialize lifecycle modifications
	domainToken, err := s.redisService.AcquireDomainLock(domainID, 30*time.Second)
	if err != nil || domainToken == "" {
		return apperr.New(423, "LOCKED", "Domain is currently locked by another active operation. Please try again.")
	}
	defer func() {
		_ = s.redisService.ReleaseDomainLock(domainID, domainToken)
	}()

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
	if sourceProjectID == targetProjectID {
		return apperr.New(400, "SAME_PROJECT", "Domain is already assigned to this project")
	}

	// 2. Lock both source and target projects in order of their IDs to prevent deadlocks
	var lockToken1, lockToken2 string
	var lockID1, lockID2 uint
	if sourceProjectID < targetProjectID {
		lockID1, lockID2 = sourceProjectID, targetProjectID
	} else {
		lockID1, lockID2 = targetProjectID, sourceProjectID
	}

	lockToken1, err = s.redisService.AcquireProjectDomainLock(lockID1, 15*time.Second)
	if err != nil || lockToken1 == "" {
		return apperr.New(423, "LOCKED", "Another domain operation is in progress on one of the projects. Please try again.")
	}
	defer func() {
		_ = s.redisService.ReleaseProjectDomainLock(lockID1, lockToken1)
	}()

	lockToken2, err = s.redisService.AcquireProjectDomainLock(lockID2, 15*time.Second)
	if err != nil || lockToken2 == "" {
		return apperr.New(423, "LOCKED", "Another domain operation is in progress on one of the projects. Please try again.")
	}
	defer func() {
		_ = s.redisService.ReleaseProjectDomainLock(lockID2, lockToken2)
	}()

	// 3. Enforce custom domains limit per project on target project
	var currentDomainsCount int64
	s.db.Model(&models.CustomDomain{}).Where("project_id = ? AND status NOT IN (?)", targetProjectID, []string{string(models.DomainStatusPendingCleanup), string(models.DomainStatusDisabled)}).Count(&currentDomainsCount)

	maxDomains := 3
	var setting models.Setting
	if err := s.db.Where("setting_key = ?", models.SettingMaxDomainsPerProject).First(&setting).Error; err == nil {
		if val, err := strconv.Atoi(setting.Value); err == nil {
			maxDomains = val
		}
	}

	if currentDomainsCount >= int64(maxDomains) {
		return apperr.New(400, "DOMAIN_LIMIT_REACHED", fmt.Sprintf("Target project has reached the maximum limit of %d domains", maxDomains))
	}

	// 4. Update project ID of custom domain
	domain.ProjectID = targetProjectID
	if err := s.db.Model(&models.CustomDomain{}).Where("id = ?", domain.ID).Update("project_id", targetProjectID).Error; err != nil {
		return err
	}

	sourceProjectName := fmt.Sprintf("%d", sourceProjectID)
	if domain.Project.Name != "" {
		sourceProjectName = domain.Project.Name
	}
	targetProjectName := fmt.Sprintf("%d", targetProjectID)
	if targetProject != nil && targetProject.Name != "" {
		targetProjectName = targetProject.Name
	}

	event, err := s.RecordEventTx(s.db, &domain, domain.Status, domain.Status, "domain_transferred", fmt.Sprintf("Domain transferred from project %s to project %s", sourceProjectName, targetProjectName), "")
	if err == nil {
		eventBytes, _ := json.Marshal(event)
		_ = s.redisService.PublishDomainEvent(domain.ID, sourceProjectID, string(eventBytes))
		_ = s.redisService.PublishDomainEvent(domain.ID, targetProjectID, string(eventBytes))
	}

	sourceProject, err := s.projectRepo.GetByID(sourceProjectID)
	if err == nil {
		_, _ = s.projectService.SyncProjectNginxFrom(sourceProject, "domain_transfer_source")
	}
	_, _ = s.projectService.SyncProjectNginxFrom(targetProject, "domain_transfer_target")

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
