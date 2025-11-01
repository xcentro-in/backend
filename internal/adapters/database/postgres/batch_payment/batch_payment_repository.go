package batch_payment

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/domain"
	"xcentro.in/internal/core/repositories"
)

// PgBatchPaymentRepository implements BatchPaymentRepository for PostgreSQL
type PgBatchPaymentRepository struct {
	querier client.Querier
	db      client.DB
}

// NewPgBatchPaymentRepository creates a new PostgreSQL batch payment repository
func NewPgBatchPaymentRepository(db client.DB) repositories.BatchPaymentRepository {
	return &PgBatchPaymentRepository{
		querier: db,
		db:      db,
	}
}

// CreateBatch creates a new batch payment record
func (r *PgBatchPaymentRepository) CreateBatch(ctx context.Context, batch *domain.BatchPayment) error {
	query := `
		INSERT INTO xcentro.batch_payments (
			batch_id, business_id, uploaded_by, file_name, status,
			total_rows, total_amount, processed_rows, success_rows, failed_rows,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.querier.Exec(ctx, query,
		batch.BatchID,
		batch.BusinessID,
		batch.UploadedBy,
		batch.FileName,
		batch.Status,
		batch.TotalRows,
		batch.TotalAmount,
		batch.ProcessedRows,
		batch.SuccessRows,
		batch.FailedRows,
		batch.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create batch: %w", err)
	}

	return nil
}

// CreateBatchItems creates multiple batch payment items
func (r *PgBatchPaymentRepository) CreateBatchItems(ctx context.Context, items []*domain.BatchPaymentItem) error {
	if len(items) == 0 {
		return nil
	}

	query := `
		INSERT INTO xcentro.batch_payment_items (
			item_id, batch_id, row_number, status,
			csv_payee_name, csv_account_last_4, csv_amount,
			payment_context, description, expense_category,
			gst_amount, tds_amount, invoice_number,
			matched_relation_id, match_confidence,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	for _, item := range items {
		_, err := r.querier.Exec(ctx, query,
			item.ItemID,
			item.BatchID,
			item.RowNumber,
			item.Status,
			item.CSVPayeeName,
			item.CSVAccountLast4,
			item.CSVAmount,
			item.PaymentContext,
			item.Description,
			item.ExpenseCategory,
			item.GSTAmount,
			item.TDSAmount,
			item.InvoiceNumber,
			item.MatchedRelationID,
			item.MatchConfidence,
			item.CreatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to create batch item row %d: %w", item.RowNumber, err)
		}
	}

	return nil
}

// GetBatchByID retrieves a batch by ID
func (r *PgBatchPaymentRepository) GetBatchByID(ctx context.Context, batchID uuid.UUID) (*domain.BatchPayment, error) {
	query := `
		SELECT
			batch_id, business_id, uploaded_by, file_name, status,
			total_rows, total_amount, processed_rows, success_rows, failed_rows,
			created_at, processed_at, completed_at
		FROM xcentro.batch_payments
		WHERE batch_id = $1
	`

	var batch domain.BatchPayment
	err := r.querier.QueryRow(ctx, query, batchID).Scan(
		&batch.BatchID,
		&batch.BusinessID,
		&batch.UploadedBy,
		&batch.FileName,
		&batch.Status,
		&batch.TotalRows,
		&batch.TotalAmount,
		&batch.ProcessedRows,
		&batch.SuccessRows,
		&batch.FailedRows,
		&batch.CreatedAt,
		&batch.ProcessedAt,
		&batch.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	return &batch, nil
}

// GetBatchItems retrieves all items for a batch
func (r *PgBatchPaymentRepository) GetBatchItems(ctx context.Context, batchID uuid.UUID) ([]*domain.BatchPaymentItem, error) {
	query := `
		SELECT
			item_id, batch_id, row_number, status,
			csv_payee_name, csv_account_last_4, csv_amount,
			payment_context, description, COALESCE(expense_category, ''),
			COALESCE(gst_amount, 0), COALESCE(tds_amount, 0), COALESCE(invoice_number, ''),
			matched_relation_id, COALESCE(match_confidence, 0),
			voucher_id, COALESCE(error_message, ''),
			created_at, processed_at
		FROM xcentro.batch_payment_items
		WHERE batch_id = $1
		ORDER BY row_number ASC
	`

	rows, err := r.querier.Query(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to query batch items: %w", err)
	}
	defer rows.Close()

	var items []*domain.BatchPaymentItem
	for rows.Next() {
		var item domain.BatchPaymentItem
		err := rows.Scan(
			&item.ItemID,
			&item.BatchID,
			&item.RowNumber,
			&item.Status,
			&item.CSVPayeeName,
			&item.CSVAccountLast4,
			&item.CSVAmount,
			&item.PaymentContext,
			&item.Description,
			&item.ExpenseCategory,
			&item.GSTAmount,
			&item.TDSAmount,
			&item.InvoiceNumber,
			&item.MatchedRelationID,
			&item.MatchConfidence,
			&item.VoucherID,
			&item.ErrorMessage,
			&item.CreatedAt,
			&item.ProcessedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan batch item: %w", err)
		}

		items = append(items, &item)
	}

	return items, nil
}

// GetBatchItemsByStatus retrieves items filtered by status
func (r *PgBatchPaymentRepository) GetBatchItemsByStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchPaymentItemStatus) ([]*domain.BatchPaymentItem, error) {
	query := `
		SELECT
			item_id, batch_id, row_number, status,
			csv_payee_name, csv_account_last_4, csv_amount,
			payment_context, description, COALESCE(expense_category, ''),
			COALESCE(gst_amount, 0), COALESCE(tds_amount, 0), COALESCE(invoice_number, ''),
			matched_relation_id, COALESCE(match_confidence, 0),
			voucher_id, COALESCE(error_message, ''),
			created_at, processed_at
		FROM xcentro.batch_payment_items
		WHERE batch_id = $1 AND status = $2
		ORDER BY row_number ASC
	`

	rows, err := r.querier.Query(ctx, query, batchID, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query batch items by status: %w", err)
	}
	defer rows.Close()

	var items []*domain.BatchPaymentItem
	for rows.Next() {
		var item domain.BatchPaymentItem
		err := rows.Scan(
			&item.ItemID,
			&item.BatchID,
			&item.RowNumber,
			&item.Status,
			&item.CSVPayeeName,
			&item.CSVAccountLast4,
			&item.CSVAmount,
			&item.PaymentContext,
			&item.Description,
			&item.ExpenseCategory,
			&item.GSTAmount,
			&item.TDSAmount,
			&item.InvoiceNumber,
			&item.MatchedRelationID,
			&item.MatchConfidence,
			&item.VoucherID,
			&item.ErrorMessage,
			&item.CreatedAt,
			&item.ProcessedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan batch item: %w", err)
		}

		items = append(items, &item)
	}

	return items, nil
}

// UpdateBatchStatus updates the batch status
func (r *PgBatchPaymentRepository) UpdateBatchStatus(ctx context.Context, batchID uuid.UUID, status domain.BatchPaymentStatus) error {
	query := `
		UPDATE xcentro.batch_payments
		SET status = $1,
		    completed_at = CASE WHEN $1 IN ('completed', 'failed', 'cancelled') THEN NOW() ELSE completed_at END
		WHERE batch_id = $2
	`

	_, err := r.querier.Exec(ctx, query, status, batchID)
	if err != nil {
		return fmt.Errorf("failed to update batch status: %w", err)
	}

	return nil
}

// UpdateBatchItem updates a batch item
func (r *PgBatchPaymentRepository) UpdateBatchItem(ctx context.Context, item *domain.BatchPaymentItem) error {
	query := `
		UPDATE xcentro.batch_payment_items
		SET status = $1,
		    matched_relation_id = $2,
		    match_confidence = $3,
		    voucher_id = $4,
		    error_message = $5,
		    processed_at = $6
		WHERE item_id = $7
	`

	_, err := r.querier.Exec(ctx, query,
		item.Status,
		item.MatchedRelationID,
		item.MatchConfidence,
		item.VoucherID,
		item.ErrorMessage,
		item.ProcessedAt,
		item.ItemID,
	)

	if err != nil {
		return fmt.Errorf("failed to update batch item: %w", err)
	}

	return nil
}

// UpdateItemMatching updates the matched relation ID for an item
func (r *PgBatchPaymentRepository) UpdateItemMatching(ctx context.Context, itemID uuid.UUID, relationID uuid.UUID) error {
	query := `
		UPDATE xcentro.batch_payment_items
		SET matched_relation_id = $1,
		    status = 'matched'
		WHERE item_id = $2
	`

	_, err := r.querier.Exec(ctx, query, relationID, itemID)
	if err != nil {
		return fmt.Errorf("failed to update item matching: %w", err)
	}

	return nil
}

// GetBusinessBatches retrieves all batches for a business
func (r *PgBatchPaymentRepository) GetBusinessBatches(ctx context.Context, businessID uuid.UUID, limit, offset int) ([]*domain.BatchPayment, int, error) {
	// Get total count
	countQuery := `
		SELECT COUNT(*)
		FROM xcentro.batch_payments
		WHERE business_id = $1
	`

	var totalCount int
	err := r.querier.QueryRow(ctx, countQuery, businessID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count batches: %w", err)
	}

	// Get paginated batches
	query := `
		SELECT
			batch_id, business_id, uploaded_by, file_name, status,
			total_rows, total_amount, processed_rows, success_rows, failed_rows,
			created_at, processed_at, completed_at
		FROM xcentro.batch_payments
		WHERE business_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, businessID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query batches: %w", err)
	}
	defer rows.Close()

	var batches []*domain.BatchPayment
	for rows.Next() {
		var batch domain.BatchPayment
		err := rows.Scan(
			&batch.BatchID,
			&batch.BusinessID,
			&batch.UploadedBy,
			&batch.FileName,
			&batch.Status,
			&batch.TotalRows,
			&batch.TotalAmount,
			&batch.ProcessedRows,
			&batch.SuccessRows,
			&batch.FailedRows,
			&batch.CreatedAt,
			&batch.ProcessedAt,
			&batch.CompletedAt,
		)

		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan batch: %w", err)
		}

		batches = append(batches, &batch)
	}

	return batches, totalCount, nil
}
