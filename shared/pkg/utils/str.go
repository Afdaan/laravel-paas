// ===========================================
// String Utilities
// ===========================================
// Centralized helpers for string generation
// used across multiple services
// ===========================================
package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"sort"
	"strings"
)

var ansiEscapePattern = regexp.MustCompile(`(?:\x1B\[[0-?]*[ -/]*[@-~]|\x1B[@-_])`)

// GenerateRandom creates a random alphanumeric string of given length using CSPRNG
func GenerateRandom(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
}

// GeneratePassword creates a random password with mixed case and digits using CSPRNG
func GeneratePassword(length int) string {
	const lowerCharset = "abcdefghijklmnopqrstuvwxyz"
	const upperCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digitCharset = "0123456789"
	const charset = lowerCharset + upperCharset + digitCharset
	if length < 3 {
		length = 3
	}
	result := make([]byte, length)
	requiredCharsets := []string{lowerCharset, upperCharset, digitCharset}
	for i, requiredCharset := range requiredCharsets {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(requiredCharset))))
		result[i] = requiredCharset[num.Int64()]
	}
	for i := len(requiredCharsets); i < len(result); i++ {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	for i := len(result) - 1; i > 0; i-- {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(num.Int64())
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// GenerateLaravelAppKey returns a Laravel-compatible base64 encoded 32-byte application key.
func GenerateLaravelAppKey() (string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", err
	}
	return "base64:" + base64.StdEncoding.EncodeToString(keyBytes), nil
}

// GenerateSubdomain creates a URL-safe subdomain from a project name
// with a random suffix to guarantee uniqueness
func GenerateSubdomain(name string) string {
	clean := strings.ToLower(name)
	clean = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")

	if len(clean) > 25 {
		clean = clean[:25]
	}

	return clean + "-" + GenerateRandom(6)
}

// StripLogControlSequences removes terminal-only formatting before logs leave the backend.
func StripLogControlSequences(logStr string) string {
	if logStr == "" {
		return ""
	}

	logStr = ansiEscapePattern.ReplaceAllString(logStr, "")
	return strings.ReplaceAll(logStr, "\r", "")
}

// SanitizeError redacts internal infrastructure names, registry URLs, container names,
// directories, passwords/tokens, and simplifies verbose error outputs into clean user-facing summaries.
func SanitizeError(errStr string) string {
	if errStr == "" {
		return ""
	}

	// 1. Redact Git Authentication Tokens in URLs
	tokenRegex := regexp.MustCompile(`https://x-access-token:[^@]+@`)
	errStr = tokenRegex.ReplaceAllString(errStr, "https://x-access-token:REDACTED@")

	// 2. Redact internal registry URLs and ports
	registryRegex := regexp.MustCompile(`(paas-registry|127\.0\.0\.1|localhost):5000`)
	errStr = registryRegex.ReplaceAllString(errStr, "registry.local")

	// 3. Redact container names
	containerNameRegex := regexp.MustCompile(`paas-project-[a-zA-Z0-9_\-]+`)
	errStr = containerNameRegex.ReplaceAllString(errStr, "app-container")

	// 4. Redact builder names
	builderRegex := regexp.MustCompile(`paas-builder`)
	errStr = builderRegex.ReplaceAllString(errStr, "builder")

	// 5. Redact local absolute paths
	pathRegex := regexp.MustCompile(`/(home|var|tmp|usr/src)/[a-zA-Z0-9_\-\.\/]+`)
	errStr = pathRegex.ReplaceAllString(errStr, "/app/workspace/")

	// 6. Redact image hashes
	shaRegex := regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)
	errStr = shaRegex.ReplaceAllString(errStr, "sha256:REDACTED")

	// 7. Simplify typical verbose Docker BuildKit errors into clean user-facing summaries
	if strings.Contains(errStr, "DOCKER_BUILD_FAILED") || strings.Contains(errStr, "Docker build failed") {
		return "Deployment failed during the container build process.\nPlease review your application configuration, build commands, or dependency files (composer.json/package.json) for errors."
	}
	if strings.Contains(errStr, "CLONE_FAILED") || strings.Contains(errStr, "Failed to clone repository") {
		return "Failed to clone repository. Please verify your repository URL, branch configuration, or GitHub connection permissions."
	}
	if strings.Contains(errStr, "MIGRATION_FAILED") || strings.Contains(errStr, "Migrations failed") {
		return "Database migrations failed during deployment. Please review your migration scripts or database connections."
	}
	if strings.Contains(errStr, "TIMEOUT_EXCEEDED") || strings.Contains(errStr, "watchdog kill") {
		return "Deployment timed out. The build or startup phase took longer than the configured maximum time limit."
	}
	if strings.Contains(errStr, "SYSTEM_PANIC") {
		return "An internal system error occurred during deployment. The operation was aborted safely. Please contact support if this persists."
	}

	return errStr
}

// SanitizeLogOutput redacts sensitive infrastructure details, IP addresses, database names,
// and system usernames from raw build, migration, or runtime logs before presenting them to users.
func SanitizeLogOutput(logStr string) string {
	if logStr == "" {
		return ""
	}

	logStr = StripLogControlSequences(logStr)

	// 1. Redact IP Addresses (e.g., 172.18.0.11)
	ipRegex := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	logStr = ipRegex.ReplaceAllString(logStr, "[HOST_REDACTED]")

	// 2. Redact dynamic database names/usernames (e.g., laravel_crud_psql_0kllgh)
	// These always follow the tenant database naming pattern
	dbNameRegex := regexp.MustCompile(`\b[a-zA-Z0-9_]+_[a-z0-9]{6}\b`)
	logStr = dbNameRegex.ReplaceAllString(logStr, "[DATABASE_REDACTED]")

	// 3. Redact internal docker service hostnames
	hostRegex := regexp.MustCompile(`\bpaas-(mysql|postgres|user-postgres|redis|registry|traefik|buildkit)\b`)
	logStr = hostRegex.ReplaceAllString(logStr, "database.local")

	// 4. Redact database user connection details
	accessDeniedRegex := regexp.MustCompile(`Access denied for user '[^']+'@'[^']+'`)
	logStr = accessDeniedRegex.ReplaceAllString(logStr, "Access denied for user '[USER_REDACTED]'@'[HOST_REDACTED]'")

	return logStr
}

// FormatEnvMap serializes a map of environment variables into a grouped, sorted dotenv string.
// Values containing spaces or special characters are safely quoted and escaped.
func FormatEnvMap(envMap map[string]string) string {
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		pI := getEnvKeyPriority(keys[i])
		pJ := getEnvKeyPriority(keys[j])
		if pI != pJ {
			return pI < pJ
		}
		return keys[i] < keys[j]
	})

	var builder strings.Builder
	for _, k := range keys {
		builder.WriteString(fmt.Sprintf("%s=%s\n", k, quoteEnvValue(envMap[k])))
	}
	return builder.String()
}

func quoteEnvValue(val string) string {
	val = strings.TrimSpace(val)
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}

	// Escape actual embedded newline characters for single-line representation in .env
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "\r", "\\r")
	return val
}

func getEnvKeyPriority(key string) int {
	// Group 1: Platform Managed / Locked variables
	if key == "APP_NAME" {
		return 10
	}
	if key == "APP_URL" {
		return 11
	}

	// Group 2: Database Auto-Provisioned / Locked variables
	if strings.HasPrefix(key, "DB_") || strings.HasPrefix(key, "DATABASE_") {
		switch key {
		case "DB_CONNECTION":
			return 20
		case "DB_HOST":
			return 21
		case "DB_PORT":
			return 22
		case "DB_DATABASE":
			return 23
		case "DB_USERNAME":
			return 24
		case "DB_PASSWORD":
			return 25
		case "DATABASE_URL":
			return 26
		default:
			return 29
		}
	}

	// Group 3: Unlocked platform keys
	if strings.HasPrefix(key, "APP_") {
		switch key {
		case "APP_ENV":
			return 50
		case "APP_DEBUG":
			return 51
		case "APP_KEY":
			return 52
		default:
			return 59
		}
	}

	// Group 4: Log keys
	if strings.HasPrefix(key, "LOG_") {
		switch key {
		case "LOG_CHANNEL":
			return 60
		case "LOG_LEVEL":
			return 61
		case "LOG_DEPRECATIONS_CHANNEL":
			return 62
		default:
			return 69
		}
	}

	// Group 5: All other custom keys
	return 100
}

// RedactInfrastructureDetails removes database hosts, database names, container IDs, and path traces from error logs.
func RedactInfrastructureDetails(errorMsg string, sensitiveValues []string) string {
	// Redact DB Hostnames
	errorMsg = strings.ReplaceAll(errorMsg, "paas-mysql", "database-host")
	errorMsg = strings.ReplaceAll(errorMsg, "paas-user-postgres", "database-host")
	if mysqlName := os.Getenv("MYSQL_CONTAINER_NAME"); mysqlName != "" {
		errorMsg = strings.ReplaceAll(errorMsg, mysqlName, "database-host")
	}
	if postgresName := os.Getenv("POSTGRES_CONTAINER_NAME"); postgresName != "" {
		errorMsg = strings.ReplaceAll(errorMsg, postgresName, "database-host")
	}

	// Redact absolute server directory paths
	pathRegex := regexp.MustCompile(`/(home|var|app|etc|usr|nix)/[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*`)
	errorMsg = pathRegex.ReplaceAllString(errorMsg, "[internal_path]")

	// Redact Container ID references and names
	containerRegex := regexp.MustCompile(`paas-project-[a-zA-Z0-9_-]+-[0-9]+`)
	errorMsg = containerRegex.ReplaceAllString(errorMsg, "[web_container]")
	workerRegex := regexp.MustCompile(`paas-worker-[a-zA-Z0-9_-]+-[0-9]+`)
	errorMsg = workerRegex.ReplaceAllString(errorMsg, "[worker_container]")

	// Clean up hex container hashes (12 chars or 64 chars)
	hexRegex := regexp.MustCompile(`\b[a-f0-9]{12}\b|\b[a-f0-9]{64}\b`)
	errorMsg = hexRegex.ReplaceAllString(errorMsg, "[container_id]")

	// Strip Nixpacks/Railpack internal path prefixes
	errorMsg = strings.ReplaceAll(errorMsg, "railpack", "builder")
	errorMsg = strings.ReplaceAll(errorMsg, "nixpacks", "builder")

	// Dynamically redact sensitive secrets/values from environment configurations
	for _, val := range sensitiveValues {
		valClean := strings.TrimSpace(val)
		valLower := strings.ToLower(valClean)
		// Prevent redacting short keywords or standard flags to avoid breaking normal log phrases
		if len(valClean) > 3 && valLower != "true" && valLower != "false" && valLower != "local" && valLower != "production" && valLower != "development" && valLower != "staging" && valLower != "testing" {
			errorMsg = strings.ReplaceAll(errorMsg, valClean, "[redacted]")
		}
	}

	return errorMsg
}

// GetSmartSuggestion parses raw deployment error messages and returns highly actionable suggestions.
func GetSmartSuggestion(errorMsg string) string {
	// 1. Database Connection Failures
	if strings.Contains(errorMsg, "connection refused") ||
		strings.Contains(errorMsg, "Access denied for user") ||
		strings.Contains(errorMsg, "dial tcp: lookup") ||
		strings.Contains(errorMsg, "driver: bad connection") {
		return "Database connection failed. Please check if your database credentials in the Environment Variables (.env) tab are correct and that the database service is running."
	}

	// 2. Out of Memory (OOM) Crashes
	if strings.Contains(errorMsg, "JavaScript heap out of memory") ||
		strings.Contains(errorMsg, "Allowed memory size exhausted") ||
		strings.Contains(errorMsg, "Killed") ||
		strings.Contains(errorMsg, "exit status 137") {
		return "Application ran out of memory (OOM). Please try increasing the RAM limit of your project in the Resource Settings tab."
	}

	// 3. Compilation & Syntax Errors
	tsErrorRegex := regexp.MustCompile(`\berror TS[0-9]+:`)
	if tsErrorRegex.MatchString(errorMsg) ||
		strings.Contains(errorMsg, "syntax error") ||
		strings.Contains(errorMsg, "Failed to compile") ||
		strings.Contains(errorMsg, "Rollup failed") {
		return "A syntax error or compilation failure was detected in your codebase. Please inspect the detailed log output above for the exact file and line numbers."
	}

	// 4. Dependency & Lockfile Conflicts
	if strings.Contains(errorMsg, "npm ERR! code ERESOLVE") ||
		strings.Contains(errorMsg, "composer.lock was created for") ||
		strings.Contains(errorMsg, "Could not find a version that satisfies the requirement") {
		return "A dependency conflict was encountered. Please verify your package.json/composer.json/requirements.txt file or run a local installation to resolve conflicts."
	}

	// 5. Missing Start Command
	if strings.Contains(errorMsg, "NO_START_COMMAND") ||
		strings.Contains(errorMsg, "No start command detected") {
		return "No start command was detected. Non-static projects require a start command. Please configure a 'start' script in your package.json or specify a 'Start Command' in project settings."
	}

	return ""
}
