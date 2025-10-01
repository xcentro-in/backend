package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	redis "xcentro.in/internal/adapters/cache/redis/client"
)

type AuthService interface {
	CreateAuthJWT(userid string) (string, error)
	ExtractAndValidateClaims(token *jwt.Token) (*jwt.RegisteredClaims, error)
	GetEchoAuthMiddleware() echo.MiddlewareFunc
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error
	GetRedisClient() *redis.RedisClient
	CheckIfTokenRevoked(ctx context.Context, userid, token string) (bool, error)
	SetNewToken(ctx context.Context, userid string, token string) error
}
