package business

import (
	"xcentro.in/internal/core/shared"
)

// Business represents a business entity.
type Business struct {
	BID              string              `json:"bid"`
	Name             string              `json:"name"`
	GSTIN            string              `json:"gstin"`
	GSTINData        string              `json:"gstinData"`
	ITID             string              `json:"itid"`
	KybStatus        shared.KYBstatus    `json:"kybStatus"`
	BusinessType     shared.BusinessType `json:"businessType"`
	BusinessModel    string              `json:"businessModel"`
	BusinessTurnover string              `json:"businessTurnover"`
	CountryCode      string              `json:"countryCode"`
	CurrencyCode     string              `json:"currencyCode"`
	HasCompletedOnboarding bool 		 `json:"hasCompletedOnboarding"`
	HasAddedPaymentMethod  bool 		 `json:"hasAddedPaymentMethod"`
	HasAddedVendor   	   bool 		 `json:"hasAddedVendor"`
	HasCreatedFirstPayment bool 		 `json:"hasCreatedFirstPayment"`
}

type BusinessKYBData struct {
	ITID     string `json:"itid"`
	ITIDData string `json:"itidData"`
	CID      string `json:"cid"`
	CIDData  string `json:"cidData"`
	DocUrl   string `json:"docUrl"`
}

// BusinessUserRelation represents the many-to-many relationship between users and businesses.
type BusinessUserRelation struct {
	UserID       string          `json:"userid"`
	BID          string          `json:"bid"`
	Role         shared.UserRole `json:"role"`
	RoleClaim    shared.UserRole `json:"roleClaim"`
	AddedAt      string          `json:"addedAt"`
	KybInitiated bool            `json:"kybInitiated"`
	KycNeeded    bool            `json:"kycNeeded"`
	Timestamp    string          `json:"timestamp"`
}

type BusinessSubscription struct {
	BID              string                  `json:"bid"`
	SubscriptionType shared.SubscriptionType `json:"subscriptionType"`
	StartedAt        string                  `json:"startedAt"`
	ValidTill        string                  `json:"validTill"`
	AutoRenew        bool                    `json:"autoRenew"`
	RenewalAmount    float64                 `json:"renewalAmount"`
}

type BusinessKYBDTO struct {
	BID          string              `json:"bid"`
	BusinessType shared.BusinessType `json:"businessType"`
	ITID         string              `json:"itid"`
	CID          string              `json:"cid"`
	DocUrl       string              `json:"docUrl"`
}

type CreateBusinessDTO struct {
	Id           string              `json:"id"`
	Name         string              `json:"name"`
	GSTIN        string              `json:"gstin"`
	BusinessType shared.BusinessType `json:"businessType"`
	ITID         string              `json:"itid"`
	CID          string              `json:"cid"`
	DocUrl       string              `json:"docUrl"`
}

// New onboarding DTOs
type OnboardingInitResponse struct {
	BID       string                 `json:"bid"`
	Name      string                 `json:"name"`
	GSTIN     string                 `json:"gstin"`
	GSTData   map[string]interface{} `json:"gstData,omitempty"`
	KYBStatus shared.KYBstatus       `json:"kybStatus"`
	Message   string                 `json:"message"`
}

type CompleteKYBRequest struct {
	BID          string              `json:"bid" validate:"required"`
	BusinessType shared.BusinessType `json:"businessType" validate:"required"`
	ITID         string              `json:"itid" validate:"required,pan"` // Income Tax ID (PAN)
	CID          string              `json:"cid,omitempty"`                // Company ID (CIN) - required for certain business types
}

type CompleteKYBResponse struct {
	BID                string           `json:"bid"`
	KYBStatus          shared.KYBstatus `json:"kybStatus"`
	RequiresDeedUpload bool             `json:"requiresDeedUpload"`
	UploadURL          string           `json:"uploadUrl,omitempty"`
	Message            string           `json:"message"`
}

type OnboardingStatusResponse struct {
	BID                string              `json:"bid,omitempty"`
	Name               string              `json:"name,omitempty"`
	GSTIN              string              `json:"gstin,omitempty"`
	BusinessType       shared.BusinessType `json:"businessType,omitempty"`
	ITID               string              `json:"itid,omitempty"`
	CID                string              `json:"cid,omitempty"`
	KYBStatus          shared.KYBstatus    `json:"kybStatus"`
	KYBInitiated       bool                `json:"kybInitiated"`
	RequiresDeedUpload bool                `json:"requiresDeedUpload"`
	HasDeedUploaded    bool                `json:"hasDeedUploaded"`
}

// BusinessUserDTO represents a user associated with a business with detailed information
type BusinessUserDTO struct {
	UserID    string          `json:"userId"`
	Email     string          `json:"email"`
	Name      string          `json:"name"`
	Role      shared.UserRole `json:"role"`
	RoleClaim shared.UserRole `json:"roleClaim"`
	KycNeeded bool            `json:"kycNeeded"`
	AddedAt   string          `json:"addedAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// InviteUserResponse represents the response when inviting a user to a business
type InviteUserResponse struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

// BusinessDetailsResponse represents the response for getting business details
type BusinessDetailsResponse struct {
	BID          string              `json:"bid"`
	Name         string              `json:"name"`
	GSTIN        string              `json:"gstin"`
	PAN          string              `json:"pan"`
	BusinessType shared.BusinessType `json:"businessType"`
	UserRole     shared.UserRole     `json:"userRole"`
}

// BusinessOnboardingStatusDTO represents the onboarding status flags for a business.
type BusinessOnboardingStatusDTO struct {
	HasCompletedOnboarding bool `json:"hasCompletedOnboarding"`
	HasAddedPaymentMethod  bool `json:"hasAddedPaymentMethod"`
	HasAddedVendor         bool `json:"hasAddedVendor"`
	HasCreatedFirstPayment bool `json:"hasCreatedFirstPayment"`
}

// InitiateOnboardingRequest represents the request to initiate business onboarding
type InitiateOnboardingRequest struct {
	Name  string `json:"name" validate:"required"`
	GSTIN string `json:"gstin" validate:"required,gstin"`
}

// InviteUserRequest represents the request to invite a user to a business
type InviteUserRequest struct {
	Email string          `json:"email" validate:"required,email"`
	Name  string          `json:"name" validate:"required"`
	Role  shared.UserRole `json:"role" validate:"required"`
}

// UpdateBusinessUserRequest represents the request to update a business user's role
type UpdateBusinessUserRequest struct {
	UserID  string          `json:"userId" validate:"required"`
	NewRole shared.UserRole `json:"newRole" validate:"required"`
}

// RemoveBusinessUserRequest represents the request to remove a user from a business
type RemoveBusinessUserRequest struct {
	UserID string `json:"userId" validate:"required"`
}
