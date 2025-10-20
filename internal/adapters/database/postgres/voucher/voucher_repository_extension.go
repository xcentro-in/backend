package voucher

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
)

// GetVouchersByRelation retrieves all vouchers for a specific payee relation with payment method details
// Validates that the relation belongs to the business (security check via business_payee_relation join)
func (r *PostgresVoucherRepository) GetVouchersByRelation(ctx context.Context, businessID, relationID string, limit, offset int) ([]*voucher.VoucherWithPaymentInfo, int64, error) {
	// First, get the count for pagination
	countQuery := `
		SELECT COUNT(*)
		FROM xcentro.payment_vouchers pv
		INNER JOIN xcentro.business_payee_relation bpr
			ON pv.business_id = bpr.business_id
			AND pv.payee_id = bpr.payee_id
			AND pv.account_id = bpr.account_id
		WHERE bpr.business_id = $1::uuid
			AND bpr.relation_id = $2::uuid
			AND bpr.is_active = TRUE
	`

	var totalCount int64
	err := r.querier.QueryRow(ctx, countQuery, businessID, relationID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count vouchers by relation: %w", err)
	}

	// Main query with LEFT JOINs to payments, stored_cards, and bank_accounts
	// Note: account_number_encrypted currently stores plaintext (TODO-PHASE-4.5 for actual encryption)
	query := `
		SELECT
			pv.voucher_id,
			COALESCE(pv.invoice_number, '') as invoice_number,
			pv.net_amount,
			pv.created_at::text as created_at,
			pv.status,
			COALESCE(pv.description, '') as description,
			pv.payment_id,
			p.status as payment_status,
			p.payment_source_type,
			CASE
				WHEN p.payment_source_type = 'card' THEN sc.last_four_digits
				WHEN p.payment_source_type = 'bank_account' THEN
					CASE
						WHEN LENGTH(ba.account_number_encrypted) >= 5
						THEN RIGHT(ba.account_number_encrypted, 5)
						ELSE ba.account_number_encrypted
					END
				ELSE ''
			END as source_digits
		FROM xcentro.payment_vouchers pv
		INNER JOIN xcentro.business_payee_relation bpr
			ON pv.business_id = bpr.business_id
			AND pv.payee_id = bpr.payee_id
			AND pv.account_id = bpr.account_id
		LEFT JOIN xcentro.payments p ON pv.payment_id = p.payment_id
		LEFT JOIN xcentro.stored_cards sc
			ON p.payment_source_id = sc.card_id
			AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba
			ON p.payment_source_id = ba.account_id
			AND p.payment_source_type = 'bank_account'
		WHERE bpr.business_id = $1::uuid
			AND bpr.relation_id = $2::uuid
			AND bpr.is_active = TRUE
		ORDER BY pv.created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.querier.Query(ctx, query, businessID, relationID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query vouchers by relation: %w", err)
	}
	defer rows.Close()

	var vouchersList []*voucher.VoucherWithPaymentInfo
	for rows.Next() {
		v := &voucher.VoucherWithPaymentInfo{}
		var createdAtStr string
		var paymentID sql.NullString
		var paymentStatus sql.NullString
		var paymentSourceType sql.NullString
		var sourceDigits sql.NullString

		err := rows.Scan(
			&v.VoucherID,
			&v.InvoiceNumber,
			&v.NetAmount,
			&createdAtStr,
			&v.Status,
			&v.Description,
			&paymentID,
			&paymentStatus,
			&paymentSourceType,
			&sourceDigits,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan voucher row: %w", err)
		}

		// Parse created_at timestamp
		createdAt, err := time.Parse("2006-01-02 15:04:05.999999-07", createdAtStr)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse created_at: %w", err)
		}
		v.CreatedAt = createdAt

		// Handle nullable payment fields
		if paymentID.Valid {
			paymentUUID, err := uuid.Parse(paymentID.String)
			if err == nil {
				v.PaymentID = &paymentUUID
			}
		}
		if paymentStatus.Valid {
			v.PaymentStatus = paymentStatus.String
		}
		if paymentSourceType.Valid {
			v.PaymentSourceType = paymentSourceType.String
		}
		if sourceDigits.Valid {
			v.PaymentSourceDigits = sourceDigits.String
		}

		vouchersList = append(vouchersList, v)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchersList, totalCount, nil
}
