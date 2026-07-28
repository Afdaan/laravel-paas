package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"golang.org/x/crypto/bcrypt"
)

const csrfPurpose = "browser-session"

type IssuedSession struct {
	Token     string
	CSRFToken string
	Claims    *models.JWTClaims
}

type AuthService struct {
	userRepo     repositories.UserRepository
	cfg          *config.Config
	redisService *infrastructure.RedisService
}

func NewAuthService(userRepo repositories.UserRepository, cfg *config.Config, redisService *infrastructure.RedisService) *AuthService {
	return &AuthService{userRepo: userRepo, cfg: cfg, redisService: redisService}
}

func (s *AuthService) Authenticate(email, password string) (*models.User, error) {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return nil, apperr.New(401, "AUTH_FAILED", "Invalid email or password")
	}
	return user, nil
}

func (s *AuthService) IssueSession(user *models.User, impersonatorID uint) (*IssuedSession, error) {
	claims := s.newClaims(user.ID, models.TokenUseSession, s.cfg.JWTAudience, s.cfg.JWTExpiryHours, impersonatorID)
	token, err := s.sign(claims)
	if err != nil {
		return nil, err
	}
	csrfToken, err := s.csrfToken(claims.ID)
	if err != nil {
		return nil, apperr.New(500, "CSRF_GEN_FAILED", "Failed to generate request integrity token")
	}
	return &IssuedSession{Token: token, CSRFToken: csrfToken, Claims: claims}, nil
}

func (s *AuthService) GenerateStreamToken(user *models.User) (string, error) {
	return s.sign(s.newClaims(user.ID, models.TokenUseStream, s.cfg.JWTAudience+"-stream", 0, 0))
}

func (s *AuthService) newClaims(userID uint, tokenUse models.TokenUse, audience string, expiryHours int, impersonatorID uint) *models.JWTClaims {
	now := time.Now().UTC()
	ttl := 60 * time.Second
	if tokenUse == models.TokenUseSession {
		ttl = time.Duration(expiryHours) * time.Hour
	}
	return &models.JWTClaims{
		TokenUse:       tokenUse,
		ImpersonatorID: impersonatorID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.JWTIssuer,
			Subject:   strconv.FormatUint(uint64(userID), 10),
			Audience:  jwt.ClaimStrings{audience},
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
}

func (s *AuthService) sign(claims *models.JWTClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = s.cfg.JWTKeyID
	value, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", apperr.New(500, "TOKEN_GEN_FAILED", "Failed to generate security token")
	}
	return value, nil
}

func (s *AuthService) Verify(tokenString string, expectedUse models.TokenUse) (*models.JWTClaims, error) {
	claims := &models.JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, s.keyForToken, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(s.cfg.JWTIssuer), jwt.WithAudience(s.audienceForUse(expectedUse)), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil || !token.Valid || claims.TokenUse != expectedUse || !s.validClaimTimes(claims, expectedUse) {
		return nil, apperr.New(401, "TOKEN_INVALID", "Invalid or expired session")
	}
	return claims, nil
}

func (s *AuthService) keyForToken(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok || !config.ValidJWTKeyID(kid) {
		return nil, fmt.Errorf("unknown signing key")
	}
	if kid == s.cfg.JWTKeyID {
		return []byte(s.cfg.JWTSecret), nil
	}
	for _, previous := range s.cfg.JWTPreviousKeys {
		if previous.ID == kid && time.Now().UTC().Before(previous.NotAfter) {
			return []byte(previous.Secret), nil
		}
	}
	return nil, fmt.Errorf("unknown signing key")
}

func (s *AuthService) validClaimTimes(claims *models.JWTClaims, expectedUse models.TokenUse) bool {
	if claims.Subject == "" || claims.ID == "" || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil {
		return false
	}
	if _, err := strconv.ParseUint(claims.Subject, 10, 64); err != nil {
		return false
	}
	if claims.NotBefore.After(claims.IssuedAt.Time) || claims.IssuedAt.After(claims.ExpiresAt.Time) {
		return false
	}
	maxLifetime := time.Duration(s.cfg.JWTExpiryHours)*time.Hour + 30*time.Second
	if expectedUse == models.TokenUseStream {
		maxLifetime = 90 * time.Second
	}
	return claims.ExpiresAt.Sub(claims.IssuedAt.Time) <= maxLifetime
}

func (s *AuthService) audienceForUse(tokenUse models.TokenUse) string {
	if tokenUse == models.TokenUseStream {
		return s.cfg.JWTAudience + "-stream"
	}
	return s.cfg.JWTAudience
}

func (s *AuthService) Revoke(claims *models.JWTClaims) error {
	if claims == nil || claims.ID == "" || claims.ExpiresAt == nil {
		return apperr.New(401, "TOKEN_INVALID", "Invalid or expired session")
	}
	return s.redisService.RevokeJTI(claims.ID, time.Until(claims.ExpiresAt.Time))
}

func (s *AuthService) IsRevoked(claims *models.JWTClaims) (bool, error) {
	return s.redisService.IsJTIRevoked(claims.ID)
}

func (s *AuthService) csrfToken(jti string) (string, error) {
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(jti + "." + csrfPurpose + "." + base64.RawURLEncoding.EncodeToString(nonce[:])))
	return payload + "." + csrfSignature(s.cfg.CSRFSecret, payload), nil
}

func csrfSignature(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func ValidateCSRFToken(secret string, previousSecrets []config.CSRFPreviousSecret, jti, value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !strings.HasPrefix(string(payload), jti+"."+csrfPurpose+".") {
		return false
	}
	if hmac.Equal([]byte(csrfSignature(secret, parts[0])), []byte(parts[1])) {
		return true
	}
	for _, previous := range previousSecrets {
		if time.Now().UTC().Before(previous.NotAfter) && hmac.Equal([]byte(csrfSignature(previous.Secret, parts[0])), []byte(parts[1])) {
			return true
		}
	}
	return false
}

func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	user, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, apperr.NewNotFound("User", id)
	}
	return user, nil
}
