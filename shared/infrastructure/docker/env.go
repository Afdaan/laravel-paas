package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// CreateEnvFile compiles the environment configuration from SecretStore and writes it atomically to the project directory.
func (s *DockerService) CreateEnvFile(project *models.Project, projectDomain string, isInitial bool) error {
	projectPath := project.GetProjectPath(s.cfg.ProjectsPath)
	envPath := filepath.Join(projectPath, ".env")
	tempPath := envPath + ".tmp"

	// Compile the complete env key-value map.
	envMap, err := s.CompileEnvForProject(project.ID, project.UserID, project.Subdomain, project.DatabaseName, project.DatabasePassword, project.Framework, "production")
	if err != nil {
		return fmt.Errorf("failed to compile env for project %s: %w", project.Name, err)
	}

	var builder strings.Builder
	for k, v := range envMap {
		builder.WriteString(fmt.Sprintf("%s=%s\n", k, v))
	}

	// Ensure parent directories exist before writing.
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	if _, err := f.Write([]byte(builder.String())); err != nil {
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

// GetEnvFile reads the .env file for a project.
func (s *DockerService) GetEnvFile(userID uint, subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, fmt.Sprintf("user-%d", userID), subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project using an atomic write pattern.
func (s *DockerService) SaveEnvFile(userID uint, subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, fmt.Sprintf("user-%d", userID), subdomain)
	envPath := filepath.Join(projectPath, ".env")
	tempPath := envPath + ".tmp"

	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	if _, err := f.Write([]byte(content)); err != nil {
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

// UpdateDatabaseCredentialsInEnv is now a no-op as all environment variables are compiled directly from GORM.
func (s *DockerService) UpdateDatabaseCredentialsInEnv(project *models.Project) error {
	return nil
}

// CompileEnvForProject compiles variables from the default, DB, and SecretStore layers.
func (s *DockerService) CompileEnvForProject(projectID uint, userID uint, subdomain string, databaseName string, databasePassword string, framework string, envName string) (map[string]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized in docker service")
	}

	envMap := make(map[string]string)

	// Layer 1: PaaS defaults.
	envMap["APP_NAME"] = subdomain
	envMap["APP_ENV"] = envName
	envMap["APP_DEBUG"] = "false"
	envMap["APP_URL"] = fmt.Sprintf("http://%s", subdomain)
	if framework == "Laravel" {
		envMap["LOG_CHANNEL"] = "stack"
		envMap["LOG_DEPRECATIONS_CHANNEL"] = "null"
		envMap["LOG_LEVEL"] = "debug"
	}

	// Layer 2: Autoprovisioned database connection parameters.
	var dbInst models.DatabaseInstance
	if err := s.db.Where("project_id = ? AND status = 'active'", projectID).First(&dbInst).Error; err == nil {
		switch dbInst.Engine {
		case "mysql":
			envMap["DB_CONNECTION"] = "mysql"
			envMap["DB_HOST"] = dbInst.Host
			envMap["DB_PORT"] = fmt.Sprintf("%d", dbInst.Port)
			envMap["DB_DATABASE"] = dbInst.Name
			envMap["DB_USERNAME"] = dbInst.Username
			envMap["DB_PASSWORD"] = dbInst.Password
		case "postgresql":
			envMap["DB_CONNECTION"] = "pgsql"
			envMap["DB_HOST"] = dbInst.Host
			envMap["DB_PORT"] = fmt.Sprintf("%d", dbInst.Port)
			envMap["DB_DATABASE"] = dbInst.Name
			envMap["DB_USERNAME"] = dbInst.Username
			envMap["DB_PASSWORD"] = dbInst.Password
			envMap["DATABASE_URL"] = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
				dbInst.Username, dbInst.Password, dbInst.Host, dbInst.Port, dbInst.Name)
		}
	} else {
		// Fallback to project-level MySQL defaults if DatabaseInstance is missing/not provisioning yet.
		envMap["DB_CONNECTION"] = "mysql"
		envMap["DB_HOST"] = "paas-mysql"
		envMap["DB_PORT"] = "3306"
		envMap["DB_DATABASE"] = databaseName
		envMap["DB_USERNAME"] = databaseName
		envMap["DB_PASSWORD"] = databasePassword
		envMap["DATABASE_URL"] = fmt.Sprintf("mysql://%s:%s@paas-mysql:3306/%s", databaseName, databasePassword, databaseName)
	}

	// Layer 3: SecretStore Bindings (Custom Env).
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("project_id = ? AND (environment = ? OR environment = 'all' OR environment = '')", projectID, envName).Order("created_at ASC").Find(&bindings).Error; err == nil {
		stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)

		for _, b := range bindings {
			var items []models.SecretStoreItem
			if err := s.db.Where("secret_store_id = ?", b.SecretStoreID).Preload("Values").Find(&items).Error; err == nil {
				for _, item := range items {
					var latestVal *models.SecretStoreItemValue
					for i := range item.Values {
						val := &item.Values[i]
						if val.Version == item.LatestSnapshotVersion {
							latestVal = val
							break
						}
					}

					if latestVal != nil {
						decrypted, err := utils.Decrypt(latestVal.EncryptedValue, stretchedKey)
						if err == nil {
							envMap[item.Key] = decrypted
						}
					}
				}
			}
		}
	}

	return envMap, nil
}
