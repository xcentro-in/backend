package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
	redis "xcentro.in/internal/adapters/cache/redis/client"
)

type AuthService interface {
	// JWT and token management
	CreateTokenPair(ctx context.Context, userID, userAgent string) (accessToken, refreshToken string, err error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)
	RevokeRefreshToken(ctx context.Context, userID, refreshToken string) error
	RevokeAllUserTokens(ctx context.Context, userID string) error
	ValidateAccessToken(tokenString string) (*jwt.RegisteredClaims, error)

	// Password utilities (internal use)
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error

	// High-level auth operations
	Signup(ctx context.Context, email, password string) (*SignupResponse, error)
	Login(ctx context.Context, email, password, userAgent string) (*LoginResponse, error)

	// Password reset
	SendPasswordResetEmail(ctx context.Context, email string) error
	ResetPasswordWithToken(ctx context.Context, token string, newPassword string) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error


	// Utility
	GetRedisClient() *redis.RedisClient
}
