package auth

import "github.com/golang-jwt/jwt/v5"

type AuthService struct {
	CreateAuthJWT            func(userid string) (string, error)
	ExtractAndValidateClaims func(token *jwt.Token) (*AuthJWTClaim, error)
}
