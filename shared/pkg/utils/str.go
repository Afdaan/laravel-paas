// ===========================================
// String Utilities
// ===========================================
// Centralized helpers for string generation
// used across multiple services
// ===========================================
package utils

import (
	"crypto/rand"
	"math/big"
	"regexp"
	"strings"
)

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
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[num.Int64()]
	}
	return string(result)
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
