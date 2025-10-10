package easebuzz

// InitiatePaymentRequest represents the request for Easebuzz Initiate Payment API
// This is Step 1 of the Easebuzz payment flow to get access_key
type InitiatePaymentRequest struct {
	Key         string `json:"key"`          // Merchant key
	TxnID       string `json:"txnid"`        // Transaction ID
	Amount      string `json:"amount"`       // Amount in rupees (e.g., "500.00")
	ProductInfo string `json:"productinfo"`  // Product description
	FirstName   string `json:"firstname"`    // Customer first name
	Phone       string `json:"phone"`        // Customer phone
	Email       string `json:"email"`        // Customer email
	SURL        string `json:"surl"`         // Success callback URL
	FURL        string `json:"furl"`         // Failure callback URL
	Hash        string `json:"hash"`         // SHA-512 hash for security
	RequestFlow string `json:"request_flow"` // Must be "SEAMLESS" for token payments

	// Optional UDF fields (User Defined Fields)
	UDF1 string `json:"udf1,omitempty"`
	UDF2 string `json:"udf2,omitempty"`
	UDF3 string `json:"udf3,omitempty"`
	UDF4 string `json:"udf4,omitempty"`
	UDF5 string `json:"udf5,omitempty"`
	// UDF8-10 must be empty per Easebuzz requirements
}

// InitiatePaymentResponse represents the response from Easebuzz Initiate Payment API
type InitiatePaymentResponse struct {
	Status int    `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		AccessKey string `json:"access_key"` // Access key for step 2
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// TokenPaymentRequest represents the request for Easebuzz Token-based Payment API
// This is Step 2 of the Easebuzz payment flow using encrypted card details
type TokenPaymentRequest struct {
	AccessKey        string `json:"access_key"`         // From step 1
	PaymentMode      string `json:"payment_mode"`       // "CC" or "DC"
	CardToken        string `json:"card_token"`         // ENCRYPTED token
	Cryptogram       string `json:"cryptogram"`         // ENCRYPTED cryptogram
	TokenExpiryDate  string `json:"token_expiry_date"`  // ENCRYPTED expiry (MM/YYYY)
	TokenRequesterID string `json:"token_requester_id"` // TSP identifier (TRID)
	CardLast4Digits  string `json:"card_last_4_digits"` // Last 4 digits (plaintext)

	// Optional/conditional fields
	CardCVV            string `json:"card_cvv,omitempty"`              // ENCRYPTED (required for RuPay)
	SubTransactionType string `json:"sub_transaction_type,omitempty"` // For guest checkout/Alt ID
	CardSignature      string `json:"card_signature,omitempty"`       // SHA-512 (for card-based, not token)
}

// TokenPaymentResponse represents the response from Easebuzz Token Payment API
type TokenPaymentResponse struct {
	Status int    `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		EasepayID       string `json:"easepayid"`        // Easebuzz payment ID
		TransactionID   string `json:"txnid"`            // Our transaction ID
		Amount          string `json:"amount"`           // Payment amount
		Status          string `json:"status"`           // Payment status
		Message         string `json:"message"`          // Status message
		RedirectURL     string `json:"redirect_url"`     // 3DS/OTP redirect URL
		BankRefNum      string `json:"bank_ref_num,omitempty"`
		PaymentSource   string `json:"payment_source,omitempty"`
		CardCategory    string `json:"card_category,omitempty"`
		Error           string `json:"error,omitempty"`
		ErrorMessage    string `json:"error_Message,omitempty"`
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// WebhookPayload represents the Easebuzz webhook notification
type WebhookPayload struct {
	TxnID         string `json:"txnid"`
	EasepayID     string `json:"easepayid"`
	Status        string `json:"status"` // success, failure, pending
	Amount        string `json:"amount"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	ProductInfo   string `json:"productinfo"`
	FirstName     string `json:"firstname"`
	PaymentSource string `json:"payment_source"`
	BankRefNum    string `json:"bank_ref_num"`
	Hash          string `json:"hash"` // For webhook verification
	ErrorMessage  string `json:"error_Message,omitempty"`

	// Timestamp fields
	AddedOn string `json:"addedon"` // Format: "YYYY-MM-DD HH:MM:SS"
}

// CardPaymentMode represents the payment mode for card payments
type CardPaymentMode string

const (
	CardPaymentModeCredit CardPaymentMode = "CC" // Credit card
	CardPaymentModeDebit  CardPaymentMode = "DC" // Debit card
)

// PaymentStatus represents Easebuzz payment status values
type PaymentStatus string

const (
	PaymentStatusSuccess PaymentStatus = "success"
	PaymentStatusFailure PaymentStatus = "failure"
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusDropped PaymentStatus = "dropped"
	PaymentStatusUserCancelled PaymentStatus = "userCancelled"
)

// TokenizationRequest represents the pre-request script parameters for tokenization
// This entire payload will be encrypted and sent as "enc_data"
type TokenizationRequest struct {
	CardNum            string `json:"card_num"`             // Card number
	ExpiryMonth        string `json:"expirymonth"`          // Expiry month (MM)
	ExpiryYear         string `json:"expiryyear"`           // Expiry year (YYYY)
	CVV                string `json:"cvv"`                  // CVV code
	NameOnCard         string `json:"name_on_card"`         // Name on card
	CardType           string `json:"card_type"`            // "dc" (debit) or "cc" (credit)
	CardBrand          string `json:"card_brand"`           // "Visa", "Mastercard", etc.
	Name               string `json:"name"`                 // Customer name
	Email              string `json:"email"`                // Customer email
	Mobile             string `json:"mobile"`               // Customer mobile
	CustomerUniqueID   string `json:"customer_unique_id"`   // Unique customer ID
	MerchantTokenID    string `json:"merchant_token_id"`    // Token ID for merchant
	TID                string `json:"tid,omitempty"`        // Transaction ID (optional)
	MerchantID         string `json:"merchant_id,omitempty"` // Merchant ID (optional)
	SubMerchantID      string `json:"submerchant_id,omitempty"` // Sub-merchant ID (optional)
	MerchantKey        string `json:"merchant_key"`         // Merchant key
	Salt               string `json:"salt"`                 // Salt
	SKey               string `json:"s_key"`                // Encryption key
	SIV                string `json:"s_iv"`                 // Encryption IV
	Amount             string `json:"amount"`               // Transaction amount
}

// TokenizationAPIRequest represents the actual API request body
// Only contains encrypted data
type TokenizationAPIRequest struct {
	EncData string `json:"enc_data"` // Encrypted payload (base64)
}

// TokenizationResponse represents the response from Easebuzz tokenization API
type TokenizationResponse struct {
	Status int    `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		Token       string `json:"token"`        // Generated token
		CardID      string `json:"card_id"`      // Card identifier
		Message     string `json:"message"`      // Response message
		LastFour    string `json:"last_four"`    // Last 4 digits
		CardBrand   string `json:"card_brand"`   // Card brand
		CardType    string `json:"card_type"`    // Card type
		ExpiryMonth string `json:"expiry_month"` // Expiry month
		ExpiryYear  string `json:"expiry_year"`  // Expiry year
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// CryptogramRequest represents the pre-encrypted parameters for cryptogram generation
// This entire payload will be encrypted and sent as "enc_data"
type CryptogramRequest struct {
	MerchantTokenID  string `json:"merchant_token_id"`  // Merchant's token ID
	EasebuzzTokenID  string `json:"easebuzz_token_id"`  // Easebuzz token (from tokenization)
}

// CryptogramAPIRequest represents the actual API request body
// Only contains encrypted data
type CryptogramAPIRequest struct {
	EncData string `json:"enc_data"` // Encrypted payload (base64)
}

// CryptogramResponse represents the response from Easebuzz cryptogram API
type CryptogramResponse struct {
	Status int    `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		Cryptogram  string `json:"cryptogram"`   // Generated cryptogram
		ExpiresAt   string `json:"expires_at"`   // Expiry timestamp
		TokenID     string `json:"token_id"`     // Token identifier
		Message     string `json:"message"`      // Response message
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}
