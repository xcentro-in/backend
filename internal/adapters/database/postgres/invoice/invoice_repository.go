package invoice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/invoice"
	"xcentro.in/internal/shared/logger"
)

// PgInvoiceRepository implements invoice.InvoiceRepository using PostgreSQL
type PgInvoiceRepository struct {
	querier client.Querier
	logger  *logger.Logger
}

// NewPgInvoiceRepository creates a new PostgreSQL invoice repository
func NewPgInvoiceRepository(db client.DB) *PgInvoiceRepository {
	return &PgInvoiceRepository{
		querier: db,
		logger:  logger.GetDefault(),
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgInvoiceRepository) WithTx(tx interface{}) invoice.InvoiceRepository {
	return &PgInvoiceRepository{
		querier: tx.(client.Tx),
		logger:  r.logger, // Preserve logger in transaction
	}
}

// CreateInvoice creates a new invoice record
func (r *PgInvoiceRepository) CreateInvoice(ctx context.Context, inv *invoice.Invoice) error {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] CreateInvoice called - PaymentID: %s, GSTINRecipient: %s",
		inv.PaymentID, inv.GSTINRecipient))

	query := `
		INSERT INTO xcentro.payment_invoices (
			payment_id, invoice_number, gstin_supplier, gstin_recipient, invoice_date,
			irn, irn_status, filing_status, description, sac_code,
			taxable_value, gst_rate, gst_amount, total_amount
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING invoice_id, created_at, updated_at
	`

	err := r.querier.QueryRow(ctx, query,
		inv.PaymentID,      // $1
		inv.InvoiceNumber,  // $2
		inv.GSTINSupplier,  // $3
		inv.GSTINRecipient, // $4
		inv.InvoiceDate,    // $5
		inv.IRN,            // $6
		inv.IRNStatus,      // $7
		inv.FilingStatus,   // $8
		inv.Description,    // $9
		inv.SACCode,        // $10
		inv.TaxableValue,   // $11
		inv.GSTRate,        // $12
		inv.GSTAmount,      // $13
		inv.TotalAmount,    // $14
	).Scan(&inv.InvoiceID, &inv.CreatedAt, &inv.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create invoice: %w", err)
	}

	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] Invoice created with ID: %d", inv.InvoiceID))
	return nil
}

// GetInvoiceByID retrieves an invoice by its ID
func (r *PgInvoiceRepository) GetInvoiceByID(ctx context.Context, invoiceID int64) (*invoice.Invoice, error) {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] GetInvoiceByID called - InvoiceID: %d", invoiceID))

	query := `
		SELECT invoice_id, payment_id, invoice_number, gstin_supplier, gstin_recipient,
		       invoice_date, irn, irn_ack_no, irn_ack_date, irn_status,
		       qr_code, filing_status, description, sac_code,
		       taxable_value, gst_rate, gst_amount, total_amount,
		       created_at, updated_at
		FROM xcentro.payment_invoices
		WHERE invoice_id = $1
	`

	var inv invoice.Invoice
	var irnAckDate *time.Time

	err := r.querier.QueryRow(ctx, query, invoiceID).Scan(
		&inv.InvoiceID,
		&inv.PaymentID,
		&inv.InvoiceNumber,
		&inv.GSTINSupplier,
		&inv.GSTINRecipient,
		&inv.InvoiceDate,
		&inv.IRN,
		&inv.IRNAckNo,
		&irnAckDate,
		&inv.IRNStatus,
		&inv.QRCode,
		&inv.FilingStatus,
		&inv.Description,
		&inv.SACCode,
		&inv.TaxableValue,
		&inv.GSTRate,
		&inv.GSTAmount,
		&inv.TotalAmount,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get invoice by ID: %w", err)
	}

	inv.IRNAckDate = irnAckDate

	return &inv, nil
}

// GetInvoiceByPaymentID retrieves an invoice by payment ID
func (r *PgInvoiceRepository) GetInvoiceByPaymentID(ctx context.Context, paymentID string) (*invoice.Invoice, error) {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] GetInvoiceByPaymentID called - PaymentID: %s", paymentID))

	query := `
		SELECT invoice_id, payment_id, invoice_number, gstin_supplier, gstin_recipient,
		       invoice_date, irn, irn_ack_no, irn_ack_date, irn_status,
		       qr_code, filing_status, description, sac_code,
		       taxable_value, gst_rate, gst_amount, total_amount,
		       created_at, updated_at
		FROM xcentro.payment_invoices
		WHERE payment_id = $1
	`

	var inv invoice.Invoice
	var irnAckDate *time.Time

	err := r.querier.QueryRow(ctx, query, paymentID).Scan(
		&inv.InvoiceID,
		&inv.PaymentID,
		&inv.InvoiceNumber,
		&inv.GSTINSupplier,
		&inv.GSTINRecipient,
		&inv.InvoiceDate,
		&inv.IRN,
		&inv.IRNAckNo,
		&irnAckDate,
		&inv.IRNStatus,
		&inv.QRCode,
		&inv.FilingStatus,
		&inv.Description,
		&inv.SACCode,
		&inv.TaxableValue,
		&inv.GSTRate,
		&inv.GSTAmount,
		&inv.TotalAmount,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get invoice by payment ID: %w", err)
	}

	inv.IRNAckDate = irnAckDate

	return &inv, nil
}

// ListInvoices retrieves invoices with filtering and pagination
func (r *PgInvoiceRepository) ListInvoices(ctx context.Context, filter *invoice.InvoiceListFilter) ([]invoice.Invoice, int64, error) {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] ListInvoices called - Page: %d, PageSize: %d", filter.Page, filter.PageSize))

	// Build WHERE clause dynamically based on filters
	whereClauses := []string{}
	args := []interface{}{}
	argPosition := 1

	if filter.InvoiceID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("invoice_id = $%d", argPosition))
		args = append(args, *filter.InvoiceID)
		argPosition++
	}

	if filter.PaymentID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("payment_id = $%d", argPosition))
		args = append(args, *filter.PaymentID)
		argPosition++
	}

	if filter.IRN != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("irn = $%d", argPosition))
		args = append(args, *filter.IRN)
		argPosition++
	}

	if filter.GSTINRecipient != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("gstin_recipient = $%d", argPosition))
		args = append(args, *filter.GSTINRecipient)
		argPosition++
	}

	if filter.IRNStatus != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("irn_status = $%d", argPosition))
		args = append(args, *filter.IRNStatus)
		argPosition++
	}

	if filter.FilingStatus != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("filing_status = $%d", argPosition))
		args = append(args, *filter.FilingStatus)
		argPosition++
	}

	if filter.DateFrom != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("invoice_date >= $%d", argPosition))
		args = append(args, *filter.DateFrom)
		argPosition++
	}

	if filter.DateTo != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("invoice_date <= $%d", argPosition))
		args = append(args, *filter.DateTo)
		argPosition++
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total records
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM xcentro.payment_invoices
		%s
	`, whereClause)

	var totalCount int64
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count invoices: %w", err)
	}

	// Fetch paginated results
	offset := (filter.Page - 1) * filter.PageSize
	dataQuery := fmt.Sprintf(`
		SELECT invoice_id, payment_id, invoice_number, gstin_supplier, gstin_recipient,
		       invoice_date, irn, irn_ack_no, irn_ack_date, irn_status,
		       qr_code, filing_status, description, sac_code,
		       taxable_value, gst_rate, gst_amount, total_amount,
		       created_at, updated_at
		FROM xcentro.payment_invoices
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPosition, argPosition+1)

	args = append(args, filter.PageSize, offset)

	rows, err := r.querier.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list invoices: %w", err)
	}
	defer rows.Close()

	invoices := []invoice.Invoice{}
	for rows.Next() {
		var inv invoice.Invoice
		var irnAckDate *time.Time

		err := rows.Scan(
			&inv.InvoiceID,
			&inv.PaymentID,
			&inv.InvoiceNumber,
			&inv.GSTINSupplier,
			&inv.GSTINRecipient,
			&inv.InvoiceDate,
			&inv.IRN,
			&inv.IRNAckNo,
			&irnAckDate,
			&inv.IRNStatus,
			&inv.QRCode,
			&inv.FilingStatus,
			&inv.Description,
			&inv.SACCode,
			&inv.TaxableValue,
			&inv.GSTRate,
			&inv.GSTAmount,
			&inv.TotalAmount,
			&inv.CreatedAt,
			&inv.UpdatedAt,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan invoice: %w", err)
		}

		inv.IRNAckDate = irnAckDate

		invoices = append(invoices, inv)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating invoice rows: %w", err)
	}

	return invoices, totalCount, nil
}

// UpdateIRNDetails updates IRN-related fields
func (r *PgInvoiceRepository) UpdateIRNDetails(ctx context.Context, invoiceID int64, irn, ackNo string, ackDate *string, status invoice.IRNStatus) error {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] UpdateIRNDetails called - InvoiceID: %d, IRN: %s, Status: %s", invoiceID, irn, status))

	query := `
		UPDATE xcentro.payment_invoices
		SET irn = $1, irn_ack_no = $2, irn_ack_date = $3, irn_status = $4, updated_at = NOW()
		WHERE invoice_id = $5
	`

	_, err := r.querier.Exec(ctx, query, irn, ackNo, ackDate, status, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to update IRN details: %w", err)
	}

	return nil
}

// UpdateFilingStatus updates the filing status
func (r *PgInvoiceRepository) UpdateFilingStatus(ctx context.Context, invoiceID int64, status invoice.FilingStatus) error {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] UpdateFilingStatus called - InvoiceID: %d, Status: %s", invoiceID, status))

	query := `
		UPDATE xcentro.payment_invoices
		SET filing_status = $1, updated_at = NOW()
		WHERE invoice_id = $2
	`

	_, err := r.querier.Exec(ctx, query, status, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to update filing status: %w", err)
	}

	return nil
}

// UpdateQRCode updates the QR code
func (r *PgInvoiceRepository) UpdateQRCode(ctx context.Context, invoiceID int64, qrCode string) error {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] UpdateQRCode called - InvoiceID: %d", invoiceID))

	query := `
		UPDATE xcentro.payment_invoices
		SET qr_code = $1, updated_at = NOW()
		WHERE invoice_id = $2
	`

	_, err := r.querier.Exec(ctx, query, qrCode, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to update QR code: %w", err)
	}

	return nil
}

// DeleteInvoice deletes an invoice by ID
func (r *PgInvoiceRepository) DeleteInvoice(ctx context.Context, invoiceID int64) error {
	r.logger.Info(fmt.Sprintf("[PgInvoiceRepository] DeleteInvoice called - InvoiceID: %d", invoiceID))

	query := `
		DELETE FROM xcentro.payment_invoices
		WHERE invoice_id = $1
	`

	_, err := r.querier.Exec(ctx, query, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to delete invoice: %w", err)
	}

	return nil
}
