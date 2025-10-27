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
