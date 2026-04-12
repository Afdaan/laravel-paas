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
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
)

// Blacklister abstraction to avoid circular dependencies with services package
type Blacklister interface {
	IsBlacklisted(token string) bool
}

// JWTAuth middleware validates JWT tokens and checks against Redis blacklist
func JWTAuth(secret string, redis Blacklister) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get token from Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return apperr.ErrUnauthorized
		}

		// Extract Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			return apperr.New(401, "INVALID_AUTH", "Invalid authorization format")
		}

		tokenString := parts[1]

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

		c.Locals("user_id", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("token", tokenString)

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
		if !ok || (role != "superadmin" && role != "admin") {
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
		if !ok || role != "superadmin" {
			return apperr.ErrForbidden
		}
		return c.Next()
	}
}
