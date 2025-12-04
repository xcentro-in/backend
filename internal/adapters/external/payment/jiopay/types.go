package jiopay

// ============================================================================
// TOKENIZATION API TYPES
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/fetch-card
// ============================================================================

// TokenizationRequest represents the card tokenization request structure
// Contains 5 mandatory top-level objects: user, entity, card, consent, audit
type TokenizationRequest struct {
	User    UserObject    `json:"user"`
	Entity  EntityObject  `json:"entity"`
	Card    CardObject    `json:"card"`
	Consent ConsentObject `json:"consent"`
	Audit   AuditObject   `json:"audit"`
}

// UserObject represents user information in the request
// email, name, and mobile are optional fields
type UserObject struct {
	ExternalID string        `json:"externalId"` // Mandatory: Customer unique ID
	Email      string        `json:"email,omitempty"`
	Name       string        `json:"name,omitempty"`
	Mobile     *MobileObject `json:"mobile,omitempty"` // Optional nested object
}

// MobileObject represents mobile phone details
// Both fields are mandatory if this object is included
type MobileObject struct {
	CountryCode int   `json:"countryCode"` // Mandatory: e.g., 91 for India
	Number      int64 `json:"number"`      // Mandatory: 10-digit number
}

// EntityObject represents merchant/entity information
type EntityObject struct {
	EntityID string `json:"entityId"` // Mandatory: Merchant identifier
}

// CardObject represents card details for tokenization
type CardObject struct {
	Number                    string `json:"number"`                                // Mandatory: Card number
	ExpiryMm                  string `json:"expiryMm"`                              // Mandatory: MM format
	ExpiryYy                  string `json:"expiryYy"`                              // Mandatory: YYYY format
	Name                      string `json:"name"`                                  // Mandatory: Name on card
	AuthenticationReferenceID string `json:"Authentication Reference Id,omitempty"` // Conditional: Required for RuPay
	Remark                    string `json:"remark,omitempty"`                      // Optional: Additional remarks
}

// ConsentObject represents user consent information
type ConsentObject struct {
	ID        string `json:"id"`        // Mandatory: Consent identifier
	Timestamp string `json:"timestamp"` // Mandatory: ISO 8601 format (e.g., "2019-07-25T15:54:07.011Z")
}

// AuditObject represents audit/authorization information
type AuditObject struct {
	Authorized        bool   `json:"authorized"`        // Mandatory: Authorization status
	AuthorizationCode string `json:"authorizationCode"` // Mandatory: Authorization code
}

// TokenizationResponse represents the NEW response from JioPay /tokenized-card API (as of 2025)
// Response structure has changed to include vaultId immediately
// Outer structure with headers, body, statusCode
type TokenizationResponse struct {
	Headers         map[string]any       `json:"headers"`         // Response headers
	Body            TokenizationBodyData `json:"body"`            // Actual response body
	StatusCode      string               `json:"statusCode"`      // HTTP status code text (e.g., "OK")
	StatusCodeValue int                  `json:"statusCodeValue"` // HTTP status code number (e.g., 200)
}

// TokenizationBodyData represents the body content of tokenization response
// Contains status, vaultId, and tokenReference
type TokenizationBodyData struct {
	Status         StatusObject `json:"status"`         // API status
	VaultID        int64        `json:"vaultId"`        // CRITICAL: Vault ID (needed for cryptogram) - now available immediately!
	TokenReference string       `json:"tokenReference"` // Token reference (for payments)
}

// ============================================================================
// FETCH CARD API TYPES
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/fetch-card
// Used to retrieve all cards for a customer (to match and get vaultId)
// ============================================================================

// FetchCardRequest represents the request to fetch all cards for a customer
type FetchCardRequest struct {
	Entity EntityObject `json:"entity"` // Mandatory: Entity (merchant) details
	User   UserObject   `json:"user"`   // Mandatory: User details (with externalId)
}

// FetchCardResponse represents the response from fetch-card API
type FetchCardResponse struct {
	Status StatusObject `json:"status"` // API status
	Card   []CardInfo   `json:"card"`   // List of cards for this customer
	// Other payment instruments (not needed for our use case)
	FoodCard          []interface{} `json:"foodCard,omitempty"`
	UPIVPA            []interface{} `json:"upiVpa,omitempty"`
	BankAccount       []interface{} `json:"bankAccount,omitempty"`
	PrePaidInstrument []interface{} `json:"prePaidInstrument,omitempty"`
	PreferredBank     []interface{} `json:"preferredBank,omitempty"`
}

// CardInfo represents a single card in the fetch-card response
type CardInfo struct {
	Type               int    `json:"type"`               // Card type code
	ExpiryMm           string `json:"expiryMm"`           // Expiry month (may be masked)
	ExpiryYy           string `json:"expiryYy"`           // Expiry year (may be masked)
	Name               string `json:"name"`               // Cardholder name (may be masked)
	Scheme             int    `json:"scheme"`             // Card scheme code (104=Visa, etc.)
	BankID             string `json:"bankId"`             // Bank ID
	Remark             string `json:"remark"`             // Remark (often contains customer ID)
	VaultID            string `json:"vaultId"`            // CRITICAL: Vault ID (needed for cryptogram)
	MaskedNumber       string `json:"maskedNumber"`       // CRITICAL: Masked number (e.g., "XXXXXXXXXXXX0055")
	Product            string `json:"product"`            // Product type
	CountryCode        string `json:"countryCode"`        // Country code
	TokenReference     string `json:"tokenReference"`     // Token reference
	BankName           string `json:"bankName"`           // Bank name
	PAR                string `json:"par"`                // Payment Account Reference
	AuthorizationState string `json:"authorizationState"` // Authorization state
	CVVLess            int    `json:"cvvLess"`            // CVV-less transaction support (0/1)
	Secure             struct {
		TokenExpiry string `json:"tokenExpiry"` // Token expiry (MMYYYY)
	} `json:"secure"`
	Status          string `json:"status"`          // Card status (e.g., "ACTIVE")
	CardReference   string `json:"cardReference"`   // Card reference
	ProductType     string `json:"productType"`     // Product type (e.g., "Normal")
	IsExpired       bool   `json:"isExpired"`       // Expiry status
	IsDomestic      bool   `json:"isDomestic"`      // Domestic/international
}

// ============================================================================
// CRYPTOGRAM API TYPES
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/getCryptogram/{vaultId}
// ============================================================================

// CryptogramRequest represents the get cryptogram request structure
// Contains 3 mandatory top-level objects: user, entity, amount
type CryptogramRequest struct {
	User   UserObject   `json:"user"`
	Entity EntityObject `json:"entity"`
	Amount AmountObject `json:"amount"`
}

// AmountObject represents transaction amount details
type AmountObject struct {
	NetAmount float64 `json:"netAmount"` // Mandatory: Amount in rupees (e.g., 1500.75)
}

// CryptogramResponse represents the response from JioPay cryptogram API
type CryptogramResponse struct {
	Status StatusObject `json:"status"`
	Card   struct {
		Type               int    `json:"type"`
		ExpiryMm           string `json:"expiryMm"`
		ExpiryYy           string `json:"expiryYy"`
		Name               string `json:"name"`
		Scheme             int    `json:"scheme"`
		BankID             string `json:"bankId"`
		Remark             string `json:"remark"`
		VaultID            string `json:"vaultId"`
		MaskedNumber       string `json:"maskedNumber"`
		Product            string `json:"product"`
		CountryCode        string `json:"countryCode"`
		TokenReference     string `json:"tokenReference"`
		BankName           string `json:"bankName"`
		PAR                string `json:"par"`
		AuthorizationState string `json:"authorizationState"`
		CVVLess            int    `json:"cvvLess"`
		CardReference      string `json:"cardReference"`
		Consent            struct {
			ID        string `json:"id"`
			Timestamp string `json:"timestamp"`
		} `json:"consent"`
		ProductType string `json:"productType"`
		Status      string `json:"status"`
		IsExpired   bool   `json:"isExpired"`
		IsDomestic  bool   `json:"isDomestic"`
	} `json:"card"`
	Secure SecureObject `json:"secure"`
}

// SecureObject represents secure payment credentials
type SecureObject struct {
	Token       string `json:"token"`       // Network token (string representation of card number for payment)
	TokenExpiry string `json:"tokenExpiry"` // Token expiry (MMYYYY format)
	Cryptogram  string `json:"cryptogram"`  // Generated cryptogram
}

// StatusObject represents API response status
type StatusObject struct {
	Status string `json:"status"` // "SUCCESS", "FAILURE", etc.
}

// ============================================================================
// INITIATE SALE API TYPES
// API: POST https://uat.jiopay.co.in/pg/api/v2/initiateSale
// ============================================================================

// InitiateSaleRequest represents the payment initiation request
type InitiateSaleRequest struct {
	// Mandatory fields
	MerchantID      string `json:"merchantId"`      // Merchant account ID
	MerchantTxnNo   string `json:"merchantTxnNo"`   // Transaction reference number
	Amount          string `json:"amount"`          // Transaction amount (e.g., "100.00")
	CurrencyCode    string `json:"currencyCode"`    // Default "356" for INR
	PayType         string `json:"payType"`         // "0" = Hosted, "1" = Direct
	CustomerEmailID string `json:"customerEmailID"` // Customer email
	TransactionType string `json:"transactionType"` // "SALE" or "PREAUTH"
	ReturnURL       string `json:"returnURL"`       // Callback URL
	TxnDate         string `json:"txnDate"`         // YYYYMMDDHHMISS format

	// Payment method specific
	PaymentMode string `json:"paymentMode,omitempty"` // "CARD", "NB", "UPI", "WALLET"

	// Token card specific fields
	CardNo          string `json:"cardNo,omitempty"`          // Token number
	CardExpiry      string `json:"cardExpiry,omitempty"`      // YYYYMM format
	NameOnCard      string `json:"nameOnCard,omitempty"`      // Name on card
	CVV             string `json:"cvv,omitempty"`             // CVV (optional for tokenized)
	TokenCryptogram string `json:"tokenCryptogram,omitempty"` // Cryptogram from getCryptogram
	TokenRefID      string `json:"tokenRefId,omitempty"`      // Token reference ID

	// Optional customer details
	CustomerName     string `json:"customerName,omitempty"`     // Customer name
	CustomerMobileNo string `json:"customerMobileNo,omitempty"` // Customer mobile

	// Optional transaction details
	InvoiceNo string `json:"invoiceNo,omitempty"` // Invoice number
	OrderDesc string `json:"orderDesc,omitempty"` // Order description

	// Additional optional fields
	AggregatorID string `json:"aggregatorID,omitempty"` // Aggregator ID
	CustomerID   string `json:"customerID,omitempty"`   // Customer ID

	// Tokenization fields
	PaymentOptionCodes string `json:"paymentOptionCodes,omitempty"` // "CC" for credit, "DC" for debit

	// Security
	SecureHash string `json:"secureHash"` // SHA-256 hash
}

// InitiateSaleResponse represents the response from initiateSale API
type InitiateSaleResponse struct {
	ResponseCode        string  `json:"responseCode"`        // "R1000" for success
	ResponseDescription string  `json:"responseDescription"` // Error description
	MerchantID          string  `json:"merchantId"`          // Merchant ID
	AggregatorID        *string `json:"aggregatorID"`        // Aggregator ID (can be null)
	MerchantTxnNo       string  `json:"merchantTxnNo"`       // Transaction reference
	RedirectURI         string  `json:"redirectURI"`         // Redirect URL for 3DS/OTP
	ShowOTPCapturePage  string  `json:"showOTPCapturePage"`  // "Y" or "N"
	GenerateOTPURI      *string `json:"generateOTPURI"`      // OTP generation URI (can be null)
	VerifyOTPURI        *string `json:"verifyOTPURI"`        // OTP verification URI (can be null)
	AuthorizeURI        *string `json:"authorizeURI"`        // Authorization URI (can be null)
	TranCtx             string  `json:"tranCtx"`             // Transaction context
	SecureHash          string  `json:"secureHash"`          // Hash value
	OthCharge           float64 `json:"oth_charge"`          // OTH charge
}

// ============================================================================
// CALLBACK TYPES (SURL/FURL)
// JioPay sends this payload to callback URL after payment completion
// ============================================================================

// CallbackPayload represents the payment callback response (SURL/FURL)
// JioPay POSTs this structure to the ReturnURL after payment completion
//
// Status Determination:
//   - responseCode: "000"/"0000" = Success, "R1000" = Initiated (UPI/NACH), other = Failed
//   - txnStatus: "SUC" = Success, "FAIL" = Failure, "PEN" = Pending (assumption - validate with PG)
//   - txnResponseCode: Secondary validation (should match responseCode)
//
// Security: secureHash must be verified using same HMAC-SHA256 algorithm as payment initiation
type CallbackPayload struct {
	ResponseCode     string `json:"responseCode"`               // "000"/"0000" = success, "R1000" = initiated, other = failed
	RespDescription  string `json:"respDescription"`            // Response description/message
	MerchantID       string `json:"merchantId"`                 // Merchant identifier
	AggregatorID     string `json:"aggregatorID,omitempty"`     // Aggregator ID (optional)
	MerchantTxnNo    string `json:"merchantTxnNo"`              // Our transaction ID (use for payment lookup)
	SettlementStatus string `json:"settlementStatus,omitempty"` // Settlement status (e.g., "STD")
	TxnStatus        string `json:"txnStatus"`                  // "SUC", "FAIL", "PEN" (assumption)
	TxnResponseCode  string `json:"txnResponseCode"`            // Transaction response code
	TxnID            string `json:"txnID"`                      // JioPay transaction ID
	SettlementID     string `json:"settlementID,omitempty"`     // Settlement ID
	SettlementDate   string `json:"settlementDate,omitempty"`   // Settlement date (YYYYMMDD format)
	SecureHash       string `json:"secureHash"`                 // HMAC-SHA256 signature for verification
}

// ============================================================================
// SETTLEMENT WEBHOOK TYPES
// Sent by JioPay when settlement is completed (T+1 to T+3 days after payment)
// ============================================================================

// SettlementWebhook represents settlement notification from JioPay
// Sent when gateway completes final settlement to merchant account
//
// Key Characteristics:
//   - Arrival of this webhook = 100% settlement success (no status field)
//   - Contains final settlement amounts after gateway fees (MDR)
//   - merchantTxnNo matches our payment_id for lookup
//   - secureHash must be verified using HMAC-SHA256
//
// Financial Flow:
//   - txnAmount: Original payment amount (e.g., "1000.00")
//   - txnCharges: Base MDR fee (e.g., "15.00")
//   - serviceTax: GST on MDR (e.g., "2.70")
//   - settledAmount: Net amount received = txnAmount - txnCharges - serviceTax
//
// Security: secureHash validation is mandatory using SecureHashGenerator
type SettlementWebhook struct {
	AggregatorID          string `json:"aggregatorID"`          // Aggregator identifier
	MerchantID            string `json:"merchantId"`            // Merchant identifier
	MerchantTxnNo         string `json:"merchantTxnNo"`         // KEY: Our payment_id for lookup
	InvoiceNos            string `json:"invoiceNos"`            // Comma-separated invoice numbers
	PaymentMode           string `json:"paymentMode"`           // "CARD", "NB", "UPI", "WALLET"
	PaymentSubInstType    string `json:"paymentSubInstType"`    // "CC" = Credit Card, "DC" = Debit Card
	TxnChannel            string `json:"txnChannel"`            // "WEB", "POS" (only for CARD payments)
	TxnAmount             string `json:"txnAmount"`             // Original payment amount in rupees (e.g., "1000.00")
	TxnCharges            string `json:"txnCharges"`            // Base MDR charges in rupees (e.g., "15.00")
	ServiceTax            string `json:"serviceTax"`            // GST on MDR in rupees (e.g., "2.70")
	SettlementID          string `json:"settlementID"`          // Settlement batch ID (e.g., "A00121-20171101-010")
	SettlementDate        string `json:"settlementDate"`        // Settlement date YYYYMMDD format (e.g., "20171101")
	SettlementAccount     string `json:"settlementAccount"`     // Bank account number where funds settled
	SettlementAccountIFSC string `json:"settlementAccountIFSC"` // IFSC code of settlement account
	SettledAmount         string `json:"settledAmount"`         // Net amount received in rupees (e.g., "982.30")
	CardNetwork           string `json:"cardNetwork"`           // "VISA", "MASTERCARD", etc. (only for card payments)
	AddlParam1            string `json:"addlParam1"`            // Additional settlement information
	SecureHash            string `json:"secureHash"`            // HMAC-SHA256 signature for verification
}
