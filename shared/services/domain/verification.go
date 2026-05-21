package domain

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
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
func (s *DomainService) VerifyDomain(ctx context.Context, domainID uint, projectID uint, project *models.Project) (*models.CustomDomain, error) {
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

	lockCtx, cancelLock := context.WithCancel(ctx)
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

	isAlreadyActive := domain.Status == models.DomainStatusActive

	if !isAlreadyActive {
		_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusPendingDNS, models.ErrNone, "Initiating DNS verification check")
	}

	projectDomain := s.cfg.ProjectDomain
	if projectDomain == "" {
		projectDomain = s.cfg.BaseDomain
	}
	expectedCNAME := fmt.Sprintf("%s.%s.", project.Subdomain, projectDomain)
	resolver := getRealtimeResolver()
	ctx, cancel := context.WithTimeout(lockCtx, 10*time.Second)
	defer cancel()

	dnsVerified := false

	isLocalEnv := s.cfg.AppMode == "local"
	isTestDomain := strings.HasSuffix(domain.Domain, ".localhost") || strings.HasSuffix(domain.Domain, ".local") || strings.HasSuffix(domain.Domain, ".test")

	if isLocalEnv || isTestDomain {
		dnsVerified = true
	} else {
		cname, err := resolver.LookupCNAME(ctx, domain.Domain)
		if err == nil {
			cname = strings.ToLower(strings.TrimSuffix(cname, "."))
			expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))
			centralCNAME := "cname." + strings.ToLower(strings.TrimSuffix(projectDomain, "."))

			if cname == expectedTrimmed || cname == centralCNAME || strings.HasSuffix(cname, projectDomain) {
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

		// Fallback to system default resolver (e.g. /etc/hosts) if public DNS fails
		if !dnsVerified {
			cname, err := net.DefaultResolver.LookupCNAME(ctx, domain.Domain)
			if err == nil {
				cname = strings.ToLower(strings.TrimSuffix(cname, "."))
				expectedTrimmed := strings.ToLower(strings.TrimSuffix(expectedCNAME, "."))
				centralCNAME := "cname." + strings.ToLower(strings.TrimSuffix(projectDomain, "."))
				if cname == expectedTrimmed || cname == centralCNAME || strings.HasSuffix(cname, projectDomain) {
					dnsVerified = true
				}
			}
		}

		if !dnsVerified {
			ips, err := net.DefaultResolver.LookupHost(ctx, domain.Domain)
			if err == nil && len(ips) > 0 {
				platformIPs, err2 := net.DefaultResolver.LookupHost(ctx, fmt.Sprintf("%s.%s", project.Subdomain, projectDomain))
				if err2 == nil && len(platformIPs) > 0 && containsAny(ips, platformIPs) {
					dnsVerified = true
				}
			}
		}
	}

	if lockCtx.Err() != nil {
		return nil, fmt.Errorf("operation aborted due to lock lease expiration or cancellation: %w", lockCtx.Err())
	}

	if dnsVerified {
		now := time.Now()
		domain.LastVerificationAt = &now
		_ = s.db.Model(&domain).Update("last_verification_at", now)

		if isAlreadyActive {
			// Passive health/status check only. Since it is already active and healthy,
			// avoid redundant state transition, Nginx re-generation, or server reload.
			s.SafeGo(ctx, domain.ID, project.ID, "CheckAppHealth", func(ctx context.Context) error {
				s.CheckAppHealth(ctx, domain.ID, project.ID)
				return nil
			})
			return &domain, nil
		}

		_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusDNSVerified, models.ErrNone, "DNS successfully verified. Queuing SSL configuration.")

		// Trigger Nginx configuration sync which automatically enqueues Let's Encrypt certificate issuance
		if _, err := s.projectService.SyncProjectNginxFrom(project, "domain_verify"); err != nil {
			_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusDegraded, models.ErrNginxReloadFailed, fmt.Sprintf("Nginx configuration sync failed: %v", err))
		} else {
			if isLocalEnv || isTestDomain {
				domain.ProvisioningCheckpoint = "completed"
				domain.SSLStatus = "active"
				now := time.Now()
				domain.SSLIssuedAt = &now
				expires := now.AddDate(0, 3, 0)
				domain.SSLExpiresAt = &expires
				_ = s.db.Model(&domain).Updates(map[string]interface{}{
					"provisioning_checkpoint": "completed",
					"ssl_status":              "active",
					"ssl_issued_at":           domain.SSLIssuedAt,
					"ssl_expires_at":          domain.SSLExpiresAt,
				})
				_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusActive, models.ErrNone, `{"verification_mode":"local"}`)
			} else {
				_ = s.TransitionStateCtx(lockCtx, &domain, models.DomainStatusSSLQueued, models.ErrNone, "Let's Encrypt SSL certificate issuance queued successfully")
				s.SafeGo(ctx, domain.ID, project.ID, "pollSSLStatusRealtime", func(ctx context.Context) error {
					s.pollSSLStatusRealtime(ctx, domain.ID, project.ID)
					return nil
				})
			}
		}

		s.SafeGo(ctx, domain.ID, project.ID, "CheckAppHealth", func(ctx context.Context) error {
			s.CheckAppHealth(ctx, domain.ID, project.ID)
			return nil
		})
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

// CheckAppHealth verifies end-to-end public access and upstream connectivity across 5 distinct operational layers: DOMAIN -> DNS -> PUBLIC ACCESS -> SSL -> UPSTREAM -> INTEGRITY.
func (s *DomainService) CheckAppHealth(ctx context.Context, domainID, projectID uint) {
	if ctx != nil && ctx.Err() != nil {
		slog.Warn("Domain healthcheck aborted: parent context cancelled", "domainID", domainID)
		return
	}

	var domain models.CustomDomain
	if err := s.db.Select("id, project_id, domain, status, current_sequence, health_status, degraded_reason_code, error_message").Where("id = ? AND project_id = ?", domainID, projectID).First(&domain).Error; err != nil {
		slog.Warn("Domain healthcheck aborted: domain not found", "domainID", domainID)
		return
	}

	var exists bool
	if err := s.db.Model(&models.Project{}).Select("1").Where("id = ?", projectID).Limit(1).Scan(&exists).Error; err != nil || !exists {
		slog.Warn("Domain healthcheck aborted: project not found", "projectID", projectID)
		return
	}

	prevHealth := domain.HealthStatus
	prevErrCode := domain.DegradedReasonCode
	prevErrMsg := domain.ErrorMessage

	now := time.Now()
	updates := map[string]interface{}{
		"last_healthcheck_at": &now,
	}

	if domain.Status != models.DomainStatusActive && domain.Status != models.DomainStatusSSLActive && domain.Status != models.DomainStatusSSLQueued {
		updates["health_status"] = models.DomainHealthUnknown
		updates["latency_ms"] = 0
		_ = s.db.Model(&domain).Updates(updates)
		domain.HealthStatus = models.DomainHealthUnknown
		return
	}

	if s.IsLocalOrTestDomain(domain.Domain) {
		updates["health_status"] = models.DomainHealthHealthy
		updates["latency_ms"] = int64(0)
		updates["layer1_dns_reachable"] = true
		updates["layer2_public_access_reachable"] = true
		updates["layer3_ssl_valid"] = true
		updates["layer4_upstream_reachable"] = true
		updates["layer5_response_integrity"] = true
		updates["degraded_reason_code"] = models.ErrNone
		updates["error_message"] = ""
		_ = s.db.Model(&domain).Updates(updates)

		if prevHealth != models.DomainHealthHealthy || prevErrCode != models.ErrNone || prevErrMsg != "" {
			_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_recovered", "Local environment bypass", "")
		}
		return
	}

	resolver := getRealtimeResolver()
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// LAYER 1: DNS Reachable (with up to 3 retries to handle transient UDP drop / network latency)
	layer1DNS := false
	var dnsErr error
dnsLoop:
	for attempt := 1; attempt <= 3; attempt++ {
		cname, err := resolver.LookupCNAME(checkCtx, domain.Domain)
		if err != nil {
			cname, err = net.DefaultResolver.LookupCNAME(checkCtx, domain.Domain)
		}
		if err == nil && cname != "" {
			layer1DNS = true
			break dnsLoop
		}

		var ips []string
		ips, err = resolver.LookupHost(checkCtx, domain.Domain)
		if err != nil {
			ips, err = net.DefaultResolver.LookupHost(checkCtx, domain.Domain)
		}
		if err == nil && len(ips) > 0 {
			layer1DNS = true
			break dnsLoop
		}

		dnsErr = err
		select {
		case <-checkCtx.Done():
			break dnsLoop
		case <-time.After(200 * time.Millisecond):
		}
	}
	updates["layer1_dns_reachable"] = layer1DNS

	if !layer1DNS {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrDomainNotResolved
		updates["error_message"] = "Layer 1: DNS resolution failed"
		_ = s.db.Model(&domain).Updates(updates)
		domain.HealthStatus = models.DomainHealthUnhealthy
		domain.DegradedReasonCode = models.ErrDomainNotResolved
		domain.ErrorMessage = "Layer 1: DNS resolution failed"
		errMsg := "DNS lookup timed out or returned NXDOMAIN"
		if dnsErr != nil {
			errMsg = dnsErr.Error()
		}
		_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_failed", "Layer 1 DNS resolution failed", errMsg)
		return
	}

	// LAYER 2: Public Access Reachable & LAYER 3: SSL Valid & LAYER 4: Upstream Reachable & LAYER 5: Response Integrity
	// We run up to 3 attempts with a backoff to survive transient HTTP errors, cold starts, and zero-downtime container swaps.
	var resp *http.Response
	var latency int64
	var err error
	layer3SSL := false
	layer5Integrity := false
	layer4Upstream := false
	scheme := "https"

	client := &http.Client{
		Transport: GetHTTPClient().Transport,
		Timeout: 7 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

httpLoop:
	for attempt := 1; attempt <= 3; attempt++ {
		scheme = "https"
		if domain.Status != models.DomainStatusActive && domain.Status != models.DomainStatusSSLActive {
			scheme = "http"
		}
		targetURL := fmt.Sprintf("%s://%s", scheme, domain.Domain)

		req, errReq := http.NewRequestWithContext(checkCtx, "GET", targetURL, nil)
		if errReq != nil {
			err = errReq
			break httpLoop
		}
		req.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")

		startHTTP := time.Now()
		resp, err = client.Do(req)
		latency = time.Since(startHTTP).Milliseconds()

		layer3SSL = false
		layer4Upstream = false
		layer5Integrity = false

		if err != nil {
			if scheme == "https" {
				// HTTPS failed, check if HTTP works
				scheme = "http"
				targetURL = fmt.Sprintf("http://%s", domain.Domain)
				req, _ = http.NewRequestWithContext(checkCtx, "GET", targetURL, nil)
				if req != nil {
					req.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")
					startHTTP = time.Now()
					resp, err = client.Do(req)
					latency = time.Since(startHTTP).Milliseconds()
				}
			}
		}

		if err == nil && resp != nil {
			if scheme == "https" && resp.TLS != nil {
				layer3SSL = true
			}

			if resp.StatusCode < 500 {
				layer4Upstream = true

				// LAYER 5: Response Integrity
				bodyBytes, _ := io.ReadAll(resp.Body)
				bodyStr := string(bodyBytes)
				_ = resp.Body.Close()

				marker := strings.TrimSpace(s.cfg.IntegrityValidationMarker)
				if marker == "" {
					layer5Integrity = true
				} else if strings.Contains(bodyStr, marker) || strings.Contains(bodyStr, `"status":"ok"`) || strings.Contains(bodyStr, `'status':'ok'`) {
					layer5Integrity = true
				} else {
					// Fallback to Laravel's health check endpoint
					upURL := fmt.Sprintf("%s://%s/up", scheme, domain.Domain)
					upReq, _ := http.NewRequestWithContext(checkCtx, "GET", upURL, nil)
					if upReq != nil {
						upReq.Header.Set("User-Agent", "LaravelPaaS-Healthcheck/1.0")
						if upResp, upErr := client.Do(upReq); upErr == nil && upResp != nil {
							upBytes, _ := io.ReadAll(upResp.Body)
							upStr := string(upBytes)
							_ = upResp.Body.Close()
							if strings.Contains(upStr, marker) || strings.Contains(upStr, `"status":"ok"`) || strings.Contains(upStr, `'status':'ok'`) {
								layer5Integrity = true
							}
						}
					}
				}

				// If the response is fully healthy (including response integrity check), break out of retry loop.
				if layer5Integrity {
					break httpLoop
				}
			} else {
				_ = resp.Body.Close()
			}
		}

		// Wait before retrying (exponential-like backoff: 500ms, 1000ms)
		select {
		case <-checkCtx.Done():
			break httpLoop
		case <-time.After(time.Duration(attempt*500) * time.Millisecond):
		}
	}

	updates["latency_ms"] = latency
	s.redisService.RecordDomainMetricDuration("healthcheck_latency", time.Duration(latency)*time.Millisecond)

	if err != nil {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["layer2_public_access_reachable"] = false
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrRoutingHealthFailed
		updates["error_message"] = fmt.Sprintf("Layer 2: Public routing unreachable (%s): %v", scheme, err)
		_ = s.db.Model(&domain).Updates(updates)
		domain.HealthStatus = models.DomainHealthUnhealthy
		domain.DegradedReasonCode = models.ErrRoutingHealthFailed
		domain.ErrorMessage = fmt.Sprintf("Layer 2: Public routing unreachable (%s): %v", scheme, err)
		_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_failed", "Public routing unreachable", err.Error())
		return
	}

	updates["layer2_public_access_reachable"] = true
	updates["layer3_ssl_valid"] = layer3SSL

	if !layer4Upstream {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["layer4_upstream_reachable"] = false
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrUpstreamTimeout
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		updates["error_message"] = fmt.Sprintf("Layer 4: Upstream application returned HTTP %d", statusCode)
		_ = s.db.Model(&domain).Updates(updates)
		domain.HealthStatus = models.DomainHealthUnhealthy
		domain.DegradedReasonCode = models.ErrUpstreamTimeout
		domain.ErrorMessage = fmt.Sprintf("Layer 4: Upstream application returned HTTP %d", statusCode)
		_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_failed", fmt.Sprintf("Upstream returned HTTP %d", statusCode), "Application backend error or 502/504 Bad Gateway")
		return
	}
	updates["layer4_upstream_reachable"] = true
	updates["layer5_response_integrity"] = layer5Integrity

	if !layer5Integrity {
		metrics.GetCollector().IncrHealthcheckFailures()
		updates["health_status"] = models.DomainHealthUnhealthy
		updates["degraded_reason_code"] = models.ErrIntegrityCheckFailed
		updates["error_message"] = "Layer 5: Response integrity marker verification failed"
		_ = s.db.Model(&domain).Updates(updates)
		domain.HealthStatus = models.DomainHealthUnhealthy
		domain.DegradedReasonCode = models.ErrIntegrityCheckFailed
		domain.ErrorMessage = "Layer 5: Response integrity marker verification failed"
		_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_degraded", "Layer 5 response integrity failed", "Application did not return expected verification marker")
		return
	}

	updates["health_status"] = models.DomainHealthHealthy
	updates["degraded_reason_code"] = models.ErrNone
	updates["error_message"] = ""
	_ = s.db.Model(&domain).Updates(updates)

	domain.HealthStatus = models.DomainHealthHealthy
	domain.DegradedReasonCode = models.ErrNone
	domain.ErrorMessage = ""

	if prevHealth != models.DomainHealthHealthy || prevErrCode != models.ErrNone || prevErrMsg != "" {
		_ = s.RecordEvent(&domain, domain.Status, domain.Status, "healthcheck_recovered", "Layer 5 response integrity verified successfully", "All 5 operational layers fully verified and healthy")
	}
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

func (s *DomainService) pollSSLStatusRealtime(ctx context.Context, domainID uint, projectID uint) {
	now := time.Now()
	if val, loaded := s.activePollers.LoadOrStore(domainID, PollerState{StartedAt: now}); loaded {
		state := val.(PollerState)
		if now.Sub(state.StartedAt) > 10*time.Minute {
			slog.Warn("Stale SSL status poller detected, replacing it", "domainID", domainID, "age", now.Sub(state.StartedAt))
			s.activePollers.Store(domainID, PollerState{StartedAt: now})
			metrics.GetCollector().IncrDomainPollerStopped()
			_ = s.RecordEvent(&models.CustomDomain{ID: domainID}, models.DomainStatusSSLQueued, models.DomainStatusSSLQueued, "poller_cleanup", `{"reason":"stale_ttl_timeout"}`, "")
		} else {
			slog.Info("SSL status poller already active for domain, skipping new goroutine spawn", "domainID", domainID)
			return
		}
	}
	metrics.GetCollector().IncrDomainPollerStarted()
	defer func() {
		s.activePollers.Delete(domainID)
		metrics.GetCollector().IncrDomainPollerStopped()
	}()

	slog.Info("Starting real-time SSL status polling", "domainID", domainID)
	pollCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	backoffSteps := []time.Duration{
		2 * time.Second,
		3 * time.Second,
		5 * time.Second,
		8 * time.Second,
		13 * time.Second,
		21 * time.Second,
		30 * time.Second,
	}
	stepIdx := 0

	getDelay := func() time.Duration {
		if stepIdx < len(backoffSteps) {
			d := backoffSteps[stepIdx]
			stepIdx++
			return d
		}
		return 30 * time.Second
	}

	delay := getDelay()
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-pollCtx.Done():
			slog.Info("Real-time SSL status polling timed out or cancelled", "domainID", domainID)
			return
		case <-timer.C:
			var d models.CustomDomain
			if err := s.db.First(&d, domainID).Error; err != nil {
				slog.Error("Failed to find domain during real-time SSL polling", "domainID", domainID, "error", err)
				return
			}

			if d.Status != models.DomainStatusSSLQueued && d.Status != models.DomainStatusSSLProvisioning {
				slog.Info("Stopping real-time SSL polling: domain status has changed", "domainID", domainID, "status", d.Status)
				return
			}

			sslStatus, err := s.projectService.GetSSLStatus(d.Domain)
			if err != nil {
				slog.Warn("Failed to query SSL status in real-time polling", "domain", d.Domain, "error", err)
				delay = getDelay()
				timer.Reset(delay)
				continue
			}

			slog.Info("Real-time SSL polling check result", "domain", d.Domain, "sslStatus", sslStatus.Status)

			switch sslStatus.Status {
			case "ssl_active":
				d.ProvisioningCheckpoint = "completed"
				d.SSLStatus = "active"
				now := time.Now()
				d.SSLIssuedAt = &now
				expires := now.AddDate(0, 3, 0)
				if sslStatus.ExpiresAt != "" {
					if parsed, err := time.Parse("Jan 2 15:04:05 2006 MST", sslStatus.ExpiresAt); err == nil {
						expires = parsed
					}
				}
				d.SSLExpiresAt = &expires
				_ = s.db.Model(&d).Updates(map[string]interface{}{
					"provisioning_checkpoint": "completed",
					"ssl_status":              "active",
					"ssl_issued_at":           d.SSLIssuedAt,
					"ssl_expires_at":          d.SSLExpiresAt,
				})
				_ = s.TransitionState(&d, models.DomainStatusActive, models.ErrNone, "Let's Encrypt SSL certificate provisioned successfully")
				s.SafeGo(ctx, d.ID, projectID, "CheckAppHealth", func(ctx context.Context) error {
					s.CheckAppHealth(ctx, d.ID, projectID)
					return nil
				})
				return

			case "ssl_failed":
				d.VerificationRetryCount++
				_ = s.db.Model(&d).Update("verification_retry_count", d.VerificationRetryCount)
				if d.VerificationRetryCount >= 5 {
					_ = s.TransitionState(&d, models.DomainStatusSSLFailed, models.ErrSSLIssuanceFailed, fmt.Sprintf("SSL issuance failed after 5 retries: %s", sslStatus.Error))
					return
				}

			case "ssl_provisioning":
				if d.Status != models.DomainStatusSSLProvisioning {
					d.ProvisioningCheckpoint = "ssl_provisioning"
					_ = s.db.Model(&d).Update("provisioning_checkpoint", "ssl_provisioning")
					_ = s.TransitionState(&d, models.DomainStatusSSLProvisioning, models.ErrNone, "Active Let's Encrypt challenge verification in progress")
				}
			}

			delay = getDelay()
			timer.Reset(delay)
		}
	}
}

// IsLocalOrTestDomain returns true if the environment is configured as local or if the domain is a local test domain.
func (s *DomainService) IsLocalOrTestDomain(domainName string) bool {
	isLocalEnv := s.cfg.AppMode == "local"
	isTestDomain := strings.HasSuffix(domainName, ".localhost") || strings.HasSuffix(domainName, ".local") || strings.HasSuffix(domainName, ".test")
	return isLocalEnv || isTestDomain
}
