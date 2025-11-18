package user

import (
	"context"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	WithTx(tx client.Tx) UserRepository
	CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUserMobile(ctx context.Context, userID, newMobile string) error
	UpdateUserTaxID(ctx context.Context, userID, newTaxID string) error
	UpdateUserKYCStatus(ctx context.Context, userID string, newStatus shared.KYCStatus) error

	// KYC Identity management
	UpsertUserIdentity(ctx context.Context, identity *UserIdentity) error
	GetUserIdentity(ctx context.Context, taxID string) (*UserIdentity, error)
	GetUserIdentityByTaxID(ctx context.Context, taxID string) (*UserIdentity, error)
	GetUserIdentityByTxnID(ctx context.Context, txnID string) (*UserIdentity, error)
	GetUserKYCStatus(ctx context.Context, userID string) (*User, *UserIdentity, error)
	GetUserByTaxID(ctx context.Context, taxID string) (*User, error)

	// Email and Mobile verification methods
	CreateVerificationCode(ctx context.Context, code *UserVerificationCodes) error
	VerifyVerificationCode(ctx context.Context, userID, code string, codeType shared.VerificationType) (bool, error)
	GetVerificationCodeByCode(ctx context.Context, code string, codeType shared.VerificationType) (*UserVerificationCodes, error)
	GetVerificationCodeByUserIDAndCode(ctx context.Context, userID, code string, codeType shared.VerificationType) (*UserVerificationCodes, error)
	MarkVerificationCodeAsUsed(ctx context.Context, codeID string) error
	UpdateUserEmailVerification(ctx context.Context, userID string, verified bool) error
	UpdateUserMobileVerification(ctx context.Context, userID string, verified bool) error
	CheckUserEmailVerified(ctx context.Context, userID string) (bool, error)
	CheckUserMobileVerified(ctx context.Context, userID string) (bool, error)
	CheckUserEmailMobileVerified(ctx context.Context, userID string) (bool, error)

	// Aadhaar verification methods
	UpdateAadhaarTransactionID(ctx context.Context, taxID, txnID string) error
	GetAadhaarTransactionID(ctx context.Context, userID string) (string, error)
	UpdateNationalIDData(ctx context.Context, taxID, nationalIDData string) error

	// User onboarding methods
	UpdateUserNameAndDOB(ctx context.Context, userID, name, dob string) error
	UpdateTaxIDData(ctx context.Context, taxID, taxIDData string) error

	// Password reset methods
	UpdateUserPasswordHash(ctx context.Context, userID, newPasswordHash string) error

	// Invited user methods
	CreateInvitedUser(ctx context.Context, email, passwordHash, name, mobile string, kycNeeded bool) (*User, error)
	CountBusinessRelations(ctx context.Context, userID string) (int, error)

	// Optimized queries for specific use cases
	GetUserBasicInfo(ctx context.Context, userID string) (id, email, name string, err error)
	GetUserMobileByID(ctx context.Context, userID string) (string, error) // Get user's mobile for JioPay tokenization

	// Ops dashboard queries
	SearchUsersByBusinesses(ctx context.Context, businessIDs []string, searchTerm string) ([]*OpsUserSearchResult, error)
	GetUserDetailsForOps(ctx context.Context, userID string, businessID string) (*OpsUserDetails, error)
	UpdateTaxIDDataPartial(ctx context.Context, taxID string, field string, value string) error
	SetKYCStatusRejected(ctx context.Context, userID string) error
	ClearUserIdentityData(ctx context.Context, taxID string) error
	SetKYCStatusVerified(ctx context.Context, userID string) error
}
