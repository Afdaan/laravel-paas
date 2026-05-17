// ===========================================
// Feedback Repository
// ===========================================
// Handles data persistence for Feedback model
// ===========================================
package repositories

import (
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

type FeedbackRepository interface {
	Create(feedback *models.Feedback) error
	ListAll(feedbackType string, status string) ([]models.Feedback, error)
	ListByUserID(userID uint) ([]models.Feedback, error)
	UpdateStatus(id uint, status models.FeedbackStatus) error
	Delete(id uint) error
}

type feedbackRepository struct {
	db *gorm.DB
}

func NewFeedbackRepository(db *gorm.DB) FeedbackRepository {
	return &feedbackRepository{db: db}
}

func (r *feedbackRepository) Create(feedback *models.Feedback) error {
	return r.db.Create(feedback).Error
}

func (r *feedbackRepository) ListAll(feedbackType string, status string) ([]models.Feedback, error) {
	var feedback []models.Feedback
	query := r.db.Preload("User").Order("created_at DESC")

	if feedbackType != "" {
		query = query.Where("type = ?", feedbackType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Find(&feedback).Error
	return feedback, err
}

func (r *feedbackRepository) ListByUserID(userID uint) ([]models.Feedback, error) {
	var feedback []models.Feedback
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&feedback).Error
	return feedback, err
}

func (r *feedbackRepository) UpdateStatus(id uint, status models.FeedbackStatus) error {
	return r.db.Model(&models.Feedback{}).Where("id = ?", id).Update("status", status).Error
}

func (r *feedbackRepository) Delete(id uint) error {
	return r.db.Delete(&models.Feedback{}, id).Error
}
