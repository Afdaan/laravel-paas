package domain

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
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

// AddDomain adds a new custom domain for a project
func (s *DomainService) AddDomain(projectID uint, domainName string) (*models.CustomDomain, error) {
	// Simple validation
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" || strings.Contains(domainName, "/") || strings.Contains(domainName, " ") {
		return nil, apperr.New(400, "INVALID_DOMAIN", "Invalid domain format")
	}

	// Check if already exists globally
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
	
	// Use GORM subquery for better reliability and automatic soft-delete handling
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

	// 1. Verify user owns the source project
	if domain.Project.UserID != userID {
		return apperr.New(403, "FORBIDDEN", "You do not own this domain")
	}

	// 2. Verify user owns the target project
	targetProject, err := s.projectRepo.GetByID(targetProjectID)
	if err != nil {
		return apperr.New(404, "TARGET_NOT_FOUND", "Target project not found")
	}
	if targetProject.UserID != userID {
		return apperr.New(403, "FORBIDDEN", "You do not own the target project")
	}

	sourceProjectID := domain.ProjectID

	// 3. Update Domain ProjectID
	domain.ProjectID = targetProjectID
	if err := s.db.Save(&domain).Error; err != nil {
		return err
	}

	// 4. Sync Nginx for BOTH projects
	// Project A (Source) - Remove domain
	sourceProject, err := s.projectRepo.GetByID(sourceProjectID)
	if err == nil {
		s.projectService.SyncProjectNginx(sourceProject)
	}

	// Project B (Target) - Add domain
	s.projectService.SyncProjectNginx(targetProject)

	return nil
}

// VerifyDomain verifies if the domain's CNAME or A record points to our platform
func (s *DomainService) VerifyDomain(domainID uint, projectID uint, project *models.Project) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := s.db.Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		return nil, apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	// Rate Limiting (max 5 checks per hour per domain)
	rateKey := fmt.Sprintf("ratelimit:domain_verify:%d", domainID)
	allowed, err := s.redisService.RateLimit(rateKey, 5, 1*time.Hour)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, apperr.New(429, "RATE_LIMIT_EXCEEDED", "Verification rate limit exceeded. Please try again later.")
	}

	// Determine the expected target
	projectDomain := s.cfg.ProjectDomain
	if projectDomain == "" {
		projectDomain = s.cfg.BaseDomain
	}
	expectedCNAME := fmt.Sprintf("%s.%s.", project.Subdomain, projectDomain)

	// Lookup CNAME
	cname, err := net.LookupCNAME(domain.Domain)
	if err == nil {
		cname = strings.ToLower(strings.TrimSuffix(cname, "."))
		expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))

		if cname == expectedTrimmed || strings.HasSuffix(cname, projectDomain) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)
			return &domain, nil
		}
		
		// If CNAME exists but is wrong
		return &domain, apperr.New(400, "VERIFICATION_FAILED", fmt.Sprintf("Domain points to %s instead of %s", cname, expectedTrimmed))
	}

	// Fallback to A record check
	ips, err := net.LookupHost(domain.Domain)
	if err == nil && len(ips) > 0 {
		// Try to resolve our project domain to see its current IP
		platformIPs, _ := net.LookupHost(fmt.Sprintf("%s.%s", project.Subdomain, projectDomain))
		
		// If they point it directly to our platform IP
		if len(platformIPs) > 0 && containsAny(ips, platformIPs) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)
			return &domain, nil
		}
	}

	domain.Status = models.DomainStatusError
	s.db.Save(&domain)

	return &domain, apperr.New(400, "VERIFICATION_FAILED", "DNS propagation not detected yet. Please ensure your CNAME is correctly pointing to " + strings.TrimSuffix(expectedCNAME, "."))
}

// containsAny checks if any element of slice A exists in slice B
func containsAny(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
