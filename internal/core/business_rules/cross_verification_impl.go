package business_rules

import (
	"context"
	"fmt"
	"strings"
)

// CrossVerificationServiceImpl provides the implementation of CrossVerificationService
// Handles anti-fraud verification operations
type CrossVerificationServiceImpl struct{}

// NewCrossVerificationService creates a new cross-verification service
func NewCrossVerificationService() CrossVerificationService {
	return &CrossVerificationServiceImpl{}
}

// CheckMoneyRotation verifies payee account holder is NOT a director of customer business
// Prevents money laundering: Business A → Director of Business A (circular flow)
func (s *CrossVerificationServiceImpl) CheckMoneyRotation(ctx context.Context, request MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error) {
	// Normalize payee account name for comparison
	normalizedPayeeName := strings.TrimSpace(strings.ToLower(request.PayeeAccountName))

	// Compare payee account holder name against each director
	for _, director := range request.PayeeGSTDirectors {
		normalizedDirectorName := strings.TrimSpace(strings.ToLower(director))

		// Check for exact match (case-insensitive)
		if normalizedPayeeName == normalizedDirectorName {
			return &MoneyRotationCheckResponse{
				IsMoneyRotation:   true,
				MatchedDirector:   director,
				RecommendedAction: "reject",
				Details:           fmt.Sprintf("Money rotation detected: Account holder '%s' matches business director '%s'", request.PayeeAccountName, director),
			}, nil
		}
	}

	// No director match found - approve
	return &MoneyRotationCheckResponse{
		IsMoneyRotation:   false,
		MatchedDirector:   "",
		RecommendedAction: "approve",
		Details:           "No director match found - money rotation check passed",
	}, nil
}
