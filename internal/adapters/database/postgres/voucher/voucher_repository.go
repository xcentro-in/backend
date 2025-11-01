package voucher

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/crypto"
)

// PostgresVoucherRepository implements the VoucherRepository interface using PostgreSQL
type PostgresVoucherRepository struct {
	querier       client.Querier
	db            client.DB // Needed for transaction support
	encryptionKey string
}

// NewPostgresVoucherRepository creates a new PostgreSQL voucher repository
func NewPostgresVoucherRepository(db client.DB, encryptionKey string) voucher.VoucherRepository {
	return &PostgresVoucherRepository{
		querier:       db,
		db:            db,
		encryptionKey: encryptionKey,
	}
}

// CreateDraft creates a new voucher in draft status
func (r *PostgresVoucherRepository) CreateDraft(ctx context.Context, v *voucher.Voucher) error {
	query := `
		INSERT INTO xcentro.payment_vouchers (
			voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
			business_id, payee_id, account_id,
			invoice_number, invoice_date, invoice_due_date,
			gross_amount, net_amount, currency,
			gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
			tds_amount, tds_rate, tds_section,
			fee_amount, discount_amount,
			financial_details, attachments,
			expense_category, description,
			status, payment_id,
			erp_sync_status, erp_sync_attempts,
			gl_expense_account, gl_bank_account, cost_center,
			created_at, created_by_user_id, metadata
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19, $20,
			$21, $22, $23,
			$24, $25,
			$26, $27,
			$28, $29,
			$30, $31,
			$32, $33,
			$34, $35, $36,
			$37, $38, $39
		)
	`

	// Marshal JSONB fields
	financialDetailsJSON, err := json.Marshal(v.FinancialDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal financial_details: %w", err)
	}

	attachmentsJSON, err := json.Marshal(v.Attachments)
	if err != nil {
		return fmt.Errorf("failed to marshal attachments: %w", err)
	}

	metadataJSON, err := json.Marshal(v.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	_, err = r.querier.Exec(ctx, query,
		v.VoucherID, v.VoucherNumber, v.VoucherDate, v.FiscalYear, v.AccountingPeriod, v.VoucherType,
		v.BusinessID, v.PayeeID, v.AccountID,
		v.InvoiceNumber, v.InvoiceDate, v.InvoiceDueDate,
		v.GrossAmount, v.NetAmount, v.Currency,
		v.GSTAmount, v.GSTRate, v.CGSTAmount, v.SGSTAmount, v.IGSTAmount,
		v.TDSAmount, v.TDSRate, v.TDSSection,
		v.FeeAmount, v.DiscountAmount,
		financialDetailsJSON, attachmentsJSON,
		v.ExpenseCategory, v.Description,
		v.Status, v.PaymentID,
		v.ERPSyncStatus, v.ERPSyncAttempts,
		v.GLExpenseAccount, v.GLBankAccount, v.CostCenter,
		v.CreatedAt, v.CreatedByID, metadataJSON,
	)

	if err != nil {
		// Check for unique constraint violation (duplicate voucher number)
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return voucher.ErrVoucherAlreadyExists
		}
		// Check for foreign key violation
		if strings.Contains(err.Error(), "foreign key") {
			return fmt.Errorf("foreign key constraint violated: %w", err)
		}
		return fmt.Errorf("failed to create draft voucher: %w", err)
	}

	return nil
}

// FinalizeVoucher updates voucher status to finalized and links it to payment
func (r *PostgresVoucherRepository) FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET status = $2,
		    payment_id = $3,
		    updated_at = NOW()
		WHERE voucher_id = $1 AND status = $4
	`

	result, err := r.querier.Exec(ctx, query, voucherID, voucher.StatusFinalized, paymentID, voucher.StatusDraft)
	if err != nil {
		return fmt.Errorf("failed to finalize voucher: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		// Check if voucher exists
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM xcentro.payment_vouchers WHERE voucher_id = $1)`
		err = r.querier.QueryRow(ctx, checkQuery, voucherID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check voucher existence: %w", err)
		}
		if !exists {
			return voucher.ErrVoucherNotFound
		}
		return voucher.ErrVoucherNotDraft
	}

	return nil
}

// ValidateVoucherForApproval checks if voucher exists, belongs to business, and is in valid status for approval
func (r *PostgresVoucherRepository) ValidateVoucherForApproval(ctx context.Context, voucherID uuid.UUID, businessID string) (voucher.VoucherStatus, error) {
	query := `
		SELECT status
		FROM xcentro.payment_vouchers
		WHERE voucher_id = $1 AND business_id = $2
	`

	var status voucher.VoucherStatus
	err := r.querier.QueryRow(ctx, query, voucherID, businessID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", voucher.ErrVoucherNotFound
		}
		return "", fmt.Errorf("failed to validate voucher: %w", err)
	}

	return status, nil
}

// GetByID retrieves a voucher by its ID
func (r *PostgresVoucherRepository) GetByID(ctx context.Context, voucherID uuid.UUID) (*voucher.Voucher, error) {
	query := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       payment_terms, vendor_reference_number,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       payout_status, payout_initiated_at, payout_initiated_by, payout_completed_at, payout_reference_id, payout_utr, payout_metadata,
		       cancellation_status, cancellation_requested_at, cancellation_requested_by, cancellation_reason,
		       cancellation_approved_at, cancellation_approved_by, refund_status, refund_initiated_at, refund_reference_id, refund_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE voucher_id = $1
	`

	row := r.querier.QueryRow(ctx, query, voucherID)
	v, err := r.scanVoucher(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, voucher.ErrVoucherNotFound
		}
		return nil, fmt.Errorf("failed to get voucher by ID: %w", err)
	}

	return v, nil
}

// GetPayeeDetailsByIDs retrieves enriched payee details for given payee_id and account_id
func (r *PostgresVoucherRepository) GetPayeeDetailsByIDs(ctx context.Context, payeeID, accountID uuid.UUID) (map[string]interface{}, error) {
	query := `
		SELECT
			bpr.relation_id,
			bpr.payee_id,
			bpr.account_id,
			bpr.display_name,
			COALESCE(pv.gstin, '') AS gstin,
			CASE
				WHEN pv.payee_id IS NOT NULL THEN 'vendor'
				WHEN pe.payee_id IS NOT NULL THEN 'employee'
				WHEN pb.payee_id IS NOT NULL THEN 'biller'
				ELSE 'unknown'
			END AS payee_type,
			COALESCE(pba.account_label, 'Account ending ****') AS bank_account_masked,
			pba.ifsc_code,
			pba.bank_name,
			pba.account_type
		FROM xcentro.business_payee_relation bpr
		LEFT JOIN xcentro.payee_vendor pv ON bpr.payee_id = pv.payee_id
		LEFT JOIN xcentro.payee_employee pe ON bpr.payee_id = pe.payee_id
		LEFT JOIN xcentro.payee_biller pb ON bpr.payee_id = pb.payee_id
		INNER JOIN xcentro.payee_bank_accounts pba ON bpr.account_id = pba.account_id
		WHERE bpr.payee_id = $1 AND bpr.account_id = $2
	`

	var (
		relationID        uuid.UUID
		dbPayeeID         uuid.UUID
		dbAccountID       uuid.UUID
		displayName       string
		gstin             string
		payeeType         string
		bankAccountMasked string
		ifscCode          string
		bankName          string
		accountType       string
	)

	err := r.querier.QueryRow(ctx, query, payeeID, accountID).Scan(
		&relationID,
		&dbPayeeID,
		&dbAccountID,
		&displayName,
		&gstin,
		&payeeType,
		&bankAccountMasked,
		&ifscCode,
		&bankName,
		&accountType,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("payee relation not found for payee_id=%s, account_id=%s", payeeID, accountID)
		}
		return nil, fmt.Errorf("failed to get payee details: %w", err)
	}

	payeeDetails := map[string]interface{}{
		"relation_id":         relationID,
		"payee_id":            dbPayeeID,
		"name":                displayName,
		"gstin":               gstin,
		"type":                payeeType,
		"bank_account_masked": bankAccountMasked,
		"account_id":          dbAccountID,
		"ifsc_code":           ifscCode,
		"bank_name":           bankName,
		"account_type":        accountType,
	}

	return payeeDetails, nil
}

// GetByVoucherNumber retrieves a voucher by its voucher number
func (r *PostgresVoucherRepository) GetByVoucherNumber(ctx context.Context, voucherNumber string) (*voucher.Voucher, error) {
	query := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       payout_status, payout_initiated_at, payout_initiated_by, payout_completed_at, payout_reference_id, payout_utr, payout_metadata,
		       cancellation_status, cancellation_requested_at, cancellation_requested_by, cancellation_reason,
		       cancellation_approved_at, cancellation_approved_by, refund_status, refund_initiated_at, refund_reference_id, refund_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE voucher_number = $1
	`

	row := r.querier.QueryRow(ctx, query, voucherNumber)
	v, err := r.scanVoucher(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, voucher.ErrVoucherNotFound
		}
		return nil, fmt.Errorf("failed to get voucher by number: %w", err)
	}

	return v, nil
}

// GetByPaymentID retrieves all vouchers linked to a payment
func (r *PostgresVoucherRepository) GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*voucher.Voucher, error) {
	query := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       payout_status, payout_initiated_at, payout_initiated_by, payout_completed_at, payout_reference_id, payout_utr, payout_metadata,
		       cancellation_status, cancellation_requested_at, cancellation_requested_by, cancellation_reason,
		       cancellation_approved_at, cancellation_approved_by, refund_status, refund_initiated_at, refund_reference_id, refund_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE payment_id = $1
		ORDER BY voucher_number
	`

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouchers by payment_id: %w", err)
	}
	defer rows.Close()

	var vouchers []*voucher.Voucher
	for rows.Next() {
		v, err := r.scanVoucherRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher row: %w", err)
		}
		vouchers = append(vouchers, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchers, nil
}

// ListByBusiness retrieves vouchers for a business with filters
func (r *PostgresVoucherRepository) ListByBusiness(ctx context.Context, businessID uuid.UUID, filters voucher.VoucherFilters) ([]*voucher.Voucher, *voucher.PaginationResult, error) {
	// Build dynamic query
	querySQL, args := r.buildFilterQuery(businessID, filters)

	// Get total count
	countQuery := strings.Replace(querySQL, "SELECT voucher_id, voucher_number", "SELECT COUNT(*)", 1)
	countQuery = countQuery[:strings.Index(countQuery, "ORDER BY")] // Remove ORDER BY and LIMIT

	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args[:len(args)-2]...).Scan(&total)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to count vouchers: %w", err)
	}

	// Get vouchers
	rows, err := r.querier.Query(ctx, querySQL, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query vouchers: %w", err)
	}
	defer rows.Close()

	var vouchers []*voucher.Voucher
	for rows.Next() {
		v, err := r.scanVoucherRow(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan voucher row: %w", err)
		}
		vouchers = append(vouchers, v)
	}

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	// Calculate pagination
	page := filters.Page
	if page < 1 {
		page = 1
	}
	limit := filters.Limit
	if limit < 1 {
		limit = 50
	}

	totalPages := int(total) / limit
	if int(total)%limit != 0 {
		totalPages++
	}

	pagination := &voucher.PaginationResult{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return vouchers, pagination, nil
}

// Update updates an existing voucher
func (r *PostgresVoucherRepository) Update(ctx context.Context, v *voucher.Voucher) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET voucher_date = $2,
		    invoice_number = $3,
		    invoice_date = $4,
		    invoice_due_date = $5,
		    gross_amount = $6,
		    net_amount = $7,
		    gst_amount = $8,
		    gst_rate = $9,
		    cgst_amount = $10,
		    sgst_amount = $11,
		    igst_amount = $12,
		    tds_amount = $13,
		    tds_rate = $14,
		    tds_section = $15,
		    fee_amount = $16,
		    discount_amount = $17,
		    financial_details = $18,
		    attachments = $19,
		    expense_category = $20,
		    description = $21,
		    gl_expense_account = $22,
		    gl_bank_account = $23,
		    cost_center = $24,
		    metadata = $25,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	// Marshal JSONB fields
	financialDetailsJSON, err := json.Marshal(v.FinancialDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal financial_details: %w", err)
	}

	attachmentsJSON, err := json.Marshal(v.Attachments)
	if err != nil {
		return fmt.Errorf("failed to marshal attachments: %w", err)
	}

	metadataJSON, err := json.Marshal(v.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	result, err := r.querier.Exec(ctx, query,
		v.VoucherID,
		v.VoucherDate,
		v.InvoiceNumber,
		v.InvoiceDate,
		v.InvoiceDueDate,
		v.GrossAmount,
		v.NetAmount,
		v.GSTAmount,
		v.GSTRate,
		v.CGSTAmount,
		v.SGSTAmount,
		v.IGSTAmount,
		v.TDSAmount,
		v.TDSRate,
		v.TDSSection,
		v.FeeAmount,
		v.DiscountAmount,
		financialDetailsJSON,
		attachmentsJSON,
		v.ExpenseCategory,
		v.Description,
		v.GLExpenseAccount,
		v.GLBankAccount,
		v.CostCenter,
		metadataJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to update voucher: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return voucher.ErrVoucherNotFound
	}

	return nil
}

// UpdateERPStatus updates the ERP sync status of a voucher
func (r *PostgresVoucherRepository) UpdateERPStatus(ctx context.Context, voucherID uuid.UUID, status voucher.ERPSyncStatus, referenceID string, metadata map[string]interface{}) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET erp_sync_status = $2,
		    erp_reference_id = $3,
		    erp_last_sync_at = NOW(),
		    erp_sync_attempts = erp_sync_attempts + 1,
		    erp_metadata = $4,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	// Marshal ERP metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal erp_metadata: %w", err)
	}

	result, err := r.querier.Exec(ctx, query, voucherID, status, referenceID, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to update ERP status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return voucher.ErrVoucherNotFound
	}

	return nil
}

// RequestCancellation requests cancellation of a voucher (Phase 7)
func (r *PostgresVoucherRepository) RequestCancellation(ctx context.Context, voucherID uuid.UUID, userID uuid.UUID, reason string, timestamp time.Time) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET cancellation_status = $2,
		    cancellation_requested_at = $3,
		    cancellation_requested_by = $4,
		    cancellation_reason = $5,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	result, err := r.querier.Exec(ctx, query, voucherID, voucher.CancellationStatusCancellationRequested, timestamp, userID, reason)
	if err != nil {
		return fmt.Errorf("failed to request cancellation: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return voucher.ErrVoucherNotFound
	}

	return nil
}

// ApproveCancellation approves a cancellation request (Phase 7)
func (r *PostgresVoucherRepository) ApproveCancellation(ctx context.Context, voucherID uuid.UUID, userID uuid.UUID, timestamp time.Time, refundStatus voucher.RefundStatus) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET cancellation_status = $2,
		    cancellation_approved_at = $3,
		    cancellation_approved_by = $4,
		    refund_status = $5,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	result, err := r.querier.Exec(ctx, query, voucherID, voucher.CancellationStatusCancelled, timestamp, userID, refundStatus)
	if err != nil {
		return fmt.Errorf("failed to approve cancellation: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return voucher.ErrVoucherNotFound
	}

	return nil
}

// RejectCancellation rejects a cancellation request (Phase 7)
func (r *PostgresVoucherRepository) RejectCancellation(ctx context.Context, voucherID uuid.UUID) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET cancellation_status = $2,
		    cancellation_requested_at = NULL,
		    cancellation_requested_by = NULL,
		    cancellation_reason = NULL,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	result, err := r.querier.Exec(ctx, query, voucherID, voucher.CancellationStatusNotCancelled)
	if err != nil {
		return fmt.Errorf("failed to reject cancellation: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return voucher.ErrVoucherNotFound
	}

	return nil
}

// GetNextVoucherNumber generates the next sequential voucher number for a business and fiscal year
func (r *PostgresVoucherRepository) GetNextVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (int, error) {
	query := `
		SELECT COALESCE(MAX(CAST(SUBSTRING(voucher_number FROM 9) AS INTEGER)), 0) + 1
		FROM xcentro.payment_vouchers
		WHERE business_id = $1 AND fiscal_year = $2
	`

	var nextNumber int
	err := r.querier.QueryRow(ctx, query, businessID, fiscalYear).Scan(&nextNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to get next voucher number: %w", err)
	}

	return nextNumber, nil
}

// GetDraftVouchers retrieves all draft vouchers for a business
func (r *PostgresVoucherRepository) GetDraftVouchers(ctx context.Context, businessID uuid.UUID, olderThan time.Duration) ([]*voucher.Voucher, error) {
	query := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE business_id = $1
		  AND status = $2
		  AND created_at < $3
		ORDER BY created_at
	`

	cutoffTime := time.Now().Add(-olderThan)

	rows, err := r.querier.Query(ctx, query, businessID, voucher.StatusDraft, cutoffTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query draft vouchers: %w", err)
	}
	defer rows.Close()

	var vouchers []*voucher.Voucher
	for rows.Next() {
		v, err := r.scanVoucherRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher row: %w", err)
		}
		vouchers = append(vouchers, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchers, nil
}

// GetVouchersForERPSync retrieves vouchers that need ERP synchronization
func (r *PostgresVoucherRepository) GetVouchersForERPSync(ctx context.Context, limit int) ([]*voucher.Voucher, error) {
	query := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE status IN ($1, $2)
		  AND erp_sync_status IN ($3, $4)
		ORDER BY created_at
		LIMIT $5
	`

	rows, err := r.querier.Query(ctx, query,
		voucher.StatusFinalized,
		voucher.StatusPosted,
		voucher.ERPNotSynced,
		voucher.ERPFailed,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouchers for ERP sync: %w", err)
	}
	defer rows.Close()

	var vouchers []*voucher.Voucher
	for rows.Next() {
		v, err := r.scanVoucherRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher row: %w", err)
		}
		vouchers = append(vouchers, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchers, nil
}

// Helper methods

// scanVoucher scans a single row into a Voucher struct
func (r *PostgresVoucherRepository) scanVoucher(row pgx.Row) (*voucher.Voucher, error) {
	var v voucher.Voucher
	var financialDetailsJSON, attachmentsJSON, erpMetadataJSON, payoutMetadataJSON, refundMetadataJSON, metadataJSON []byte

	// Nullable string fields
	var invoiceNumber, expenseCategory, description sql.NullString
	var paymentTerms, vendorReferenceNumber sql.NullString
	var tdsSection sql.NullString
	var erpReferenceID, payoutReferenceID, payoutUTR, cancellationReason, refundReferenceID sql.NullString
	var glExpenseAccount, glBankAccount, costCenter sql.NullString

	// Nullable numeric fields
	var gstRate, tdsRate sql.NullFloat64

	err := row.Scan(
		&v.VoucherID, &v.VoucherNumber, &v.VoucherDate, &v.FiscalYear, &v.AccountingPeriod, &v.VoucherType,
		&v.BusinessID, &v.PayeeID, &v.AccountID,
		&invoiceNumber, &v.InvoiceDate, &v.InvoiceDueDate,
		&v.GrossAmount, &v.NetAmount, &v.Currency,
		&v.GSTAmount, &gstRate, &v.CGSTAmount, &v.SGSTAmount, &v.IGSTAmount,
		&v.TDSAmount, &tdsRate, &tdsSection,
		&v.FeeAmount, &v.DiscountAmount,
		&financialDetailsJSON, &attachmentsJSON,
		&expenseCategory, &description,
		&paymentTerms, &vendorReferenceNumber,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &erpReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
		&v.PayoutStatus, &v.PayoutInitiatedAt, &v.PayoutInitiatedBy, &v.PayoutCompletedAt, &payoutReferenceID, &payoutUTR, &payoutMetadataJSON,
		&v.CancellationStatus, &v.CancellationRequestedAt, &v.CancellationRequestedBy, &cancellationReason,
		&v.CancellationApprovedAt, &v.CancellationApprovedBy, &v.RefundStatus, &v.RefundInitiatedAt, &refundReferenceID, &refundMetadataJSON,
		&glExpenseAccount, &glBankAccount, &costCenter,
		&v.CreatedAt, &v.CreatedByID, &v.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable strings
	if invoiceNumber.Valid {
		v.InvoiceNumber = invoiceNumber.String
	}
	if expenseCategory.Valid {
		v.ExpenseCategory = expenseCategory.String
	}
	if description.Valid {
		v.Description = description.String
	}
	if paymentTerms.Valid {
		v.PaymentTerms = paymentTerms.String
	}
	if vendorReferenceNumber.Valid {
		v.VendorReferenceNumber = vendorReferenceNumber.String
	}
	if tdsSection.Valid {
		v.TDSSection = tdsSection.String
	}
	if erpReferenceID.Valid {
		v.ERPReferenceID = erpReferenceID.String
	}
	if payoutReferenceID.Valid {
		v.PayoutReferenceID = payoutReferenceID.String
	}
	if payoutUTR.Valid {
		v.PayoutUTR = payoutUTR.String
	}
	if cancellationReason.Valid {
		v.CancellationReason = cancellationReason.String
	}
	if refundReferenceID.Valid {
		v.RefundReferenceID = refundReferenceID.String
	}
	if glExpenseAccount.Valid {
		v.GLExpenseAccount = glExpenseAccount.String
	}
	if glBankAccount.Valid {
		v.GLBankAccount = glBankAccount.String
	}
	if costCenter.Valid {
		v.CostCenter = costCenter.String
	}

	// Handle nullable floats
	if gstRate.Valid {
		v.GSTRate = gstRate.Float64
	}
	if tdsRate.Valid {
		v.TDSRate = tdsRate.Float64
	}

	// Unmarshal JSONB fields
	if len(financialDetailsJSON) > 0 {
		if err := json.Unmarshal(financialDetailsJSON, &v.FinancialDetails); err != nil {
			return nil, fmt.Errorf("failed to unmarshal financial_details: %w", err)
		}
	}

	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &v.Attachments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal attachments: %w", err)
		}
	}

	if len(erpMetadataJSON) > 0 {
		if err := json.Unmarshal(erpMetadataJSON, &v.ERPMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal erp_metadata: %w", err)
		}
	}

	if len(payoutMetadataJSON) > 0 {
		if err := json.Unmarshal(payoutMetadataJSON, &v.PayoutMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payout_metadata: %w", err)
		}
	}

	if len(refundMetadataJSON) > 0 {
		if err := json.Unmarshal(refundMetadataJSON, &v.RefundMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal refund_metadata: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &v.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &v, nil
}

// scanVoucherRow scans rows (for Query) into a Voucher struct
func (r *PostgresVoucherRepository) scanVoucherRow(rows pgx.Rows) (*voucher.Voucher, error) {
	var v voucher.Voucher
	var financialDetailsJSON, attachmentsJSON, erpMetadataJSON, payoutMetadataJSON, refundMetadataJSON, metadataJSON []byte

	// Nullable string fields
	var invoiceNumber, expenseCategory, description sql.NullString
	var paymentTerms, vendorReferenceNumber sql.NullString
	var tdsSection sql.NullString
	var erpReferenceID, payoutReferenceID, payoutUTR, cancellationReason, refundReferenceID sql.NullString
	var glExpenseAccount, glBankAccount, costCenter sql.NullString

	// Nullable numeric fields
	var gstRate, tdsRate sql.NullFloat64

	err := rows.Scan(
		&v.VoucherID, &v.VoucherNumber, &v.VoucherDate, &v.FiscalYear, &v.AccountingPeriod, &v.VoucherType,
		&v.BusinessID, &v.PayeeID, &v.AccountID,
		&v.InvoiceNumber, &v.InvoiceDate, &v.InvoiceDueDate,
		&v.GrossAmount, &v.NetAmount, &v.Currency,
		&v.GSTAmount, &gstRate, &v.CGSTAmount, &v.SGSTAmount, &v.IGSTAmount,
		&v.TDSAmount, &tdsRate, &tdsSection,
		&v.FeeAmount, &v.DiscountAmount,
		&financialDetailsJSON, &attachmentsJSON,
		&v.ExpenseCategory, &v.Description,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &erpReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
		&v.PayoutStatus, &v.PayoutInitiatedAt, &v.PayoutInitiatedBy, &v.PayoutCompletedAt, &payoutReferenceID, &payoutUTR, &payoutMetadataJSON,
		&v.CancellationStatus, &v.CancellationRequestedAt, &v.CancellationRequestedBy, &cancellationReason,
		&v.CancellationApprovedAt, &v.CancellationApprovedBy, &v.RefundStatus, &v.RefundInitiatedAt, &refundReferenceID, &refundMetadataJSON,
		&glExpenseAccount, &glBankAccount, &costCenter,
		&v.CreatedAt, &v.CreatedByID, &v.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable strings
	if invoiceNumber.Valid {
		v.InvoiceNumber = invoiceNumber.String
	}
	if expenseCategory.Valid {
		v.ExpenseCategory = expenseCategory.String
	}
	if description.Valid {
		v.Description = description.String
	}
	if paymentTerms.Valid {
		v.PaymentTerms = paymentTerms.String
	}
	if vendorReferenceNumber.Valid {
		v.VendorReferenceNumber = vendorReferenceNumber.String
	}
	if tdsSection.Valid {
		v.TDSSection = tdsSection.String
	}
	if erpReferenceID.Valid {
		v.ERPReferenceID = erpReferenceID.String
	}
	if payoutReferenceID.Valid {
		v.PayoutReferenceID = payoutReferenceID.String
	}
	if payoutUTR.Valid {
		v.PayoutUTR = payoutUTR.String
	}
	if cancellationReason.Valid {
		v.CancellationReason = cancellationReason.String
	}
	if refundReferenceID.Valid {
		v.RefundReferenceID = refundReferenceID.String
	}
	if glExpenseAccount.Valid {
		v.GLExpenseAccount = glExpenseAccount.String
	}
	if glBankAccount.Valid {
		v.GLBankAccount = glBankAccount.String
	}
	if costCenter.Valid {
		v.CostCenter = costCenter.String
	}

	// Handle nullable floats
	if gstRate.Valid {
		v.GSTRate = gstRate.Float64
	}
	if tdsRate.Valid {
		v.TDSRate = tdsRate.Float64
	}

	// Unmarshal JSONB fields
	if len(financialDetailsJSON) > 0 {
		if err := json.Unmarshal(financialDetailsJSON, &v.FinancialDetails); err != nil {
			return nil, fmt.Errorf("failed to unmarshal financial_details: %w", err)
		}
	}

	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &v.Attachments); err != nil {
			return nil, fmt.Errorf("failed to unmarshal attachments: %w", err)
		}
	}

	if len(erpMetadataJSON) > 0 {
		if err := json.Unmarshal(erpMetadataJSON, &v.ERPMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal erp_metadata: %w", err)
		}
	}

	if len(payoutMetadataJSON) > 0 {
		if err := json.Unmarshal(payoutMetadataJSON, &v.PayoutMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payout_metadata: %w", err)
		}
	}

	if len(refundMetadataJSON) > 0 {
		if err := json.Unmarshal(refundMetadataJSON, &v.RefundMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal refund_metadata: %w", err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &v.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &v, nil
}

// buildFilterQuery builds dynamic SQL query based on filters
func (r *PostgresVoucherRepository) buildFilterQuery(businessID uuid.UUID, filters voucher.VoucherFilters) (string, []interface{}) {
	baseQuery := `
		SELECT voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
		       business_id, payee_id, account_id,
		       invoice_number, invoice_date, invoice_due_date,
		       gross_amount, net_amount, currency,
		       gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
		       tds_amount, tds_rate, tds_section,
		       fee_amount, discount_amount,
		       financial_details, attachments,
		       expense_category, description,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       payout_status, payout_initiated_at, payout_initiated_by, payout_completed_at, payout_reference_id, payout_utr, payout_metadata,
		       cancellation_status, cancellation_requested_at, cancellation_requested_by, cancellation_reason,
		       cancellation_approved_at, cancellation_approved_by, refund_status, refund_initiated_at, refund_reference_id, refund_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE business_id = $1
	`

	var whereClauses []string
	args := []interface{}{businessID}
	argCounter := 2

	// Add filters
	if filters.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argCounter))
		args = append(args, *filters.Status)
		argCounter++
	}

	if filters.ERPSyncStatus != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("erp_sync_status = $%d", argCounter))
		args = append(args, *filters.ERPSyncStatus)
		argCounter++
	}

	if filters.DateFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("voucher_date >= $%d", argCounter))
		args = append(args, *filters.DateFrom)
		argCounter++
	}

	if filters.DateTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("voucher_date <= $%d", argCounter))
		args = append(args, *filters.DateTo)
		argCounter++
	}

	if filters.FiscalYear != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("fiscal_year = $%d", argCounter))
		args = append(args, *filters.FiscalYear)
		argCounter++
	}

	if filters.PayeeID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("payee_id = $%d", argCounter))
		args = append(args, *filters.PayeeID)
		argCounter++
	}

	if filters.ExpenseCategory != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("expense_category = $%d", argCounter))
		args = append(args, *filters.ExpenseCategory)
		argCounter++
	}

	if filters.InvoiceNumber != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("invoice_number = $%d", argCounter))
		args = append(args, *filters.InvoiceNumber)
		argCounter++
	}

	// Append WHERE clauses
	if len(whereClauses) > 0 {
		baseQuery += " AND " + strings.Join(whereClauses, " AND ")
	}

	// Add ORDER BY
	sortBy := "voucher_date"
	if filters.SortBy != "" {
		sortBy = filters.SortBy
	}
	sortOrder := "DESC"
	if filters.SortOrder != "" {
		sortOrder = strings.ToUpper(filters.SortOrder)
	}
	baseQuery += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Add pagination
	limit := filters.Limit
	if limit < 1 {
		limit = 50
	}
	page := filters.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
	args = append(args, limit, offset)

	return baseQuery, args
}

// UpdatePayoutStatus updates the payout status of a voucher (Phase 9)
func (r *PostgresVoucherRepository) UpdatePayoutStatus(ctx context.Context, voucherID string, status voucher.PayoutStatus, utr string) error {
	query := `
		UPDATE xcentro.payment_vouchers
		SET payout_status = $2,
		    payout_reference = $3,
		    updated_at = NOW()
		WHERE voucher_id = $1
	`

	result, err := r.querier.Exec(ctx, query, voucherID, status, utr)
	if err != nil {
		return fmt.Errorf("failed to update payout status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("voucher not found: %s", voucherID)
	}

	return nil
}

// GetVouchersReadyForPayout retrieves vouchers ready for payout processing (Phase 9)
// Returns finalized, paid, non-cancelled vouchers with payout_status = 'pending'
func (r *PostgresVoucherRepository) GetVouchersReadyForPayout(ctx context.Context, limit int) ([]*voucher.Voucher, error) {
	query := `
		SELECT
			voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type,
			business_id, payee_id, account_id,
			invoice_number, invoice_date, invoice_due_date,
			gross_amount, net_amount, currency,
			gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount,
			tds_amount, tds_rate, tds_section,
			fee_amount, discount_amount,
			financial_details, attachments,
			expense_category, description,
			status, payment_id,
			erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
			payout_status, payout_initiated_at, payout_initiated_by, payout_completed_at, payout_reference_id, payout_utr, payout_metadata,
			cancellation_status, cancellation_requested_at, cancellation_requested_by, cancellation_reason,
			cancellation_approved_at, cancellation_approved_by, refund_status, refund_metadata,
			gl_expense_account, gl_bank_account, cost_center,
			created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE status = $1
			AND payment_id IS NOT NULL
			AND cancellation_status = $2
			AND payout_status = $3
		ORDER BY voucher_date ASC, created_at ASC
		LIMIT $4
	`

	rows, err := r.querier.Query(ctx, query,
		voucher.StatusFinalized,
		voucher.CancellationStatusNotCancelled,
		voucher.PayoutStatusPending,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouchers ready for payout: %w", err)
	}
	defer rows.Close()

	var vouchers []*voucher.Voucher
	for rows.Next() {
		v, err := r.scanVoucherRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher row: %w", err)
		}
		vouchers = append(vouchers, v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchers, nil
}

// GetVoucherListForBusiness retrieves paginated voucher/bill list for a business
// Calculates payment status (paid, open, overdue) and supports optional status filtering
func (r *PostgresVoucherRepository) GetVoucherListForBusiness(ctx context.Context, businessID string, status *voucher.BillPaymentStatus, limit, offset int) ([]voucher.VoucherListItem, int, error) {
	// First, get total count for pagination
	countQuery := `
		SELECT COUNT(v.voucher_id)
		FROM xcentro.payment_vouchers v
		LEFT JOIN xcentro.payments p ON v.payment_id = p.payment_id
		WHERE v.business_id = $1
	`

	countArgs := []interface{}{businessID}

	// Add status filter to count query if specified
	if status != nil {
		statusCondition := buildStatusCondition(*status)
		countQuery += " AND " + statusCondition
	}

	var totalCount int
	err := r.querier.QueryRow(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get voucher count: %w", err)
	}

	// Main query to fetch voucher list with payment status calculation
	query := `
		SELECT
			v.voucher_id,
			v.voucher_number,
			COALESCE(v.invoice_number, '') as invoice_number,
			v.invoice_due_date,
			bpr.display_name as payee_name,
			v.net_amount,
			v.status as voucher_status,
			p.payment_id,
			p.status as payment_status,
			p.created_at as payment_date,
			-- Calculate bill payment status
			CASE
				WHEN p.payment_id IS NOT NULL AND p.status = 'success' THEN 'paid'
				WHEN p.payment_id IS NULL AND v.invoice_due_date IS NOT NULL AND v.invoice_due_date <= CURRENT_DATE THEN 'overdue'
				WHEN p.payment_id IS NULL AND (v.invoice_due_date IS NULL OR v.invoice_due_date > CURRENT_DATE) THEN 'open'
				ELSE 'open'
			END as bill_status,
			-- Get latest approval status (if any)
			COALESCE(va.action::text, '') as approval_status
		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.business_payee_relation bpr
			ON v.business_id = bpr.business_id
			AND v.payee_id = bpr.payee_id
			AND v.account_id = bpr.account_id
		LEFT JOIN xcentro.payments p ON v.payment_id = p.payment_id
		LEFT JOIN LATERAL (
			SELECT action
			FROM xcentro.voucher_approvals
			WHERE voucher_id = v.voucher_id
			ORDER BY created_at DESC
			LIMIT 1
		) va ON true
		WHERE v.business_id = $1
	`

	args := []interface{}{businessID}

	// Add status filter if specified
	if status != nil {
		statusCondition := buildStatusCondition(*status)
		query += " AND " + statusCondition
	}

	// Add ordering and pagination (newest vouchers first)
	query += `
		ORDER BY v.created_at DESC
		LIMIT $2 OFFSET $3
	`

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query voucher list: %w", err)
	}
	defer rows.Close()

	// Initialize as empty slice to return [] instead of null when no items
	items := []voucher.VoucherListItem{}
	for rows.Next() {
		var item voucher.VoucherListItem
		var netAmount int64
		var paymentID *string
		var paymentStatus *string
		var paymentDate *time.Time
		var invoiceDueDateDB *time.Time
		var billStatus string
		var approvalStatus string

		err := rows.Scan(
			&item.VoucherID,
			&item.VoucherNumber,
			&item.InvoiceNumber,
			&invoiceDueDateDB,
			&item.PayeeName,
			&netAmount,
			&item.VoucherStatus,
			&paymentID,
			&paymentStatus,
			&paymentDate,
			&billStatus,
			&approvalStatus,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan voucher list item: %w", err)
		}

		// Keep amount in paise (int64)
		item.TotalAmount = netAmount

		// Set invoice due date
		item.InvoiceDueDate = invoiceDueDateDB

		// Set payment date if paid
		item.PaymentDate = paymentDate

		// Map bill status
		item.Status = voucher.BillPaymentStatus(billStatus)

		// Set approval status (empty string if no approval)
		item.ApprovalStatus = approvalStatus

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating voucher list rows: %w", err)
	}

	return items, totalCount, nil
}

// buildStatusCondition builds the SQL condition for status filtering
func buildStatusCondition(status voucher.BillPaymentStatus) string {
	switch status {
	case voucher.BillStatusPaid:
		return "p.payment_id IS NOT NULL AND p.status = 'success'"
	case voucher.BillStatusOverdue:
		return "p.payment_id IS NULL AND v.invoice_due_date IS NOT NULL AND v.invoice_due_date <= CURRENT_DATE"
	case voucher.BillStatusOpen:
		return "p.payment_id IS NULL AND (v.invoice_due_date IS NULL OR v.invoice_due_date > CURRENT_DATE)"
	default:
		return "TRUE" // No filtering
	}
}

// CreateOrUpdateApproval creates a new approval record for a voucher
// Each action creates a new row (audit trail)
func (r *PostgresVoucherRepository) CreateOrUpdateApproval(ctx context.Context, req *voucher.VoucherApprovalRequest) (*voucher.VoucherApprovalResponse, error) {
	// Parse UUIDs
	voucherID, err := uuid.Parse(req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("invalid voucher_id: %w", err)
	}

	requesterUserID, err := uuid.Parse(req.RequesterUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid requester_user_id: %w", err)
	}

	approverUserID, err := uuid.Parse(req.ApproverUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid approver_user_id: %w", err)
	}

	// Always INSERT a new row for each approval action (audit trail)
	approvalID := uuid.Must(uuid.NewV7())
	now := time.Now()

	query := `
		INSERT INTO xcentro.voucher_approvals (
			approval_id, voucher_id, requester_user_id, approver_user_id,
			action, remarks, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING approval_id, created_at
	`

	var createdAt time.Time
	err = r.querier.QueryRow(ctx, query,
		approvalID,
		voucherID,
		requesterUserID,
		approverUserID,
		req.Action,
		req.Remarks,
		now,
	).Scan(&approvalID, &createdAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create approval record: %w", err)
	}

	// Set message based on action
	var message string
	switch req.Action {
	case voucher.ApprovalActionPending:
		message = "Approval request has been submitted"
	case voucher.ApprovalActionApproved:
		message = "Payment has been approved"
	case voucher.ApprovalActionRejected:
		message = "Payment has been rejected"
	case voucher.ApprovalActionRequestChanges:
		message = "Changes have been requested"
	}

	return &voucher.VoucherApprovalResponse{
		Success:    true,
		Message:    message,
		ApprovalID: approvalID.String(),
		VoucherID:  req.VoucherID,
		Action:     req.Action,
		CreatedAt:  createdAt,
	}, nil
}

// GetApprovalInboxVouchers retrieves vouchers for approval inbox filtered by approval status
func (r *PostgresVoucherRepository) GetApprovalInboxVouchers(ctx context.Context, businessID string, status voucher.ApprovalAction) ([]voucher.VoucherApprovalInboxItem, error) {
	// Query strategy:
	// 1. CTE to get latest approval per voucher and filter by status
	// 2. For approved status, apply 7-day date filter
	// 3. Join with vouchers and fetch all approval history with user details
	// 4. Group results by voucher_id in Go code

	query := `
		WITH latest_approvals AS (
			SELECT DISTINCT ON (voucher_id)
				voucher_id,
				action,
				created_at as latest_approval_date
			FROM xcentro.voucher_approvals
			ORDER BY voucher_id, created_at DESC
		),
		filtered_vouchers AS (
			SELECT voucher_id
			FROM latest_approvals
			WHERE action = $2
				AND (
					$2 != 'approved'
					OR latest_approval_date >= CURRENT_DATE - INTERVAL '6 days'
				)
		)
		SELECT
			v.voucher_id,
			v.voucher_number,
			COALESCE(v.invoice_number, '') as invoice_number,
			v.invoice_due_date,
			v.net_amount,
			v.status as voucher_status,
			bpr.display_name as payee_name,
			va.approval_id,
			va.requester_user_id,
			COALESCE(req_user.name, '') as requester_name,
			va.approver_user_id,
			COALESCE(app_user.name, '') as approver_name,
			va.action,
			COALESCE(va.remarks, '') as remarks,
			va.created_at
		FROM filtered_vouchers fv
		INNER JOIN xcentro.payment_vouchers v ON fv.voucher_id = v.voucher_id
		INNER JOIN xcentro.business_payee_relation bpr
			ON v.business_id = bpr.business_id
			AND v.payee_id = bpr.payee_id
			AND v.account_id = bpr.account_id
		INNER JOIN xcentro.voucher_approvals va ON v.voucher_id = va.voucher_id
		LEFT JOIN xcentro.user_table req_user ON va.requester_user_id = req_user.id
		LEFT JOIN xcentro.user_table app_user ON va.approver_user_id = app_user.id
		WHERE v.business_id = $1
		ORDER BY v.created_at DESC, va.created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, businessID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query approval inbox vouchers: %w", err)
	}
	defer rows.Close()

	// Map to group approval history by voucher_id
	voucherMap := make(map[string]*voucher.VoucherApprovalInboxItem)
	voucherOrder := []string{} // To maintain order

	for rows.Next() {
		var voucherID, voucherNumber, invoiceNumber, voucherStatus, payeeName string
		var netAmount int64
		var invoiceDueDateDB *time.Time

		var approvalID, requesterUserID, requesterName, approverUserID, approverName string
		var action voucher.ApprovalAction
		var remarks string
		var createdAt time.Time

		err := rows.Scan(
			&voucherID,
			&voucherNumber,
			&invoiceNumber,
			&invoiceDueDateDB,
			&netAmount,
			&voucherStatus,
			&payeeName,
			&approvalID,
			&requesterUserID,
			&requesterName,
			&approverUserID,
			&approverName,
			&action,
			&remarks,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval inbox row: %w", err)
		}

		// Check if voucher already exists in map
		if _, exists := voucherMap[voucherID]; !exists {
			// First time seeing this voucher, create entry
			voucherMap[voucherID] = &voucher.VoucherApprovalInboxItem{
				VoucherID:             voucherID,
				VoucherNumber:         voucherNumber,
				InvoiceNumber:         invoiceNumber,
				InvoiceDueDate:        invoiceDueDateDB,
				PayeeName:             payeeName,
				TotalAmount:           netAmount, // Keep in paise (int64)
				VoucherStatus:         voucherStatus,
				CurrentApprovalStatus: status, // Latest status from filter
				ApprovalHistory:       []voucher.ApprovalHistoryItem{},
			}
			voucherOrder = append(voucherOrder, voucherID)
		}

		// Append approval history item
		historyItem := voucher.ApprovalHistoryItem{
			ApprovalID:      approvalID,
			RequesterUserID: requesterUserID,
			RequesterName:   requesterName,
			ApproverUserID:  approverUserID,
			ApproverName:    approverName,
			Action:          action,
			Remarks:         remarks,
			CreatedAt:       createdAt,
		}
		voucherMap[voucherID].ApprovalHistory = append(voucherMap[voucherID].ApprovalHistory, historyItem)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval inbox rows: %w", err)
	}

	// Convert map to ordered slice
	result := make([]voucher.VoucherApprovalInboxItem, 0, len(voucherOrder))
	for _, voucherID := range voucherOrder {
		result = append(result, *voucherMap[voucherID])
	}

	return result, nil
}

// GetApprovalInboxCounts retrieves counts for each approval status
func (r *PostgresVoucherRepository) GetApprovalInboxCounts(ctx context.Context, businessID string) (*voucher.VoucherApprovalInboxCounts, error) {
	// Optimized query to get all counts in a single database round-trip
	// Uses CTEs and aggregation for efficiency
	query := `
		WITH latest_approvals AS (
			SELECT DISTINCT ON (voucher_id)
				voucher_id,
				action,
				created_at as latest_approval_date
			FROM xcentro.voucher_approvals
			ORDER BY voucher_id, created_at DESC
		),
		voucher_approvals_with_status AS (
			SELECT
				v.voucher_id,
				la.action,
				la.latest_approval_date
			FROM xcentro.payment_vouchers v
			INNER JOIN latest_approvals la ON v.voucher_id = la.voucher_id
			WHERE v.business_id = $1
		)
		SELECT
			COUNT(*) FILTER (WHERE action = 'pending') as pending_count,
			COUNT(*) FILTER (WHERE action = 'approved' AND latest_approval_date >= CURRENT_DATE - INTERVAL '6 days') as approved_count,
			COUNT(*) FILTER (WHERE action = 'rejected') as rejected_count,
			COUNT(*) FILTER (WHERE action = 'request_changes') as request_changes_count
		FROM voucher_approvals_with_status
	`

	var counts voucher.VoucherApprovalInboxCounts
	err := r.querier.QueryRow(ctx, query, businessID).Scan(
		&counts.Pending,
		&counts.Approved,
		&counts.Rejected,
		&counts.RequestChanges,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get approval inbox counts: %w", err)
	}

	return &counts, nil
}

// BatchApproveVouchers approves multiple vouchers in a single transaction
func (r *PostgresVoucherRepository) BatchApproveVouchers(ctx context.Context, req *voucher.BatchApprovalRequest) (*voucher.BatchApprovalResponse, error) {
	// Start a transaction for atomicity
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	response := &voucher.BatchApprovalResponse{
		TotalRequested: len(req.VoucherIDs),
		Results:        make([]voucher.BatchApprovalResultItem, 0, len(req.VoucherIDs)),
	}

	// Parse approver user ID
	approverUserID, err := uuid.Parse(req.ApprovedByID)
	if err != nil {
		return nil, fmt.Errorf("invalid approved_by_id: %w", err)
	}

	// Parse requester user ID
	requesterUserID, err := uuid.Parse(req.RequesterID)
	if err != nil {
		return nil, fmt.Errorf("invalid requester_id: %w", err)
	}

	now := time.Now()

	// Process each voucher
	for _, voucherIDStr := range req.VoucherIDs {
		result := voucher.BatchApprovalResultItem{
			VoucherID: voucherIDStr,
		}

		// Parse voucher ID
		voucherID, err := uuid.Parse(voucherIDStr)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Invalid voucher ID: %v", err)
			response.Results = append(response.Results, result)
			response.TotalFailed++
			continue
		}

		// Get voucher details (number and verify it exists + belongs to business)
		var voucherNumber string
		var voucherBusinessID string
		var voucherStatus string
		checkQuery := `
			SELECT voucher_number, business_id, status
			FROM xcentro.payment_vouchers
			WHERE voucher_id = $1
		`
		err = tx.QueryRow(ctx, checkQuery, voucherID).Scan(&voucherNumber, &voucherBusinessID, &voucherStatus)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Voucher not found: %v", err)
			response.Results = append(response.Results, result)
			response.TotalFailed++
			continue
		}

		result.VoucherNumber = voucherNumber

		// Verify voucher belongs to the business
		if voucherBusinessID != req.BusinessID {
			result.Success = false
			result.Message = "Voucher does not belong to this business"
			response.Results = append(response.Results, result)
			response.TotalFailed++
			continue
		}

		// Verify voucher is in appropriate status (draft or finalized)
		if voucherStatus != "draft" && voucherStatus != "finalized" {
			result.Success = false
			result.Message = fmt.Sprintf("Cannot approve voucher in status: %s", voucherStatus)
			response.Results = append(response.Results, result)
			response.TotalFailed++
			continue
		}

		// Create approval record (INSERT new row for audit trail)
		approvalID := uuid.Must(uuid.NewV7())
		insertQuery := `
			INSERT INTO xcentro.voucher_approvals (
				approval_id, voucher_id, requester_user_id, approver_user_id,
				action, remarks, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING approval_id
		`

		err = tx.QueryRow(ctx, insertQuery,
			approvalID,
			voucherID,
			requesterUserID,
			approverUserID,
			voucher.ApprovalActionApproved,
			req.Remarks,
			now,
		).Scan(&approvalID)

		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to create approval: %v", err)
			response.Results = append(response.Results, result)
			response.TotalFailed++
			continue
		}

		// Success
		result.Success = true
		result.Message = "Approved successfully"
		result.ApprovalID = approvalID.String()
		response.Results = append(response.Results, result)
		response.TotalApproved++
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Set overall success flag and message
	response.Success = response.TotalApproved > 0
	if response.TotalFailed == 0 {
		response.Message = fmt.Sprintf("Successfully approved all %d vouchers", response.TotalApproved)
	} else if response.TotalApproved == 0 {
		response.Message = fmt.Sprintf("Failed to approve all %d vouchers", response.TotalFailed)
	} else {
		response.Message = fmt.Sprintf("Approved %d out of %d vouchers, %d failed", response.TotalApproved, response.TotalRequested, response.TotalFailed)
	}

	return response, nil
}

// GetUnpaidBills retrieves unpaid bills with role-based filtering
func (r *PostgresVoucherRepository) GetUnpaidBills(ctx context.Context, req *voucher.UnpaidBillsRequest, isOwner bool) ([]voucher.UnpaidBillItem, int, error) {
	// Build query with role-based approval filtering
	query := `
		WITH latest_approvals AS (
			SELECT DISTINCT ON (voucher_id)
				voucher_id,
				action
			FROM xcentro.voucher_approvals
			ORDER BY voucher_id, created_at DESC
		)
		SELECT
			v.voucher_id,
			v.invoice_number,
			v.net_amount AS total_amount,
			v.invoice_due_date,
			CASE
				WHEN v.invoice_due_date < CURRENT_DATE THEN 'overdue'
				ELSE 'open'
			END AS status,
			pb.legal_name AS vendor_name,
			pb.gstin AS vendor_gstin,
			COUNT(*) OVER() AS total_count
		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.business_payee_relation bpr ON v.payee_id = bpr.payee_id
		INNER JOIN xcentro.payee_vendor pb ON bpr.payee_id = pb.payee_id
		LEFT JOIN latest_approvals la ON v.voucher_id = la.voucher_id
		WHERE v.business_id = $1
		  AND v.payment_id IS NULL
		  AND v.status IN ('draft', 'finalized')
	`

	// Add approval filter for non-owner roles
	if !isOwner {
		query += ` AND la.action = 'approved'`
	}

	query += `
		ORDER BY v.invoice_due_date ASC NULLS LAST
		LIMIT $2 OFFSET $3
	`

	// Set default pagination if not provided
	limit := req.Limit
	if limit <= 0 {
		limit = 50 // Default page size
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := r.querier.Query(ctx, query, req.BusinessID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query unpaid bills: %w", err)
	}
	defer rows.Close()

	bills := make([]voucher.UnpaidBillItem, 0)
	var totalCount int

	for rows.Next() {
		var bill voucher.UnpaidBillItem
		var dueDate sql.NullTime
		var status string

		err := rows.Scan(
			&bill.VoucherID,
			&bill.InvoiceNumber,
			&bill.TotalAmount,
			&dueDate,
			&status,
			&bill.VendorName,
			&bill.VendorGSTIN,
			&totalCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan unpaid bill: %w", err)
		}

		if dueDate.Valid {
			bill.DueDate = &dueDate.Time
		}
		bill.Status = voucher.BillPaymentStatus(status)

		bills = append(bills, bill)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating unpaid bills: %w", err)
	}

	return bills, totalCount, nil
}

// GetVoucherCounts retrieves counts of vouchers by payment status
func (r *PostgresVoucherRepository) GetVoucherCounts(ctx context.Context, businessID string) (*voucher.VoucherCountsResponse, error) {
	query := `
		SELECT
			COUNT(*) as all_count,
			COUNT(*) FILTER (WHERE payment_id IS NOT NULL) as paid_count,
			COUNT(*) FILTER (WHERE payment_id IS NULL AND (invoice_due_date IS NULL OR invoice_due_date >= CURRENT_DATE)) as open_count,
			COUNT(*) FILTER (WHERE payment_id IS NULL AND invoice_due_date < CURRENT_DATE) as overdue_count
		FROM xcentro.payment_vouchers
		WHERE business_id = $1
		  AND status IN ('draft', 'finalized', 'posted', 'synced')
	`

	var counts voucher.VoucherCountsResponse
	err := r.querier.QueryRow(ctx, query, businessID).Scan(
		&counts.All,
		&counts.Paid,
		&counts.Open,
		&counts.Overdue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher counts: %w", err)
	}

	return &counts, nil
}

// GetVoucherActivity retrieves complete activity history for a voucher
func (r *PostgresVoucherRepository) GetVoucherActivity(ctx context.Context, voucherID string) (*voucher.VoucherActivityResponse, error) {
	// Query strategy:
	// 1. CTE for voucher creation activity
	// 2. CTE for approval activities with user joins
	// 3. TODO: UNION with payment activities when payment activity table is created
	// 4. ORDER BY timestamp chronologically (oldest first)

	query := `
		WITH voucher_creation AS (
			SELECT
				v.voucher_id::text as activity_id,
				'voucher_created' as activity_type,
				v.created_by_user_id::text as user_id,
				COALESCE(u.name, '') as user_name,
				'created' as action,
				'' as remarks,
				v.created_at as timestamp,
				'' as metadata
			FROM xcentro.payment_vouchers v
			LEFT JOIN xcentro.user_table u ON v.created_by_user_id = u.id
			WHERE v.voucher_id = $1
		),
		approval_activities AS (
			SELECT
				va.approval_id::text as activity_id,
				CASE
					WHEN va.action = 'pending' THEN 'approval_requested'
					ELSE 'approval_action'
				END as activity_type,
				CASE
					WHEN va.action = 'pending' THEN va.requester_user_id::text
					ELSE va.approver_user_id::text
				END as user_id,
				CASE
					WHEN va.action = 'pending' THEN COALESCE(req_user.name, '')
					ELSE COALESCE(app_user.name, '')
				END as user_name,
				va.action::text as action,
				COALESCE(va.remarks, '') as remarks,
				va.created_at as timestamp,
				CASE
					WHEN va.action = 'pending' THEN
						'Requested approval from ' || COALESCE(app_user.name, 'Unknown')
					WHEN va.action = 'approved' THEN 'Approved the voucher'
					WHEN va.action = 'rejected' THEN 'Rejected the voucher'
					WHEN va.action = 'request_changes' THEN 'Requested changes'
					ELSE ''
				END as metadata
			FROM xcentro.voucher_approvals va
			LEFT JOIN xcentro.user_table req_user ON va.requester_user_id = req_user.id
			LEFT JOIN xcentro.user_table app_user ON va.approver_user_id = app_user.id
			WHERE va.voucher_id = $1
		)
		-- TODO: Add payment activities UNION when payment activity table is created
		-- payment_activities AS (
		--     SELECT
		--         pa.payment_activity_id::text as activity_id,
		--         pa.activity_type,
		--         pa.user_id::text,
		--         COALESCE(u.name, '') as user_name,
		--         pa.status as action,
		--         COALESCE(pa.remarks, '') as remarks,
		--         pa.created_at as timestamp,
		--         pa.metadata
		--     FROM xcentro.payment_activities pa
		--     LEFT JOIN xcentro.user_table u ON pa.user_id = u.id
		--     WHERE pa.voucher_id = $1
		-- )
		SELECT
			activity_id,
			activity_type,
			user_id,
			user_name,
			action,
			remarks,
			timestamp,
			metadata
		FROM (
			SELECT * FROM voucher_creation
			UNION ALL
			SELECT * FROM approval_activities
			-- TODO: Uncomment when payment activity table is created
			-- UNION ALL
			-- SELECT * FROM payment_activities
		) combined_activities
		ORDER BY timestamp ASC
	`

	rows, err := r.querier.Query(ctx, query, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to query voucher activity: %w", err)
	}
	defer rows.Close()

	var activities []voucher.VoucherActivityItem
	for rows.Next() {
		var activity voucher.VoucherActivityItem
		err := rows.Scan(
			&activity.ActivityID,
			&activity.ActivityType,
			&activity.UserID,
			&activity.UserName,
			&activity.Action,
			&activity.Remarks,
			&activity.Timestamp,
			&activity.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher activity: %w", err)
		}
		activities = append(activities, activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher activities: %w", err)
	}

	// Get voucher details for response
	var voucherNumber string
	voucherQuery := `SELECT voucher_number FROM xcentro.payment_vouchers WHERE voucher_id = $1`
	err = r.querier.QueryRow(ctx, voucherQuery, voucherID).Scan(&voucherNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("voucher not found: %s", voucherID)
		}
		return nil, fmt.Errorf("failed to get voucher number: %w", err)
	}

	response := &voucher.VoucherActivityResponse{
		VoucherID:     voucherID,
		VoucherNumber: voucherNumber,
		Activities:    activities,
		TotalCount:    len(activities),
	}

	return response, nil
}

// GetPaymentDetailsByVoucher retrieves complete payment details for a voucher
func (r *PostgresVoucherRepository) GetPaymentDetailsByVoucher(ctx context.Context, voucherID string) (*voucher.VoucherPaymentDetails, error) {
	query := `
		SELECT
			-- Payment identification
			p.payment_id::text,
			v.voucher_id::text,
			v.voucher_number,
			p.status::text as payment_status,
			p.created_at as payment_date,
			COALESCE(p.gateway_used::text, '') as gateway_used,
			COALESCE(p.gateway_payment_id, '') as gateway_payment_id,

			-- Payment source details
			p.payment_method::text,
			COALESCE(p.payment_source_type::text, '') as payment_source_type,
			COALESCE(p.payment_source_id::text, '') as payment_source_id,

			-- Card details (from stored_cards)
			COALESCE(sc.card_type::text, '') as card_type,
			COALESCE(sc.card_category::text, '') as card_category,
			COALESCE(sc.last_four_digits, '') as card_last_four,
			COALESCE(sc.card_category::text, '') as card_network,

			-- Bank account details (encrypted, will decrypt in Go)
			COALESCE(ba.account_number_encrypted, '') as bank_account_encrypted,
			COALESCE(ba.bank_name, '') as bank_name,

			-- Payment amounts (in paise)
			p.amount,
			p.fee_amount,
			p.total_amount,

			-- Settlement details
			COALESCE(p.settlement_status::text, 'pending') as settlement_status,
			p.settled_at,
			COALESCE(p.settlement_amount, 0) as settlement_amount,
			COALESCE(p.mdr_amount, 0) as mdr_amount,
			COALESCE(p.net_settlement_amount, 0) as net_settlement_amount,
			COALESCE(p.settlement_reference_id, '') as settlement_reference_id,
			COALESCE(p.settlement_utr, '') as settlement_utr,

			-- Additional metadata
			COALESCE(p.description, '') as description,
			COALESCE(p.metadata, '{}'::jsonb) as metadata

		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.payments p ON v.payment_id = p.payment_id
		LEFT JOIN xcentro.stored_cards sc ON p.payment_source_id = sc.card_id AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba ON p.payment_source_id = ba.account_id AND p.payment_source_type = 'bank_account'
		WHERE v.voucher_id = $1
	`

	var details voucher.VoucherPaymentDetails
	var metadataJSON []byte
	var settledAt sql.NullTime
	var cardLastFour, cardNetwork string
	var bankAccountEncrypted, bankName string

	err := r.querier.QueryRow(ctx, query, voucherID).Scan(
		&details.PaymentID,
		&details.VoucherID,
		&details.VoucherNumber,
		&details.PaymentStatus,
		&details.PaymentDate,
		&details.GatewayUsed,
		&details.GatewayPaymentID,
		&details.PaymentMethod,
		&details.PaymentSourceType,
		&details.PaymentSourceID,
		&details.CardType,
		&details.CardCategory,
		&cardLastFour,
		&cardNetwork,
		&bankAccountEncrypted,
		&bankName,
		&details.Amount,
		&details.FeeAmount,
		&details.TotalAmount,
		&details.SettlementStatus,
		&settledAt,
		&details.SettlementAmount,
		&details.MDRAmount,
		&details.NetSettlementAmount,
		&details.SettlementReferenceID,
		&details.SettlementUTR,
		&details.Description,
		&metadataJSON,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("payment not found for voucher: %s", voucherID)
		}
		return nil, fmt.Errorf("failed to get payment details: %w", err)
	}

	// Handle card vs bank account display info
	if details.PaymentSourceType == "card" {
		details.LastFourDigits = cardLastFour
		details.BankName = cardNetwork // For cards, this is the card network
	} else if details.PaymentSourceType == "bank_account" && bankAccountEncrypted != "" {
		// Decrypt bank account number to get last digits
		decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(bankAccountEncrypted, r.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt bank account for voucher %s: %w", voucherID, err)
		}
		details.LastFourDigits = crypto.ExtractLastDigits(decryptedAccountNumber, 4)
		details.BankName = bankName
	}

	// Handle nullable settled_at
	if settledAt.Valid {
		details.SettledAt = &settledAt.Time
	}

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &details.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &details, nil
}
