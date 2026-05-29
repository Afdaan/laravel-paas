package docker

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/laravel-paas/shared/models"
)

func (s *DockerService) CreateEnvFile(project *models.Project, projectDomain string, isInitial bool) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)
	examplePath := filepath.Join(projectPath, ".env.example")
	envPath := filepath.Join(projectPath, ".env")

	// If .env already exists, NEVER overwrite it.
	// We only proceed if the file is missing.
	if _, err := os.Stat(envPath); err == nil {
		return nil
	}

	// 1. Load mandatory variables from template
	mandatory, err := s.loadMandatoryEnv(project, projectDomain)
	if err != nil {
		slog.Error("Failed to load mandatory env template, falling back to basic config", "error", err)

		// Securely generate a fallback APP_KEY
		key := make([]byte, 32)
		var appKey string
		if _, err := rand.Read(key); err != nil {
			appKey = "base64:fallback-key-generation-failed-check-logs"
		} else {
			appKey = "base64:" + base64.StdEncoding.EncodeToString(key)
		}

		// Basic fallback if template is missing
		mandatory = map[string]string{
			"APP_NAME":      fmt.Sprintf("\"%s\"", project.Name),
			"APP_KEY":       appKey,
			"DB_CONNECTION": "mysql",
			"DB_HOST":       "paas-mysql",
			"DB_PORT":       "3306",
			"DB_DATABASE":   project.DatabaseName,
			"DB_USERNAME":   project.DatabaseName,
			"DB_PASSWORD":   project.DatabasePassword,
			"DATABASE_URL":  fmt.Sprintf("mysql://%s:%s@paas-mysql:3306/%s", project.DatabaseName, project.DatabasePassword, project.DatabaseName),
		}
	}

	var finalLines []string
	seen := make(map[string]bool)

	// Select base file:
	// - On initial deploy: try to use .env.example as a template to help user get started.
	// - On redeploy: NEVER use .env.example as a base, as it causes resets.
	//   We only use the mandatory DB variables if .env is missing.
	basePath := ""
	if isInitial {
		if _, err := os.Stat(examplePath); err == nil {
			basePath = examplePath
		}
	}

	// Load from base file if it exists
	if data, err := os.ReadFile(basePath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				finalLines = append(finalLines, line)
				continue
			}

			// Handle commented lines that might contain mandatory keys
			// e.g. "# DB_HOST=127.0.0.1" -> we want to uncomment and set to paas-mysql
			isCommented := strings.HasPrefix(trimmed, "#")
			effectiveLine := trimmed
			if isCommented {
				effectiveLine = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			}

			parts := strings.SplitN(effectiveLine, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				if val, ok := mandatory[key]; ok {
					// We found a mandatory key (even if it was commented out)
					finalLines = append(finalLines, fmt.Sprintf("%s=%s", key, val))
					seen[key] = true
				} else if isCommented {
					// It's a comment but not a mandatory key, keep as is
					finalLines = append(finalLines, line)
				} else {
					// It's an active line but not mandatory, keep as is
					finalLines = append(finalLines, line)
				}
			} else {
				finalLines = append(finalLines, line)
			}
		}
	}

	// 3. Add any missing mandatory variables that weren't found in .env.example
	for key, val := range mandatory {
		if !seen[key] {
			// Special handling: Skip adding DATABASE_URL if it's not already present in the file.
			// This avoids redundancy for projects that use individual DB_* variables (standard Laravel).
			if key == "DATABASE_URL" {
				continue
			}
			finalLines = append(finalLines, fmt.Sprintf("%s=%s", key, val))
		}
	}

	return os.WriteFile(envPath, []byte(strings.Join(finalLines, "\n")), 0644)
}

// GetEnvFile reads the .env file for a project
func (s *DockerService) GetEnvFile(subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project
func (s *DockerService) SaveEnvFile(subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(content), 0644)
}

// loadMandatoryEnv renders the default.env template and returns a map of key-values.
func (s *DockerService) loadMandatoryEnv(project *models.Project, projectDomain string) (map[string]string, error) {
	templatePath := filepath.Join(s.cfg.TemplatesPath, "default.env")
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	// Prepare template data
	key := make([]byte, 32)
	var appKey string
	if _, err := rand.Read(key); err != nil {
		slog.Error("Failed to generate random bytes for APP_KEY", "error", err)
		appKey = "generation-failed-check-logs"
	} else {
		appKey = base64.StdEncoding.EncodeToString(key)
	}

	queueConn := "sync"
	if project.QueueEnabled {
		queueConn = "database"
	}

	data := struct {
		ProjectName      string
		Subdomain        string
		Domain           string
		AppKey           string
		DatabaseName     string
		DatabasePassword string
		QueueConnection  string
	}{
		ProjectName:      project.Name,
		Subdomain:        project.Subdomain,
		Domain:           projectDomain,
		AppKey:           "base64:" + appKey,
		DatabaseName:     project.DatabaseName,
		DatabasePassword: project.DatabasePassword,
		QueueConnection:  queueConn,
	}

	tmpl, err := template.New("env").Parse(string(content))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}

	// Parse rendered template into map
	result := make(map[string]string)
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	// Dynamic override for PostgreSQL if active
	if project.DatabaseInstance != nil && project.DatabaseInstance.Engine == "postgresql" {
		result["DB_CONNECTION"] = "pgsql"
		result["DB_HOST"] = "paas-user-postgres"
		result["DB_PORT"] = "5432"
		result["DATABASE_URL"] = fmt.Sprintf("postgres://%s:%s@paas-user-postgres:5432/%s?sslmode=disable", project.DatabaseName, project.DatabasePassword, project.DatabaseName)
	}

	return result, nil
}

// parseProjectEnv reads a .env file and returns a map of key-values.
// It handles basic environment variable parsing including comments and quotes.
func (s *DockerService) parseProjectEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	envVars := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Split by the first '=' character
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if they exist (e.g., "value" or 'value')
		value = strings.Trim(value, "\"'")

		envVars[key] = value
	}

	return envVars, nil
}
