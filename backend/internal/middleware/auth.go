// ===========================================
// JWT Middleware
// ===========================================
// Handles authentication via JWT tokens
// ===========================================
package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
)

// Blacklister abstraction to avoid circular dependencies with services package
type Blacklister interface {
	IsBlacklisted(token string) bool
}

// ActivityTracker abstraction
type ActivityTracker interface {
	UpdateActivity(userID uint, ip string, forceLoginUpdate bool)
}

// JWTAuth middleware validates JWT tokens and checks against Redis blacklist.
// Security Context: Exposing long-lived primary JWTs in URL query parameters is dangerous
// because URLs are logged in reverse proxy access logs, browser history, and analytics.
// To prevent token leakage, standard API endpoints require Authorization header Bearer tokens.
// Streaming endpoints (SSE) use short-lived (60s) ephemeral tokens via stream_token query param.
func JWTAuth(secret string, redis Blacklister, tracker ActivityTracker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		var tokenString string

		path := c.Path()
		isStreamEndpoint := strings.HasSuffix(path, "/stream") || strings.HasSuffix(path, "/logs") || strings.HasSuffix(path, "/build-logs") || strings.HasSuffix(path, "/deployment-events")

		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return apperr.New(401, "INVALID_AUTH", "Invalid authorization format")
			}
			tokenString = parts[1]
		} else if isStreamEndpoint {
			tokenString = c.Query("stream_token")
		}

		if tokenString == "" {
			return apperr.ErrUnauthorized
		}

		// Check Blacklist
		if redis != nil && redis.IsBlacklisted(tokenString) {
			return apperr.New(401, "TOKEN_BLACKLISTED", "This session has been invalidated")
		}

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &models.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			return apperr.New(401, "TOKEN_INVALID", "Invalid or expired session")
		}

		// Store claims in context
		claims, ok := token.Claims.(*models.JWTClaims)
		if !ok {
			return apperr.ErrUnauthorized
		}

		// Security constraint: Ephemeral stream tokens cannot be reused for standard API auth
		if claims.StreamOnly && !isStreamEndpoint {
			return apperr.New(403, "FORBIDDEN", "Ephemeral stream tokens are restricted exclusively to streaming endpoints")
		}
		// Security constraint: Standard long-lived tokens cannot be passed via query string to stream endpoints
		if isStreamEndpoint && authHeader == "" && !claims.StreamOnly {
			return apperr.New(403, "FORBIDDEN", "Primary tokens cannot be passed via query strings. Please use an ephemeral stream token.")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("token", tokenString)

		// Update activity (non-blocking)
		if tracker != nil {
			go tracker.UpdateActivity(claims.UserID, c.IP(), false)
		}

		return c.Next()
	}
}

// RequireAdmin middleware ensures user has admin privileges
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		roleVal := c.Locals("role")
		if roleVal == nil {
			return apperr.ErrUnauthorized
		}

		role, ok := roleVal.(string)
		if !ok || (role != string(models.RoleSuperAdmin) && role != string(models.RoleAdmin)) {
			return apperr.ErrForbidden
		}
		return c.Next()
	}
}

// RequireSuperAdmin middleware ensures user is superadmin
func RequireSuperAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		roleVal := c.Locals("role")
		if roleVal == nil {
			return apperr.ErrUnauthorized
		}

		role, ok := roleVal.(string)
		if !ok || role != string(models.RoleSuperAdmin) {
			return apperr.ErrForbidden
		}
		return c.Next()
	}
}
