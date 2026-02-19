package opsauth

import "time"

// OpsUser represents an ops dashboard user authenticated via OIDC
type OpsUser struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"` // Roles extracted from OIDC access token
	Status    string   `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// OpsSession represents an ops user session with OIDC tokens
type OpsSession struct {
	SessionID           string    `json:"sessionId"`
	OpsUserID           string    `json:"opsUserId"`
	RefreshTokenHash    string    `json:"refreshTokenHash"`     // Hash of our internal refresh token
	OIDCRefreshToken    string    `json:"oidcRefreshToken"`     // Encrypted OIDC refresh token
	ExpiresAt           time.Time `json:"expiresAt"`            // Expiry of our internal refresh token
	UserAgent           string    `json:"userAgent,omitempty"`
	CreatedAt           time.Time `json:"createdAt"`
	LastUsedAt          time.Time `json:"lastUsedAt"`
}

// OAuthCallbackResponse contains tokens and user info from OAuth callback
type OAuthCallbackResponse struct {
	AccessToken  string `json:"accessToken"`  // Our internal access token (JWT)
	RefreshToken string `json:"refreshToken"` // Our internal refresh token
}

// OpsUserProfileResponse represents the ops user profile
type OpsUserProfileResponse struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	Status string   `json:"status"`
}
