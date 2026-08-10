package models

// Setting Keys
// Use these constants throughout the application to avoid magic strings
const (
	SettingMaxProjects          = "max_projects_per_user"
	SettingProjectExpiry        = "project_expiry_days"
	SettingCPULimit             = "cpu_limit_percent"
	SettingMemoryLimit          = "memory_limit_mb"
	SettingBaseDomain           = "base_domain"
	SettingProjectDomain        = "project_domain"
	SettingAdminIdleTimeout     = "admin_idle_timeout"
	SettingMaxConcurrent        = "max_concurrent_builds"
	SettingBuildTimeout         = "build_timeout_seconds"
	SettingMaxDomainsPerProject = "max_domains_per_project"
	SettingMaxImageRetention    = "max_image_retention"
	SettingDefaultPaymentProvider = "default_payment_provider"
)

// Default Settings Values
// These are used when a setting is missing from the database
const (
	DefaultMaxProjects          = "3"
	DefaultProjectExpiry        = "30"
	DefaultCPULimit             = "50"
	DefaultMemoryLimit          = "512"
	DefaultAdminIdleTimeout     = "30"
	DefaultMaxConcurrent        = "3"
	DefaultBuildTimeout         = "1800"
	DefaultMaxDomainsPerProject = "3"
	DefaultMaxImageRetention    = "3"
	DefaultPaymentProvider    = "midtrans"
)
