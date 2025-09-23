package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type AuthService interface {
	CreateAuthJWT(userid string) (string, error)
	ExtractAndValidateClaims(token *jwt.Token) (*jwt.RegisteredClaims, error)
	GetEchoAuthMiddleware() echo.MiddlewareFunc
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error
}
