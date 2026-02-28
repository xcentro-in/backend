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

// UnifiedUserProfile is the user portion of the unified login response
type UnifiedUserProfile struct {
	Id                    string `json:"id"`
	Name                  string `json:"name"`
	Email                 string `json:"email"`
	Mobile                string `json:"mobile"`
	KycStatus             string `json:"kyc_status"`
	HasAddedPaymentMethod bool   `json:"has_added_payment_method"`
	HasAddedVendor        bool   `json:"has_added_vendor"`
	HasCreatedFirstPayment bool  `json:"has_created_first_payment"`
}

// UnifiedBusinessInfo is one entry in the businesses list of the unified login response
type UnifiedBusinessInfo struct {
	Bid       string `json:"bid"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	KybStatus string `json:"kyb_status"`
	KycNeeded bool   `json:"kyc_needed"`
}

// UnifiedLoginResponse replaces LoginResponse for the /auth/login endpoint.
// Contains complete frontend context: user profile, all businesses, and active business ID.
// AccessToken and RefreshToken are still set as cookies (not in response body).
type UnifiedLoginResponse struct {
	AccessToken  string                 `json:"accessToken"`  // kept for backward compat — also set as cookie
	RefreshToken string                 `json:"refreshToken"` // kept for backward compat — also set as cookie
	User         *UnifiedUserProfile    `json:"user"`
	Businesses   []*UnifiedBusinessInfo `json:"businesses"`
	ActiveBid    string                 `json:"active_bid"`
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
	Bid           string `json:"bid"`        // active business ID; empty string if user has no business
	Role          string `json:"role"`       // user's role in active business; empty string if none
	KycStatus     string `json:"kyc_status"` // user's KYC status at token issuance time
	KycNeeded     bool   `json:"kyc_needed"` // whether KYC is still required for the active business relation
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

// SwitchBusinessRequest is the request body for POST /v1/auth/switch-business
type SwitchBusinessRequest struct {
	Bid string `json:"bid" validate:"required,uuid"`
}

// SwitchBusinessResponse is the response body for a successful business switch.
// Contains the switched business context so the frontend can update its state immediately.
type SwitchBusinessResponse struct {
	Bid       string `json:"bid"`
	Role      string `json:"role"`
	KybStatus string `json:"kyb_status"`
	KycNeeded bool   `json:"kyc_needed"`
}

// VerifySessionResponse is the response for GET /v1/auth/verify-session.
// Contains full session context: user KYC status, all businesses, and active business ID.
type VerifySessionResponse struct {
	UID        string                 `json:"uid"`
	KycStatus  string                 `json:"kyc_status"`
	Businesses []*UnifiedBusinessInfo `json:"businesses"`
	ActiveBid  string                 `json:"active_bid"`
}

// type AuthJWTClaim struct {
// 	jwt.RegisteredClaims
// 	Roles []shared.UserRole `json:"rls"`
// 	BIDs  []string          `json:"bids"`
// }
