package dashboard

import "time"

// DashboardMetrics represents all metrics displayed on the dashboard
type DashboardMetrics struct {
	TotalSpendThisMonth     int64                     `json:"total_spend_this_month"`      // in paise
	OverdueBills            OverdueBillsSummary       `json:"overdue_bills"`
	RecentPayments          []RecentPaymentSummary    `json:"recent_payments"`
	UpcomingLargePayments   []UpcomingPaymentSummary  `json:"upcoming_large_payments"`
	MonthlySpends           []MonthlySpendSummary     `json:"monthly_spends"`
	PaymentSourceBreakdown  []PaymentSourceSummary    `json:"payment_source_breakdown"`
	PendingApprovals        int                       `json:"pending_approvals"`
}

// OverdueBillsSummary represents overdue bills count and total value
type OverdueBillsSummary struct {
	Count       int   `json:"count"`
	TotalAmount int64 `json:"total_amount"` // in paise
}

// RecentPaymentSummary represents a recent payment (last 6 in 2 weeks)
type RecentPaymentSummary struct {
	PaymentID     string    `json:"payment_id"`
	Amount        int64     `json:"amount"`         // in paise
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpcomingPaymentSummary represents an upcoming large payment
type UpcomingPaymentSummary struct {
	VoucherID       string     `json:"voucher_id"`
	VoucherNumber   string     `json:"voucher_number"`
	PayeeID         string     `json:"payee_id"`
	NetAmount       int64      `json:"net_amount"` // in paise
	InvoiceDueDate  *time.Time `json:"invoice_due_date"`
	InvoiceNumber   *string    `json:"invoice_number"`
}

// MonthlySpendSummary represents total spend for a specific month
type MonthlySpendSummary struct {
	Month       time.Time `json:"month"`        // First day of the month
	TotalSpend  int64     `json:"total_spend"`  // in paise
	TxnCount    int       `json:"txn_count"`
}

// PaymentSourceSummary represents spending breakdown by actual payment source
// (e.g., "Visa •••• 1234", "HDFC Bank", "UPI")
type PaymentSourceSummary struct {
	PaymentMethod  string  `json:"payment_method"`   // card, token_card, net_banking, upi
	SourceLabel    string  `json:"source_label"`     // "Visa •••• 1234", "HDFC Bank", "UPI"
	CardNetwork    *string `json:"card_network"`     // visa, mastercard, rupay, amex (only for cards)
	LastFourDigits *string `json:"last_four_digits"` // only for cards
	BankName       *string `json:"bank_name"`        // only for bank accounts
	TotalAmount    int64   `json:"total_amount"`     // in paise
	TxnCount       int     `json:"txn_count"`
}
