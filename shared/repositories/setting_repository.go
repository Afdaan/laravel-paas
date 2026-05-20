// ===========================================
// Setting Repository
// ===========================================
// Handles data persistence for Setting model
// ===========================================
package repositories

import (
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

type SettingRepository interface {
	GetByKey(key string) (*models.Setting, error)
	GetValue(key string, defaultValue string) string
	Upsert(key string, value string) error
	ListAll() ([]models.Setting, error)
}

type settingRepository struct {
	db *gorm.DB
}

func NewSettingRepository(db *gorm.DB) SettingRepository {
	return &settingRepository{db: db}
}

func (r *settingRepository) GetByKey(key string) (*models.Setting, error) {
	var setting models.Setting
	if err := r.db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		return nil, err
	}
	return &setting, nil
}

func (r *settingRepository) GetValue(key string, defaultValue string) string {
	var setting models.Setting
	if err := r.db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}

func (r *settingRepository) Upsert(key string, value string) error {
	var setting models.Setting
	if err := r.db.Where("setting_key = ?", key).First(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.db.Create(&models.Setting{Key: key, Value: value}).Error
		}
		return err
	}
	setting.Value = value
	return r.db.Save(&setting).Error
}

func (r *settingRepository) ListAll() ([]models.Setting, error) {
	var settings []models.Setting
	if err := r.db.Find(&settings).Error; err != nil {
		return nil, err
	}
	return settings, nil
}
