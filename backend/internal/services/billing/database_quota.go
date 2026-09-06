package billing

import (
	"errors"
	"fmt"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

type ProjectQuota struct {
	CPULimit    float64
	MemoryLimit string
}

type DatabaseQuota struct {
	ConnectionLimit int
}

func LoadActiveDatabaseQuotaTx(tx *gorm.DB, specID uint) (DatabaseQuota, error) {
	if tx == nil || specID == 0 {
		return DatabaseQuota{}, ErrInvalidInvoiceInput
	}

	var spec models.BillableSpec
	if err := tx.Where("id = ? AND type = ? AND is_active = ?", specID, models.BillableTypeDatabase, true).First(&spec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return DatabaseQuota{}, ErrInvalidInvoiceInput
		}
		return DatabaseQuota{}, fmt.Errorf("load active database billing specification: %w", err)
	}
	if spec.StorageGB <= 0 || spec.ConnectionLimit == nil || *spec.ConnectionLimit <= 0 {
		return DatabaseQuota{}, ErrInvalidInvoiceInput
	}

	return DatabaseQuota{
		ConnectionLimit: *spec.ConnectionLimit,
	}, nil
}

func LoadActiveProjectQuotaTx(tx *gorm.DB, specID uint) (ProjectQuota, error) {
	if tx == nil || specID == 0 {
		return ProjectQuota{}, ErrInvalidInvoiceInput
	}

	var spec models.BillableSpec
	if err := tx.Where("id = ? AND type = ? AND is_active = ?", specID, models.BillableTypeProject, true).First(&spec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProjectQuota{}, ErrInvalidInvoiceInput
		}
		return ProjectQuota{}, fmt.Errorf("load active project billing specification: %w", err)
	}
	if spec.CPUMillicores <= 0 || spec.MemoryMB <= 0 {
		return ProjectQuota{}, ErrInvalidInvoiceInput
	}

	return ProjectQuota{
		CPULimit:    float64(spec.CPUMillicores) / 1000,
		MemoryLimit: fmt.Sprintf("%dm", spec.MemoryMB),
	}, nil
}
