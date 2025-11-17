package opsauth

import "context"

// OpsUserRepository defines database operations for ops dashboard users
type OpsUserRepository interface {
	// CreateOrUpdateUser creates a new ops user or updates existing one
	// Returns the user ID and whether it was created (true) or updated (false)
	CreateOrUpdateUser(ctx context.Context, email, name string, roles []string) (string, bool, error)

	// GetUserByID retrieves an ops user by ID
	GetUserByID(ctx context.Context, userID string) (*OpsUser, error)

	// GetUserByEmail retrieves an ops user by email
	GetUserByEmail(ctx context.Context, email string) (*OpsUser, error)

	// UpdateUserRoles updates the roles for an ops user
	UpdateUserRoles(ctx context.Context, userID string, roles []string) error

	// UpdateUserStatus updates the status of an ops user
	UpdateUserStatus(ctx context.Context, userID string, status string) error

	// WithTx returns a new repository instance that uses the given transaction
	WithTx(tx interface{}) OpsUserRepository
}
