// ===========================================
// Auth Handler
// ===========================================
// Handles login, logout, and user session
// ===========================================
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	service     *services.AuthService
	userService *services.UserService
	cfg         *config.Config
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(service *services.AuthService, cfg *config.Config, userService *services.UserService) *AuthHandler {
	return &AuthHandler{
		service:     service,
		userService: userService,
		cfg:         cfg,
	}
}

// LoginRequest represents login payload
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates user and returns JWT token
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	if req.Email == "" || req.Password == "" {
		return apperr.NewBadRequest("Email and password are required")
	}

	user, err := h.service.Authenticate(req.Email, req.Password)
	if err != nil {
		return err
	}

	token, err := h.service.GenerateToken(user)
	if err != nil {
		return err
	}

	h.setSessionCookies(c, token, "")

	// Track login activity
	go h.userService.UpdateActivity(user.ID, c.IP(), true)

	return c.JSON(fiber.Map{
		"user": user,
	})
}

// Logout invalidates user session
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		token = c.Cookies(middleware.SessionCookieName)
	}

	if token != "" && token != authHeader {
		// Blacklist for the remaining duration
		if err := h.service.Logout(token, time.Duration(h.cfg.JWTExpiryHours)*time.Hour); err != nil {
			slog.Warn("Failed to logout token", "error", err)
		}
	}

	adminToken := c.Cookies(middleware.AdminCookieName)
	if adminToken != "" {
		if err := h.service.Logout(adminToken, time.Duration(h.cfg.JWTExpiryHours)*time.Hour); err != nil {
			slog.Warn("Failed to logout admin token", "error", err)
		}
	}

	h.clearSessionCookies(c)

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

// Me returns current user information
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	user, err := h.service.GetUserByID(userID)
	if err != nil {
		return err
	}

	if impersonating, ok := c.Locals("impersonating").(bool); ok && impersonating {
		c.Set("X-Impersonating", "true")
	}

	return c.JSON(user)
}

// UpdateProfileRequest represents profile update payload
type UpdateProfileRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
}

// UpdateProfile updates the current authenticated user's profile
func (h *AuthHandler) UpdateProfile(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	user, err := h.userService.UpdateUser(userID, req.Name, req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user)
}

// GenerateStreamToken generates a short-lived (60s) ephemeral stream JWT intended exclusively for SSE endpoints
func (h *AuthHandler) GenerateStreamToken(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	user, err := h.service.GetUserByID(userID)
	if err != nil {
		return err
	}

	token, err := h.service.GenerateStreamToken(user)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"token": token,
	})
}

// LoginAsUser generates a JWT token for the specified user, allowing administrators to impersonate them.
func (h *AuthHandler) LoginAsUser(c *fiber.Ctx) error {
	targetUserID, err := c.ParamsInt("id")
	if err != nil {
		return apperr.NewBadRequest("Invalid user ID")
	}

	// Make sure the user exists
	targetUser, err := h.userService.GetUserByID(uint(targetUserID))
	if err != nil {
		return apperr.NewBadRequest("User not found")
	}

	// Make sure an admin is not trying to target another admin/superadmin to escalate privileges
	if targetUser.Role != models.RoleUser {
		return apperr.New(403, "FORBIDDEN", "Cannot impersonate administrator accounts")
	}

	// Generate JWT token for the target user
	token, err := h.service.GenerateToken(targetUser)
	if err != nil {
		return err
	}

	adminToken := c.Cookies(middleware.AdminCookieName)
	if adminToken == "" {
		adminToken, _ = c.Locals("token").(string)
	}
	if adminToken == "" {
		return apperr.ErrUnauthorized
	}

	// Optionally: Record the login attempt by the admin acting as the user
	go h.userService.UpdateActivity(targetUser.ID, c.IP(), true)

	h.setSessionCookies(c, token, adminToken)

	return c.JSON(fiber.Map{
		"user": targetUser,
	})
}

// ReturnToAdmin restores the original admin session after an impersonation flow.
func (h *AuthHandler) ReturnToAdmin(c *fiber.Ctx) error {
	adminToken := c.Cookies(middleware.AdminCookieName)
	if adminToken == "" {
		return apperr.ErrUnauthorized
	}

	h.setSessionCookies(c, adminToken, "")

	return c.JSON(fiber.Map{
		"message": "Returned to admin session",
	})
}

func (h *AuthHandler) setSessionCookies(c *fiber.Ctx, sessionToken string, adminToken string) {
	maxAge := h.cfg.JWTExpiryHours * 60 * 60
	expires := time.Now().Add(time.Duration(h.cfg.JWTExpiryHours) * time.Hour)
	secure := strings.HasPrefix(h.cfg.FrontendURL, "https://")
	csrfToken := generateCSRFToken()

	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Strict",
	})

	c.Cookie(&fiber.Cookie{
		Name:     middleware.CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  expires,
		HTTPOnly: false,
		Secure:   secure,
		SameSite: "Strict",
	})

	if adminToken != "" {
		c.Cookie(&fiber.Cookie{
			Name:     middleware.AdminCookieName,
			Value:    adminToken,
			Path:     "/",
			MaxAge:   maxAge,
			Expires:  expires,
			HTTPOnly: true,
			Secure:   secure,
			SameSite: "Strict",
		})
		return
	}

	c.Cookie(&fiber.Cookie{
		Name:     middleware.AdminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Strict",
	})
}

func (h *AuthHandler) clearSessionCookies(c *fiber.Ctx) {
	secure := strings.HasPrefix(h.cfg.FrontendURL, "https://")
	for _, name := range []string{middleware.SessionCookieName, middleware.AdminCookieName, middleware.CSRFCookieName} {
		c.Cookie(&fiber.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Now().Add(-time.Hour),
			HTTPOnly: name != middleware.CSRFCookieName,
			Secure:   secure,
			SameSite: "Strict",
		})
	}
}

func generateCSRFToken() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(bytes[:])
}
