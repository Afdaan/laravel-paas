// ===========================================
// Setting Service
// ===========================================
// Manages system-wide configurations and settings
// ===========================================
package services

import (
	"fmt"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
)

type SettingService struct {
	repo repositories.SettingRepository
}

func NewSettingService(repo repositories.SettingRepository) *SettingService {
	return &SettingService{repo: repo}
}

// Get fetches a setting value with a fallback default
func (s *SettingService) Get(key, defaultValue string) string {
	return s.repo.GetValue(key, defaultValue)
}

// ListAll returns all settings as a map for consumption
func (s *SettingService) ListAll() (map[string]string, error) {
	settings, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}

	settingsMap := make(map[string]string)
	for _, setting := range settings {
		settingsMap[setting.Key] = setting.Value
	}
	return settingsMap, nil
}

// ListAllModels returns the raw slice of setting models
func (s *SettingService) ListAllModels() ([]models.Setting, error) {
	return s.repo.ListAll()
}

// Update settings from a map of interface values (handling type conversion)
func (s *SettingService) UpdateBulk(settings map[string]interface{}) error {
	for key, value := range settings {
		// Convert any incoming type (bool, float, etc) to string
		strValue := fmt.Sprintf("%v", value)
		if err := s.repo.Upsert(key, strValue); err != nil {
			return err
		}
	}
	return nil
}

// GetInt fetches a setting and converts it to integer
func (s *SettingService) GetInt(key string, defaultValue int) int {
	valStr := s.Get(key, fmt.Sprintf("%d", defaultValue))
	var val int
	fmt.Sscanf(valStr, "%d", &val)
	return val
}
