package payment

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PaymentAction represents the type of payment/payout state change
type PaymentAction string

const (
	// Payment lifecycle
	PaymentActionInitiated   PaymentAction = "payment_initiated"
	PaymentActionProcessing  PaymentAction = "payment_processing"
	PaymentActionSuccess     PaymentAction = "payment_success"
	PaymentActionFailed      PaymentAction = "payment_failed"
	PaymentActionRefunded    PaymentAction = "payment_refunded"

	// Settlement lifecycle (funds to escrow)
	SettlementActionPending  PaymentAction = "settlement_pending"
	SettlementActionReceived PaymentAction = "settlement_received"
	SettlementActionFailed   PaymentAction = "settlement_failed"

)

// MapPaymentActionToUserFriendly converts internal payment actions to user-friendly strings
// For user-facing activity feeds, simplify the complex payment lifecycle into meaningful states
// Note: Settlement actions are filtered out at the repository level to avoid duplicate "Payment Processing" entries
func MapPaymentActionToUserFriendly(action PaymentAction) string {
	switch action {
	// Payment lifecycle: Each state has distinct label
	case PaymentActionInitiated:
		return "Payment Initiated"
	case PaymentActionProcessing:
		return "Payment Processing"
	case PaymentActionSuccess:
		return "Payment Successful"

	// Payment failures → Show actual failure
	case PaymentActionFailed:
		return "Payment Failed"
	case PaymentActionRefunded:
		return "Payment Refunded"

	// Settlement lifecycle: Hidden from user-facing activity (filtered at repository level)
	// These are kept here for completeness but won't appear in /vouchers/:id/activity
	case SettlementActionPending, SettlementActionReceived, SettlementActionFailed:
		return "Payment Processing"

	default:
		return string(action)
	}
}

// PaymentActorType represents who/what triggered the state change
type PaymentActorType string

const (
	ActorTypeUser    PaymentActorType = "user"    // Human user
	ActorTypeSystem  PaymentActorType = "system"  // System/worker
	ActorTypeWebhook PaymentActorType = "webhook" // External webhook
	ActorTypeAPI     PaymentActorType = "api"     // External API call
)

// PaymentStateChange represents a single state change event in payment/payout lifecycle
// Note: No voucher_id to avoid duplicates in multi-bill payments (multiple vouchers share one payment_id)
type PaymentStateChange struct {
	StateChangeID   uuid.UUID        `json:"state_change_id"`
	PaymentID       uuid.UUID        `json:"payment_id"`
	Action          PaymentAction    `json:"action"`
	ActionTimestamp time.Time        `json:"action_timestamp"`
	ActorType       PaymentActorType `json:"actor_type"`
	ActorUserID     *uuid.UUID       `json:"actor_user_id,omitempty"`
	ActorName       string           `json:"actor_name,omitempty"` // Denormalized for system actors
	PreviousState   string           `json:"previous_state,omitempty"`
	NewState        string           `json:"new_state,omitempty"`
	Amount          int64            `json:"amount,omitempty"`
	Currency        string           `json:"currency,omitempty"`
	UTR             string           `json:"utr,omitempty"`
	Provider        string           `json:"provider,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	Remarks         string           `json:"remarks,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

// CreatePaymentStateChangeRequest represents the request to create a state change record
type CreatePaymentStateChangeRequest struct {
	PaymentID       uuid.UUID  // Required: multiple vouchers can share same payment
	Action          PaymentAction
	ActionTimestamp time.Time
	ActorType       PaymentActorType
	ActorUserID     *uuid.UUID
	ActorName       string // e.g., "payout_worker", "razorpay_webhook"
	PreviousState   string
	NewState        string
	Amount          int64
	Currency        string
	UTR             string
	Provider        string
	Metadata        map[string]any
	Remarks         string
}

// PaymentStateChangeRepository defines the interface for payment state change persistence
type PaymentStateChangeRepository interface {
	// Create records a new payment state change
	Create(ctx context.Context, req *CreatePaymentStateChangeRequest) (*PaymentStateChange, error)

	// GetByPaymentID retrieves all state changes for a payment (chronologically)
	GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*PaymentStateChange, error)


	// GetLatestByPaymentID retrieves the most recent state change for a payment
	GetLatestByPaymentID(ctx context.Context, paymentID uuid.UUID) (*PaymentStateChange, error)

	// GetByAction retrieves state changes by action type (for monitoring/reporting)
	GetByAction(ctx context.Context, action PaymentAction, limit int, offset int) ([]*PaymentStateChange, error)
}
