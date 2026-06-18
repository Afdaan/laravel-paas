package metrics

import "time"

type DomainMetrics interface {
	Increment(metric string)
	IncrementBy(metric string, val int64)
	ObserveDuration(metric string, d time.Duration)
}

type noopDomainMetrics struct{}

func NewNoopDomainMetrics() DomainMetrics {
	return &noopDomainMetrics{}
}

func (n *noopDomainMetrics) Increment(metric string)                        {}
func (n *noopDomainMetrics) IncrementBy(metric string, val int64)           {}
func (n *noopDomainMetrics) ObserveDuration(metric string, d time.Duration) {}
