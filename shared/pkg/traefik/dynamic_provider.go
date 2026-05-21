package traefik

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

// GetProjectDynamicFilePath returns the path to the dynamic configuration file for a given subdomain
func GetProjectDynamicFilePath(cfg *config.Config, subdomain string) string {
	return filepath.Join(cfg.TraefikDynamicDir, fmt.Sprintf("project-%s.yml", subdomain))
}

// WriteProjectDynamicFile writes an atomic YAML configuration file to Traefik's dynamic directory for a project
func WriteProjectDynamicFile(cfg *config.Config, project *models.Project, domains []models.CustomDomain) error {
	if cfg.TraefikDynamicDir == "" {
		return fmt.Errorf("traefik dynamic directory config is empty")
	}

	filePath := GetProjectDynamicFilePath(cfg, project.Subdomain)

	// Filter active routable custom domains
	var activeDomains []string
	for _, d := range domains {
		if models.IsNginxRoutableCustomDomainStatus(d.Status) {
			activeDomains = append(activeDomains, d.Domain)
		}
	}

	// If no active custom domains, clean up and remove the dynamic file
	if len(activeDomains) == 0 {
		return DeleteProjectDynamicFile(cfg, project.Subdomain)
	}

	// Determine target backend URL port
	internalPort := "8080"
	if project.Port != nil {
		internalPort = fmt.Sprintf("%d", *project.Port)
	} else if project.Framework == "Laravel" {
		internalPort = "80"
	}

	// Generate rule: Host(`domain1`) || Host(`domain2`)
	var rules []string
	for _, d := range activeDomains {
		rules = append(rules, fmt.Sprintf("Host(`%s`)", d))
	}
	ruleStr := strings.Join(rules, " || ")

	// Build dynamic routing config
	yamlContent := fmt.Sprintf(`http:
  routers:
    project-%s-custom:
      rule: "%s"
      service: "project-%s-custom"
      tls:
        certResolver: "letsencrypt"
  services:
    project-%s-custom:
      loadBalancer:
        servers:
          - url: "http://project-%s:%s"
`, project.Subdomain, ruleStr, project.Subdomain, project.Subdomain, project.Subdomain, internalPort)

	// Write atomically using a temporary file
	tmpPath := filePath + ".tmp"
	if err := os.MkdirAll(cfg.TraefikDynamicDir, 0777); err != nil {
		return fmt.Errorf("failed to create traefik dynamic directory: %w", err)
	}

	if err := os.WriteFile(tmpPath, []byte(yamlContent), 0666); err != nil {
		return fmt.Errorf("failed to write temporary dynamic routing file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath) // cleanup
		return fmt.Errorf("failed to atomically apply dynamic routing config: %w", err)
	}

	return nil
}

// DeleteProjectDynamicFile deletes the project's dynamic configuration file if it exists
func DeleteProjectDynamicFile(cfg *config.Config, subdomain string) error {
	filePath := GetProjectDynamicFilePath(cfg, subdomain)
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove dynamic routing file: %w", err)
		}
	}
	return nil
}
