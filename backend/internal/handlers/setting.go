// ===========================================
// Setting Handler
// ===========================================
// Handles system settings management
// ===========================================
package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/services/setting"
)

// SettingHandler handles settings endpoints
type SettingHandler struct {
	service *setting.SettingService
}

// NewSettingHandler creates a new setting handler
func NewSettingHandler(service *setting.SettingService) *SettingHandler {
	return &SettingHandler{
		service: service,
	}
}

// List returns all settings
func (h *SettingHandler) List(c *fiber.Ctx) error {
	settings, err := h.service.ListAllModels()
	if err != nil {
		return apperr.New(500, "SETTING_FETCH_FAILED", "Failed to fetch system settings")
	}

	settingsMap, _ := h.service.ListAll()

	return c.JSON(fiber.Map{
		"data": settings,
		"map":  settingsMap,
	})
}

// UpdateSettingsRequest represents settings update payload
type UpdateSettingsRequest struct {
	Settings map[string]interface{} `json:"settings"`
}

// Update modifies multiple settings at once
func (h *SettingHandler) Update(c *fiber.Ctx) error {
	var req UpdateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return apperr.ErrBadRequest
	}

	if err := h.service.UpdateBulk(req.Settings); err != nil {
		return apperr.New(500, "SETTING_UPDATE_FAILED", "Failed to save system settings")
	}

	return c.JSON(fiber.Map{
		"message": "Settings updated successfully",
	})
}
