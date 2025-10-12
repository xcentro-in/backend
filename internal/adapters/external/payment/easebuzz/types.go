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
	Status int `json:"status"` // 1 for success, 0 for failure
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
	CardCVV            string `json:"card_cvv,omitempty"`             // ENCRYPTED (required for RuPay)
	SubTransactionType string `json:"sub_transaction_type,omitempty"` // For guest checkout/Alt ID
	CardSignature      string `json:"card_signature,omitempty"`       // SHA-512 (for card-based, not token)
}

// TokenPaymentResponse represents the response from Easebuzz Token Payment API
type TokenPaymentResponse struct {
	Status int `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		EasepayID     string `json:"easepayid"`    // Easebuzz payment ID
		TransactionID string `json:"txnid"`        // Our transaction ID
		Amount        string `json:"amount"`       // Payment amount
		Status        string `json:"status"`       // Payment status
		Message       string `json:"message"`      // Status message
		RedirectURL   string `json:"redirect_url"` // 3DS/OTP redirect URL
		BankRefNum    string `json:"bank_ref_num,omitempty"`
		PaymentSource string `json:"payment_source,omitempty"`
		CardCategory  string `json:"card_category,omitempty"`
		Error         string `json:"error,omitempty"`
		ErrorMessage  string `json:"error_Message,omitempty"`
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
	PaymentStatusSuccess       PaymentStatus = "success"
	PaymentStatusFailure       PaymentStatus = "failure"
	PaymentStatusPending       PaymentStatus = "pending"
	PaymentStatusDropped       PaymentStatus = "dropped"
	PaymentStatusUserCancelled PaymentStatus = "userCancelled"
)

// TokenizationRequest represents the pre-request script parameters for tokenization
// This entire payload will be encrypted and sent as "enc_data"
type TokenizationRequest struct {
	CardNum          string `json:"card_num"`                 // Card number
	ExpiryMonth      string `json:"expirymonth"`              // Expiry month (MM)
	ExpiryYear       string `json:"expiryyear"`               // Expiry year (YYYY)
	CVV              string `json:"cvv"`                      // CVV code
	NameOnCard       string `json:"name_on_card"`             // Name on card
	CardType         string `json:"card_type"`                // "dc" (debit) or "cc" (credit)
	CardBrand        string `json:"card_brand"`               // "Visa", "Mastercard", etc.
	Name             string `json:"name"`                     // Customer name
	Email            string `json:"email"`                    // Customer email
	Mobile           string `json:"mobile"`                   // Customer mobile
	CustomerUniqueID string `json:"customer_unique_id"`       // Unique customer ID
	MerchantTokenID  string `json:"merchant_token_id"`        // Token ID for merchant
	TID              string `json:"tid,omitempty"`            // Transaction ID (optional)
	MerchantID       string `json:"merchant_id,omitempty"`    // Merchant ID (optional)
	SubMerchantID    string `json:"submerchant_id,omitempty"` // Sub-merchant ID (optional)
	MerchantKey      string `json:"merchant_key"`             // Merchant key
	Salt             string `json:"salt"`                     // Salt
	SKey             string `json:"s_key"`                    // Encryption key
	SIV              string `json:"s_iv"`                     // Encryption IV
	Amount           string `json:"amount"`                   // Transaction amount
}

// TokenizationAPIRequest represents the actual API request body
// Only contains encrypted data
type TokenizationAPIRequest struct {
	EncData string `json:"enc_data"` // Encrypted payload (base64)
}

// TokenizationResponse represents the response from Easebuzz tokenization API
type TokenizationResponse struct {
	Status int `json:"status"` // 1 for success, 0 for failure
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
	MerchantTokenID string `json:"merchant_token_id"` // Merchant's token ID
	EasebuzzTokenID string `json:"easebuzz_token_id"` // Easebuzz token (from tokenization)
}

// CryptogramAPIRequest represents the actual API request body
// Only contains encrypted data
type CryptogramAPIRequest struct {
	EncData string `json:"enc_data"` // Encrypted payload (base64)
}

// CryptogramResponse represents the response from Easebuzz cryptogram API
type CryptogramResponse struct {
	Status int `json:"status"` // 1 for success, 0 for failure
	Data   struct {
		Cryptogram string `json:"cryptogram"` // Generated cryptogram
		ExpiresAt  string `json:"expires_at"` // Expiry timestamp
		TokenID    string `json:"token_id"`   // Token identifier
		Message    string `json:"message"`    // Response message
	} `json:"data"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// CallbackPayload represents the payment callback response (SURL/FURL) and webhook notification
// Easebuzz POSTs this structure to the callback URL after payment completion
// This struct covers all fields from both SURL responses and webhook notifications
type CallbackPayload struct {
	// ============================================================================
	// CORE FIELDS (Required for hash validation)
	// ============================================================================
	TxnID       string `json:"txnid"`       // Transaction ID (our reference)
	Status      string `json:"status"`      // Payment status: "success", "failure", "pending", "dropped", "userCancelled"
	Amount      string `json:"amount"`      // Transaction amount in rupees (e.g., "500.00")
	ProductInfo string `json:"productinfo"` // Product/payment description
	FirstName   string `json:"firstname"`   // Customer first name
	Email       string `json:"email"`       // Customer email
	Hash        string `json:"hash"`        // SHA-512 hash for verification
	Key         string `json:"key"`         // Merchant key (for reverse hash validation)

	// ============================================================================
	// USER-DEFINED FIELDS (For custom data passing)
	// ============================================================================
	UDF1  string `json:"udf1"`  // User-defined field 1
	UDF2  string `json:"udf2"`  // User-defined field 2
	UDF3  string `json:"udf3"`  // User-defined field 3
	UDF4  string `json:"udf4"`  // User-defined field 4
	UDF5  string `json:"udf5"`  // User-defined field 5
	UDF6  string `json:"udf6"`  // User-defined field 6
	UDF7  string `json:"udf7"`  // User-defined field 7
	UDF8  string `json:"udf8"`  // User-defined field 8
	UDF9  string `json:"udf9"`  // User-defined field 9
	UDF10 string `json:"udf10"` // User-defined field 10

	// ============================================================================
	// PAYMENT IDENTIFIERS
	// ============================================================================
	EasepayID string `json:"easepayid"` // Easebuzz payment ID (e.g., "E2501789CVRAAA")

	// ============================================================================
	// CUSTOMER DETAILS
	// ============================================================================
	Phone string `json:"phone"` // Customer phone number

	// ============================================================================
	// PAYMENT METHOD DETAILS
	// ============================================================================
	Mode          string `json:"mode"`           // Payment mode: "CC", "DC", "UPI", "NB", etc.
	CardType      string `json:"card_type"`      // Card type: "Credit Card", "Debit Card", "NA"
	CardNum       string `json:"cardnum"`        // Masked card number (e.g., "XXXXXXXXXXXX1111")
	CardCategory  string `json:"cardCategory"`   // Card category (e.g., "NA")
	NameOnCard    string `json:"name_on_card"`   // Name on card (e.g., "NA" if not applicable)
	BankName      string `json:"bank_name"`      // Bank name (e.g., "HDFC", "NA")
	IssuingBank   string `json:"issuing_bank"`   // Issuing bank (e.g., "NA")
	BankCode      string `json:"bankcode"`       // Bank code (e.g., "NA")
	UPIVA         string `json:"upi_va"`         // UPI virtual address (e.g., "john@okicici", "NA")
	PaymentSource string `json:"payment_source"` // Payment source: "Easebuzz", etc.

	// ============================================================================
	// TRANSACTION REFERENCES
	// ============================================================================
	BankRefNum          string `json:"bank_ref_num"`          // Bank reference number
	AuthCode            string `json:"auth_code"`             // Authorization code
	AuthRefNum          string `json:"auth_ref_num"`          // Auth reference number (e.g., "NA")
	AuthReferenceNumber string `json:"auth_reference_number"` // Alternative auth ref field
	ResponseCode        string `json:"response_code"`         // Gateway response code (e.g., "00")

	// ============================================================================
	// FINANCIAL DETAILS
	// ============================================================================
	NetAmountDebit      string `json:"net_amount_debit"`     // Net amount debited from customer
	DeductionPercentage string `json:"deduction_percentage"` // Fee deduction percentage
	CashBackPercentage  string `json:"cash_back_percentage"` // Cashback percentage (if applicable)
	ServiceTax          string `json:"service_tax"`          // Service tax amount
	ServiceCharge       string `json:"service_charge"`       // Service charge amount
	SettlementAmount    string `json:"settlement_amount"`    // Settlement amount (webhook)
	DiscountAmount      string `json:"discount_amount"`      // Discount amount (webhook)
	DiscountCode        string `json:"discount_code"`        // Discount code applied (webhook)

	// ============================================================================
	// ERROR HANDLING
	// ============================================================================
	Error              string `json:"error"`               // Error message (e.g., "Transaction is successful.")
	ErrorMessage       string `json:"error_Message"`       // Detailed error message
	ErrorCode          string `json:"error_code"`          // Error code (e.g., "NA")
	CancellationReason string `json:"cancellation_reason"` // Cancellation reason (webhook)

	// ============================================================================
	// CALLBACK URLS
	// ============================================================================
	SURL string `json:"surl"` // Success callback URL
	FURL string `json:"furl"` // Failure callback URL

	// ============================================================================
	// METADATA
	// ============================================================================
	AddedOn         string `json:"addedon"`          // Transaction timestamp (e.g., "2025-01-30 06:02:37.000000")
	PaymentCategory string `json:"payment_category"` // Payment category (e.g., "DEFAULT")
	PGType          string `json:"PG_TYPE"`          // Payment gateway type (e.g., "NA")
	MerchantLogo    string `json:"merchant_logo"`    // Merchant logo URL (e.g., "NA")
	UnmappedStatus  string `json:"unmappedstatus"`   // Unmapped status (e.g., "NA")
}
