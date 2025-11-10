package payment

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
)

// PaymentVoucherPendingRepository handles operations for the payment_voucher_pending table
// This is a temporary table used during payment initiation to store voucher-payment relationships
type PaymentVoucherPendingRepository struct {
	querier client.Querier
}

// NewPaymentVoucherPendingRepository creates a new repository instance
func NewPaymentVoucherPendingRepository(db client.DB) *PaymentVoucherPendingRepository {
	return &PaymentVoucherPendingRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PaymentVoucherPendingRepository) WithTx(tx client.Tx) *PaymentVoucherPendingRepository {
	return &PaymentVoucherPendingRepository{
		querier: tx,
	}
}

// StorePendingVouchers stores the voucher IDs for a payment in the pending table
// This is called during payment initiation to temporarily link vouchers to the payment
func (r *PaymentVoucherPendingRepository) StorePendingVouchers(ctx context.Context, paymentID uuid.UUID, voucherIDs []uuid.UUID) error {
	if len(voucherIDs) == 0 {
		return nil // Nothing to store
	}

	// Build bulk insert query
	query := `
		INSERT INTO xcentro.payment_voucher_pending (payment_id, voucher_id, created_at)
		VALUES `

	// Build values clause
	values := make([]interface{}, 0, len(voucherIDs)*2)
	for i, voucherID := range voucherIDs {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($1, $%d, NOW())", i+2)
		values = append(values, voucherID)
	}

	// Prepend payment_id to values
	values = append([]interface{}{paymentID}, values...)

	// Execute insert
	_, err := r.querier.Exec(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("failed to store pending vouchers: %w", err)
	}

	return nil
}

// GetPendingVouchers retrieves all voucher IDs for a payment from the pending table
// This is called in callback handler to finalize vouchers when payment succeeds
func (r *PaymentVoucherPendingRepository) GetPendingVouchers(ctx context.Context, paymentID uuid.UUID) ([]uuid.UUID, error) {
	query := `
		SELECT voucher_id
		FROM xcentro.payment_voucher_pending
		WHERE payment_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending vouchers: %w", err)
	}
	defer rows.Close()

	var voucherIDs []uuid.UUID
	for rows.Next() {
		var voucherID uuid.UUID
		if err := rows.Scan(&voucherID); err != nil {
			return nil, fmt.Errorf("failed to scan voucher ID: %w", err)
		}
		voucherIDs = append(voucherIDs, voucherID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending vouchers: %w", err)
	}

	return voucherIDs, nil
}

// DeletePendingVouchers removes the voucher-payment relationships from the pending table
// This is called after vouchers are finalized (on payment success) or when cleaning up
func (r *PaymentVoucherPendingRepository) DeletePendingVouchers(ctx context.Context, paymentID uuid.UUID) error {
	query := `
		DELETE FROM xcentro.payment_voucher_pending
		WHERE payment_id = $1
	`

	result, err := r.querier.Exec(ctx, query, paymentID)
	if err != nil {
		return fmt.Errorf("failed to delete pending vouchers: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		// No records found - this is OK, might have been already cleaned up
		return nil
	}

	return nil
}

// CleanupOldPendingVouchers removes pending voucher records older than the specified duration
// This is meant to be called by a cron job to clean up abandoned/failed payments
// durationHours: number of hours after which a pending record is considered abandoned
func (r *PaymentVoucherPendingRepository) CleanupOldPendingVouchers(ctx context.Context, durationHours int) (int64, error) {
	query := `
		DELETE FROM xcentro.payment_voucher_pending
		WHERE created_at < NOW() - INTERVAL '%d hours'
	`

	query = fmt.Sprintf(query, durationHours)

	result, err := r.querier.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old pending vouchers: %w", err)
	}

	return result.RowsAffected(), nil
}

// HasPendingVoucher checks if a voucher is already part of a pending payment
// This can be used to prevent double-payment of the same voucher
func (r *PaymentVoucherPendingRepository) HasPendingVoucher(ctx context.Context, voucherID uuid.UUID) (bool, string, error) {
	query := `
		SELECT payment_id
		FROM xcentro.payment_voucher_pending
		WHERE voucher_id = $1
		LIMIT 1
	`

	var paymentID string
	err := r.querier.QueryRow(ctx, query, voucherID).Scan(&paymentID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return false, "", nil
		}
		return false, "", fmt.Errorf("failed to check pending voucher: %w", err)
	}

	return true, paymentID, nil
}
