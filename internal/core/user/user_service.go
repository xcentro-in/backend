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

	// KYC workflow methods
	InitiateKYC(ctx context.Context, userID string, claimedRole shared.UserRole, taxID, nationalID string) error
	UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus) error
	GetUserKYCStatus(ctx context.Context, userID string) (*UserResponseDTO, error) // JSON response like business service

	// Verification methods
	VerifyEmail(ctx context.Context, userID, code string) error
	VerifyMobile(ctx context.Context, userID, code string) error
	SendVerificationCode(ctx context.Context, userID string, codeType shared.VerificationType) error

	// Business role management
	ClaimBusinessRole(ctx context.Context, userID, businessID string, role shared.UserRole) error

	// Aadhaar verification methods
	GenerateAadhaarLink(ctx context.Context, userID string) (*AadhaarLinkResponse, error)
	CompleteAadhaarVerification(ctx context.Context, userID string) error
}
