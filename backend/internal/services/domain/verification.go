package domain

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
)

// VerifyDomain verifies if the domain's CNAME or A record points to our platform
func (s *DomainService) VerifyDomain(domainID uint, projectID uint, project *models.Project) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := s.db.Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		return nil, apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	rateKey := fmt.Sprintf("ratelimit:domain_verify:%d", domainID)
	allowed, err := s.redisService.RateLimit(rateKey, 5, 1*time.Hour)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, apperr.New(429, "RATE_LIMIT_EXCEEDED", "Verification rate limit exceeded. Please try again later.")
	}

	projectDomain := s.cfg.ProjectDomain
	if projectDomain == "" {
		projectDomain = s.cfg.BaseDomain
	}
	expectedCNAME := fmt.Sprintf("%s.%s.", project.Subdomain, projectDomain)

	cname, err := net.LookupCNAME(domain.Domain)
	if err == nil {
		cname = strings.ToLower(strings.TrimSuffix(cname, "."))
		expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))

		if cname == expectedTrimmed || strings.HasSuffix(cname, projectDomain) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)

			// Trigger Nginx Sync to provision SSL for the new domain
			s.projectService.SyncProjectNginx(project)
			
			return &domain, nil
		}
		
		return &domain, apperr.New(400, "VERIFICATION_FAILED", fmt.Sprintf("Domain points to %s instead of %s", cname, expectedTrimmed))
	}

	ips, err := net.LookupHost(domain.Domain)
	if err == nil && len(ips) > 0 {
		platformIPs, _ := net.LookupHost(fmt.Sprintf("%s.%s", project.Subdomain, projectDomain))
		if len(platformIPs) > 0 && containsAny(ips, platformIPs) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)

			// Trigger Nginx Sync to provision SSL for the new domain
			s.projectService.SyncProjectNginx(project)

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
