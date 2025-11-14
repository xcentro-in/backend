package castler

// CastlerBeneficiaryRequest represents Castler's beneficiary registration request
type CastlerBeneficiaryRequest struct {
	AccountNumber     string `json:"account_number"`
	IFSCCode          string `json:"ifsc_code"`
	AccountHolderName string `json:"account_holder_name"`
	BankName          string `json:"bank_name"`
	AccountType       string `json:"account_type"` // savings, current
	Email             string `json:"email,omitempty"`
	Phone             string `json:"phone,omitempty"`
	GSTIN             string `json:"gstin,omitempty"`
	PAN               string `json:"pan,omitempty"`
}

// CastlerBeneficiaryResponse represents Castler's beneficiary registration response
type CastlerBeneficiaryResponse struct {
	BeneficiaryID      string `json:"beneficiary_id"`
	AccountID          string `json:"account_id"`
	Status             string `json:"status"` // active, pending, failed
	VerificationStatus string `json:"verification_status"`
	VerifiedAt         string `json:"verified_at"`
	Message            string `json:"message"`
}

// CastlerTransferRequest represents Castler's transfer request
type CastlerTransferRequest struct {
	BeneficiaryID string  `json:"beneficiary_id"`
	Amount        float64 `json:"amount"` // In rupees (Castler uses rupees, not paise)
	Currency      string  `json:"currency"`
	Purpose       string  `json:"purpose"`
	Reference     string  `json:"reference"`
	Narration     string  `json:"narration"`
	TransferMode  string  `json:"transfer_mode,omitempty"` // IMPS, NEFT, RTGS
}

// CastlerTransferResponse represents Castler's transfer response
type CastlerTransferResponse struct {
	TransferID          string `json:"transfer_id"`
	Status              string `json:"status"` // pending, processing, completed, failed
	Message             string `json:"message"`
	UTR                 string `json:"utr,omitempty"`
	FailureReason       string `json:"failure_reason,omitempty"`
	EstimatedSettlement string `json:"estimated_settlement,omitempty"` // ISO timestamp
	ProcessedAt         string `json:"processed_at,omitempty"`         // ISO timestamp
	SettledAt           string `json:"settled_at,omitempty"`           // ISO timestamp
}

// CastlerApprovalRequest represents Castler's transfer approval request
type CastlerApprovalRequest struct {
	Remarks string `json:"remarks,omitempty"`
}

// CastlerWebhookPayload represents Castler's webhook notification
type CastlerWebhookPayload struct {
	Event      string                 `json:"event"` // transfer.completed, transfer.failed
	TransferID string                 `json:"transfer_id"`
	Status     string                 `json:"status"`
	UTR        string                 `json:"utr,omitempty"`
	SettledAt  string                 `json:"settled_at,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// CastlerPennyDropRequest represents the request for penny-drop bank verification
// API: POST /api/v1/payee/validation
type CastlerPennyDropRequest struct {
	Payees []CastlerPayeeValidation `json:"payees"`
}

// CastlerPayeeValidation represents individual payee for validation
type CastlerPayeeValidation struct {
	AccountHolder string `json:"accountHolder"`
	Email         string `json:"email"`
	Mobile        string `json:"mobile"`
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	BankName      string `json:"bankName"`
	BranchName    string `json:"branchName"`
}

// CastlerPennyDropResponse represents the response from penny-drop API
type CastlerPennyDropResponse struct {
	Result        string   `json:"result"` // Validation ID
	Message       *string  `json:"message"`
	Errors        []string `json:"errors"`
	Success       bool     `json:"success"`
	TotalElements int      `json:"totalElements"`
}

// CastlerPayeeOnboardRequest represents the request for payee onboarding
// API: POST /api/v1/payee
type CastlerPayeeOnboardRequest struct {
	AccountHolder string `json:"accountHolder"`
	Email         string `json:"email"`
	Mobile        int64  `json:"mobile"` // Note: Castler expects integer, not string
	AccountNumber string `json:"accountNumber"`
	IFSC          string `json:"ifsc"`
	BankName      string `json:"bankName"`
}

// CastlerPayeeOnboardResponse represents the response from payee onboarding API
type CastlerPayeeOnboardResponse struct {
	Result        string   `json:"result"` // Payee ID
	Message       *string  `json:"message"`
	Errors        []string `json:"errors"`
	Success       bool     `json:"success"`
	TotalElements int      `json:"totalElements"`
}
