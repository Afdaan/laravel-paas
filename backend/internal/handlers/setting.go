// ===========================================
// Setting Handler
// ===========================================
// Handles system settings management
// ===========================================
package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
	"gorm.io/gorm"
)

// SettingHandler handles settings endpoints
type SettingHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewSettingHandler creates a new setting handler
func NewSettingHandler(db *gorm.DB, cfg *config.Config) *SettingHandler {
	return &SettingHandler{db: db, cfg: cfg}
}

// List returns all settings
func (h *SettingHandler) List(c *fiber.Ctx) error {
	var settings []models.Setting
	if err := h.db.Find(&settings).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch settings",
		})
	}

	// Convert to map for easier frontend consumption
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}

	return c.JSON(fiber.Map{
		"data": settings,
		"map":  settingsMap,
	})
}

// UpdateSettingsRequest represents settings update payload
type UpdateSettingsRequest struct {
	Settings map[string]string `json:"settings"`
}

// Update modifies multiple settings at once
func (h *SettingHandler) Update(c *fiber.Ctx) error {
	var req UpdateSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Update each setting
	for key, value := range req.Settings {
		result := h.db.Model(&models.Setting{}).
			Where("setting_key = ?", key).
			Update("value", value)
		
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update settings",
			})
		}
	}

	// Regenerate Traefik file-provider config so domain changes take effect immediately.
	traefikUpdated := false
	var traefikErr error
	if h.cfg != nil {
		baseDomain := GetSetting(h.db, "base_domain", h.cfg.BaseDomain)
		projectDomain := GetSetting(h.db, "project_domain", h.cfg.ProjectDomain)
		traefikErr = services.GenerateTraefikDynamicConfig(h.cfg, baseDomain, projectDomain)
		traefikUpdated = traefikErr == nil
	}

	resp := fiber.Map{
		"message":               "Settings updated successfully",
		"traefik_config_updated": traefikUpdated,
	}
	if traefikErr != nil {
		resp["traefik_error"] = traefikErr.Error()
	}

	return c.JSON(resp)
}

// GetSetting helper to get a setting value
func GetSetting(db *gorm.DB, key string, defaultValue string) string {
	var setting models.Setting
	if err := db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}
