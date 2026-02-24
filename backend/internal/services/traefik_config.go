package services

import (
	"fmt"
	"os"
	"strings"

	"github.com/laravel-paas/backend/internal/config"
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

	content := string(tpl)
	content = strings.ReplaceAll(content, "{{BASE_DOMAIN}}", baseDomain)
	content = strings.ReplaceAll(content, "{{PROJECT_DOMAIN}}", projectDomain)

	if err := os.WriteFile(cfg.TraefikDynamicConfigPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write traefik dynamic config: %w", err)
	}

	return nil
}
