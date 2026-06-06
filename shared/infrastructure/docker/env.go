package docker

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateEnvFile compiles the environment configurati		on from SecretStore and writes it atomically to the project directory.
func (s *DockerService) CreateEnvFile(project *models.Project, projectDomain string, isInitial bool) error {
	projectPath := project.GetProjectPath(s.cfg.ProjectsPath)
	envPath := filepath.Join(projectPath, ".env")
	tempPath := envPath + ".tmp"

	// Compile the complete env key-value map.
	envMap, err := s.CompileEnvForProject(project.ID, project.UserID, project.Subdomain, project.DatabaseName, project.DatabasePassword, project.Framework, "production")
	if err != nil {
		return fmt.Errorf("failed to compile env for project %s: %w", project.Name, err)
	}

	envContent := utils.FormatEnvMap(envMap)

	// Ensure parent directories exist before writing.
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	if _, err := f.Write([]byte(envContent)); err != nil {
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
	projectPath := filepath.Join(s.cfg.ProjectsPath, models.GetUserDirName(s.db, userID), subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project using an atomic write pattern.
func (s *DockerService) SaveEnvFile(userID uint, subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, models.GetUserDirName(s.db, userID), subdomain)
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

	// Resolve APP_URL dynamically based on custom domains
	appURL := fmt.Sprintf("http://%s", subdomain)
	var domains []models.CustomDomain
	if err := s.db.Where("project_id = ? AND status IN (?)", projectID, []string{string(models.DomainStatusActive), string(models.DomainStatusSSLActive)}).Order("created_at ASC").Find(&domains).Error; err == nil {
		var primaryDomain string
		var firstActiveDomain string
		for _, d := range domains {
			if d.IsPrimary {
				primaryDomain = d.Domain
				break
			}
			if firstActiveDomain == "" {
				firstActiveDomain = d.Domain
			}
		}
		if primaryDomain != "" {
			appURL = fmt.Sprintf("https://%s", primaryDomain)
		} else if firstActiveDomain != "" {
			appURL = fmt.Sprintf("https://%s", firstActiveDomain)
		}
	}
	envMap["APP_URL"] = appURL

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
		legacyKey := utils.DeriveKeyLegacy(s.cfg.CredentialEncryptionKey)

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
						decrypted, err := utils.Decrypt(latestVal.EncryptedValue, stretchedKey, legacyKey)
						if err == nil {
							envMap[item.Key] = decrypted
						}
					}
				}
			}
		}
	}

	// Layer 4: Laravel APP_KEY auto-provisioning
	if framework == "Laravel" {
		if _, ok := envMap["APP_KEY"]; !ok {
			var appKey string
			errTx := s.db.Transaction(func(tx *gorm.DB) error {
				// Lock project to prevent concurrent creation of duplicate SecretStores.
				var project models.Project
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&project, projectID).Error; err != nil {
					return err
				}

				var binding models.SecretStoreBinding
				err := tx.Where("project_id = ?", projectID).First(&binding).Error
				var storeID uint
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// No binding exists. Create a new SecretStore and bind it.
						store := models.SecretStore{
							UserID:      userID,
							Name:        fmt.Sprintf("Environment Secrets (%s)", subdomain),
							Description: "Managed variables for project " + subdomain,
						}
						if err := tx.Create(&store).Error; err != nil {
							return err
						}
						storeID = store.ID
						newBinding := models.SecretStoreBinding{
							ProjectID:     projectID,
							SecretStoreID: store.ID,
							Environment:   "production",
						}
						if err := tx.Create(&newBinding).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				} else {
					storeID = binding.SecretStoreID
				}

				var item models.SecretStoreItem
				errItem := tx.Where("secret_store_id = ? AND key = ?", storeID, "APP_KEY").First(&item).Error
				stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)

				if errItem != nil {
					if errors.Is(errItem, gorm.ErrRecordNotFound) {
						// APP_KEY not found in DB, generate one.
						keyBytes := make([]byte, 32)
						if _, randErr := rand.Read(keyBytes); randErr != nil {
							return randErr
						}
						appKey = "base64:" + base64.StdEncoding.EncodeToString(keyBytes)
						encryptedVal, encErr := utils.Encrypt(appKey, stretchedKey)
						if encErr != nil {
							return encErr
						}
						newItem := models.SecretStoreItem{
							SecretStoreID:         storeID,
							Key:                   "APP_KEY",
							LatestSnapshotVersion: 1,
						}
						if err := tx.Create(&newItem).Error; err != nil {
							return err
						}
						itemVal := models.SecretStoreItemValue{
							SecretStoreItemID: newItem.ID,
							Version:           1,
							EncryptedValue:    encryptedVal,
							CreatedBy:         userID,
						}
						if err := tx.Create(&itemVal).Error; err != nil {
							return err
						}
					} else {
						return errItem
					}
				} else {
					// APP_KEY item exists, retrieve and decrypt its value.
					var val models.SecretStoreItemValue
					if errVal := tx.Where("secret_store_item_id = ? AND version = ?", item.ID, item.LatestSnapshotVersion).First(&val).Error; errVal != nil {
						return errVal
					}
					legacyKey := utils.DeriveKeyLegacy(s.cfg.CredentialEncryptionKey)
					decrypted, decErr := utils.Decrypt(val.EncryptedValue, stretchedKey, legacyKey)
					if decErr != nil {
						return decErr
					}
					appKey = decrypted
				}
				return nil
			})

			if errTx == nil && appKey != "" {
				envMap["APP_KEY"] = appKey
			} else if errTx != nil {
				slog.Error("Failed to auto-provision APP_KEY for Laravel project", "projectID", projectID, "error", errTx)
			}
		}
	}

	return envMap, nil
}
