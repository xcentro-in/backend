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
	UpdateUserProfile(ctx context.Context, userID string, mobile, taxID string) error

	// UpdateKYCStatus updates user's KYC status
	UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus) error

	// Email verification specific methods
	SendEmailVerification(ctx context.Context, userID string) error
	VerifyEmailWithToken(ctx context.Context, token string) error
	ResendEmailVerification(ctx context.Context, email string) error

	// User onboarding methods
	InitiateOnboarding(ctx context.Context, userID, name, dob, taxID, nationalID string) (*InitiateOnboardingResponseDTO, error)
	CompleteOnboarding(ctx context.Context, userID, businessID string, claimedRole shared.UserRole) error
	GetOnboardingStatus(ctx context.Context, userID string) (*OnboardingStatusDTO, error)

	// Password reset methods
	SendPasswordResetEmail(ctx context.Context, email string) error
	ResetPasswordWithToken(ctx context.Context, token string, newPasswordHash string) error

	// Invited user methods
	CreateInvitedUser(ctx context.Context, email, name string, kycNeeded bool) (*User, error)
}
