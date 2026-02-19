package user

import (
	"time"

	"xcentro.in/internal/core/shared"
)

type UserVerificationCodes struct {
	ID            string                  `json:"id"`
	UserID        string                  `json:"userId"`
	Code          string                  `json:"code"`
	CodeType      shared.VerificationType `json:"codeType"`
	ExpiresAt     time.Time               `json:"expiresAt"`
	CreatedAt     time.Time               `json:"createdAt"`
	Used          bool                    `json:"used"`
	RetryAttempts uint8                   `json:"retryAttempts"`
}

// UserIdentity represents externalized user identity data
type UserIdentity struct {
	TaxID          string `json:"taxId"`          // PAN or equivalent tax identifier
	TaxIDData      string `json:"taxIdData"`      // JSON metadata for tax ID
	NationalID     string `json:"nationalId"`     // Aadhaar or equivalent national ID
	NationalIDData string `json:"nationalIdData"` // JSON metadata for national ID
	NTxnID         string `json:"nTxnId"`         // Transaction ID for Aadhaar verification
}

type User struct {
	Id             string           `json:"id"`
	Email          string           `json:"email"`
	PwdHash        string           `json:"-"`
	Mobile         string           `json:"mobile"`
	Name           string           `json:"name"`
	DOB            string           `json:"dob"`
	TaxID          string           `json:"taxId"` // Reference to person_identity.tax_id
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	KycNeeded      bool             `json:"kycNeeded"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
}

// InitiateOnboardingDTO represents the request payload for initiating user onboarding
// User KYC is independent of business - identity verification only
type InitiateOnboardingDTO struct {
	Name       string `json:"name" validate:"required,person_name,min=2,max=100"`
	DOB        string `json:"dob" validate:"required"` // Format: YYYY-MM-DD
	TaxID      string `json:"taxId" validate:"required,pan"`
	NationalID string `json:"nationalId" validate:"required,aadhaar"`
}

// CompleteOnboardingDTO represents the request payload for completing onboarding
// After identity verification, user claims role in a specific business
// Note: BusinessID is now passed via X-Business-ID header
type CompleteOnboardingDTO struct {
	ClaimedRole shared.UserRole `json:"claimedRole" validate:"required"`
	GSTIN       string          `json:"gstin,omitempty" validate:"omitempty,gstin"`
}

// OnboardingStatusDTO represents the response for onboarding status
type OnboardingStatusDTO struct {
	UserID         string           `json:"userId"`
	Email          string           `json:"email"`
	Name           string           `json:"name"`
	DOB            string           `json:"dob"`
	Mobile         string           `json:"mobile"`
	TaxID          string           `json:"taxId"`
	NationalID     string           `json:"nationalId"`
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
}

// InitiateOnboardingResponseDTO represents the response after initiating onboarding
type InitiateOnboardingResponseDTO struct {
	Status      shared.KYCStatus `json:"status"`
	AadhaarLink string           `json:"aadhaarLink"`
	TxnID       string           `json:"txnId"`
}

// UserTaxIDVerificationData combines PAN verification results for storage in tax_id_data
type UserTaxIDVerificationData struct {
	PANCOMData     map[string]interface{} `json:"pan_com_data"`
	PANAadhaarLink map[string]interface{} `json:"pan_aadhaar_link_data"`
}

// ResendVerificationDTO represents the request payload for resending verification email
type ResendVerificationDTO struct {
	Email string `json:"email" validate:"required,email"`
}

// ForgotPasswordDTO represents the request payload for forgot password
type ForgotPasswordDTO struct {
	Email string `json:"email" validate:"required,email"`
}

// ResetPasswordDTO represents the request payload for reset password
type ResetPasswordDTO struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"newPassword" validate:"required,min=8"`
}

// UserProfileDTO represents the user profile response with masked national ID
type UserProfileDTO struct {
	UserID           string `json:"userId"`
	Email            string `json:"email"`
	Name             string `json:"name"`
	DOB              string `json:"dob"`
	Mobile           string `json:"mobile"`
	TaxID            string `json:"taxId"`
	MaskedNationalID string `json:"maskedNationalId"` // Only last 4 chars visible
}

// UpdateProfileDTO represents the request payload for updating user profile
type UpdateProfileDTO struct {
	Mobile string `json:"mobile" validate:"required,phone_in"`
}

// Ops Dashboard DTOs

// OpsUserSearchResult represents a user search result for ops dashboard
type OpsUserSearchResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	BusinessID string `json:"business_id"`
}

// OpsUserDetails represents detailed user information for ops dashboard
type OpsUserDetails struct {
	// User basic info
	ID             string           `json:"id"`
	Email          string           `json:"email"`
	Mobile         string           `json:"mobile"`
	Name           string           `json:"name"`
	DOB            string           `json:"dob"`
	TaxID          string           `json:"taxId"`
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	KycNeeded      bool             `json:"kycNeeded"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`

	// Identity data (if available)
	TaxIDData      string `json:"taxIdData"`
	NationalID     string `json:"nationalId"`
	NationalIDData string `json:"nationalIdData"`
	NTxnID         string `json:"nTxnId"`

	// Business role data (if bid query param provided)
	Role      *shared.UserRole `json:"role,omitempty"`
	RoleClaim *shared.UserRole `json:"roleClaim,omitempty"`
}
