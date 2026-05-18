// ===========================================
// Database Models
// ===========================================
// Defines all database entities using GORM
// ===========================================
package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ===========================================
// User Model
// ===========================================

// Role represents user permission level
type Role string

const (
	RoleSuperAdmin Role = "superadmin"
	RoleAdmin      Role = "admin"
	RoleStudent    Role = "student"
)

// User represents a system user (admin or student)
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Email        string         `gorm:"uniqueIndex:uni_users_email;size:255;not null" json:"email"`
	Password     string         `gorm:"size:255;not null" json:"-"` // Never expose password
	Name         string         `gorm:"size:255;not null" json:"name"`
	Role         Role           `gorm:"size:20;not null;default:student" json:"role"`
	CreatedBy    *uint          `json:"created_by,omitempty"`
	Creator      *User          `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Projects     []Project      `gorm:"foreignKey:UserID" json:"projects,omitempty"`
	LastLogin    *time.Time     `json:"last_login,omitempty"`
	LastActivity *time.Time     `json:"last_activity,omitempty"`
	LastIP       string         `gorm:"size:45" json:"last_ip,omitempty"`
	LastLocation string         `gorm:"size:255" json:"last_location,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// ===========================================
// Project Model
// ===========================================

// ProjectStatus represents runtime deployment state
type ProjectStatus string

const (
	StatusPending        ProjectStatus = "pending"
	StatusQueued         ProjectStatus = "queued"
	StatusPreparing      ProjectStatus = "preparing"
	StatusCloning        ProjectStatus = "cloning"
	StatusBuilding       ProjectStatus = "building"
	StatusProvisioning   ProjectStatus = "provisioning"
	StatusStarting       ProjectStatus = "starting"
	StatusHealthchecking ProjectStatus = "healthchecking"
	StatusMigrating      ProjectStatus = "migrating"
	StatusPromoting      ProjectStatus = "promoting"
	StatusCleanup        ProjectStatus = "cleanup"
	StatusCompleted      ProjectStatus = "completed"
	StatusRunning        ProjectStatus = "running"
	StatusFailed         ProjectStatus = "failed"
	StatusCancelled      ProjectStatus = "cancelled"
	StatusRollback       ProjectStatus = "rollback"
	StatusDeleting       ProjectStatus = "deleting"
	StatusStopped        ProjectStatus = "stopped"
	StatusRestarting     ProjectStatus = "restarting"
)

// DeploymentStatus represents deployment execution state
type DeploymentStatus string

const (
	DepStatusQueued         DeploymentStatus = "queued"
	DepStatusPreparing      DeploymentStatus = "preparing"
	DepStatusCloning        DeploymentStatus = "cloning"
	DepStatusBuilding       DeploymentStatus = "building"
	DepStatusProvisioning   DeploymentStatus = "provisioning"
	DepStatusStarting       DeploymentStatus = "starting"
	DepStatusHealthchecking DeploymentStatus = "healthchecking"
	DepStatusMigrating      DeploymentStatus = "migrating"
	DepStatusPromoting      DeploymentStatus = "promoting"
	DepStatusCleanup        DeploymentStatus = "cleanup"
	DepStatusCompleted      DeploymentStatus = "completed"
	DepStatusFailed         DeploymentStatus = "failed"
	DepStatusRollback       DeploymentStatus = "rollback"
	DepStatusCancelled      DeploymentStatus = "cancelled"
)

// Project represents a deployed Laravel application
type Project struct {
	ID                    uint             `gorm:"primaryKey" json:"id"`
	UserID                uint             `gorm:"not null;index" json:"user_id"`
	User                  User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name                  string           `gorm:"size:255;not null" json:"name"`
	GithubURL             string           `gorm:"size:500;not null" json:"github_url"`
	Branch                string           `gorm:"size:200;not null;default:main" json:"branch"`
	Subdomain             string           `gorm:"uniqueIndex:uni_projects_subdomain;size:100;not null" json:"subdomain"`
	DatabaseName          string           `gorm:"uniqueIndex:uni_projects_database_name;size:100;not null" json:"database_name"`
	DatabasePassword      string           `gorm:"size:255;not null;default:''" json:"-"` // Never expose in JSON
	Status                ProjectStatus    `gorm:"size:20;not null;default:pending;index:idx_status_active" json:"status"`
	DeploymentStatus      DeploymentStatus `gorm:"size:30;not null;default:completed;index:idx_dep_status" json:"deployment_status"`
	DeploymentJobID       *string          `gorm:"size:100;index" json:"deployment_job_id,omitempty"`
	RolloutContainerID    *string          `gorm:"size:100" json:"rollout_container_id,omitempty"`
	DeploymentStartedAt   *time.Time       `json:"deployment_started_at,omitempty"`
	DeploymentFinishedAt  *time.Time       `json:"deployment_finished_at,omitempty"`
	DeploymentHeartbeatAt *time.Time       `json:"deployment_heartbeat_at,omitempty"`
	DeploymentMessage     *string          `gorm:"type:text" json:"deployment_message,omitempty"`
	DeploymentProgress    int              `gorm:"default:0" json:"deployment_progress"`
	ContainerID           *string          `gorm:"size:100" json:"container_id,omitempty"`
	Port                  *int             `json:"port,omitempty"`
	BaseDirectory         string           `gorm:"size:255" json:"base_directory,omitempty"` // Custom build root
	ErrorLog              *string          `gorm:"type:text" json:"error_log,omitempty"`
	LastCommitHash        string           `gorm:"size:100" json:"last_commit_hash,omitempty"`

	// Detected Laravel/PHP versions
	LaravelVersion    string  `gorm:"size:20" json:"laravel_version,omitempty"`
	PHPVersion        string  `gorm:"size:20" json:"php_version,omitempty"`
	Framework         string  `gorm:"size:50" json:"framework,omitempty"`
	LanguageVersion   string  `gorm:"size:20" json:"language_version,omitempty"`
	IsManualVersion   bool    `gorm:"default:false" json:"is_manual_version"`
	QueueEnabled      bool    `gorm:"default:false" json:"queue_enabled"` // Enables worker process
	WorkerCommand     string  `gorm:"size:500" json:"worker_command"`     // Custom command for background service (non-PHP)
	WorkerContainerID *string `gorm:"size:100" json:"worker_container_id,omitempty"`

	// Custom Build/Run Commands
	BuildCommand string `gorm:"size:500" json:"build_command"` // Custom build step (e.g. npm run build)
	StartCommand string `gorm:"size:500" json:"start_command"` // Custom start command (e.g. node dist/main.js)
	NodeVersion  string `gorm:"size:20" json:"node_version"`   // Specific Node.js version (e.g. 18, 20)

	// Resource limits (override defaults)
	CPULimit    *float64 `json:"cpu_limit,omitempty"`
	MemoryLimit *string  `gorm:"size:20" json:"memory_limit,omitempty"`

	LastAccessedAt *time.Time     `json:"last_accessed_at,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index:idx_status_active" json:"-"`

	// Virtual field for frontend
	URL string `gorm:"-" json:"url,omitempty"`
	UID string `gorm:"uniqueIndex:uni_projects_uid;size:100" json:"uid"`

	// Config Hashing
	ConfigHash string `gorm:"size:64" json:"config_hash,omitempty"`

	CustomDomains []CustomDomain `gorm:"foreignKey:ProjectID" json:"custom_domains,omitempty"`
}

// ===========================================
// Setting Model
// ===========================================

// Setting represents a configurable system setting
type Setting struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Key         string `gorm:"column:setting_key;uniqueIndex:idx_settings_key;size:100;not null" json:"key"`
	Value       string `gorm:"type:text;not null" json:"value"`
	Description string `gorm:"size:500" json:"description,omitempty"`
	Type        string `gorm:"size:20;default:string" json:"type"` // string, int, bool
}

// ===========================================
// ResourceLog Model
// ===========================================

// ResourceLog tracks CPU/memory usage over time
type ResourceLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProjectID  uint      `gorm:"not null;index" json:"project_id"`
	Project    Project   `gorm:"foreignKey:ProjectID" json:"-"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   float64   `json:"memory_mb"`
	RecordedAt time.Time `gorm:"index" json:"recorded_at"`
}

// ===========================================
// Helper Methods
// ===========================================

// IsAdmin checks if user has admin privileges
func (u *User) IsAdmin() bool {
	return u.Role == RoleSuperAdmin || u.Role == RoleAdmin
}

// IsSuperAdmin checks if user is superadmin
func (u *User) IsSuperAdmin() bool {
	return u.Role == RoleSuperAdmin
}

// GetFullDomain returns complete project URL
func (p *Project) GetFullDomain(baseDomain string) string {
	return p.Subdomain + "." + baseDomain
}

// GetTraefikHostRule returns the Traefik host rule including all active custom domains
func (p *Project) GetTraefikHostRule(projectDomain string) string {
	// Start with the primary domain
	rule := fmt.Sprintf("Host(`%s.%s`)", p.Subdomain, projectDomain)

	// Append active custom domains
	for _, cd := range p.CustomDomains {
		if cd.Status == DomainStatusActive {
			rule += fmt.Sprintf(" || Host(`%s`)", cd.Domain)
		}
	}
	return rule
}

// ===========================================
// Feedback Model
// ===========================================

// FeedbackType represents kind of feedback
type FeedbackType string

const (
	FeedbackSuggestion FeedbackType = "suggestion"
	FeedbackTrouble    FeedbackType = "trouble"
	FeedbackBug        FeedbackType = "bug"
)

// FeedbackStatus represents status of feedback
type FeedbackStatus string

const (
	FeedbackStatusPending  FeedbackStatus = "pending"
	FeedbackStatusInReview FeedbackStatus = "in_review"
	FeedbackStatusResolved FeedbackStatus = "resolved"
)

// Feedback represents user feedback or bug report
type Feedback struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"not null;index" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Title     string         `gorm:"size:255;not null" json:"title"`
	Content   string         `gorm:"type:text;not null" json:"content"`
	Type      FeedbackType   `gorm:"size:20;not null;default:suggestion" json:"type"`
	Status    FeedbackStatus `gorm:"size:20;not null;default:pending" json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ===========================================
// CustomDomain Model
// ===========================================

// CustomDomainStatus represents the deterministic lifecycle state of a custom domain.
type CustomDomainStatus string

const (
	DomainStatusPending            CustomDomainStatus = "pending"             // Initial registration; awaiting DNS setup.
	DomainStatusPendingDNS         CustomDomainStatus = "pending_dns"         // DNS verification initiated but propagation not yet observed.
	DomainStatusDNSVerified        CustomDomainStatus = "dns_verified"        // DNS verified successfully; ready for SSL queueing.
	DomainStatusSSLQueued          CustomDomainStatus = "ssl_queued"          // Enqueued for Let's Encrypt certificate issuance.
	DomainStatusSSLProvisioning    CustomDomainStatus = "ssl_provisioning"    // Active HTTP-01 challenge verification in progress on edge Nginx.
	DomainStatusSSLActive          CustomDomainStatus = "ssl_active"          // Certificate issued and verified; Nginx configured with HTTPS.
	DomainStatusActive             CustomDomainStatus = "active"              // Full operational readiness (DNS, SSL, and Nginx edge routing verified).
	DomainStatusPropagationPending CustomDomainStatus = "propagation_pending" // Transitioning or propagating after DNS/routing changes.
	DomainStatusDegraded           CustomDomainStatus = "degraded"            // Partial failure (e.g., SSL near expiry or transient validation timeout).
	DomainStatusRenewalPending     CustomDomainStatus = "renewal_pending"     // Proactively scheduled Let's Encrypt renewal in progress (<=21d remaining).
	DomainStatusRenewalFailed      CustomDomainStatus = "renewal_failed"      // Renewal attempts exhausted without issuance; requires intervention.
	DomainStatusSSLFailed          CustomDomainStatus = "ssl_failed"          // Initial Let's Encrypt challenge failed after exponential retries.
	DomainStatusPendingCleanup     CustomDomainStatus = "pending_cleanup"     // Domain scheduled for removal; awaiting orphaned cert/config teardown.
	DomainStatusDisabled           CustomDomainStatus = "disabled"            // Explicitly paused by user or admin; edge routing disabled.
	DomainStatusError              CustomDomainStatus = "error"               // Generic or unrecoverable error state.
)

// IsNginxRoutableCustomDomainStatus returns true after DNS ownership has been verified.
// Edge config must keep these domains routable while SSL and health reconciliation continue.
func IsNginxRoutableCustomDomainStatus(status CustomDomainStatus) bool {
	switch status {
	case DomainStatusDNSVerified,
		DomainStatusSSLQueued,
		DomainStatusSSLProvisioning,
		DomainStatusSSLActive,
		DomainStatusActive,
		DomainStatusPropagationPending,
		DomainStatusDegraded,
		DomainStatusRenewalPending,
		DomainStatusRenewalFailed:
		return true
	default:
		return false
	}
}

// DomainHealthStatus represents the derived operational health overlay.
type DomainHealthStatus string

const (
	DomainHealthHealthy   DomainHealthStatus = "healthy"
	DomainHealthUnhealthy DomainHealthStatus = "unhealthy"
	DomainHealthUnknown   DomainHealthStatus = "unknown"
)

// DomainErrorCode represents deterministic, machine-readable failure classifications.
type DomainErrorCode string

const (
	ErrNone                  DomainErrorCode = ""
	ErrDNSConflict           DomainErrorCode = "DNS_CONFLICT"
	ErrDomainNotResolved     DomainErrorCode = "DOMAIN_NOT_RESOLVED"
	ErrInvalidCNAMETarget    DomainErrorCode = "INVALID_CNAME_TARGET"
	ErrSSLRateLimit          DomainErrorCode = "SSL_RATE_LIMIT"
	ErrSSLIssuanceFailed     DomainErrorCode = "SSL_ISSUANCE_FAILED"
	ErrCertbotTimeout        DomainErrorCode = "CERTBOT_TIMEOUT"
	ErrNginxValidationFailed DomainErrorCode = "NGINX_VALIDATION_FAILED"
	ErrRoutingHealthFailed   DomainErrorCode = "ROUTING_HEALTH_FAILED"
	ErrLockAcquisitionFailed DomainErrorCode = "LOCK_ACQUISITION_FAILED"

	// Explicit Degraded Reason Codes
	ErrRedisUnavailable      DomainErrorCode = "redis_unavailable"
	ErrNginxReloadFailed     DomainErrorCode = "nginx_reload_failed"
	ErrSSLExpired            DomainErrorCode = "ssl_expired"
	ErrEdgeUnreachable       DomainErrorCode = "edge_unreachable"
	ErrUpstreamTimeout       DomainErrorCode = "upstream_timeout"
	ErrIntegrityCheckFailed  DomainErrorCode = "integrity_check_failed"
	ErrReconciliationStalled DomainErrorCode = "reconciliation_stalled"
)

// CustomDomain represents a custom domain mapped to a project, tracking both its lifecycle state and operational health overlay.
type CustomDomain struct {
	ID        uint               `gorm:"primaryKey" json:"id"`
	ProjectID uint               `gorm:"not null;index" json:"project_id"`
	Project   Project            `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Domain    string             `gorm:"uniqueIndex:uni_custom_domains_domain;size:255;not null" json:"domain"`
	Status    CustomDomainStatus `gorm:"size:30;not null;default:pending" json:"status"`

	// SSL & Certificate Lifecycle
	SSLStatus            string     `gorm:"size:50;default:'none'" json:"ssl_status"`
	SSLIssuedAt          *time.Time `json:"ssl_issued_at,omitempty"`
	SSLExpiresAt         *time.Time `json:"ssl_expires_at,omitempty"`
	LastRenewalAttemptAt *time.Time `json:"last_renewal_attempt_at,omitempty"`
	RenewalRetryCount    int        `gorm:"default:0" json:"renewal_retry_count"`

	// Verification & Reconciliation
	LastVerificationAt     *time.Time `json:"last_verification_at,omitempty"`
	LastReconciliationAt   *time.Time `json:"last_reconciliation_at,omitempty"`
	VerificationRetryCount int        `gorm:"default:0" json:"verification_retry_count"`

	// Derived Operational Health Overlay (DOMAIN -> DNS -> EDGE -> APP HEALTH)
	HealthStatus      DomainHealthStatus `gorm:"size:20;default:'unknown'" json:"health_status"`
	LastHealthcheckAt *time.Time         `json:"last_healthcheck_at,omitempty"`
	LatencyMs         int64              `json:"latency_ms"`
	ConfigHash        string             `gorm:"size:64" json:"config_hash,omitempty"`
	CurrentSequence   int                `gorm:"default:0" json:"current_sequence"`

	// Granular Layered Health Validation
	Layer1DNSReachable      bool `gorm:"default:false" json:"layer1_dns_reachable"`
	Layer2EdgeReachable     bool `gorm:"default:false" json:"layer2_edge_reachable"`
	Layer3SSLValid          bool `gorm:"default:false" json:"layer3_ssl_valid"`
	Layer4UpstreamReachable bool `gorm:"default:false" json:"layer4_upstream_reachable"`
	Layer5ResponseIntegrity bool `gorm:"default:false" json:"layer5_response_integrity"`

	// Staged Cleanup & Tombstone Metadata
	CleanupRetryCount int    `gorm:"default:0" json:"cleanup_retry_count"`
	CleanupCheckpoint string `gorm:"size:50;default:'init'" json:"cleanup_checkpoint"`

	// Resumable Checkpoint Metadata
	ProvisioningCheckpoint string `gorm:"size:50;default:'init'" json:"provisioning_checkpoint"`
	RenewalCheckpoint      string `gorm:"size:50;default:'init'" json:"renewal_checkpoint"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Structured Error Tracking
	ErrorCode          DomainErrorCode `gorm:"size:50" json:"error_code,omitempty"`
	DegradedReasonCode DomainErrorCode `gorm:"size:50" json:"degraded_reason_code,omitempty"`
	ErrorMessage       string          `gorm:"type:text" json:"error_message,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ===========================================
// DomainEvent Model
// ===========================================

// DomainEvent tracks granular custom domain lifecycle transitions and audit logs for observability and realtime delivery.
type DomainEvent struct {
	ID             uint               `gorm:"primaryKey" json:"id"`
	DomainID       uint               `gorm:"not null;index" json:"domain_id"`
	JobID          string             `gorm:"size:100;index" json:"job_id"`
	SequenceNumber int                `gorm:"not null;default:0;index" json:"sequence_number"`
	StateFrom      CustomDomainStatus `gorm:"size:50" json:"state_from"`
	StateTo        CustomDomainStatus `gorm:"size:50" json:"state_to"`
	EventType      string             `gorm:"size:100;not null;index" json:"event_type"` // e.g., "dns_verified", "ssl_issued", "healthcheck_failed"
	Payload        string             `gorm:"type:text" json:"payload"`
	DurationMs     int64              `json:"duration_ms"`
	Error          string             `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time          `gorm:"index" json:"created_at"`
}

// ===========================================
// DeploymentEvent Model
// ===========================================

// DeploymentEvent tracks granular lifecycle transitions and audit events
type DeploymentEvent struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProjectID      uint      `gorm:"not null;index:idx_project_job_seq" json:"project_id"`
	JobID          string    `gorm:"size:100;index:idx_project_job_seq" json:"job_id"`
	SequenceNumber int       `gorm:"not null;default:0;index:idx_project_job_seq" json:"sequence_number"`
	WorkerID       string    `gorm:"size:100" json:"worker_id"`
	StateFrom      string    `gorm:"size:50" json:"state_from"`
	StateTo        string    `gorm:"size:50" json:"state_to"`
	EventType      string    `gorm:"size:100;not null;index" json:"event_type"`
	Payload        string    `gorm:"type:text" json:"payload"`
	DurationMs     int64     `json:"duration_ms"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}
