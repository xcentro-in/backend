package payment

import (
	"context"
	"fmt"

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
	fmt.Printf("[PgPaymentRepository] CreatePayment called - UserID: %s, Amount: %d, Currency: %s, Method: %s\n",
		p.UserID, p.Amount, p.Currency, p.PaymentMethod)
	// TODO-PHASE-4.4: Implement SQL INSERT
	// - INSERT INTO xcentro.payments (...) VALUES (...) RETURNING payment_id
	// - payment_id generated via uuidv7()
	// - Set p.ID with returned UUID
	return nil
}

// GetPaymentByID retrieves a payment by its ID
func (r *PgPaymentRepository) GetPaymentByID(ctx context.Context, paymentID string) (*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentByID called - PaymentID: %s\n", paymentID)
	// TODO-PHASE-4.4: Implement SQL SELECT
	// - SELECT * FROM xcentro.payments WHERE payment_id = $1
	// - Handle NULL values with sql.NullString
	// - Deserialize JSONB metadata
	return nil, nil
}

// GetPaymentByGatewayPaymentID retrieves payment by gateway's transaction ID
func (r *PgPaymentRepository) GetPaymentByGatewayPaymentID(ctx context.Context, gatewayPaymentID string) (*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentByGatewayPaymentID called - GatewayPaymentID: %s\n", gatewayPaymentID)
	// TODO-PHASE-4.4: Implement SQL SELECT with gateway_payment_id
	// - SELECT * FROM xcentro.payments WHERE gateway_payment_id = $1
	// - Critical for webhook payment lookup
	return nil, nil
}

// UpdatePaymentStatus updates payment status and metadata
func (r *PgPaymentRepository) UpdatePaymentStatus(ctx context.Context, paymentID string, status payment.PaymentStatus, metadata map[string]any) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentStatus called - PaymentID: %s, Status: %s\n", paymentID, status)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.payments SET status = $1, metadata = $2, updated_at = NOW() WHERE payment_id = $3
	// - Serialize metadata to JSONB
	return nil
}

// UpdatePaymentGateway updates the selected gateway
func (r *PgPaymentRepository) UpdatePaymentGateway(ctx context.Context, paymentID string, gateway payment.GatewayType) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentGateway called - PaymentID: %s, Gateway: %s\n", paymentID, gateway)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.payments SET gateway_used = $1, updated_at = NOW() WHERE payment_id = $2
	return nil
}

// UpdateGatewayPaymentID updates gateway transaction ID and redirect URL
func (r *PgPaymentRepository) UpdateGatewayPaymentID(ctx context.Context, paymentID string, gatewayPaymentID, redirectURL string) error {
	fmt.Printf("[PgPaymentRepository] UpdateGatewayPaymentID called - PaymentID: %s, GatewayPaymentID: %s\n", paymentID, gatewayPaymentID)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.payments SET gateway_payment_id = $1, redirect_url = $2, updated_at = NOW() WHERE payment_id = $3
	return nil
}

// UpdatePaymentFees updates fee breakdown
func (r *PgPaymentRepository) UpdatePaymentFees(ctx context.Context, paymentID string, feeAmount, totalAmount int64) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentFees called - PaymentID: %s, FeeAmount: %d, TotalAmount: %d\n", paymentID, feeAmount, totalAmount)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.payments SET fee_amount = $1, total_amount = $2, updated_at = NOW() WHERE payment_id = $3
	return nil
}

// UpdatePaymentSource links payment to a payment source
func (r *PgPaymentRepository) UpdatePaymentSource(ctx context.Context, paymentID string, sourceID string, sourceType payment.PaymentSourceType) error {
	fmt.Printf("[PgPaymentRepository] UpdatePaymentSource called - PaymentID: %s, SourceID: %s, SourceType: %s\n", paymentID, sourceID, sourceType)
	// TODO-PHASE-4.4: Implement SQL UPDATE
	// - UPDATE xcentro.payments SET payment_source_id = $1, payment_source_type = $2, updated_at = NOW() WHERE payment_id = $3
	return nil
}

// GetPaymentsByBusinessID retrieves payments for a business with pagination
func (r *PgPaymentRepository) GetPaymentsByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*payment.Payment, error) {
	fmt.Printf("[PgPaymentRepository] GetPaymentsByBusinessID called - BusinessID: %s, Limit: %d, Offset: %d\n", businessID, limit, offset)
	// TODO-PHASE-4.4: Implement SQL SELECT with LIMIT/OFFSET
	// - SELECT * FROM xcentro.payments WHERE business_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
	// - Return slice of Payment pointers
	return nil, nil
}