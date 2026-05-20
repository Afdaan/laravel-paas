// ===========================================
// Auth Service
// ===========================================
// Logic for authentication and session management
// ===========================================
package services

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo     repositories.UserRepository
	cfg          *config.Config
	redisService *infrastructure.RedisService
}

func NewAuthService(userRepo repositories.UserRepository, cfg *config.Config, redisService *infrastructure.RedisService) *AuthService {
	return &AuthService{userRepo: userRepo, cfg: cfg, redisService: redisService}
}

// Authenticate checks credentials and returns a user if valid
func (s *AuthService) Authenticate(email, password string) (*models.User, error) {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		return nil, apperr.New(401, "AUTH_FAILED", "Invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, apperr.New(401, "AUTH_FAILED", "Invalid email or password")
	}

	return user, nil
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

// GenerateStreamToken creates a short-lived (60s) ephemeral stream JWT for SSE connections
func (s *AuthService) GenerateStreamToken(user *models.User) (string, error) {
	claims := models.JWTClaims{
		UserID:     user.ID,
		Email:      user.Email,
		Role:       string(user.Role),
		StreamOnly: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * time.Second)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", apperr.New(500, "TOKEN_GEN_FAILED", "Failed to generate ephemeral stream token")
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
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, apperr.NewNotFound("User", id)
	}
	return user, nil
}
