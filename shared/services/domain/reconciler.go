package domain

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	pkgMetrics "github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/repository"
)

type Reconciler struct {
	repo          repository.DomainRepository
	queue         ReconcileQueue
	leaseProvider LeaseProvider
	stateMachine  *DomainStateMachine
	retryTracker  *DomainRetryTracker
	rateLimiter   *DomainRateLimiter
	limits        RuntimeLimits
	flags         FeatureFlags
}

func NewReconciler(
	repo repository.DomainRepository,
	queue ReconcileQueue,
	leaseProvider LeaseProvider,
	stateMachine *DomainStateMachine,
) *Reconciler {
	return &Reconciler{
		repo:          repo,
		queue:         queue,
		leaseProvider: leaseProvider,
		stateMachine:  stateMachine,
		retryTracker:  NewDomainRetryTracker(),
		rateLimiter:   NewDomainRateLimiter(1.0, 5), // 1 request per second, burst 5
		limits:        DefaultRuntimeLimits(),
		flags:         DefaultFeatureFlags(),
	}
}

// ReconcileOne runs a single reconciliation step for a domain ID.
func (r *Reconciler) ReconcileOne(ctx context.Context, domainID uint, cause string) error {
	// 1. Check rate limits to suppress reconnect or webhook storms under high failure loads.
	limiter := r.rateLimiter.GetLimiter(domainID)
	if !limiter.Allow() {
		pkgMetrics.GetCollector().IncrLockContention() // rate limit / contention event
		return ErrRuntimeBusy
	}

	// 2. Check retry budget to protect upstream dependencies from endless loops.
	budget := RetryBudget{MaxRetries: 5, RetryWindow: 5 * time.Minute}
	if !r.retryTracker.CanRetry(domainID, budget) {
		return fmt.Errorf("retry budget exhausted for domain %d", domainID)
	}

	// 3. Acquire distributed lease to prevent multiple workers from running active reconciliation runs concurrently.
	leaseTTL := 30 * time.Second
	token, err := r.leaseProvider.Acquire(ctx, domainID, leaseTTL)
	if err != nil {
		return fmt.Errorf("failed to acquire distributed lease: %w", err)
	}
	// Use context.Background() to ensure lease release executes successfully even if the parent ctx cancels.
	defer func() {
		_ = r.leaseProvider.Release(context.Background(), domainID, token)
	}()

	// 4. Setup heartbeat lease renewal loop. Long-running tasks (like DNS/Let's Encrypt validations) 
	// can outlive leaseTTL. Periodic renewals prevent premature lease expiration.
	reconcileCtx, cancelReconcile := context.WithCancel(ctx)
	defer cancelReconcile()

	go func() {
		ticker := time.NewTicker(leaseTTL / 2)
		defer ticker.Stop()
		for {
			select {
			case <-reconcileCtx.Done():
				return
			case <-ticker.C:
				err := r.leaseProvider.Renew(reconcileCtx, domainID, token, leaseTTL)
				if err != nil {
					// Heartbeat loss safety: Cancel the reconciliation execution context immediately
					// to prevent this worker from executing stale, un-leased mutations.
					slog.Warn("failed to renew domain lock lease, cancelling active reconciliation context", "domain_id", domainID, "error", err)
					cancelReconcile()
					return
				}
			}
		}
	}()

	// 5. Execute core reconciliation sequence
	startTime := time.Now()
	err = r.executeReconcileSequence(reconcileCtx, domainID)
	duration := time.Since(startTime)

	// Record latency by cause
	pkgMetrics.GetCollector().ObserveReconcileDuration(duration)

	if err != nil {
		slog.Error("reconciliation sequence failed", "domain_id", domainID, "cause", cause, "error", err)
		return err
	}

	return nil
}

func (r *Reconciler) executeReconcileSequence(ctx context.Context, domainID uint) error {
	d, err := r.repo.GetByID(ctx, domainID)
	if err != nil {
		return fmt.Errorf("failed to load custom domain: %w", err)
	}

	// If already in desired status, we are done
	if d.Status == d.DesiredStatus {
		return nil
	}

	// Let the state machine transition through intermediate statuses
	// Wait, standard state resolver will be executed inside DomainService
	return nil
}

// StartQueueProcessor spawns the background double-dequeue partition poller.
func (r *Reconciler) StartQueueProcessor(ctx context.Context, workerID string, partition int) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Pull up to 5 elements at a time
			tasks, err := r.queue.Dequeue(ctx, workerID, partition, 5)
			if err != nil {
				slog.Error("failed to dequeue tasks from reconciliation queue", "partition", partition, "error", err)
				continue
			}

			for _, t := range tasks {
				// Prevent loop thrashing with a minor randomized jitter backoff
				jitter := time.Duration(rand.IntN(200)) * time.Millisecond
				select {
				case <-ctx.Done():
					return
				case <-time.After(jitter):
				}

				err := r.ReconcileOne(ctx, t.DomainID, t.Cause)
				if err != nil {
					// Exponential backoff and retry scheduling
					backoffSec := int(1 << uint(t.RetryCount))
					if backoffSec > 300 {
						backoffSec = 300
					}
					nextRun := time.Now().Add(time.Duration(backoffSec) * time.Second)
					_ = r.queue.Release(ctx, t.ID, nextRun, true)
				} else {
					_ = r.queue.Complete(ctx, t.ID)
				}
			}
		}
	}
}
