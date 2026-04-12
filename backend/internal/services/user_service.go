// ===========================================
// User Service
// ===========================================
// Business logic for user management
// ===========================================
package services

import (
	"log/slog"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/stringutil"
	"github.com/laravel-paas/backend/internal/repositories"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo       repositories.UserRepository
	projectService *ProjectService
}

func NewUserService(userRepo repositories.UserRepository, projectService *ProjectService) *UserService {
	return &UserService{
		userRepo:       userRepo,
		projectService: projectService,
	}
}

func (s *UserService) GetAllUsers(page, limit int, role string, search string) ([]models.User, int64, error) {
	return s.userRepo.List(page, limit, role, search)
}

func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	return s.userRepo.GetByID(id)
}

func (s *UserService) CreateUser(name, email, password string, role models.Role, creatorID *uint) (*models.User, string, error) {
	// Check if email exists
	existing, _ := s.userRepo.GetByEmail(email)
	if existing != nil {
		return nil, "", apperr.New(409, "EMAIL_EXISTS", "A user with this email already exists")
	}

	plainPassword := password
	if plainPassword == "" {
		plainPassword = stringutil.GeneratePassword(12)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", apperr.New(500, "PASSWORD_HASH_FAILED", "Failed to process password securely")
	}

	user := &models.User{
		Name:      name,
		Email:     email,
		Password:  string(hashedPassword),
		Role:      role,
		CreatedBy: creatorID,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, "", err
	}

	return user, plainPassword, nil
}

func (s *UserService) UpdateUser(id uint, name, email, password string) (*models.User, error) {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if name != "" {
		user.Name = name
	}

	if email != "" && email != user.Email {
		existing, _ := s.userRepo.GetByEmail(email)
		if existing != nil && existing.ID != id {
			return nil, apperr.New(409, "EMAIL_EXISTS", "The chosen email is already taken by another user")
		}
		user.Email = email
	}

	if password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, apperr.New(500, "PASSWORD_HASH_FAILED", "Failed to process password securely")
		}
		user.Password = string(hashedPassword)
	}

	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) DeleteUser(id uint) error {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return err
	}

	if user.Role == models.RoleSuperAdmin {
		return apperr.New(403, "DELETE_RESTRICTED", "The system protection prevents deleting the superadmin account")
	}

	// 1. Fetch all projects belonging to this user
	projects, err := s.projectService.ListByUserID(id)
	if err == nil {
		slog.Info("Cascading deletion: Cleaning up user projects", "userId", id, "count", len(projects))
		for i := range projects {
			// Thorough cleanup (Docker, DB, Files, etc.)
			s.projectService.DeleteProject(&projects[i])
		}
	}

	// 2. Finally delete the user account
	return s.userRepo.Delete(id)
}

func (s *UserService) GetStudentCount() (int64, error) {
	return s.userRepo.CountStudents()
}

func (s *UserService) IsInitialized() (bool, error) {
	// We need a way to count admins in repo
	count, err := s.userRepo.CountByRole(models.RoleSuperAdmin)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	count, err = s.userRepo.CountByRole(models.RoleAdmin)
	return count > 0, err
}

func (s *UserService) InitializeSuperAdmin(name, email, password string) (*models.User, error) {
	initialized, err := s.IsInitialized()
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, apperr.New(403, "ALREADY_INITIALIZED", "The system has already been initialized with an administrative user")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Name:     name,
		Email:    email,
		Password: string(hashedPassword),
		Role:     models.RoleSuperAdmin,
	}

	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

