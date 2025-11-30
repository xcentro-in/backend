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
	// Joins: payout_transfer_requests -> payment_vouchers -> business_payee_relation
	query := fmt.Sprintf(`
		SELECT
			ptr.transfer_request_id,
			ptr.created_at,
			COALESCE(bpr.display_name, 'Unknown') as recipient_name,
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
	// Query to fetch payout transfer summary with business, payee, bank, and provider details
	// Joins: payout_transfer_requests -> payment_vouchers -> business + business_payee_relation + payee_bank_accounts
	query := `
		SELECT
			b.bid::text as business_id,
			b.name as business_name,
			CONCAT(pv.business_id::text, ':', pv.payee_id::text, ':', pv.account_id::text) as payee_relation_id,
			COALESCE(bpr.display_name, 'Unknown') as payee_display_name,
			COALESCE(pba.bank_name, 'Unknown') as bank_name,
			COALESCE(RIGHT(pba.account_number_encrypted, 4), '****') as account_last_4,
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
		LEFT JOIN xcentro.payee_bank_accounts pba ON pv.account_id = pba.account_id
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
		&summary.BankName,
		&summary.AccountLast4,
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

// GetPayoutDetail retrieves complete detailed information for a payout transfer
func (r *PostgresOpsPayoutRepository) GetPayoutDetail(ctx context.Context, transferRequestID string) (*payout.OpsPayoutDetailResponse, error) {
	// Main query to get payout, voucher, payee, collection, and provider details
	query := `
		SELECT
			-- Payout overview
			ptr.transfer_request_id,
			ptr.status::text,
			ptr.amount,
			ptr.currency,
			ptr.created_at,
			ptr.updated_at,
			ptr.initiated_at,
			ptr.processing_at,
			ptr.completed_at,
			ptr.failed_at,
			COALESCE(ptr.failure_reason, '') as failure_reason,
			COALESCE(ptr.utr, '') as utr,
			
			-- Voucher details
			pv.voucher_id::text,
			pv.voucher_number,
			COALESCE(pv.invoice_number, '') as invoice_number,
			pv.net_amount as voucher_amount,
			pv.currency as voucher_currency,
			pv.created_at as voucher_created_at,
			
			-- Payee details
			pv.payee_id::text,
			COALESCE(bpr.display_name, 'Unknown') as payee_name,
			COALESCE(pm.payee_type::text, 'unknown') as payee_type,
			CONCAT(pv.business_id::text, ':', pv.payee_id::text, ':', pv.account_id::text) as relation_id,
			COALESCE(pba.bank_name, 'Unknown') as bank_name,
			COALESCE(RIGHT(pba.account_number_encrypted, 4), '****') as account_last_4,
			COALESCE(pba.ifsc_code, '') as ifsc_code,
			COALESCE(pba.account_holder_name, '') as account_holder_name,
			COALESCE(vendor.gstin, '') as gstin,
			
			-- Collection/Payment details
			p.payment_id::text,
			p.status::text as payment_status,
			p.amount as payment_amount,
			p.payment_method::text as payment_method,
			p.settlement_status::text as settlement_status,
			p.settled_at,
			
			-- Business details
			b.bid::text as business_id,
			b.name as business_name,
			
			-- Provider details
			ptr.provider,
			COALESCE(ptr.provider_transfer_id, '') as provider_transfer_id,
			ptr.provider_beneficiary_id,
			COALESCE(ptr.escrow_id, '') as escrow_id,
			COALESCE(ptr.transfer_mode, 'NEFT') as transfer_mode,
			COALESCE(ptr.purpose, '') as purpose,
			COALESCE(ptr.reference, '') as reference,
			COALESCE(ptr.narration, '') as narration,
			
			-- Retry info
			ptr.retry_count,
			ptr.max_retries
		FROM xcentro.payout_transfer_requests ptr
		INNER JOIN xcentro.payment_vouchers pv ON ptr.voucher_id = pv.voucher_id
		INNER JOIN xcentro.payments p ON ptr.payment_id = p.payment_id
		INNER JOIN xcentro.business b ON pv.business_id = b.bid
		LEFT JOIN xcentro.business_payee_relation bpr
			ON pv.business_id = bpr.business_id
			AND pv.payee_id = bpr.payee_id
			AND pv.account_id = bpr.account_id
		LEFT JOIN xcentro.payee_master pm ON pv.payee_id = pm.payee_id
		LEFT JOIN xcentro.payee_vendor vendor ON pm.payee_id = vendor.payee_id
		LEFT JOIN xcentro.payee_bank_accounts pba ON pv.account_id = pba.account_id
		WHERE ptr.transfer_request_id = $1
	`

	var detail payout.OpsPayoutDetailResponse
	var (
		updatedAt, initiatedAt, processingAt, completedAt, failedAt sql.NullTime
		settledAt sql.NullTime
	)

	err := r.querier.QueryRow(ctx, query, transferRequestID).Scan(
		// Payout overview
		&detail.TransferRequestID,
		&detail.Status,
		&detail.Amount,
		&detail.Currency,
		&detail.CreatedAt,
		&updatedAt,
		&initiatedAt,
		&processingAt,
		&completedAt,
		&failedAt,
		&detail.FailureReason,
		&detail.UTR,
		
		// Voucher details
		&detail.Voucher.VoucherID,
		&detail.Voucher.VoucherNumber,
		&detail.Voucher.InvoiceNumber,
		&detail.Voucher.Amount,
		&detail.Voucher.Currency,
		&detail.Voucher.CreatedAt,
		
		// Payee details
		&detail.Payee.PayeeID,
		&detail.Payee.PayeeName,
		&detail.Payee.PayeeType,
		&detail.Payee.RelationID,
		&detail.Payee.BankName,
		&detail.Payee.AccountNumberLast4,
		&detail.Payee.IFSCCode,
		&detail.Payee.AccountHolderName,
		&detail.Payee.GSTIN,
		
		// Collection details
		&detail.Collection.PaymentID,
		&detail.Collection.PaymentStatus,
		&detail.Collection.PaymentAmount,
		&detail.Collection.PaymentMethod,
		&detail.Collection.SettlementStatus,
		&settledAt,
		
		// Business details
		&detail.Collection.BusinessID,
		&detail.Collection.BusinessName,
		
		// Provider details
		&detail.Provider.Provider,
		&detail.Provider.ProviderTransferID,
		&detail.Provider.ProviderBeneficiaryID,
		&detail.Provider.EscrowID,
		&detail.Provider.TransferMode,
		&detail.Provider.Purpose,
		&detail.Provider.Reference,
		&detail.Provider.Narration,
		
		// Retry info
		&detail.RetryHistory.CurrentAttempt.AttemptNumber,
		&detail.RetryHistory.MaxRetries,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payout transfer not found: %s", transferRequestID)
		}
		return nil, fmt.Errorf("failed to get payout detail: %w", err)
	}

	// Handle nullable timestamp fields
	if updatedAt.Valid {
		detail.UpdatedAt = &updatedAt.Time
	}
	if initiatedAt.Valid {
		detail.InitiatedAt = &initiatedAt.Time
	}
	if processingAt.Valid {
		detail.ProcessingAt = &processingAt.Time
	}
	if completedAt.Valid {
		detail.CompletedAt = &completedAt.Time
	}
	if failedAt.Valid {
		detail.FailedAt = &failedAt.Time
	}
	if settledAt.Valid {
		detail.Collection.SettledAt = &settledAt.Time
	}

	// Set current attempt details
	detail.RetryHistory.CurrentAttempt.TransferRequestID = detail.TransferRequestID
	detail.RetryHistory.CurrentAttempt.Status = detail.Status
	detail.RetryHistory.CurrentAttempt.CreatedAt = detail.CreatedAt
	detail.RetryHistory.CurrentAttempt.FailedAt = detail.FailedAt
	detail.RetryHistory.CurrentAttempt.CompletedAt = detail.CompletedAt
	detail.RetryHistory.CurrentAttempt.FailureReason = detail.FailureReason
	detail.RetryHistory.CurrentAttempt.IsDeleted = false

	// Increment attempt number for display (0-indexed in DB, 1-indexed for UI)
	detail.RetryHistory.CurrentAttempt.AttemptNumber += 1

	// Get retry history (all attempts for this voucher)
	retryHistory, err := r.getRetryHistory(ctx, detail.Voucher.VoucherID, transferRequestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get retry history: %w", err)
	}

	detail.RetryHistory.PreviousAttempts = retryHistory
	detail.RetryHistory.TotalAttempts = len(retryHistory) + 1 // Including current
	detail.RetryHistory.CanRetry = detail.RetryHistory.CurrentAttempt.AttemptNumber < detail.RetryHistory.MaxRetries

	return &detail, nil
}

// getRetryHistory retrieves all previous attempts (soft deleted) for a voucher
func (r *PostgresOpsPayoutRepository) getRetryHistory(ctx context.Context, voucherID, currentTransferID string) ([]payout.OpsPayoutAttemptSummary, error) {
	query := `
		SELECT
			transfer_request_id,
			retry_count,
			status::text,
			created_at,
			failed_at,
			completed_at,
			COALESCE(failure_reason, '') as failure_reason,
			is_deleted,
			COALESCE(deleted_reason, '') as deleted_reason
		FROM xcentro.payout_transfer_requests
		WHERE voucher_id = $1
		  AND transfer_request_id != $2
		  AND is_deleted = true
		ORDER BY created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, voucherID, currentTransferID)
	if err != nil {
		return nil, fmt.Errorf("failed to query retry history: %w", err)
	}
	defer rows.Close()

	var attempts []payout.OpsPayoutAttemptSummary

	for rows.Next() {
		var attempt payout.OpsPayoutAttemptSummary
		var failedAt, completedAt sql.NullTime

		err := rows.Scan(
			&attempt.TransferRequestID,
			&attempt.AttemptNumber,
			&attempt.Status,
			&attempt.CreatedAt,
			&failedAt,
			&completedAt,
			&attempt.FailureReason,
			&attempt.IsDeleted,
			&attempt.DeletedReason,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan retry attempt: %w", err)
		}

		// Handle nullable timestamps
		if failedAt.Valid {
			attempt.FailedAt = &failedAt.Time
		}
		if completedAt.Valid {
			attempt.CompletedAt = &completedAt.Time
		}

		// Increment attempt number for display
		attempt.AttemptNumber += 1

		attempts = append(attempts, attempt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating retry history: %w", err)
	}

	return attempts, nil
}
