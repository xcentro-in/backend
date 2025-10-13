package payment

import "context"

// PaymentRepository defines database operations for payments
type PaymentRepository interface {
	// Core CRUD operations
	CreatePayment(ctx context.Context, payment *Payment) error
	GetPaymentByID(ctx context.Context, paymentID string) (*Payment, error)
	GetPaymentByGatewayPaymentID(ctx context.Context, gatewayPaymentID string) (*Payment, error)

	// Field updates (fine-grained for payment lifecycle)
	UpdatePaymentStatus(ctx context.Context, paymentID string, status PaymentStatus, metadata map[string]any) error
	UpdatePaymentGateway(ctx context.Context, paymentID string, gateway GatewayType) error
	UpdateGatewayPaymentID(ctx context.Context, paymentID string, gatewayPaymentID, redirectURL string) error
	UpdatePaymentFees(ctx context.Context, paymentID string, feeAmount, totalAmount int64) error
	UpdatePaymentSource(ctx context.Context, paymentID string, sourceID string, sourceType PaymentSourceType) error

	// Querying for business operations
	GetPaymentsByBusinessID(ctx context.Context, businessID string, limit, offset int) ([]*Payment, error)

	// Settlement tracking (Phase 7)
	UpdateSettlementStatus(ctx context.Context, paymentID string, status SettlementStatus, metadata map[string]any) error
	UpdateSettlementAmounts(ctx context.Context, paymentID string, settlementAmount, mdrAmount, netSettlementAmount int64) error
	GetPendingSettlements(ctx context.Context, businessID string) ([]*Payment, error)

	// Transaction support (follows user_repository.go pattern)
	WithTx(tx interface{}) PaymentRepository
}
