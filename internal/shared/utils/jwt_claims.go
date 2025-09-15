package utils

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type UserJWTClaims struct {
	UserId string `json:"userid"`
	jwt.RegisteredClaims
}

func ClaimsWithUserId(userid string) jwt.Claims {
	return &UserJWTClaims{
		UserId: userid,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
}
