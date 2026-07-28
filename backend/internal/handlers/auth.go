package handlers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

type AuthHandler struct {
	service     *services.AuthService
	userService *services.UserService
	cfg         *config.Config
	db          *gorm.DB
}

func NewAuthHandler(service *services.AuthService, cfg *config.Config, userService *services.UserService, db *gorm.DB) *AuthHandler {
	return &AuthHandler{service: service, cfg: cfg, userService: userService, db: db}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" || req.Password == "" {
		return apperr.NewBadRequest("Email and password are required")
	}
	user, err := h.service.Authenticate(req.Email, req.Password)
	if err != nil {
		return err
	}
	issued, err := h.service.IssueSession(user, 0)
	if err != nil {
		return err
	}
	h.setSessionCookies(c, issued, "")
	go h.userService.UpdateActivity(user.ID, c.IP(), true)
	return c.JSON(fiber.Map{"user": user})
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	claims, _ := c.Locals("claims").(*models.JWTClaims)
	if claims != nil {
		if err := h.service.Revoke(claims); err != nil {
			return apperr.New(503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable")
		}
	}
	_, adminCookie, _ := middleware.CookieNames(h.cfg)
	if backup := c.Cookies(adminCookie); backup != "" {
		if adminClaims, err := h.service.Verify(backup, models.TokenUseSession); err == nil {
			if err := h.service.Revoke(adminClaims); err != nil {
				return apperr.New(503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable")
			}
		}
	}
	h.clearSessionCookies(c)
	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	user, err := h.service.GetUserByID(userID)
	if err != nil {
		return err
	}
	if impersonating, _ := c.Locals("impersonating").(bool); impersonating {
		c.Set("X-Impersonating", "true")
	}
	return c.JSON(user)
}

type UpdateProfileRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password,omitempty"`
}

func (h *AuthHandler) UpdateProfile(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}
	user, err := h.userService.UpdateUser(userID, req.Name, req.Email, req.Password)
	if err != nil {
		return apperr.NewBadRequest("Failed to update profile")
	}
	return c.JSON(user)
}

func (h *AuthHandler) GenerateStreamToken(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	user, err := h.service.GetUserByID(userID)
	if err != nil {
		return err
	}
	token, err := h.service.GenerateStreamToken(user)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"token": token})
}

func (h *AuthHandler) LoginAsUser(c *fiber.Ctx) error {
	if impersonating, _ := c.Locals("impersonating").(bool); impersonating {
		h.auditImpersonation(c, nil, nil, "start", "rejected")
		return apperr.New(409, "ALREADY_IMPERSONATING", "Return to the administrator session first")
	}
	actorID, _ := c.Locals("user_id").(uint)
	actor, err := h.userService.GetUserByID(actorID)
	if err != nil || !actor.IsAdmin() {
		h.auditImpersonation(c, &actorID, nil, "start", "rejected")
		return apperr.ErrForbidden
	}
	targetID, err := c.ParamsInt("id")
	if err != nil {
		return apperr.NewBadRequest("Invalid user ID")
	}
	target, err := h.userService.GetUserByID(uint(targetID))
	if err != nil || target.Role != models.RoleUser {
		h.auditImpersonation(c, &actor.ID, nil, "start", "rejected")
		return apperr.New(403, "FORBIDDEN", "Cannot impersonate administrator accounts")
	}
	backup, _ := c.Locals("token").(string)
	issued, err := h.service.IssueSession(target, actor.ID)
	if err != nil {
		return err
	}
	if err := h.auditImpersonation(c, &actor.ID, &target.ID, "start", "succeeded"); err != nil {
		return apperr.New(503, "AUDIT_UNAVAILABLE", "Impersonation audit is temporarily unavailable")
	}
	h.setSessionCookies(c, issued, backup)
	go h.userService.UpdateActivity(target.ID, c.IP(), true)
	return c.JSON(fiber.Map{"user": target})
}

func (h *AuthHandler) ReturnToAdmin(c *fiber.Ctx) error {
	claims, _ := c.Locals("claims").(*models.JWTClaims)
	if claims == nil || claims.ImpersonatorID == 0 {
		return apperr.ErrUnauthorized
	}
	_, adminCookie, _ := middleware.CookieNames(h.cfg)
	backup := c.Cookies(adminCookie)
	adminClaims, err := h.service.Verify(backup, models.TokenUseSession)
	if err != nil || adminClaims.ImpersonatorID != 0 || adminClaims.Subject != strconv.FormatUint(uint64(claims.ImpersonatorID), 10) {
		return apperr.ErrUnauthorized
	}
	revoked, err := h.service.IsRevoked(adminClaims)
	if err != nil || revoked {
		return apperr.ErrUnauthorized
	}
	admin, err := h.userService.GetUserByID(claims.ImpersonatorID)
	if err != nil || !admin.IsAdmin() {
		return apperr.ErrForbidden
	}
	effectiveUserID, _ := strconv.ParseUint(claims.Subject, 10, 64)
	effectiveID := uint(effectiveUserID)
	if err := h.auditImpersonation(c, &admin.ID, &effectiveID, "return", "attempted"); err != nil {
		return apperr.New(503, "AUDIT_UNAVAILABLE", "Impersonation audit is temporarily unavailable")
	}
	if err := h.service.Revoke(claims); err != nil {
		h.auditImpersonationBestEffort(c, &admin.ID, &effectiveID, "return", "rejected")
		return apperr.New(503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable")
	}
	if err := h.service.Revoke(adminClaims); err != nil {
		h.auditImpersonationBestEffort(c, &admin.ID, &effectiveID, "return", "rejected")
		return apperr.New(503, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable")
	}
	issued, err := h.service.IssueSession(admin, 0)
	if err != nil {
		h.auditImpersonationBestEffort(c, &admin.ID, &effectiveID, "return", "rejected")
		return err
	}
	if err := h.auditImpersonation(c, &admin.ID, &effectiveID, "return", "succeeded"); err != nil {
		return apperr.New(503, "AUDIT_UNAVAILABLE", "Impersonation audit is temporarily unavailable")
	}
	h.setSessionCookies(c, issued, "")
	return c.JSON(fiber.Map{"message": "Returned to admin session"})
}

func (h *AuthHandler) auditImpersonation(c *fiber.Ctx, actorID, effectiveUserID *uint, event, result string) error {
	if h.db == nil {
		return fmt.Errorf("impersonation audit database unavailable")
	}
	return h.db.Create(&models.ImpersonationAudit{
		ActorUserID:     actorID,
		EffectiveUserID: effectiveUserID,
		Event:           event,
		Result:          result,
		SourceIP:        c.IP(),
	}).Error
}

func (h *AuthHandler) auditImpersonationBestEffort(c *fiber.Ctx, actorID, effectiveUserID *uint, event, result string) {
	if err := h.auditImpersonation(c, actorID, effectiveUserID, event, result); err != nil {
		slog.Error("failed to write impersonation audit", "event", event, "result", result, "error", err)
	}
}

func (h *AuthHandler) setSessionCookies(c *fiber.Ctx, issued *services.IssuedSession, adminToken string) {
	sessionName, adminName, csrfName := middleware.CookieNames(h.cfg)
	expires := issued.Claims.ExpiresAt.Time
	maxAge := int(time.Until(expires).Seconds())
	secure := strings.EqualFold(h.cfg.AppEnv, "production")
	for _, cookie := range []*fiber.Cookie{
		{Name: sessionName, Value: issued.Token, Path: "/", MaxAge: maxAge, Expires: expires, HTTPOnly: true, Secure: secure, SameSite: "Strict"},
		{Name: csrfName, Value: issued.CSRFToken, Path: "/", MaxAge: maxAge, Expires: expires, HTTPOnly: false, Secure: secure, SameSite: "Strict"},
	} {
		c.Cookie(cookie)
	}
	if adminToken != "" {
		c.Cookie(&fiber.Cookie{Name: adminName, Value: adminToken, Path: "/", MaxAge: maxAge, Expires: expires, HTTPOnly: true, Secure: secure, SameSite: "Strict"})
	} else {
		c.Cookie(&fiber.Cookie{Name: adminName, Value: "", Path: "/", MaxAge: -1, HTTPOnly: true, Secure: secure, SameSite: "Strict"})
	}
	if strings.EqualFold(h.cfg.AppEnv, "production") {
		h.clearLegacyCookies(c)
	}
}

func (h *AuthHandler) clearSessionCookies(c *fiber.Ctx) {
	sessionName, adminName, csrfName := middleware.CookieNames(h.cfg)
	secure := strings.EqualFold(h.cfg.AppEnv, "production")
	for _, name := range []string{sessionName, adminName, csrfName} {
		c.Cookie(&fiber.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HTTPOnly: name != csrfName, Secure: secure, SameSite: "Strict"})
	}
	if strings.EqualFold(h.cfg.AppEnv, "production") {
		h.clearLegacyCookies(c)
	}
}

func (h *AuthHandler) clearLegacyCookies(c *fiber.Ctx) {
	for _, name := range []string{middleware.SessionCookieName, middleware.AdminCookieName, middleware.CSRFCookieName} {
		c.Cookie(&fiber.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HTTPOnly: name != middleware.CSRFCookieName, Secure: true, SameSite: "Strict"})
	}
}
