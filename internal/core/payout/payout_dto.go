package payout

import "time"

// ============================================================================
// Ops Dashboard DTOs
// ============================================================================

// OpsPayoutDetailResponse represents detailed payout information for ops dashboard
type OpsPayoutDetailResponse struct {
	// Payout Overview
	TransferRequestID string             `json:"transfer_request_id"`
	Status            TransferStatusType `json:"status"`
	Amount            int64              `json:"amount"`
	Currency          string             `json:"currency"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         *time.Time         `json:"updated_at,omitempty"`

	// Status-specific timestamps
	InitiatedAt  *time.Time `json:"initiated_at,omitempty"`
	ProcessingAt *time.Time `json:"processing_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	FailedAt     *time.Time `json:"failed_at,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`

	// UTR (for completed payouts)
	UTR string `json:"utr,omitempty"`

	// Voucher details
	Voucher OpsPayoutVoucherDetail `json:"voucher"`

	// Payee/Beneficiary details
	Payee OpsPayoutPayeeDetail `json:"payee"`

	// Linked collection/payment
	Collection OpsPayoutCollectionDetail `json:"collection"`

	// Provider details
	Provider OpsPayoutProviderDetail `json:"provider"`

	// Retry history (all attempts for this voucher)
	RetryHistory OpsPayoutRetryHistory `json:"retry_history"`
}

// OpsPayoutVoucherDetail represents voucher information for payout
type OpsPayoutVoucherDetail struct {
	VoucherID     string    `json:"voucher_id"`
	VoucherNumber string    `json:"voucher_number"`
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`
}

// OpsPayoutPayeeDetail represents payee/beneficiary information for payout
type OpsPayoutPayeeDetail struct {
	PayeeID     string `json:"payee_id"`
	PayeeName   string `json:"payee_name"`
	PayeeType   string `json:"payee_type"` // vendor, employee, biller
	RelationID  string `json:"relation_id"` // Composite: business_id:payee_id:account_id

	// Bank account details
	BankName            string `json:"bank_name"`
	AccountNumberLast4  string `json:"account_number_last4"`
	IFSCCode            string `json:"ifsc_code"`
	AccountHolderName   string `json:"account_holder_name"`

	// GSTIN (for vendors)
	GSTIN string `json:"gstin,omitempty"`
}

// OpsPayoutCollectionDetail represents linked payment/collection information
type OpsPayoutCollectionDetail struct {
	PaymentID        string     `json:"payment_id"`
	PaymentStatus    string     `json:"payment_status"`
	PaymentAmount    int64      `json:"payment_amount"`
	PaymentMethod    string     `json:"payment_method"`
	SettlementStatus string     `json:"settlement_status"`
	SettledAt        *time.Time `json:"settled_at,omitempty"`

	// Business context
	BusinessID   string `json:"business_id"`
	BusinessName string `json:"business_name"`
}

// OpsPayoutProviderDetail represents provider-specific information
type OpsPayoutProviderDetail struct {
	Provider             ProviderType `json:"provider"`
	ProviderTransferID   string       `json:"provider_transfer_id,omitempty"`
	ProviderBeneficiaryID string      `json:"provider_beneficiary_id"`
	EscrowID             string       `json:"escrow_id,omitempty"`
	TransferMode         string       `json:"transfer_mode"` // NEFT, RTGS, IMPS
	Purpose              string       `json:"purpose"`
	Reference            string       `json:"reference"`
	Narration            string       `json:"narration"`
}

// OpsPayoutRetryHistory represents retry history for a voucher
type OpsPayoutRetryHistory struct {
	CurrentAttempt    OpsPayoutAttemptSummary   `json:"current_attempt"`
	TotalAttempts     int                       `json:"total_attempts"`
	MaxRetries        int                       `json:"max_retries"`
	CanRetry          bool                      `json:"can_retry"`
	PreviousAttempts  []OpsPayoutAttemptSummary `json:"previous_attempts"`
}

// OpsPayoutAttemptSummary represents a single payout attempt
type OpsPayoutAttemptSummary struct {
	TransferRequestID string             `json:"transfer_request_id"`
	AttemptNumber     int                `json:"attempt_number"` // 1, 2, 3, etc.
	Status            TransferStatusType `json:"status"`
	CreatedAt         time.Time          `json:"created_at"`
	FailedAt          *time.Time         `json:"failed_at,omitempty"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
	FailureReason     string             `json:"failure_reason,omitempty"`
	IsDeleted         bool               `json:"is_deleted"`
	DeletedReason     string             `json:"deleted_reason,omitempty"`
}
