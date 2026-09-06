package models

import "github.com/golang-jwt/jwt/v5"

type TokenUse string

const (
	TokenUseSession TokenUse = "session"
	TokenUseStream  TokenUse = "stream"
)

// JWTClaims keeps browser-session identity and authentication freshness.
type JWTClaims struct {
	TokenUse       TokenUse         `json:"token_use"`
	ImpersonatorID uint             `json:"impersonator_id,omitempty"`
	AuthTime       *jwt.NumericDate `json:"auth_time,omitempty"`
	jwt.RegisteredClaims
}
