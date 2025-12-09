package business_rules

import (
	"context"
	"time"
)

// GSTVerificationService defines the interface for GST verification services
// Used for fetching business details from GSTIN via 3rd party APIs (AuthBridge, etc.)
type GSTVerificationService interface {
	// VerifyGSTIN performs GST verification and fetches business details
	VerifyGSTIN(ctx context.Context, request GSTVerificationRequest) (*GSTVerificationResponse, error)

	// GetProviderType returns the verification service provider identifier
	GetProviderType() GSTVerificationProvider
}

// GSTVerificationProvider represents different GST verification service providers
type GSTVerificationProvider string

const (
	GSTProviderAuthBridge GSTVerificationProvider = "authbridge" // Real 3rd party API -- this will replacce the Mock written below
	GSTProviderMock       GSTVerificationProvider = "mock"       // Mock for testing
)

// GSTVerificationRequest represents a request to verify a GSTIN
type GSTVerificationRequest struct {
	GSTIN     string `json:"gstin"`      // 15-character GST identification number
	RequestID string `json:"request_id"` // Unique request identifier for tracking
}

// GSTVerificationResponse represents the response from GST verification
type GSTVerificationResponse struct {
	RequestID        string        `json:"request_id"`
	GSTIN            string        `json:"gstin"`
	LegalName        string        `json:"legal_name"`         // Official registered name
	TradeName        string        `json:"trade_name"`         // Trade/brand name
	ProprietorName   []string      `json:"proprietor_name,omitempty"` // For proprietorship firms
	BusinessType     string        `json:"business_type"`      // Proprietorship, Partnership, Private Limited, etc.
	RegistrationDate time.Time     `json:"registration_date"`
	Status           string        `json:"status"`             // Active, Cancelled, Suspended
	StateCode        string        `json:"state_code"`         // First 2 digits of GSTIN
	Address          GSTAddress    `json:"address"`
	Directors        []GSTDirector `json:"directors,omitempty"` // For companies/LLPs
	Metadata         map[string]any `json:"metadata,omitempty"` // Full API response
}

// GSTAddress represents a business address from GST data
type GSTAddress struct {
	BuildingName string `json:"building_name"`
	Street       string `json:"street"`
	City         string `json:"city"`
	State        string `json:"state"`
	Pincode      string `json:"pincode"`
}

// GSTDirector represents a director/partner from GST data
type GSTDirector struct {
	Name string `json:"name"`
	DIN  string `json:"din,omitempty"` // Director Identification Number (optional)
}

// GSTVerificationError represents specific error types for GST verification
type GSTVerificationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *GSTVerificationError) Error() string {
	return e.Message
}

// Common GST verification error codes
const (
	ErrCodeGSTInvalidGSTIN = "INVALID_GSTIN"
	ErrCodeGSTCancelled    = "GST_CANCELLED"
	ErrCodeGSTSuspended    = "GST_SUSPENDED"
	ErrCodeGSTAPITimeout   = "GST_API_TIMEOUT"
	ErrCodeGSTAPIError     = "GST_API_ERROR"
)

// NewGSTVerificationError creates a new GST verification error
func NewGSTVerificationError(code, message, details string) *GSTVerificationError {
	return &GSTVerificationError{
		Code:    code,
		Message: message,
		Details: details,
	}
}
