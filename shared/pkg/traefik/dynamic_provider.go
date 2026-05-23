package traefik

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/redis/go-redis/v9"
)

// GetProjectDynamicFilePath returns the path to the dynamic configuration file for a given project
func GetProjectDynamicFilePath(cfg *config.Config, userID uint, projectID uint, subdomain string) string {
	return filepath.Join(cfg.TraefikDynamicDir, fmt.Sprintf("user-%d-project-%d-%s.yml", userID, projectID, subdomain))
}

// InvalidateTraefikConfigCache deletes the cached Traefik config from Redis
func InvalidateTraefikConfigCache(cfg *config.Config) error {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       0,
	})
	defer client.Close()

	ctx := context.Background()
	cacheKey := "traefik:dynamic_config"
	return client.Del(ctx, cacheKey).Err()
}

// WriteProjectDynamicFile invalidates the Redis cache for Traefik config and removes legacy flat files if present
func WriteProjectDynamicFile(cfg *config.Config, project *models.Project, domains []models.CustomDomain) error {
	if cfg.TraefikDynamicDir != "" {
		filePath := GetProjectDynamicFilePath(cfg, project.UserID, project.ID, project.Subdomain)
		if _, err := os.Stat(filePath); err == nil {
			_ = os.Remove(filePath)
		}
	}

	return InvalidateTraefikConfigCache(cfg)
}

// DeleteProjectDynamicFile invalidates the Redis cache for Traefik config and removes legacy flat files if present
func DeleteProjectDynamicFile(cfg *config.Config, userID uint, projectID uint, subdomain string) error {
	if cfg.TraefikDynamicDir != "" {
		filePath := GetProjectDynamicFilePath(cfg, userID, projectID, subdomain)
		if _, err := os.Stat(filePath); err == nil {
			_ = os.Remove(filePath)
		}
	}

	return InvalidateTraefikConfigCache(cfg)
}
