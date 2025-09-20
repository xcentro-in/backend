package user

import (
	"context"

	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUserMobile(ctx context.Context, userID, newMobile string) error
	UpdateUserPAN(ctx context.Context, userID, newPAN string) error
	UpdateUserKYCStatus(ctx context.Context, userID string, newStatus shared.KYCStatus) error
	UpdateUserKYCData(ctx context.Context, userID, kycData string) error
	// Email and Mobile verification methods
	CreateVerificationCode(ctx context.Context, code *UserVerificationCodes) error
	VerifyVerificationCode(ctx context.Context, userID, code string, codeType shared.VerificationType) (bool, error)
	CheckUserEmailVerified(ctx context.Context, userID string) (bool, error)
	CheckUserMobileVerified(ctx context.Context, userID string) (bool, error)
	CheckUserEmailMobileVerified(ctx context.Context, userID string) (bool, error)
}
