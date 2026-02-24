// ===========================================
// Configuration Package
// ===========================================
// Loads and manages application configuration
// ===========================================
package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration
type Config struct {
	// App
	AppEnv   string
	AppDebug bool

	// Database
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	// JWT
	JWTSecret      string
	JWTExpiryHours int

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// Domain
	BaseDomain    string
	ProjectDomain string
	ACMEEmail     string

	// Docker
	DockerSocket   string
	ProjectsPath   string
	TemplatesPath  string
	DockerNetwork  string

	// Traefik dynamic config (file provider)
	TraefikDynamicTemplatePath string
	TraefikDynamicConfigPath   string
}

// Load reads configuration from environment variables
func Load() *Config {
	return &Config{
		// App
		AppEnv:   getEnv("APP_ENV", "production"),
		AppDebug: getEnvBool("APP_DEBUG", false),

		// Database
		DBHost:     getEnv("MYSQL_HOST", "mysql"),
		DBPort:     getEnv("MYSQL_PORT", "3306"),
		DBName:     getEnv("MYSQL_DATABASE", "paas"),
		DBUser:     getEnv("MYSQL_USER", "paas"),
		DBPassword: getEnv("MYSQL_PASSWORD", ""),

		// JWT
		JWTSecret:      getEnv("JWT_SECRET", "change-this-secret"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "paas-redis"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// Domain
		BaseDomain:    getEnv("BASE_DOMAIN", "localhost"),
		ProjectDomain: getEnv("PROJECT_DOMAIN", getEnv("BASE_DOMAIN", "localhost")),
		ACMEEmail:     getEnv("ACME_EMAIL", "admin@localhost"),

		// Docker
		DockerSocket:  getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
		ProjectsPath:  getEnv("PROJECTS_PATH", "/app/storage/projects"),
		TemplatesPath: getEnv("TEMPLATES_PATH", "/app/docker/templates"),
		DockerNetwork: getEnv("DOCKER_NETWORK", "paas-network"),

		// Traefik dynamic config
		TraefikDynamicTemplatePath: getEnv("TRAEFIK_DYNAMIC_TEMPLATE_PATH", "/app/docker/traefik/dynamic.yml.template"),
		TraefikDynamicConfigPath:   getEnv("TRAEFIK_DYNAMIC_CONFIG_PATH", "/app/docker/traefik/dynamic/dynamic.yml"),
	}
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
