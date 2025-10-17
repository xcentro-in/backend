package auth

import "time"

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	AuthCode string `query:"code"`
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

// type AuthJWTClaim struct {
// 	jwt.RegisteredClaims
// 	Roles []shared.UserRole `json:"rls"`
// 	BIDs  []string          `json:"bids"`
// }
