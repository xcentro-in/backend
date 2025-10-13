package business

import (
	"xcentro.in/internal/core/shared"
)

// Business represents a business entity.
type Business struct {
	BID          string              `json:"bid"`
	Name         string              `json:"name"`
	GSTIN        string              `json:"gstin"`
	GSTINData    string              `json:"gstinData"`
	ITID         string              `json:"itid"`
	KybStatus    shared.KYBstatus    `json:"kybStatus"`
	BusinessType shared.BusinessType `json:"businessType"`
	CountryCode  string              `json:"countryCode"`
	CurrencyCode string              `json:"currencyCode"`
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
	BID          string              `json:"bid"`
	BusinessType shared.BusinessType `json:"businessType"`
	ITID         string              `json:"itid"`          // Income Tax ID (PAN)
	CID          string              `json:"cid,omitempty"` // Company ID (CIN) - required for certain business types
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
	AddedAt   string          `json:"addedAt"`
	UpdatedAt string          `json:"updatedAt"`
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
