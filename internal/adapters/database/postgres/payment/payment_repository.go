package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
)

// PgPaymentRepository implements payment.PaymentRepository using PostgreSQL
type PgPaymentRepository struct {
	querier       client.Querier
	logger        *logger.Logger
	encryptionKey string
}

// NewPgPaymentRepository creates a new PostgreSQL payment repository
func NewPgPaymentRepository(db client.DB, encryptionKey string) *PgPaymentRepository {
	return &PgPaymentRepository{
		querier:       db,
		logger:        logger.GetDefault(),
		encryptionKey: encryptionKey,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPaymentRepository) WithTx(tx interface{}) payment.PaymentRepository {
	return &PgPaymentRepository{
		querier:       tx.(client.Tx),
		logger:        r.logger, // Preserve logger in transaction
		encryptionKey: r.encryptionKey,
	}
}

// CreatePayment creates a new payment record
func (r *PgPaymentRepository) CreatePayment(ctx context.Context, p *payment.Payment) error {
	// Log payment creation with source tracking
	sourceIDLog := "nil"
	sourceTypeLog := "nil"
	if p.PaymentSourceID != nil {
		sourceIDLog = *p.PaymentSourceID
	}
	if p.PaymentSourceType != nil {
		sourceTypeLog = string(*p.PaymentSourceType)
	}

	fmt.Printf("[PgPaymentRepository] CreatePayment called - BusinessID: %s, UserID: %s, Amount: %d, Currency: %s, Method: %s, Type: %s, SourceID: %s, SourceType: %s\n",
		p.BusinessID, p.UserID, p.Amount, p.Currency, p.PaymentMethod, p.PaymentType, sourceIDLog, sourceTypeLog)

	query := `
		INSERT INTO xcentro.payments (
			payment_id, user_id, business_id, amount, currency, payment_method, payment_type, status,
			description, callback_url, fee_amount, total_amount, metadata, payment_source_id, payment_source_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING created_at
	`

	// Serialize metadata to JSONB
	var metadataJSON []byte
	if p.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(p.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	err := r.querier.QueryRow(ctx, query,
		p.ID,                // $1 - payment_id (now explicitly provided)
		p.UserID,            // $2
		p.BusinessID,        // $3
		p.Amount,            // $4
		p.Currency,          // $5
		p.PaymentMethod,     // $6
		p.PaymentType,       // $7
		p.Status,            // $8
		p.Description,       // $9
		p.CallbackURL,       // $10
		p.FeeAmount,         // $11
		p.TotalAmount,       // $12
		metadataJSON,        // $13
		p.PaymentSourceID,   // $14 - CRITICAL: Must be set for tokenized payments
		p.PaymentSourceType, // $15 - CRITICAL: Must be set for tokenized payments
	).Scan(&p.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	fmt.Printf("[PgPaymentRepository] Payment created with ID: %s (source_id: %s, source_type: %s)\n", p.ID, sourceIDLog, sourceTypeLog)
	return nil
}

// GetPaymentByID retrieves a payment by its ID
func (r *PgPaymentRepository) GetPaymentByID(ctx context.Context, paymentID string) (*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentByID called - PaymentID: %s\n", paymentID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, total_amount,
		       metadata, created_at, updated_at
		FROM xcentro.payments WHERE payment_id = $1
	`

	var p payment.Payment
	var gatewayUsed, gatewayPaymentID, redirectURL, paymentSourceID, paymentSourceType *string
	var metadataJSON []byte
	var updatedAt *time.Time

	err := r.querier.QueryRow(ctx, query, paymentID).Scan(
		&p.ID,
		&p.UserID,
		&p.BusinessID,
		&p.Amount,
		&p.Currency,
		&p.PaymentMethod,
		&p.Status,
		&p.Description,
		&gatewayUsed,
		&gatewayPaymentID,
		&p.CallbackURL,
		&redirectURL,
		&paymentSourceID,
		&paymentSourceType,
		&p.FeeAmount,
		&p.TotalAmount,
		&metadataJSON,
		&p.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get payment by ID: %w", err)
	}

	// Handle NULL fields
	if gatewayUsed != nil {
		p.GatewayUsed = payment.GatewayType(*gatewayUsed)
	}
	if gatewayPaymentID != nil {
		p.GatewayPaymentID = *gatewayPaymentID
	}
	if redirectURL != nil {
		p.RedirectURL = *redirectURL
	}
	p.PaymentSourceID = paymentSourceID
	if paymentSourceType != nil {
		pst := payment.PaymentSourceType(*paymentSourceType)
		p.PaymentSourceType = &pst
	}
	if updatedAt != nil {
		p.UpdatedAt = *updatedAt
	}

	// Deserialize JSONB metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &p, nil
}

// GetPaymentByGatewayPaymentID retrieves payment by gateway's transaction ID or payment UUID
// Searches both gateway_payment_id and payment_id columns for maximum flexibility
// This enables webhook matching for:
//   - Regular payments: where gateway_payment_id = gateway's txn ID
//   - Tokenization payments: where txnid = our payment UUID
func (r *PgPaymentRepository) GetPaymentByGatewayPaymentID(ctx context.Context, gatewayPaymentID string) (*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentByGatewayPaymentID called - GatewayPaymentID: %s\n", gatewayPaymentID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, total_amount,
		       metadata, created_at, updated_at
		FROM xcentro.payments
		WHERE gateway_payment_id = $1 OR payment_id = $1
	`

	var p payment.Payment
	var gatewayUsed, gatewayPaymentIDNullable, redirectURL, paymentSourceID, paymentSourceType *string
	var metadataJSON []byte
	var updatedAt *time.Time

	err := r.querier.QueryRow(ctx, query, gatewayPaymentID).Scan(
		&p.ID,
		&p.UserID,
		&p.BusinessID,
		&p.Amount,
		&p.Currency,
		&p.PaymentMethod,
		&p.Status,
		&p.Description,
		&gatewayUsed,
		&gatewayPaymentIDNullable,
		&p.CallbackURL,
		&redirectURL,
		&paymentSourceID,
		&paymentSourceType,
		&p.FeeAmount,
		&p.TotalAmount,
		&metadataJSON,
		&p.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get payment by gateway payment ID: %w", err)
	}

	// Handle NULL fields
	if gatewayUsed != nil {
		p.GatewayUsed = payment.GatewayType(*gatewayUsed)
	}
	if gatewayPaymentIDNullable != nil {
		p.GatewayPaymentID = *gatewayPaymentIDNullable
	}
	if redirectURL != nil {
		p.RedirectURL = *redirectURL
	}
	p.PaymentSourceID = paymentSourceID
	if paymentSourceType != nil {
		pst := payment.PaymentSourceType(*paymentSourceType)
		p.PaymentSourceType = &pst
	}
	if updatedAt != nil {
		p.UpdatedAt = *updatedAt
	}

	// Deserialize JSONB metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &p, nil
}

// UpdatePaymentStatus updates payment status and metadata
func (r *PgPaymentRepository) UpdatePaymentStatus(ctx context.Context, paymentID string, status payment.PaymentStatus, metadata map[string]any) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentStatus called - PaymentID: %s, Status: %s\n", paymentID, status)

	query := `
		UPDATE xcentro.payments
		SET status = $1, metadata = COALESCE(metadata, '{}'::jsonb) || $2::jsonb, updated_at = NOW()
		WHERE payment_id = $3
	`

	// Serialize metadata to JSONB
	var metadataJSON []byte
	if metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	_, err := r.querier.Exec(ctx, query, status, metadataJSON, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	return nil
}

// UpdatePaymentGateway updates the selected gateway
func (r *PgPaymentRepository) UpdatePaymentGateway(ctx context.Context, paymentID string, gateway payment.GatewayType) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentGateway called - PaymentID: %s, Gateway: %s\n", paymentID, gateway)

	query := `UPDATE xcentro.payments SET gateway_used = $1, updated_at = NOW() WHERE payment_id = $2`
	_, err := r.querier.Exec(ctx, query, gateway, paymentID)
	return err
}

// UpdateGatewayPaymentID updates gateway transaction ID and redirect URL
func (r *PgPaymentRepository) UpdateGatewayPaymentID(ctx context.Context, paymentID string, gatewayPaymentID, redirectURL string) error {
	fmt.Printf("[PgPaymentRepository] UpdateGatewayPaymentID called - PaymentID: %s, GatewayPaymentID: %s\n", paymentID, gatewayPaymentID)

	query := `UPDATE xcentro.payments SET gateway_payment_id = $1, redirect_url = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, gatewayPaymentID, redirectURL, paymentID)
	return err
}

// UpdatePaymentFees updates fee breakdown
func (r *PgPaymentRepository) UpdatePaymentFees(ctx context.Context, paymentID string, feeAmount, totalAmount int64) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentFees called - PaymentID: %s, FeeAmount: %d, TotalAmount: %d\n", paymentID, feeAmount, totalAmount)

	query := `UPDATE xcentro.payments SET fee_amount = $1, total_amount = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, feeAmount, totalAmount, paymentID)
	return err
}

// UpdatePaymentSource links payment to a payment source
func (r *PgPaymentRepository) UpdatePaymentSource(ctx context.Context, paymentID string, sourceID string, sourceType payment.PaymentSourceType) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentSource called - PaymentID: %s, SourceID: %s, SourceType: %s\n", paymentID, sourceID, sourceType)

	query := `UPDATE xcentro.payments SET payment_source_id = $1, payment_source_type = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, sourceID, sourceType, paymentID)
	return err
}

// GetPaymentsByBusinessID retrieves payments for a business with pagination
func (r *PgPaymentRepository) GetPaymentsByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentsByBusinessID called - BusinessID: %s, Limit: %d, Offset: %d\n", businessID, limit, offset)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, total_amount,
		       metadata, created_at, updated_at
		FROM xcentro.payments
		WHERE business_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, businessID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query payments: %w", err)
	}
	defer rows.Close()

	var payments []*payment.Payment
	for rows.Next() {
		var p payment.Payment
		var gatewayUsed, gatewayPaymentID, redirectURL, paymentSourceID, paymentSourceType *string
		var metadataJSON []byte
		var updatedAt *time.Time

		err := rows.Scan(
			&p.ID,
			&p.UserID,
			&p.BusinessID,
			&p.Amount,
			&p.Currency,
			&p.PaymentMethod,
			&p.Status,
			&p.Description,
			&gatewayUsed,
			&gatewayPaymentID,
			&p.CallbackURL,
			&redirectURL,
			&paymentSourceID,
			&paymentSourceType,
			&p.FeeAmount,
			&p.TotalAmount,
			&metadataJSON,
			&p.CreatedAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}

		// Handle NULL fields
		if gatewayUsed != nil {
			p.GatewayUsed = payment.GatewayType(*gatewayUsed)
		}
		if gatewayPaymentID != nil {
			p.GatewayPaymentID = *gatewayPaymentID
		}
		if redirectURL != nil {
			p.RedirectURL = *redirectURL
		}
		p.PaymentSourceID = paymentSourceID
		if paymentSourceType != nil {
			pst := payment.PaymentSourceType(*paymentSourceType)
			p.PaymentSourceType = &pst
		}
		if updatedAt != nil {
			p.UpdatedAt = *updatedAt
		}

		// Deserialize JSONB metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		payments = append(payments, &p)
	}

	return payments, nil
}

// ============================================================================
// Settlement Tracking Methods (Phase 7)
// ============================================================================

// UpdateSettlementStatus updates settlement status and related metadata
func (r *PgPaymentRepository) UpdateSettlementStatus(ctx context.Context, paymentID string, status payment.SettlementStatus, metadata map[string]any) error {
	fmt.Printf("[PgPaymentRepository] UpdateSettlementStatus called - PaymentID: %s, SettlementStatus: %s\n", paymentID, status)

	query := `
		UPDATE xcentro.payments
		SET settlement_status = $1,
		    settlement_metadata = COALESCE(settlement_metadata, '{}'::jsonb) || $2::jsonb,
		    settled_at = CASE WHEN $1 = 'settled' THEN NOW() ELSE settled_at END,
		    updated_at = NOW()
		WHERE payment_id = $3
	`

	// Serialize metadata to JSONB
	var metadataJSON []byte
	if metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal settlement metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	_, err := r.querier.Exec(ctx, query, status, metadataJSON, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update settlement status: %w", err)
	}

	return nil
}

// UpdateSettlementAmounts updates settlement financial fields
func (r *PgPaymentRepository) UpdateSettlementAmounts(ctx context.Context, paymentID string, settlementAmount, mdrAmount, netSettlementAmount int64) error {
	fmt.Printf("[PgPaymentRepository] UpdateSettlementAmounts called - PaymentID: %s, SettlementAmount: %d, MDR: %d, Net: %d\n",
		paymentID, settlementAmount, mdrAmount, netSettlementAmount)

	query := `
		UPDATE xcentro.payments
		SET settlement_amount = $1,
		    mdr_amount = $2,
		    net_settlement_amount = $3,
		    updated_at = NOW()
		WHERE payment_id = $4
	`

	_, err := r.querier.Exec(ctx, query, settlementAmount, mdrAmount, netSettlementAmount, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update settlement amounts: %w", err)
	}

	return nil
}

// GetPendingSettlements retrieves payments pending settlement for a business
func (r *PgPaymentRepository) GetPendingSettlements(ctx context.Context, businessID string) ([]*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPendingSettlements called - BusinessID: %s\n", businessID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, total_amount,
		       metadata, created_at, updated_at
		FROM xcentro.payments
		WHERE business_id = $1
		  AND status = 'success'
		  AND settlement_status = 'pending'
		ORDER BY created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending settlements: %w", err)
	}
	defer rows.Close()

	var payments []*payment.Payment
	for rows.Next() {
		var p payment.Payment
		var gatewayUsed, gatewayPaymentID, redirectURL, paymentSourceID, paymentSourceType *string
		var metadataJSON []byte
		var updatedAt *time.Time

		err := rows.Scan(
			&p.ID,
			&p.UserID,
			&p.BusinessID,
			&p.Amount,
			&p.Currency,
			&p.PaymentMethod,
			&p.Status,
			&p.Description,
			&gatewayUsed,
			&gatewayPaymentID,
			&p.CallbackURL,
			&redirectURL,
			&paymentSourceID,
			&paymentSourceType,
			&p.FeeAmount,
			&p.TotalAmount,
			&metadataJSON,
			&p.CreatedAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}

		// Handle NULL fields
		if gatewayUsed != nil {
			p.GatewayUsed = payment.GatewayType(*gatewayUsed)
		}
		if gatewayPaymentID != nil {
			p.GatewayPaymentID = *gatewayPaymentID
		}
		if redirectURL != nil {
			p.RedirectURL = *redirectURL
		}
		p.PaymentSourceID = paymentSourceID
		if paymentSourceType != nil {
			pst := payment.PaymentSourceType(*paymentSourceType)
			p.PaymentSourceType = &pst
		}
		if updatedAt != nil {
			p.UpdatedAt = *updatedAt
		}

		// Deserialize JSONB metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		payments = append(payments, &p)
	}

	return payments, nil
}

// ============================================================================
// Payment List Methods (for display/dashboard)
// ============================================================================

// GetPaymentListForBusiness retrieves payments with aggregated voucher and payee information
// Optimized for payment history display with bill breakdown
func (r *PgPaymentRepository) GetPaymentListForBusiness(ctx context.Context, businessID string, limit, offset int) ([]payment.PaymentListItem, int, error) {
	// First, get total count
	countQuery := `
		SELECT COUNT(DISTINCT p.payment_id)
		FROM xcentro.payments p
		WHERE p.business_id = $1
		  AND p.status IN ('success', 'pending', 'failed')
		  AND p.payment_type = 'payment'
	`

	var totalCount int
	err := r.querier.QueryRow(ctx, countQuery, businessID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get payment count: %w", err)
	}

	// Main query to fetch payments with aggregated data
	query := `
		WITH payment_vouchers_finalized AS (
			-- Vouchers that have been finalized (success payments)
			SELECT
				v.payment_id,
				v.voucher_id,
				v.invoice_number,
				v.net_amount,
				bpr.display_name as payee_name
			FROM xcentro.payment_vouchers v
			INNER JOIN xcentro.business_payee_relation bpr
				ON v.payee_id = bpr.payee_id
				AND v.account_id = bpr.account_id
			WHERE v.payment_id IS NOT NULL
		),
		payment_vouchers_pending AS (
			-- Vouchers that are still pending (pending/failed payments)
			SELECT
				pvp.payment_id,
				v.voucher_id,
				v.invoice_number,
				v.net_amount,
				bpr.display_name as payee_name
			FROM xcentro.payment_voucher_pending pvp
			INNER JOIN xcentro.payment_vouchers v ON pvp.voucher_id = v.voucher_id
			INNER JOIN xcentro.business_payee_relation bpr
				ON v.payee_id = bpr.payee_id
				AND v.account_id = bpr.account_id
		),
		payment_aggregates AS (
			SELECT
				p.payment_id,
				CASE
					WHEN p.status = 'success' THEN (
						SELECT ARRAY_AGG(DISTINCT pvf.payee_name)
						FROM payment_vouchers_finalized pvf
						WHERE pvf.payment_id = p.payment_id
					)
					ELSE (
						SELECT ARRAY_AGG(DISTINCT pvp.payee_name)
						FROM payment_vouchers_pending pvp
						WHERE pvp.payment_id = p.payment_id
					)
				END as payees,
				CASE
					WHEN p.status = 'success' THEN (
						SELECT COUNT(pvf.voucher_id)
						FROM payment_vouchers_finalized pvf
						WHERE pvf.payment_id = p.payment_id
					)
					ELSE (
						SELECT COUNT(pvp.voucher_id)
						FROM payment_vouchers_pending pvp
						WHERE pvp.payment_id = p.payment_id
					)
				END as bill_count,
				CASE
					WHEN p.status = 'success' THEN (
						SELECT JSONB_AGG(
							JSONB_BUILD_OBJECT(
								'voucherId', pvf.voucher_id,
								'billId', pvf.invoice_number,
								'amount', pvf.net_amount
							)
						)
						FROM payment_vouchers_finalized pvf
						WHERE pvf.payment_id = p.payment_id
					)
					ELSE (
						SELECT JSONB_AGG(
							JSONB_BUILD_OBJECT(
								'voucherId', pvp.voucher_id,
								'billId', pvp.invoice_number,
								'amount', pvp.net_amount
							)
						)
						FROM payment_vouchers_pending pvp
						WHERE pvp.payment_id = p.payment_id
					)
				END as bills
			FROM xcentro.payments p
			WHERE p.business_id = $1
			  AND p.status IN ('success', 'pending', 'failed')
			  AND p.payment_type = 'payment'
		)
		SELECT
			p.payment_id,
			p.created_at,
			p.payment_method,
			p.total_amount,
			p.amount,
			p.fee_amount,
			p.status,
			p.payment_source_id,
			p.payment_source_type,
			p.retry_of_payment_id,
			p.retry_count,
			EXISTS(
				SELECT 1 FROM xcentro.payments p2
				WHERE p2.retry_of_payment_id = p.payment_id
				AND p2.status = 'success'
			) as has_successful_retry,
			COALESCE(pa.payees, ARRAY[]::TEXT[]) as payees,
			COALESCE(pa.bill_count, 0) as bill_count,
			COALESCE(pa.bills, '[]'::JSONB) as bills,
			-- Card details (if payment source is card)
			CASE
				WHEN sc.card_type = 'visa' THEN 'Visa'
				WHEN sc.card_type = 'mastercard' THEN 'Mastercard'
				WHEN sc.card_type = 'amex' THEN 'Amex'
				WHEN sc.card_type = 'rupay' THEN 'RuPay'
				WHEN sc.card_type = 'diners' THEN 'Diners'
				ELSE NULL
			END as card_provider,
			sc.last_four_digits as card_last4,
			-- Bank account details (if payment source is bank_account)
			ba.bank_name,
			ba.account_number_encrypted
		FROM xcentro.payments p
		LEFT JOIN payment_aggregates pa ON p.payment_id = pa.payment_id
		LEFT JOIN xcentro.stored_cards sc
			ON p.payment_source_id = sc.card_id
			AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba
			ON p.payment_source_id = ba.account_id
			AND p.payment_source_type = 'bank_account'
		WHERE p.business_id = $1
		  AND p.status IN ('success', 'pending', 'failed')
		  AND p.payment_type = 'payment'
		ORDER BY p.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, businessID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query payment list: %w", err)
	}
	defer rows.Close()

	var items []payment.PaymentListItem
	for rows.Next() {
		var item payment.PaymentListItem
		var paymentMethod string
		var totalAmount, baseAmount, feeAmount int64
		var status string
		var paymentSourceID, paymentSourceType *string
		var retryOfPaymentID *string
		var retryCount int
		var hasSuccessfulRetry bool
		var payees []string
		var billsJSON []byte
		var cardProvider, cardLast4, bankName, accountNumberEncrypted *string

		err := rows.Scan(
			&item.PaymentID,
			&item.Date,
			&paymentMethod,
			&totalAmount,
			&baseAmount,
			&feeAmount,
			&status,
			&paymentSourceID,
			&paymentSourceType,
			&retryOfPaymentID,
			&retryCount,
			&hasSuccessfulRetry,
			&payees,
			&item.BillCount,
			&billsJSON,
			&cardProvider,
			&cardLast4,
			&bankName,
			&accountNumberEncrypted,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan payment list item: %w", err)
		}

		// Convert amounts from paise to rupees
		item.TotalAmount = float64(totalAmount) / 100.0
		convenienceFee := float64(feeAmount) / 100.0

		// Calculate GST on fee (18% of convenience fee)
		gstOnFee := convenienceFee * 0.18

		// Map status
		item.Status = status

		// Set payees
		item.Payees = payees

		// Parse payment method details using data from JOIN (no separate queries!)
		item.PaymentMethod = buildPaymentMethodDetails(
			paymentMethod,
			cardProvider,
			cardLast4,
			bankName,
			accountNumberEncrypted,
			r.encryptionKey,
		)

		// Parse bills
		var bills []payment.BillItem
		if len(billsJSON) > 0 {
			if err := json.Unmarshal(billsJSON, &bills); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal bills: %w", err)
			}
			// Convert bill amounts from paise to rupees
			for i := range bills {
				bills[i].Amount = bills[i].Amount / 100.0
			}
		}

		item.Breakdown = payment.PaymentBreakdown{
			Bills:          bills,
			ConvenienceFee: convenienceFee,
			GSTOnFee:       gstOnFee,
		}

		// Set retry-related fields
		item.RetryOfPaymentID = retryOfPaymentID
		item.RetryCount = retryCount
		item.HasSuccessfulRetry = hasSuccessfulRetry

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating payment list rows: %w", err)
	}

	return items, totalCount, nil
}

// mapPaymentStatus maps internal payment status to frontend-friendly status
func mapPaymentStatus(status string) string {
	switch status {
	case "success":
		return "Completed"
	case "pending":
		return "In Progress"
	case "failed":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// buildPaymentMethodDetails builds payment method details from JOIN data (no separate queries)
func buildPaymentMethodDetails(
	method string,
	cardProvider *string,
	cardLast4 *string,
	bankName *string,
	accountNumberEncrypted *string,
	encryptionKey string,
) payment.PaymentMethodDetails {
	details := payment.PaymentMethodDetails{}

	switch method {
	case "card", "token_card":
		details.Type = "credit"
		if cardProvider != nil {
			details.CardProvider = *cardProvider
		}
		if cardLast4 != nil {
			details.Last4 = *cardLast4
		}
	case "net_banking":
		details.Type = "bank"
		if bankName != nil {
			details.Bank = *bankName
		}
		if accountNumberEncrypted != nil {
			// Decrypt and extract last 4 digits
			decrypted, err := crypto.DecryptBankAccountNumber(*accountNumberEncrypted, encryptionKey)
			if err == nil {
				details.Last4 = crypto.ExtractLastDigits(decrypted, 4)
			} else {
				details.Last4 = "****"
			}
		}
	case "upi":
		details.Type = "upi"
		details.Last4 = "UPI"
	default:
		details.Type = "other"
	}

	return details
}

// extractPaymentMethodDetails is kept for backward compatibility (single payment queries)
// For list queries, use buildPaymentMethodDetails which uses JOIN data
func extractPaymentMethodDetails(method string, sourceID, sourceType *string, ctx context.Context, r *PgPaymentRepository) payment.PaymentMethodDetails {
	details := payment.PaymentMethodDetails{}

	// Log the inputs for debugging
	sourceIDStr := "nil"
	sourceTypeStr := "nil"
	if sourceID != nil {
		sourceIDStr = *sourceID
	}
	if sourceType != nil {
		sourceTypeStr = *sourceType
	}
	r.logger.DebugWith("Extracting payment method details",
		"method", method,
		"source_id", sourceIDStr,
		"source_type", sourceTypeStr)

	switch method {
	case "card", "token_card":
		details.Type = "credit"
		if sourceID != nil && sourceType != nil && *sourceType == "card" {
			// Fetch card details from stored_card_repository
			cardDetails := fetchCardDetails(ctx, r, *sourceID)
			if cardDetails != nil {
				details.CardProvider = cardDetails.Provider
				details.Last4 = cardDetails.Last4
				r.logger.DebugWith("Fetched card details",
					"provider", cardDetails.Provider,
					"last4", cardDetails.Last4)
			} else {
				r.logger.DebugWith("Card details fetch returned nil")
			}
		} else {
			r.logger.DebugWith("Skipping card details fetch - missing source info or wrong type")
		}
	case "net_banking":
		details.Type = "bank"
		if sourceID != nil && sourceType != nil && *sourceType == "bank_account" {
			// Fetch bank account details
			bankDetails := fetchBankAccountDetails(ctx, r, *sourceID)
			if bankDetails != nil {
				details.Bank = bankDetails.BankName
				details.Last4 = bankDetails.Last4
			}
		}
	case "upi":
		details.Type = "upi"
		details.Last4 = "UPI"
	default:
		r.logger.InfoWith("Unknown payment method, defaulting to 'other'", "method", method)
		details.Type = "other"
	}

	return details
}

// Helper structs for card and bank details
type cardDetailsResult struct {
	Provider string
	Last4    string
}

type bankDetailsResult struct {
	BankName string
	Last4    string
}

// fetchCardDetails fetches card information from stored cards
func fetchCardDetails(ctx context.Context, r *PgPaymentRepository, cardID string) *cardDetailsResult {
	query := `
		SELECT
			CASE
				WHEN card_type = 'visa' THEN 'Visa'
				WHEN card_type = 'mastercard' THEN 'Mastercard'
				WHEN card_type = 'amex' THEN 'Amex'
				WHEN card_type = 'rupay' THEN 'RuPay'
				WHEN card_type = 'diners' THEN 'Diners'
				ELSE 'Card'
			END as provider,
			last_four_digits
		FROM xcentro.stored_cards
		WHERE card_id = $1
	`

	var result cardDetailsResult
	err := r.querier.QueryRow(ctx, query, cardID).Scan(&result.Provider, &result.Last4)
	if err != nil {
		r.logger.ErrorWith("Failed to fetch card details",
			"error", err,
			"card_id", cardID)
		return &cardDetailsResult{Provider: "Card", Last4: "****"}
	}
	return &result
}

// fetchBankAccountDetails fetches bank account information
func fetchBankAccountDetails(ctx context.Context, r *PgPaymentRepository, accountID string) *bankDetailsResult {
	query := `
		SELECT
			bank_name,
			account_number_encrypted
		FROM xcentro.bank_accounts
		WHERE account_id = $1
	`

	var bankName, encryptedAccountNumber string
	err := r.querier.QueryRow(ctx, query, accountID).Scan(&bankName, &encryptedAccountNumber)
	if err != nil {
		return &bankDetailsResult{BankName: "Bank", Last4: "****"}
	}

	// Decrypt account number to extract last 4 digits
	decryptedAccountNumber, err := crypto.DecryptBankAccountNumber(encryptedAccountNumber, r.encryptionKey)
	if err != nil {
		r.logger.ErrorWith("Failed to decrypt account number for payment display",
			"account_id", accountID,
			"error", err)
		return &bankDetailsResult{BankName: "Bank", Last4: "****"} // Return masked fallback on error
	}

	last4 := crypto.ExtractLastDigits(decryptedAccountNumber, 4)
	return &bankDetailsResult{BankName: bankName, Last4: last4}
}
