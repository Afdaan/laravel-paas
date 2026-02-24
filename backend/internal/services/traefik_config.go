package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

// GenerateTraefikDynamicConfig renders docker/traefik/dynamic.yml from dynamic.yml.template
// by replacing placeholders with the provided domains.
func GenerateTraefikDynamicConfig(cfg *config.Config, baseDomain, projectDomain string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.TraefikDynamicTemplatePath == "" || cfg.TraefikDynamicConfigPath == "" {
		return fmt.Errorf("traefik dynamic paths are not configured")
	}

	tpl, err := os.ReadFile(cfg.TraefikDynamicTemplatePath)
	if err != nil {
		return fmt.Errorf("read traefik dynamic template: %w", err)
	}

	baseDomain = strings.TrimSpace(baseDomain)
	projectDomain = strings.TrimSpace(projectDomain)
	if baseDomain == "" || projectDomain == "" {
		return fmt.Errorf("baseDomain/projectDomain must not be empty")
	}

	content := string(tpl)
	content = strings.ReplaceAll(content, "{{BASE_DOMAIN}}", baseDomain)
	content = strings.ReplaceAll(content, "{{PROJECT_DOMAIN}}", projectDomain)

	// Atomic write to avoid Traefik reading a partially-written file.
	outputDir := filepath.Dir(cfg.TraefikDynamicConfigPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create traefik dynamic config dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(outputDir, "dynamic-*.yml")
	if err != nil {
		return fmt.Errorf("create temp traefik dynamic config: %w", err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp traefik dynamic config: %w", err)
	}
	if err := tmpFile.Chmod(0644); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod temp traefik dynamic config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp traefik dynamic config: %w", err)
	}

	if err := os.Rename(tmpName, cfg.TraefikDynamicConfigPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace traefik dynamic config: %w", err)
	}

	return nil
}

// SyncTraefikDynamicConfigFromDB regenerates Traefik file-provider config using persisted settings.
// This prevents panel-changed domains from being lost across restarts.
func SyncTraefikDynamicConfigFromDB(db *gorm.DB, cfg *config.Config) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	getSetting := func(key, defaultValue string) string {
		var setting models.Setting
		if err := db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
			return defaultValue
		}
		if strings.TrimSpace(setting.Value) == "" {
			return defaultValue
		}
		return setting.Value
	}

	baseDomain := getSetting("base_domain", cfg.BaseDomain)
	projectDomain := getSetting("project_domain", cfg.ProjectDomain)
	return GenerateTraefikDynamicConfig(cfg, baseDomain, projectDomain)
}
