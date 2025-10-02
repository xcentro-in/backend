package user

import "xcentro.in/internal/core/shared"

type UserVerificationCodes struct {
	ID            string                  `json:"id"`
	UserID        string                  `json:"userId"`
	Code          string                  `json:"code"`
	CodeType      shared.VerificationType `json:"codeType"`
	ExpiresAt     string                  `json:"expiresAt"`
	CreatedAt     string                  `json:"createdAt"`
	Used          bool                    `json:"used"`
	RetryAttempts uint8                   `json:"retryAttempts"`
}

// UserIdentity represents externalized user identity data
type UserIdentity struct {
	TaxID          string `json:"taxId"`          // PAN or equivalent tax identifier
	TaxIDData      string `json:"taxIdData"`      // JSON metadata for tax ID
	NationalID     string `json:"nationalId"`     // Aadhaar or equivalent national ID
	NationalIDData string `json:"nationalIdData"` // JSON metadata for national ID
}

type User struct {
	Id             string           `json:"id"`
	Email          string           `json:"email"`
	PwdHash        string           `json:"-"`
	Mobile         string           `json:"mobile"`
	TaxID          string           `json:"taxId"` // Reference to person_identity.tax_id
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
}

type CreateUserKYCCTO struct {
	ClaimedRole shared.UserRole `json:"claimedRole"`
	TaxID       string          `json:"taxId"`
	NationalID  string          `json:"nationalId"`
}

// VerifyMobileDTO represents the request payload for mobile verification
type VerifyMobileDTO struct {
	Code string `json:"code" validate:"required"`
}

// ClaimRoleDTO represents the request payload for claiming a business role
type ClaimRoleDTO struct {
	BusinessID string          `json:"businessId" validate:"required"`
	Role       shared.UserRole `json:"role" validate:"required"`
}

type UserResponseDTO struct {
	Id             string           `json:"id"`
	Email          string           `json:"email"`
	Mobile         string           `json:"mobile"`
	TaxID          string           `json:"taxId"`
	NationalID     string           `json:"nationalId"`
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
}
