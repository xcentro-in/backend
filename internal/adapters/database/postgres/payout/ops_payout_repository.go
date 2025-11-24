package payout

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payout"
)

// PostgresOpsPayoutRepository implements OpsPayoutRepository using PostgreSQL
type PostgresOpsPayoutRepository struct {
	querier client.Querier
}

// NewPostgresOpsPayoutRepository creates a new PostgreSQL ops payout repository
func NewPostgresOpsPayoutRepository(db client.DB) payout.OpsPayoutRepository {
	return &PostgresOpsPayoutRepository{
		querier: db,
	}
}

// ListPayoutsPaginated retrieves paginated list of payout transfers with recipient info
func (r *PostgresOpsPayoutRepository) ListPayoutsPaginated(ctx context.Context, page int, limit int, status string) ([]*payout.OpsPayoutListItem, int, error) {
	// Build status filter based on ops status categories
	var statusFilter string
	switch status {
	case "in_progress":
		statusFilter = "ptr.status IN ('initiated', 'processing')"
	case "completed":
		statusFilter = "ptr.status = 'completed'"
	case "failed":
		statusFilter = "ptr.status IN ('failed', 'timeout')"
	default:
		statusFilter = "1=1" // No filter
	}

	// Calculate offset
	offset := (page - 1) * limit

	// Query to fetch payout transfers with recipient info
	// Joins: payout_transfer_requests -> payment_vouchers -> business_payee_relation -> payee_master + payee_bank_accounts
	query := fmt.Sprintf(`
		SELECT
			ptr.transfer_request_id,
			ptr.created_at,
			COALESCE(bpr.display_name, pm.display_name, 'Unknown') as recipient_name,
			COALESCE(RIGHT(pba.account_number_encrypted, 4), '****') as bank_account_last_4,
			COALESCE(pm.payee_type::text, 'unknown') as payee_type,
			ptr.amount,
			ptr.status::text as status
		FROM xcentro.payout_transfer_requests ptr
		INNER JOIN xcentro.payment_vouchers pv ON ptr.voucher_id = pv.voucher_id
		LEFT JOIN xcentro.business_payee_relation bpr
			ON pv.business_id = bpr.business_id
			AND pv.payee_id = bpr.payee_id
			AND pv.account_id = bpr.account_id
		LEFT JOIN xcentro.payee_master pm ON pv.payee_id = pm.payee_id
		LEFT JOIN xcentro.payee_bank_accounts pba ON pv.account_id = pba.account_id
		WHERE %s
		ORDER BY ptr.created_at DESC
		LIMIT $1 OFFSET $2
	`, statusFilter)

	rows, err := r.querier.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list payout transfers: %w", err)
	}
	defer rows.Close()

	var items []*payout.OpsPayoutListItem

	for rows.Next() {
		var item payout.OpsPayoutListItem
		err := rows.Scan(
			&item.TransferRequestID,
			&item.CreatedAt,
			&item.RecipientName,
			&item.BankAccountLast4,
			&item.PayeeType,
			&item.Amount,
			&item.Status,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan payout transfer: %w", err)
		}
		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating payout transfer rows: %w", err)
	}

	// Get total count
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM xcentro.payout_transfer_requests ptr
		WHERE %s
	`, statusFilter)

	var totalCount int
	err = r.querier.QueryRow(ctx, countQuery).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	return items, totalCount, nil
}

// GetPayoutSummary retrieves detailed summary for a single payout transfer
func (r *PostgresOpsPayoutRepository) GetPayoutSummary(ctx context.Context, transferRequestID string) (*payout.OpsPayoutSummary, error) {
	// Query to fetch payout transfer summary with business, payee, and provider details
	// Joins: payout_transfer_requests -> payment_vouchers -> business + business_payee_relation
	query := `
		SELECT
			b.bid::text as business_id,
			b.business_name,
			CONCAT(pv.business_id::text, ':', pv.payee_id::text, ':', pv.account_id::text) as payee_relation_id,
			COALESCE(bpr.display_name, pm.display_name, 'Unknown') as payee_display_name,
			ptr.provider,
			COALESCE(ptr.provider_transfer_id, '') as provider_transfer_id,
			ptr.utr,
			ptr.completed_at,
			ptr.status::text as payout_status
		FROM xcentro.payout_transfer_requests ptr
		INNER JOIN xcentro.payment_vouchers pv ON ptr.voucher_id = pv.voucher_id
		INNER JOIN xcentro.business b ON pv.business_id = b.bid
		LEFT JOIN xcentro.business_payee_relation bpr
			ON pv.business_id = bpr.business_id
			AND pv.payee_id = bpr.payee_id
			AND pv.account_id = bpr.account_id
		LEFT JOIN xcentro.payee_master pm ON pv.payee_id = pm.payee_id
		WHERE ptr.transfer_request_id = $1
	`

	var summary payout.OpsPayoutSummary
	var utr, providerTransferID sql.NullString
	var settlementDate sql.NullTime

	err := r.querier.QueryRow(ctx, query, transferRequestID).Scan(
		&summary.BusinessID,
		&summary.BusinessName,
		&summary.PayeeRelationID,
		&summary.PayeeDisplayName,
		&summary.PayoutProvider,
		&providerTransferID,
		&utr,
		&settlementDate,
		&summary.PayoutStatus,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payout transfer not found: %s", transferRequestID)
		}
		return nil, fmt.Errorf("failed to get payout summary: %w", err)
	}

	// Handle nullable fields
	if utr.Valid && utr.String != "" {
		summary.UTR = &utr.String
	}

	if providerTransferID.Valid && providerTransferID.String != "" {
		summary.ProviderTransferID = providerTransferID.String
	}

	if settlementDate.Valid {
		summary.SettlementDate = &settlementDate.Time
	}

	return &summary, nil
}
