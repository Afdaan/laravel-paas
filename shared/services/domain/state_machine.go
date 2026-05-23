package domain

import (
	"context"
	"fmt"

	domainAgg "github.com/laravel-paas/shared/domain"
	pkgMetrics "github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repository"
)

type DomainStateMachine struct {
	repo   repository.DomainRepository
	outbox OutboxService
	audit  AuditService
}

func NewDomainStateMachine(repo repository.DomainRepository, outbox OutboxService, audit AuditService) *DomainStateMachine {
	return &DomainStateMachine{
		repo:   repo,
		outbox: outbox,
		audit:  audit,
	}
}

func (sm *DomainStateMachine) Transition(ctx context.Context, id uint, nextStatus models.CustomDomainStatus, cause string) error {
	// Execute the entire state transition sequence inside a database transaction
	// to ensure atomicity of the state update, outbox staging, and audit logging.
	return sm.repo.WithTx(ctx, func(txRepo repository.DomainRepository) error {
		// Acquire a pessimistic row-level lock (SELECT FOR UPDATE) on the domain record
		// to block concurrent worker actions or API updates from racing.
		d, err := txRepo.GetForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to fetch domain for update: %w", err)
		}

		// Enforce domain aggregate transition validation rules.
		agg := domainAgg.NewDomainAggregate(d)
		if !agg.CanTransitionTo(nextStatus) {
			pkgMetrics.GetCollector().IncrDomainTransitionRejected()
			return fmt.Errorf("invalid state transition path: from %s to %s", d.Status, nextStatus)
		}

		fromStatus := d.Status
		if fromStatus == nextStatus {
			// Suppress duplicate transitions to avoid DB write amplification and outbox event noise.
			return nil
		}

		// Atomically increment the sequence number. This forms a monotonic sequence of modifications,
		// allowing the frontend SSE subscriber to reject stale delayed updates or out-of-order writes.
		nextSeq, err := txRepo.IncrementSequenceAtomic(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to increment sequence atomically: %w", err)
		}
		d.Status = nextStatus
		d.CurrentSequence = nextSeq

		// Force health to unhealthy on degraded or failed status transitions to prevent UI mismatch.
		if nextStatus == models.DomainStatusDegraded ||
			nextStatus == models.DomainStatusSSLFailed ||
			nextStatus == models.DomainStatusRenewalFailed ||
			nextStatus == models.DomainStatusError {
			d.HealthStatus = models.DomainHealthUnhealthy
		}

		if err := txRepo.Save(ctx, d); err != nil {
			return fmt.Errorf("failed to save domain state: %w", err)
		}

		dto := DomainStatusDTO{
			ID:              d.ID,
			ProjectID:       d.ProjectID,
			Domain:          d.Domain,
			Status:          string(d.Status),
			HealthStatus:    string(d.HealthStatus),
			LatencyMs:       d.LatencyMs,
			CurrentSequence: d.CurrentSequence,
			SnapshotVersion: d.SnapshotVersion,
			ErrorCode:       string(d.ErrorCode),
			ErrorMessage:    d.ErrorMessage,
			UpdatedAt:       d.UpdatedAt,
		}

		// Stage event to the transactional outbox table in the same transaction.
		// Safe dual-write: Redis pub/sub delivery will only fire after this transaction commits.
		if err := sm.outbox.Enqueue(ctx, txRepo, string(nextStatus), d, dto); err != nil {
			return fmt.Errorf("failed to enqueue outbox event: %w", err)
		}

		// Write append-only diagnostic trail.
		if err := sm.audit.Log(ctx, d.ID, "state_transition", string(fromStatus), string(nextStatus), "success", nil, 0); err != nil {
			return fmt.Errorf("failed to write audit log: %w", err)
		}

		return nil
	})
}
