package domain

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
)

// getRealtimeResolver returns a resolver that bypasses local cache by querying a public DNS server
func getRealtimeResolver() *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			// Query Google DNS (8.8.8.8) directly to bypass local caching
			return d.DialContext(ctx, network, "8.8.8.8:53")
		},
	}
}

// VerifyDomain verifies if the domain's CNAME or A record points to our platform
func (s *DomainService) VerifyDomain(domainID uint, projectID uint, project *models.Project) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := s.db.Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		return nil, apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	rateKey := fmt.Sprintf("ratelimit:domain_verify_v2:%d", domainID)
	// Loosened limit to 20 requests per hour for better developer experience
	allowed, err := s.redisService.RateLimit(rateKey, 20, 1*time.Hour)
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
	resolver := getRealtimeResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cname, err := resolver.LookupCNAME(ctx, domain.Domain)
	if err == nil {
		cname = strings.ToLower(strings.TrimSuffix(cname, "."))
		expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))

		if cname == expectedTrimmed || strings.HasSuffix(cname, projectDomain) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)
			
			return &domain, nil
		}
		// Fallthrough to IP check if CNAME doesn't match exactly
	}

	ips, err := resolver.LookupHost(ctx, domain.Domain)
	if err == nil && len(ips) > 0 {
		platformIPs, _ := resolver.LookupHost(ctx, fmt.Sprintf("%s.%s", project.Subdomain, projectDomain))
		if len(platformIPs) > 0 && containsAny(ips, platformIPs) {
			domain.Status = models.DomainStatusActive
			s.db.Save(&domain)

			return &domain, nil
		}
	}

	// Set back to PENDING instead of ERROR to avoid scaring the user during propagation
	domain.Status = models.DomainStatusPending
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
