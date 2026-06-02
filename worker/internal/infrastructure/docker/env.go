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

type DBConfig struct {
	Connection  string
	Host        string
	Port        string
	DatabaseURL string
}

type EngineConfig struct {
	Connection string
	Host       string
	Port       string
	GetURL     func(project *models.Project) string
}

// DBRegistry holds the configuration adapters for all database engines supported by the PaaS.
// To add support for a new engine (e.g. MongoDB, Redis, SQLite, etc.) in the future,
// simply declare its configuration in this registry. Zero core code changes required.
var DBRegistry = map[string]EngineConfig{
	"mysql": {
		Connection: "mysql",
		Host:       "paas-mysql",
		Port:       "3306",
		GetURL: func(p *models.Project) string {
			return fmt.Sprintf("mysql://%s:%s@paas-mysql:3306/%s", p.DatabaseName, p.DatabasePassword, p.DatabaseName)
		},
	},
	"postgresql": {
		Connection: "pgsql",
		Host:       "paas-user-postgres",
		Port:       "5432",
		GetURL: func(p *models.Project) string {
			return fmt.Sprintf("postgres://%s:%s@paas-user-postgres:5432/%s?sslmode=disable", p.DatabaseName, p.DatabasePassword, p.DatabaseName)
		},
	},
	"mongodb": {
		Connection: "mongodb",
		Host:       "paas-mongodb",
		Port:       "27017",
		GetURL: func(p *models.Project) string {
			return fmt.Sprintf("mongodb://%s:%s@paas-mongodb:27017/%s", p.DatabaseName, p.DatabasePassword, p.DatabaseName)
		},
	},
}

// getDBConfig returns the environment variables for the specified database engine.
func getDBConfig(engine string, project *models.Project) DBConfig {
	cfg, exists := DBRegistry[engine]
	if !exists {
		// Fallback to MySQL if engine is unsupported or empty
		cfg = DBRegistry["mysql"]
	}
	return DBConfig{
		Connection:  cfg.Connection,
		Host:        cfg.Host,
		Port:        cfg.Port,
		DatabaseURL: cfg.GetURL(project),
	}
}

// UpdateDatabaseCredentialsInEnv safely updates all database connection and credential variables in an existing .env file.
// It auto-detects changes in the database engine (e.g. MySQL to PostgreSQL) and dynamically heals the file.
func (s *DockerService) UpdateDatabaseCredentialsInEnv(project *models.Project) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)
	envPath := filepath.Join(projectPath, ".env")

	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // If it doesn't exist, CreateEnvFile will handle it anyway
		}
		return err
	}

	engine := "mysql"
	if project.DatabaseInstance != nil {
		engine = project.DatabaseInstance.Engine
	}
	dbCfg := getDBConfig(engine, project)

	// Define the map of environment variable updates we want to apply to the .env file.
	// This makes the replacement logic completely DRY and generic.
	updates := map[string]string{
		"DB_CONNECTION": dbCfg.Connection,
		"DB_HOST":       dbCfg.Host,
		"DB_PORT":       dbCfg.Port,
		"DB_DATABASE":   project.DatabaseName,
		"DB_USERNAME":   project.DatabaseName,
		"DB_PASSWORD":   project.DatabasePassword,
		"DATABASE_URL":  dbCfg.DatabaseURL,
	}

	lines := strings.Split(string(data), "\n")
	var finalLines []string
	dbPasswordUpdated := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// If it's a comment or empty line, preserve it
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			finalLines = append(finalLines, line)
			continue
		}

		updated := false
		for key, val := range updates {
			if strings.HasPrefix(trimmed, key+"=") {
				finalLines = append(finalLines, fmt.Sprintf("%s=%s", key, val))
				if key == "DB_PASSWORD" {
					dbPasswordUpdated = true
				}
				updated = true
				break
			}
		}

		if !updated {
			finalLines = append(finalLines, line)
		}
	}

	// If DB_PASSWORD wasn't found in the file, we add it at the end
	if !dbPasswordUpdated && project.DatabasePassword != "" {
		finalLines = append(finalLines, fmt.Sprintf("DB_PASSWORD=%s", project.DatabasePassword))
	}

	tempPath := envPath + ".tmp"
	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	if _, err := f.Write([]byte(strings.Join(finalLines, "\n"))); err != nil {
		f.Close()
		return err
	}
	
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return os.Rename(tempPath, envPath)
}
