package domain

import (
	"time"

	"github.com/laravel-paas/shared/models"
)

// Map status domain to be showed
var allowedTransitions = map[models.CustomDomainStatus]map[models.CustomDomainStatus]bool{
	models.DomainStatusPending: {
		models.DomainStatusPendingDNS:     true,
		models.DomainStatusActive:         true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
		models.DomainStatusDegraded:       true,
	},
	models.DomainStatusPendingDNS: {
		models.DomainStatusPendingDNS:     true,
		models.DomainStatusDNSVerified:    true,
		models.DomainStatusActive:         true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
		models.DomainStatusDegraded:       true,
	},
	models.DomainStatusDNSVerified: {
		models.DomainStatusSSLQueued:      true,
		models.DomainStatusActive:         true,
		models.DomainStatusDegraded:       true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusSSLQueued: {
		models.DomainStatusSSLProvisioning: true,
		models.DomainStatusActive:          true,
		models.DomainStatusSSLFailed:       true,
		models.DomainStatusDisabled:        true,
		models.DomainStatusPendingCleanup:  true,
		models.DomainStatusDegraded:        true,
	},
	models.DomainStatusSSLProvisioning: {
		models.DomainStatusActive:         true,
		models.DomainStatusSSLFailed:      true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
		models.DomainStatusDegraded:       true,
	},
	models.DomainStatusSSLActive: {
		models.DomainStatusActive:         true,
		models.DomainStatusDegraded:       true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusActive: {
		models.DomainStatusDegraded:       true,
		models.DomainStatusRenewalPending: true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusDegraded: {
		models.DomainStatusActive:         true,
		models.DomainStatusSSLFailed:      true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusRenewalPending: {
		models.DomainStatusActive:         true,
		models.DomainStatusRenewalFailed:  true,
		models.DomainStatusDegraded:       true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusRenewalFailed: {
		models.DomainStatusRenewalPending: true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusSSLFailed: {
		models.DomainStatusPendingDNS:     true,
		models.DomainStatusDisabled:       true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusDisabled: {
		models.DomainStatusPendingDNS:     true,
		models.DomainStatusPending:        true,
		models.DomainStatusPendingCleanup: true,
	},
	models.DomainStatusPendingCleanup: {
		models.DomainStatusPending:    true,
		models.DomainStatusPendingDNS: true,
		models.DomainStatusActive:     true,
		models.DomainStatusDisabled:   true,
	},
}

type DomainAggregate struct {
	Domain *models.CustomDomain
}

func NewDomainAggregate(d *models.CustomDomain) *DomainAggregate {
	return &DomainAggregate{Domain: d}
}

func (da *DomainAggregate) CanTransitionTo(next models.CustomDomainStatus) bool {
	if da.Domain.Status == next {
		return true
	}
	allowed, ok := allowedTransitions[da.Domain.Status][next]
	return ok && allowed
}

func (da *DomainAggregate) MarkHealthy() {
	da.Domain.HealthStatus = models.DomainHealthHealthy
	da.Domain.ErrorCode = ""
	da.Domain.ErrorMessage = ""
	da.Domain.LastHealthcheckAt = pointerTime(time.Now())
}

func (da *DomainAggregate) MarkFailed(errCode string, errMsg string) {
	da.Domain.HealthStatus = models.DomainHealthUnhealthy
	da.Domain.ErrorCode = models.DomainErrorCode(errCode)
	da.Domain.ErrorMessage = errMsg
	da.Domain.LastHealthcheckAt = pointerTime(time.Now())
}

func pointerTime(t time.Time) *time.Time {
	return &t
}
