// ===========================================
// Feedback Service
// ===========================================
// Business logic for feedback management
// ===========================================
package services

import (
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/repositories"
)

type FeedbackService struct {
	feedbackRepo repositories.FeedbackRepository
}

func NewFeedbackService(feedbackRepo repositories.FeedbackRepository) *FeedbackService {
	return &FeedbackService{
		feedbackRepo: feedbackRepo,
	}
}

func (s *FeedbackService) SubmitFeedback(userID uint, title, content string, feedbackType models.FeedbackType) (*models.Feedback, error) {
	feedback := &models.Feedback{
		UserID:  userID,
		Title:   title,
		Content: content,
		Type:    feedbackType,
		Status:  models.FeedbackStatusPending,
	}

	if err := s.feedbackRepo.Create(feedback); err != nil {
		return nil, err
	}

	return feedback, nil
}

func (s *FeedbackService) GetAllFeedback(feedbackType string, status string) ([]models.Feedback, error) {
	return s.feedbackRepo.ListAll(feedbackType, status)
}

func (s *FeedbackService) GetUserFeedback(userID uint) ([]models.Feedback, error) {
	return s.feedbackRepo.ListByUserID(userID)
}

func (s *FeedbackService) UpdateStatus(id uint, status models.FeedbackStatus) error {
	return s.feedbackRepo.UpdateStatus(id, status)
}

func (s *FeedbackService) DeleteFeedback(id uint) error {
	return s.feedbackRepo.Delete(id)
}
