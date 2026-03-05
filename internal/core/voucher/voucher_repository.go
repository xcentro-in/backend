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

	// ValidateVoucherForApproval checks if voucher exists, belongs to business, and is in valid status for approval
	// Returns voucher status if valid, error if not found or invalid
	ValidateVoucherForApproval(ctx context.Context, voucherID uuid.UUID, businessID string) (VoucherStatus, error)

	// GetByVoucherNumber retrieves a voucher by its voucher number
	GetByVoucherNumber(ctx context.Context, voucherNumber string) (*Voucher, error)

	// GetByPaymentID retrieves all vouchers linked to a payment
	// (supports multi-bill payments where multiple vouchers share same payment_id)
	GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*Voucher, error)

	// GetVouchersByPaymentIDWithStatus retrieves vouchers for a payment based on payment status
	// For 'success' status: queries payment_vouchers where payment_id is set (finalized)
	// For other statuses: queries payment_voucher_pending table (transient pending records)
	// This method encapsulates the conditional logic for fetching vouchers in different payment states
	GetVouchersByPaymentIDWithStatus(ctx context.Context, paymentID uuid.UUID, paymentStatus string) ([]*Voucher, error)

	// GetVouchersWithPayeeDetailsByPaymentID retrieves vouchers with payee details in a single JOIN query
	// Avoids N+1 problem by joining with business_payee_relation table
	// For 'success' status: queries finalized vouchers
	// For other statuses: queries from payment_voucher_pending table
	// Returns complete voucher and payee information ready for display
	GetVouchersWithPayeeDetailsByPaymentID(ctx context.Context, paymentID uuid.UUID, paymentStatus string) ([]*VoucherWithPayeeDetails, error)

	// ListByBusiness retrieves vouchers for a business with filters
	ListByBusiness(ctx context.Context, businessID uuid.UUID, filters VoucherFilters) ([]*Voucher, *PaginationResult, error)

	// Update updates an existing voucher
	Update(ctx context.Context, voucher *Voucher) error

	// UpdateERPStatus updates the ERP sync status of a voucher
	UpdateERPStatus(ctx context.Context, voucherID uuid.UUID, status ERPSyncStatus, referenceID string, metadata map[string]interface{}) error

	// GetNextVoucherNumber generates the next sequential voucher number for a business and fiscal year
	GetNextVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (int, error)

	// GetDraftVouchers retrieves all draft vouchers for a business (for abandonment reminders)
	GetDraftVouchers(ctx context.Context, businessID uuid.UUID, olderThan time.Duration) ([]*Voucher, error)

	// GetVouchersForERPSync retrieves vouchers that need ERP synchronization
	GetVouchersForERPSync(ctx context.Context, limit int) ([]*Voucher, error)

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

	// BatchApproveVouchers approves multiple vouchers in a single transaction
	// Returns detailed results for each voucher (success/failure with reasons)
	BatchApproveVouchers(ctx context.Context, req *BatchApprovalRequest) (*BatchApprovalResponse, error)

	// GetUnpaidBills retrieves unpaid bills with role-based filtering
	// Owner role: Returns all unpaid bills
	// Other roles: Returns only approved unpaid bills
	GetUnpaidBills(ctx context.Context, req *UnpaidBillsRequest, isOwner bool) ([]UnpaidBillItem, int, error)

	// GetVoucherCounts retrieves counts of vouchers by payment status
	// Returns counts for: all, paid, open, overdue
	GetVoucherCounts(ctx context.Context, businessID string) (*VoucherCountsResponse, error)

	// GetCalendarViewBills retrieves bills for a specific month and year
	// Returns bills filtered by invoice_due_date for calendar view
	GetCalendarViewBills(ctx context.Context, businessID string, year, month int) ([]CalendarBillItem, error)

	// GetVoucherActivity retrieves complete activity history for a voucher
	// Returns chronologically sorted list of activities including:
	// - Voucher creation
	// - Approval actions (request, approve, reject, request_changes)
	// - Payment activities (TODO: when payment activity table is created)
	GetVoucherActivity(ctx context.Context, voucherID string) (*VoucherActivityResponse, error)

	// GetPaymentDetailsByVoucher retrieves complete payment details for a voucher
	// Returns payment breakdown including convenience fees, GST, settlement status, etc.
	// Returns error if voucher not found or payment not linked
	GetPaymentDetailsByVoucher(ctx context.Context, voucherID string) (*VoucherPaymentDetails, error)

	// Delete deletes a voucher by ID (only draft vouchers can be deleted)
	// Cascades to voucher_approvals and erp_sync_queue
	// Fails if payout_transfer_requests exist (ON DELETE RESTRICT)
	Delete(ctx context.Context, voucherID uuid.UUID) error

	// WithTx creates a new repository instance that uses a transaction
	// This enables transactional voucher operations (e.g., finalizing vouchers atomically with payments)
	WithTx(tx interface{}) VoucherRepository

	// AcquireVoucherLockAndValidate acquires advisory lock and validates voucher status
	// Used for race condition prevention in concurrent payment scenarios
	// Returns ErrVoucherLocked if another transaction holds the lock
	// Returns ErrVoucherNotFound if voucher doesn't exist or doesn't belong to business
	// Returns ErrVoucherNotDraft if voucher is not in draft status
	AcquireVoucherLockAndValidate(ctx context.Context, voucherID uuid.UUID, businessID string) error

	// ValidateAndSumVouchers validates vouchers and returns their total net amount in a single query
	// Validates: all vouchers exist, belong to business, are in draft status
	// Returns: (totalAmount, validVoucherCount, error)
	// Error cases: not all vouchers found, wrong business, not in draft status
	ValidateAndSumVouchers(ctx context.Context, voucherIDs []uuid.UUID, businessID string) (int64, int, error)

	// GetVouchersForSplit retrieves per-voucher data required to build split-settlement instructions.
	// For each voucher ID it returns the net_amount and the gateway sub-merchant IDs stored on
	// the linked payee bank account.  Vouchers that are NOT in draft status or do not belong to
	// the business are excluded (the caller should have already validated ownership via
	// ValidateAndSumVouchers).
	GetVouchersForSplit(ctx context.Context, voucherIDs []uuid.UUID, businessID string) ([]*VoucherSplitData, error)
}

// VoucherSplitData holds the per-voucher information needed to build split-settlement legs.
type VoucherSplitData struct {
	VoucherID             uuid.UUID
	NetAmount             int64  // in paise
	JioPaySubmerchantID   string // empty if not yet onboarded with JioPay
	EasebuzzSubmerchantID string // empty if not yet onboarded with Easebuzz
}

// VoucherFilters defines filter criteria for listing vouchers
type VoucherFilters struct {
	Status          *VoucherStatus
	ERPSyncStatus   *ERPSyncStatus
	DateFrom        *time.Time
	DateTo          *time.Time
	FiscalYear      *string
	PayeeID         *uuid.UUID
	ExpenseCategory *string
	InvoiceNumber   *string

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
