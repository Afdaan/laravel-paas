// ===========================================
// String Utilities
// ===========================================
// Centralized helpers for string generation
// used across multiple services
// ===========================================
package string_util

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
