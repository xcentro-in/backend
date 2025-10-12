package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	redis "xcentro.in/internal/adapters/cache/redis/client"
)

type AuthService interface {
	// JWT and token management
	CreateAuthJWT(userid string) (string, error)
	ExtractAndValidateClaims(token *jwt.Token) (*jwt.RegisteredClaims, error)
	GetEchoAuthMiddleware() echo.MiddlewareFunc
	CheckIfTokenRevoked(ctx context.Context, userid, token string) (bool, error)
	SetNewToken(ctx context.Context, userid string, token string) error

	// Password utilities (internal use)
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error

	// High-level auth operations
	Signup(ctx context.Context, email, password string) (*SignupResponse, error)
	Login(ctx context.Context, email, password string) (*LoginResponse, error)

	// Utility
	GetRedisClient() *redis.RedisClient
}
