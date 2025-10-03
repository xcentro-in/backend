package voucher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/voucher"
)

// PostgresVoucherRepository implements the VoucherRepository interface using PostgreSQL
type PostgresVoucherRepository struct {
	querier client.Querier
}

// NewPostgresVoucherRepository creates a new PostgreSQL voucher repository
func NewPostgresVoucherRepository(db client.DB) voucher.VoucherRepository {
	return &PostgresVoucherRepository{
		querier: db,
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

	err := row.Scan(
		&v.VoucherID, &v.VoucherNumber, &v.VoucherDate, &v.FiscalYear, &v.AccountingPeriod, &v.VoucherType,
		&v.BusinessID, &v.PayeeID, &v.AccountID,
		&v.InvoiceNumber, &v.InvoiceDate, &v.InvoiceDueDate,
		&v.GrossAmount, &v.NetAmount, &v.Currency,
		&v.GSTAmount, &v.GSTRate, &v.CGSTAmount, &v.SGSTAmount, &v.IGSTAmount,
		&v.TDSAmount, &v.TDSRate, &v.TDSSection,
		&v.FeeAmount, &v.DiscountAmount,
		&financialDetailsJSON, &attachmentsJSON,
		&v.ExpenseCategory, &v.Description,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &v.ERPReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
		&v.GLExpenseAccount, &v.GLBankAccount, &v.CostCenter,
		&v.CreatedAt, &v.CreatedByID, &v.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return nil, err
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

	err := rows.Scan(
		&v.VoucherID, &v.VoucherNumber, &v.VoucherDate, &v.FiscalYear, &v.AccountingPeriod, &v.VoucherType,
		&v.BusinessID, &v.PayeeID, &v.AccountID,
		&v.InvoiceNumber, &v.InvoiceDate, &v.InvoiceDueDate,
		&v.GrossAmount, &v.NetAmount, &v.Currency,
		&v.GSTAmount, &v.GSTRate, &v.CGSTAmount, &v.SGSTAmount, &v.IGSTAmount,
		&v.TDSAmount, &v.TDSRate, &v.TDSSection,
		&v.FeeAmount, &v.DiscountAmount,
		&financialDetailsJSON, &attachmentsJSON,
		&v.ExpenseCategory, &v.Description,
		&v.Status, &v.PaymentID,
		&v.ERPSyncStatus, &v.ERPReferenceID, &v.ERPPostingDate, &v.ERPLastSyncAt, &v.ERPSyncAttempts, &erpMetadataJSON,
		&v.GLExpenseAccount, &v.GLBankAccount, &v.CostCenter,
		&v.CreatedAt, &v.CreatedByID, &v.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return nil, err
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
