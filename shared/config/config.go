// ===========================================
// Configuration Package
// ===========================================
// Loads and manages application configuration
// ===========================================
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all application configuration
type Config struct {
	// App
	AppMode      string // "local" or "docker"
	HostRootPath string // Root directory on the host machine
	AppEnv       string
	AppDebug     bool

	// Infra Database (PostgreSQL)
	PGHost     string
	PGPort     string
	PGDatabase string
	PGUser     string
	PGPassword string

	// User Database (MySQL)
	MYSQLHost         string
	MYSQLPort         string
	MYSQLDatabase     string
	MYSQLUser         string
	MYSQLPassword     string
	MYSQLRootPassword string

	// User Database (PostgreSQL Engine)
	UserPGHost     string
	UserPGPort     string
	UserPGPassword string

	// JWT
	JWTSecret      string
	JWTExpiryHours int

	// UID Obfuscation
	UIDSalt string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// Domain
	BaseDomain    string
	ProjectDomain string
	FrontendURL   string
	ACMEEmail     string

	// Docker
	DockerSocket      string
	ProjectsPath      string
	DataPath          string
	HostProjectsPath  string
	HostDataPath      string
	HostTemplatesPath string
	TemplatesPath     string
	DockerNetwork     string
	TraefikDynamicDir string
	HostRailpacksPath string
	RailpacksPath     string

	// Nginx Remote Webhook
	NginxWebhookEnabled       bool
	NginxWebhookURL           string
	NginxWebhookKey           string
	InternalIP                string
	IntegrityValidationMarker string
	TraefikHTTPPort           int

	// GitHub App
	GithubAppID             string
	GithubAppPrivateKeyPath string
	GithubAppWebhookSecret  string

	// SecretStore Credentials Encryption
	CredentialEncryptionKey string
}

// Load reads configuration from environment variables
func Load() *Config {
	appMode := getEnv("APP_MODE", "local")
	hostRoot := getEnv("HOST_ROOT_PATH", ".")

	userPGHostDefault := "paas-user-postgres"
	userPGPortDefault := "5432"
	if appMode != "docker" {
		userPGHostDefault = "localhost"
		userPGPortDefault = "5433"
	}

	userPGPortVal := getEnv("USER_PG_PORT", userPGPortDefault)
	if appMode == "docker" {
		// Inside the docker network, the paas-user-postgres container always listens on 5432 internally.
		// The USER_PG_PORT environment variable (e.g. 5433) is the published port on the host VPS.
		userPGPortVal = "5432"
	}

	if abs, err := filepath.Abs(hostRoot); err == nil {
		hostRoot = abs
	}

	// Determine internal paths based on mode
	var projectsPath, dataPath, templatesPath, traefikDynamicDir string
	var railpacksPath string
	if appMode == "docker" {
		projectsPath = getEnv("PROJECTS_PATH", "/app/storage/projects")
		dataPath = getEnv("DATA_PATH", "/app/storage/data")
		templatesPath = getEnv("TEMPLATES_PATH", "/app/docker/templates")
		railpacksPath = getEnv("RAILPACKS_PATH", "/app/railpacks")
		traefikDynamicDir = getEnv("TRAEFIK_DYNAMIC_DIR", "/etc/traefik/dynamic")
	} else {
		projectsPath = getEnv("PROJECTS_PATH", "./storage/projects")
		dataPath = getEnv("DATA_PATH", "./storage/data")
		templatesPath = getEnv("TEMPLATES_PATH", "./docker/templates")
		railpacksPath = getEnv("RAILPACKS_PATH", "./railpacks")
		traefikDynamicDir = getEnv("TRAEFIK_DYNAMIC_DIR", "./docker/traefik/dynamic")
	}

	cfg := &Config{
		// App
		AppMode:      appMode,
		HostRootPath: hostRoot,
		AppEnv:       getEnv("APP_ENV", "production"),
		AppDebug:     getEnvBool("APP_DEBUG", false),

		// Infra Database (PostgreSQL)
		PGHost:     getEnv("PG_HOST", "paas-postgres"),
		PGPort:     getEnv("PG_PORT", "5432"),
		PGDatabase: getEnv("PG_DATABASE", "paas"),
		PGUser:     getEnv("PG_USER", "paas"),
		PGPassword: getEnv("PG_PASSWORD", ""),

		// User Database (MySQL)
		MYSQLHost:         getEnv("MYSQL_HOST", "paas-mysql"),
		MYSQLPort:         getEnv("MYSQL_PORT", "3306"),
		MYSQLDatabase:     getEnv("MYSQL_DATABASE", "paas"),
		MYSQLUser:         getEnv("MYSQL_USER", "paas"),
		MYSQLPassword:     getEnv("MYSQL_PASSWORD", ""),
		MYSQLRootPassword: getEnv("MYSQL_ROOT_PASSWORD", "rootpassword"),

		// User Database (PostgreSQL Engine)
		UserPGHost:     getEnv("USER_PG_HOST", userPGHostDefault),
		UserPGPort:     userPGPortVal,
		UserPGPassword: getEnv("USER_PG_PASSWORD", "user-pg-rootpassword"),

		// JWT
		JWTSecret:      getEnv("JWT_SECRET", "change-this-secret"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),

		// UID Obfuscation
		UIDSalt: getEnv("UID_SALT", "change-this-salt"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "paas-redis"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// Domain
		BaseDomain:    getEnv("BASE_DOMAIN", "localhost"),
		ProjectDomain: getEnv("PROJECT_DOMAIN", getEnv("BASE_DOMAIN", "localhost")),
		FrontendURL:   getEnv("FRONTEND_URL", "http://localhost:5173"),
		ACMEEmail:     getEnv("ACME_EMAIL", "admin@localhost"),

		// Docker & Paths
		DockerSocket:      getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ProjectsPath:      projectsPath,
		DataPath:          dataPath,
		HostProjectsPath:  getEnv("HOST_PROJECTS_PATH", filepath.Join(hostRoot, "storage/projects")),
		HostDataPath:      getEnv("HOST_DATA_PATH", filepath.Join(hostRoot, "storage/data")),
		HostTemplatesPath: getEnv("HOST_TEMPLATES_PATH", filepath.Join(hostRoot, "docker/templates")),
		TemplatesPath:     templatesPath,
		DockerNetwork:     getEnv("DOCKER_NETWORK", "paas-network"),
		TraefikDynamicDir: traefikDynamicDir,
		HostRailpacksPath: getEnv("HOST_RAILPACKS_PATH", filepath.Join(hostRoot, "railpacks")),
		RailpacksPath:     railpacksPath,

		// Nginx Remote Webhook
		NginxWebhookEnabled:       getEnvBool("NGINX_WEBHOOK_ENABLED", false),
		NginxWebhookURL:           getEnv("NGINX_WEBHOOK_URL", ""),
		NginxWebhookKey:           getEnv("NGINX_WEBHOOK_KEY", ""),
		InternalIP:                getEnv("INTERNAL_IP", "127.0.0.1"),
		IntegrityValidationMarker: getEnv("INTEGRITY_VALIDATION_MARKER", ""),
		TraefikHTTPPort:           getEnvInt("HTTP_PORT", 80),

		// GitHub App
		GithubAppID:             getEnv("GITHUB_APP_ID", ""),
		GithubAppPrivateKeyPath: getEnv("GITHUB_APP_PRIVATE_KEY_PATH", ""),
		GithubAppWebhookSecret:  getEnv("GITHUB_APP_WEBHOOK_SECRET", ""),

		// SecretStore Credentials Encryption
		CredentialEncryptionKey: getEnv("CREDENTIAL_ENCRYPTION_KEY", "default-fallback-encryption-key-for-dev"),
	}

	// Ensure host paths are absolute to prevent Docker volume naming errors
	if abs, err := filepath.Abs(cfg.HostProjectsPath); err == nil {
		cfg.HostProjectsPath = abs
	}
	if abs, err := filepath.Abs(cfg.HostDataPath); err == nil {
		cfg.HostDataPath = abs
	}
	if abs, err := filepath.Abs(cfg.HostTemplatesPath); err == nil {
		cfg.HostTemplatesPath = abs
	}
	if abs, err := filepath.Abs(cfg.HostRailpacksPath); err == nil {
		cfg.HostRailpacksPath = abs
	}
	if abs, err := filepath.Abs(cfg.TraefikDynamicDir); err == nil {
		cfg.TraefikDynamicDir = abs
	}

	return cfg
}

// ValidateProductionSecurity fails closed when production boot would use known weak secrets.
func (c *Config) ValidateProductionSecurity() error {
	if !strings.EqualFold(c.AppEnv, "production") {
		return nil
	}

	weakSecrets := map[string]string{
		"JWT_SECRET":                c.JWTSecret,
		"UID_SALT":                  c.UIDSalt,
		"CREDENTIAL_ENCRYPTION_KEY": c.CredentialEncryptionKey,
	}
	defaults := map[string]string{
		"JWT_SECRET":                "change-this-secret",
		"UID_SALT":                  "change-this-salt",
		"CREDENTIAL_ENCRYPTION_KEY": "default-fallback-encryption-key-for-dev",
	}

	for key, value := range weakSecrets {
		if value == "" || value == defaults[key] || strings.Contains(strings.ToLower(value), "change-me") || strings.Contains(strings.ToLower(value), "placeholder") || len(value) < 32 {
			return fmt.Errorf("%s must be configured with a strong production secret", key)
		}
	}

	return nil
}

// Helper functions to read environment variables
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
