package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"xcentro.in/internal/core/shared"
)

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AuthCode string `query:"code"`
}

type AuthJWTClaim struct {
	jwt.RegisteredClaims
	Role      []shared.UserRole `json:"rl"`
	BIDs      []string          `json:"bids"`
	DeviceOS  string            `json:"do"`
	DeviceApp string            `json:"da"`
}
