package services

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SecretStoreService handles secure business logic for secret stores.
type SecretStoreService struct {
	db           *gorm.DB
	cfg          *config.Config
	redisService *infrastructure.RedisService
}

func NewSecretStoreService(db *gorm.DB, cfg *config.Config, redisService *infrastructure.RedisService) *SecretStoreService {
	return &SecretStoreService{
		db:           db,
		cfg:          cfg,
		redisService: redisService,
	}
}

func (s *SecretStoreService) CreateSecretStore(userID uint, name, description string, ipAddress, userAgent string) (*models.SecretStore, error) {
	store := &models.SecretStore{
		UserID:      userID,
		Name:        name,
		Description: description,
	}
	if err := s.db.Create(store).Error; err != nil {
		return nil, err
	}
	s.LogActivity(userID, &store.ID, nil, nil, "create_secretstore", "Created secret store container", ipAddress, userAgent)
	return store, nil
}

func (s *SecretStoreService) GetSecretStore(userID uint, storeID uint) (*models.SecretStore, error) {
	var store models.SecretStore
	if err := s.db.Preload("Items.Values").Preload("Bindings.Project").Where("id = ? AND user_id = ?", storeID, userID).First(&store).Error; err != nil {
		return nil, err
	}
	return &store, nil
}

func (s *SecretStoreService) ListSecretStores(userID uint) ([]models.SecretStore, error) {
	var stores []models.SecretStore
	if err := s.db.Preload("Bindings").Where("user_id = ?", userID).Find(&stores).Error; err != nil {
		return nil, err
	}
	return stores, nil
}

func (s *SecretStoreService) UpdateSecretStore(userID uint, storeID uint, name, description string, ipAddress, userAgent string) (*models.SecretStore, error) {
	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return nil, err
	}
	store.Name = name
	store.Description = description
	if err := s.db.Save(store).Error; err != nil {
		return nil, err
	}
	s.LogActivity(userID, &storeID, nil, nil, "update_secretstore", "Updated secret store container metadata", ipAddress, userAgent)
	return store, nil
}

func (s *SecretStoreService) DeleteSecretStore(userID uint, storeID uint, ipAddress, userAgent string) error {
	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return err
	}

	// Remove bindings and clean up GORM models inside transaction.
	errTx := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("secret_store_id = ?", storeID).Delete(&models.SecretStoreBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(store).Error; err != nil {
			return err
		}
		return nil
	})

	if errTx != nil {
		return errTx
	}

	s.LogActivity(userID, &storeID, nil, nil, "delete_secretstore", "Soft-deleted secret store container", ipAddress, userAgent)
	return nil
}

func (s *SecretStoreService) SetSecretValue(userID uint, storeID uint, key, value string, ipAddress, userAgent string) (*models.SecretStoreItem, error) {
	if s.IsBaselineMatchForStore(storeID, key, value) {
		var item models.SecretStoreItem
		if errItem := s.db.Where("secret_store_id = ? AND key = ?", storeID, key).First(&item).Error; errItem == nil {
			s.db.Delete(&item)
			s.LogActivity(userID, &storeID, &item.ID, nil, "delete_secret_key", "Removed baseline-matching secret key: "+key, ipAddress, userAgent)
		}
		return &models.SecretStoreItem{Key: key, SecretStoreID: storeID}, nil
	}

	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return nil, err
	}

	stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
	encryptedVal, errEnc := utils.Encrypt(value, stretchedKey)
	if errEnc != nil {
		return nil, fmt.Errorf("failed to encrypt value: %w", errEnc)
	}

	var item models.SecretStoreItem
	var version int = 1
	errTx := s.db.Transaction(func(tx *gorm.DB) error {
		// Query inside the transaction with pessimistic locking to prevent race conditions
		errItem := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("secret_store_id = ? AND key = ?", store.ID, key).First(&item).Error

		if errors.Is(errItem, gorm.ErrRecordNotFound) {
			item = models.SecretStoreItem{
				SecretStoreID:         store.ID,
				Key:                   key,
				LatestSnapshotVersion: 1,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			version = 1
		} else if errItem == nil {
			version = item.LatestSnapshotVersion + 1
			item.LatestSnapshotVersion = version
			if err := tx.Save(&item).Error; err != nil {
				return err
			}
		} else {
			return errItem
		}

		itemValue := models.SecretStoreItemValue{
			SecretStoreItemID: item.ID,
			Version:           version,
			EncryptedValue:    encryptedVal,
			CreatedBy:         userID,
		}
		if err := tx.Create(&itemValue).Error; err != nil {
			return err
		}

		return nil
	})

	if errTx != nil {
		return nil, errTx
	}

	s.LogActivity(userID, &storeID, &item.ID, nil, "set_secret_value", fmt.Sprintf("Set value for key %s (version %d)", key, version), ipAddress, userAgent)

	// Propagate updates asynchronously using safe panic recovery wrapper.
	utils.SafeGo(func() {
		s.PropagateSecretStoreUpdates(storeID)
	})

	return &item, nil
}

func (s *SecretStoreService) SetSecretValueNoPropagate(userID uint, storeID uint, key, value string, ipAddress, userAgent string) (*models.SecretStoreItem, error) {
	if s.IsBaselineMatchForStore(storeID, key, value) {
		var item models.SecretStoreItem
		if errItem := s.db.Where("secret_store_id = ? AND key = ?", storeID, key).First(&item).Error; errItem == nil {
			s.db.Delete(&item)
			s.LogActivity(userID, &storeID, &item.ID, nil, "delete_secret_key", "Removed baseline-matching secret key: "+key, ipAddress, userAgent)
		}
		return &models.SecretStoreItem{Key: key, SecretStoreID: storeID}, nil
	}

	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return nil, err
	}

	stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
	encryptedVal, errEnc := utils.Encrypt(value, stretchedKey)
	if errEnc != nil {
		return nil, fmt.Errorf("failed to encrypt value: %w", errEnc)
	}

	var item models.SecretStoreItem
	var version int = 1
	errTx := s.db.Transaction(func(tx *gorm.DB) error {
		// Query inside the transaction with pessimistic locking to prevent race conditions
		errItem := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("secret_store_id = ? AND key = ?", store.ID, key).First(&item).Error

		if errors.Is(errItem, gorm.ErrRecordNotFound) {
			item = models.SecretStoreItem{
				SecretStoreID:         store.ID,
				Key:                   key,
				LatestSnapshotVersion: 1,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			version = 1
		} else if errItem == nil {
			version = item.LatestSnapshotVersion + 1
			item.LatestSnapshotVersion = version
			if err := tx.Save(&item).Error; err != nil {
				return err
			}
		} else {
			return errItem
		}

		itemValue := models.SecretStoreItemValue{
			SecretStoreItemID: item.ID,
			Version:           version,
			EncryptedValue:    encryptedVal,
			CreatedBy:         userID,
		}
		if err := tx.Create(&itemValue).Error; err != nil {
			return err
		}

		return nil
	})

	if errTx != nil {
		return nil, errTx
	}

	s.LogActivity(userID, &storeID, &item.ID, nil, "set_secret_value", fmt.Sprintf("Set value for key %s (version %d)", key, version), ipAddress, userAgent)

	return &item, nil
}

func (s *SecretStoreService) SetSecretValueInternal(userID uint, storeID uint, key, value string) (*models.SecretStoreItem, error) {
	var item models.SecretStoreItem
	errItem := s.db.Where("secret_store_id = ? AND key = ?", storeID, key).First(&item).Error

	stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
	encryptedVal, errEnc := utils.Encrypt(value, stretchedKey)
	if errEnc != nil {
		return nil, fmt.Errorf("failed to encrypt value: %w", errEnc)
	}

	var version int = 1
	if errors.Is(errItem, gorm.ErrRecordNotFound) {
		item = models.SecretStoreItem{
			SecretStoreID:         storeID,
			Key:                   key,
			LatestSnapshotVersion: 1,
		}
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else if errItem == nil {
		version = item.LatestSnapshotVersion + 1
		item.LatestSnapshotVersion = version
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	} else {
		return nil, errItem
	}

	itemValue := models.SecretStoreItemValue{
		SecretStoreItemID: item.ID,
		Version:           version,
		EncryptedValue:    encryptedVal,
		CreatedBy:         userID,
	}
	if err := s.db.Create(&itemValue).Error; err != nil {
		return nil, err
	}

	s.LogActivity(userID, &storeID, &item.ID, nil, "set_secret_value", fmt.Sprintf("Set database credentials key %s (version %d)", key, version), "", "")
	return &item, nil
}

func (s *SecretStoreService) BindSecretStore(userID uint, storeID, projectID uint, environment string, ipAddress, userAgent string) (*models.SecretStoreBinding, error) {
	var project models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return nil, errors.New("project not found or unauthorized")
	}

	// Verify SecretStore ownership to prevent cross-tenant IDOR binding vulnerabilities
	var store models.SecretStore
	if err := s.db.Where("id = ? AND user_id = ?", storeID, userID).First(&store).Error; err != nil {
		return nil, errors.New("secret store not found or unauthorized")
	}

	binding := &models.SecretStoreBinding{
		ProjectID:     projectID,
		SecretStoreID: storeID,
		Environment:   environment,
	}
	if err := s.db.Create(binding).Error; err != nil {
		return nil, err
	}

	s.LogActivity(userID, &storeID, nil, &projectID, "bind_secretstore", fmt.Sprintf("Bound secret store container to project environment %s", environment), ipAddress, userAgent)

	// Trigger env propagation for the newly bound project.
	utils.SafeGo(func() {
		s.PropagateSecretStoreUpdates(storeID)
	})

	return binding, nil
}

func (s *SecretStoreService) UnbindSecretStore(userID uint, storeID, bindingID uint, ipAddress, userAgent string) error {
	// Verify that the SecretStore belongs to this user
	var store models.SecretStore
	if err := s.db.Where("id = ? AND user_id = ?", storeID, userID).First(&store).Error; err != nil {
		return errors.New("secret store not found or unauthorized")
	}

	// Find the binding and verify it belongs to this SecretStore
	var binding models.SecretStoreBinding
	if err := s.db.Where("id = ? AND secret_store_id = ?", bindingID, storeID).First(&binding).Error; err != nil {
		return errors.New("binding not found")
	}

	// Delete the binding
	if err := s.db.Delete(&binding).Error; err != nil {
		return err
	}

	s.LogActivity(userID, &storeID, nil, &binding.ProjectID, "unbind_secretstore", "Unbound secret store container from project", ipAddress, userAgent)

	// Trigger env propagation to clear unbound keys for remaining projects and the unbound project.
	utils.SafeGo(func() {
		s.PropagateSecretStoreUpdates(storeID)
		s.PropagateProjectEnvUpdate(binding.ProjectID, userID)
	})

	return nil
}

func (s *SecretStoreService) UpdateDatabaseSecrets(project *models.Project, username, password string) error {
	var binding models.SecretStoreBinding
	err := s.db.Where("project_id = ?", project.ID).First(&binding).Error
	if err == nil {
		utils.SafeGo(func() {
			s.PropagateSecretStoreUpdates(binding.SecretStoreID)
		})
	} else {
		utils.SafeGo(func() {
			s.PropagateProjectEnvUpdate(project.ID, project.UserID)
		})
	}
	return nil
}

func (s *SecretStoreService) CompileEnvForProject(projectID uint, environment string) (map[string]string, error) {
	var project models.Project
	if err := s.db.Preload("DatabaseInstance").Preload("CustomDomains").First(&project, projectID).Error; err != nil {
		return nil, err
	}

	envMap := make(map[string]string)

	// Layer 1: PaaS defaults.
	envMap["APP_NAME"] = project.Name
	envMap["APP_ENV"] = environment
	envMap["APP_DEBUG"] = "false"

	// Resolve APP_URL dynamically based on custom domains
	appURL := fmt.Sprintf("http://%s", project.Subdomain)
	var primaryDomain string
	var firstActiveDomain string
	for _, d := range project.CustomDomains {
		if d.Status == models.DomainStatusActive || d.Status == models.DomainStatusSSLActive {
			if d.IsPrimary {
				primaryDomain = d.Domain
				break
			}
			if firstActiveDomain == "" {
				firstActiveDomain = d.Domain
			}
		}
	}
	if primaryDomain != "" {
		appURL = fmt.Sprintf("https://%s", primaryDomain)
	} else if firstActiveDomain != "" {
		appURL = fmt.Sprintf("https://%s", firstActiveDomain)
	}
	envMap["APP_URL"] = appURL

	if project.Framework == "Laravel" {
		envMap["LOG_CHANNEL"] = "stack"
		envMap["LOG_DEPRECATIONS_CHANNEL"] = "null"
		envMap["LOG_LEVEL"] = "debug"
	}

	// Layer 2: Autoprovisioned database connection parameters.
	if project.DatabaseInstance != nil && project.DatabaseInstance.Status == models.DBStatusActive {
		inst := project.DatabaseInstance
		switch inst.Engine {
		case "mysql":
			envMap["DB_CONNECTION"] = "mysql"
			envMap["DB_HOST"] = inst.Host
			envMap["DB_PORT"] = fmt.Sprintf("%d", inst.Port)
			envMap["DB_DATABASE"] = inst.Name
			envMap["DB_USERNAME"] = inst.Username
			envMap["DB_PASSWORD"] = inst.Password
		case "postgresql":
			envMap["DB_CONNECTION"] = "pgsql"
			envMap["DB_HOST"] = inst.Host
			envMap["DB_PORT"] = fmt.Sprintf("%d", inst.Port)
			envMap["DB_DATABASE"] = inst.Name
			envMap["DB_USERNAME"] = inst.Username
			envMap["DB_PASSWORD"] = inst.Password
			envMap["DATABASE_URL"] = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
				inst.Username, inst.Password, inst.Host, inst.Port, inst.Name)
		}
	}

	// Layer 3: SecretStore bindings (Custom Env).
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("project_id = ? AND (environment = ? OR environment = 'all' OR environment = '')", project.ID, environment).Order("created_at ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}

	stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
	legacyKey := utils.DeriveKeyLegacy(s.cfg.CredentialEncryptionKey)

	for _, b := range bindings {
		var items []models.SecretStoreItem
		if err := s.db.Where("secret_store_id = ?", b.SecretStoreID).Preload("Values").Find(&items).Error; err != nil {
			continue
		}

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
				if err != nil {
					slog.Error("Failed to decrypt secret value during env compilation", "item_id", item.ID, "error", err)
					continue
				}
				envMap[item.Key] = decrypted
			}
		}
	}

	// Layer 4: Laravel APP_KEY auto-provisioning
	if project.Framework == "Laravel" {
		if _, ok := envMap["APP_KEY"]; !ok {
			var appKey string
			errTx := s.db.Transaction(func(tx *gorm.DB) error {
				// Lock project to prevent concurrent creation of duplicate SecretStores.
				var proj models.Project
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&proj, project.ID).Error; err != nil {
					return err
				}

				var binding models.SecretStoreBinding
				err := tx.Where("project_id = ?", project.ID).First(&binding).Error
				var storeID uint
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						// No binding exists. Create a new SecretStore and bind it.
						store := models.SecretStore{
							UserID:      project.UserID,
							Name:        fmt.Sprintf("Environment Secrets (%s)", project.Subdomain),
							Description: "Managed variables for project " + project.Subdomain,
						}
						if err := tx.Create(&store).Error; err != nil {
							return err
						}
						storeID = store.ID
						newBinding := models.SecretStoreBinding{
							ProjectID:     project.ID,
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
							CreatedBy:         project.UserID,
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
					if decErr == nil {
						appKey = decrypted
					}
				}
				return nil
			})

			if errTx == nil && appKey != "" {
				envMap["APP_KEY"] = appKey
			} else if errTx != nil {
				slog.Error("Failed to auto-provision APP_KEY for Laravel project", "projectID", project.ID, "error", errTx)
			}
		}
	}

	return envMap, nil
}

func (s *SecretStoreService) PropagateSecretStoreUpdates(storeID uint) {
	s.PropagateSecretStoreUpdatesExcept(storeID, 0)
}

func (s *SecretStoreService) PropagateSecretStoreUpdatesExcept(storeID uint, skipProjectID uint) {
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("secret_store_id = ?", storeID).Find(&bindings).Error; err == nil {
		for _, b := range bindings {
			if skipProjectID > 0 && b.ProjectID == skipProjectID {
				continue
			}
			var project models.Project
			if err := s.db.First(&project, b.ProjectID).Error; err == nil {
				jobID, errQueue := s.redisService.EnqueueDeployment(project.ID, project.UserID, "update_env")
				if errQueue == nil {
					slog.Info("Successfully enqueued update_env job for bound project", "project_id", project.ID, "job_id", jobID)
				} else {
					slog.Error("Failed to enqueue update_env job for bound project", "project_id", project.ID, "error", errQueue)
				}
			}
		}
	}
}

func (s *SecretStoreService) PropagateProjectEnvUpdate(projectID, userID uint) {
	jobID, errQueue := s.redisService.EnqueueDeployment(projectID, userID, "update_env")
	if errQueue == nil {
		slog.Info("Successfully enqueued update_env job for project", "project_id", projectID, "job_id", jobID)
	} else {
		slog.Error("Failed to enqueue update_env job for project", "project_id", projectID, "error", errQueue)
	}
}

func (s *SecretStoreService) LogActivity(userID uint, storeID, itemID, projectID *uint, action, details string, ipAddress, userAgent string) {
	log := models.SecretStoreActivityLog{
		UserID:            userID,
		SecretStoreID:     storeID,
		SecretStoreItemID: itemID,
		ProjectID:         projectID,
		Action:            action,
		Details:           details,
		IpAddress:         ipAddress,
		UserAgent:         userAgent,
	}
	if err := s.db.Create(&log).Error; err != nil {
		slog.Error("Failed to write secret store activity log", "error", err)
	}
}

// GetBaselineEnvMap generates Layer 1 + Layer 2 environment map for comparison.
func (s *SecretStoreService) GetBaselineEnvMap(project *models.Project) map[string]string {
	baseline := make(map[string]string)
	baseline["APP_NAME"] = project.Name
	baseline["APP_ENV"] = "production"
	baseline["APP_DEBUG"] = "false"

	appURL := fmt.Sprintf("http://%s", project.Subdomain)
	var primaryDomain string
	var firstActiveDomain string
	for _, d := range project.CustomDomains {
		if d.Status == models.DomainStatusActive || d.Status == models.DomainStatusSSLActive {
			if d.IsPrimary {
				primaryDomain = d.Domain
				break
			}
			if firstActiveDomain == "" {
				firstActiveDomain = d.Domain
			}
		}
	}
	if primaryDomain != "" {
		appURL = fmt.Sprintf("https://%s", primaryDomain)
	} else if firstActiveDomain != "" {
		appURL = fmt.Sprintf("https://%s", firstActiveDomain)
	}
	baseline["APP_URL"] = appURL

	if project.Framework == "Laravel" {
		baseline["LOG_CHANNEL"] = "stack"
		baseline["LOG_DEPRECATIONS_CHANNEL"] = "null"
		baseline["LOG_LEVEL"] = "debug"
	}

	if project.DatabaseInstance != nil && project.DatabaseInstance.Status == models.DBStatusActive {
		inst := project.DatabaseInstance
		switch inst.Engine {
		case "mysql":
			baseline["DB_CONNECTION"] = "mysql"
			baseline["DB_HOST"] = inst.Host
			baseline["DB_PORT"] = fmt.Sprintf("%d", inst.Port)
			baseline["DB_DATABASE"] = inst.Name
			baseline["DB_USERNAME"] = inst.Username
			baseline["DB_PASSWORD"] = inst.Password
		case "postgresql":
			baseline["DB_CONNECTION"] = "pgsql"
			baseline["DB_HOST"] = inst.Host
			baseline["DB_PORT"] = fmt.Sprintf("%d", inst.Port)
			baseline["DB_DATABASE"] = inst.Name
			baseline["DB_USERNAME"] = inst.Username
			baseline["DB_PASSWORD"] = inst.Password
			baseline["DATABASE_URL"] = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
				inst.Username, inst.Password, inst.Host, inst.Port, inst.Name)
		}
	}
	return baseline
}

// IsBaselineMatchForStore checks if key and value match any project's baseline bounds.
func (s *SecretStoreService) IsBaselineMatchForStore(storeID uint, key, value string) bool {
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("secret_store_id = ?", storeID).Find(&bindings).Error; err != nil {
		return false
	}
	if len(bindings) == 0 {
		globalDefaults := map[string]string{
			"APP_ENV":                  "production",
			"APP_DEBUG":                "false",
			"LOG_CHANNEL":              "stack",
			"LOG_DEPRECATIONS_CHANNEL": "null",
			"LOG_LEVEL":                "debug",
		}
		if val, exists := globalDefaults[key]; exists && val == value {
			return true
		}
		return false
	}
	var projectIDs []uint
	for _, b := range bindings {
		projectIDs = append(projectIDs, b.ProjectID)
	}
	var projects []models.Project
	if err := s.db.Preload("DatabaseInstance").Preload("CustomDomains").Where("id IN (?)", projectIDs).Find(&projects).Error; err != nil {
		return false
	}
	for _, p := range projects {
		baseline := s.GetBaselineEnvMap(&p)
		if val, exists := baseline[key]; exists && val == value {
			return true
		}
	}
	return false
}
