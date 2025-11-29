package payment

import (
	"context"
	"database/sql"
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

	r.logger.DebugWith("Creating payment",
		"business_id", p.BusinessID,
		"user_id", p.UserID,
		"amount", p.Amount,
		"currency", p.Currency,
		"payment_method", p.PaymentMethod,
		"payment_type", p.PaymentType,
		"source_id", sourceIDLog,
		"source_type", sourceTypeLog)

	query := `
		INSERT INTO xcentro.payments (
			payment_id, user_id, business_id, amount, currency, payment_method, payment_type, status,
			description, callback_url, fee_amount, convenience_fee, convenience_fee_gst, total_amount,
			metadata, payment_source_id, payment_source_type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
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
		p.ID,                 // $1 - payment_id (now explicitly provided)
		p.UserID,             // $2
		p.BusinessID,         // $3
		p.Amount,             // $4
		p.Currency,           // $5
		p.PaymentMethod,      // $6
		p.PaymentType,        // $7
		p.Status,             // $8
		p.Description,        // $9
		p.CallbackURL,        // $10
		p.FeeAmount,          // $11 - Total fee (kept for backward compatibility)
		p.ConvenienceFee,     // $12 - Base convenience fee before GST
		p.ConvenienceFeeGST,  // $13 - GST on convenience fee (18%)
		p.TotalAmount,        // $14
		metadataJSON,         // $15
		p.PaymentSourceID,    // $16 - CRITICAL: Must be set for tokenized payments
		p.PaymentSourceType,  // $17 - CRITICAL: Must be set for tokenized payments
	).Scan(&p.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	r.logger.InfoWith("Payment created",
		"payment_id", p.ID,
		"source_id", sourceIDLog,
		"source_type", sourceTypeLog)

	return nil
}

// GetPaymentByID retrieves a payment by its ID
func (r *PgPaymentRepository) GetPaymentByID(ctx context.Context, paymentID string) (*payment.Payment, error) {
	r.logger.DebugWith("Getting payment by ID", "payment_id", paymentID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, payment_type, status,
		       description, gateway_used::text, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, retry_of_payment_id, retry_count,
		       fee_amount, convenience_fee, convenience_fee_gst, total_amount, metadata,
		       settlement_status, settled_at, settlement_reference_id, settlement_utr, settlement_metadata,
		       settlement_amount, mdr_amount, net_settlement_amount,
		       created_at, updated_at
		FROM xcentro.payments WHERE payment_id = $1
	`

	var p payment.Payment
	var gatewayUsed, gatewayPaymentID, redirectURL, paymentSourceID, paymentSourceType, retryOfPaymentID *string
	var retryCount *int
	var metadataJSON, settlementMetadataJSON []byte
	var updatedAt, settledAt *time.Time
	var settlementStatus *string
	var settlementReferenceID, settlementUTR *string
	var settlementAmount, netSettlementAmount *int64

	err := r.querier.QueryRow(ctx, query, paymentID).Scan(
		&p.ID,
		&p.UserID,
		&p.BusinessID,
		&p.Amount,
		&p.Currency,
		&p.PaymentMethod,
		&p.PaymentType,
		&p.Status,
		&p.Description,
		&gatewayUsed,
		&gatewayPaymentID,
		&p.CallbackURL,
		&redirectURL,
		&paymentSourceID,
		&paymentSourceType,
		&retryOfPaymentID,
		&retryCount,
		&p.FeeAmount,
		&p.ConvenienceFee,
		&p.ConvenienceFeeGST,
		&p.TotalAmount,
		&metadataJSON,
		&settlementStatus,
		&settledAt,
		&settlementReferenceID,
		&settlementUTR,
		&settlementMetadataJSON,
		&settlementAmount,
		&p.MDRAmount,
		&netSettlementAmount,
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
	p.RetryOfPaymentID = retryOfPaymentID
	p.RetryCount = retryCount
	if updatedAt != nil {
		p.UpdatedAt = *updatedAt
	}

	// Deserialize JSONB metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &p.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Handle settlement fields
	if settlementStatus != nil {
		p.SettlementStatus = payment.SettlementStatus(*settlementStatus)
	}
	p.SettledAt = settledAt
	if settlementReferenceID != nil {
		p.SettlementReferenceID = *settlementReferenceID
	}
	if settlementUTR != nil {
		p.SettlementUTR = *settlementUTR
	}
	if len(settlementMetadataJSON) > 0 {
		if err := json.Unmarshal(settlementMetadataJSON, &p.SettlementMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settlement metadata: %w", err)
		}
	}
	p.SettlementAmount = settlementAmount
	p.NetSettlementAmount = netSettlementAmount

	return &p, nil
}

// GetPaymentByGatewayPaymentID retrieves payment by gateway's transaction ID or payment UUID
// Searches both gateway_payment_id and payment_id columns for maximum flexibility
// This enables webhook matching for:
//   - Regular payments: where gateway_payment_id = gateway's txn ID
//   - Tokenization payments: where txnid = our payment UUID
func (r *PgPaymentRepository) GetPaymentByGatewayPaymentID(ctx context.Context, gatewayPaymentID string) (*payment.Payment, error) {
	r.logger.DebugWith("Getting payment by gateway payment ID", "gateway_payment_id", gatewayPaymentID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, payment_type, status,
		       description, gateway_used::text, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, convenience_fee, convenience_fee_gst,
		       total_amount, metadata,
		       settlement_status, settled_at, settlement_reference_id, settlement_utr, settlement_metadata,
		       settlement_amount, mdr_amount, net_settlement_amount,
		       created_at, updated_at
		FROM xcentro.payments
		WHERE gateway_payment_id = $1 OR payment_id = $1
	`

	var p payment.Payment
	var gatewayUsed, gatewayPaymentIDNullable, redirectURL, paymentSourceID, paymentSourceType *string
	var metadataJSON, settlementMetadataJSON []byte
	var updatedAt, settledAt *time.Time
	var settlementStatus *string
	var settlementReferenceID, settlementUTR *string
	var settlementAmount, netSettlementAmount *int64

	err := r.querier.QueryRow(ctx, query, gatewayPaymentID).Scan(
		&p.ID,
		&p.UserID,
		&p.BusinessID,
		&p.Amount,
		&p.Currency,
		&p.PaymentMethod,
		&p.PaymentType,
		&p.Status,
		&p.Description,
		&gatewayUsed,
		&gatewayPaymentIDNullable,
		&p.CallbackURL,
		&redirectURL,
		&paymentSourceID,
		&paymentSourceType,
		&p.FeeAmount,
		&p.ConvenienceFee,
		&p.ConvenienceFeeGST,
		&p.TotalAmount,
		&metadataJSON,
		&settlementStatus,
		&settledAt,
		&settlementReferenceID,
		&settlementUTR,
		&settlementMetadataJSON,
		&settlementAmount,
		&p.MDRAmount,
		&netSettlementAmount,
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

	// Handle settlement fields
	if settlementStatus != nil {
		p.SettlementStatus = payment.SettlementStatus(*settlementStatus)
	}
	p.SettledAt = settledAt
	if settlementReferenceID != nil {
		p.SettlementReferenceID = *settlementReferenceID
	}
	if settlementUTR != nil {
		p.SettlementUTR = *settlementUTR
	}
	if len(settlementMetadataJSON) > 0 {
		if err := json.Unmarshal(settlementMetadataJSON, &p.SettlementMetadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settlement metadata: %w", err)
		}
	}
	p.SettlementAmount = settlementAmount
	p.NetSettlementAmount = netSettlementAmount

	return &p, nil
}

// UpdatePaymentStatus updates payment status and metadata
func (r *PgPaymentRepository) UpdatePaymentStatus(ctx context.Context, paymentID string, status payment.PaymentStatus, metadata map[string]any) error {
	r.logger.DebugWith("Updating payment status", "payment_id", paymentID, "status", status)

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
	r.logger.DebugWith("Updating payment gateway", "payment_id", paymentID, "gateway", gateway)

	query := `UPDATE xcentro.payments SET gateway_used = $1, updated_at = NOW() WHERE payment_id = $2`
	_, err := r.querier.Exec(ctx, query, gateway, paymentID)
	return err
}

// UpdateGatewayPaymentID updates gateway transaction ID and redirect URL
func (r *PgPaymentRepository) UpdateGatewayPaymentID(ctx context.Context, paymentID string, gatewayPaymentID, redirectURL string) error {
	r.logger.DebugWith("Updating gateway payment ID", "payment_id", paymentID, "gateway_payment_id", gatewayPaymentID)

	query := `UPDATE xcentro.payments SET gateway_payment_id = $1, redirect_url = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, gatewayPaymentID, redirectURL, paymentID)
	return err
}

// UpdatePaymentFees updates fee breakdown
func (r *PgPaymentRepository) UpdatePaymentFees(ctx context.Context, paymentID string, feeAmount, totalAmount int64) error {
	r.logger.DebugWith("Updating payment fees", "payment_id", paymentID, "fee_amount", feeAmount, "total_amount", totalAmount)

	query := `UPDATE xcentro.payments SET fee_amount = $1, total_amount = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, feeAmount, totalAmount, paymentID)
	return err
}

// UpdatePaymentSource links payment to a payment source
func (r *PgPaymentRepository) UpdatePaymentSource(ctx context.Context, paymentID string, sourceID string, sourceType payment.PaymentSourceType) error {
	r.logger.DebugWith("Updating payment source", "payment_id", paymentID, "source_id", sourceID, "source_type", sourceType)

	query := `UPDATE xcentro.payments SET payment_source_id = $1, payment_source_type = $2, updated_at = NOW() WHERE payment_id = $3`
	_, err := r.querier.Exec(ctx, query, sourceID, sourceType, paymentID)
	return err
}

// GetPaymentsByBusinessID retrieves payments for a business with pagination
func (r *PgPaymentRepository) GetPaymentsByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*payment.Payment, error) {
	r.logger.DebugWith("Getting payments by business ID", "business_id", businessID, "limit", limit, "offset", offset)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used::text, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, convenience_fee, convenience_fee_gst,
		       total_amount, metadata, created_at, updated_at
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
			&p.ConvenienceFee,
			&p.ConvenienceFeeGST,
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
func (r *PgPaymentRepository) UpdateSettlementStatus(ctx context.Context, paymentID string, status payment.SettlementStatus, settlementReferenceID, settlementUTR string, metadata map[string]any) error {
	r.logger.DebugWith("Updating settlement status", "payment_id", paymentID, "settlement_status", status)

	query := `
		UPDATE xcentro.payments
		SET settlement_status = $1::settlement_status_enum,
		    settlement_reference_id = $2,
		    settlement_utr = $3,
		    settlement_metadata = COALESCE(settlement_metadata, '{}'::jsonb) || $4::jsonb,
		    settled_at = CASE WHEN $1 = 'settled' THEN NOW() ELSE settled_at END,
		    updated_at = NOW()
		WHERE payment_id = $5
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

	_, err := r.querier.Exec(ctx, query, string(status), settlementReferenceID, settlementUTR, metadataJSON, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update settlement status: %w", err)
	}

	return nil
}

// UpdateSettlementAmounts updates settlement financial fields
func (r *PgPaymentRepository) UpdateSettlementAmounts(ctx context.Context, paymentID string, settlementAmount, mdrAmount, netSettlementAmount int64) error {
	r.logger.DebugWith("Updating settlement amounts",
		"payment_id", paymentID,
		"settlement_amount", settlementAmount,
		"mdr_amount", mdrAmount,
		"net_settlement_amount", netSettlementAmount)

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
	r.logger.DebugWith("Getting pending settlements", "business_id", businessID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used::text, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, convenience_fee, convenience_fee_gst,
		       total_amount, metadata, created_at, updated_at
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
			&p.ConvenienceFee,
			&p.ConvenienceFeeGST,
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
			p.convenience_fee,
			p.convenience_fee_gst,
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
		var totalAmount, baseAmount, feeAmount, convenienceFee, convenienceFeeGST int64
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
			&convenienceFee,
			&convenienceFeeGST,
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

		// Keep amounts in paise (no conversion)
		item.TotalAmount = totalAmount

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

		// Parse bills - amounts already in paise from database
		var bills []payment.BillItem
		if len(billsJSON) > 0 {
			if err := json.Unmarshal(billsJSON, &bills); err != nil {
				return nil, 0, fmt.Errorf("failed to unmarshal bills: %w", err)
			}
			// Bills already in paise, no conversion needed
		}

		item.Breakdown = payment.PaymentBreakdown{
			Bills:          bills,
			ConvenienceFee: convenienceFee,    // Use stored convenience fee
			GSTOnFee:       convenienceFeeGST, // Use stored GST on fee
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

// UpdateRetryMetadata updates retry-related fields on a payment
func (r *PgPaymentRepository) UpdateRetryMetadata(ctx context.Context, paymentID string, retryOfPaymentID string, retryCount int) error {
	query := `
		UPDATE xcentro.payments
		SET retry_of_payment_id = $1, retry_count = $2, updated_at = NOW()
		WHERE payment_id = $3
	`

	result, err := r.querier.Exec(ctx, query, retryOfPaymentID, retryCount, paymentID)
	if err != nil {
		return fmt.Errorf("failed to update retry metadata: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	r.logger.DebugWith("Updated retry metadata",
		"payment_id", paymentID,
		"retry_of_payment_id", retryOfPaymentID,
		"retry_count", retryCount)

	return nil
}

// UpdateHasSuccessfulRetry updates the has_successful_retry flag on the original payment
// This is called when a retry payment succeeds
func (r *PgPaymentRepository) UpdateHasSuccessfulRetry(ctx context.Context, paymentID string, hasSuccessfulRetry bool) error {
	// Note: has_successful_retry is computed dynamically in the payment list query
	// via EXISTS check. This method is a no-op since we don't store it.
	// The flag is determined by: EXISTS(SELECT 1 FROM payments WHERE retry_of_payment_id = p.payment_id AND status = 'success')
	// So we don't need to update anything - it will automatically reflect when the retry payment has status='success'

	r.logger.DebugWith("UpdateHasSuccessfulRetry called (no-op - computed dynamically)",
		"payment_id", paymentID,
		"has_successful_retry", hasSuccessfulRetry)

	return nil
}

// ============================================================================
// Ops Dashboard Methods for Collections
// ============================================================================

// ListCollectionsPaginated retrieves paginated list of payments (collections) for ops dashboard
// Supports filtering by status: pending, success, failed, payoutReady, or all
func (r *PgPaymentRepository) ListCollectionsPaginated(ctx context.Context, page int, limit int, status string) ([]*payment.OpsCollectionListItem, int, error) {
	// Build status filter
	statusFilter := ""
	var args []interface{}
	argIndex := 1

	// Map ops status to payment/settlement status
	switch status {
	case "pending":
		// Payment is pending
		statusFilter = fmt.Sprintf(" AND p.status = $%d", argIndex)
		args = append(args, "pending")
		argIndex++
	case "success":
		// Payment successful but not yet settled
		statusFilter = fmt.Sprintf(" AND p.status = $%d AND (p.settlement_status IS NULL OR p.settlement_status != 'settled')", argIndex)
		args = append(args, "success")
		argIndex++
	case "failed":
		// Payment failed
		statusFilter = fmt.Sprintf(" AND p.status = $%d", argIndex)
		args = append(args, "failed")
		argIndex++
	case "payoutReady":
		// Payment settled and ready for payout
		statusFilter = fmt.Sprintf(" AND p.status = 'success' AND p.settlement_status = $%d", argIndex)
		args = append(args, "settled")
		argIndex++
	default:
		// No filter - return all statuses
	}

	// Get total count
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM xcentro.payments p
		WHERE p.payment_type = 'payment'%s
	`, statusFilter)

	var totalCount int
	err := r.querier.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get collection count: %w", err)
	}

	// Build main query with pagination
	offset := (page - 1) * limit
	mainQuery := fmt.Sprintf(`
		SELECT
			p.payment_id,
			p.created_at,
			p.total_amount,
			p.status,
			p.business_id,
			b.name as business_name,
			p.payment_method
		FROM xcentro.payments p
		LEFT JOIN xcentro.business b ON p.business_id = b.bid
		WHERE p.payment_type = 'payment'%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, statusFilter, argIndex, argIndex+1)

	// Add pagination args
	args = append(args, limit, offset)

	rows, err := r.querier.Query(ctx, mainQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query collections: %w", err)
	}
	defer rows.Close()

	var collections []*payment.OpsCollectionListItem
	for rows.Next() {
		var item payment.OpsCollectionListItem
		var status string
		var paymentMethod string
		var businessName *string

		err := rows.Scan(
			&item.PaymentID,
			&item.Date,
			&item.TotalAmount,
			&status,
			&item.BusinessID,
			&businessName,
			&paymentMethod,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan collection: %w", err)
		}

		item.Status = payment.PaymentStatus(status)
		item.PaymentMethod = payment.PaymentMethod(paymentMethod)
		if businessName != nil {
			item.BusinessName = *businessName
		}

		collections = append(collections, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating collection rows: %w", err)
	}

	return collections, totalCount, nil
}

// GetCollectionSummary retrieves lightweight summary for a single collection (payment)
// Returns payment method details, gateway info, and payee details
func (r *PgPaymentRepository) GetCollectionSummary(ctx context.Context, paymentID string) (*payment.OpsCollectionSummary, error) {
	// Query for payment with method details
	query := `
		SELECT
			p.payment_id,
			p.created_at,
			p.total_amount,
			p.status,
			p.payment_method,
			COALESCE(p.gateway_used::text, '') as gateway_used,
			COALESCE(p.gateway_payment_id, '') as gateway_payment_id,
			p.payment_source_id,
			p.payment_source_type,
			-- Card details (if payment source is card)
			sc.card_type,
			sc.card_category,
			sc.issuer_bank,
			sc.last_four_digits as card_last4,
			-- Bank account details (if payment source is bank_account)
			ba.bank_name,
			ba.ifsc_code,
			ba.account_number_encrypted
		FROM xcentro.payments p
		LEFT JOIN xcentro.stored_cards sc
			ON p.payment_source_id = sc.card_id
			AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba
			ON p.payment_source_id = ba.account_id
			AND p.payment_source_type = 'bank_account'
		WHERE p.payment_id = $1
	`

	var summary payment.OpsCollectionSummary
	var paymentMethod, gatewayUsed, gatewayPaymentID string
	var paymentSourceID, paymentSourceType *string
	var cardType, cardCategory, issuerBank, cardLast4 *string
	var bankName, ifscCode, accountNumberEncrypted *string
	var status string

	err := r.querier.QueryRow(ctx, query, paymentID).Scan(
		&summary.PaymentID,
		&summary.Date,
		&summary.TotalAmount,
		&status,
		&paymentMethod,
		&gatewayUsed,
		&gatewayPaymentID,
		&paymentSourceID,
		&paymentSourceType,
		&cardType,
		&cardCategory,
		&issuerBank,
		&cardLast4,
		&bankName,
		&ifscCode,
		&accountNumberEncrypted,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("payment not found")
		}
		return nil, fmt.Errorf("failed to get collection summary: %w", err)
	}

	summary.Status = payment.PaymentStatus(status)

	// Build payment method details
	summary.PaymentMethod = r.buildOpsPaymentMethodDetails(
		paymentMethod,
		cardType, cardCategory, issuerBank, cardLast4,
		bankName, ifscCode, accountNumberEncrypted,
	)

	// Build gateway details
	summary.Gateway = payment.OpsGatewayDetails{
		Name:          gatewayUsed,
		TransactionID: gatewayPaymentID,
	}

	// Fetch payee details based on payment status
	payees, err := r.getPayeesForPayment(ctx, paymentID, status)
	if err != nil {
		r.logger.WarnWith("Failed to fetch payees for payment", "payment_id", paymentID, "error", err)
		summary.Payees = []payment.OpsPayeeDetails{}
	} else {
		summary.Payees = payees
	}

	return &summary, nil
}

// buildOpsPaymentMethodDetails builds detailed payment method info for ops
func (r *PgPaymentRepository) buildOpsPaymentMethodDetails(
	method string,
	cardType, cardCategory, issuerBank, cardLast4 *string,
	bankName, ifscCode, accountNumberEncrypted *string,
) payment.OpsPaymentMethodDetails {
	details := payment.OpsPaymentMethodDetails{}

	switch method {
	case "card", "token_card":
		details.Type = "card"
		if cardCategory != nil {
			details.CardType = *cardCategory // credit, debit, corporate
		}
		if cardType != nil {
			// Map card type to display name
			switch *cardType {
			case "visa":
				details.CardNetwork = "Visa"
			case "mastercard":
				details.CardNetwork = "Mastercard"
			case "amex":
				details.CardNetwork = "Amex"
			case "rupay":
				details.CardNetwork = "RuPay"
			case "diners":
				details.CardNetwork = "Diners"
			default:
				details.CardNetwork = *cardType
			}
		}
		if issuerBank != nil {
			details.IssuerBank = *issuerBank
		}
		if cardLast4 != nil {
			details.Last4 = *cardLast4
		}
	case "net_banking":
		details.Type = "bank"
		if bankName != nil {
			details.BankName = *bankName
		}
		if ifscCode != nil {
			details.IFSC = *ifscCode
		}
		if accountNumberEncrypted != nil {
			// Decrypt and extract last 4 digits
			decrypted, err := crypto.DecryptBankAccountNumber(*accountNumberEncrypted, r.encryptionKey)
			if err == nil {
				details.Last4 = crypto.ExtractLastDigits(decrypted, 4)
			} else {
				details.Last4 = "****"
			}
		}
	case "upi":
		details.Type = "upi"
	default:
		details.Type = method
	}

	return details
}

// getPayeesForPayment retrieves payee details based on payment status
// Success payments: payees from payment_vouchers table
// Pending/Failed payments: payees from payment_voucher_pending table
func (r *PgPaymentRepository) getPayeesForPayment(ctx context.Context, paymentID string, status string) ([]payment.OpsPayeeDetails, error) {
	var query string

	if status == "success" {
		// Finalized vouchers - payment_id is set in payment_vouchers
		query = `
			SELECT DISTINCT
				bpr.relation_id::text,
				bpr.display_name
			FROM xcentro.payment_vouchers v
			INNER JOIN xcentro.business_payee_relation bpr
				ON v.payee_id = bpr.payee_id
				AND v.account_id = bpr.account_id
			WHERE v.payment_id = $1
		`
	} else {
		// Pending vouchers - linked via payment_voucher_pending table
		query = `
			SELECT DISTINCT
				bpr.relation_id::text,
				bpr.display_name
			FROM xcentro.payment_voucher_pending pvp
			INNER JOIN xcentro.payment_vouchers v ON pvp.voucher_id = v.voucher_id
			INNER JOIN xcentro.business_payee_relation bpr
				ON v.payee_id = bpr.payee_id
				AND v.account_id = bpr.account_id
			WHERE pvp.payment_id = $1
		`
	}

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query payees: %w", err)
	}
	defer rows.Close()

	var payees []payment.OpsPayeeDetails
	for rows.Next() {
		var payee payment.OpsPayeeDetails
		if err := rows.Scan(&payee.RelationID, &payee.DisplayName); err != nil {
			return nil, fmt.Errorf("failed to scan payee: %w", err)
		}
		payees = append(payees, payee)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payee rows: %w", err)
	}

	return payees, nil
}

// GetCollectionDetails retrieves complete details for a single collection (payment)
// Unifies data from list view, summary, payment breakdown, and settlement information
func (r *PgPaymentRepository) GetCollectionDetails(ctx context.Context, paymentID string) (*payment.OpsCollectionDetails, error) {
	// Query for complete payment information
	query := `
		SELECT
			p.payment_id,
			p.created_at,
			p.status,
			p.business_id,
			b.name as business_name,
			-- Payment breakdown fields
			p.amount as base_amount,
			p.convenience_fee,
			p.convenience_fee_gst,
			p.total_amount,
			-- Payment method fields
			p.payment_method,
			COALESCE(p.gateway_used::text, '') as gateway_used,
			COALESCE(p.gateway_payment_id, '') as gateway_payment_id,
			p.payment_source_id,
			p.payment_source_type,
			-- Card details (if payment source is card)
			sc.card_type,
			sc.card_category,
			sc.issuer_bank,
			sc.last_four_digits as card_last4,
			-- Bank account details (if payment source is bank_account)
			ba.bank_name,
			ba.ifsc_code,
			ba.account_number_encrypted,
			-- Settlement fields
			COALESCE(p.settlement_status::text, '') as settlement_status,
			COALESCE(p.settled_at::text, '') as settled_at,
			p.settlement_amount,
			p.mdr_amount,
			p.net_settlement_amount,
			COALESCE(p.settlement_utr, '') as settlement_utr,
			COALESCE(p.settlement_reference_id, '') as settlement_reference_id
		FROM xcentro.payments p
		LEFT JOIN xcentro.business b ON p.business_id = b.bid
		LEFT JOIN xcentro.stored_cards sc
			ON p.payment_source_id = sc.card_id
			AND p.payment_source_type = 'card'
		LEFT JOIN xcentro.bank_accounts ba
			ON p.payment_source_id = ba.account_id
			AND p.payment_source_type = 'bank_account'
		WHERE p.payment_id = $1
	`

	var details payment.OpsCollectionDetails
	var status, paymentMethod, gatewayUsed, gatewayPaymentID string
	var paymentSourceID, paymentSourceType *string
	var cardType, cardCategory, issuerBank, cardLast4 *string
	var bankName, ifscCode, accountNumberEncrypted *string
	var businessName *string
	var settlementStatus, settlementUTR, settlementReferenceID, settledAtStr string
	var settlementAmount, netSettlementAmount *int64
	var mdrAmount int64

	err := r.querier.QueryRow(ctx, query, paymentID).Scan(
		&details.PaymentID,
		&details.Date,
		&status,
		&details.BusinessID,
		&businessName,
		// Payment breakdown
		&details.PaymentBreakdown.BaseAmount,
		&details.PaymentBreakdown.ConvenienceFee,
		&details.PaymentBreakdown.ConvenienceFeeGST,
		&details.PaymentBreakdown.TotalAmount,
		// Payment method
		&paymentMethod,
		&gatewayUsed,
		&gatewayPaymentID,
		&paymentSourceID,
		&paymentSourceType,
		// Card details
		&cardType,
		&cardCategory,
		&issuerBank,
		&cardLast4,
		// Bank details
		&bankName,
		&ifscCode,
		&accountNumberEncrypted,
		// Settlement
		&settlementStatus,
		&settledAtStr,
		&settlementAmount,
		&mdrAmount,
		&netSettlementAmount,
		&settlementUTR,
		&settlementReferenceID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("payment not found")
		}
		return nil, fmt.Errorf("failed to get collection details: %w", err)
	}

	// Set basic fields
	details.Status = payment.PaymentStatus(status)
	if businessName != nil {
		details.BusinessName = *businessName
	}

	// Build payment method details
	details.PaymentMethod = r.buildOpsPaymentMethodDetails(
		paymentMethod,
		cardType, cardCategory, issuerBank, cardLast4,
		bankName, ifscCode, accountNumberEncrypted,
	)

	// Build gateway details
	details.Gateway = payment.OpsGatewayDetails{
		Name:          gatewayUsed,
		TransactionID: gatewayPaymentID,
	}

	// Build settlement details (if applicable)
	if settlementStatus != "" {
		settlement := &payment.OpsSettlementDetails{
			Status:                settlementStatus,
			SettlementAmount:      settlementAmount,
			MDRAmount:             mdrAmount,
			NetSettlementAmount:   netSettlementAmount,
			SettlementUTR:         settlementUTR,
			SettlementReferenceID: settlementReferenceID,
		}

		// Parse settled_at timestamp if present
		if settledAtStr != "" {
			settledAt, err := time.Parse(time.RFC3339, settledAtStr)
			if err == nil {
				settlement.SettledAt = &settledAt
			}
		}

		details.Settlement = settlement
	}

	// Fetch payee details based on payment status
	payees, err := r.getPayeesForPayment(ctx, paymentID, status)
	if err != nil {
		r.logger.WarnWith("Failed to fetch payees for payment", "payment_id", paymentID, "error", err)
		details.Payees = []payment.OpsPayeeDetails{}
	} else {
		details.Payees = payees
	}

	return &details, nil
}

// GetCollectionActivity retrieves complete activity log for a payment from payment_state_changes
// Only includes payment and settlement related actions, excludes payout actions
func (r *PgPaymentRepository) GetCollectionActivity(ctx context.Context, paymentID string) (*payment.OpsCollectionActivityResponse, error) {
	// Query to fetch payment/settlement state changes only (exclude payout actions)
	// Collections can fund multiple vouchers, each with separate payout lifecycle
	query := `
		SELECT
			psc.state_change_id::text,
			psc.action::text,
			psc.action_timestamp,
			psc.actor_type::text,
			psc.actor_user_id::text,
			COALESCE(u.name, psc.actor_name, 'System') as actor_name,
			COALESCE(psc.previous_state, '') as previous_state,
			COALESCE(psc.new_state, '') as new_state,
			psc.amount,
			COALESCE(psc.currency, '') as currency,
			COALESCE(psc.utr, '') as utr,
			COALESCE(psc.provider, '') as provider,
			COALESCE(psc.remarks, '') as remarks,
			psc.metadata
		FROM xcentro.payment_state_changes psc
		LEFT JOIN xcentro.user_table u ON psc.actor_user_id = u.id
		WHERE psc.payment_id = $1
		  AND psc.action::text NOT LIKE 'payout_%'
		ORDER BY psc.action_timestamp ASC
	`

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment activity: %w", err)
	}
	defer rows.Close()

	var activities []*payment.OpsCollectionActivity

	for rows.Next() {
		var activity payment.OpsCollectionActivity
		var actorUserID *string
		var amount sql.NullInt64
		var metadataJSON []byte

		err := rows.Scan(
			&activity.StateChangeID,
			&activity.Action,
			&activity.ActionTimestamp,
			&activity.ActorType,
			&actorUserID,
			&activity.ActorName,
			&activity.PreviousState,
			&activity.NewState,
			&amount,
			&activity.Currency,
			&activity.UTR,
			&activity.Provider,
			&activity.Remarks,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}

		// Handle nullable fields
		if actorUserID != nil {
			activity.ActorUserID = actorUserID
		}
		if amount.Valid {
			activity.Amount = &amount.Int64
		}

		// Parse metadata JSON
		if len(metadataJSON) > 0 {
			var metadata map[string]interface{}
			if err := json.Unmarshal(metadataJSON, &metadata); err == nil {
				activity.Metadata = metadata
			}
		}

		activities = append(activities, &activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating activity rows: %w", err)
	}

	return &payment.OpsCollectionActivityResponse{
		PaymentID:  paymentID,
		Activities: activities,
		TotalCount: len(activities),
	}, nil
}

// GetCollectionPayouts retrieves all voucher-payout mappings for a payment/collection
// A single payment can fund multiple vouchers, each with potential payouts to payees
func (r *PgPaymentRepository) GetCollectionPayouts(ctx context.Context, paymentID string) (*payment.OpsCollectionPayoutsResponse, error) {
	// Query to get all vouchers linked to this payment and their latest payout status
	// Uses LATERAL joins to get latest transfer per voucher (active or deleted)
	// Shows latest attempt even if soft-deleted (for max retry cases)
	query := `
		SELECT
			v.voucher_id::text,
			v.voucher_number,
			v.net_amount,
			v.payee_id::text,
			bpr.display_name as payee_name,
			COALESCE(latest_pt.transfer_request_id::text, '') as payout_id,
			COALESCE(latest_pt.status::text, '') as payout_status,
			latest_pt.retry_count as retry_count,
			latest_pt.max_retries as max_retries
		FROM xcentro.payment_vouchers v
		INNER JOIN xcentro.business_payee_relation bpr
			ON v.payee_id = bpr.payee_id
			AND v.business_id = bpr.business_id
		LEFT JOIN LATERAL (
			SELECT transfer_request_id, status, retry_count, max_retries
			FROM xcentro.payout_transfer_requests
			WHERE voucher_id = v.voucher_id
			ORDER BY created_at DESC
			LIMIT 1
		) latest_pt ON true
		WHERE v.payment_id = $1
		ORDER BY v.created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection payouts: %w", err)
	}
	defer rows.Close()

	// Initialize as empty slice to return [] instead of null when no vouchers
	payouts := make([]*payment.OpsCollectionPayoutItem, 0)

	for rows.Next() {
		var item payment.OpsCollectionPayoutItem
		var payoutID, payoutStatus string
		var retryCount, maxRetries sql.NullInt32

		err := rows.Scan(
			&item.VoucherID,
			&item.VoucherNumber,
			&item.Amount,
			&item.PayeeID,
			&item.PayeeName,
			&payoutID,
			&payoutStatus,
			&retryCount,
			&maxRetries,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payout item: %w", err)
		}

		// Set payout fields only if payout exists
		if payoutID != "" {
			item.PayoutID = &payoutID
		}
		if payoutStatus != "" {
			item.PayoutStatus = &payoutStatus
		}
		if retryCount.Valid {
			// Convert 0-indexed retry_count to 1-indexed attempt_number for UI display
			attemptNumber := int(retryCount.Int32) + 1
			item.AttemptNumber = &attemptNumber
		}
		if maxRetries.Valid {
			maxRetriesInt := int(maxRetries.Int32)
			item.MaxRetries = &maxRetriesInt
		}

		payouts = append(payouts, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating payout rows: %w", err)
	}

	return &payment.OpsCollectionPayoutsResponse{
		PaymentID:  paymentID,
		Data:       payouts,
		TotalCount: len(payouts),
	}, nil
}
