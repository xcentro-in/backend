package auth

import (
	"context"
	"time"
)

// SessionRepository defines database operations for refresh token sessions
type SessionRepository interface {
	// CreateSession creates a new refresh token session with device information
	CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time, userAgent string, deviceType string, deviceVersion int) (string, error)

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

	// CountSessionsByDevice counts active sessions for a user by device type
	CountSessionsByDevice(ctx context.Context, userID string, deviceType string) (int, error)

	// DeleteOldestSessionByDevice deletes the oldest session for a user by device type
	DeleteOldestSessionByDevice(ctx context.Context, userID string, deviceType string) error

	// WithTx returns a new repository instance that uses the given transaction
	WithTx(tx interface{}) SessionRepository
}

// DeviceVersionRepository defines database operations for user device versions
type DeviceVersionRepository interface {
	// GetDeviceVersion retrieves the current version for a user's device type
	GetDeviceVersion(ctx context.Context, userID string, deviceType string) (int, error)

	// GetDeviceVersionWithRoles retrieves device version and user roles in a single query
	GetDeviceVersionWithRoles(ctx context.Context, userID string, deviceType string) (int, interface{}, error)

	// IncrementDeviceVersion increments the version for a user's device type and returns new version
	IncrementDeviceVersion(ctx context.Context, userID string, deviceType string) (int, error)

	// UpsertDeviceVersion creates or updates device version
	UpsertDeviceVersion(ctx context.Context, userID string, deviceType string, version int) error

	// WithTx returns a new repository instance that uses the given transaction
	WithTx(tx interface{}) DeviceVersionRepository
}
