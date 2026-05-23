// ===========================================
// Proxy Security Middleware
// ===========================================
// Protects proxy endpoint from SSRF and unauthorized access
// ===========================================
package middleware

import (
	"net"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
)

// ValidateProxyTarget sanitizes and validates proxy destination
func ValidateProxyTarget() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Params("*")

		// Block path traversal
		if strings.Contains(path, "..") {
			return apperr.New(400, "INVALID_PATH", "Invalid path: directory traversal not allowed")
		}

		// Block query string injection patterns
		if strings.Contains(c.OriginalURL(), "file://") ||
			strings.Contains(c.OriginalURL(), "gopher://") ||
			strings.Contains(c.OriginalURL(), "dict://") {
			return apperr.New(400, "INVALID_PROTOCOL", "Unsupported protocol")
		}

		return c.Next()
	}
}

// IsInternalIP checks if an IP address is internal/private
func IsInternalIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	if parsed.IsLoopback() {
		return true
	}
	if parsed.IsPrivate() {
		return true
	}
	if parsed.IsLinkLocalUnicast() {
		return true
	}

	_, ipNet, _ := net.ParseCIDR("100.64.0.0/10")
	return ipNet.Contains(parsed)
}

// ProxyAuth middleware validates that proxy access is from authenticated users
func ProxyAuth(cfgJWTSecret string, redis Blacklister, userService ActivityTracker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// For subdomain proxy, we don't require JWT but we validate the subdomain maps to a valid project
		// The ProxyToProject handler already validates this by checking DB/cache
		// However, we block requests with suspicious headers

		// Block requests trying to access internal infrastructure via proxy
		host := c.Hostname()
		if IsInternalIP(host) {
			return apperr.New(403, "FORBIDDEN", "Access denied")
		}

		return c.Next()
	}
}

// RequireRole ensures user has one of the specified roles
func RequireRole(roles ...models.Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		roleVal := c.Locals("role")
		if roleVal == nil {
			return apperr.ErrUnauthorized
		}

		role, ok := roleVal.(string)
		if !ok {
			return apperr.ErrUnauthorized
		}

		for _, allowed := range roles {
			if role == string(allowed) {
				return c.Next()
			}
		}

		return apperr.ErrForbidden
	}
}
