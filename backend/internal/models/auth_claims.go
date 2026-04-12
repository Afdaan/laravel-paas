package models

import "github.com/golang-jwt/jwt/v5"

// JWTClaims defines the JWT payload structure for authentication
type JWTClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}
