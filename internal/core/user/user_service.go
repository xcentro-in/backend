package user

import (
	"context"

	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
)

// UserService defines the interface for user business logic operations
type UserService interface {
	// CreateUser creates a new user with authentication payload
	CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*User, error)

	// GetUserByEmail retrieves a user by email address
	GetUserByEmail(ctx context.Context, email string) (*User, error)

	// GetUserByID retrieves a user by ID
	GetUserByID(ctx context.Context, userID string) (*User, error)

	// UpdateUserProfile updates user profile information
	UpdateUserProfile(ctx context.Context, userID string, mobile, pan string) error

	// UpdateKYCStatus updates user's KYC status and data
	UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus, kycData string) error
}
