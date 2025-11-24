package payout

import "time"

// ============================================================================
// Ops Dashboard DTOs for Payout Transfers
// ============================================================================

// OpsPayoutStatus represents valid status filters for ops payout transfers
type OpsPayoutStatus string

const (
	OpsPayoutStatusInProgress OpsPayoutStatus = "in_progress" // initiated or processing
	OpsPayoutStatusCompleted  OpsPayoutStatus = "completed"   // completed with UTR
	OpsPayoutStatusFailed     OpsPayoutStatus = "failed"      // failed or timeout
)

// OpsListPayoutsRequest represents the request for listing payout transfers
type OpsListPayoutsRequest struct {
	Page   int             `query:"page" validate:"omitempty,min=1"`
	Limit  int             `query:"limit" validate:"omitempty,min=1,max=100"`
	Status OpsPayoutStatus `query:"status" validate:"omitempty,oneof=in_progress completed failed"`
}

// OpsPayoutListItem represents a single payout transfer in the ops list view
type OpsPayoutListItem struct {
	TransferRequestID string    `json:"transfer_request_id"`
	CreatedAt         time.Time `json:"created_at"`
	// Recipient info
	RecipientName        string `json:"recipient_name"`          // Payee display name
	BankAccountLast4     string `json:"bank_account_last_4"`     // Last 4 digits of bank account
	PayeeType            string `json:"payee_type"`              // vendor, employee, etc.
	Amount               int64  `json:"amount"`                  // Amount in paise
	Status               string `json:"status"`                  // initiated, processing, completed, failed, timeout
}

// OpsListPayoutsResponse represents the paginated response for ops payout transfers
type OpsListPayoutsResponse struct {
	Data       []*OpsPayoutListItem   `json:"data"`
	Pagination OpsPaginationResponse  `json:"pagination"`
}

// OpsPaginationResponse represents pagination metadata
type OpsPaginationResponse struct {
	CurrentPage int `json:"current_page"`
	PageSize    int `json:"page_size"`
	TotalItems  int `json:"total_items"`
	TotalPages  int `json:"total_pages"`
}

// ============================================================================
// Ops Payout Summary DTO (single payout transfer details)
// ============================================================================

// OpsPayoutSummary represents detailed summary of a single payout transfer
type OpsPayoutSummary struct {
	// Entity owning the voucher
	BusinessID   string `json:"business_id"`
	BusinessName string `json:"business_name"`

	// Recipient (payee) info
	PayeeRelationID string `json:"payee_relation_id"` // business_payee_relation composite key
	PayeeDisplayName string `json:"payee_display_name"`

	// Payout provider info
	PayoutProvider       string  `json:"payout_provider"`          // e.g., "castler"
	ProviderTransferID   string  `json:"provider_transfer_id"`     // Provider's transfer ID

	// Settlement info
	UTR            *string    `json:"utr"`              // Bank UTR (if settled)
	SettlementDate *time.Time `json:"settlement_date"` // Completion date (if settled)

	// Status
	PayoutStatus string `json:"payout_status"` // initiated, processing, completed, failed, timeout
}
