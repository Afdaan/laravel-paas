// ===========================================
// Docker Circuit Breaker Protection
// ===========================================
package docker

import (
	"log/slog"
	"sync"
	"time"
)

type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half-open"
)

type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failureCount    int
	threshold       int
	window          time.Duration
	cooldown        time.Duration
	lastFailure     time.Time
	lastStateChange time.Time
}

var globalBreaker = &CircuitBreaker{
	state:     StateClosed,
	threshold: 5,
	window:    2 * time.Minute,
	cooldown:  30 * time.Second,
}

// GetCircuitBreaker returns the singleton instance of the Docker circuit breaker.
func GetCircuitBreaker() *CircuitBreaker {
	return globalBreaker
}

// Allow evaluates the current state of the circuit breaker to decide if a Docker operation should proceed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	switch cb.state {
	case StateClosed:
		if !cb.lastFailure.IsZero() && now.Sub(cb.lastFailure) > cb.window {
			cb.failureCount = 0
		}
		return true
	case StateOpen:
		if now.Sub(cb.lastStateChange) > cb.cooldown {
			slog.Info("Circuit breaker entering half-open state for probing")
			cb.state = StateHalfOpen
			cb.lastStateChange = now
			return true
		}
		return false
	case StateHalfOpen:
		return true
	}
	return true
}

// RecordSuccess registers a successful Docker operation, resetting failure counters and fully closing the circuit if it was in recovery.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen || cb.state == StateOpen {
		slog.Info("Circuit breaker health restored, transitioning to closed state")
		cb.state = StateClosed
		cb.failureCount = 0
		cb.lastStateChange = time.Now()
	}
}

// RecordFailure increments the failure tally for Docker socket operations.
// If the failure count reaches the configured threshold (e.g., 5 failures in 2 minutes), the breaker trips to StateOpen to shed load and allow the Docker daemon time to recover.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	if cb.lastFailure.IsZero() || now.Sub(cb.lastFailure) > cb.window {
		cb.failureCount = 1
	} else {
		cb.failureCount++
	}
	cb.lastFailure = now

	if cb.state == StateClosed && cb.failureCount >= cb.threshold {
		slog.Warn("Circuit breaker threshold exceeded, tripping to open state", "failures", cb.failureCount)
		cb.state = StateOpen
		cb.lastStateChange = now
	} else if cb.state == StateHalfOpen {
		slog.Warn("Circuit breaker probe failed, tripping back to open state")
		cb.state = StateOpen
		cb.lastStateChange = now
	}
}
