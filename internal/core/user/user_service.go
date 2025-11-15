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

	// GetUserProfile retrieves user profile with masked national ID
	GetUserProfile(ctx context.Context, userID string) (*UserProfileDTO, error)

	// UpdateUserProfile updates user profile information (mobile only)
	UpdateUserProfile(ctx context.Context, userID string, mobile string) error

	// UpdateKYCStatus updates user's KYC status
	UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus) error

	// User onboarding methods
	InitiateOnboarding(ctx context.Context, userID, name, dob, taxID, nationalID string) (*InitiateOnboardingResponseDTO, error)
	CompleteOnboarding(ctx context.Context, userID, businessID, gstin string, claimedRole shared.UserRole) error
	GetOnboardingStatus(ctx context.Context, userID string) (*OnboardingStatusDTO, error)

	// Aadhaar verification callback methods
	ProcessAadhaarRedirectCallback(ctx context.Context, userID, encryptedResponse string) error
	ProcessAadhaarWebhook(ctx context.Context, encryptedResponse string) error

	// Invited user methods
	CreateInvitedUser(ctx context.Context, email, name string, kycNeeded bool) (*User, error)

	// Ops dashboard methods
	SearchUsersByBusinesses(ctx context.Context, businessIDs []string, searchTerm string) ([]*OpsUserSearchResult, error)
	GetUserDetailsForOps(ctx context.Context, userID string, businessID string) (*OpsUserDetails, error)
	ReloadUserPANData(ctx context.Context, userID string, dataType string) error
	RejectKYC(ctx context.Context, userID string, notes string) error
}
