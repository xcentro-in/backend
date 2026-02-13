package opsauth

import "context"

// OpsAuthService defines business logic operations for ops dashboard authentication
type OpsAuthService interface {
	// GetLoginURL generates the OAuth2 authorization URL for Office365 login
	GetLoginURL(state string) string

	// HandleCallback processes the OAuth callback, exchanges code for tokens,
	// creates/updates ops user, and returns our internal tokens
	HandleCallback(ctx context.Context, code, state string, userAgent string) (*OAuthCallbackResponse, error)

	// RefreshAccessToken validates internal refresh token and issues new access token
	// Also refreshes OIDC tokens with provider and updates roles
	RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)

	// Logout revokes the internal refresh token session
	Logout(ctx context.Context, opsUserID, refreshToken string) error

	// ValidateAccessToken validates an access token and returns its claims
	ValidateAccessToken(tokenString string) (*OpsJWTClaims, error)

	// GetUserProfile retrieves the ops user profile by ID
	GetUserProfile(ctx context.Context, opsUserID string) (*OpsUserProfileResponse, error)
}

// OpsJWTClaims represents the claims in our internal JWT access token
// For security, only includes: subject (OID), session ID, and roles
// Email/name should be fetched from database if needed
type OpsJWTClaims struct {
	Subject   string   `json:"sub"`        // Ops user ID (OID from Entra)
	SessionID string   `json:"session_id"` // Session ID for CSRF token validation
	Roles     []string `json:"roles"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
}
