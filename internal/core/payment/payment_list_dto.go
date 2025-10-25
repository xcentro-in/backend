package payment

import "time"

// PaymentListItem represents a single payment in the list view
// Used for displaying payment history with aggregated voucher/bill information
type PaymentListItem struct {
	PaymentID     string               `json:"payoutId"`      // Using payoutId for frontend compatibility
	Date          time.Time            `json:"date"`
	Payees        []string             `json:"payees"`        // List of unique payee display names
	BillCount     int                  `json:"billCount"`     // Number of vouchers/bills
	PaymentMethod PaymentMethodDetails `json:"paymentMethod"`
	TotalAmount   float64              `json:"totalAmount"`   // In rupees (converted from paise)
	Status        string               `json:"status"`        // Completed, In Progress, Failed
	Breakdown     PaymentBreakdown     `json:"breakdown"`
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
	ConvenienceFee float64    `json:"convenienceFee"` // In rupees
	GSTOnFee       float64    `json:"gstOnFee"`       // GST on convenience fee in rupees
}

// BillItem represents a single bill/voucher in the breakdown
type BillItem struct {
	VoucherID string  `json:"voucherId"` // Voucher UUID for fetching full details
	BillID    string  `json:"billId"`    // Voucher number (e.g., PV-2024-00001)
	Amount    float64 `json:"amount"`    // Bill amount in rupees
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
