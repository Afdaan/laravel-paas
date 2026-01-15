// ===========================================
// Setting Handler
// ===========================================
// Handles system settings management
// ===========================================
package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

// SettingHandler handles settings endpoints
type SettingHandler struct {
	db *gorm.DB
}

// NewSettingHandler creates a new setting handler
func NewSettingHandler(db *gorm.DB) *SettingHandler {
	return &SettingHandler{db: db}
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
			Where("key = ?", key).
			Update("value", value)
		
		if result.Error != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to update settings",
			})
		}
	}

	return c.JSON(fiber.Map{
		"message": "Settings updated successfully",
	})
}

// GetSetting helper to get a setting value
func GetSetting(db *gorm.DB, key string, defaultValue string) string {
	var setting models.Setting
	if err := db.Where("key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}
