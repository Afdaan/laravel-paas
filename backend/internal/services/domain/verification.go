package domain

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/metrics"
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

// VerifyDomain verifies if the domain's CNAME or A record points to our platform and initiates SSL provisioning.
func (s *DomainService) VerifyDomain(domainID uint, projectID uint, project *models.Project) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := s.db.Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		return nil, apperr.New(404, "NOT_FOUND", "Domain not found")
	}

	token, err := s.redisService.AcquireDomainLock(domain.ID, 30*time.Second)
	if err != nil || token == "" {
		metrics.GetCollector().IncrLockContention()
		return nil, apperr.New(423, "LOCKED", "Domain is currently locked by an active verification or provisioning operation. Please wait a few seconds.")
	}
	defer func() {
		_ = s.redisService.ReleaseDomainLock(domain.ID, token)
	}()

	lockCtx, cancelLock := context.WithCancel(context.Background())
	defer cancelLock()
	s.StartLockHeartbeat(lockCtx, cancelLock, &domain, token, 30*time.Second)

	rateKey := fmt.Sprintf("ratelimit:domain_verify_v2:%d", domainID)
	allowed, err := s.redisService.RateLimit(rateKey, 20, 1*time.Hour)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, apperr.New(429, "RATE_LIMIT_EXCEEDED", "Verification rate limit exceeded. Please try again later.")
	}

	_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusPendingDNS, models.ErrNone, "Initiating DNS verification check")

	projectDomain := s.cfg.ProjectDomain
	if projectDomain == "" {
		projectDomain = s.cfg.BaseDomain
	}
	expectedCNAME := fmt.Sprintf("%s.%s.", project.Subdomain, projectDomain)
	resolver := getRealtimeResolver()
	ctx, cancel := context.WithTimeout(lockCtx, 10*time.Second)
	defer cancel()

	dnsVerified := false

	cname, err := resolver.LookupCNAME(ctx, domain.Domain)
	if err == nil {
		cname = strings.ToLower(strings.TrimSuffix(cname, "."))
		expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))

		if cname == expectedTrimmed || strings.HasSuffix(cname, projectDomain) {
			dnsVerified = true
		}
	}

	if !dnsVerified {
		ips, err := resolver.LookupHost(ctx, domain.Domain)
		if err == nil && len(ips) > 0 {
			platformIPs, _ := resolver.LookupHost(ctx, fmt.Sprintf("%s.%s", project.Subdomain, projectDomain))
			if len(platformIPs) > 0 && containsAny(ips, platformIPs) {
				dnsVerified = true
			}
		}
	}

	if lockCtx.Err() != nil {
		return nil, fmt.Errorf("operation aborted due to lock lease expiration or cancellation: %w", lockCtx.Err())
	}

	if dnsVerified {
		_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusDNSVerified, models.ErrNone, "DNS successfully verified. Queuing SSL configuration.")

		now := time.Now()
		domain.LastVerificationAt = &now
		_ = s.db.Model(&domain).Update("last_verification_at", now)

		// Trigger Nginx configuration sync which automatically enqueues Let's Encrypt certificate issuance
		if _, err := s.projectService.SyncProjectNginxFrom(project, "domain_verify"); err != nil {
			_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusDegraded, models.ErrNginxReloadFailed, fmt.Sprintf("Nginx configuration sync failed: %v", err))
		} else {
			_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusSSLQueued, models.ErrNone, "Let's Encrypt SSL certificate issuance queued successfully")
		}

		go s.CheckAppHealth(&domain, project)
		return &domain, nil
	}

	now := time.Now()
	domain.LastVerificationAt = &now
	domain.VerificationRetryCount++
	_ = s.db.Model(&domain).Updates(map[string]interface{}{
		"last_verification_at":     now,
		"verification_retry_count": domain.VerificationRetryCount,
	})

	_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusPendingDNS, models.ErrDomainNotResolved, "DNS verification pending propagation. Please ensure your CNAME record is configured correctly.")
	return &domain, apperr.New(400, "VERIFICATION_FAILED", "DNS propagation not detected yet. Please ensure your CNAME is correctly pointing to "+strings.TrimSuffix(expectedCNAME, "."))
}

// CheckAppHealth verifies end-to-end edge and upstream connectivity across 5 distinct operational layers: DOMAIN -> DNS -> EDGE -> SSL -> UPSTREAM -> INTEGRITY.
func (s *DomainService) CheckAppHealth(domain *models.CustomDomain, project *models.Project) {
	now := time.Now()
	updates := map[string]interface{}{
		"last_healthcheck_at": &now,
	}

	if domain.Status != models.DomainStatusActive && domain.Status != models.DomainStatusSSLActive && domain.Status != models.DomainStatusSSLQueued {
		updates["health_status"] = models.DomainHealthUnknown
		updates["latency_ms"] = 0
		_ = s.db.Model(domain).Updates(updates)
		return
	}

	start := time.Now()
	resolver := getRealtimeResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// LAYER 1: DNS Reachable
	layer1DNS := false
	cname, err := resolver.LookupCNAME(ctx, domain.Domain)
	if err == nil && cname != "" {
		layer1DNS = true
	} else {
		ips, err := resolver.LookupHost(ctx, domain.Domain)
		if err == nil && len(ips) > 0 {
			layer1DNS = true
		}
	}
	updates["layer1_dns_reachable"] = layer1DNS

	if !layer1DNS {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrDomainNotResolved
		updates["error_message"] = "Layer 1: DNS resolution failed"
		_ = s.db.Model(domain).Updates(updates)
		_ = s.RecordEvent(domain, domain.Status, domain.Status, "healthcheck_failed", "Layer 1 DNS resolution failed", "DNS lookup timed out or returned NXDOMAIN")
		return
	}

	// LAYER 2: Edge Reachable & LAYER 3: SSL Valid & LAYER 4: Upstream Reachable & LAYER 5: Response Integrity
	scheme := "https"
	if domain.Status != models.DomainStatusActive && domain.Status != models.DomainStatusSSLActive {
		scheme = "http"
	}
	targetURL := fmt.Sprintf("%s://%s", scheme, domain.Domain)

	client := &http.Client{
		Timeout: 7 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["error_message"] = "Failed to construct healthcheck request"
		_ = s.db.Model(domain).Updates(updates)
		return
	}
	req.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")

	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	updates["latency_ms"] = latency
	s.redisService.RecordDomainMetricDuration("healthcheck_latency", time.Since(start))

	layer3SSL := false
	layer5Integrity := false

	if err != nil {
		if scheme == "https" {
			// HTTPS failed, check if HTTP works
			scheme = "http"
			targetURL = fmt.Sprintf("http://%s", domain.Domain)
			req, _ = http.NewRequest("GET", targetURL, nil)
			req.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")
			startHTTP := time.Now()
			resp, err = client.Do(req)
			latency = time.Since(startHTTP).Milliseconds()
			updates["latency_ms"] = latency
		}

		if err != nil {
			metrics.GetCollector().IncrHealthcheckFailures()
			updates["layer2_edge_reachable"] = false
			updates["health_status"] = models.DomainHealthUnhealthy
			updates["degraded_reason_code"] = models.ErrRoutingHealthFailed
			updates["error_message"] = fmt.Sprintf("Layer 2: Edge routing unreachable (%s): %v", scheme, err)
			_ = s.db.Model(domain).Updates(updates)
			_ = s.RecordEvent(domain, domain.Status, domain.Status, "healthcheck_failed", "Edge routing unreachable", err.Error())
			return
		}
	}

	if scheme == "https" && resp != nil && resp.TLS != nil {
		layer3SSL = true
	}

	updates["layer2_edge_reachable"] = true
	updates["layer3_ssl_valid"] = layer3SSL

	if resp != nil && resp.StatusCode >= 500 {
		_ = resp.Body.Close()
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["layer4_upstream_reachable"] = false
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrUpstreamTimeout
		updates["error_message"] = fmt.Sprintf("Layer 4: Upstream application returned HTTP %d", resp.StatusCode)
		_ = s.db.Model(domain).Updates(updates)
		_ = s.RecordEvent(domain, domain.Status, domain.Status, "healthcheck_failed", fmt.Sprintf("Upstream returned HTTP %d", resp.StatusCode), "Application backend error or 502/504 Bad Gateway")
		return
	}
	updates["layer4_upstream_reachable"] = true

	// LAYER 5: Response Integrity Validation
	// Make explicit GET request to verify actual deployment ownership against configurable marker
	if resp != nil {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		_ = resp.Body.Close()

		marker := s.cfg.IntegrityValidationMarker
		if marker == "" {
			marker = "laravel-paas"
		}

		if strings.Contains(bodyStr, marker) || strings.Contains(bodyStr, `"status":"ok"`) || strings.Contains(bodyStr, `'status':'ok'`) {
			layer5Integrity = true
		} else {
			// Make explicit GET /up request to verify Laravel 11+ healthcheck endpoint
			upURL := fmt.Sprintf("%s://%s/up", scheme, domain.Domain)
			upReq, _ := http.NewRequest("GET", upURL, nil)
			upReq.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")
			if upResp, err := client.Do(upReq); err == nil && upResp != nil {
				upBytes, _ := io.ReadAll(upResp.Body)
				upStr := string(upBytes)
				_ = upResp.Body.Close()
				if strings.Contains(upStr, marker) || strings.Contains(upStr, `"status":"ok"`) || strings.Contains(upStr, `'status':'ok'`) {
					layer5Integrity = true
				}
			}
		}
	}
	updates["layer5_response_integrity"] = layer5Integrity

	if !layer5Integrity {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrIntegrityCheckFailed
		updates["error_message"] = "Layer 5: Response integrity marker verification failed"
		_ = s.db.Model(domain).Updates(updates)
		_ = s.RecordEvent(domain, domain.Status, domain.Status, "healthcheck_degraded", "Layer 5 response integrity failed", "Application did not return expected verification marker")
		return
	}

	updates["health_status"] = models.DomainHealthHealthy
	updates["degraded_reason_code"] = models.ErrNone
	updates["error_message"] = ""
	_ = s.db.Model(domain).Updates(updates)
	s.redisService.IncrDomainMetric("healthcheck_success", 1)
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
