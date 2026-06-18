package domain

import "errors"

type DomainErrorCode string

const (
	ErrStaleSequence          DomainErrorCode = "STALE_SEQUENCE"
	ErrInvalidTransition      DomainErrorCode = "INVALID_TRANSITION"
	ErrPollerConflict         DomainErrorCode = "POLLER_CONFLICT"
	ErrVerificationTimeout    DomainErrorCode = "VERIFICATION_TIMEOUT"
	ErrSSLProvisioningFailed  DomainErrorCode = "SSL_PROVISIONING_FAILED"
	ErrDNSResolutionFailed    DomainErrorCode = "DNS_RESOLUTION_FAILED"
	ErrPublicRouteUnreachable DomainErrorCode = "PUBLIC_ROUTE_UNREACHABLE"
	ErrUpstreamTimeout        DomainErrorCode = "UPSTREAM_TIMEOUT"
	ErrIntegrityCheckFailed   DomainErrorCode = "INTEGRITY_CHECK_FAILED"
)

var ErrRuntimeBusy = errors.New("runtime queue busy: execution overloaded")
