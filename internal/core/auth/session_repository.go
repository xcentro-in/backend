package auth

import (
	"context"
	"time"
)

// SessionRepository defines database operations for refresh token sessions
type SessionRepository interface {
	// CreateSession creates a new refresh token session
	CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time, userAgent string) (string, error)

	// GetSessionByTokenHash retrieves a session by user ID and refresh token hash
	GetSessionByTokenHash(ctx context.Context, userID, tokenHash string) (*Session, error)

	// UpdateLastUsed updates the last_used_at timestamp for a session
	UpdateLastUsed(ctx context.Context, userID, tokenHash string) error

	// DeleteSession deletes a specific session by user ID and token hash
	DeleteSession(ctx context.Context, userID, tokenHash string) error

	// DeleteAllUserSessions deletes all sessions for a user
	DeleteAllUserSessions(ctx context.Context, userID string) ([]string, error) // returns list of deleted token hashes

	// GetAllUserSessions retrieves all sessions for a user
	GetAllUserSessions(ctx context.Context, userID string) ([]*Session, error)

	// WithTx returns a new repository instance that uses the given transaction
	WithTx(tx interface{}) SessionRepository
}
