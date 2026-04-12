// ===========================================
// Auth Service
// ===========================================
// Logic for authentication and session management
// ===========================================
package services

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct {
	db           *gorm.DB
	cfg          *config.Config
	redisService *RedisService
}

func NewAuthService(db *gorm.DB, cfg *config.Config, redisService *RedisService) *AuthService {
	return &AuthService{db: db, cfg: cfg, redisService: redisService}
}

// Authenticate checks credentials and returns a user if valid
func (s *AuthService) Authenticate(email, password string) (*models.User, error) {
	var user models.User
	if err := s.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, apperr.New(401, "AUTH_FAILED", "Invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, apperr.New(401, "AUTH_FAILED", "Invalid email or password")
	}

	return &user, nil
}

// GenerateToken creates a JWT token for a user
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	claims := models.JWTClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   string(user.Role),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.cfg.JWTExpiryHours) * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", apperr.New(500, "TOKEN_GEN_FAILED", "Failed to generate security token")
	}

	return tokenString, nil
}

// Logout blacklists the token in Redis
func (s *AuthService) Logout(token string, expiry time.Duration) error {
	if s.redisService != nil {
		return s.redisService.AddToBlacklist(token, expiry)
	}
	return nil
}

// GetUserByID fetches a user or returns AppError
func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	if err := s.db.First(&user, id).Error; err != nil {
		return nil, apperr.NewNotFound("User", id)
	}
	return &user, nil
}
