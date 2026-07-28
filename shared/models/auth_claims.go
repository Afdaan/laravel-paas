package models

import "github.com/golang-jwt/jwt/v5"

type TokenUse string

const (
	TokenUseSession TokenUse = "session"
	TokenUseStream  TokenUse = "stream"
)

// JWTClaims keeps browser-session identity in standard subject claim only.
type JWTClaims struct {
	TokenUse       TokenUse `json:"token_use"`
	ImpersonatorID uint     `json:"impersonator_id,omitempty"`
	jwt.RegisteredClaims
}
