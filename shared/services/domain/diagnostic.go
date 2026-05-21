package domain

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"golang.org/x/net/publicsuffix"
)

// DomainDiagnostic holds the results of a DNS diagnostic check
type DomainDiagnostic struct {
	Domain        string   `json:"domain"`
	ExpectedType  string   `json:"expected_type"`
	ExpectedHost  string   `json:"expected_host"`
	ExpectedValue string   `json:"expected_value"`
	CurrentCNAME  string   `json:"current_cname"`
	CurrentIPs    []string `json:"current_ips"`
	IsMatch       bool     `json:"is_match"`
	Message       string   `json:"message"`
}

// GetDomainDiagnostic performs a real-time DNS lookup and analysis
func (s *DomainService) GetDomainDiagnostic(domainName string, project *models.Project) (*DomainDiagnostic, error) {
	domainName = strings.ToLower(strings.TrimSpace(domainName))

	// Determine Expected Configuration
	projectDomain := s.cfg.ProjectDomain
	if projectDomain == "" {
		projectDomain = s.cfg.BaseDomain
	}
	expectedValue := fmt.Sprintf("%s.%s", project.Subdomain, projectDomain)

	// DYNAMIC HOST DETECTION using Public Suffix List
	expectedHost := "@"
	registeredDomain, err := publicsuffix.EffectiveTLDPlusOne(domainName)
	if err == nil {
		if domainName != registeredDomain {
			expectedHost = strings.TrimSuffix(domainName, "."+registeredDomain)
		}
	} else {
		// Fallback for custom, local, or single-word domains when public suffix lookup fails
		parts := strings.Split(domainName, ".")
		if len(parts) == 1 {
			expectedHost = domainName
		} else if len(parts) > 2 {
			expectedHost = strings.Join(parts[:len(parts)-2], ".")
		}
	}

	diagnostic := &DomainDiagnostic{
		Domain:        domainName,
		ExpectedType:  "CNAME",
		ExpectedHost:  expectedHost,
		ExpectedValue: expectedValue,
	}

	if s.IsLocalOrTestDomain(domainName) {
		diagnostic.IsMatch = true
		diagnostic.Message = "Local environment DNS bypass."
		return diagnostic, nil
	}

	resolver := getRealtimeResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Current State Check using real-time resolver
	cname, err := resolver.LookupCNAME(ctx, domainName)
	if err == nil {
		diagnostic.CurrentCNAME = strings.ToLower(strings.TrimSuffix(cname, "."))
	}

	ips, _ := resolver.LookupHost(ctx, domainName)
	diagnostic.CurrentIPs = ips

	// Logic for Match
	expectedValueLower := strings.ToLower(expectedValue)
	if diagnostic.CurrentCNAME == expectedValueLower || strings.HasSuffix(diagnostic.CurrentCNAME, projectDomain) {
		diagnostic.IsMatch = true
		diagnostic.Message = "Valid configuration detected."
	} else if len(ips) > 0 {
		platformIPs, _ := resolver.LookupHost(ctx, expectedValue)
		if len(platformIPs) > 0 && containsAny(ips, platformIPs) {
			diagnostic.IsMatch = true
			diagnostic.Message = "Routing established via A record."
		} else {
			diagnostic.Message = "Domain resolves to an external IP address."
		}
	} else {
		diagnostic.Message = "No active DNS records detected."
	}

	return diagnostic, nil
}
