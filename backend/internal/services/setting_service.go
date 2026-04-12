// ===========================================
// Setting Service
// ===========================================
// Manages system-wide configurations and settings
// ===========================================
package services

import (
	"fmt"
	"time"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
)

type SettingService struct {
	repo         repositories.SettingRepository
	redisService *RedisService
}

func NewSettingService(repo repositories.SettingRepository, redisService *RedisService) *SettingService {
	return &SettingService{
		repo:         repo,
		redisService: redisService,
	}
}

// Get fetches a setting value with a fallback default (Cached)
func (s *SettingService) Get(key, defaultValue string) string {
	var value string
	cacheKey := "setting:" + key

	// Try Redis
	if err := s.redisService.GetCache(cacheKey, &value); err == nil {
		return value
	}

	// Fetch from DB
	value = s.repo.GetValue(key, defaultValue)

	// Save to Redis (long expiry since settings change rarely)
	s.redisService.SetCache(cacheKey, value, 24*time.Hour)

	return value
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
		// Invalidate Cache
		s.redisService.DeleteCache("setting:" + key)
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
