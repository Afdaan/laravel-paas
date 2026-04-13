package models

// Setting Keys
// Use these constants throughout the application to avoid magic strings
const (
	SettingMaxProjects      = "max_projects_per_user"
	SettingProjectExpiry    = "project_expiry_days"
	SettingCPULimit         = "cpu_limit_percent"
	SettingMemoryLimit      = "memory_limit_mb"
	SettingBaseDomain       = "base_domain"
	SettingProjectDomain    = "project_domain"
	SettingAdminIdleTimeout = "admin_idle_timeout"
	SettingMaxConcurrent    = "max_concurrent_builds"
)

// Default Settings Values
// These are used when a setting is missing from the database
const (
	DefaultMaxProjects      = "3"
	DefaultProjectExpiry    = "30"
	DefaultCPULimit         = "50"
	DefaultMemoryLimit      = "512"
	DefaultAdminIdleTimeout = "30"
	DefaultMaxConcurrent    = "3"
)
