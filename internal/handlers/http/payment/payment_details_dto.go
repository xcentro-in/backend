package payment

// PaymentDetailsResponse represents comprehensive payment details for user-facing API
type PaymentDetailsResponse struct {
	PaymentID        string                 `json:"payment_id"`
	Status           string                 `json:"status"`
	NetAmount        int64                  `json:"net_amount"`         // Base amount paid (in paise)
	ServiceCharge    int64                  `json:"service_charge"`     // Convenience fee (in paise)
	ServiceChargeGST int64                  `json:"service_charge_gst"` // GST on convenience fee (in paise)
	TotalAmount      int64                  `json:"total_amount"`       // Net + Service Charge + GST (in paise)
	Currency         string                 `json:"currency"`
	PaymentMethod    string                 `json:"payment_method"`
	Description      string                 `json:"description"`
	PaymentSource    *PaymentSourceDetails  `json:"payment_source,omitempty"`
	Bills            []BillDetails          `json:"bills"`
	Activity         []PaymentActivityItem  `json:"activity"`
	CreatedAt        string                 `json:"created_at"`
}

// PaymentSourceDetails represents payment source information (card/bank)
type PaymentSourceDetails struct {
	SourceType     string  `json:"source_type"`      // card, bank_account
	LastFourDigits *string `json:"last_four_digits,omitempty"`
	LastFiveDigits *string `json:"last_five_digits,omitempty"` // For bank accounts
	BankName       string  `json:"bank_name,omitempty"`
	CardType       *string `json:"card_type,omitempty"`     // visa, mastercard, etc.
	CardCategory   *string `json:"card_category,omitempty"` // credit, debit
}

// BillDetails represents voucher/bill information linked to payment
type BillDetails struct {
	VoucherID     string  `json:"voucher_id"`
	InvoiceNumber string  `json:"invoice_number"`
	PayeeName     string  `json:"payee_name"`
	PayeeType     string  `json:"payee_type"` // vendor, employee, biller, gst_payment
	GSTIN         *string `json:"gstin,omitempty"`
	RelationID    string  `json:"relation_id"`
	Amount        int64   `json:"amount"` // Net amount for this bill (in paise)
}

// PaymentActivityItem represents a single activity entry
type PaymentActivityItem struct {
	Action    string `json:"action"`
	ActorName string `json:"actor_name"` // User name or system actor
	Timestamp string `json:"timestamp"`
}
