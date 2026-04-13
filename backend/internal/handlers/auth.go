// ===========================================
// Auth Handler
// ===========================================
// Handles login, logout, and user session
// ===========================================
package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/services"
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

	// Track login activity
	go h.userService.UpdateActivity(user.ID, c.IP(), true)

	return c.JSON(fiber.Map{
		"token": token,
		"user":  user,
	})
}

// Logout invalidates user session
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")

	if token != "" && token != authHeader {
		// Blacklist for the remaining duration
		h.service.Logout(token, time.Duration(h.cfg.JWTExpiryHours)*time.Hour)
	}

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

	return c.JSON(user)
}
