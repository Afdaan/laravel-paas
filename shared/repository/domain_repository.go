package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DomainRepository interface {
	GetByID(ctx context.Context, id uint) (*models.CustomDomain, error)
	GetForUpdate(ctx context.Context, id uint) (*models.CustomDomain, error)
	Save(ctx context.Context, domain *models.CustomDomain) error
	Delete(ctx context.Context, id uint) error
	IncrementSequenceAtomic(ctx context.Context, id uint) (int, error)
	UpdateHealthMetrics(ctx context.Context, id uint, updates map[string]interface{}) error
	ListPendingReconciliation(ctx context.Context, statuses []string, limit, offset int) ([]models.CustomDomain, error)
	WithTx(ctx context.Context, fn func(repo DomainRepository) error) error
	RawDB() *gorm.DB
}

type gormDomainRepository struct {
	db *gorm.DB
}

func NewDomainRepository(db *gorm.DB) DomainRepository {
	return &gormDomainRepository{db: db}
}

func (r *gormDomainRepository) RawDB() *gorm.DB {
	return r.db
}

func (r *gormDomainRepository) GetByID(ctx context.Context, id uint) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := r.db.WithContext(ctx).First(&domain, id).Error; err != nil {
		return nil, err
	}
	return &domain, nil
}

func (r *gormDomainRepository) GetForUpdate(ctx context.Context, id uint) (*models.CustomDomain, error) {
	var domain models.CustomDomain
	if err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&domain, id).Error; err != nil {
		return nil, err
	}
	return &domain, nil
}

func (r *gormDomainRepository) Save(ctx context.Context, domain *models.CustomDomain) error {
	domain.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(domain).Error
}

func (r *gormDomainRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&models.CustomDomain{}, id).Error
}

func (r *gormDomainRepository) IncrementSequenceAtomic(ctx context.Context, id uint) (int, error) {
	var nextSeq int
	err := r.db.WithContext(ctx).Raw(
		"UPDATE custom_domains SET current_sequence = current_sequence + 1 WHERE id = ? RETURNING current_sequence",
		id,
	).Scan(&nextSeq).Error
	if err != nil {
		return 0, fmt.Errorf("failed atomic sequence increment: %w", err)
	}
	return nextSeq, nil
}

func (r *gormDomainRepository) UpdateHealthMetrics(ctx context.Context, id uint, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&models.CustomDomain{}).Where("id = ?", id).Updates(updates).Error
}

func (r *gormDomainRepository) ListPendingReconciliation(ctx context.Context, statuses []string, limit, offset int) ([]models.CustomDomain, error) {
	var domains []models.CustomDomain
	query := r.db.WithContext(ctx).Where("status IN (?)", statuses)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Find(&domains).Error
	return domains, err
}

func (r *gormDomainRepository) WithTx(ctx context.Context, fn func(repo DomainRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &gormDomainRepository{db: tx}
		return fn(txRepo)
	})
}
