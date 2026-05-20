package domain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/laravel-paas/shared/infrastructure"
)

type LeaseProvider interface {
	Acquire(ctx context.Context, domainID uint, ttl time.Duration) (uuid.UUID, error)
	Release(ctx context.Context, domainID uint, token uuid.UUID) error
	Renew(ctx context.Context, domainID uint, token uuid.UUID, ttl time.Duration) error
}

type redisLeaseProvider struct {
	redisService *infrastructure.RedisService
}

func NewRedisLeaseProvider(redisService *infrastructure.RedisService) LeaseProvider {
	return &redisLeaseProvider{redisService: redisService}
}

func (p *redisLeaseProvider) Acquire(ctx context.Context, domainID uint, ttl time.Duration) (uuid.UUID, error) {
	tokenStr, err := p.redisService.AcquireDomainLock(domainID, ttl)
	if err != nil {
		return uuid.Nil, err
	}
	if tokenStr == "" {
		return uuid.Nil, fmt.Errorf("lease conflict: lock already held for domain %d", domainID)
	}
	// Parse returned token as UUID to enforce standard validation across the control-plane.
	token, err := uuid.Parse(tokenStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse token as UUID: %w", err)
	}
	return token, nil
}

func (p *redisLeaseProvider) Release(ctx context.Context, domainID uint, token uuid.UUID) error {
	tokenStr := token.String()
	// Redis script uses exact match. Since uuid.String() has hyphens but AcquireDomainLock may use 
	// generateLockToken without hyphens, we try releasing using the hex-encoded string without hyphens.
	// This avoids silent lease leakage due to string format mismatches.
	hexStr := fmt.Sprintf("%x", token)
	err := p.redisService.ReleaseDomainLock(domainID, hexStr)
	if err != nil {
		// Fallback check with hyphens for legacy compatibility.
		return p.redisService.ReleaseDomainLock(domainID, tokenStr)
	}
	return nil
}

func (p *redisLeaseProvider) Renew(ctx context.Context, domainID uint, token uuid.UUID, ttl time.Duration) error {
	hexStr := fmt.Sprintf("%x", token)
	err := p.redisService.RenewDomainLock(domainID, hexStr, ttl)
	if err != nil {
		tokenStr := token.String()
		// Fallback check with hyphens for legacy compatibility.
		return p.redisService.RenewDomainLock(domainID, tokenStr, ttl)
	}
	return nil
}

type memoryLease struct {
	token     uuid.UUID
	expiresAt time.Time
}

type memoryLeaseProvider struct {
	mu     sync.Mutex
	leases map[uint]memoryLease
}

func NewMemoryLeaseProvider() LeaseProvider {
	return &memoryLeaseProvider{
		leases: make(map[uint]memoryLease),
	}
}

func (p *memoryLeaseProvider) Acquire(ctx context.Context, domainID uint, ttl time.Duration) (uuid.UUID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if l, ok := p.leases[domainID]; ok && l.expiresAt.After(now) {
		return uuid.Nil, fmt.Errorf("lease conflict: lock already held for domain %d", domainID)
	}

	token := uuid.New()
	p.leases[domainID] = memoryLease{
		token:     token,
		expiresAt: now.Add(ttl),
	}
	return token, nil
}

func (p *memoryLeaseProvider) Release(ctx context.Context, domainID uint, token uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	l, ok := p.leases[domainID]
	if !ok {
		return nil
	}
	if l.token != token {
		return fmt.Errorf("invalid token for domain %d", domainID)
	}

	delete(p.leases, domainID)
	return nil
}

func (p *memoryLeaseProvider) Renew(ctx context.Context, domainID uint, token uuid.UUID, ttl time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	l, ok := p.leases[domainID]
	if !ok {
		return fmt.Errorf("lease not found or expired for domain %d", domainID)
	}
	if l.token != token {
		return fmt.Errorf("invalid token for domain %d", domainID)
	}
	if l.expiresAt.Before(time.Now()) {
		return fmt.Errorf("lease already expired for domain %d", domainID)
	}

	l.expiresAt = time.Now().Add(ttl)
	p.leases[domainID] = l
	return nil
}
