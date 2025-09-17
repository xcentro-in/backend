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

type User struct {
	Id             string           `json:"id"`
	Email          string           `json:"email"`
	Mobile         string           `json:"mobile"`
	PAN            string           `json:"pan"`
	KycStatus      shared.KYCStatus `json:"kycStatus"`
	KycData        string           `json:"kycData"`
	EmailVerified  bool             `json:"emailVerified"`
	MobileVerified bool             `json:"mobileVerified"`
	Role           shared.UserRole  `json:"role"`
}
