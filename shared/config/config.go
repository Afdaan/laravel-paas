// ===========================================
// Configuration Package
// ===========================================
// Loads and manages application configuration
// ===========================================
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type JWTPreviousKey struct {
	ID       string
	Secret   string
	NotAfter time.Time
}

type CSRFPreviousSecret struct {
	ID       string
	Secret   string
	NotAfter time.Time
}

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
	MYSQLHost          string
	MYSQLPort          string
	MYSQLDatabase      string
	MYSQLUser          string
	MYSQLPassword      string
	MYSQLRootPassword  string
	MYSQLContainerName string

	// User Database (PostgreSQL Engine)
	UserPGHost            string
	UserPGPort            string
	UserPGPassword        string
	POSTGRESContainerName string

	// JWT
	JWTSecret           string
	JWTKeyID            string
	JWTPreviousKeys     []JWTPreviousKey
	JWTPreviousKeysErr  error
	JWTIssuer           string
	JWTAudience         string
	JWTExpiryHours      int
	CSRFSecret          string
	CSRFPreviousSecrets []CSRFPreviousSecret
	CSRFPreviousErr     error

	// Billing rollout
	BillingEnabled         bool
	BillingTopupEnabled    bool
	BillingTopupProvider   string
	BillingGraceDays       int
	BillingDeployBlockDays int
	MidtransServerKey      string
	MidtransClientKey      string
	MidtransMerchantID     string
	MidtransProduction     bool

	PakasirEnabled         bool
	PakasirProjectSlug     string
	PakasirAPIKey          string
	PakasirProduction      bool

	// Telegram Bot Payment Notifications
	TelegramBotPaymentEnabled bool
	TelegramBotPaymentToken   string
	TelegramBotPaymentChatID  string
	TelegramBotPaymentTopicID int64

	// UID Obfuscation
	UIDSalt string

	// Redis
	RedisHost     string
	RedisPort     string
	RedisPassword string

	// Domain
	BaseDomain                  string
	ProjectDomain               string
	FrontendURL                 string
	ACMEEmail                   string
	TrustedProxyCIDRs           []string
	TrustedProxyCIDRsConfigured bool
	TrustedProxyCIDRsErr        error
	InternalAPIToken            string

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
	CredentialEncryptionKey                   string
	CredentialEncryptionPreviousKeys          []string
	CredentialEncryptionAllowInsecurePrevious bool
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

	jwtPreviousKeys, jwtPreviousErr := parseJWTPreviousKeys(getEnv("JWT_PREVIOUS_KEYS", ""))
	csrfPreviousSecrets, csrfPreviousErr := parseCSRFPreviousSecrets(getEnv("CSRF_SECRET_PREVIOUS", ""))
	trustedProxyCIDRsRaw, trustedProxyCIDRsConfigured := os.LookupEnv("TRUSTED_PROXY_CIDRS")
	trustedProxyCIDRsRaw = strings.TrimSpace(trustedProxyCIDRsRaw)
	if !trustedProxyCIDRsConfigured || trustedProxyCIDRsRaw == "" {
		trustedProxyCIDRsRaw = "127.0.0.1/32,::1/128"
		trustedProxyCIDRsConfigured = false
	}
	trustedProxyCIDRs, trustedProxyCIDRsErr := parseCIDRList(trustedProxyCIDRsRaw)

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
		MYSQLHost:          getEnv("MYSQL_HOST", "paas-mysql"),
		MYSQLPort:          getEnv("MYSQL_PORT", "3306"),
		MYSQLDatabase:      getEnv("MYSQL_DATABASE", "paas"),
		MYSQLUser:          getEnv("MYSQL_USER", "paas"),
		MYSQLPassword:      getEnv("MYSQL_PASSWORD", ""),
		MYSQLRootPassword:  getEnv("MYSQL_ROOT_PASSWORD", "rootpassword"),
		MYSQLContainerName: getEnv("MYSQL_CONTAINER_NAME", "paas-mysql"),

		// User Database (PostgreSQL Engine)
		UserPGHost:            getEnv("USER_PG_HOST", userPGHostDefault),
		UserPGPort:            userPGPortVal,
		UserPGPassword:        getEnv("USER_PG_PASSWORD", "user-pg-rootpassword"),
		POSTGRESContainerName: getEnv("POSTGRES_CONTAINER_NAME", "paas-user-postgres"),

		// JWT
		JWTSecret:           getEnv("JWT_SECRET", "change-this-secret"),
		JWTKeyID:            getEnv("JWT_KEY_ID", "primary"),
		JWTPreviousKeys:     jwtPreviousKeys,
		JWTPreviousKeysErr:  jwtPreviousErr,
		JWTIssuer:           getEnv("JWT_ISSUER", "runara"),
		JWTAudience:         getEnv("JWT_AUDIENCE", "runara-api"),
		JWTExpiryHours:      getEnvInt("JWT_EXPIRY_HOURS", 24),
		CSRFSecret:          getEnv("CSRF_SECRET", "change-this-csrf-secret"),
		CSRFPreviousSecrets: csrfPreviousSecrets,
		CSRFPreviousErr:     csrfPreviousErr,

		// Billing rollout
		BillingEnabled:         getEnvBool("BILLING_ENABLED", false),
		BillingTopupEnabled:    getEnvBool("BILLING_TOPUP_ENABLED", false),
		BillingTopupProvider:   getEnv("BILLING_TOPUP_PROVIDER", getEnv("PAYMENT_GATEWAY_PROVIDER", "pakasir")),
		BillingGraceDays:       getEnvInt("BILLING_GRACE_DAYS", 7),
		BillingDeployBlockDays: getEnvInt("BILLING_DEPLOY_BLOCK_DAYS", 3),
		MidtransServerKey:      getEnv("MIDTRANS_SERVER_KEY", ""),
		MidtransClientKey:      getEnv("MIDTRANS_CLIENT_KEY", ""),
		MidtransMerchantID:     getEnv("MIDTRANS_MERCHANT_ID", ""),
		MidtransProduction:     getEnvBool("MIDTRANS_IS_PRODUCTION", false),

		PakasirEnabled:         getEnvBool("PAKASIR_ENABLED", false),
		PakasirProjectSlug:     getEnv("PAKASIR_PROJECT_SLUG", ""),
		PakasirAPIKey:          getEnv("PAKASIR_API_KEY", ""),
		PakasirProduction:      getEnvBool("PAKASIR_IS_PRODUCTION", false),

		// Telegram Bot Payment Notifications
		TelegramBotPaymentEnabled: getEnvBool("TELEGRAM_BOT_PAYMENT_ENABLED", false),
		TelegramBotPaymentToken:   getEnv("TELEGRAM_BOT_PAYMENT_TOKEN", ""),
		TelegramBotPaymentChatID:  getEnv("TELEGRAM_BOT_PAYMENT_CHAT_ID", getEnv("TELEGRAM_ADMIN_PAYMENT_CHAT_ID", "")),
		TelegramBotPaymentTopicID: getEnvInt64("TELEGRAM_BOT_PAYMENT_TOPIC_ID", 0),

		// UID Obfuscation
		UIDSalt: getEnv("UID_SALT", "change-this-salt"),

		// Redis
		RedisHost:     getEnv("REDIS_HOST", "paas-redis"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		// Domain
		BaseDomain:                  getEnv("BASE_DOMAIN", "localhost"),
		ProjectDomain:               getEnv("PROJECT_DOMAIN", getEnv("BASE_DOMAIN", "localhost")),
		FrontendURL:                 getEnv("FRONTEND_URL", "http://localhost:5173"),
		ACMEEmail:                   getEnv("ACME_EMAIL", "admin@localhost"),
		TrustedProxyCIDRs:           trustedProxyCIDRs,
		TrustedProxyCIDRsConfigured: trustedProxyCIDRsConfigured,
		TrustedProxyCIDRsErr:        trustedProxyCIDRsErr,
		InternalAPIToken:            getEnv("INTERNAL_API_TOKEN", ""),

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
		CredentialEncryptionKey:                   getEnv("CREDENTIAL_ENCRYPTION_KEY", "default-fallback-encryption-key-for-dev"),
		CredentialEncryptionPreviousKeys:          parseSecretList(getEnv("CREDENTIAL_ENCRYPTION_KEY_PREVIOUS", "")),
		CredentialEncryptionAllowInsecurePrevious: getEnvBool("CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS", false),
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
	return c.validateProductionSecurity(true)
}

// ValidateProductionWorkerSecurity excludes browser-only signing secrets from worker processes.
func (c *Config) ValidateProductionWorkerSecurity() error {
	return c.validateProductionSecurity(false)
}

func (c *Config) validateProductionSecurity(includeAuth bool) error {
	if !strings.EqualFold(c.AppEnv, "production") {
		return nil
	}

	weakSecrets := map[string]string{
		"UID_SALT":                  c.UIDSalt,
		"CREDENTIAL_ENCRYPTION_KEY": c.CredentialEncryptionKey,
	}
	defaults := map[string]string{
		"UID_SALT":                  "change-this-salt",
		"CREDENTIAL_ENCRYPTION_KEY": "default-fallback-encryption-key-for-dev",
	}
	if includeAuth {
		weakSecrets["JWT_SECRET"] = c.JWTSecret
		weakSecrets["CSRF_SECRET"] = c.CSRFSecret
		defaults["JWT_SECRET"] = "change-this-secret"
		defaults["CSRF_SECRET"] = "change-this-csrf-secret"
	}
	for key, value := range weakSecrets {
		if value == "" || value == defaults[key] || strings.Contains(strings.ToLower(value), "change-me") || strings.Contains(strings.ToLower(value), "placeholder") || len(value) < 32 {
			return fmt.Errorf("%s must be configured with a strong production secret", key)
		}
	}
	for index, value := range c.CredentialEncryptionPreviousKeys {
		if value == "" || value == defaults["CREDENTIAL_ENCRYPTION_KEY"] || strings.Contains(strings.ToLower(value), "change-me") || strings.Contains(strings.ToLower(value), "placeholder") || len(value) < 32 {
			if !c.CredentialEncryptionAllowInsecurePrevious {
				return fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY_PREVIOUS entry %d is weak; set CREDENTIAL_ENCRYPTION_ALLOW_INSECURE_PREVIOUS=true only for a temporary one-time migration", index+1)
			}
		}
	}
	if !c.TrustedProxyCIDRsConfigured || c.TrustedProxyCIDRsErr != nil || len(c.TrustedProxyCIDRs) == 0 {
		return fmt.Errorf("TRUSTED_PROXY_CIDRS must be explicitly configured in production")
	}
	if includeAuth && !ValidInternalAPIToken(c.InternalAPIToken) {
		return fmt.Errorf("INTERNAL_API_TOKEN must be 64 lowercase hexadecimal characters")
	}
	if includeAuth {
		if c.JWTKeyID == "" || !ValidJWTKeyID(c.JWTKeyID) {
			return fmt.Errorf("JWT_KEY_ID is invalid")
		}
		if c.JWTPreviousKeysErr != nil || len(c.JWTPreviousKeys) > 3 {
			return fmt.Errorf("JWT_PREVIOUS_KEYS is invalid or exceeds maximum of 3 keys")
		}
		for _, key := range c.JWTPreviousKeys {
			if !ValidJWTKeyID(key.ID) || key.ID == c.JWTKeyID || len(key.Secret) < 32 {
				return fmt.Errorf("JWT_PREVIOUS_KEYS contains an invalid key")
			}
		}
		if c.CSRFPreviousErr != nil || len(c.CSRFPreviousSecrets) > 3 {
			return fmt.Errorf("CSRF_SECRET_PREVIOUS is invalid or exceeds maximum of 3 secrets")
		}
		for _, secret := range c.CSRFPreviousSecrets {
			if !ValidJWTKeyID(secret.ID) || len(secret.Secret) < 32 {
				return fmt.Errorf("CSRF_SECRET_PREVIOUS contains an invalid secret")
			}
		}
	}
	baseDomain, err := NormalizeHostname(c.BaseDomain)
	if err != nil {
		return fmt.Errorf("BASE_DOMAIN: %w", err)
	}
	projectDomain, err := NormalizeHostname(c.ProjectDomain)
	if err != nil {
		return fmt.Errorf("PROJECT_DOMAIN: %w", err)
	}
	if _, err := NormalizeOrigin(c.FrontendURL); err != nil {
		return fmt.Errorf("FRONTEND_URL: %w", err)
	}
	if err := distinctRegistrableDomains(baseDomain, projectDomain); err != nil {
		return err
	}
	c.BaseDomain = baseDomain
	c.ProjectDomain = projectDomain
	if c.BillingTopupEnabled && !c.BillingEnabled {
		return fmt.Errorf("BILLING_TOPUP_ENABLED requires BILLING_ENABLED=true")
	}
	if c.BillingEnabled && (c.BillingDeployBlockDays <= 0 || c.BillingGraceDays <= 0 || c.BillingDeployBlockDays >= c.BillingGraceDays) {
		return fmt.Errorf("BILLING_DEPLOY_BLOCK_DAYS must be positive and less than BILLING_GRACE_DAYS")
	}
	if includeAuth && c.BillingTopupEnabled {
		provider := strings.ToLower(strings.TrimSpace(c.BillingTopupProvider))
		if provider == "" {
			provider = "pakasir"
		}
		if provider == "midtrans" {
			if c.MidtransServerKey == "" || c.MidtransMerchantID == "" {
				return fmt.Errorf("Midtrans credentials (MIDTRANS_SERVER_KEY, MIDTRANS_MERCHANT_ID) are required when selected as default billing provider")
			}
		} else if provider == "pakasir" {
			if !c.PakasirEnabled {
				return fmt.Errorf("PAKASIR_ENABLED must be true when Pakasir is selected as default billing provider")
			}
			if c.PakasirProjectSlug == "" || c.PakasirAPIKey == "" {
				return fmt.Errorf("Pakasir credentials (PAKASIR_PROJECT_SLUG and PAKASIR_API_KEY) are required when selected as default billing provider")
			}
		} else {
			return fmt.Errorf("invalid billing provider %q: must be midtrans or pakasir", provider)
		}
		if c.PakasirEnabled && (c.PakasirProjectSlug == "" || c.PakasirAPIKey == "") {
			return fmt.Errorf("Pakasir credentials (PAKASIR_PROJECT_SLUG and PAKASIR_API_KEY) are required when PAKASIR_ENABLED=true")
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

// parseJWTPreviousKeys expects comma-separated key_id|secret|not_after_utc entries.
func parseJWTPreviousKeys(value string) ([]JWTPreviousKey, error) {
	if value == "" {
		return nil, nil
	}
	keys := make([]JWTPreviousKey, 0)
	seen := make(map[string]struct{})
	for _, entry := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "|", 3)
		if len(parts) != 3 || !ValidJWTKeyID(parts[0]) || parts[1] == "" {
			return nil, fmt.Errorf("invalid previous JWT key")
		}
		notAfter, err := time.Parse(time.RFC3339, parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid previous JWT key expiry")
		}
		if _, duplicate := seen[parts[0]]; duplicate {
			return nil, fmt.Errorf("duplicate previous JWT key ID")
		}
		seen[parts[0]] = struct{}{}
		keys = append(keys, JWTPreviousKey{ID: parts[0], Secret: parts[1], NotAfter: notAfter.UTC()})
	}
	return keys, nil
}

// parseCSRFPreviousSecrets expects comma-separated key_id|secret|not_after_utc entries.
func parseCSRFPreviousSecrets(value string) ([]CSRFPreviousSecret, error) {
	if value == "" {
		return nil, nil
	}
	secrets := make([]CSRFPreviousSecret, 0)
	seen := make(map[string]struct{})
	for _, entry := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "|", 3)
		if len(parts) != 3 || !ValidJWTKeyID(parts[0]) || parts[1] == "" {
			return nil, fmt.Errorf("invalid previous CSRF secret")
		}
		notAfter, err := time.Parse(time.RFC3339, parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid previous CSRF secret expiry")
		}
		if _, duplicate := seen[parts[0]]; duplicate {
			return nil, fmt.Errorf("duplicate previous CSRF secret ID")
		}
		seen[parts[0]] = struct{}{}
		secrets = append(secrets, CSRFPreviousSecret{ID: parts[0], Secret: parts[1], NotAfter: notAfter.UTC()})
	}
	return secrets, nil
}

func parseCIDRList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	cidrs := make([]string, 0, len(parts))
	for _, part := range parts {
		cidr := strings.TrimSpace(part)
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return nil, fmt.Errorf("invalid CIDR")
		}
		cidrs = append(cidrs, cidr)
	}
	return cidrs, nil
}

func parseSecretList(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	})

	secrets := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		secret := strings.TrimSpace(part)
		if secret == "" {
			continue
		}
		if _, ok := seen[secret]; ok {
			continue
		}
		seen[secret] = struct{}{}
		secrets = append(secrets, secret)
	}
	return secrets
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

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if int64Value, err := strconv.ParseInt(value, 10, 64); err == nil {
			return int64Value
		}
	}
	return defaultValue
}
