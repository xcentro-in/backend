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
)

// PgPaymentRepository implements payment.PaymentRepository using PostgreSQL
type PgPaymentRepository struct {
	querier client.Querier
}

// NewPgPaymentRepository creates a new PostgreSQL payment repository
func NewPgPaymentRepository(db client.DB) *PgPaymentRepository {
	return &PgPaymentRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PgPaymentRepository) WithTx(tx interface{}) payment.PaymentRepository {
	return &PgPaymentRepository{
		querier: tx.(client.Tx),
	}
}

// CreatePayment creates a new payment record
func (r *PgPaymentRepository) CreatePayment(ctx context.Context, p *payment.Payment) error {
	fmt.Printf("[PgPaymentRepository] CreatePayment called - BusinessID: %s, UserID: %s, Amount: %d, Currency: %s, Method: %s\n",
		p.BusinessID, p.UserID, p.Amount, p.Currency, p.PaymentMethod)

	query := `
		INSERT INTO xcentro.payments (
			user_id, business_id, amount, currency, payment_method, status,
			description, callback_url, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING payment_id, created_at
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
		p.UserID,
		p.BusinessID,
		p.Amount,
		p.Currency,
		p.PaymentMethod,
		p.Status,
		p.Description,
		p.CallbackURL,
		metadataJSON,
	).Scan(&p.ID, &p.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	fmt.Printf("[PgPaymentRepository] Payment created with ID: %s\n", p.ID)
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

// GetPaymentByGatewayPaymentID retrieves payment by gateway's transaction ID
func (r *PgPaymentRepository) GetPaymentByGatewayPaymentID(ctx context.Context, gatewayPaymentID string) (*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentByGatewayPaymentID called - GatewayPaymentID: %s\n", gatewayPaymentID)

	query := `
		SELECT payment_id, user_id, business_id, amount, currency, payment_method, status,
		       description, gateway_used, gateway_payment_id, callback_url, redirect_url,
		       payment_source_id, payment_source_type, fee_amount, total_amount,
		       metadata, created_at, updated_at
		FROM xcentro.payments WHERE gateway_payment_id = $1
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
