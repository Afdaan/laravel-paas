package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestVerifyRejectsFutureIssuedAt(t *testing.T) {
	cfg := &config.Config{JWTSecret: "abcdefghijklmnopqrstuvwxyz123456", JWTKeyID: "current", JWTIssuer: "runara", JWTAudience: "runara-api", JWTExpiryHours: 24}
	service := &AuthService{cfg: cfg}
	now := time.Now().UTC()
	claims := &models.JWTClaims{TokenUse: models.TokenUseSession, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: cfg.JWTIssuer, Subject: "1", Audience: jwt.ClaimStrings{cfg.JWTAudience}, ID: "future-issued", IssuedAt: jwt.NewNumericDate(now.Add(time.Minute)), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}}
	token, err := service.sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(token, models.TokenUseSession); err == nil {
		t.Fatal("future issued token accepted")
	}
}

func TestCSRFTokenBindsSessionAndExpiresPreviousSecret(t *testing.T) {
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	service := &AuthService{cfg: &config.Config{CSRFSecret: secret}}
	token, err := service.csrfToken("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateCSRFToken(secret, nil, "session-a", token) || ValidateCSRFToken(secret, nil, "session-b", token) {
		t.Fatal("CSRF session binding failed")
	}
	previous := []config.CSRFPreviousSecret{{ID: "old", Secret: secret, NotAfter: time.Now().UTC().Add(-time.Second)}}
	if ValidateCSRFToken("different-secret", previous, "session-a", token) {
		t.Fatal("expired previous secret accepted")
	}
}
