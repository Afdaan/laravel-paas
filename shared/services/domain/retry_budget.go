package domain

import (
	"sync"
	"time"
)

type RetryBudget struct {
	MaxRetries      int
	RetryWindow     time.Duration
	RemainingBudget int
}

type DomainRetryTracker struct {
	mu       sync.Mutex
	attempts map[uint][]time.Time
}

func NewDomainRetryTracker() *DomainRetryTracker {
	return &DomainRetryTracker{
		attempts: make(map[uint][]time.Time),
	}
}

func (rt *DomainRetryTracker) CanRetry(domainID uint, budget RetryBudget) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-budget.RetryWindow)

	attempts := rt.attempts[domainID]
	var activeAttempts []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			activeAttempts = append(activeAttempts, t)
		}
	}

	if len(activeAttempts) >= budget.MaxRetries {
		return false
	}

	activeAttempts = append(activeAttempts, now)
	rt.attempts[domainID] = activeAttempts
	return true
}
