package voucher

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// VoucherRepository defines the interface for voucher data persistence
type VoucherRepository interface {
	// CreateDraft creates a new voucher in draft status
	CreateDraft(ctx context.Context, voucher *Voucher) error

	// FinalizeVoucher updates voucher status to finalized and links it to payment
	FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error

	// GetByID retrieves a voucher by its ID
	GetByID(ctx context.Context, voucherID uuid.UUID) (*Voucher, error)

	// GetByVoucherNumber retrieves a voucher by its voucher number
	GetByVoucherNumber(ctx context.Context, voucherNumber string) (*Voucher, error)

	// GetByPaymentID retrieves all vouchers linked to a payment
	// (supports multi-bill payments where multiple vouchers share same payment_id)
	GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*Voucher, error)

	// ListByBusiness retrieves vouchers for a business with filters
	ListByBusiness(ctx context.Context, businessID uuid.UUID, filters VoucherFilters) ([]*Voucher, *PaginationResult, error)

	// Update updates an existing voucher
	Update(ctx context.Context, voucher *Voucher) error

	// UpdateERPStatus updates the ERP sync status of a voucher
	UpdateERPStatus(ctx context.Context, voucherID uuid.UUID, status ERPSyncStatus, referenceID string, metadata map[string]interface{}) error

	// RequestCancellation requests cancellation of a voucher (Phase 7)
	RequestCancellation(ctx context.Context, voucherID uuid.UUID, userID uuid.UUID, reason string, timestamp time.Time) error

	// ApproveCancellation approves a cancellation request (Phase 7)
	ApproveCancellation(ctx context.Context, voucherID uuid.UUID, userID uuid.UUID, timestamp time.Time, refundStatus RefundStatus) error

	// RejectCancellation rejects a cancellation request (Phase 7)
	RejectCancellation(ctx context.Context, voucherID uuid.UUID) error

	// UpdatePayoutStatus updates the payout status of a voucher (Phase 9)
	UpdatePayoutStatus(ctx context.Context, voucherID string, status PayoutStatus, utr string) error

	// GetNextVoucherNumber generates the next sequential voucher number for a business and fiscal year
	GetNextVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (int, error)

	// GetDraftVouchers retrieves all draft vouchers for a business (for abandonment reminders)
	GetDraftVouchers(ctx context.Context, businessID uuid.UUID, olderThan time.Duration) ([]*Voucher, error)

	// GetVouchersForERPSync retrieves vouchers that need ERP synchronization
	GetVouchersForERPSync(ctx context.Context, limit int) ([]*Voucher, error)

	// GetVouchersReadyForPayout retrieves vouchers ready for payout processing (Phase 9)
	// Returns finalized, paid, non-cancelled vouchers with payout_status = 'pending'
	GetVouchersReadyForPayout(ctx context.Context, limit int) ([]*Voucher, error)

	// GetVouchersByRelation retrieves all vouchers for a specific payee relation with payment method details
	// Validates that the relation belongs to the business (security check via business_payee_relation join)
	// Returns vouchers with payment source information (card/bank last digits)
	GetVouchersByRelation(ctx context.Context, businessID, relationID string, limit, offset int) ([]*VoucherWithPaymentInfo, int64, error)

	// GetVoucherListForBusiness retrieves paginated voucher/bill list for a business
	// Includes payment status calculation (paid, open, overdue) and optional status filtering
	GetVoucherListForBusiness(ctx context.Context, businessID string, status *BillPaymentStatus, limit, offset int) ([]VoucherListItem, int, error)

	// CreateOrUpdateApproval creates or updates an approval record for a voucher
	CreateOrUpdateApproval(ctx context.Context, req *VoucherApprovalRequest) (*VoucherApprovalResponse, error)

	// GetApprovalInboxVouchers retrieves vouchers for approval inbox filtered by approval status
	// Returns vouchers with complete approval history (all actions chronologically)
	// For approved status, applies 7-day filter (only approved within last 7 days)
	GetApprovalInboxVouchers(ctx context.Context, businessID string, status ApprovalAction) ([]VoucherApprovalInboxItem, error)

	// GetApprovalInboxCounts retrieves counts for each approval status
	// Returns counts for pending, approved (7-day), rejected, and request_changes
	GetApprovalInboxCounts(ctx context.Context, businessID string) (*VoucherApprovalInboxCounts, error)
}

// VoucherFilters defines filter criteria for listing vouchers
type VoucherFilters struct {
	Status        *VoucherStatus
	ERPSyncStatus *ERPSyncStatus
	DateFrom      *time.Time
	DateTo        *time.Time
	FiscalYear    *string
	PayeeID       *uuid.UUID
	ExpenseCategory *string
	InvoiceNumber *string

	// Pagination
	Page  int
	Limit int

	// Sorting
	SortBy    string // voucher_date, voucher_number, net_amount
	SortOrder string // asc, desc
}

// PaginationResult contains pagination metadata
type PaginationResult struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ApprovalRepository defines the interface for voucher approval operations
type ApprovalRepository interface {
	// CreateApproval creates a new approval record
	CreateApproval(ctx context.Context, approval *VoucherApproval) error

	// GetApprovalsByVoucher retrieves all approvals for a voucher
	GetApprovalsByVoucher(ctx context.Context, voucherID uuid.UUID) ([]*VoucherApproval, error)

	// UpdateApprovalAction updates the approval action (approve/reject)
	UpdateApprovalAction(ctx context.Context, approvalID uuid.UUID, action string, remarks string) error

	// GetPendingApprovals retrieves pending approvals for a user
	GetPendingApprovals(ctx context.Context, userID uuid.UUID) ([]*VoucherApproval, error)
}

// ERPSyncQueueRepository defines the interface for ERP sync queue operations
type ERPSyncQueueRepository interface {
	// Enqueue adds a voucher to the ERP sync queue
	Enqueue(ctx context.Context, queueEntry *ERPSyncQueue) error

	// Dequeue retrieves pending queue entries for processing
	Dequeue(ctx context.Context, limit int) ([]*ERPSyncQueue, error)

	// UpdateQueueStatus updates the status of a queue entry
	UpdateQueueStatus(ctx context.Context, queueID uuid.UUID, status string, response map[string]interface{}, errorCode string, errorMessage string) error

	// IncrementRetryCount increments retry count and schedules next retry
	IncrementRetryCount(ctx context.Context, queueID uuid.UUID, nextRetryAt time.Time) error

	// GetByVoucher retrieves queue entries for a voucher
	GetByVoucher(ctx context.Context, voucherID uuid.UUID) ([]*ERPSyncQueue, error)

	// GetFailedEntries retrieves failed queue entries for monitoring
	GetFailedEntries(ctx context.Context, limit int) ([]*ERPSyncQueue, error)
}
