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

// TokenizationResponse represents the response from JioPay tokenization API
type TokenizationResponse struct {
	Status StatusObject `json:"status"`
	Card   struct {
		Type           int    `json:"type"`           // Card type code
		ExpiryMm       string `json:"expiryMm"`       // Expiry month
		ExpiryYy       string `json:"expiryYy"`       // Expiry year
		Name           string `json:"name"`           // Cardholder name
		Scheme         int    `json:"scheme"`         // Card scheme code
		Remark         string `json:"remark"`         // Response remark
		VaultID        string `json:"vaultId"`        // Vault ID (use as CardID)
		MaskedNumber   string `json:"maskedNumber"`   // Masked card number
		PAR            string `json:"par"`            // Payment Account Reference
		TokenReference string `json:"tokenReference"` // Token reference
		BankName       string `json:"bankName"`       // Issuing bank name
		Status         string `json:"status"`         // Card status (e.g., "ACTIVE")
		IsExpired      bool   `json:"isExpired"`      // Expiry status
		IsDomestic     bool   `json:"isDomestic"`     // Domestic/international
		CVVLess        int    `json:"cvvLess"`        // CVV-less transaction support
		Consent        struct {
			ID        string `json:"id"`        // Consent ID
			Timestamp string `json:"timestamp"` // Consent timestamp
		} `json:"consent"`
	} `json:"card"`
	Secure *SecureObject `json:"secure,omitempty"` // May be present in some responses
}

// ============================================================================
// CRYPTOGRAM API TYPES
// API: POST https://uat.jiopay.co.in/qatsp/api/vault/getCryptogram
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
		Type           int    `json:"type"`
		ExpiryMm       string `json:"expiryMm"`
		ExpiryYy       string `json:"expiryYy"`
		Name           string `json:"name"`
		Scheme         int    `json:"scheme"`
		Remark         string `json:"remark"`
		VaultID        string `json:"vaultId"`
		MaskedNumber   string `json:"maskedNumber"`
		PAR            string `json:"par"`
		TokenReference string `json:"tokenReference"`
		BankName       string `json:"bankName"`
		Status         string `json:"status"`
		IsExpired      bool   `json:"isExpired"`
		IsDomestic     bool   `json:"isDomestic"`
		CVVLess        int    `json:"cvvLess"`
		Consent        struct {
			ID        string `json:"id"`
			Timestamp string `json:"timestamp"`
		} `json:"consent"`
	} `json:"card"`
	Secure SecureObject `json:"secure"`
}

// SecureObject represents secure payment credentials
type SecureObject struct {
	Token       int64  `json:"token"`       // Network token (numeric)
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
