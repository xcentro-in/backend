package business_rules

import "context"

// CrossVerificationService defines the interface for cross-verification operations
// Handles name matching between GST and bank data, and anti-money rotation checks
type CrossVerificationService interface {
	// VerifyGSTBankMatch checks if GST legal name matches bank account holder name
	// Will call 3rd party API (AuthBridge or similar) for fuzzy name matching
	VerifyGSTBankMatch(ctx context.Context, request CrossVerificationRequest) (*CrossVerificationResponse, error)

	// CheckMoneyRotation verifies payee account holder is NOT a director of customer business
	// Prevents money laundering: Business A → Director of Business A (circular flow)
	CheckMoneyRotation(ctx context.Context, request MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error)
}

// CrossVerificationRequest represents a request to verify GST-Bank name match
type CrossVerificationRequest struct {
	GSTIN           string `json:"gstin"`
	GSTLegalName    string `json:"gst_legal_name"`     // Name from GST verification
	BankAccountName string `json:"bank_account_name"`  // Name from bank verification
	BankIFSC        string `json:"bank_ifsc"`
}

// CrossVerificationResponse represents the response from GST-Bank name matching
type CrossVerificationResponse struct {
	IsMatch           bool           `json:"is_match"`            // Whether names match
	MatchScore        float64        `json:"match_score"`         // Match confidence from API (0.0 to 1.0)
	RecommendedAction string         `json:"recommended_action"`  // "approve", "manual_review", "reject"
	Metadata          map[string]any `json:"metadata,omitempty"`  // Full API response
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
