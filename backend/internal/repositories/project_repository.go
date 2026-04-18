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
	List(page, limit int, userID uint, status string, search string) ([]models.Project, int64, error)
	ListByUserID(userID uint) ([]models.Project, error)
	ListAll() ([]models.Project, error)
	ListExpired() ([]models.Project, error)
	ListByStatus(status models.ProjectStatus) ([]models.Project, error)
	ListByStatuses(statuses []models.ProjectStatus) ([]models.Project, error)
	Create(project *models.Project) error
	Update(project *models.Project) error
	UpdateStatus(id uint, status models.ProjectStatus) error
	Delete(id uint) error
	UpdateActivity(id uint) error
	CountTotal() (int64, error)
	CountByUserID(userID uint) (int64, error)
	CountRunning() (int64, error)
	GetRunningWithContainers() ([]models.Project, error)
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

func (r *projectRepository) List(page, limit int, userID uint, status string, search string) ([]models.Project, int64, error) {
	var projects []models.Project
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&models.Project{}).Preload("User")

	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		query = query.Where("name LIKE ? OR subdomain LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&projects).Error
	return projects, total, err
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

// ListExpired returns projects whose expiry date has passed
func (r *projectRepository) ListExpired() ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Where("expires_at IS NOT NULL AND expires_at < NOW()").Find(&projects).Error
	return projects, err
}

// ListByStatus returns projects matching a specific status
func (r *projectRepository) ListByStatus(status models.ProjectStatus) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("User").Where("status = ?", status).Find(&projects).Error
	return projects, err
}

// ListByStatuses returns projects matching any of the given statuses
func (r *projectRepository) ListByStatuses(statuses []models.ProjectStatus) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("User").Where("status IN ?", statuses).Find(&projects).Error
	return projects, err
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
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("last_accessed_at", gorm.Expr("NOW()")).Error
}

func (r *projectRepository) CountTotal() (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Count(&count).Error
	return count, err
}

func (r *projectRepository) CountByUserID(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *projectRepository) CountRunning() (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Where("status = ?", models.StatusRunning).Count(&count).Error
	return count, err
}

func (r *projectRepository) GetRunningWithContainers() ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Where("status = ? AND container_id IS NOT NULL", models.StatusRunning).Find(&projects).Error
	return projects, err
}
