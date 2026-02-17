package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthPayload struct {
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,strong_password"`
	AuthCode   string `query:"code"`
	DeviceType string `json:"deviceType" validate:"omitempty,oneof=web mobile"`
}

// SignupResponse represents the response after successful user registration
type SignupResponse struct {
	UserID  string `json:"userId"`
	Message string `json:"message"`
}

// LoginResponse represents the response after successful login
// Contains access token and refresh token
type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// Session represents a user refresh token session
type Session struct {
	SessionID        string    `json:"sessionId"`
	UserID           string    `json:"userId"`
	RefreshTokenHash string    `json:"refreshTokenHash"`
	ExpiresAt        time.Time `json:"expiresAt"`
	CreatedAt        time.Time `json:"createdAt"`
	LastUsedAt       time.Time `json:"lastUsedAt"`
	UserAgent        string    `json:"userAgent,omitempty"`
	DeviceType       string    `json:"deviceType"`
	DeviceVersion    int       `json:"deviceVersion"`
}

// CustomClaims extends JWT registered claims with device information and session ID
type CustomClaims struct {
	jwt.RegisteredClaims
	SessionID     string `json:"session_id"`
	DeviceType    string `json:"device_type"`
	DeviceVersion int    `json:"device_version"`
}

// UserDeviceVersion tracks JWT version per user per device type
type UserDeviceVersion struct {
	UserID         string    `json:"userId"`
	DeviceType     string    `json:"deviceType"`
	CurrentVersion int       `json:"currentVersion"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// RefreshTokenRequest represents the request to refresh an access token
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

// RefreshTokenResponse represents the response after refreshing access token
type RefreshTokenResponse struct {
	AccessToken string `json:"accessToken"`
}

// RevokeTokenRequest represents the request to revoke a refresh token
type RevokeTokenRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

// ResetPasswordRequest represents the request to reset password with token
type ResetPasswordRequest struct {
	UserID      string `json:"uid" validate:"required"`
	Token       string `json:"token" validate:"required"`
	Signature   string `json:"sig" validate:"required"`
	NewPassword string `json:"newPassword" validate:"required,strong_password"`
}

// ChangePasswordRequest represents the request to change password
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required"`
	NewPassword string `json:"newPassword" validate:"required,strong_password"`
	OTP         string `json:"otp" validate:"required,len=6,numeric"`
}

// ResendVerificationRequest represents the request to resend email verification
type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ResendMobileOTPRequest represents the request to resend mobile OTP during signup
type ResendMobileOTPRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// VerifyEmailWithMobileRequest represents the request to verify email and capture mobile number
type VerifyEmailWithMobileRequest struct {
	UserID      string `json:"uid" validate:"required"`
	Token       string `json:"token" validate:"required"`
	Signature   string `json:"sig" validate:"required"`
	MobileNumber string `json:"mobileNumber" validate:"required,phone_in"`
}

// VerifyMobileOTPRequest represents the request to verify mobile OTP
type VerifyMobileOTPRequest struct {
	UserID string `json:"uid" validate:"required"`
	OTP    string `json:"otp" validate:"required,len=6,numeric"`
}

// ResetPasswordWithMobileOTPRequest represents the request to reset password with mobile OTP
type ResetPasswordWithMobileOTPRequest struct {
	UserID      string `json:"uid" validate:"required"`
	Token       string `json:"token" validate:"required"`
	Signature   string `json:"sig" validate:"required"`
	OTP         string `json:"otp" validate:"required,len=6,numeric"`
	NewPassword string `json:"newPassword" validate:"required,strong_password"`
}

// SendInviteOTPRequest represents the request to send OTP for invited user
type SendInviteOTPRequest struct {
	UserID    string `json:"uid" validate:"required"`
	Token     string `json:"token" validate:"required"`
	Signature string `json:"sig" validate:"required"`
}

// SendMobileUpdateOTPRequest represents the request to send OTP to new mobile number
type SendMobileUpdateOTPRequest struct {
	MobileNumber string `json:"mobileNumber" validate:"required,phone_in"`
}

// VerifyMobileUpdateRequest represents the request to verify OTP and update mobile
type VerifyMobileUpdateRequest struct {
	MobileNumber string `json:"mobileNumber" validate:"required,phone_in"`
	OTP          string `json:"otp" validate:"required,len=6,numeric"`
}

// type AuthJWTClaim struct {
// 	jwt.RegisteredClaims
// 	Roles []shared.UserRole `json:"rls"`
// 	BIDs  []string          `json:"bids"`
// }
