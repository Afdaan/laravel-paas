// ===========================================
// Setting Handler
// ===========================================
// Handles system settings management
// ===========================================
package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/services/setting"
)

// SettingHandler handles settings endpoints
type SettingHandler struct {
	service *setting.SettingService
	cfg     *config.Config
}

// NewSettingHandler creates a new setting handler
func NewSettingHandler(service *setting.SettingService, cfg *config.Config) *SettingHandler {
	return &SettingHandler{
		service: service,
		cfg:     cfg,
	}
}

// List returns all settings
func (h *SettingHandler) List(c *fiber.Ctx) error {
	settings, err := h.service.ListAllModels()
	if err != nil {
		return apperr.New(500, "SETTING_FETCH_FAILED", "Failed to fetch system settings")
	}

	settingsMap, _ := h.service.ListAll()
	settingsMap[models.SettingBaseDomain] = h.cfg.BaseDomain
	settingsMap[models.SettingProjectDomain] = h.cfg.ProjectDomain
	for i := range settings {
		switch settings[i].Key {
		case models.SettingBaseDomain:
			settings[i].Value = h.cfg.BaseDomain
		case models.SettingProjectDomain:
			settings[i].Value = h.cfg.ProjectDomain
		}
	}

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

	if _, exists := req.Settings[models.SettingDefaultPaymentProvider]; exists {
		return apperr.NewBadRequest("default_payment_provider must be updated via dedicated finance endpoint /admin/billing/payment-provider")
	}

	if err := h.service.UpdateBulk(req.Settings); err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"message": "Settings updated successfully",
	})
}
