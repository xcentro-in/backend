package voucher

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/crypto"
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
	// Fetch encrypted bank account and decrypt in Go code
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
			COALESCE(sc.last_four_digits, '') as card_last_four,
			COALESCE(ba.account_number_encrypted, '') as bank_account_encrypted
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
		var cardLastFour string
		var bankAccountEncrypted string

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
			&cardLastFour,
			&bankAccountEncrypted,
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

			// Decrypt and extract last digits based on payment source type
			if paymentSourceType.String == "card" && cardLastFour != "" {
				v.PaymentSourceDigits = cardLastFour
			} else if paymentSourceType.String == "bank_account" && bankAccountEncrypted != "" {
				// Decrypt bank account to get last digits (with backwards compatibility for plaintext)
				decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(bankAccountEncrypted, r.encryptionKey)
				if err != nil {
					// If decryption fails, assume it's legacy plaintext data
					// TODO: Remove this fallback after migrating all accounts to encrypted format
					fmt.Printf("[VoucherRepositoryExtension] WARN: Failed to decrypt bank account for voucher %s, treating as plaintext: %v\n", v.VoucherID, err)
					decryptedAccountNumber = bankAccountEncrypted
				}
				v.PaymentSourceDigits = crypto.ExtractLastDigits(decryptedAccountNumber, 5)
			}
		}

		vouchersList = append(vouchersList, v)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating voucher rows: %w", err)
	}

	return vouchersList, totalCount, nil
}
