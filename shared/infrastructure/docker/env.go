package docker

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CreateEnvFile compiles the environment configurati		on from SecretStore and writes it atomically to the project directory.
func (s *DockerService) CreateEnvFile(project *models.Project, projectDomain string, isInitial bool) error {
	projectPath := project.GetProjectPath(s.cfg.ProjectsPath)
	envPath := filepath.Join(projectPath, ".env")

	// Compile the complete env key-value map.
	envMap, err := s.CompileEnvForProject(project.ID, project.UserID, project.Subdomain, project.GetDatabaseName(), project.DatabasePassword, project.Framework, "production")
	if err != nil {
		return fmt.Errorf("failed to compile env for project %s: %w", project.Name, err)
	}

	envContent := utils.FormatEnvMap(envMap)

	// Ensure parent directories exist before writing.
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	return utils.WriteFileAtomic(envPath, []byte(envContent), 0644)
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

	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return err
	}

	return utils.WriteFileAtomic(envPath, []byte(content), 0644)
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
	projectDomain := s.cfg.ProjectDomain
	var settingVal models.Setting
	if err := s.db.Where("key = ?", models.SettingProjectDomain).First(&settingVal).Error; err == nil && settingVal.Value != "" {
		projectDomain = settingVal.Value
	}
	appURL := fmt.Sprintf("http://%s.%s", subdomain, projectDomain)
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
	}

	// Layer 3: SecretStore Bindings (Custom Env).
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("project_id = ? AND (environment = ? OR environment = 'all' OR environment = '')", projectID, envName).Order("created_at ASC").Find(&bindings).Error; err == nil {
		currentKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
		decryptionKeys := utils.CredentialDecryptionKeys(s.cfg.CredentialEncryptionKey, s.cfg.CredentialEncryptionPreviousKeys)

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
						result, err := utils.DecryptWithResult(latestVal.EncryptedValue, decryptionKeys...)
						if err != nil {
							if s.cfg.CredentialEncryptionAllowInsecurePrevious {
								result = utils.DecryptionResult{Plaintext: latestVal.EncryptedValue, UsedFallbackKey: true}
							} else {
								return nil, secretDecryptError(projectID, b.SecretStoreID, item.ID, err)
							}
						}
						if !isSystemManagedEnvKey(item.Key) {
							envMap[item.Key] = result.Plaintext
						}
						if result.UsedFallbackKey {
							if err := rotateSecretValueToCurrentKey(s.db, userID, b.SecretStoreID, item.ID, item.LatestSnapshotVersion, projectID, currentKey, result.Plaintext); err != nil {
								slog.Warn("Failed to re-encrypt SecretStore value with current credential key", "projectID", projectID, "secret_store_id", b.SecretStoreID, "item_id", item.ID, "error", err)
							}
						}
					}
				}
			}
		}
	}

	// Layer 4: Laravel APP_KEY auto-provisioning
	if framework == "Laravel" {
		if existingAppKey, ok := envMap["APP_KEY"]; ok {
			if strings.TrimSpace(existingAppKey) == "" {
				return nil, emptyLaravelAppKeyError(projectID)
			}
		} else {
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
				currentKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
				decryptionKeys := utils.CredentialDecryptionKeys(s.cfg.CredentialEncryptionKey, s.cfg.CredentialEncryptionPreviousKeys)

				if errItem != nil {
					if errors.Is(errItem, gorm.ErrRecordNotFound) {
						// APP_KEY not found in DB, generate one.
						generatedKey, randErr := utils.GenerateLaravelAppKey()
						if randErr != nil {
							return randErr
						}
						appKey = generatedKey
						encryptedVal, encErr := utils.Encrypt(appKey, currentKey)
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
						if err := createManagedSecretActivityLogTx(tx, userID, storeID, newItem.ID, projectID, "Provisioned managed Laravel APP_KEY"); err != nil {
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
					result, decErr := utils.DecryptWithResult(val.EncryptedValue, decryptionKeys...)
					if decErr != nil {
						if s.cfg.CredentialEncryptionAllowInsecurePrevious {
							result = utils.DecryptionResult{Plaintext: val.EncryptedValue, UsedFallbackKey: true}
						} else {
							return secretDecryptError(projectID, storeID, item.ID, decErr)
						}
					}
					appKey = result.Plaintext
					if result.UsedFallbackKey {
						if err := rotateSecretValueToCurrentKey(tx, userID, storeID, item.ID, item.LatestSnapshotVersion, projectID, currentKey, result.Plaintext); err != nil {
							return err
						}
					}
				}
				return nil
			})

			if errTx == nil && strings.TrimSpace(appKey) != "" {
				envMap["APP_KEY"] = appKey
			} else if errTx != nil {
				slog.Error("Failed to auto-provision APP_KEY for Laravel project", "projectID", projectID, "error", errTx)
				return nil, errTx
			} else {
				return nil, emptyLaravelAppKeyError(projectID)
			}
		}
	}

	return envMap, nil
}

func secretDecryptError(projectID uint, storeID uint, itemID uint, err error) error {
	appErr := apperr.NewSecretDecryptionUnavailable(err)
	appErr.Details = map[string]uint{
		"project_id":      projectID,
		"secret_store_id": storeID,
		"item_id":         itemID,
	}
	return appErr
}

func emptyLaravelAppKeyError(projectID uint) error {
	appErr := apperr.NewSecretDecryptionFailed("Laravel APP_KEY is empty. Restore the managed APP_KEY value before deploying.", nil)
	appErr.Details = map[string]uint{"project_id": projectID}
	return appErr
}

func createManagedSecretActivityLogTx(tx *gorm.DB, userID uint, storeID uint, itemID uint, projectID uint, details string) error {
	log := models.SecretStoreActivityLog{
		UserID:            userID,
		SecretStoreID:     &storeID,
		SecretStoreItemID: &itemID,
		ProjectID:         &projectID,
		Action:            "set_secret_value",
		Details:           details,
	}
	return tx.Create(&log).Error
}

func rotateSecretValueToCurrentKey(db *gorm.DB, userID uint, storeID uint, itemID uint, expectedVersion int, projectID uint, currentKey []byte, plaintext string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var item models.SecretStoreItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
			return err
		}
		if item.LatestSnapshotVersion != expectedVersion {
			return nil
		}

		encryptedVal, err := utils.Encrypt(plaintext, currentKey)
		if err != nil {
			return err
		}

		nextVersion := item.LatestSnapshotVersion + 1
		itemValue := models.SecretStoreItemValue{
			SecretStoreItemID: item.ID,
			Version:           nextVersion,
			EncryptedValue:    encryptedVal,
			CreatedBy:         userID,
		}
		if err := tx.Create(&itemValue).Error; err != nil {
			return err
		}
		if err := tx.Model(&item).Update("latest_snapshot_version", nextVersion).Error; err != nil {
			return err
		}
		return createManagedSecretActivityLogTx(tx, userID, storeID, item.ID, projectID, "Re-encrypted secret value with active credential key")
	})
}

func isSystemManagedEnvKey(key string) bool {
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "APP_NAME", "APP_URL":
		return true
	default:
		return false
	}
}
