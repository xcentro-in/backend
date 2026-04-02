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

	// CreateVoucherDocument inserts a new document record into voucher_documents
	CreateVoucherDocument(ctx context.Context, doc *VoucherDocument) error

	// GetVoucherDocumentByID retrieves a voucher document by its ID
	GetVoucherDocumentByID(ctx context.Context, docID uuid.UUID) (*VoucherDocument, error)

	// UpdateVoucherDocumentValidation updates the validated data and status of a voucher document
	UpdateVoucherDocumentValidation(ctx context.Context, docID uuid.UUID, status DocumentStatus, validatedData map[string]interface{}) error

	// CreateInvoiceFromExtraction persists validated invoice data to the existing invoice table
	// Returns the generated invoice UUID for downstream use (e.g., matching engine)
	CreateInvoiceFromExtraction(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error)

	// CreatePurchaseOrder persists validated PO data to the purchase_orders table
	// Returns the generated PO UUID for downstream use
	CreatePurchaseOrder(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error)

	// GetBusinessIDByGSTIN looks up a business by its GSTIN and returns the bid.
	// Returns empty string if not found, error only on query failures.
	GetBusinessIDByGSTIN(ctx context.Context, gstin string) (string, error)

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

	// ListVoucherDocuments retrieves paginated list of voucher documents
	ListVoucherDocuments(ctx context.Context, filter DocumentListFilter) ([]*VoucherDocument, int64, error)

	// ListExtractedInvoices retrieves paginated list of extracted invoices
	ListExtractedInvoices(ctx context.Context, filter ExtractedInvoiceListFilter) ([]*ExtractedInvoice, int64, error)

	// ListPurchaseOrders retrieves paginated list of purchase orders
	ListPurchaseOrders(ctx context.Context, filter PurchaseOrderListFilter) ([]*PurchaseOrder, int64, error)

	// --- Matching Engine Methods ---

	// GetInvoiceByID retrieves a fully-typed invoice record by its primary key.
	GetInvoiceByID(ctx context.Context, invoiceID uuid.UUID) (*ExtractedInvoice, error)

	// GetPurchaseOrderByPONumber retrieves a PO by its po_number, scoped to a business.
	GetPurchaseOrderByPONumber(ctx context.Context, poNumber string, businessID uuid.UUID) (*PurchaseOrder, error)

	// GetLineItemsByPurchaseOrderID retrieves all typed line items for a PO.
	GetLineItemsByPurchaseOrderID(ctx context.Context, poID uuid.UUID) ([]LineItemDetail, error)

	// GetLineItemsByInvoiceID retrieves all typed line items for an invoice.
	GetLineItemsByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]LineItemDetail, error)

	// UpdateInvoiceMatchStatus updates only the match_status column on an invoice.
	UpdateInvoiceMatchStatus(ctx context.Context, invoiceID uuid.UUID, status string) error

	// UpdatePurchaseOrderBalance atomically updates remaining_balance and po_status.
	UpdatePurchaseOrderBalance(ctx context.Context, poID uuid.UUID, remainingBalance float64, status string) error

	// UpdateLineItemQtyReceived adds to the qty_received on a line item.
	UpdateLineItemQtyReceived(ctx context.Context, lineItemID uuid.UUID, qtyReceived float64) error

	// CreateMatchResult persists a matching audit record.
	CreateMatchResult(ctx context.Context, result *MatchResult) error

	// GetOrphanInvoicesByPONumber finds invoices in ORPHAN_INVOICE status with the given po_number.
	GetOrphanInvoicesByPONumber(ctx context.Context, poNumber string) ([]*ExtractedInvoice, error)
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
