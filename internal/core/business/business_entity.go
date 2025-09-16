package business

import (
	"time"

	"xcentro.in/internal/core/shared"
)

// Business represents a business entity.
type Business struct {
	BID          string `json:"bid"`
	Name         string `json:"name"`
	ITID         string `json:"itid"`
	KybStatus    string `json:"kybStatus"`
	BusinessType string `json:"businessType"`
	CountryCode  string `json:"countryCode"`
	CurrencyCode string `json:"currencyCode"`
}

// BusinessUserRelation represents the many-to-many relationship between users and businesses.
type BusinessUserRelation struct {
	UserID    string          `json:"userid"`
	BID       string          `json:"bid"`
	Role      shared.UserRole `json:"role"`
	RoleClaim shared.UserRole `json:"roleClaim"`
	AddedAt   time.Time       `json:"addedAt"`
	Timestamp time.Time       `json:"timestamp"`
}
