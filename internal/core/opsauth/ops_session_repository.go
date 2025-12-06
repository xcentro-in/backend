package opsauth

import (
	"context"
	"time"
)

// OpsSessionRepository defines database operations for ops user sessions
type OpsSessionRepository interface {
	// CreateSession creates a new ops user session
	// oidcRefreshToken should already be encrypted before calling this
	CreateSession(ctx context.Context, opsUserID, refreshTokenHash, oidcRefreshToken string, expiresAt time.Time, userAgent string) (string, error)

	// GetSessionByTokenHash retrieves a session by ops user ID and refresh token hash
	GetSessionByTokenHash(ctx context.Context, opsUserID, refreshTokenHash string) (*OpsSession, error)

	// UpdateLastUsed updates the last_used_at timestamp for a session
	UpdateLastUsed(ctx context.Context, opsUserID, refreshTokenHash string) error

	// UpdateOIDCRefreshToken updates the encrypted OIDC refresh token in session
	// Used after refreshing with OIDC provider to store new refresh token
	UpdateOIDCRefreshToken(ctx context.Context, opsUserID, refreshTokenHash, newEncryptedOIDCRefreshToken string) error

	// DeleteSession deletes a specific session by ops user ID and token hash
	DeleteSession(ctx context.Context, opsUserID, refreshTokenHash string) error

	// DeleteAllUserSessions deletes all sessions for an ops user
	// Returns list of deleted token hashes
	DeleteAllUserSessions(ctx context.Context, opsUserID string) ([]string, error)

	// GetAllUserSessions retrieves all sessions for an ops user
	GetAllUserSessions(ctx context.Context, opsUserID string) ([]*OpsSession, error)

	// WithTx returns a new repository instance that uses the given transaction
	WithTx(tx interface{}) OpsSessionRepository
}
