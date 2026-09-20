package middleware

import (
	"crypto/subtle"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

const (
	SessionCookieName = "paas_session"
	AdminCookieName   = "paas_admin_session"
	CSRFCookieName    = "paas_csrf"
	CSRFHeaderName    = "X-CSRF-Token"

	legacySessionCookieName = SessionCookieName
	legacyAdminCookieName   = AdminCookieName
	legacyCSRFCookieName    = CSRFCookieName
	billingRecentAuthWindow = 5 * time.Minute
)

func CookieNames(cfg *config.Config) (session, admin, csrf string) {
	if strings.EqualFold(cfg.AppEnv, "production") {
		return "__Host-paas_session", "__Host-paas_admin_session", "__Host-paas_csrf"
	}
	return legacySessionCookieName, legacyAdminCookieName, legacyCSRFCookieName
}

type CurrentUserProvider interface {
	GetUserByID(id uint) (*models.User, error)
}

type ActivityTracker interface {
	UpdateActivity(userID uint, ip string, forceLoginUpdate bool)
}

func JWTAuth(cfg *config.Config, auth *services.AuthService, userProvider CurrentUserProvider, tracker ActivityTracker) fiber.Handler {
	return jwtAuth(cfg, auth, userProvider, tracker, models.TokenUseSession, false)
}

func JWTStreamAuth(cfg *config.Config, auth *services.AuthService, userProvider CurrentUserProvider, tracker ActivityTracker) fiber.Handler {
	return jwtAuth(cfg, auth, userProvider, tracker, models.TokenUseStream, true)
}

func jwtAuth(cfg *config.Config, auth *services.AuthService, userProvider CurrentUserProvider, tracker ActivityTracker, expectedUse models.TokenUse, allowStreamToken bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sessionCookie, adminCookie, csrfCookie := CookieNames(cfg)
		currentCookie := c.Cookies(sessionCookie)
		legacyCookie := c.Cookies(legacySessionCookieName)
		if strings.EqualFold(cfg.AppEnv, "production") && legacyCookie != "" {
			clearLegacyCookies(c, cfg)
			if currentCookie == "" {
				return apperr.ErrUnauthorized
			}
		}

		authHeader := c.Get("Authorization")
		if authHeader != "" && currentCookie != "" {
			return apperr.New(401, "AMBIGUOUS_AUTH", "Multiple credentials are not allowed")
		}

		var tokenString string
		fromCookie := false
		if allowStreamToken && c.Query("stream_token") != "" {
			tokenString = c.Query("stream_token")
		} else if currentCookie != "" {
			tokenString, fromCookie = currentCookie, true
		} else if authHeader != "" {
			parts := strings.Fields(authHeader)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return apperr.New(401, "INVALID_AUTH", "Invalid authorization format")
			}
			tokenString = parts[1]
		}
		if tokenString == "" {
			return apperr.ErrUnauthorized
		}

		claims, err := auth.Verify(tokenString, expectedUse)
		if err != nil {
			return err
		}
		revoked, err := auth.IsRevoked(claims)
		if err != nil {
			return apperr.New(503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable")
		}
		if revoked {
			return apperr.New(401, "TOKEN_REVOKED", "This session has been invalidated")
		}
		if fromCookie && isUnsafeMethod(c.Method()) && !validCSRF(c, cfg, claims.ID, csrfCookie) {
			return apperr.New(403, "CSRF_FAILED", "Invalid request integrity token")
		}
		if fromCookie && isUnsafeMethod(c.Method()) && !validOrigin(c, cfg) {
			return apperr.New(403, "ORIGIN_FAILED", "Invalid request origin")
		}
		if c.Get("Sec-Fetch-Site") == "cross-site" {
			return apperr.New(403, "ORIGIN_FAILED", "Cross-site request denied")
		}

		userID, _ := strconv.ParseUint(claims.Subject, 10, 64)
		user, err := userProvider.GetUserByID(uint(userID))
		if err != nil {
			return apperr.New(401, "TOKEN_INVALID", "Invalid or expired session")
		}
		impersonating := false
		if claims.ImpersonatorID != 0 {
			backup := c.Cookies(adminCookie)
			adminClaims, err := auth.Verify(backup, models.TokenUseSession)
			if err != nil || adminClaims.ImpersonatorID != 0 || adminClaims.Subject != strconv.FormatUint(uint64(claims.ImpersonatorID), 10) {
				return apperr.New(401, "TOKEN_INVALID", "Invalid administrator session")
			}
			revoked, err := auth.IsRevoked(adminClaims)
			if err != nil || revoked {
				return apperr.New(401, "TOKEN_INVALID", "Invalid administrator session")
			}
			admin, err := userProvider.GetUserByID(claims.ImpersonatorID)
			if err != nil || !admin.IsAdmin() {
				return apperr.New(403, "FORBIDDEN", "Administrator access required")
			}
			c.Locals("actor_user_id", claims.ImpersonatorID)
			impersonating = true
		}
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		c.Locals("token", tokenString)
		c.Locals("claims", claims)
		c.Locals("impersonating", impersonating)
		if tracker != nil {
			go tracker.UpdateActivity(user.ID, c.IP(), false)
		}
		return c.Next()
	}
}

func isUnsafeMethod(method string) bool {
	return method != fiber.MethodGet && method != fiber.MethodHead && method != fiber.MethodOptions
}

func validCSRF(c *fiber.Ctx, cfg *config.Config, jti, csrfCookie string) bool {
	cookieToken, headerToken := c.Cookies(csrfCookie), c.Get(CSRFHeaderName)
	if cookieToken == "" || headerToken == "" || len(cookieToken) != len(headerToken) || subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
		return false
	}
	return services.ValidateCSRFToken(cfg.CSRFSecret, cfg.CSRFPreviousSecrets, jti, headerToken)
}

func validOrigin(c *fiber.Ctx, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return validOriginURL(c, cfg.FrontendURL, cfg.AppEnv)
}

func validOriginURL(c *fiber.Ctx, expectedURL, appEnv string) bool {
	origin := c.Get("Origin")
	if origin == "" {
		return false
	}
	a, errA := url.Parse(origin)
	if errA != nil || a.Path != "" || a.RawQuery != "" || a.Fragment != "" {
		return false
	}
	b, errB := url.Parse(expectedURL)
	if errB == nil && a.Scheme == b.Scheme && a.Host == b.Host {
		return true
	}
	host := c.Get("X-Forwarded-Host")
	if host == "" {
		host = c.Hostname()
	}
	if host != "" && strings.EqualFold(a.Host, host) {
		return true
	}
	// Allow local development origins ONLY when APP_ENV is not production
	if !strings.EqualFold(appEnv, "production") {
		aHost := a.Hostname()
		if aHost == "localhost" || aHost == "127.0.0.1" {
			return true
		}
	}
	return false
}


func clearLegacyCookies(c *fiber.Ctx, _ *config.Config) {
	for _, name := range []string{legacySessionCookieName, legacyAdminCookieName, legacyCSRFCookieName} {
		c.Cookie(&fiber.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HTTPOnly: name != legacyCSRFCookieName, Secure: true, SameSite: "Strict"})
	}
}

func RequireAdmin() fiber.Handler {
	return RequireRole(models.RoleAdmin, models.RoleSuperAdmin)
}

func RequireSuperAdmin() fiber.Handler {
	return RequireRole(models.RoleSuperAdmin)
}

func RequireNoBillingImpersonation() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if impersonating, _ := c.Locals("impersonating").(bool); impersonating {
			return apperr.New(403, "IMPERSONATION_FORBIDDEN", "Impersonated sessions cannot perform billing-affecting actions")
		}
		return c.Next()
	}
}

// RequireRecentBillingAuthentication permits only a freshly password-authenticated browser session.
func RequireRecentBillingAuthentication(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if impersonating, _ := c.Locals("impersonating").(bool); impersonating {
			return apperr.New(403, "IMPERSONATION_FORBIDDEN", "Impersonated sessions cannot change billing settings")
		}
		claims, _ := c.Locals("claims").(*models.JWTClaims)
		currentToken, _ := c.Locals("token").(string)
		sessionName, _, _ := CookieNames(cfg)
		sessionToken := c.Cookies(sessionName)
		if claims == nil || sessionToken == "" || subtle.ConstantTimeCompare([]byte(sessionToken), []byte(currentToken)) != 1 {
			return apperr.ErrUnauthorized
		}
		if claims.AuthTime == nil || time.Since(claims.AuthTime.Time) > billingRecentAuthWindow || claims.AuthTime.After(time.Now().UTC().Add(30*time.Second)) {
			return apperr.New(403, "RECENT_AUTH_REQUIRED", "Recent password authentication is required")
		}
		return c.Next()
	}
}
