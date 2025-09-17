package user

import (
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/shared"
)

type UserRepository interface {
	CreateUser(authPayload auth.AuthPayload) (*User, error)
	GetUserByID(userID string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	UpdateUserMobile(userID, newMobile string) error
	UpdateUserPAN(userID, newPAN string) error
	UpdateUserKYCStatus(userID string, newStatus shared.KYCStatus) error
	UpdateUserKYCData(userID, kycData string) error
	// Email and Mobile verification methods
	CreateVerificationCode(code *UserVerificationCodes) error
	VerifyVerificationCode(userID, code string, codeType shared.VerificationType) (bool, error)
	CheckUserEmailVerified(userID string) (bool, error)
	CheckUserMobileVerified(userID string) (bool, error)
	CheckUserEmailMobileVerified(userID string) (bool, error)
}
