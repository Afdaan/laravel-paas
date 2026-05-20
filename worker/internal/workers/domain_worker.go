package workers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/services/domain"
	"github.com/laravel-paas/worker/internal/services/project"
	"gorm.io/gorm"
)

type DomainWorker struct {
	db             *gorm.DB
	domainService  *domain.DomainService
	projectService *project.ProjectService
	redisService   *infrastructure.RedisService
}

func NewDomainWorker(db *gorm.DB, domainService *domain.DomainService, projectService *project.ProjectService, redisService *infrastructure.RedisService) *DomainWorker {
	return &DomainWorker{
		db:             db,
		domainService:  domainService,
		projectService: projectService,
		redisService:   redisService,
	}
}

func (w *DomainWorker) Start(ctx context.Context) {
	slog.Info("Starting Production Domain Reconciliation Watchdog (every 1 minute)")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	workerID := fmt.Sprintf("reconciler-%s", hostname)

	w.Reconcile(ctx, workerID)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Domain reconciliation worker shutting down cleanly")
			return
		case <-ticker.C:
			w.Reconcile(ctx, workerID)
		}
	}
}

// Reconcile performs a single complete reconciliation pass across all domain lifecycle stages
func (w *DomainWorker) Reconcile(ctx context.Context, workerID string) {
	start := time.Now()
	defer func() { metrics.GetCollector().ObserveReconcileDuration(time.Since(start)) }()

	select {
	case <-ctx.Done():
		return
	default:
	}

	leader, err := w.redisService.AcquireOrRenewReconcilerLock(workerID, 2*time.Minute)
	if err != nil {
		metrics.GetCollector().IncrReconcileFailures()
		slog.Warn("Reconciler lock acquisition failed due to Redis unavailability. Degrading gracefully.", "workerID", workerID, "error", err)
		return
	}
	if !leader {
		return
	}

	slog.Info("Reconciler Leadership confirmed. Starting reconciliation cycle.", "workerID", workerID)

	// Track overall health states
	var degradedCount, unhealthyCount int64
	_ = w.db.Model(&models.CustomDomain{}).Where("status = ?", string(models.DomainStatusDegraded)).Count(&degradedCount)
	_ = w.db.Model(&models.CustomDomain{}).Where("health_status = ?", string(models.DomainHealthUnhealthy)).Count(&unhealthyCount)
	metrics.GetCollector().SetDegradedDomains(degradedCount)
	metrics.GetCollector().SetUnhealthyDomains(unhealthyCount)

	w.detectAndReconcileDrift(ctx)
	w.detectStalledReconciliation(ctx)
	w.reconcilePendingDomains(ctx)
	w.reconcileSSLRenewals(ctx)
	w.reconcileCleanupDomains(ctx)
	w.pruneDomainEvents(ctx)
}

func (w *DomainWorker) pruneDomainEvents(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if err := w.db.Where("created_at < ?", cutoff).Delete(&models.DomainEvent{}).Error; err != nil {
		slog.Warn("Failed to prune old domain events", "error", err)
	}
}

func (w *DomainWorker) detectStalledReconciliation(ctx context.Context) {
	var domains []models.CustomDomain
	stalledCutoff := time.Now().Add(-15 * time.Minute)
	statuses := []string{
		string(models.DomainStatusPending),
		string(models.DomainStatusPendingDNS),
		string(models.DomainStatusDNSVerified),
		string(models.DomainStatusSSLQueued),
		string(models.DomainStatusSSLProvisioning),
		string(models.DomainStatusRenewalPending),
	}
	err := w.db.Where("status IN (?) AND updated_at < ?", statuses, stalledCutoff).Find(&domains).Error
	if err != nil || len(domains) == 0 {
		return
	}
	for i := range domains {
		select {
		case <-ctx.Done():
			return
		default:
		}
		d := &domains[i]
		metrics.GetCollector().IncrReconcileFailures()
		slog.Warn("Recovery trace: Reconciliation stalled for domain", "domain", d.Domain, "status", d.Status, "lastUpdatedAt", d.UpdatedAt)
		_ = w.domainService.TransitionState(d, models.DomainStatusDegraded, models.ErrReconciliationStalled, fmt.Sprintf("Reconciliation stalled in state %s for >15 minutes", d.Status))
	}
}

func (w *DomainWorker) detectAndReconcileDrift(ctx context.Context) {
	var domains []models.CustomDomain
	err := w.db.Where("status = ?", string(models.DomainStatusActive)).Find(&domains).Error
	if err != nil || len(domains) == 0 {
		return
	}

	for i := range domains {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d := &domains[i]
		project, err := w.projectService.GetProjectByID(d.ProjectID)
		if err != nil {
			continue
		}

		// 1. Check DNS Drift
		diag, err := w.domainService.GetDomainDiagnostic(d.Domain, project)
		if err == nil && !diag.IsMatch {
			metrics.GetCollector().IncrReconciliationDrift()
			slog.Warn("Reconciliation drift detected: DNS resolution mismatch on active domain", "domain", d.Domain, "currentIPs", diag.CurrentIPs, "expectedValue", diag.ExpectedValue)
			_ = w.domainService.TransitionState(d, models.DomainStatusDegraded, models.ErrDomainNotResolved, fmt.Sprintf("Drift detected: domain resolves to %v instead of %s", diag.CurrentIPs, diag.ExpectedValue))
			continue
		}

		// 2. Check SSL / Nginx Drift
		sslStatus, err := w.projectService.GetSSLStatus(d.Domain)
		if err != nil {
			metrics.GetCollector().IncrReconciliationDrift()
			slog.Warn("Reconciliation drift detected: Public routing Nginx webhook unreachable or SSL check failed", "domain", d.Domain, "error", err)
			_ = w.domainService.TransitionState(d, models.DomainStatusDegraded, models.ErrPublicRouteUnreachable, fmt.Sprintf("Drift detected: public routing Nginx verification failed: %v", err))
			continue
		}

		if sslStatus.Status == "failed" || sslStatus.Status == "expired" {
			metrics.GetCollector().IncrReconciliationDrift()
			slog.Warn("Reconciliation drift detected: SSL status is failed or expired on active domain", "domain", d.Domain, "sslStatus", sslStatus.Status)
			errCode := models.ErrSSLIssuanceFailed
			if sslStatus.Status == "expired" {
				errCode = models.ErrSSLExpired
			}
			_ = w.domainService.TransitionState(d, models.DomainStatusDegraded, errCode, fmt.Sprintf("Drift detected: SSL certificate is %s", sslStatus.Status))
			continue
		}

		if d.HealthStatus == models.DomainHealthUnhealthy || d.LastHealthcheckAt == nil || time.Since(*d.LastHealthcheckAt) > 5*time.Minute {
			w.domainService.SafeGo(ctx, d.ID, d.ProjectID, "CheckAppHealth", func(ctx context.Context) error {
				w.domainService.CheckAppHealth(ctx, d.ID, d.ProjectID)
				return nil
			})
		}
	}
}

func (w *DomainWorker) reconcilePendingDomains(ctx context.Context) {
	limit := 50
	offset := 0
	statuses := []string{
		string(models.DomainStatusPending),
		string(models.DomainStatusPendingDNS),
		string(models.DomainStatusDNSVerified),
		string(models.DomainStatusSSLQueued),
		string(models.DomainStatusSSLProvisioning),
		string(models.DomainStatusDegraded),
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var domains []models.CustomDomain
		err := w.db.Where("status IN (?)", statuses).Order("id ASC").Offset(offset).Limit(limit).Find(&domains).Error
		if err != nil || len(domains) == 0 {
			break
		}

		for i := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}

			d := &domains[i]
			if d.Status == models.DomainStatusDegraded && d.ProvisioningCheckpoint == "completed" {
				continue
			}
			if d.Status == models.DomainStatusSSLQueued || d.Status == models.DomainStatusSSLProvisioning {
				sslStatus, err := w.projectService.GetSSLStatus(d.Domain)
				if err != nil {
					slog.Warn("Recovery trace: Public routing Nginx unreachable during SSL polling", "domain", d.Domain, "error", err)
					continue
				}
				slog.Info("Recovery trace: SSL polling status received", "domain", d.Domain, "sslStatus", sslStatus.Status, "retryCount", sslStatus.RetryCount)
				if sslStatus.RetryCount > 0 {
					metrics.GetCollector().IncrSSLRetry()
				}
				switch sslStatus.Status {
				case "ssl_active":
					metrics.GetCollector().ObserveSSLIssueDuration(time.Since(d.CreatedAt))
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
					_ = w.db.Model(d).Updates(map[string]interface{}{
						"provisioning_checkpoint": "completed",
						"ssl_status":              "active",
						"ssl_issued_at":           d.SSLIssuedAt,
						"ssl_expires_at":          d.SSLExpiresAt,
					})
					_ = w.domainService.TransitionState(d, models.DomainStatusActive, models.ErrNone, "Let's Encrypt SSL certificate provisioned successfully")
					w.domainService.SafeGo(ctx, d.ID, d.ProjectID, "CheckAppHealth", func(ctx context.Context) error {
						w.domainService.CheckAppHealth(ctx, d.ID, d.ProjectID)
						return nil
					})
					slog.Info("Recovery trace: SSL certificate issued successfully", "domain", d.Domain, "durationSec", time.Since(d.CreatedAt).Seconds())
				case "ssl_failed":
					d.VerificationRetryCount++
					_ = w.db.Model(d).Update("verification_retry_count", d.VerificationRetryCount)
					if d.VerificationRetryCount >= 5 {
						_ = w.domainService.TransitionState(d, models.DomainStatusSSLFailed, models.ErrSSLIssuanceFailed, fmt.Sprintf("SSL issuance failed after %d retries: %s", d.VerificationRetryCount, sslStatus.Error))
					} else {
						slog.Warn("Recovery trace: SSL provisioning retry in progress", "domain", d.Domain, "attempt", d.VerificationRetryCount, "error", sslStatus.Error)
					}
				case "ssl_provisioning":
					if d.Status != models.DomainStatusSSLProvisioning {
						d.ProvisioningCheckpoint = "ssl_provisioning"
						_ = w.db.Model(d).Update("provisioning_checkpoint", "ssl_provisioning")
						_ = w.domainService.TransitionState(d, models.DomainStatusSSLProvisioning, models.ErrNone, "Active Let's Encrypt challenge verification in progress")
					}
				}
				now := time.Now()
				_ = w.db.Model(d).Update("last_reconciliation_at", now)
				continue
			}

			if d.LastReconciliationAt != nil && d.VerificationRetryCount > 0 {
				backoffMinutes := (1 << (d.VerificationRetryCount - 1)) * 2
				if time.Since(*d.LastReconciliationAt).Minutes() < float64(backoffMinutes) {
					continue
				}
			}

			project, err := w.projectService.GetProjectByID(d.ProjectID)
			if err != nil {
				continue
			}

			verifyStart := time.Now()
			if d.ProvisioningCheckpoint == "init" {
				d.ProvisioningCheckpoint = "verifying_dns"
				_ = w.db.Model(d).Update("provisioning_checkpoint", "verifying_dns")
			}

			_, verifyErr := w.domainService.VerifyDomain(ctx, d.ID, d.ProjectID, project)
			if verifyErr == nil {
				d.ProvisioningCheckpoint = "completed"
				d.VerificationRetryCount = 0
				_ = w.db.Model(d).Updates(map[string]interface{}{
					"provisioning_checkpoint":  "completed",
					"verification_retry_count": 0,
				})
				slog.Info("Recovery trace: Domain successfully verified and provisioned.", "domain", d.Domain, "durationMs", time.Since(verifyStart).Milliseconds())
			} else {
				d.VerificationRetryCount++
				_ = w.db.Model(d).Update("verification_retry_count", d.VerificationRetryCount)
				slog.Warn("Recovery trace: Domain verification retry in progress.", "domain", d.Domain, "attempt", d.VerificationRetryCount, "error", verifyErr)
			}

			now := time.Now()
			_ = w.db.Model(d).Updates(map[string]interface{}{
				"last_reconciliation_at":   now,
				"verification_retry_count": d.VerificationRetryCount,
			})
		}

		if len(domains) < limit {
			break
		}
		offset += limit
		time.Sleep(100 * time.Millisecond) // Jitter between batches
	}
}

func (w *DomainWorker) reconcileSSLRenewals(ctx context.Context) {
	limit := 50
	offset := 0
	statuses := []string{
		string(models.DomainStatusActive),
		string(models.DomainStatusDegraded),
		string(models.DomainStatusRenewalPending),
		string(models.DomainStatusRenewalFailed),
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var domains []models.CustomDomain
		err := w.db.Where("status IN (?) AND ssl_expires_at IS NOT NULL", statuses).Order("id ASC").Offset(offset).Limit(limit).Find(&domains).Error
		if err != nil || len(domains) == 0 {
			break
		}

		now := time.Now()
		var expiredCount int64
		for i := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}

			d := &domains[i]
			if d.SSLExpiresAt == nil {
				continue
			}

			timeRemaining := d.SSLExpiresAt.Sub(now)
			daysRemaining := timeRemaining.Hours() / 24.0

			if daysRemaining < 0 {
				expiredCount++
			}

			if d.LastRenewalAttemptAt != nil && d.RenewalRetryCount > 0 {
				backoffHours := (1 << (d.RenewalRetryCount - 1)) * 2
				if now.Sub(*d.LastRenewalAttemptAt).Hours() < float64(backoffHours) {
					continue
				}
			}

			if daysRemaining <= 3.0 {
				metrics.GetCollector().IncrSSLRenewalFailures()
				_ = w.domainService.TransitionState(d, models.DomainStatusDegraded, models.ErrSSLIssuanceFailed, fmt.Sprintf("SSL certificate expires in %.1f days! Urgent renewal required.", daysRemaining))
			} else if daysRemaining <= 7.0 {
				slog.Warn("Aggressive SSL renewal triggered", "domain", d.Domain, "daysRemaining", daysRemaining)
				w.triggerRenewal(ctx, d)
			} else if daysRemaining <= 21.0 && d.Status != models.DomainStatusRenewalPending {
				slog.Info("Standard Let's Encrypt renewal window reached", "domain", d.Domain, "daysRemaining", daysRemaining)
				_ = w.domainService.TransitionState(d, models.DomainStatusRenewalPending, models.ErrNone, "Proactive SSL renewal scheduled")
				w.triggerRenewal(ctx, d)
			}
		}

		metrics.GetCollector().SetSSLExpiredTotal(expiredCount)

		if len(domains) < limit {
			break
		}
		offset += limit
		time.Sleep(100 * time.Millisecond)
	}
}

func (w *DomainWorker) reconcileCleanupDomains(ctx context.Context) {
	limit := 50
	offset := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var domains []models.CustomDomain
		err := w.db.Where("status = ?", string(models.DomainStatusPendingCleanup)).Order("id ASC").Offset(offset).Limit(limit).Find(&domains).Error
		if err != nil || len(domains) == 0 {
			metrics.GetCollector().SetOrphanResources(0)
			break
		}

		metrics.GetCollector().SetOrphanResources(int64(len(domains)))

		for i := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}

			d := &domains[i]
			token, err := w.redisService.AcquireDomainLock(d.ID, 30*time.Second)
			if err != nil || token == "" {
				metrics.GetCollector().IncrLockContention()
				continue
			}

			lockCtx, cancelLock := context.WithCancel(context.Background())
			w.domainService.StartLockHeartbeat(lockCtx, cancelLock, d, token, 30*time.Second)

			// Checkpoint 1: Purge Nginx Configuration
			select {
			case <-ctx.Done():
				_ = w.redisService.ReleaseDomainLock(d.ID, token)
				cancelLock()
				return
			case <-lockCtx.Done():
				_ = w.redisService.ReleaseDomainLock(d.ID, token)
				cancelLock()
				continue
			default:
			}

			if d.CleanupCheckpoint == "init" {
				project, err := w.projectService.GetProjectByID(d.ProjectID)
				if err == nil {
					if _, err := w.projectService.SyncProjectNginxFrom(project, "domain_cleanup_reconcile"); err != nil {
						w.handleCleanupFailure(d, token, cancelLock, fmt.Sprintf("Nginx purge failed: %v", err))
						continue
					}
				}
				d.CleanupCheckpoint = "nginx_purged"
				_ = w.db.Model(d).Update("cleanup_checkpoint", "nginx_purged")
				_ = w.domainService.RecordEvent(d, d.Status, d.Status, "cleanup_step", "Nginx routing configuration successfully purged", "")
			}

			select {
			case <-ctx.Done():
				_ = w.redisService.ReleaseDomainLock(d.ID, token)
				cancelLock()
				return
			case <-lockCtx.Done():
				_ = w.redisService.ReleaseDomainLock(d.ID, token)
				cancelLock()
				continue
			default:
			}

			// Checkpoint 2: Finalize Teardown & Soft Delete / Tombstone
			if d.CleanupCheckpoint == "nginx_purged" {
				d.CleanupCheckpoint = "done"
				_ = w.db.Model(d).Update("cleanup_checkpoint", "done")
				_ = w.domainService.TransitionStateCtx(lockCtx, d, models.DomainStatusDisabled, models.ErrNone, "Domain cleanup finalized and routing purged")
				metrics.GetCollector().IncrCleanupRecovered()

				// Soft delete / tombstone
				_ = w.db.Delete(d).Error
				_ = w.redisService.ReleaseDomainLock(d.ID, token)
				cancelLock()
				slog.Info("Recovery trace: Custom domain routing completely purged and tombstoned", "domain", d.Domain)
			}
		}

		if len(domains) < limit {
			break
		}
		offset += limit
		time.Sleep(100 * time.Millisecond)
	}
}

func (w *DomainWorker) handleCleanupFailure(d *models.CustomDomain, token string, cancel context.CancelFunc, errMsg string) {
	metrics.GetCollector().IncrCleanupRetries()
	d.CleanupRetryCount++
	_ = w.db.Model(d).Updates(map[string]interface{}{
		"cleanup_retry_count": d.CleanupRetryCount,
	})
	_ = w.domainService.RecordEvent(d, d.Status, d.Status, "cleanup_failed", errMsg, fmt.Sprintf("Retry count: %d", d.CleanupRetryCount))
	_ = w.redisService.ReleaseDomainLock(d.ID, token)
	cancel()
}

func (w *DomainWorker) triggerRenewal(ctx context.Context, domain *models.CustomDomain) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	token, err := w.redisService.AcquireDomainLock(domain.ID, 30*time.Second)
	if err != nil || token == "" {
		metrics.GetCollector().IncrLockContention()
		return
	}
	lockCtx, cancelLock := context.WithCancel(context.Background())
	defer cancelLock()
	defer func() {
		_ = w.redisService.ReleaseDomainLock(domain.ID, token)
	}()
	w.domainService.StartLockHeartbeat(lockCtx, cancelLock, domain, token, 30*time.Second)

	if domain.RenewalCheckpoint == "init" {
		domain.RenewalCheckpoint = "sync_requested"
		_ = w.db.Model(domain).Update("renewal_checkpoint", "sync_requested")
	}

	now := time.Now()
	domain.LastRenewalAttemptAt = &now
	domain.RenewalRetryCount++
	_ = w.db.Model(domain).Updates(map[string]interface{}{
		"last_renewal_attempt_at": now,
		"renewal_retry_count":     domain.RenewalRetryCount,
	})

	project, err := w.projectService.GetProjectByID(domain.ProjectID)
	if err != nil {
		return
	}

	select {
	case <-ctx.Done():
		return
	case <-lockCtx.Done():
		return
	default:
	}

	_, err = w.projectService.SyncProjectNginxFrom(project, "ssl_renewal_reconcile")
	if err != nil {
		metrics.GetCollector().IncrSSLRenewalFailures()
		slog.Error("SSL renewal Nginx sync failed", "domain", domain.Domain, "error", err)
		_ = w.domainService.TransitionStateCtx(lockCtx, domain, models.DomainStatusRenewalFailed, models.ErrSSLIssuanceFailed, fmt.Sprintf("Renewal attempt %d failed: %v", domain.RenewalRetryCount, err))
	} else {
		domain.RenewalCheckpoint = "active"
		_ = w.db.Model(domain).Update("renewal_checkpoint", "active")
		slog.Info("Recovery trace: SSL renewal successfully synced", "domain", domain.Domain)
	}
}
