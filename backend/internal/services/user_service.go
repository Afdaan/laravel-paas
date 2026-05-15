// ===========================================
// User Service
// ===========================================
// Business logic for user management
// ===========================================
package services

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/pkg/utils"
	"github.com/laravel-paas/backend/internal/repositories"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	userRepo       repositories.UserRepository
	projectService *projectServicePkg.ProjectService
	initMu         sync.Mutex
}

func NewUserService(userRepo repositories.UserRepository, projectService *projectServicePkg.ProjectService) *UserService {
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
	// Check if email exists (Unscoped checked in Repo)
	existing, _ := s.userRepo.GetByEmail(email)

	plainPassword := password
	if plainPassword == "" {
		plainPassword = utils.GeneratePassword(12)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", apperr.New(500, "PASSWORD_HASH_FAILED", "Failed to process password securely")
	}

	if existing != nil {
		// If user was soft-deleted, we automatically restore them
		if existing.DeletedAt.Valid {
			existing.DeletedAt.Valid = false
			existing.Name = name
			existing.Password = string(hashedPassword)
			existing.Role = role
			existing.CreatedBy = creatorID

			if err := s.userRepo.Update(existing); err != nil {
				return nil, "", err
			}
			return existing, plainPassword, nil
		}
		return nil, "", apperr.New(409, "EMAIL_EXISTS", "A user with this email already exists")
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
			if err := s.projectService.DeleteProject(&projects[i]); err != nil {
				slog.Error("Failed to delete user project during cascade", "userId", id, "projectId", projects[i].ID, "error", err)
			}
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
	s.initMu.Lock()
	defer s.initMu.Unlock()

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

	slog.Info("System initialized with superadmin", "email", email, "user_id", user.ID)
	return user, nil
}

// UpdateActivity updates LastActivity and optionally LastIP/LastLocation
func (s *UserService) UpdateActivity(userID uint, ip string, forceLoginUpdate bool) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return
	}

	now := time.Now()

	// Throttling: only update activity if at least 5 minutes have passed
	// This reduces unnecessary database writes on every API call
	shouldUpdateActivity := forceLoginUpdate
	if user.LastActivity == nil || now.Sub(*user.LastActivity) > 5*time.Minute {
		shouldUpdateActivity = true
	}

	if shouldUpdateActivity {
		user.LastActivity = &now
		if forceLoginUpdate {
			user.LastLogin = &now
			user.LastIP = ip

			// Attempt to detect location asynchronously
			go func(uID uint, userIP string) {
				location := s.detectLocation(userIP)
				if location != "" {
					u, err := s.userRepo.GetByID(uID)
					if err == nil {
						u.LastLocation = location
						if err := s.userRepo.Update(u); err != nil {
							slog.Warn("Failed to update user location", "userId", uID, "error", err)
						}
					}
				}
			}(user.ID, ip)
		}
		if err := s.userRepo.Update(user); err != nil {
			slog.Error("Failed to update user activity", "userId", user.ID, "error", err)
		}
	}
}

// detectLocation fetches location data from a public IP API
func (s *UserService) detectLocation(ip string) string {
	if ip == "127.0.0.1" || ip == "::1" || ip == "" {
		return "Localhost"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	// ip-api.com returns a simple JSON for location lookup
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=status,message,country,city")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Country string `json:"country"`
		City    string `json:"city"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if result.Status == "success" {
		return result.City + ", " + result.Country
	}

	return ""
}
