package payment

import "time"

// PaymentListItem represents a single payment in the list view
// Used for displaying payment history with aggregated voucher/bill information
type PaymentListItem struct {
	PaymentID           string               `json:"payoutId"`                // Using payoutId for frontend compatibility
	Date                time.Time            `json:"date"`
	Payees              []string             `json:"payees"`                  // List of unique payee display names
	BillCount           int                  `json:"billCount"`               // Number of vouchers/bills
	PaymentMethod       PaymentMethodDetails `json:"paymentMethod"`
	TotalAmount         int64                `json:"totalAmount"`             // In paise (smallest currency unit)
	Status              string               `json:"status"`                  // Completed, In Progress, Failed
	Breakdown           PaymentBreakdown     `json:"breakdown"`
	RetryOfPaymentID    *string              `json:"retryOfPaymentId,omitempty"` // ID of original payment if this is a retry
	RetryCount          int                  `json:"retryCount"`              // Number of times this payment has been retried
	HasSuccessfulRetry  bool                 `json:"hasSuccessfulRetry"`      // True if this failed payment was retried successfully
}

// PaymentMethodDetails represents payment method information for display
type PaymentMethodDetails struct {
	Type         string `json:"type"`                   // "credit", "bank", "upi"
	CardProvider string `json:"cardProvider,omitempty"` // "Visa", "Mastercard", "Amex", etc.
	Bank         string `json:"bank,omitempty"`         // Bank name for net banking
	Last4        string `json:"last4"`                  // Last 4 digits of card/account
}

// PaymentBreakdown represents the detailed breakdown of a payment
type PaymentBreakdown struct {
	Bills          []BillItem `json:"bills"`
	ConvenienceFee int64      `json:"convenienceFee"` // In paise (smallest currency unit)
	GSTOnFee       int64      `json:"gstOnFee"`       // GST on convenience fee in paise
}

// BillItem represents a single bill/voucher in the breakdown
type BillItem struct {
	VoucherID string `json:"voucherId"` // Voucher UUID for fetching full details
	BillID    string `json:"billId"`    // Voucher number (e.g., PV-2024-00001)
	Amount    int64  `json:"amount"`    // Bill amount in paise (smallest currency unit)
}

// PaymentListRequest represents pagination and filter parameters
type PaymentListRequest struct {
	BusinessID string `json:"business_id"` // From header
	Limit      int    `json:"limit"`       // Page size
	Offset     int    `json:"offset"`      // Pagination offset
}

// PaymentListResponse represents the paginated response
type PaymentListResponse struct {
	Payments   []PaymentListItem `json:"payments"`
	TotalCount int               `json:"total_count"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
}

// ============================================================================
// Ops Dashboard DTOs for Collections
// ============================================================================

// OpsCollectionStatus represents valid status filters for ops collections
type OpsCollectionStatus string

const (
	OpsCollectionStatusPending     OpsCollectionStatus = "pending"     // Payment pending
	OpsCollectionStatusSuccess     OpsCollectionStatus = "success"     // Payment successful but not yet settled
	OpsCollectionStatusFailed      OpsCollectionStatus = "failed"      // Payment failed
	OpsCollectionStatusPayoutReady OpsCollectionStatus = "payoutReady" // Payment settled and ready for payout
)

// OpsListCollectionsRequest represents the request for listing collections
type OpsListCollectionsRequest struct {
	Page   int                 `query:"page" validate:"omitempty,min=1"`
	Limit  int                 `query:"limit" validate:"omitempty,min=1,max=100"`
	Status OpsCollectionStatus `query:"status" validate:"omitempty,oneof=pending success failed payoutReady"`
}

// OpsCollectionListItem represents a single collection in the ops list view
type OpsCollectionListItem struct {
	PaymentID     string        `json:"payment_id"`
	Date          time.Time     `json:"date"`
	TotalAmount   int64         `json:"total_amount"` // In paise (smallest currency unit)
	Status        PaymentStatus `json:"status"`
	BusinessID    string        `json:"business_id"`
	BusinessName  string        `json:"business_name"`
	PaymentMethod PaymentMethod `json:"payment_method"`
}

// OpsListCollectionsResponse represents the paginated response for ops collections
type OpsListCollectionsResponse struct {
	Data       []*OpsCollectionListItem `json:"data"`
	Pagination PaginationResponse       `json:"pagination"`
}

// PaginationResponse represents pagination metadata
type PaginationResponse struct {
	CurrentPage int `json:"current_page"`
	PageSize    int `json:"page_size"`
	TotalItems  int `json:"total_items"`
	TotalPages  int `json:"total_pages"`
}

// ============================================================================
// Ops Collection Summary DTOs (lightweight single payment view)
// ============================================================================

// OpsCollectionSummary represents lightweight summary for a single collection
// Use this for quick overview; full details endpoint coming later
type OpsCollectionSummary struct {
	PaymentID     string                  `json:"payment_id"`
	Date          time.Time               `json:"date"`
	TotalAmount   int64                   `json:"total_amount"`
	Status        PaymentStatus           `json:"status"`
	PaymentMethod OpsPaymentMethodDetails `json:"payment_method"`
	Gateway       OpsGatewayDetails       `json:"gateway"`
	Payees        []OpsPayeeDetails       `json:"payees"`
}

// OpsPaymentMethodDetails represents detailed payment method info for ops
type OpsPaymentMethodDetails struct {
	Type        string `json:"type"`                   // "card" or "bank"
	// Card-specific fields
	CardType    string `json:"card_type,omitempty"`    // "credit", "debit", "corporate"
	CardNetwork string `json:"card_network,omitempty"` // "Visa", "Mastercard", "Amex", "RuPay", "Diners"
	IssuerBank  string `json:"issuer_bank,omitempty"`  // Bank that issued the card
	Last4       string `json:"last_4,omitempty"`       // Last 4 digits
	// Bank-specific fields
	BankName    string `json:"bank_name,omitempty"`    // Bank name for net banking/account
	IFSC        string `json:"ifsc,omitempty"`         // IFSC code
}

// OpsGatewayDetails represents payment gateway information
type OpsGatewayDetails struct {
	Name          string `json:"name"`           // Gateway name: "jiopay", "easebuzz"
	TransactionID string `json:"transaction_id"` // Gateway-generated transaction ID
}

// OpsPayeeDetails represents payee information for the collection
type OpsPayeeDetails struct {
	RelationID  string `json:"relation_id"`  // business_payee_relation.relation_id
	DisplayName string `json:"display_name"` // Payee display name
}
