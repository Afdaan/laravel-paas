// ===========================================
// User Repository
// ===========================================
// Handles data persistence for User model
// ===========================================
package repositories

import (
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
)

type UserRepository interface {
	GetByID(id uint) (*models.User, error)
	GetByEmail(email string) (*models.User, error)
	List(page, limit int, role string, search string) ([]models.User, int64, error)
	Create(user *models.User) error
	Update(user *models.User) error
	Delete(id uint) error
	CountByRole(role models.Role) (int64, error)
	CountStudents() (int64, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(id uint) (*models.User, error) {
	var user models.User
	if err := r.db.Preload("Projects").First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	var user models.User
	if err := r.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) List(page, limit int, role string, search string) ([]models.User, int64, error) {
	var users []models.User
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&models.User{})

	if role != "" {
		query = query.Where("role = ?", role)
	}

	if search != "" {
		query = query.Where("name LIKE ? OR email LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error
	return users, total, err
}

func (r *userRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) Delete(id uint) error {
	return r.db.Delete(&models.User{}, id).Error
}

func (r *userRepository) CountByRole(role models.Role) (int64, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

func (r *userRepository) CountStudents() (int64, error) {
	return r.CountByRole(models.RoleStudent)
}
