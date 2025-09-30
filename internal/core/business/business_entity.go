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
