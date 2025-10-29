package voucher

import "time"

// ApprovalAction represents the approval action states
type ApprovalAction string

const (
	ApprovalActionPending        ApprovalAction = "pending"
	ApprovalActionApproved       ApprovalAction = "approved"
	ApprovalActionRejected       ApprovalAction = "rejected"
	ApprovalActionRequestChanges ApprovalAction = "request_changes"
)

// VoucherApprovalRequest represents the request to create/update an approval
type VoucherApprovalRequest struct {
	VoucherID       string         `json:"voucher_id" validate:"required,uuid"`
	BusinessID      string         `json:"-"` // Set from header, not from request body
	RequesterUserID string         `json:"-"` // Set from JWT context, not from request body
	ApproverUserID  string         `json:"approver_user_id" validate:"required,uuid"` // Who should approve
	Action          ApprovalAction `json:"action" validate:"required,oneof=pending approved rejected request_changes"`
	Remarks         string         `json:"remarks"` // Optional remarks
}

// VoucherApprovalResponse represents the response after approval action
type VoucherApprovalResponse struct {
	Success    bool           `json:"success"`
	Message    string         `json:"message"`
	ApprovalID string         `json:"approval_id,omitempty"`
	VoucherID  string         `json:"voucher_id"`
	Action     ApprovalAction `json:"action"`
	CreatedAt  time.Time      `json:"created_at"` // When this action was taken
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
	Status     ApprovalAction `json:"status" validate:"required,oneof=pending approved rejected request_changes"`
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
