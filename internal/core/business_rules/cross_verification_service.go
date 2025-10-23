package business_rules

import "context"

// CrossVerificationService defines the interface for anti-fraud verification operations
// Handles money rotation detection during payee onboarding
type CrossVerificationService interface {
	// CheckMoneyRotation verifies payee account holder is NOT a director of customer business
	// Prevents money laundering: Business A → Director of Business A (circular flow)
	CheckMoneyRotation(ctx context.Context, request MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error)
}

// MoneyRotationCheckRequest represents a request to check for money rotation
// Verifies that payee is not sending money back to directors of customer business
type MoneyRotationCheckRequest struct {
	CustomerBusinessID string   `json:"customer_business_id"` // Our customer business (FK → business.bid)
	PayeeAccountName   string   `json:"payee_account_name"`   // Name from bank verification
	PayeeGSTDirectors  []string `json:"payee_gst_directors"`  // Directors from GST verification
}

// MoneyRotationCheckResponse represents the result of money rotation check
type MoneyRotationCheckResponse struct {
	IsMoneyRotation   bool   `json:"is_money_rotation"`   // Whether rotation detected
	MatchedDirector   string `json:"matched_director"`    // Which director matched (if any)
	RecommendedAction string `json:"recommended_action"`  // "approve", "reject"
	Details           string `json:"details"`             // Explanation
}
