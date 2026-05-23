package domain

import (
	"context"
	"sync"
)

type PollerRegistry struct {
	mu      sync.Mutex
	pollers map[uint]context.CancelFunc
}

func NewPollerRegistry() *PollerRegistry {
	return &PollerRegistry{
		pollers: make(map[uint]context.CancelFunc),
	}
}

func (r *PollerRegistry) Register(domainID uint, cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if oldCancel, exists := r.pollers[domainID]; exists {
		oldCancel()
	}

	r.pollers[domainID] = cancel
	return true
}

func (r *PollerRegistry) Unregister(domainID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pollers, domainID)
}

func (r *PollerRegistry) Cancel(domainID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cancel, exists := r.pollers[domainID]; exists {
		cancel()
		delete(r.pollers, domainID)
	}
}

func (r *PollerRegistry) CancelAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cancel := range r.pollers {
		cancel()
		delete(r.pollers, id)
	}
}
