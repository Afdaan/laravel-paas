package runtime

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

type PanicSeverity string

const (
	SeverityRecoverable PanicSeverity = "recoverable"
	SeverityDegraded    PanicSeverity = "degraded"
	SeverityCritical    PanicSeverity = "critical"
)

type Supervisor struct {
	mu           sync.Mutex
	restarts     map[string][]time.Time
	throttleTime time.Duration
}

func NewSupervisor() *Supervisor {
	return &Supervisor{
		restarts:     make(map[string][]time.Time),
		throttleTime: 2 * time.Second,
	}
}

type SafeGoContext struct {
	TraceID      string
	DomainID     uint
	Operation    string
	RetryCount   int
	LeaseOwner   string
	FailureClass string
}

func (s *Supervisor) SafeGo(ctx context.Context, name string, severity PanicSeverity, fn func() error) {
	s.SafeGoWithContext(ctx, name, severity, SafeGoContext{Operation: name}, fn)
}

func (s *Supervisor) SafeGoWithContext(ctx context.Context, name string, severity PanicSeverity, sgc SafeGoContext, fn func() error) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				slog.Error("async goroutine panicked",
					"goroutine", name,
					"severity", severity,
					"panic", r,
					"trace_id", sgc.TraceID,
					"domain_id", sgc.DomainID,
					"operation", sgc.Operation,
					"retry_count", sgc.RetryCount,
					"lease_owner", sgc.LeaseOwner,
					"failure_class", sgc.FailureClass,
					"stack", stack,
				)

				s.mu.Lock()
				now := time.Now()
				history := s.restarts[name]
				cutoff := now.Add(-1 * time.Minute)
				var filtered []time.Time
				for _, t := range history {
					if t.After(cutoff) {
						filtered = append(filtered, t)
					}
				}
				filtered = append(filtered, now)
				s.restarts[name] = filtered
				panicCount := len(filtered)
				s.mu.Unlock()

				if panicCount > 5 {
					slog.Warn("goroutine is restarting too fast, throttling", "goroutine", name, "sleep", s.throttleTime)
					time.Sleep(s.throttleTime)
				}

				if severity != SeverityCritical && ctx.Err() == nil {
					slog.Info("restarting panic-safeguarded goroutine", "goroutine", name)
					s.SafeGoWithContext(ctx, name, severity, sgc, fn)
				}
			}
		}()

		if err := fn(); err != nil && ctx.Err() == nil {
			slog.Error("goroutine exited with error", "goroutine", name, "error", err)
		}
	}()
}
