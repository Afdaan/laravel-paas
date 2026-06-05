package services

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
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

func (s *SecretStoreService) CreateSecretStore(userID uint, name, description string) (*models.SecretStore, error) {
	store := &models.SecretStore{
		UserID:      userID,
		Name:        name,
		Description: description,
	}
	if err := s.db.Create(store).Error; err != nil {
		return nil, err
	}
	s.LogActivity(userID, &store.ID, nil, nil, "create_secretstore", "Created secret store container")
	return store, nil
}

func (s *SecretStoreService) GetSecretStore(userID uint, storeID uint) (*models.SecretStore, error) {
	var store models.SecretStore
	if err := s.db.Preload("Items.Values").Where("id = ? AND user_id = ?", storeID, userID).First(&store).Error; err != nil {
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

func (s *SecretStoreService) UpdateSecretStore(userID uint, storeID uint, name, description string) (*models.SecretStore, error) {
	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return nil, err
	}
	store.Name = name
	store.Description = description
	if err := s.db.Save(store).Error; err != nil {
		return nil, err
	}
	s.LogActivity(userID, &storeID, nil, nil, "update_secretstore", "Updated secret store container metadata")
	return store, nil
}

func (s *SecretStoreService) DeleteSecretStore(userID uint, storeID uint) error {
	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return err
	}

	// Remove bindings and clean up GORM models inside transaction.
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("secret_store_id = ?", storeID).Delete(&models.SecretStoreBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(store).Error; err != nil {
			return err
		}
		s.LogActivity(userID, &storeID, nil, nil, "delete_secretstore", "Soft-deleted secret store container")
		return nil
	})
}

func (s *SecretStoreService) SetSecretValue(userID uint, storeID uint, key, value string) (*models.SecretStoreItem, error) {
	store, err := s.GetSecretStore(userID, storeID)
	if err != nil {
		return nil, err
	}

	var item models.SecretStoreItem
	errItem := s.db.Where("secret_store_id = ? AND key = ?", store.ID, key).First(&item).Error

	stretchedKey := utils.DeriveKey(s.cfg.CredentialEncryptionKey)
	encryptedVal, errEnc := utils.Encrypt(value, stretchedKey)
	if errEnc != nil {
		return nil, fmt.Errorf("failed to encrypt value: %w", errEnc)
	}

	errTx := s.db.Transaction(func(tx *gorm.DB) error {
		var version int = 1
		if errors.Is(errItem, gorm.ErrRecordNotFound) {
			item = models.SecretStoreItem{
				SecretStoreID:         store.ID,
				Key:                   key,
				LatestSnapshotVersion: 1,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
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

		s.LogActivity(userID, &storeID, &item.ID, nil, "set_secret_value", fmt.Sprintf("Set value for key %s (version %d)", key, version))
		return nil
	})

	if errTx != nil {
		return nil, errTx
	}

	// Propagate updates asynchronously to any active projects.
	go s.PropagateSecretStoreUpdates(storeID)

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

	s.LogActivity(userID, &storeID, &item.ID, nil, "set_secret_value", fmt.Sprintf("Set database credentials key %s (version %d)", key, version))
	return &item, nil
}

func (s *SecretStoreService) BindSecretStore(userID uint, storeID, projectID uint, environment string) (*models.SecretStoreBinding, error) {
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

	s.LogActivity(userID, &storeID, nil, &projectID, "bind_secretstore", fmt.Sprintf("Bound secret store container to project environment %s", environment))

	// Trigger env propagation for the newly bound project.
	go s.PropagateSecretStoreUpdates(storeID)

	return binding, nil
}

func (s *SecretStoreService) UnbindSecretStore(userID uint, storeID, projectID uint) error {
	var project models.Project
	if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
		return errors.New("project not found or unauthorized")
	}

	var binding models.SecretStoreBinding
	if err := s.db.Where("secret_store_id = ? AND project_id = ?", storeID, projectID).First(&binding).Error; err != nil {
		return err
	}

	if err := s.db.Delete(&binding).Error; err != nil {
		return err
	}

	s.LogActivity(userID, &storeID, nil, &projectID, "unbind_secretstore", "Unbound secret store container from project")

	// Trigger env propagation to clear unbound keys.
	go s.PropagateSecretStoreUpdates(storeID)

	return nil
}

func (s *SecretStoreService) UpdateDatabaseSecrets(project *models.Project, username, password string) error {
	var binding models.SecretStoreBinding
	err := s.db.Where("project_id = ?", project.ID).First(&binding).Error
	var storeID uint
	if err != nil {
		store := models.SecretStore{
			UserID:      project.UserID,
			Name:        fmt.Sprintf("Database Credentials (%s)", project.Name),
			Description: "System-generated credentials for database instance",
		}
		if err := s.db.Create(&store).Error; err != nil {
			return err
		}
		storeID = store.ID

		newBinding := models.SecretStoreBinding{
			ProjectID:     project.ID,
			SecretStoreID: storeID,
			Environment:   "all",
		}
		if err := s.db.Create(&newBinding).Error; err != nil {
			return err
		}
	} else {
		storeID = binding.SecretStoreID
	}

	if _, err := s.SetSecretValueInternal(project.UserID, storeID, "DB_PASSWORD", password); err != nil {
		return err
	}

	if project.DatabaseInstance != nil {
		inst := project.DatabaseInstance
		var dbURL string
		if inst.Engine == "mysql" {
			dbURL = fmt.Sprintf("mysql://%s:%s@%s:%d/%s", username, password, inst.Host, inst.Port, inst.Name)
		} else {
			dbURL = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", username, password, inst.Host, inst.Port, inst.Name)
		}
		if _, err := s.SetSecretValueInternal(project.UserID, storeID, "DATABASE_URL", dbURL); err != nil {
			return err
		}
	}

	return nil
}

func (s *SecretStoreService) CompileEnvForProject(projectID uint, environment string) (map[string]string, error) {
	var project models.Project
	if err := s.db.Preload("DatabaseInstance").First(&project, projectID).Error; err != nil {
		return nil, err
	}

	envMap := make(map[string]string)

	// Layer 1: PaaS defaults.
	envMap["APP_NAME"] = project.Name
	envMap["APP_ENV"] = environment
	envMap["APP_DEBUG"] = "false"
	envMap["APP_URL"] = fmt.Sprintf("http://%s", project.Subdomain)
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
				decrypted, err := utils.Decrypt(latestVal.EncryptedValue, stretchedKey)
				if err != nil {
					slog.Error("Failed to decrypt secret value during env compilation", "item_id", item.ID, "error", err)
					continue
				}
				envMap[item.Key] = decrypted
			}
		}
	}

	return envMap, nil
}

func (s *SecretStoreService) PropagateSecretStoreUpdates(storeID uint) {
	var bindings []models.SecretStoreBinding
	if err := s.db.Where("secret_store_id = ?", storeID).Find(&bindings).Error; err == nil {
		for _, b := range bindings {
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

func (s *SecretStoreService) LogActivity(userID uint, storeID, itemID, projectID *uint, action, details string) {
	log := models.SecretStoreActivityLog{
		UserID:            userID,
		SecretStoreID:     storeID,
		SecretStoreItemID: itemID,
		ProjectID:         projectID,
		Action:            action,
		Details:           details,
	}
	if err := s.db.Create(&log).Error; err != nil {
		slog.Error("Failed to write secret store activity log", "error", err)
	}
}
