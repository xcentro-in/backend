package payment

// === REQUEST MODELS ===

// TokenizeRequest represents the request for card tokenization
type TokenizeRequest struct {
	CardNumber   string `json:"card_number"`
	ExpiryMonth  string `json:"expiry_month"`
	ExpiryYear   string `json:"expiry_year"`
	CVV          string `json:"cvv"`
	HolderName   string `json:"holder_name"`
	CardType     string `json:"card_type"`     // visa, mastercard, rupay, amex, diners
	CardCategory string `json:"card_category"` // credit, debit, corporate
}

// MakePaymentRequest represents the request for making a payment
type MakePaymentRequest struct {
	Amount        int64                  `json:"amount"`
	Currency      string                 `json:"currency"`
	PaymentMethod string                 `json:"payment_method"` // card, netbanking, upi, wallet
	Description   string                 `json:"description"`
	CardDetails   *CardDetails           `json:"card_details,omitempty"`
	NetBanking    *NetBankingDetails     `json:"net_banking,omitempty"`
	UPI           *UPIDetails            `json:"upi,omitempty"`
	Wallet        *WalletDetails         `json:"wallet,omitempty"`
	CallbackURL   string                 `json:"callback_url"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// CardDetails represents card payment details
type CardDetails struct {
	CardNumber   string `json:"card_number"`
	ExpiryMonth  string `json:"expiry_month"`
	ExpiryYear   string `json:"expiry_year"`
	CVV          string `json:"cvv"`
	HolderName   string `json:"holder_name"`
	CardType     string `json:"card_type"`
	CardCategory string `json:"card_category"`
}

// NetBankingDetails represents net banking payment details
type NetBankingDetails struct {
	BankCode string `json:"bank_code"`
	BankName string `json:"bank_name"`
}

// UPIDetails represents UPI payment details
type UPIDetails struct {
	VPA string `json:"vpa"`
}

// WalletDetails represents wallet payment details
type WalletDetails struct {
	WalletType string `json:"wallet_type"`
	WalletID   string `json:"wallet_id"`
}

// === RESPONSE MODELS ===

// FetchWalletResponse represents the response for fetching wallet information
type FetchWalletResponse struct {
	WalletID      string  `json:"wallet_id"`
	Balance       float64 `json:"balance"`
	Currency      string  `json:"currency"`
	WalletType    string  `json:"wallet_type"`
	Status        string  `json:"status"`
	LastUpdated   string  `json:"last_updated"`
}

// GetFeeRulesResponse represents the response for fee rules
type GetFeeRulesResponse struct {
	PaymentMethod string  `json:"payment_method"`
	FeePercentage float64 `json:"fee_percentage"`
	FixedFee      float64 `json:"fixed_fee"`
	MinFee        float64 `json:"min_fee"`
	MaxFee        float64 `json:"max_fee"`
	Currency      string  `json:"currency"`
}

// TokenizeResponse represents the response for card tokenization
type TokenizeResponse struct {
	Token          string `json:"token"`
	CardID         string `json:"card_id"`
	LastFourDigits string `json:"last_four_digits"`
	CardType       string `json:"card_type"`
	CardCategory   string `json:"card_category"`
	ExpiryMonth    string `json:"expiry_month"`
	ExpiryYear     string `json:"expiry_year"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

// MakePaymentResponse represents the response for payment processing
type MakePaymentResponse struct {
	PaymentID       string                 `json:"payment_id"`
	Status          string                 `json:"status"`
	Amount          int64                  `json:"amount"`
	Currency        string                 `json:"currency"`
	PaymentMethod   string                 `json:"payment_method"`
	GatewayUsed     string                 `json:"gateway_used"`
	GatewayPaymentID string                `json:"gateway_payment_id,omitempty"`
	RedirectURL     string                 `json:"redirect_url,omitempty"`
	Message         string                 `json:"message"`
	CreatedAt       string                 `json:"created_at"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// === COMMON RESPONSE MODELS ===

// StandardResponse represents a standard API response
type StandardResponse struct {
	Message  string      `json:"message"`
	Endpoint string      `json:"endpoint"`
	Status   string      `json:"status"`
	Data     interface{} `json:"data,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}