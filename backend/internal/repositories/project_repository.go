// ===========================================
// Project Repository
// ===========================================
// Handles data persistence for Project model
// ===========================================
package repositories

import (
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

type ProjectRepository interface {
	GetByID(id uint) (*models.Project, error)
	GetBySubdomain(subdomain string) (*models.Project, error)
	ListByUserID(userID uint) ([]models.Project, error)
	ListAll() ([]models.Project, error)
	Create(project *models.Project) error
	Update(project *models.Project) error
	UpdateStatus(id uint, status models.ProjectStatus) error
	Delete(id uint) error
	UpdateActivity(id uint) error
}

type projectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) GetByID(id uint) (*models.Project, error) {
	var project models.Project
	if err := r.db.Preload("User").First(&project, id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) GetBySubdomain(subdomain string) (*models.Project, error) {
	var project models.Project
	if err := r.db.Where("subdomain = ?", subdomain).First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) ListByUserID(userID uint) ([]models.Project, error) {
	var projects []models.Project
	if err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *projectRepository) ListAll() ([]models.Project, error) {
	var projects []models.Project
	if err := r.db.Preload("User").Order("created_at DESC").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *projectRepository) Create(project *models.Project) error {
	return r.db.Create(project).Error
}

func (r *projectRepository) Update(project *models.Project) error {
	return r.db.Save(project).Error
}

func (r *projectRepository) UpdateStatus(id uint, status models.ProjectStatus) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("status", status).Error
}

func (r *projectRepository) Delete(id uint) error {
	return r.db.Delete(&models.Project{}, id).Error
}

func (r *projectRepository) UpdateActivity(id uint) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("last_activity", gorm.Expr("NOW()")).Error
}
