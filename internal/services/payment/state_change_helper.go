package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payment"
)

// PaymentStateLogger provides helper methods for logging payment state changes
type PaymentStateLogger struct {
	stateChangeRepo payment.PaymentStateChangeRepository
}

// NewPaymentStateLogger creates a new state logger
func NewPaymentStateLogger(repo payment.PaymentStateChangeRepository) *PaymentStateLogger {
	return &PaymentStateLogger{
		stateChangeRepo: repo,
	}
}

// LogPaymentInitiated logs when a user initiates a payment
func (l *PaymentStateLogger) LogPaymentInitiated(ctx context.Context, paymentID uuid.UUID, userID uuid.UUID, amount int64, provider string) error {
	req := &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		Action:          payment.PaymentActionInitiated,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeUser,
		ActorUserID:     &userID,
		Amount:          amount,
		Currency:        "INR",
		Provider:        provider,
		Remarks:         fmt.Sprintf("Payment initiated via %s gateway", provider),
	}

	_, err := l.stateChangeRepo.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to log payment initiated: %w", err)
	}

	return nil
}

// LogPaymentSuccess logs when payment succeeds (from webhook)
func (l *PaymentStateLogger) LogPaymentSuccess(ctx context.Context, paymentID uuid.UUID, amount int64, provider string, gatewayPaymentID string, metadata map[string]any) error {
	req := &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		Action:          payment.PaymentActionSuccess,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeWebhook,
		ActorName:       fmt.Sprintf("%s_webhook", provider),
		Amount:          amount,
		Currency:        "INR",
		Provider:        provider,
		NewState:        "success",
		Metadata:        metadata,
		Remarks:         fmt.Sprintf("Payment successful. Gateway Payment ID: %s", gatewayPaymentID),
	}

	_, err := l.stateChangeRepo.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to log payment success: %w", err)
	}

	return nil
}

// LogPaymentFailed logs when payment fails (from webhook)
func (l *PaymentStateLogger) LogPaymentFailed(ctx context.Context, paymentID uuid.UUID, provider string, failureReason string, metadata map[string]any) error {
	req := &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		Action:          payment.PaymentActionFailed,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeWebhook,
		ActorName:       fmt.Sprintf("%s_webhook", provider),
		Provider:        provider,
		NewState:        "failed",
		Metadata:        metadata,
		Remarks:         fmt.Sprintf("Payment failed: %s", failureReason),
	}

	_, err := l.stateChangeRepo.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to log payment failure: %w", err)
	}

	return nil
}

// LogSettlementReceived logs when settlement is received in escrow
func (l *PaymentStateLogger) LogSettlementReceived(ctx context.Context, paymentID uuid.UUID, amount int64, settlementUTR string, metadata map[string]any) error {
	req := &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		Action:          payment.SettlementActionReceived,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeWebhook,
		ActorName:       "settlement_webhook",
		Amount:          amount,
		Currency:        "INR",
		UTR:             settlementUTR,
		NewState:        "settled",
		Metadata:        metadata,
		Remarks:         fmt.Sprintf("Settlement received. UTR: %s", settlementUTR),
	}

	_, err := l.stateChangeRepo.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to log settlement received: %w", err)
	}

	return nil
}




// LogCustom logs a custom state change (for flexibility)
func (l *PaymentStateLogger) LogCustom(ctx context.Context, req *payment.CreatePaymentStateChangeRequest) error {
	_, err := l.stateChangeRepo.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to log custom state change: %w", err)
	}

	return nil
}
