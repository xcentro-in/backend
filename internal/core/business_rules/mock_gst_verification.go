package business_rules

import (
	"context"
	"time"
)

// MockGSTVerificationService provides a mock implementation of GSTVerificationService
// Returns hardcoded success responses for development and testing
// No pattern-based logic - simple mock to avoid wasting resources
type MockGSTVerificationService struct {
	provider GSTVerificationProvider
}

// NewMockGSTVerificationService creates a new mock GST verification service
func NewMockGSTVerificationService() GSTVerificationService {
	return &MockGSTVerificationService{
		provider: GSTProviderMock,
	}
}

// VerifyGSTIN returns a hardcoded successful GST verification response
// Simulates API delay but no actual verification logic
func (m *MockGSTVerificationService) VerifyGSTIN(ctx context.Context, request GSTVerificationRequest) (*GSTVerificationResponse, error) {
	// Simulate API delay (500ms)
	time.Sleep(500 * time.Millisecond)

	// Return hardcoded success response
	// Real implementation will call AuthBridge or other 3rd party API
	return &GSTVerificationResponse{
		RequestID:        request.RequestID,
		GSTIN:            request.GSTIN,
		LegalName:        "MOCK BUSINESS PRIVATE LIMITED",
		TradeName:        "Mock Business",
		BusinessType:     "Private Limited Company",
		RegistrationDate: time.Now().AddDate(-2, 0, 0), // Registered 2 years ago
		Status:           "Active",
		StateCode:        "27", // Maharashtra
		Address: GSTAddress{
			BuildingName: "Mock Plaza",
			Street:       "Mock Street, Andheri East",
			City:         "Mumbai",
			State:        "Maharashtra",
			Pincode:      "400069",
		},
		Directors: []GSTDirector{
			{Name: "JOHN DOE", DIN: ""},
			{Name: "JANE SMITH", DIN: ""},
		},
		Metadata: map[string]any{
			"provider": "mock",
			"note":     "Mock implementation - always returns success",
		},
	}, nil
}

// GetProviderType returns the provider type
func (m *MockGSTVerificationService) GetProviderType() GSTVerificationProvider {
	return m.provider
}
