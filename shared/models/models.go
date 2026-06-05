// ===========================================
// Database Models
// ===========================================
// Defines all database entities using GORM
// ===========================================
package models

import (
	"fmt"
	"path/filepath"
	"strings"
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
	RoleUser       Role = "user"
)

// User represents a system user (admin or regular user)
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Email        string         `gorm:"uniqueIndex:uni_users_email;size:255;not null" json:"email"`
	Password     string         `gorm:"size:255;not null" json:"-"` // Never expose password
	Name         string         `gorm:"size:255;not null" json:"name"`
	Role         Role           `gorm:"size:20;not null;default:user" json:"role"`
	CreatedBy    *uint          `json:"created_by,omitempty"`
	Creator      *User          `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Projects     []Project      `gorm:"foreignKey:UserID" json:"projects,omitempty"`
	LastLogin    *time.Time     `json:"last_login,omitempty"`
	LastActivity *time.Time     `json:"last_activity,omitempty"`
	LastIP       string         `gorm:"size:45" json:"last_ip,omitempty"`
	LastLocation string         `gorm:"size:255" json:"last_location,omitempty"`
	AvatarURL    string         `gorm:"size:500" json:"avatar_url,omitempty"`
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
	UserSlug              string           `gorm:"size:255;not null;default:'user-unknown'" json:"user_slug"`
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

	// Health Check Configuration
	HealthCheckPath string `gorm:"size:255" json:"health_check_path"` // Custom health check path (default: "/" for most, "/health" for Laravel)

	// GitHub Integration (Git Connect)
	GithubInstallationID *int64 `gorm:"index" json:"github_installation_id,omitempty"`
	GithubRepoOwner      string `gorm:"size:255" json:"github_repo_owner,omitempty"`
	GithubRepoName       string `gorm:"size:255" json:"github_repo_name,omitempty"`

	CustomDomains    []CustomDomain    `gorm:"foreignKey:ProjectID" json:"custom_domains,omitempty"`
	DatabaseInstance *DatabaseInstance `gorm:"foreignKey:ProjectID" json:"database_instance,omitempty"`
}

// GetInternalPort returns the target port for Traefik routing
func (p *Project) GetInternalPort() string {
	if p.Port != nil {
		return fmt.Sprintf("%d", *p.Port)
	}
	if p.Framework == "Laravel" {
		return "80"
	}
	return "8080"
}

// GetHealthCheckPath returns the configured health check path, or "/" for most frameworks ("/health" for Laravel).
func (p *Project) GetHealthCheckPath() string {
	if p.HealthCheckPath != "" {
		return p.HealthCheckPath
	}
	if p.Framework == "Laravel" {
		return "/health"
	}
	return "/"
}

// GetProjectPath returns the multi-tenant directory path for the project on the host or inside the workspace.
func (p *Project) GetProjectPath(basePath string) string {
	userFolder := p.UserSlug
	if userFolder == "" || userFolder == "user-unknown" {
		userFolder = fmt.Sprintf("user-%d", p.UserID)
	}
	return filepath.Join(basePath, userFolder, p.Subdomain)
}

// BeforeCreate hooks into GORM's creation cycle to auto-populate UserSlug.
func (p *Project) BeforeCreate(tx *gorm.DB) error {
	var user User
	if err := tx.Select("name, email").First(&user, p.UserID).Error; err == nil {
		slug := NormalizeSlug(user.Name)
		if slug == "" {
			parts := strings.Split(user.Email, "@")
			if len(parts) > 0 {
				slug = NormalizeSlug(parts[0])
			}
		}
		if slug == "" {
			slug = "user"
		}
		p.UserSlug = fmt.Sprintf("%s-%d", slug, p.UserID)
	} else {
		p.UserSlug = fmt.Sprintf("user-%d", p.UserID)
	}
	return nil
}

// NormalizeSlug converts a string (like name or email prefix) to a clean, safe slug.
func NormalizeSlug(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			sb.WriteByte(c)
		} else {
			sb.WriteByte('-')
		}
	}
	res := sb.String()
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	res = strings.Trim(res, "-")
	return res
}

// GetUserDirName retrieves a clean, human-readable directory name for a user (e.g., "afdaan-1").
func GetUserDirName(db *gorm.DB, userID uint) string {
	defaultName := fmt.Sprintf("user-%d", userID)
	if db == nil {
		return defaultName
	}

	var user User
	if err := db.Select("name, email").First(&user, userID).Error; err != nil {
		return defaultName
	}

	slug := NormalizeSlug(user.Name)
	if slug == "" {
		parts := strings.Split(user.Email, "@")
		if len(parts) > 0 {
			slug = NormalizeSlug(parts[0])
		}
	}
	if slug == "" {
		slug = "user"
	}

	return fmt.Sprintf("%s-%d", slug, userID)
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
		if IsNginxRoutableCustomDomainStatus(cd.Status) {
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
	ErrPublicRouteUnreachable DomainErrorCode = "public_route_unreachable"
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
	DesiredStatus CustomDomainStatus `gorm:"size:30;not null;default:active" json:"desired_status"`

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

	// Derived Operational Health Overlay (DOMAIN -> DNS -> PUBLIC ACCESS -> APP HEALTH)
	HealthStatus      DomainHealthStatus `gorm:"size:20;default:'unknown'" json:"health_status"`
	LastHealthcheckAt *time.Time         `json:"last_healthcheck_at,omitempty"`
	LatencyMs         int64              `json:"latency_ms"`
	ConfigHash        string             `gorm:"size:64" json:"config_hash,omitempty"`
	CurrentSequence   int                `gorm:"default:0" json:"current_sequence"`
	SnapshotVersion   int                `gorm:"default:0" json:"snapshot_version"`

	// Granular Layered Health Validation
	Layer1DNSReachable           bool `gorm:"default:false" json:"layer1_dns_reachable"`
	Layer2PublicAccessReachable  bool `gorm:"default:false" json:"layer2_public_access_reachable"`
	Layer3SSLValid               bool `gorm:"default:false" json:"layer3_ssl_valid"`
	Layer4UpstreamReachable      bool `gorm:"default:false" json:"layer4_upstream_reachable"`
	Layer5ResponseIntegrity      bool `gorm:"default:false" json:"layer5_response_integrity"`

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
	EventVersion   int                `gorm:"default:1" json:"event_version"`
	CreatedAt      time.Time          `gorm:"index" json:"created_at"`
}

// ===========================================
// DeploymentEvent Model
// ===========================================

// DeploymentEvent tracks granular lifecycle transitions and audit events
type DeploymentEvent struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ProjectID      uint      `gorm:"not null;uniqueIndex:uni_project_job_seq" json:"project_id"`
	JobID          string    `gorm:"size:100;uniqueIndex:uni_project_job_seq" json:"job_id"`
	SequenceNumber int       `gorm:"not null;default:0;uniqueIndex:uni_project_job_seq" json:"sequence_number"`
	WorkerID       string    `gorm:"size:100" json:"worker_id"`
	StateFrom      string    `gorm:"size:50" json:"state_from"`
	StateTo        string    `gorm:"size:50" json:"state_to"`
	EventType      string    `gorm:"size:100;not null;index" json:"event_type"`
	Payload        string    `gorm:"type:text" json:"payload"`
	DurationMs     int64     `json:"duration_ms"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}

// ===========================================
// OutboxEvent Model
// ===========================================

// OutboxEvent tracks transactional outbox messages to ensure clean at-least-once pub/sub delivery.
type OutboxEvent struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	EventType  string    `gorm:"size:100;not null;index" json:"event_type"`
	DomainID   uint      `gorm:"not null;index" json:"domain_id"`
	ProjectID  uint      `gorm:"not null" json:"project_id"`
	Sequence   int       `gorm:"not null" json:"sequence"`
	Payload    []byte    `gorm:"type:text;not null" json:"payload"`
	Published  bool      `gorm:"default:false;index" json:"published"`
	RetryCount int       `gorm:"default:0" json:"retry_count"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

// ===========================================
// IdempotentOperation Model
// ===========================================

// IdempotentOperation represents a request tracking entry to guarantee idempotency across restarts and retries.
type IdempotentOperation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Key        string    `gorm:"size:255;uniqueIndex;not null" json:"key"`
	Operation  string    `gorm:"size:100;index;not null" json:"operation"`
	ResourceID uint      `gorm:"index;not null" json:"resource_id"`
	Status     string    `gorm:"size:50;index;not null" json:"status"`
	Response   []byte    `gorm:"type:text" json:"response"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ===========================================
// PendingReconcile Model
// ===========================================

// PendingReconcile tracks scheduled or enqueued domain reconciliation items durably in the database.
type PendingReconcile struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	DomainID   uint       `gorm:"not null;uniqueIndex" json:"domain_id"`
	RetryCount int        `gorm:"default:0" json:"retry_count"`
	RunAfter   time.Time  `gorm:"index" json:"run_after"`
	Priority   int        `gorm:"default:0;index" json:"priority"`
	Cause      string     `gorm:"size:100" json:"cause"`
	Partition  int        `gorm:"default:0;index" json:"partition"`
	LockedBy   *string    `gorm:"size:100" json:"locked_by"`
	LockedAt   *time.Time `json:"locked_at"`
}

// ===========================================
// AuditLog Model
// ===========================================

// AuditLog tracks granular, append-only domain state transitions and background verification traces.
type AuditLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	DomainID     uint      `gorm:"index;not null" json:"domain_id"`
	Operation    string    `gorm:"size:100;index;not null" json:"operation"`
	StateFrom    string    `gorm:"size:50" json:"state_from"`
	StateTo      string    `gorm:"size:50" json:"state_to"`
	Status       string    `gorm:"size:50;not null" json:"status"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
	TraceID      string    `gorm:"size:100;index" json:"trace_id"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// ===========================================
// GithubAppInstallation Model
// ===========================================

// GithubAppInstallation maps a user's authenticated GitHub organization/account.
type GithubAppInstallation struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	InstallationID int64     `gorm:"uniqueIndex;not null" json:"installation_id"`
	AccountName    string    `gorm:"size:255;not null;index:idx_gh_install_acc" json:"account_name"`
	AvatarURL      string    `gorm:"size:500" json:"avatar_url"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ===========================================
// DatabaseInstance Model
// ===========================================

// DatabaseInstanceStatus represents the lifecycle state of a managed database instance.
type DatabaseInstanceStatus string

const (
	DBStatusActive    DatabaseInstanceStatus = "active"
	DBStatusSuspended DatabaseInstanceStatus = "suspended"
	DBStatusDeleted   DatabaseInstanceStatus = "deleted"
)

// DatabaseInstance represents a managed database provisioned for a user project.
// Each project has at most one database instance. The engine field determines
// which container and driver handles provisioning and queries.
type DatabaseInstance struct {
	ID                 uint                   `gorm:"primaryKey" json:"id"`
	ProjectID          uint                   `gorm:"uniqueIndex:uni_database_instances_project_id;not null" json:"project_id"`
	Project            Project                `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Engine             string                 `gorm:"size:20;not null;default:mysql" json:"engine"` // "mysql" or "postgresql"
	Version            string                 `gorm:"size:50" json:"version,omitempty"`
	Status             DatabaseInstanceStatus `gorm:"size:20;not null;default:active" json:"status"`
	Name               string                 `gorm:"size:100;not null" json:"name"`
	Username           string                 `gorm:"size:100;not null" json:"username"`
	Password           string                 `gorm:"size:255;not null" json:"-"` // Never expose in JSON
	Host               string                 `gorm:"size:255;not null" json:"host"`
	Port               int                    `gorm:"not null" json:"port"`
	StorageAllocation  int64                  `gorm:"default:1073741824" json:"storage_allocation"` // Default 1GB in bytes
	StorageConsumption int64                  `gorm:"default:0" json:"storage_consumption"`
	ConnectionCount    int                    `gorm:"default:0" json:"connection_count"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// ===========================================
// DatabaseBackup Model
// ===========================================

// DatabaseBackupStatus represents the state of a database backup job.
type DatabaseBackupStatus string

const (
	BackupStatusPending   DatabaseBackupStatus = "pending"
	BackupStatusCompleted DatabaseBackupStatus = "completed"
	BackupStatusFailed    DatabaseBackupStatus = "failed"
)

// DatabaseBackup tracks point-in-time SQL snapshots for a database instance.
// A strict retention policy of 5 backups per database is enforced at creation time.
type DatabaseBackup struct {
	ID                 uint                 `gorm:"primaryKey" json:"id"`
	DatabaseInstanceID uint                 `gorm:"not null;index" json:"database_instance_id"`
	DatabaseInstance   DatabaseInstance     `gorm:"foreignKey:DatabaseInstanceID" json:"database_instance,omitempty"`
	ProjectID          uint                 `gorm:"not null;index" json:"project_id"`
	Name               string               `gorm:"size:255;not null" json:"name"`
	Path               string               `gorm:"size:500;not null" json:"path"`
	Size               string               `gorm:"size:50" json:"size"`
	Status             DatabaseBackupStatus `gorm:"size:20;not null;default:pending" json:"status"`
	CreatedAt          time.Time            `json:"created_at"`
}

// ===========================================
// SecretStore Models
// ===========================================

// SecretStore represents a credentials container that can be linked to multiple projects.
type SecretStore struct {
	ID          uint                 `gorm:"primaryKey" json:"id"`
	UserID      uint                 `gorm:"not null;index" json:"user_id"`
	User        User                 `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name        string               `gorm:"size:255;not null" json:"name"`
	Description string               `gorm:"size:500" json:"description,omitempty"`
	IsDisabled  bool                 `gorm:"default:false" json:"is_disabled"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	DeletedAt   gorm.DeletedAt       `gorm:"index" json:"-"`
	Items       []SecretStoreItem    `gorm:"foreignKey:SecretStoreID" json:"items,omitempty"`
	Bindings    []SecretStoreBinding `gorm:"foreignKey:SecretStoreID" json:"bindings,omitempty"`
}

// SecretStoreItem represents an environment variable key within a SecretStore.
type SecretStoreItem struct {
	ID                    uint                   `gorm:"primaryKey" json:"id"`
	SecretStoreID         uint                   `gorm:"not null;index" json:"secret_store_id"`
	SecretStore           SecretStore            `gorm:"foreignKey:SecretStoreID" json:"-"`
	Key                   string                 `gorm:"size:255;not null;index" json:"key"`
	LatestSnapshotVersion int                    `gorm:"default:1" json:"latest_snapshot_version"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
	DeletedAt             gorm.DeletedAt         `gorm:"index" json:"-"`
	Values                []SecretStoreItemValue `gorm:"foreignKey:SecretStoreItemID" json:"values,omitempty"`
}

// SecretStoreItemValue stores the encrypted credentials of a secret item version.
type SecretStoreItemValue struct {
	ID                uint            `gorm:"primaryKey" json:"id"`
	SecretStoreItemID uint            `gorm:"not null;index" json:"secret_store_item_id"`
	SecretStoreItem   SecretStoreItem `gorm:"foreignKey:SecretStoreItemID" json:"-"`
	Version           int             `gorm:"not null;default:1" json:"version"`
	EncryptedValue    string          `gorm:"type:text;not null" json:"-"`
	CreatedBy         uint            `gorm:"not null" json:"created_by"`
	CreatedAt         time.Time       `json:"created_at"`
}

// SecretStoreBinding binds a SecretStore container to a project environment.
type SecretStoreBinding struct {
	ID            uint        `gorm:"primaryKey" json:"id"`
	ProjectID     uint        `gorm:"not null;index" json:"project_id"`
	Project       Project     `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	SecretStoreID uint        `gorm:"not null;index" json:"secret_store_id"`
	SecretStore   SecretStore `gorm:"foreignKey:SecretStoreID" json:"secret_store,omitempty"`
	Environment   string      `gorm:"size:50;not null;default:'production'" json:"environment"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// SecretStoreActivityLog logs auditing actions on SecretStores.
type SecretStoreActivityLog struct {
	ID                uint             `gorm:"primaryKey" json:"id"`
	SecretStoreID     *uint            `gorm:"index" json:"secret_store_id,omitempty"`
	SecretStore       *SecretStore     `gorm:"foreignKey:SecretStoreID" json:"secret_store,omitempty"`
	SecretStoreItemID *uint            `gorm:"index" json:"secret_store_item_id,omitempty"`
	SecretStoreItem   *SecretStoreItem `gorm:"foreignKey:SecretStoreItemID" json:"secret_store_item,omitempty"`
	UserID            uint             `gorm:"not null;index" json:"user_id"`
	User              User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ProjectID         *uint            `gorm:"index" json:"project_id,omitempty"`
	Project           *Project         `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Action            string           `gorm:"size:100;not null" json:"action"`
	IpAddress         string           `gorm:"size:45" json:"ip_address"`
	UserAgent         string           `gorm:"size:500" json:"user_agent"`
	Details           string           `gorm:"type:text" json:"details"`
	CreatedAt         time.Time        `json:"created_at"`
}
