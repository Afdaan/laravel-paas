// ===========================================
// Database Models
// ===========================================
// Defines all database entities using GORM
// ===========================================
package models

import (
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
	ID        uint           `gorm:"primaryKey" json:"id"`
	Email     string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Password  string         `gorm:"size:255;not null" json:"-"` // Never expose password
	Name      string         `gorm:"size:255;not null" json:"name"`
	Role      Role           `gorm:"size:20;not null;default:student" json:"role"`
	CreatedBy *uint          `json:"created_by,omitempty"`
	Creator   *User          `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
	Projects  []Project      `gorm:"foreignKey:UserID" json:"projects,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// ===========================================
// Project Model
// ===========================================

// ProjectStatus represents deployment state
type ProjectStatus string

const (
	StatusPending  ProjectStatus = "pending"
	StatusBuilding ProjectStatus = "building"
	StatusRunning  ProjectStatus = "running"
	StatusFailed   ProjectStatus = "failed"
	StatusStopped  ProjectStatus = "stopped"
)

// Project represents a deployed Laravel application
type Project struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	UserID       uint           `gorm:"not null;index" json:"user_id"`
	User         User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Name         string         `gorm:"size:255;not null" json:"name"`
	GithubURL    string         `gorm:"size:500;not null" json:"github_url"`
	Branch       string         `gorm:"size:200;not null;default:main" json:"branch"`
	Subdomain    string         `gorm:"uniqueIndex;size:100;not null" json:"subdomain"`
	DatabaseName string         `gorm:"uniqueIndex;size:100;not null" json:"database_name"`
	Status       ProjectStatus  `gorm:"size:20;not null;default:pending;index:idx_status_active" json:"status"`
	ContainerID  *string        `gorm:"size:100" json:"container_id,omitempty"`
	Port         *int           `json:"port,omitempty"`
	ErrorLog     *string        `gorm:"type:text" json:"error_log,omitempty"`
	
	// Detected Laravel/PHP versions
	LaravelVersion string `gorm:"size:20" json:"laravel_version,omitempty"`
	PHPVersion     string `gorm:"size:20" json:"php_version,omitempty"`
	IsManualVersion bool  `gorm:"default:false" json:"is_manual_version"`
	QueueEnabled    bool  `gorm:"default:false" json:"queue_enabled"` // Enables worker process
	
	// Resource limits (override defaults)
	CPULimit    *float64 `json:"cpu_limit,omitempty"`
	MemoryLimit *string  `gorm:"size:20" json:"memory_limit,omitempty"`
	
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index:idx_status_active" json:"-"`

	// Virtual field for frontend
	URL string `gorm:"-" json:"url,omitempty"`
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
