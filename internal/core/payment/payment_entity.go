package payment

import "time"

// Payment represents a payment transaction
type Payment struct {
	ID               string        `json:"id"`
	UserID           string        `json:"user_id"`
	BusinessID       string        `json:"business_id"`
	Amount           int64         `json:"amount"` // Amount in smallest currency unit (paise for INR)
	Currency         Currency      `json:"currency"`
	PaymentMethod    PaymentMethod `json:"payment_method"`
	PaymentType      PaymentType   `json:"payment_type"`      // Type: payment, tokenization, refund
	Status           PaymentStatus `json:"status"`
	Description      string        `json:"description"`
	GatewayUsed      GatewayType   `json:"gateway_used"`
	GatewayPaymentID string        `json:"gateway_payment_id,omitempty"`
	CallbackURL      string        `json:"callback_url"`
	RedirectURL      string        `json:"redirect_url,omitempty"`
	// Link to payment source used (for tokenized payments)
	PaymentSourceID   *string            `json:"payment_source_id,omitempty"`   // UUID of card/bank account
	PaymentSourceType *PaymentSourceType `json:"payment_source_type,omitempty"` // 'card', 'bank_account', etc.
	CardDetails       *Card              `json:"card_details,omitempty"`
	NetBankingDetails *NetBanking        `json:"net_banking_details,omitempty"`
	UPIDetails        *UPI               `json:"upi_details,omitempty"`
	WalletDetails     *Wallet            `json:"wallet_details,omitempty"`
	FeeAmount         int64              `json:"fee_amount"`
	TotalAmount       int64              `json:"total_amount"` // Amount + Fee
	Metadata          map[string]any     `json:"metadata,omitempty"`
	// Settlement tracking (Phase 7)
	SettlementStatus      SettlementStatus       `json:"settlement_status"`
	SettledAt             *time.Time             `json:"settled_at,omitempty"`
	SettlementReferenceID string                 `json:"settlement_reference_id,omitempty"`
	SettlementUTR         string                 `json:"settlement_utr,omitempty"`
	SettlementMetadata    map[string]interface{} `json:"settlement_metadata,omitempty"`
	// Settlement financial tracking (Phase 7.1)
	SettlementAmount    *int64    `json:"settlement_amount,omitempty"`     // Actual amount settled (may differ from total_amount)
	MDRAmount           int64     `json:"mdr_amount"`                      // Merchant Discount Rate charged by gateway
	NetSettlementAmount *int64    `json:"net_settlement_amount,omitempty"` // Final amount: settlement_amount - mdr_amount
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// Card represents card payment details
type Card struct {
	Number         string       `json:"number,omitempty"` // Masked for security
	ExpiryMonth    string       `json:"expiry_month"`
	ExpiryYear     string       `json:"expiry_year"`
	CVV            string       `json:"cvv,omitempty"` // Never stored
	HolderName     string       `json:"holder_name"`
	CardType       CardType     `json:"card_type"`
	CardCategory   CardCategory `json:"card_category"`
	LastFourDigits string       `json:"last_four_digits"`
	Token          string       `json:"token,omitempty"` // For tokenized payments
}

// NetBanking represents net banking payment details
type NetBanking struct {
	BankCode string `json:"bank_code"`
	BankName string `json:"bank_name"`
}

// UPI represents UPI payment details
type UPI struct {
	VPA string `json:"vpa"` // Virtual Payment Address
}

// Wallet represents wallet payment details (legacy - kept for backward compatibility)
type Wallet struct {
	WalletID string   `json:"wallet_id"`
	Balance  int64    `json:"balance"` // Current balance in smallest currency unit
	Currency Currency `json:"currency"`
}

// TokenizedCard represents a tokenized card for future payments
type TokenizedCard struct {
	ID             string       `json:"id"`
	UserID         string       `json:"user_id"`
	Token          string       `json:"token"`
	CardType       CardType     `json:"card_type"`
	CardCategory   CardCategory `json:"card_category"`
	LastFourDigits string       `json:"last_four_digits"`
	ExpiryMonth    string       `json:"expiry_month"`
	ExpiryYear     string       `json:"expiry_year"`
	GatewayUsed    GatewayType  `json:"gateway_used"`
	IsActive       bool         `json:"is_active"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// TokenCardDetails represents tokenized card payment details
type TokenCardDetails struct {
	CardID string `json:"card_id"` // Reference to stored card
}

// PaymentRequest represents a request to process a payment
type PaymentRequest struct {
	PaymentID         string            `json:"payment_id,omitempty"` // Set by service layer after creating payment record
	CallbackURL       string            `json:"callback_url,omitempty"` // Set by service layer based on calling device
	UserID            string            `json:"user_id"`
	BusinessID        string            `json:"business_id"`
	Amount            int64             `json:"amount"`
	Currency          Currency          `json:"currency"`
	PaymentMethod     PaymentMethod     `json:"payment_method"`
	Description       string            `json:"description"`
	CallingDevice     CallingDevice     `json:"calling_device"`       // Device type initiating payment (web, mobile, pos)
	CardDetails       *Card             `json:"card_details,omitempty"`
	TokenCardDetails  *TokenCardDetails `json:"token_card_details,omitempty"`
	NetBankingDetails *NetBanking       `json:"net_banking_details,omitempty"`
	UPIDetails        *UPI              `json:"upi_details,omitempty"`
	WalletDetails     *Wallet           `json:"wallet_details,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
	// Voucher integration (Phase 6)
	VoucherIDs []string `json:"voucher_ids,omitempty"` // Draft voucher IDs to finalize on payment success
}

// PaymentResponse represents a response from payment processing
type PaymentResponse struct {
	PaymentID        string         `json:"payment_id"`
	Status           PaymentStatus  `json:"status"`
	Amount           int64          `json:"amount"`
	FeeAmount        int64          `json:"fee_amount"`
	TotalAmount      int64          `json:"total_amount"`
	Currency         Currency       `json:"currency"`
	PaymentMethod    PaymentMethod  `json:"payment_method"`
	GatewayUsed      GatewayType    `json:"gateway_used"`
	GatewayPaymentID string         `json:"gateway_payment_id,omitempty"`
	RedirectURL      string         `json:"redirect_url,omitempty"`
	Message          string         `json:"message"`
	CreatedAt        time.Time      `json:"created_at"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	// Voucher finalization results (Phase 6)
	VouchersFinalized []string `json:"vouchers_finalized,omitempty"` // Successfully finalized voucher IDs
	VoucherStatus     string   `json:"voucher_status,omitempty"`     // "finalized", "partial", "failed"
}

// TokenizeRequest represents a request to tokenize a card
type TokenizeRequest struct {
	UserID        string `json:"user_id"`
	BusinessID    string `json:"business_id"`
	TransactionID string `json:"transaction_id,omitempty"` // Payment ID for the ₹1 tokenization payment
	CardDetails   Card   `json:"card_details"`
}

// TokenizeResponse represents a response from card tokenization
type TokenizeResponse struct {
	Token          string       `json:"token"`
	CardID         string       `json:"card_id"`
	LastFourDigits string       `json:"last_four_digits"`
	CardType       CardType     `json:"card_type"`
	CardCategory   CardCategory `json:"card_category"`
	ExpiryMonth    string       `json:"expiry_month"`
	ExpiryYear     string       `json:"expiry_year"`
	GatewayUsed    GatewayType  `json:"gateway_used"`
	Status         string       `json:"status"`
	Message        string       `json:"message"`
	CreatedAt      time.Time    `json:"created_at"`
}
