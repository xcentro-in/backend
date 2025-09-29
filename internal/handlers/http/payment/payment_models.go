package payment

// === REQUEST MODELS ===

// FetchWalletRequest represents the request for fetching business wallet information
type FetchWalletRequest struct {
	BusinessID string `json:"business_id"`
}

// GetFeeRulesRequest represents the request for getting fee rules
type GetFeeRulesRequest struct {
	PaymentMethod string `json:"payment_method"`
	CardType      string `json:"card_type,omitempty"`
	CardCategory  string `json:"card_category,omitempty"`
	Amount        int64  `json:"amount,omitempty"`
}

// TokenizeRequest represents the request for card tokenization
type TokenizeRequest struct {
	UserID      string       `json:"user_id"`
	CardDetails *CardDetails `json:"card_details"`
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
	Metadata      map[string]any 		 `json:"metadata,omitempty"`
}

// CardDetails represents card payment details
type CardDetails struct {
	CardNumber     string `json:"card_number"`
	ExpiryMonth    string `json:"expiry_month"`
	ExpiryYear     string `json:"expiry_year"`
	CVV            string `json:"cvv"`
	HolderName     string `json:"holder_name"`
	CardType       string `json:"card_type"`
	CardCategory   string `json:"card_category"`
	LastFourDigits string `json:"last_four_digits,omitempty"`
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

// FetchWalletResponse represents the response for fetching business wallet information
type FetchWalletResponse struct {
	BusinessID   string                   `json:"business_id"`
	StoredCards  []StoredCardResponse     `json:"stored_cards"`
	BankAccounts []BankAccountResponse    `json:"bank_accounts"`
	UPIDetails   []UPIDetailResponse      `json:"upi_details"`
	CreditLines  []CreditLineResponse     `json:"credit_lines"`
	LastUpdated  string                   `json:"last_updated"`
}

// StoredCardResponse represents a stored card in the API response
type StoredCardResponse struct {
	CardID         string `json:"card_id"`
	LastFourDigits string `json:"last_four_digits"`
	CardType       string `json:"card_type"`
	CardCategory   string `json:"card_category"`
	ExpiryMonth    string `json:"expiry_month"`
	ExpiryYear     string `json:"expiry_year"`
	IssuerBank     string `json:"issuer_bank"`
	IsActive       bool   `json:"is_active"`
	Nickname       string `json:"nickname,omitempty"`
}

// BankAccountResponse represents a bank account in the API response
type BankAccountResponse struct {
	AccountID       string `json:"account_id"`
	LastFiveDigits  string `json:"last_five_digits"`
	BankName        string `json:"bank_name"`
	IFSCCode        string `json:"ifsc_code"`
	AccountType     string `json:"account_type"`
	IsActive        bool   `json:"is_active"`
	IsVerified      bool   `json:"is_verified"`
	Nickname        string `json:"nickname,omitempty"`
}

// UPIDetailResponse represents a UPI detail in the API response
type UPIDetailResponse struct {
	UPIID      string `json:"upi_id"`
	UPIAddress string `json:"upi_address"`
	Provider   string `json:"provider"`
	IsActive   bool   `json:"is_active"`
	IsVerified bool   `json:"is_verified"`
	Nickname   string `json:"nickname,omitempty"`
}

// CreditLineResponse represents a credit line in the API response
type CreditLineResponse struct {
	CreditLineID   string  `json:"credit_line_id"`
	Provider       string  `json:"provider"`
	AvailableLimit float64 `json:"available_limit"`
	TotalLimit     float64 `json:"total_limit"`
	Currency       string  `json:"currency"`
	Status         string  `json:"status"`
	IsActive       bool    `json:"is_active"`
	Nickname       string  `json:"nickname,omitempty"`
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
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// === COMMON RESPONSE MODELS ===

// StandardResponse represents a standard API response
type StandardResponse struct {
	Message  string      `json:"message"`
	Endpoint string      `json:"endpoint"`
	Status   string      `json:"status"`
	Data     any `json:"data,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}