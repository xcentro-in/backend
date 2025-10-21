package payment

// === REQUEST MODELS ===

// FetchWalletRequest represents the request for fetching business wallet information
type FetchWalletRequest struct {
	// No fields needed - businessID comes from X-Business-ID header
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
	CardDetails *CardDetails `json:"card_details"`
}

// MakePaymentRequest represents the request for making a payment
type MakePaymentRequest struct {
	Amount        int64              `json:"amount"`
	Currency      string             `json:"currency"`
	PaymentMethod string             `json:"payment_method"` // card, token-card, netbanking, upi, wallet
	Description   string             `json:"description"`
	CallingDevice string             `json:"calling_device"` // web, mobile, pos (default: web)
	VoucherIDs    []string           `json:"voucher_ids,omitempty"` // Optional: Draft voucher IDs to finalize on payment success
	CardDetails   *CardDetails       `json:"card_details,omitempty"`
	TokenDetails  *TokenCardDetails  `json:"token_details,omitempty"`
	NetBanking    *NetBankingDetails `json:"net_banking,omitempty"`
	UPI           *UPIDetails        `json:"upi,omitempty"`
	Wallet        *WalletDetails     `json:"wallet,omitempty"`
	Metadata      map[string]any     `json:"metadata,omitempty"`
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

// TokenCardDetails represents tokenized card payment details
type TokenCardDetails struct {
	CardID string `json:"card_id"` // Reference to stored card (primary key)
}

// AddPaymentSourceRequest represents the request for adding a payment source to wallet
type AddPaymentSourceRequest struct {
	BusinessID         string              `json:"business_id"`
	SourceType         string              `json:"source_type"` // bank_account, upi, etc.
	BankAccountDetails *BankAccountDetails `json:"bank_account_details,omitempty"`
	UPIAccountDetails  *UPIAccountDetails  `json:"upi_account_details,omitempty"`
}

// BankAccountDetails represents bank account details for verification and addition
type BankAccountDetails struct {
	AccountNumber     string `json:"account_number"`
	IFSCCode          string `json:"ifsc_code"`
	AccountHolderName string `json:"account_holder_name"`
	BankName          string `json:"bank_name"`
	BranchName        string `json:"branch_name,omitempty"`
	AccountType       string `json:"account_type"` // savings, current, etc.
}

// UPIAccountDetails represents UPI account details for verification and addition
type UPIAccountDetails struct {
	UPIAddress string `json:"upi_address"`
	Provider   string `json:"provider,omitempty"`
}

// === RESPONSE MODELS ===

// FetchWalletResponse represents the response for fetching business wallet information
type FetchWalletResponse struct {
	BusinessID   string                `json:"business_id"`
	StoredCards  []StoredCardResponse  `json:"stored_cards"`
	BankAccounts []BankAccountResponse `json:"bank_accounts"`
	UPIDetails   []UPIDetailResponse   `json:"upi_details"`
	CreditLines  []CreditLineResponse  `json:"credit_lines"`
	LastUpdated  string                `json:"last_updated"`
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
	AccountID      string `json:"account_id"`
	LastFiveDigits string `json:"last_five_digits"`
	BankName       string `json:"bank_name"`
	IFSCCode       string `json:"ifsc_code"`
	AccountType    string `json:"account_type"`
	IsActive       bool   `json:"is_active"`
	IsVerified     bool   `json:"is_verified"`
	Nickname       string `json:"nickname,omitempty"`
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
// Only includes information needed by frontend
type MakePaymentResponse struct {
	PaymentID   string `json:"payment_id"`            // For tracking payment status
	Status      string `json:"status"`                // pending, success, failed
	Amount      int64  `json:"amount"`                // Confirmation of amount in paise
	RedirectURL string `json:"redirect_url,omitempty"` // URL to redirect user for 3DS/OTP (CRITICAL)
	Message     string `json:"message"`               // User-facing message
}

// AddPaymentSourceResponse represents the response for adding a payment source
type AddPaymentSourceResponse struct {
	Success            bool                    `json:"success"`
	Message            string                  `json:"message"`
	SourceID           string                  `json:"source_id,omitempty"`
	SourceType         string                  `json:"source_type"`
	VerificationStatus string                  `json:"verification_status"`
	AccountDetails     *VerifiedAccountDetails `json:"account_details,omitempty"`
	UPIDetails         *VerifiedUPIDetails     `json:"upi_details,omitempty"`
}

// VerifiedAccountDetails represents verified bank account details in response
type VerifiedAccountDetails struct {
	MaskedAccountNumber string `json:"masked_account_number"`
	VerifiedName        string `json:"verified_name"`
	BankName            string `json:"bank_name"`
	BranchName          string `json:"branch_name,omitempty"`
	IFSCCode            string `json:"ifsc_code"`
	AccountType         string `json:"account_type"`
	IsActive            bool   `json:"is_active"`
}

// VerifiedUPIDetails represents verified UPI details in response
type VerifiedUPIDetails struct {
	UPIAddress string `json:"upi_address"`
	Provider   string `json:"provider"`
	IsActive   bool   `json:"is_active"`
}

// === COMMON RESPONSE MODELS ===

// StandardResponse represents a standard API response
type StandardResponse struct {
	Message  string `json:"message"`
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
	Data     any    `json:"data,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// === PHASE 10: PAYMENT SOURCE SELECTION MODELS ===

// GetPaymentSourcesWithFeesRequest represents the request for Phase 10
type GetPaymentSourcesWithFeesRequest struct {
	BusinessID     string `json:"business_id"`     // Business identifier
	PaymentContext string `json:"payment_context"` // vendor_payment, gst_payment, bill_payment, payroll
	BaseAmount     int64  `json:"base_amount"`     // Amount in paise
}

// GetPaymentSourcesWithFeesResponse represents the response for Phase 10
type GetPaymentSourcesWithFeesResponse struct {
	BusinessID     string                          `json:"business_id"`
	PaymentContext string                          `json:"payment_context"`
	BaseAmount     int64                           `json:"base_amount"`
	Sources        []PaymentSourceWithFeesResponse `json:"sources"`
}

// PaymentSourceWithFeesResponse represents a payment source with fee breakdown
type PaymentSourceWithFeesResponse struct {
	// Source identification
	SourceID      string `json:"source_id"`
	SourceType    string `json:"source_type"`
	PaymentMethod string `json:"payment_method"`
	DisplayName   string `json:"display_name"`

	// Card-specific fields
	CardType       *string `json:"card_type,omitempty"`
	CardCategory   *string `json:"card_category,omitempty"`
	LastFourDigits string  `json:"last_four_digits,omitempty"`

	// Bank account specific
	LastFiveDigits string `json:"last_five_digits,omitempty"`
	BankName       string `json:"bank_name,omitempty"`
	IFSCCode       string `json:"ifsc_code,omitempty"`

	// UPI specific
	UPIAddress string `json:"upi_address,omitempty"`

	// Restriction information
	IsAllowed         bool   `json:"is_allowed"`
	RestrictionReason string `json:"restriction_reason,omitempty"`

	// Fee breakdown (only for allowed sources)
	FeeBreakdown *FeeBreakdownResponse `json:"fee_breakdown,omitempty"`
}

// FeeBreakdownResponse represents fee calculation with GST
type FeeBreakdownResponse struct {
	BaseAmount            int64   `json:"base_amount"`
	ConvenienceFee        int64   `json:"convenience_fee"`
	ConvenienceFeeGST     int64   `json:"convenience_fee_gst"`
	TotalFee              int64   `json:"total_fee"`
	TotalAmount           int64   `json:"total_amount"`
	ConvenienceFeePercent float64 `json:"convenience_fee_percent"`
	GSTPercent            float64 `json:"gst_percent"`
}
