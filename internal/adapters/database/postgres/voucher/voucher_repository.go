package voucher

import (
	"context"
	"database/sql"
	"encoding/binary"
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

// WithTx creates a new repository instance that uses a transaction
// This enables transactional voucher operations (e.g., finalizing vouchers atomically with payments)
func (r *PostgresVoucherRepository) WithTx(tx interface{}) voucher.VoucherRepository {
	return &PostgresVoucherRepository{
		querier:       tx.(client.Tx),
		db:            r.db, // Preserve original DB for nested transactions if needed
		encryptionKey: r.encryptionKey,
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

// AcquireVoucherLockAndValidate acquires an advisory lock on voucher and validates it's available for payment
// This prevents concurrent payments for the same voucher (race condition protection)
// Lock is automatically released when transaction commits or rolls back
func (r *PostgresVoucherRepository) AcquireVoucherLockAndValidate(ctx context.Context, voucherID uuid.UUID, businessID string) error {
	// Step 1: Acquire advisory lock using pg_try_advisory_xact_lock
	// This is a transaction-level lock that's automatically released on commit/rollback
	lockKey := hashUUIDToInt64(voucherID)

	query := `SELECT pg_try_advisory_xact_lock($1)`

	var acquired bool
	err := r.querier.QueryRow(ctx, query, lockKey).Scan(&acquired)
	if err != nil {
		return fmt.Errorf("failed to acquire advisory lock: %w", err)
	}

	if !acquired {
		// Lock already held by another transaction - payment in progress
		return voucher.ErrVoucherLocked
	}

	// Step 2: Verify voucher exists, belongs to business, and is in draft status
	validateQuery := `
		SELECT status, business_id
		FROM xcentro.payment_vouchers
		WHERE voucher_id = $1
	`

	var status voucher.VoucherStatus
	var voucherBusinessID string

	err = r.querier.QueryRow(ctx, validateQuery, voucherID).Scan(&status, &voucherBusinessID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return voucher.ErrVoucherNotFound
		}
		return fmt.Errorf("failed to validate voucher: %w", err)
	}

	// Verify business ownership
	if voucherBusinessID != businessID {
		return voucher.ErrVoucherNotFound // Don't leak existence of vouchers from other businesses
	}

	// Verify voucher is in draft status
	if status != voucher.StatusDraft {
		return voucher.ErrVoucherNotDraft
	}

	return nil
}

// hashUUIDToInt64 converts UUID to int64 for PostgreSQL advisory lock
// Uses first 8 bytes of UUID as the lock key
func hashUUIDToInt64(id uuid.UUID) int64 {
	bytes := id[:]
	// Use first 8 bytes and convert to int64
	return int64(binary.BigEndian.Uint64(bytes[:8]))
}

// ValidateAndSumVouchers validates vouchers and returns their total net amount in a single query
// This method atomically:
// - Validates all vouchers exist
// - Validates all belong to the specified business
// - Validates all are in draft status
// - Calculates the sum of net_amount
// Returns: (totalAmount, validVoucherCount, error)
func (r *PostgresVoucherRepository) ValidateAndSumVouchers(ctx context.Context, voucherIDs []uuid.UUID, businessID string) (int64, int, error) {
	query := `
		SELECT
			COUNT(*) as voucher_count,
			COALESCE(SUM(net_amount), 0) as total_amount,
			COUNT(CASE WHEN business_id != $1 THEN 1 END) as wrong_business_count,
			COUNT(CASE WHEN status != 'draft' THEN 1 END) as non_draft_count
		FROM xcentro.payment_vouchers
		WHERE voucher_id = ANY($2)
	`

	var count, wrongBusinessCount, nonDraftCount int
	var totalAmount int64

	err := r.querier.QueryRow(ctx, query, businessID, voucherIDs).Scan(
		&count, &totalAmount, &wrongBusinessCount, &nonDraftCount,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to validate vouchers: %w", err)
	}

	// Validate all vouchers were found
	if count != len(voucherIDs) {
		return 0, 0, fmt.Errorf("not all vouchers found (expected %d, found %d)", len(voucherIDs), count)
	}

	// Validate all belong to business
	if wrongBusinessCount > 0 {
		return 0, 0, fmt.Errorf("%d voucher(s) do not belong to this business", wrongBusinessCount)
	}

	// Validate all are in draft status
	if nonDraftCount > 0 {
		return 0, 0, fmt.Errorf("%d voucher(s) are not in draft status", nonDraftCount)
	}

	return totalAmount, count, nil
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
		       payment_terms,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
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
		       payment_terms,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
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
		       payment_terms,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
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

// GetVouchersByPaymentIDWithStatus retrieves vouchers for a payment based on payment status
// Handles both finalized (payment_id set) and pending (payment_voucher_pending table) states
func (r *PostgresVoucherRepository) GetVouchersByPaymentIDWithStatus(ctx context.Context, paymentID uuid.UUID, paymentStatus string) ([]*voucher.Voucher, error) {
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
		       payment_terms,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
		       gl_expense_account, gl_bank_account, cost_center,
		       created_at, created_by_user_id, updated_at, metadata
		FROM xcentro.payment_vouchers
		WHERE CASE
			WHEN $2 = 'success' THEN
				-- For successful payments: query by payment_id (finalized vouchers)
				payment_id = $1
			ELSE
				-- For pending/failed payments: query from payment_voucher_pending table
				voucher_id IN (
					SELECT voucher_id
					FROM xcentro.payment_voucher_pending
					WHERE payment_id = $1
				)
		END
		ORDER BY voucher_number
	`

	rows, err := r.querier.Query(ctx, query, paymentID, paymentStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouchers by payment_id and status: %w", err)
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

// GetVouchersWithPayeeDetailsByPaymentID retrieves vouchers with payee details in a single JOIN query
// This avoids N+1 problem by joining with business_payee_relation and payee_master tables
// Handles both finalized (success) and pending (pending/failed) payment states
func (r *PostgresVoucherRepository) GetVouchersWithPayeeDetailsByPaymentID(ctx context.Context, paymentID uuid.UUID, paymentStatus string) ([]*voucher.VoucherWithPayeeDetails, error) {
	query := `
		SELECT
			v.voucher_id,
			v.voucher_number,
			v.invoice_number,
			v.net_amount,
			bpr.relation_id,
			bpr.display_name,
			pm.payee_type,
			bpr.type_specific_data
		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.business_payee_relation bpr
			ON v.payee_id = bpr.payee_id
			AND v.account_id = bpr.account_id
			AND v.business_id = bpr.business_id
		INNER JOIN xcentro.payee_master pm
			ON v.payee_id = pm.payee_id
		WHERE CASE
			WHEN $2 = 'success' THEN
				-- For successful payments: query by payment_id (finalized vouchers)
				v.payment_id = $1
			ELSE
				-- For pending/failed payments: query from payment_voucher_pending table
				v.voucher_id IN (
					SELECT voucher_id
					FROM xcentro.payment_voucher_pending
					WHERE payment_id = $1
				)
		END
		ORDER BY v.voucher_number
	`

	rows, err := r.querier.Query(ctx, query, paymentID, paymentStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to query vouchers with payee details: %w", err)
	}
	defer rows.Close()

	var vouchersWithPayee []*voucher.VoucherWithPayeeDetails
	for rows.Next() {
		var v voucher.VoucherWithPayeeDetails
		var typeSpecificDataJSON []byte

		err := rows.Scan(
			&v.VoucherID,
			&v.VoucherNumber,
			&v.InvoiceNumber,
			&v.NetAmount,
			&v.RelationID,
			&v.DisplayName,
			&v.PayeeType,
			&typeSpecificDataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan voucher with payee details: %w", err)
		}

		// Parse type_specific_data JSONB
		if len(typeSpecificDataJSON) > 0 {
			var typeSpecificData map[string]interface{}
			if err := json.Unmarshal(typeSpecificDataJSON, &typeSpecificData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal type_specific_data: %w", err)
			}
			v.TypeSpecificData = typeSpecificData
		}

		vouchersWithPayee = append(vouchersWithPayee, &v)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating voucher with payee rows: %w", err)
	}

	return vouchersWithPayee, nil
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
	var financialDetailsJSON, attachmentsJSON, erpMetadataJSON, metadataJSON []byte

	// Nullable string fields
	var invoiceNumber, expenseCategory, description sql.NullString
	var paymentTerms sql.NullString
	var tdsSection sql.NullString
	var erpReferenceID sql.NullString
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
		&paymentTerms,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &erpReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
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
	if tdsSection.Valid {
		v.TDSSection = tdsSection.String
	}
	if erpReferenceID.Valid {
		v.ERPReferenceID = erpReferenceID.String
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
	var financialDetailsJSON, attachmentsJSON, erpMetadataJSON, metadataJSON []byte

	// Nullable string fields
	var paymentTerms sql.NullString
	var tdsSection sql.NullString
	var erpReferenceID sql.NullString
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
		&paymentTerms,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &erpReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
		&glExpenseAccount, &glBankAccount, &costCenter,
		&v.CreatedAt, &v.CreatedByID, &v.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable strings
	if paymentTerms.Valid {
		v.PaymentTerms = paymentTerms.String
	}
	if tdsSection.Valid {
		v.TDSSection = tdsSection.String
	}
	if erpReferenceID.Valid {
		v.ERPReferenceID = erpReferenceID.String
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
		       payment_terms,
		       status, payment_id,
		       erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, erp_metadata,
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
				WHEN p.payment_id IS NULL AND v.invoice_due_date IS NOT NULL AND v.invoice_due_date < CURRENT_DATE THEN 'overdue'
				WHEN p.payment_id IS NULL AND (v.invoice_due_date IS NULL OR v.invoice_due_date >= CURRENT_DATE) THEN 'open'
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

		// Set payment status (null if no payment)
		item.PaymentStatus = paymentStatus

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
		return "p.payment_id IS NULL AND v.invoice_due_date IS NOT NULL AND v.invoice_due_date < CURRENT_DATE"
	case voucher.BillStatusOpen:
		return "p.payment_id IS NULL AND (v.invoice_due_date IS NULL OR v.invoice_due_date >= CURRENT_DATE)"
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

	// Fetch the most recent approval state BEFORE creating the new one
	var previousApprovalState voucher.ApprovalAction
	var previousApproverID string

	previousApprovalQuery := `
		SELECT action, approver_user_id
		FROM xcentro.voucher_approvals
		WHERE voucher_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var prevApproverUUID uuid.UUID
	err = r.querier.QueryRow(ctx, previousApprovalQuery, voucherID).Scan(&previousApprovalState, &prevApproverUUID)
	if err == nil {
		// Previous approval found - populate the approver ID
		previousApproverID = prevApproverUUID.String()
	}
	// If err == pgx.ErrNoRows, it means this is a fresh voucher (no previous approvals)
	// previousApprovalState and previousApproverID remain empty (zero values)

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
		Success:               true,
		Message:               message,
		ApprovalID:            approvalID.String(),
		VoucherID:             req.VoucherID,
		Action:                req.Action,
		CreatedAt:             createdAt,
		PreviousApprovalState: previousApprovalState,
		PreviousApproverID:    previousApproverID,
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
			bpr.display_name AS vendor_name,
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
		ORDER BY
			CASE WHEN v.invoice_due_date < CURRENT_DATE THEN 0 ELSE 1 END,
			v.invoice_due_date ASC NULLS LAST
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
	// 3. CTE for payment state changes with user-friendly action mapping
	// 4. ORDER BY timestamp chronologically (oldest first)

	query := `
		WITH voucher_creation AS (
			SELECT
				v.voucher_id::text as activity_id,
				v.created_by_user_id::text as user_id,
				COALESCE(u.name, '') as user_name,
				'Voucher Created' as action,
				v.created_at as timestamp
			FROM xcentro.payment_vouchers v
			LEFT JOIN xcentro.user_table u ON v.created_by_user_id = u.id
			WHERE v.voucher_id = $1
		),
		approval_activities AS (
			SELECT
				va.approval_id::text as activity_id,
				CASE
					WHEN va.action = 'pending' THEN va.requester_user_id::text
					ELSE va.approver_user_id::text
				END as user_id,
				CASE
					WHEN va.action = 'pending' THEN COALESCE(req_user.name, '')
					ELSE COALESCE(app_user.name, '')
				END as user_name,
				CASE
					WHEN va.action = 'pending' THEN 'Approval Requested'
					WHEN va.action = 'approved' THEN 'Approved'
					WHEN va.action = 'rejected' THEN 'Rejected'
					WHEN va.action = 'request_changes' THEN 'Changes Requested'
					WHEN va.action = 'edited' THEN 'Voucher Edited'
					ELSE va.action::text
				END as action,
				va.created_at as timestamp
			FROM xcentro.voucher_approvals va
			LEFT JOIN xcentro.user_table req_user ON va.requester_user_id = req_user.id
			LEFT JOIN xcentro.user_table app_user ON va.approver_user_id = app_user.id
			WHERE va.voucher_id = $1
		),
		payment_activities AS (
			SELECT
				psc.state_change_id::text as activity_id,
				COALESCE(psc.actor_user_id::text, '') as user_id,
				-- Only show user name for actual users (payment_initiated), hide internal actor names (webhooks, system)
				CASE
					WHEN psc.actor_type = 'user' AND psc.actor_user_id IS NOT NULL THEN COALESCE(u.name, '')
					ELSE ''
				END as user_name,
				-- Map internal payment actions to user-friendly strings
				CASE
					-- Payment lifecycle: Each state has distinct label
					WHEN psc.action = 'payment_initiated' THEN 'Payment Initiated'
					WHEN psc.action = 'payment_processing' THEN 'Payment Processing'
					WHEN psc.action = 'payment_success' THEN 'Payment Successful'
					-- Payment failures
					WHEN psc.action = 'payment_failed' THEN 'Payment Failed'
					WHEN psc.action = 'payment_refunded' THEN 'Payment Refunded'
					-- Payout lifecycle: In-progress → "Payment Processing"
					WHEN psc.action IN ('payout_initiated', 'payout_processing') THEN 'Payment Processing'
					-- Payout completed → Final success
					WHEN psc.action = 'payout_completed' THEN 'Payment Completed to Payee'
					-- Payout failures
					WHEN psc.action = 'payout_failed' THEN 'Payout Failed'
					WHEN psc.action = 'payout_timeout' THEN 'Payout Under Investigation'
					WHEN psc.action = 'payout_cancelled' THEN 'Payout Cancelled'
					ELSE psc.action::text
				END as action,
				psc.action_timestamp as timestamp
			FROM xcentro.payment_vouchers v
			INNER JOIN xcentro.payment_state_changes psc ON psc.payment_id = v.payment_id
			LEFT JOIN xcentro.user_table u ON psc.actor_user_id = u.id
			WHERE v.voucher_id = $1
			  -- Exclude settlement lifecycle (internal step, not user-facing)
			  AND psc.action NOT IN ('settlement_pending', 'settlement_received', 'settlement_failed')
			  -- Exclude payout_initiated to avoid duplicate "Payment Processing" entries
			  -- (both payout_initiated and payout_processing map to same display text,
			  --  but payout_processing is more meaningful as it happens after API success)
			  AND psc.action != 'payout_initiated'
		)
		SELECT
			activity_id,
			user_id,
			user_name,
			action,
			timestamp
		FROM (
			SELECT * FROM voucher_creation
			UNION ALL
			SELECT * FROM approval_activities
			UNION ALL
			SELECT * FROM payment_activities
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
			&activity.UserID,
			&activity.UserName,
			&activity.Action,
			&activity.Timestamp,
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

			-- Payment source details
			p.payment_method::text,

			-- Card details (from stored_cards)
			COALESCE(sc.card_type::text, '') as card_type,
			COALESCE(sc.card_category::text, '') as card_category,
			COALESCE(sc.last_four_digits, '') as card_last_four,
			COALESCE(sc.issuer_bank, '') as card_issuer_bank,

			-- Bank account details (encrypted, will decrypt in Go)
			COALESCE(ba.account_number_encrypted, '') as bank_account_encrypted,
			COALESCE(ba.bank_name, '') as bank_name,

			-- Payment amounts (in paise)
			p.amount,
			p.fee_amount,
			p.convenience_fee,
			p.convenience_fee_gst,
			p.total_amount,

			-- Payout details (from payout_transfer_requests)
			COALESCE(ptr.utr, '') as payout_utr,
			ptr.completed_at as payout_completed_at,

			-- Additional metadata
			COALESCE(p.description, '') as description

		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.payments p ON v.payment_id = p.payment_id
		LEFT JOIN xcentro.stored_cards sc ON p.payment_source_id = sc.card_id AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba ON p.payment_source_id = ba.account_id AND p.payment_source_type = 'bank_account'
		LEFT JOIN xcentro.payout_transfer_requests ptr ON v.voucher_id = ptr.voucher_id
		WHERE v.voucher_id = $1
	`

	var details voucher.VoucherPaymentDetails
	var cardLastFour, cardIssuerBank string
	var bankAccountEncrypted, bankName string
	var payoutCompletedAt sql.NullTime

	err := r.querier.QueryRow(ctx, query, voucherID).Scan(
		&details.PaymentID,
		&details.VoucherID,
		&details.VoucherNumber,
		&details.PaymentStatus,
		&details.PaymentDate,
		&details.PaymentMethod,
		&details.CardType,
		&details.CardCategory,
		&cardLastFour,
		&cardIssuerBank,
		&bankAccountEncrypted,
		&bankName,
		&details.Amount,
		&details.FeeAmount,
		&details.ConvenienceFee,
		&details.ConvenienceFeeGST,
		&details.TotalAmount,
		&details.PayoutUTR,
		&payoutCompletedAt,
		&details.Description,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("payment not found for voucher: %s", voucherID)
		}
		return nil, fmt.Errorf("failed to get payment details: %w", err)
	}

	// Handle card vs bank account display info
	if details.PaymentMethod == "card" || details.PaymentMethod == "token_card" {
		details.LastFourDigits = cardLastFour
		details.BankName = cardIssuerBank // For cards, this is the issuer bank (HDFC, SBI, etc.)
	} else if (details.PaymentMethod == "net_banking" || details.PaymentMethod == "upi") && bankAccountEncrypted != "" {
		// Decrypt bank account number to get last digits
		decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(bankAccountEncrypted, r.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt bank account for voucher %s: %w", voucherID, err)
		}
		details.LastFourDigits = crypto.ExtractLastDigits(decryptedAccountNumber, 4)
		details.BankName = bankName
	}

	// Handle nullable payout completed_at
	if payoutCompletedAt.Valid {
		details.PayoutCompletedAt = &payoutCompletedAt.Time
	}

	return &details, nil
}

// Delete deletes a voucher by ID
// Only draft vouchers can be deleted (business logic enforced at service layer)
// Cascades to voucher_approvals and erp_sync_queue automatically
// Fails if payout_transfer_requests exist (ON DELETE RESTRICT constraint)
func (r *PostgresVoucherRepository) Delete(ctx context.Context, voucherID uuid.UUID) error {
	// Check for payout transfers first (better error message than constraint violation)
	var payoutCount int
	checkQuery := `
		SELECT COUNT(*)
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1
	`

	err := r.querier.QueryRow(ctx, checkQuery, voucherID).Scan(&payoutCount)
	if err != nil {
		return fmt.Errorf("failed to check payout transfers: %w", err)
	}

	if payoutCount > 0 {
		return fmt.Errorf("cannot delete voucher: %d active payout transfer(s) exist", payoutCount)
	}

	// Delete voucher (cascades to approvals and sync queue)
	deleteQuery := `
		DELETE FROM xcentro.payment_vouchers
		WHERE voucher_id = $1
	`

	result, err := r.querier.Exec(ctx, deleteQuery, voucherID)
	if err != nil {
		return fmt.Errorf("failed to delete voucher: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("voucher not found")
	}

	return nil
}

// GetLatestApprovalStatus retrieves the latest approval status for a voucher
// Returns nil if no approvals exist
func (r *PostgresVoucherRepository) GetLatestApprovalStatus(ctx context.Context, voucherID uuid.UUID) (*string, error) {
	query := `
		SELECT action
		FROM xcentro.voucher_approvals
		WHERE voucher_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var action string
	err := r.querier.QueryRow(ctx, query, voucherID).Scan(&action)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No approvals exist
		}
		return nil, fmt.Errorf("failed to get latest approval status: %w", err)
	}

	return &action, nil
}

// GetPaymentStatus retrieves the payment status for a voucher from the payments table
// Returns nil if voucher has no associated payment
func (r *PostgresVoucherRepository) GetPaymentStatus(ctx context.Context, voucherID uuid.UUID) (*string, error) {
	query := `
		SELECT p.status
		FROM xcentro.payments p
		INNER JOIN xcentro.payment_vouchers pv ON p.payment_id = pv.payment_id
		WHERE pv.voucher_id = $1
	`

	var status string
	err := r.querier.QueryRow(ctx, query, voucherID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No payment associated
		}
		return nil, fmt.Errorf("failed to get payment status: %w", err)
	}

	return &status, nil
}

// GetCalendarViewBills retrieves bills for a specific month and year based on invoice_due_date
// Used for calendar view to show bills month by month
func (r *PostgresVoucherRepository) GetCalendarViewBills(ctx context.Context, businessID string, year, month int) ([]voucher.CalendarBillItem, error) {
	query := `
		SELECT
			v.voucher_id::text,
			v.voucher_number,
			COALESCE(v.invoice_number, '') as invoice_number,
			v.invoice_due_date,
			v.gross_amount,
			v.net_amount,
			v.payee_id::text,
			v.account_id::text,
			bpr.display_name as payee_name,
			COALESCE(v.description, '') as description,
			v.status::text as voucher_status,
			p.status as payment_status
		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.business_payee_relation bpr
			ON v.business_id = bpr.business_id
			AND v.payee_id = bpr.payee_id
			AND v.account_id = bpr.account_id
		LEFT JOIN xcentro.payments p ON v.payment_id = p.payment_id
		WHERE v.business_id = $1::uuid
			AND EXTRACT(YEAR FROM v.invoice_due_date) = $2
			AND EXTRACT(MONTH FROM v.invoice_due_date) = $3
			AND v.invoice_due_date IS NOT NULL
		ORDER BY v.invoice_due_date ASC
	`

	businessUUID, err := uuid.Parse(businessID)
	if err != nil {
		return nil, fmt.Errorf("invalid business_id: %w", err)
	}

	rows, err := r.querier.Query(ctx, query, businessUUID, year, month)
	if err != nil {
		return nil, fmt.Errorf("failed to query calendar view bills: %w", err)
	}
	defer rows.Close()

	bills := []voucher.CalendarBillItem{}
	for rows.Next() {
		var bill voucher.CalendarBillItem
		var dueDateDB *time.Time
		var paymentStatus sql.NullString

		err := rows.Scan(
			&bill.VoucherID,
			&bill.VoucherNumber,
			&bill.InvoiceNumber,
			&dueDateDB,
			&bill.GrossAmount,
			&bill.NetAmount,
			&bill.PayeeID,
			&bill.AccountID,
			&bill.PayeeName,
			&bill.Description,
			&bill.VoucherStatus,
			&paymentStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan calendar bill row: %w", err)
		}

		bill.DueDate = dueDateDB
		if paymentStatus.Valid {
			bill.PaymentStatus = &paymentStatus.String
		}

		bills = append(bills, bill)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating calendar bill rows: %w", err)
	}

	return bills, nil
}

// CreateVoucherDocument inserts a new document record into the voucher_documents table
func (r *PostgresVoucherRepository) CreateVoucherDocument(ctx context.Context, doc *voucher.VoucherDocument) error {
	query := `
		INSERT INTO xcentro.voucher_documents (
			id, s3_path, doc_type, status, file_hash
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`

	_, err := r.querier.Exec(ctx, query,
		doc.ID, doc.S3Path, doc.DocType, doc.Status, doc.FileHash,
	)
	if err != nil {
		return fmt.Errorf("failed to create voucher document: %w", err)
	}

	return nil
}

// GetDocumentByFileHash looks up a document by its SHA-256 file hash.
// Returns nil, nil when no match is found.
func (r *PostgresVoucherRepository) GetDocumentByFileHash(ctx context.Context, fileHash string) (*voucher.VoucherDocument, error) {
	query := `
		SELECT id, s3_path, doc_type, status, COALESCE(file_hash, '')
		FROM xcentro.voucher_documents
		WHERE file_hash = $1
		LIMIT 1
	`

	var doc voucher.VoucherDocument
	var docType sql.NullString
	err := r.querier.QueryRow(ctx, query, fileHash).Scan(
		&doc.ID, &doc.S3Path, &docType, &doc.Status, &doc.FileHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to lookup document by file hash: %w", err)
	}
	if docType.Valid {
		doc.DocType = docType.String
	}
	return &doc, nil
}

// GetVoucherDocumentByID retrieves a voucher document by its ID
func (r *PostgresVoucherRepository) GetVoucherDocumentByID(ctx context.Context, docID uuid.UUID) (*voucher.VoucherDocument, error) {
	query := `
		SELECT id, s3_path, doc_type, status, raw_ocr, draft_data, i2d_template, processed_by, validated_data, created_at
		FROM xcentro.voucher_documents
		WHERE id = $1
	`

	var doc voucher.VoucherDocument
	var docType sql.NullString
	var draftData, validatedData []byte
	var i2dTemplate, processedBy, rawOCR sql.NullString

	err := r.querier.QueryRow(ctx, query, docID).Scan(
		&doc.ID,
		&doc.S3Path,
		&docType,
		&doc.Status,
		&rawOCR,
		&draftData,
		&i2dTemplate,
		&processedBy,
		&validatedData,
		&doc.UploadedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("voucher document not found: %s", docID)
		}
		return nil, fmt.Errorf("failed to get voucher document: %w", err)
	}

	if docType.Valid {
		doc.DocType = docType.String
	}
	if i2dTemplate.Valid {
		doc.I2DTemplate = i2dTemplate.String
	}
	if processedBy.Valid {
		doc.ProcessedBy = processedBy.String
	}
	if rawOCR.Valid {
		doc.RawOCR = rawOCR.String
	}
	if draftData != nil {
		if err := json.Unmarshal(draftData, &doc.DraftData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal draft_data: %w", err)
		}
	}
	if validatedData != nil {
		if err := json.Unmarshal(validatedData, &doc.ValidatedData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal validated_data: %w", err)
		}
	}

	return &doc, nil
}

// UpdateVoucherDocumentValidation updates the validated data and status of a voucher document
func (r *PostgresVoucherRepository) UpdateVoucherDocumentValidation(ctx context.Context, docID uuid.UUID, status voucher.DocumentStatus, validatedData map[string]interface{}) error {
	validatedJSON, err := json.Marshal(validatedData)
	if err != nil {
		return fmt.Errorf("failed to marshal validated_data: %w", err)
	}

	query := `
		UPDATE xcentro.voucher_documents
		SET status = $2, validated_data = $3
		WHERE id = $1
	`

	result, err := r.querier.Exec(ctx, query, docID, status, validatedJSON)
	if err != nil {
		return fmt.Errorf("failed to update voucher document validation: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("voucher document not found: %s", docID)
	}

	return nil
}

// CreateInvoiceFromExtraction persists validated invoice data to the existing invoice table
// and inserts line items into the line_items table within a transaction.
// Returns the generated invoice UUID.
func (r *PostgresVoucherRepository) CreateInvoiceFromExtraction(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Compute derived fields
	totalAmount := toFloat64(data["total_amount"])
	tds := toFloat64(data["tds_amount"])
	discount := toFloat64(data["discount"])
	netAmount := totalAmount - tds - discount
	if netAmount < 0 {
		netAmount = totalAmount // fallback if no deductions extracted
	}

	// Derive gst_type from tax components
	igst := toFloat64(data["igst"])
	sgst := toFloat64(data["sgst"])
	cgst := toFloat64(data["cgst"])
	gstType := ""
	if igst > 0 {
		gstType = "INTER_STATE"
	} else if sgst > 0 || cgst > 0 {
		gstType = "INTRA_STATE"
	}

	// 2. Insert into invoice table and return generated ID
	invoiceQuery := `
		INSERT INTO xcentro.invoice (
			document_id, inv_id, type, bid, payee_id,
			inv_date, inv_due_date, total_amount, net_amount, po_number,
			sgst, cgst, igst,
			vendor_gstin, buyer_gstin,
			match_status,
			gst_type, subtotal, gross_amount, discount,
			payment_terms, expense_category
		) VALUES (
			$1, $2, 'PURCHASE'::xcentro.invoice_type_enum,
			$3, $4,
			NULLIF($5, '')::DATE, NULLIF($6, '')::DATE, $7, $8, $9,
			$10, $11, $12,
			$13, $14,
			$15,
			NULLIF($16, ''), $17, $18, $19,
			NULLIF($20, ''), NULLIF($21, '')
		) RETURNING id
	`

	var invDate interface{} = data["invoice_date"]
	if invDateStr, ok := invDate.(string); ok && invDateStr == "" {
		invDate = nil
	}
	var invDueDate interface{} = data["invoice_due_date"]
	if invDueDateStr, ok := invDueDate.(string); ok && invDueDateStr == "" {
		invDueDate = nil
	}

	poNumber, _ := data["po_number"].(string)
	paymentTerms, _ := data["payment_terms"].(string)
	expenseCategory, _ := data["expense_category"].(string)

	var invoiceID uuid.UUID
	err = tx.QueryRow(ctx, invoiceQuery,
		docID,
		data["invoice_number"],
		parseOptionalUUID(data["business_id"]),
		parseOptionalUUID(data["payee_id"]),
		invDate,
		invDueDate,
		totalAmount,
		netAmount,
		poNumber,
		data["sgst"],
		data["cgst"],
		data["igst"],
		optionalString(data["vendor_gstin"]),
		optionalString(data["buyer_gstin"]),
		voucher.InvoiceMatchPending,
		gstType,
		data["subtotal"],
		data["gross_amount"],
		discount,
		paymentTerms,
		expenseCategory,
	).Scan(&invoiceID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create invoice from extraction: %w", err)
	}

	// 3. Insert line items
	if err := insertLineItems(ctx, tx, nil, &invoiceID, data); err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert invoice line items: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return invoiceID, nil
}

// CreatePurchaseOrder persists validated PO data to the purchase_orders table
// and inserts line items into the line_items table within a transaction.
// Returns the generated PO UUID.
func (r *PostgresVoucherRepository) CreatePurchaseOrder(ctx context.Context, docID uuid.UUID, data map[string]interface{}) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// 1. Insert into purchase_orders table and return generated ID
	poQuery := `
		INSERT INTO xcentro.purchase_orders (
			document_id, bid, payee_id,
			po_number, po_date,
			vendor_name, vendor_gstin, buyer_gstin,
			total_amount, discount, discount_type,
			remaining_balance, po_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`

	totalAmount := toFloat64(data["total_amount"])

	var poID uuid.UUID
	err = tx.QueryRow(ctx, poQuery,
		docID,
		parseOptionalUUID(data["business_id"]),
		parseOptionalUUID(data["payee_id"]),
		data["po_number"],
		data["invoice_date"], // po_date maps from invoice_date in draft_data
		data["vendor_name"],
		data["vendor_gstin"],
		data["buyer_gstin"],
		totalAmount,
		data["discount"],
		data["discount_type"],
		totalAmount, // remaining_balance starts equal to total_amount
		voucher.POStatusOpen,
	).Scan(&poID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create purchase order: %w", err)
	}

	// 2. Insert line items
	if err := insertLineItems(ctx, tx, &poID, nil, data); err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert purchase order line items: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return poID, nil
}

// insertLineItems inserts line items from the data map into the line_items table.
// Exactly one of poID or invoiceID must be non-nil.
func insertLineItems(ctx context.Context, tx client.Tx, poID *uuid.UUID, invoiceID *uuid.UUID, data map[string]interface{}) error {
	rawItems, ok := data["line_items"]
	if !ok || rawItems == nil {
		return nil // no line items to insert
	}

	items, ok := rawItems.([]interface{})
	if !ok {
		return nil // unexpected shape, skip gracefully
	}

	lineItemQuery := `
		INSERT INTO xcentro.line_items (
			purchase_order_id, invoice_id,
			sku, description, quantity, unit_price,
			tax_rate, tax_amount, total_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		sku, _ := item["sku"].(string)
		description, _ := item["description"].(string)
		quantity := toFloat64(item["quantity"])
		// Frontend sends "rate" (unit price), "gst_rate" (tax %), "gst_amount" (tax in rupees), "amount" (pre-tax subtotal)
		unitPrice := toPaise(item["rate"])
		taxRate := toFloat64(item["gst_rate"])
		taxAmount := toPaise(item["gst_amount"])
		// total_amount = pre-tax amount + GST amount
		totalAmount := toPaise(item["amount"]) + toPaise(item["gst_amount"])

		_, err := tx.Exec(ctx, lineItemQuery,
			poID,
			invoiceID,
			sku,
			description,
			quantity,
			unitPrice,
			taxRate,
			taxAmount,
			totalAmount,
		)
		if err != nil {
			return fmt.Errorf("failed to insert line item: %w", err)
		}
	}

	return nil
}

// toFloat64 safely converts an interface{} to float64.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// toPaise converts a float64 amount (in main currency unit) to paise (int64).
func toPaise(v interface{}) int64 {
	return int64(toFloat64(v) * 100)
}

// parseOptionalUUID safely extracts a UUID from an interface{} value.
// Returns nil if the value is nil, empty, or not a valid UUID string.
func parseOptionalUUID(v interface{}) *uuid.UUID {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &parsed
}

// optionalString safely extracts a string from an interface{} value.
// Returns nil if the value is nil or not a string.
func optionalString(v interface{}) *string {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

// ListVoucherDocuments retrieves paginated list of voucher documents
func (r *PostgresVoucherRepository) ListVoucherDocuments(ctx context.Context, filter voucher.DocumentListFilter) ([]*voucher.VoucherDocument, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.Status != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argPos))
		args = append(args, *filter.Status)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.voucher_documents WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count voucher documents: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, s3_path, COALESCE(doc_type, ''), status, raw_ocr, draft_data, COALESCE(i2d_template, ''), COALESCE(processed_by, ''), validated_data, created_at
		FROM xcentro.voucher_documents
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query voucher documents: %w", err)
	}
	defer rows.Close()

	var docs []*voucher.VoucherDocument
	for rows.Next() {
		var doc voucher.VoucherDocument
		var rawOCR sql.NullString
		var draftData, validatedData []byte

		err := rows.Scan(
			&doc.ID, &doc.S3Path, &doc.DocType, &doc.Status,
			&rawOCR, &draftData, &doc.I2DTemplate, &doc.ProcessedBy, &validatedData,
			&doc.UploadedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan voucher document: %w", err)
		}

		if rawOCR.Valid {
			doc.RawOCR = rawOCR.String
		}
		if len(draftData) > 0 {
			json.Unmarshal(draftData, &doc.DraftData)
		}
		if len(validatedData) > 0 {
			json.Unmarshal(validatedData, &doc.ValidatedData)
		}

		docs = append(docs, &doc)
	}

	return docs, total, nil
}

// ListExtractedInvoices retrieves paginated list of extracted invoices
func (r *PostgresVoucherRepository) ListExtractedInvoices(ctx context.Context, filter voucher.ExtractedInvoiceListFilter) ([]*voucher.ExtractedInvoice, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.invoice WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count extracted invoices: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(inv_id, ''), COALESCE(type::text, ''), bid, payment_id, payee_id, inv_date, inv_due_date, COALESCE(cgst, 0), COALESCE(igst, 0), COALESCE(sgst, 0), COALESCE(total_amount, 0), COALESCE(net_amount, 0), COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'), COALESCE(currency, ''), COALESCE(invoice_url, ''), COALESCE(comments, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''), document_id, created_at, updated_at
		FROM xcentro.invoice
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query extracted invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*voucher.ExtractedInvoice
	for rows.Next() {
		var inv voucher.ExtractedInvoice
		err := rows.Scan(
			&inv.ID, &inv.InvID, &inv.Type, &inv.BusinessID, &inv.PaymentID, &inv.PayeeID,
			&inv.InvDate, &inv.InvDueDate, &inv.CGST, &inv.IGST, &inv.SGST, &inv.TotalAmount,
			&inv.NetAmount, &inv.PONumber, &inv.MatchStatus,
			&inv.Currency, &inv.InvoiceURL, &inv.Comments, &inv.VendorGSTIN, &inv.BuyerGSTIN, &inv.DocumentID, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan extracted invoice: %w", err)
		}
		invoices = append(invoices, &inv)
	}

	return invoices, total, nil
}

// ListPurchaseOrders retrieves paginated list of purchase orders
func (r *PostgresVoucherRepository) ListPurchaseOrders(ctx context.Context, filter voucher.PurchaseOrderListFilter) ([]*voucher.PurchaseOrder, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.DocumentID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("document_id = $%d", argPos))
		args = append(args, *filter.DocumentID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM xcentro.purchase_orders WHERE %s", whereClause)
	var total int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count purchase orders: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT id, document_id, bid, payee_id, COALESCE(po_number, ''), COALESCE(po_date, ''), COALESCE(vendor_name, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''), COALESCE(total_amount, 0), COALESCE(remaining_balance, 0), COALESCE(po_status, 'OPEN'), COALESCE(discount, 0), COALESCE(discount_type, ''), created_at, updated_at
		FROM xcentro.purchase_orders
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query purchase orders: %w", err)
	}
	defer rows.Close()

	var pos []*voucher.PurchaseOrder
	for rows.Next() {
		var po voucher.PurchaseOrder
		err := rows.Scan(
			&po.ID, &po.DocumentID, &po.BusinessID, &po.PayeeID, &po.PONumber, &po.PODate, &po.VendorName, &po.VendorGSTIN,
			&po.BuyerGSTIN, &po.TotalAmount, &po.RemainingBalance, &po.POStatus, &po.Discount, &po.DiscountType, &po.CreatedAt, &po.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan purchase order: %w", err)
		}
		pos = append(pos, &po)
	}

	return pos, total, nil
}

// GetBusinessIDByGSTIN looks up a business by its GSTIN and returns the bid.
// Returns empty string if not found, error only on query failures.
func (r *PostgresVoucherRepository) GetBusinessIDByGSTIN(ctx context.Context, gstin string) (string, error) {
	query := `SELECT bid::text FROM xcentro.business WHERE gstin = $1 LIMIT 1`

	var bid string
	err := r.querier.QueryRow(ctx, query, gstin).Scan(&bid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to lookup business by GSTIN: %w", err)
	}
	return bid, nil
}

// --- Matching Engine Repository Methods ---

// GetInvoiceByID retrieves a fully-typed invoice record by its primary key.
func (r *PostgresVoucherRepository) GetInvoiceByID(ctx context.Context, invoiceID uuid.UUID) (*voucher.ExtractedInvoice, error) {
	query := `
		SELECT id, document_id,
		       COALESCE(inv_id, ''), COALESCE(type::text, ''),
		       bid, payee_id, inv_date,
		       COALESCE(total_amount, 0), COALESCE(net_amount, 0),
		       COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'),
		       COALESCE(sgst, 0), COALESCE(cgst, 0), COALESCE(igst, 0),
		       COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       created_at, updated_at
		FROM xcentro.invoice
		WHERE id = $1
	`

	var inv voucher.ExtractedInvoice
	var invDate sql.NullTime
	var bidPtr *uuid.UUID
	err := r.querier.QueryRow(ctx, query, invoiceID).Scan(
		&inv.ID, &inv.DocumentID,
		&inv.InvID, &inv.Type,
		&bidPtr, &inv.PayeeID, &invDate,
		&inv.TotalAmount, &inv.NetAmount,
		&inv.PONumber, &inv.MatchStatus,
		&inv.SGST, &inv.CGST, &inv.IGST,
		&inv.VendorGSTIN, &inv.BuyerGSTIN,
		&inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("invoice not found: %s", invoiceID)
		}
		return nil, fmt.Errorf("failed to get invoice by ID: %w", err)
	}
	if bidPtr != nil {
		inv.BusinessID = *bidPtr
	}
	if invDate.Valid {
		inv.InvDate = &invDate.Time
	}
	return &inv, nil
}

// GetPurchaseOrderByPONumber retrieves a PO by its po_number, scoped to a business.
func (r *PostgresVoucherRepository) GetPurchaseOrderByPONumber(ctx context.Context, poNumber string, businessID uuid.UUID) (*voucher.PurchaseOrder, error) {
	query := `
		SELECT id, document_id, bid, payee_id,
		       COALESCE(po_number, ''), COALESCE(po_date, ''),
		       COALESCE(vendor_name, ''), COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       COALESCE(total_amount, 0), COALESCE(remaining_balance, 0), COALESCE(po_status, 'OPEN'),
		       COALESCE(discount, 0), COALESCE(discount_type, ''),
		       created_at, updated_at
		FROM xcentro.purchase_orders
		WHERE po_number = $1 AND bid = $2
		LIMIT 1
	`

	var po voucher.PurchaseOrder
	err := r.querier.QueryRow(ctx, query, poNumber, businessID).Scan(
		&po.ID, &po.DocumentID, &po.BusinessID, &po.PayeeID,
		&po.PONumber, &po.PODate,
		&po.VendorName, &po.VendorGSTIN, &po.BuyerGSTIN,
		&po.TotalAmount, &po.RemainingBalance, &po.POStatus,
		&po.Discount, &po.DiscountType,
		&po.CreatedAt, &po.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // nil, nil = not found
		}
		return nil, fmt.Errorf("failed to get PO by po_number: %w", err)
	}
	return &po, nil
}

// getLineItemsByFilter is the shared implementation for fetching typed line items.
// For legacy rows where unit_price/tax_rate/tax_amount were stored as 0 (due to a now-fixed
// field-name mismatch), it falls back to the JSONB draft/validated data on the source document.
func (r *PostgresVoucherRepository) getLineItemsByFilter(ctx context.Context, column string, id uuid.UUID) ([]voucher.LineItemDetail, error) {
	query := fmt.Sprintf(`
		SELECT
		    li.id, li.purchase_order_id, li.invoice_id,
		    COALESCE(li.sku, ''), li.description,
		    li.quantity, COALESCE(li.qty_received, 0),
		    -- unit_price: prefer stored value; fall back to JSONB "rate" field (in rupees → paise)
		    CASE WHEN li.unit_price != 0 THEN li.unit_price
		         ELSE COALESCE(ROUND((jdata.jitem->>'rate')::numeric * 100)::bigint, 0)
		    END AS unit_price,
		    -- tax_rate: prefer stored value; fall back to JSONB "gst_rate"
		    CASE WHEN COALESCE(li.tax_rate, 0) != 0 THEN COALESCE(li.tax_rate, 0)
		         ELSE COALESCE((jdata.jitem->>'gst_rate')::numeric, 0)
		    END AS tax_rate,
		    -- tax_amount: prefer stored value; fall back to JSONB "gst_amount" (in rupees → paise)
		    CASE WHEN COALESCE(li.tax_amount, 0) != 0 THEN COALESCE(li.tax_amount, 0)
		         ELSE COALESCE(ROUND((jdata.jitem->>'gst_amount')::numeric * 100)::bigint, 0)
		    END AS tax_amount,
		    COALESCE(li.total_amount, 0)
		FROM xcentro.line_items li
		LEFT JOIN xcentro.invoice inv
		    ON inv.id = li.invoice_id
		LEFT JOIN xcentro.purchase_orders po
		    ON po.id = li.purchase_order_id
		LEFT JOIN xcentro.voucher_documents vd
		    ON vd.id = COALESCE(inv.document_id, po.document_id)
		LEFT JOIN LATERAL (
		    SELECT elem AS jitem
		    FROM jsonb_array_elements(
		        COALESCE(vd.validated_data->'line_items', vd.draft_data->'line_items')
		    ) elem
		    WHERE COALESCE(elem->>'description', '') = li.description
		    LIMIT 1
		) jdata ON true
		WHERE li.%s = $1
	`, column)

	rows, err := r.querier.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query line items (%s=%s): %w", column, id, err)
	}
	defer rows.Close()

	var items []voucher.LineItemDetail
	for rows.Next() {
		var item voucher.LineItemDetail
		err := rows.Scan(
			&item.ID, &item.PurchaseOrderID, &item.InvoiceID,
			&item.SKU, &item.Description,
			&item.Quantity, &item.QtyReceived, &item.UnitPrice,
			&item.TaxRate, &item.TaxAmount, &item.TotalAmount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan line item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetLineItemsByPurchaseOrderID retrieves all typed line items for a PO.
func (r *PostgresVoucherRepository) GetLineItemsByPurchaseOrderID(ctx context.Context, poID uuid.UUID) ([]voucher.LineItemDetail, error) {
	return r.getLineItemsByFilter(ctx, "purchase_order_id", poID)
}

// GetLineItemsByInvoiceID retrieves all typed line items for an invoice.
func (r *PostgresVoucherRepository) GetLineItemsByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]voucher.LineItemDetail, error) {
	return r.getLineItemsByFilter(ctx, "invoice_id", invoiceID)
}

// UpdateInvoiceMatchStatus updates only the match_status column on an invoice.
func (r *PostgresVoucherRepository) UpdateInvoiceMatchStatus(ctx context.Context, invoiceID uuid.UUID, status string) error {
	query := `UPDATE xcentro.invoice SET match_status = $2 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, invoiceID, status)
	if err != nil {
		return fmt.Errorf("failed to update invoice match status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("invoice not found: %s", invoiceID)
	}
	return nil
}

// UpdateInvoicePOID sets the po_id FK on an invoice after a successful match.
func (r *PostgresVoucherRepository) UpdateInvoicePOID(ctx context.Context, invoiceID uuid.UUID, poID uuid.UUID) error {
	query := `UPDATE xcentro.invoice SET po_id = $2 WHERE id = $1`
	_, err := r.querier.Exec(ctx, query, invoiceID, poID)
	if err != nil {
		return fmt.Errorf("failed to update invoice po_id: %w", err)
	}
	return nil
}

// UpdatePurchaseOrderBalance atomically updates remaining_balance and po_status on a PO.
func (r *PostgresVoucherRepository) UpdatePurchaseOrderBalance(ctx context.Context, poID uuid.UUID, remainingBalance float64, status string) error {
	query := `UPDATE xcentro.purchase_orders SET remaining_balance = $2, po_status = $3 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, poID, remainingBalance, status)
	if err != nil {
		return fmt.Errorf("failed to update PO balance: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("purchase order not found: %s", poID)
	}
	return nil
}

// UpdateLineItemQtyReceived adds to qty_received for a line item.
func (r *PostgresVoucherRepository) UpdateLineItemQtyReceived(ctx context.Context, lineItemID uuid.UUID, qtyReceived float64) error {
	query := `UPDATE xcentro.line_items SET qty_received = qty_received + $2 WHERE id = $1`
	result, err := r.querier.Exec(ctx, query, lineItemID, qtyReceived)
	if err != nil {
		return fmt.Errorf("failed to update line item qty_received: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("line item not found: %s", lineItemID)
	}
	return nil
}

// CreateMatchResult persists a matching audit record.
func (r *PostgresVoucherRepository) CreateMatchResult(ctx context.Context, result *voucher.MatchResult) error {
	detailsJSON, err := json.Marshal(result.MatchDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal match_details: %w", err)
	}

	query := `
		INSERT INTO xcentro.match_results (invoice_id, purchase_order_id, match_status, match_details)
		VALUES ($1, $2, $3, $4)
	`
	_, err = r.querier.Exec(ctx, query, result.InvoiceID, result.PurchaseOrderID, result.MatchStatus, detailsJSON)
	if err != nil {
		return fmt.Errorf("failed to create match result: %w", err)
	}
	return nil
}

// GetMatchResultsByInvoiceID retrieves all match result records for a given invoice, most recent first.
func (r *PostgresVoucherRepository) GetMatchResultsByInvoiceID(ctx context.Context, invoiceID uuid.UUID) ([]*voucher.MatchResult, error) {
	query := `
		SELECT id, invoice_id, purchase_order_id, match_status, match_details, created_at
		FROM xcentro.match_results
		WHERE invoice_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.querier.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query match results: %w", err)
	}
	defer rows.Close()

	var results []*voucher.MatchResult
	for rows.Next() {
		var mr voucher.MatchResult
		var detailsJSON []byte
		if err := rows.Scan(
			&mr.ID, &mr.InvoiceID, &mr.PurchaseOrderID,
			&mr.MatchStatus, &detailsJSON, &mr.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan match result: %w", err)
		}
		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &mr.MatchDetails); err != nil {
				mr.MatchDetails = map[string]interface{}{"raw": string(detailsJSON)}
			}
		}
		results = append(results, &mr)
	}
	if results == nil {
		results = []*voucher.MatchResult{}
	}
	return results, rows.Err()
}

// GetValidatedDocumentsByTemplate retrieves validated_data for recent documents using a given i2d template.
func (r *PostgresVoucherRepository) GetValidatedDocumentsByTemplate(ctx context.Context, templateName string, limit int) ([]*voucher.VoucherDocument, error) {
	query := `
		SELECT id, s3_path, doc_type, status, validated_data, i2d_template, processed_by
		FROM xcentro.voucher_documents
		WHERE i2d_template = $1 AND status = 'VALIDATED' AND validated_data IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT $2
	`

	rows, err := r.querier.Query(ctx, query, templateName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query validated documents by template: %w", err)
	}
	defer rows.Close()

	var docs []*voucher.VoucherDocument
	for rows.Next() {
		var doc voucher.VoucherDocument
		var docType sql.NullString
		var validatedData []byte
		var i2dTemplate, processedBy sql.NullString
		if err := rows.Scan(
			&doc.ID, &doc.S3Path, &docType, &doc.Status,
			&validatedData, &i2dTemplate, &processedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan validated document: %w", err)
		}
		if docType.Valid {
			doc.DocType = docType.String
		}
		if i2dTemplate.Valid {
			doc.I2DTemplate = i2dTemplate.String
		}
		if processedBy.Valid {
			doc.ProcessedBy = processedBy.String
		}
		if len(validatedData) > 0 {
			if err := json.Unmarshal(validatedData, &doc.ValidatedData); err != nil {
				doc.ValidatedData = nil
			}
		}
		docs = append(docs, &doc)
	}
	if docs == nil {
		docs = []*voucher.VoucherDocument{}
	}
	return docs, rows.Err()
}

// GetOrphanInvoicesByPONumber finds invoices in ORPHAN_INVOICE status with the given po_number.
func (r *PostgresVoucherRepository) GetOrphanInvoicesByPONumber(ctx context.Context, poNumber string) ([]*voucher.ExtractedInvoice, error) {
	query := `
		SELECT id, document_id,
		       COALESCE(inv_id, ''), COALESCE(type::text, ''),
		       bid, payee_id, inv_date,
		       COALESCE(total_amount, 0), COALESCE(net_amount, 0),
		       COALESCE(po_number, ''), COALESCE(match_status, 'PENDING_MATCH'),
		       COALESCE(sgst, 0), COALESCE(cgst, 0), COALESCE(igst, 0),
		       COALESCE(vendor_gstin, ''), COALESCE(buyer_gstin, ''),
		       created_at, updated_at
		FROM xcentro.invoice
		WHERE po_number = $1 AND match_status = $2
	`

	rows, err := r.querier.Query(ctx, query, poNumber, voucher.InvoiceMatchOrphan)
	if err != nil {
		return nil, fmt.Errorf("failed to query orphan invoices: %w", err)
	}
	defer rows.Close()

	var invoices []*voucher.ExtractedInvoice
	for rows.Next() {
		var inv voucher.ExtractedInvoice
		var invDate sql.NullTime
		var bidPtr *uuid.UUID
		err := rows.Scan(
			&inv.ID, &inv.DocumentID,
			&inv.InvID, &inv.Type,
			&bidPtr, &inv.PayeeID, &invDate,
			&inv.TotalAmount, &inv.NetAmount,
			&inv.PONumber, &inv.MatchStatus,
			&inv.SGST, &inv.CGST, &inv.IGST,
			&inv.VendorGSTIN, &inv.BuyerGSTIN,
			&inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan orphan invoice: %w", err)
		}
		if bidPtr != nil {
			inv.BusinessID = *bidPtr
		}
		if invDate.Valid {
			inv.InvDate = &invDate.Time
		}
		invoices = append(invoices, &inv)
	}
	return invoices, rows.Err()
}

// ListMatchResults retrieves a paginated, joined view of match_results + invoice + purchase_orders.
func (r *PostgresVoucherRepository) ListMatchResults(ctx context.Context, filter voucher.MatchResultListFilter) ([]*voucher.MatchResultSummary, int64, error) {
	whereClauses := []string{"1=1"}
	args := []interface{}{}
	argPos := 1

	if filter.BusinessID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("inv.bid = $%d", argPos))
		args = append(args, *filter.BusinessID)
		argPos++
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM xcentro.match_results mr
		JOIN xcentro.invoice inv ON inv.id = mr.invoice_id
		LEFT JOIN xcentro.purchase_orders po ON po.id = mr.purchase_order_id
		WHERE %s
	`, whereClause)

	var total int64
	if err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count match results: %w", err)
	}

	limit := filter.PageSize
	if limit == 0 {
		limit = 20
	}
	offset := (filter.Page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT
			mr.id,
			mr.invoice_id,
			mr.purchase_order_id,
			mr.match_status,
			mr.match_details,
			mr.created_at,
			COALESCE(inv.inv_id, ''),
			inv.inv_date,
			COALESCE(inv.net_amount, 0),
			COALESCE(inv.total_amount, 0),
			inv.document_id,
			COALESCE(po.po_number, ''),
			COALESCE(po.po_date, ''),
			COALESCE(po.vendor_name, ''),
			COALESCE(po.total_amount, 0),
			COALESCE(po.remaining_balance, 0),
			COALESCE(po.po_status, ''),
			po.document_id
		FROM xcentro.match_results mr
		JOIN xcentro.invoice inv ON inv.id = mr.invoice_id
		LEFT JOIN xcentro.purchase_orders po ON po.id = mr.purchase_order_id
		WHERE %s
		ORDER BY mr.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query match results: %w", err)
	}
	defer rows.Close()

	var results []*voucher.MatchResultSummary
	for rows.Next() {
		var s voucher.MatchResultSummary
		var detailsJSON []byte
		var invDate sql.NullTime
		var invDocID *uuid.UUID
		var poDocID *uuid.UUID

		if err := rows.Scan(
			&s.MatchResultID,
			&s.InvoiceID,
			&s.PurchaseOrderID,
			&s.MatchStatus,
			&detailsJSON,
			&s.MatchedAt,
			&s.InvoiceNumber,
			&invDate,
			&s.InvoiceNetAmount,
			&s.InvoiceTotalAmount,
			&invDocID,
			&s.PONumber,
			&s.PODate,
			&s.VendorName,
			&s.POAmount,
			&s.PORemainingBalance,
			&s.POStatus,
			&poDocID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan match result summary: %w", err)
		}

		if invDate.Valid {
			s.InvoiceDate = &invDate.Time
		}
		s.InvoiceDocumentID = invDocID
		s.PODocumentID = poDocID
		s.InvoiceAmount = s.InvoiceNetAmount
		s.Variance = s.InvoiceNetAmount - s.POAmount

		if len(detailsJSON) > 0 {
			if err := json.Unmarshal(detailsJSON, &s.MatchNotes); err != nil {
				s.MatchNotes = map[string]interface{}{"raw": string(detailsJSON)}
			}
		}

		results = append(results, &s)
	}
	if results == nil {
		results = []*voucher.MatchResultSummary{}
	}
	return results, total, rows.Err()
}
