package auth

import "github.com/golang-jwt/jwt/v5"

type CreateAuthJWT func(token string, secret interface{}) (string, error)

type ExtractAndValidateClaims func(token *jwt.Token) (*AuthJWTClaim, error)
