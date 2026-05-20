package domain

import (
	"sync"

	"golang.org/x/time/rate"
)

type DomainRateLimiter struct {
	mu       sync.RWMutex
	limiters map[uint]*rate.Limiter
	r        rate.Limit
	b        int
}

func NewDomainRateLimiter(r rate.Limit, b int) *DomainRateLimiter {
	return &DomainRateLimiter{
		limiters: make(map[uint]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

func (l *DomainRateLimiter) GetLimiter(domainID uint) *rate.Limiter {
	l.mu.RLock()
	lim, ok := l.limiters[domainID]
	l.mu.RUnlock()
	if ok {
		return lim
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok = l.limiters[domainID]
	if ok {
		return lim
	}
	lim = rate.NewLimiter(l.r, l.b)
	l.limiters[domainID] = lim
	return lim
}
