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
