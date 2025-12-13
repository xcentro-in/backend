package auth

import (
	"context"

	redis "xcentro.in/internal/adapters/cache/redis/client"
)

type AuthService interface {
	// JWT and token management
	CreateTokenPair(ctx context.Context, userID, userAgent string, deviceType string) (accessToken, refreshToken string, err error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)
	RevokeRefreshToken(ctx context.Context, userID, refreshToken string) error
	RevokeAllUserTokens(ctx context.Context, userID string) error
	ValidateAccessToken(tokenString string) (*CustomClaims, error)
	ValidateAccessTokenWithVersion(ctx context.Context, tokenString string) (*CustomClaims, error)

	// Password utilities (internal use)
	HashPassword(password string) (string, error)
	ComparePassword(hashedPassword, password string) error

	// High-level auth operations
	Signup(ctx context.Context, email, password string) (*SignupResponse, error)
	Login(ctx context.Context, email, password, userAgent, deviceType string) (*LoginResponse, error)

	// Password reset
	SendPasswordResetEmail(ctx context.Context, email string, emailType string) error
	ResetPasswordWithToken(ctx context.Context, userID, token, signature, newPassword string) error
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword, otp string) error

	// Email verification
	SendEmailVerification(ctx context.Context, userID string) error
	VerifyEmailWithToken(ctx context.Context, userID, token, signature string) error
	ResendEmailVerification(ctx context.Context, email string) error

	// Mobile verification
	SendMobileVerificationOTP(ctx context.Context, userID, mobileNumber string) error
	GenerateMobileOTP(ctx context.Context, userID string) error
	SendInviteOTP(ctx context.Context, userID, token, signature string) error
	VerifyEmailWithMobile(ctx context.Context, userID, token, signature, mobileNumber string) error
	VerifyMobileOTP(ctx context.Context, userID, otp string) error
	ResetPasswordWithMobileOTP(ctx context.Context, userID, token, signature, otp, newPassword string) error
	VerifyAndUpdateMobile(ctx context.Context, userID, newMobile, otp string) error

	// Utility
	GetRedisClient() *redis.RedisClient
}
