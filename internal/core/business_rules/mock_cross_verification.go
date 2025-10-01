package business_rules

import (
	"context"
	"time"
)

// MockCrossVerificationService provides a mock implementation of CrossVerificationService
// Simple pass/fail logic with no fuzzy matching algorithms
// Real implementation will call 3rd party APIs
type MockCrossVerificationService struct{}

// NewMockCrossVerificationService creates a new mock cross-verification service
func NewMockCrossVerificationService() CrossVerificationService {
	return &MockCrossVerificationService{}
}

// VerifyGSTBankMatch returns a simple mock approval
// No fuzzy matching logic - waste of resources when using 3rd party API
func (m *MockCrossVerificationService) VerifyGSTBankMatch(ctx context.Context, request CrossVerificationRequest) (*CrossVerificationResponse, error) {
	// Simulate API delay (300ms)
	time.Sleep(300 * time.Millisecond)

	// Simple mock: always return approve with high score
	// Real implementation will call 3rd party API (AuthBridge, etc.)
	return &CrossVerificationResponse{
		IsMatch:           true,
		MatchScore:        0.95,
		RecommendedAction: "approve",
		Metadata: map[string]any{
			"provider": "mock",
			"note":     "Mock implementation - always approves GST-Bank name match",
		},
	}, nil
}

// CheckMoneyRotation is a placeholder for anti-money rotation logic
// Will be fully implemented when Business table director data is available
func (m *MockCrossVerificationService) CheckMoneyRotation(ctx context.Context, request MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error) {
	// TODO: Will be implemented when Business table director data is available
	// This check prevents scenarios like:
	// Business A (Director: John Doe) → Payee C (Account Holder: John Doe)
	// This is money rotation/laundering

	// FUTURE IMPLEMENTATION PLAN:
	// 1. Call BusinessService.GetBusinessDirectors(request.CustomerBusinessID)
	// 2. Compare payee account name with customer business director names
	// 3. Compare payee GST directors with customer business directors
	// 4. If match found: return rejection
	// 5. If no match: return approval

	// For now, placeholder that always approves
	return &MoneyRotationCheckResponse{
		IsMoneyRotation:   false,
		MatchedDirector:   "",
		RecommendedAction: "approve",
		Details:           "Mock implementation - money rotation check not yet implemented (waiting on Business table director data)",
	}, nil
}
