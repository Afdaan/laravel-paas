// ===========================================
// Setting Service
// ===========================================
// Manages system-wide configurations and settings
// ===========================================
package setting

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
)

type SettingService struct {
	repo         repositories.SettingRepository
	redisService *infrastructure.RedisService
}

func NewSettingService(repo repositories.SettingRepository, redisService *infrastructure.RedisService) *SettingService {
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
	if err := s.redisService.SetCache(cacheKey, value, 24*time.Hour); err != nil {
		slog.Warn("Failed to cache setting in Redis", "key", key, "error", err)
	}

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
		if err := s.redisService.DeleteCache("setting:" + key); err != nil {
			slog.Warn("Failed to invalidate setting cache", "key", key, "error", err)
		}
	}
	return nil
}

// GetInt fetches a setting and converts it to integer
func (s *SettingService) GetInt(key string, defaultValue int) int {
	valStr := s.Get(key, fmt.Sprintf("%d", defaultValue))
	var val int
	if _, err := fmt.Sscanf(valStr, "%d", &val); err != nil {
		slog.Warn("Failed to parse setting as integer", "key", key, "value", valStr, "error", err)
	}
	return val
}
