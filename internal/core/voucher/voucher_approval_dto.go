package voucher

import "time"

// ApprovalAction represents the approval action states
type ApprovalAction string

const (
	ApprovalActionPending        ApprovalAction = "pending"
	ApprovalActionApproved       ApprovalAction = "approved"
	ApprovalActionRejected       ApprovalAction = "rejected"
	ApprovalActionRequestChanges ApprovalAction = "request_changes"
	ApprovalActionEdited         ApprovalAction = "edited"
)

// VoucherApprovalRequest represents the request to create/update an approval
type VoucherApprovalRequest struct {
	VoucherID       string         `json:"voucher_id" validate:"required,uuid"`
	BusinessID      string         `json:"-"` // Set from header, not from request body
	RequesterUserID string         `json:"-"` // Set from JWT context, not from request body
	ApproverUserID  string         `json:"approver_user_id" validate:"required,uuid"` // Who should approve
	Action          ApprovalAction `json:"action" validate:"required,oneof=pending approved rejected request_changes edited"`
	Remarks         string         `json:"remarks"` // Optional remarks
}

// VoucherApprovalResponse represents the response after approval action
type VoucherApprovalResponse struct {
	Success               bool           `json:"success"`
	Message               string         `json:"message"`
	ApprovalID            string         `json:"approval_id,omitempty"`
	VoucherID             string         `json:"voucher_id"`
	Action                ApprovalAction `json:"action"`
	CreatedAt             time.Time      `json:"created_at"` // When this action was taken
	PreviousApprovalState ApprovalAction `json:"previous_approval_state,omitempty"` // State before this change (empty if fresh)
	PreviousApproverID    string         `json:"previous_approver_id,omitempty"`    // User who was responsible for previous state (empty if fresh)
}

// GetApprovalsByVoucherResponse represents list of approvals for a voucher
type GetApprovalsByVoucherResponse struct {
	Approvals []ApprovalHistoryItem `json:"approvals"`
}

// ApprovalHistoryItem represents a single approval record
type ApprovalHistoryItem struct {
	ApprovalID      string         `json:"approval_id"`
	RequesterUserID string         `json:"requester_user_id"`
	RequesterName   string         `json:"requester_name"` // Fetched from user table
	ApproverUserID  string         `json:"approver_user_id"`
	ApproverName    string         `json:"approver_name"` // Fetched from user table
	Action          ApprovalAction `json:"action"`
	Remarks         string         `json:"remarks"`
	CreatedAt       time.Time      `json:"created_at"` // When this action was taken
}

// VoucherApprovalInboxRequest represents request for approval inbox listing
type VoucherApprovalInboxRequest struct {
	BusinessID string         `json:"business_id"` // From header
	Status     ApprovalAction `json:"status" validate:"required,oneof=pending approved rejected request_changes edited"`
}

// VoucherApprovalInboxItem represents a single voucher in the approval inbox
type VoucherApprovalInboxItem struct {
	VoucherID              string                `json:"voucher_id"`
	VoucherNumber          string                `json:"voucher_number"`
	InvoiceNumber          string                `json:"invoice_number"`
	InvoiceDueDate         *time.Time            `json:"invoice_due_date"`
	PayeeName              string                `json:"payee_name"`
	TotalAmount            int64                 `json:"total_amount"` // In paise
	VoucherStatus          string                `json:"voucher_status"`
	CurrentApprovalStatus  ApprovalAction        `json:"current_approval_status"` // Latest approval action
	ApprovalHistory        []ApprovalHistoryItem `json:"approval_history"`        // Complete chronological history
}

// VoucherApprovalInboxResponse represents the approval inbox response
type VoucherApprovalInboxResponse struct {
	Vouchers   []VoucherApprovalInboxItem `json:"vouchers"`
	TotalCount int                        `json:"total_count"`
}

// VoucherApprovalInboxCounts represents counts for each approval status
type VoucherApprovalInboxCounts struct {
	Pending        int `json:"pending"`
	Approved       int `json:"approved"`
	Rejected       int `json:"rejected"`
	RequestChanges int `json:"request_changes"`
}

// BatchApprovalRequest represents a batch approval request for multiple vouchers
type BatchApprovalRequest struct {
	VoucherIDs     []string `json:"voucher_ids" validate:"required,min=1,dive,uuid"`
	ApprovedByID   string   `json:"approved_by_id" validate:"required,uuid"`
	RequesterID    string   `json:"-"` // Set from JWT context
	BusinessID     string   `json:"-"` // Set from header
	Remarks        string   `json:"remarks"` // Optional remarks for all approvals
}

// BatchApprovalResponse represents the result of batch approval operation
type BatchApprovalResponse struct {
	Success        bool                       `json:"success"`
	Message        string                     `json:"message"`
	TotalRequested int                        `json:"total_requested"`
	TotalApproved  int                        `json:"total_approved"`
	TotalFailed    int                        `json:"total_failed"`
	Results        []BatchApprovalResultItem  `json:"results"`
}

// BatchApprovalResultItem represents the result for a single voucher in batch operation
type BatchApprovalResultItem struct {
	VoucherID     string `json:"voucher_id"`
	VoucherNumber string `json:"voucher_number"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	ApprovalID    string `json:"approval_id,omitempty"` // Only present if successful
}

// VoucherActivityItem represents a single activity entry in voucher history
type VoucherActivityItem struct {
	ActivityID   string    `json:"activity_id"`   // Unique ID for this activity (approval_id or payment_id or voucher_id for creation)
	ActivityType string    `json:"activity_type"` // voucher_created, approval_requested, approval_action, payment_initiated, payment_completed, payment_failed
	UserID       string    `json:"user_id"`       // User who performed the action
	UserName     string    `json:"user_name"`     // Name of the user
	Action       string    `json:"action"`        // created, pending, approved, rejected, request_changes, payment_success, payment_failed
	Remarks      string    `json:"remarks"`       // Optional remarks/description
	Timestamp    time.Time `json:"timestamp"`     // When this activity occurred
	Metadata     string    `json:"metadata"`      // Additional contextual information
}

// VoucherActivityResponse represents the complete activity history for a voucher
type VoucherActivityResponse struct {
	VoucherID     string                `json:"voucher_id"`
	VoucherNumber string                `json:"voucher_number"`
	Activities    []VoucherActivityItem `json:"activities"` // Sorted chronologically (oldest first)
	TotalCount    int                   `json:"total_count"`
}

// VoucherPaymentDetails represents complete payment details for a voucher
// Used by repository layer to return raw payment data
type VoucherPaymentDetails struct {
	// Payment identification
	PaymentID     string    `json:"payment_id"`
	VoucherID     string    `json:"voucher_id"`
	VoucherNumber string    `json:"voucher_number"`
	PaymentStatus string    `json:"payment_status"`
	PaymentDate   time.Time `json:"payment_date"`

	// Payment source details
	PaymentMethod  string `json:"payment_method"`
	CardType       string `json:"card_type"`
	CardCategory   string `json:"card_category"`
	LastFourDigits string `json:"last_four_digits"`
	BankName       string `json:"bank_name"`

	// Payment amounts (in paise)
	Amount            int64 `json:"amount"`              // Original voucher amount
	FeeAmount         int64 `json:"fee_amount"`          // Total fee (convenience fee + GST)
	ConvenienceFee    int64 `json:"convenience_fee"`     // Base convenience fee before GST
	ConvenienceFeeGST int64 `json:"convenience_fee_gst"` // GST on convenience fee (18%)
	TotalAmount       int64 `json:"total_amount"`        // amount + fee_amount (what customer paid)

	// Payout details
	PayoutUTR         string     `json:"payout_utr"`          // Bank UTR from payout provider (empty if not yet paid to vendor)
	PayoutCompletedAt *time.Time `json:"payout_completed_at"` // When payout completed to vendor (null if not yet completed)

	Description string `json:"description"`
}

// VoucherWithPayeeDetails represents a voucher with its associated payee relationship details
// Used by payment details endpoint to avoid N+1 queries
type VoucherWithPayeeDetails struct {
	// Voucher core details
	VoucherID     string `json:"voucher_id"`
	VoucherNumber string `json:"voucher_number"`
	InvoiceNumber string `json:"invoice_number"`
	NetAmount     int64  `json:"net_amount"`

	// Payee relation details (from business_payee_relation JOIN)
	RelationID         string                 `json:"relation_id"`
	DisplayName        string                 `json:"display_name"`
	PayeeType          string                 `json:"payee_type"`
	TypeSpecificData   map[string]interface{} `json:"type_specific_data,omitempty"`
}
