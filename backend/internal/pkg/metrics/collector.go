package metrics

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

// SummaryMetric tracks sum and count for duration metrics
type SummaryMetric struct {
	sum   int64 // in milliseconds to preserve precision
	count int64
}

func (s *SummaryMetric) Observe(d time.Duration) {
	atomic.AddInt64(&s.sum, d.Milliseconds())
	atomic.AddInt64(&s.count, 1)
}

// MetricsCollector holds all required Prometheus recovery and observability metrics.
type MetricsCollector struct {
	mu sync.RWMutex

	// Reconciliation
	reconcileDurationSeconds SumCount
	reconcileFailuresTotal   int64
	reconciliationDriftTotal int64

	// Lease / Locking
	leaseOwnershipLossTotal   int64
	leaseRenewalFailuresTotal int64
	lockContentionTotal       int64

	// SSL Lifecycle
	sslIssueDurationSeconds SumCount
	sslRetryTotal           int64
	sslRenewalFailuresTotal int64
	sslExpiredTotal         int64

	// Nginx
	nginxReloadTotal           int64
	nginxReloadFailedTotal     int64
	nginxReloadSkippedTotal    int64
	nginxReloadDurationSeconds SumCount

	// SSE / Streaming
	sseConnectionsTotal  int64
	sseActiveConnections int64
	sseReplayTotal       int64
	sseOverflowTotal     int64
	sseDisconnectTotal   int64

	// Cleanup
	cleanupRetriesTotal            int64
	cleanupRecoveredResourcesTotal int64
	orphanResourcesTotal           int64

	// Health
	healthcheckFailuresTotal int64
	degradedDomainsTotal     int64
	unhealthyDomainsTotal    int64
}

type SumCount struct {
	SumMs int64
	Count int64
}

func (sc *SumCount) Observe(d time.Duration) {
	atomic.AddInt64(&sc.SumMs, d.Milliseconds())
	atomic.AddInt64(&sc.Count, 1)
}

var (
	collectorInstance *MetricsCollector
	once              sync.Once
)

// GetCollector returns the global singleton metrics collector.
func GetCollector() *MetricsCollector {
	once.Do(func() {
		collectorInstance = &MetricsCollector{}
	})
	return collectorInstance
}

// ObserveReconcileDuration records the duration of a reconciliation cycle.
func (m *MetricsCollector) ObserveReconcileDuration(d time.Duration) {
	m.reconcileDurationSeconds.Observe(d)
}

func (m *MetricsCollector) IncrReconcileFailures() {
	atomic.AddInt64(&m.reconcileFailuresTotal, 1)
}

func (m *MetricsCollector) IncrReconciliationDrift() {
	atomic.AddInt64(&m.reconciliationDriftTotal, 1)
}

func (m *MetricsCollector) IncrLeaseOwnershipLoss() {
	atomic.AddInt64(&m.leaseOwnershipLossTotal, 1)
}

func (m *MetricsCollector) IncrLeaseRenewalFailures() {
	atomic.AddInt64(&m.leaseRenewalFailuresTotal, 1)
}

func (m *MetricsCollector) IncrLockContention() {
	atomic.AddInt64(&m.lockContentionTotal, 1)
}

func (m *MetricsCollector) ObserveSSLIssueDuration(d time.Duration) {
	m.sslIssueDurationSeconds.Observe(d)
}

func (m *MetricsCollector) IncrSSLRetry() {
	atomic.AddInt64(&m.sslRetryTotal, 1)
}

func (m *MetricsCollector) IncrSSLRenewalFailures() {
	atomic.AddInt64(&m.sslRenewalFailuresTotal, 1)
}

func (m *MetricsCollector) SetSSLExpiredTotal(val int64) {
	atomic.StoreInt64(&m.sslExpiredTotal, val)
}

func (m *MetricsCollector) IncrNginxReloadTotal() {
	atomic.AddInt64(&m.nginxReloadTotal, 1)
}

func (m *MetricsCollector) IncrNginxReloadFailedTotal() {
	atomic.AddInt64(&m.nginxReloadFailedTotal, 1)
}

func (m *MetricsCollector) IncrNginxReloadSkippedTotal() {
	atomic.AddInt64(&m.nginxReloadSkippedTotal, 1)
}

func (m *MetricsCollector) ObserveNginxReloadDuration(d time.Duration) {
	m.nginxReloadDurationSeconds.Observe(d)
}

func (m *MetricsCollector) IncrSSEConnections() {
	atomic.AddInt64(&m.sseConnectionsTotal, 1)
	atomic.AddInt64(&m.sseActiveConnections, 1)
}

func (m *MetricsCollector) DecrSSEConnections() {
	atomic.AddInt64(&m.sseActiveConnections, -1)
	atomic.AddInt64(&m.sseDisconnectTotal, 1)
}

func (m *MetricsCollector) IncrSSEReplayTotal() {
	atomic.AddInt64(&m.sseReplayTotal, 1)
}

func (m *MetricsCollector) IncrSSEOverflowTotal() {
	atomic.AddInt64(&m.sseOverflowTotal, 1)
}

func (m *MetricsCollector) IncrCleanupRetries() {
	atomic.AddInt64(&m.cleanupRetriesTotal, 1)
}

func (m *MetricsCollector) IncrCleanupRecovered() {
	atomic.AddInt64(&m.cleanupRecoveredResourcesTotal, 1)
}

func (m *MetricsCollector) SetOrphanResources(val int64) {
	atomic.StoreInt64(&m.orphanResourcesTotal, val)
}

func (m *MetricsCollector) IncrHealthcheckFailures() {
	atomic.AddInt64(&m.healthcheckFailuresTotal, 1)
}

func (m *MetricsCollector) SetDegradedDomains(val int64) {
	atomic.StoreInt64(&m.degradedDomainsTotal, val)
}

func (m *MetricsCollector) SetUnhealthyDomains(val int64) {
	atomic.StoreInt64(&m.unhealthyDomainsTotal, val)
}

// PrometheusHandler exposes the recorded metrics in standard Prometheus / OpenMetrics text format.
func PrometheusHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		col := GetCollector()
		var buf bytes.Buffer

		writeSummaryMetric(&buf, "reconcile_duration_seconds", "Duration of domain reconciliation passes", atomic.LoadInt64(&col.reconcileDurationSeconds.SumMs), atomic.LoadInt64(&col.reconcileDurationSeconds.Count))
		writeCounterMetric(&buf, "reconcile_failures_total", "Total failed reconciliation passes", atomic.LoadInt64(&col.reconcileFailuresTotal))
		writeCounterMetric(&buf, "reconciliation_drift_total", "Total detected drift instances across state components", atomic.LoadInt64(&col.reconciliationDriftTotal))

		writeCounterMetric(&buf, "lease_ownership_loss_total", "Total worker lease ownership losses", atomic.LoadInt64(&col.leaseOwnershipLossTotal))
		writeCounterMetric(&buf, "lease_renewal_failures_total", "Total lock lease renewal failures", atomic.LoadInt64(&col.leaseRenewalFailuresTotal))
		writeCounterMetric(&buf, "lock_contention_total", "Total lock contention and collision events", atomic.LoadInt64(&col.lockContentionTotal))

		writeSummaryMetric(&buf, "ssl_issue_duration_seconds", "Duration of SSL certificate issuance and verification", atomic.LoadInt64(&col.sslIssueDurationSeconds.SumMs), atomic.LoadInt64(&col.sslIssueDurationSeconds.Count))
		writeCounterMetric(&buf, "ssl_retry_total", "Total retry attempts for Let's Encrypt SSL issuance", atomic.LoadInt64(&col.sslRetryTotal))
		writeCounterMetric(&buf, "ssl_renewal_failures_total", "Total SSL renewal failures", atomic.LoadInt64(&col.sslRenewalFailuresTotal))
		writeGaugeMetric(&buf, "ssl_expired_total", "Total currently expired SSL certificates detected", atomic.LoadInt64(&col.sslExpiredTotal))

		writeCounterMetric(&buf, "nginx_reload_total", "Total triggered Nginx reloads", atomic.LoadInt64(&col.nginxReloadTotal))
		writeCounterMetric(&buf, "nginx_reload_failed_total", "Total failed Nginx reload validation checks", atomic.LoadInt64(&col.nginxReloadFailedTotal))
		writeCounterMetric(&buf, "nginx_reload_skipped_total", "Total skipped Nginx reloads due to matching configuration hash", atomic.LoadInt64(&col.nginxReloadSkippedTotal))
		writeSummaryMetric(&buf, "nginx_reload_duration_seconds", "Duration of Nginx configuration sync and reload execution", atomic.LoadInt64(&col.nginxReloadDurationSeconds.SumMs), atomic.LoadInt64(&col.nginxReloadDurationSeconds.Count))

		writeCounterMetric(&buf, "sse_connections_total", "Total SSE realtime stream connections established", atomic.LoadInt64(&col.sseConnectionsTotal))
		writeGaugeMetric(&buf, "sse_active_connections", "Current active SSE streaming connections", atomic.LoadInt64(&col.sseActiveConnections))
		writeCounterMetric(&buf, "sse_replay_total", "Total SSE event replays triggered via Last-Event-ID", atomic.LoadInt64(&col.sseReplayTotal))
		writeCounterMetric(&buf, "sse_overflow_total", "Total SSE subscriber buffer overflow events", atomic.LoadInt64(&col.sseOverflowTotal))
		writeCounterMetric(&buf, "sse_disconnect_total", "Total SSE client disconnects", atomic.LoadInt64(&col.sseDisconnectTotal))

		writeCounterMetric(&buf, "cleanup_retries_total", "Total retried cleanup operations for disabled domains", atomic.LoadInt64(&col.cleanupRetriesTotal))
		writeCounterMetric(&buf, "cleanup_recovered_resources_total", "Total successfully recovered orphaned resources", atomic.LoadInt64(&col.cleanupRecoveredResourcesTotal))
		writeGaugeMetric(&buf, "orphan_resources_total", "Current orphaned resources pending recovery cleanup", atomic.LoadInt64(&col.orphanResourcesTotal))

		writeCounterMetric(&buf, "healthcheck_failures_total", "Total failed domain healthcheck probing executions", atomic.LoadInt64(&col.healthcheckFailuresTotal))
		writeGaugeMetric(&buf, "degraded_domains_total", "Total custom domains currently in degraded health state", atomic.LoadInt64(&col.degradedDomainsTotal))
		writeGaugeMetric(&buf, "unhealthy_domains_total", "Total custom domains currently in unhealthy operational state", atomic.LoadInt64(&col.unhealthyDomainsTotal))

		c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.Send(buf.Bytes())
	}
}

func writeCounterMetric(buf *bytes.Buffer, name, help string, val int64) {
	buf.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	buf.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
	buf.WriteString(fmt.Sprintf("%s %d\n\n", name, val))
}

func writeGaugeMetric(buf *bytes.Buffer, name, help string, val int64) {
	buf.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	buf.WriteString(fmt.Sprintf("# TYPE %s gauge\n", name))
	buf.WriteString(fmt.Sprintf("%s %d\n\n", name, val))
}

func writeSummaryMetric(buf *bytes.Buffer, name, help string, sumMs, count int64) {
	buf.WriteString(fmt.Sprintf("# HELP %s %s\n", name, help))
	buf.WriteString(fmt.Sprintf("# TYPE %s summary\n", name))
	sumSec := float64(sumMs) / 1000.0
	buf.WriteString(fmt.Sprintf("%s_sum %f\n", name, sumSec))
	buf.WriteString(fmt.Sprintf("%s_count %d\n\n", name, count))
}
