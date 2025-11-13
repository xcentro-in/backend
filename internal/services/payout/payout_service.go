package payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// PayoutService orchestrates payout transfer creation and management
type PayoutService struct {
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	bankAccountRepo payee.PayeeBankAccountRepository
	configRepo      payout.PayoutConfigurationRepository
	providerFactory PayoutProviderFactory // Interface to get payout provider
	logger          *logger.Logger
}

// PayoutProviderFactory is an interface to get payout providers
type PayoutProviderFactory interface {
	GetProvider(providerType payout.ProviderType) (payout.PayoutProvider, error)
}

// NewPayoutService creates a new payout service
func NewPayoutService(
	voucherRepo voucher.VoucherRepository,
	transferRepo payout.PayoutTransferRepository,
	bankAccountRepo payee.PayeeBankAccountRepository,
	configRepo payout.PayoutConfigurationRepository,
	providerFactory PayoutProviderFactory,
) *PayoutService {
	return &PayoutService{
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		bankAccountRepo: bankAccountRepo,
		configRepo:      configRepo,
		providerFactory: providerFactory,
		logger:          logger.GetDefault(),
	}
}

// InitiatePayoutForVoucher creates a transfer request for a voucher
// DEPRECATED: Use InitiateManualPayout with payment_id instead
// Kept for backward compatibility during transition
func (s *PayoutService) InitiatePayoutForVoucher(ctx context.Context, voucherID string) (*payout.PayoutTransferRequest, error) {
	s.logger.InfoWith("InitiatePayoutForVoucher called (DEPRECATED)", "voucher_id", voucherID)

	// Step 1: Get voucher
	voucherUUID, err := uuid.Parse(voucherID)
	if err != nil {
		return nil, fmt.Errorf("invalid voucher ID: %w", err)
	}

	vch, err := s.voucherRepo.GetByID(ctx, voucherUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// Step 2: Validate voucher is ready for payout
	// Note: PaymentID must exist (voucher finalized after payment)
	if vch.PaymentID == nil {
		return nil, fmt.Errorf("voucher not linked to payment (still in draft)")
	}

	// Step 3: Check for existing transfer (idempotent)
	existing, err := s.transferRepo.GetByVoucherID(ctx, voucherID)
	if err == nil && existing != nil {
		s.logger.InfoWith("Transfer request already exists", "transfer_request_id", existing.TransferRequestID)
		return existing, nil
	}

	// Step 4: Get bank account with Castler beneficiary ID
	bankAccount, err := s.bankAccountRepo.FindByID(ctx, vch.AccountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get bank account for payee: %w", err)
	}

	if bankAccount == nil {
		return nil, fmt.Errorf("bank account not found: %s", vch.AccountID.String())
	}

	if bankAccount.CastlerBeneficiaryID == "" {
		return nil, fmt.Errorf("payee not onboarded to Castler: missing beneficiary ID")
	}

	// Step 5: Get configuration
	config, err := s.configRepo.GetConfig(ctx, nil) // Use global config for MVP
	if err != nil {
		return nil, fmt.Errorf("failed to get payout configuration: %w", err)
	}

	// Step 6: Create transfer request (simplified - no approval workflow)
	transferReq := &payout.PayoutTransferRequest{
		TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
		PaymentID:             vch.PaymentID.String(),
		VoucherID:             voucherID,
		Provider:              payout.ProviderCastler, // Always Castler for now
		ProviderBeneficiaryID: bankAccount.CastlerBeneficiaryID,
		EscrowID:              "", // TODO: Get from config/environment
		Amount:                vch.NetAmount,
		Currency:              vch.Currency,
		Purpose:               "vendor_payment",
		Reference:             vch.VoucherNumber, // Will use transfer_request_id in new flow
		Narration:             fmt.Sprintf("Payment for %s", vch.VoucherNumber),
		TransferMode:          config.PreferredTransferMode,
		Status:                payout.TransferStatusInitiated,
		RetryCount:            0,
		MaxRetries:            config.MaxRetryAttempts,
		CreatedAt:             time.Now(),
	}

	// Step 7: Save transfer request
	if err := s.transferRepo.Create(ctx, transferReq); err != nil {
		return nil, fmt.Errorf("failed to create transfer request: %w", err)
	}

	s.logger.InfoWith("Transfer request created successfully",
		"transfer_request_id", transferReq.TransferRequestID,
		"status", transferReq.Status,
		"amount", transferReq.Amount)
	return transferReq, nil
}

// GetTransferDetails retrieves transfer details
func (s *PayoutService) GetTransferDetails(ctx context.Context, transferRequestID string) (*payout.PayoutTransferRequest, error) {
	transfer, err := s.transferRepo.GetByID(ctx, transferRequestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer: %w", err)
	}
	return transfer, nil
}

// GetWebhookHistory retrieves webhook history for a transfer
func (s *PayoutService) GetWebhookHistory(ctx context.Context, transferRequestID string) ([]map[string]interface{}, error) {
	transfer, err := s.transferRepo.GetByID(ctx, transferRequestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer: %w", err)
	}
	return transfer.WebhookHistory, nil
}

// RetryTransfer retries a failed transfer (PLACEHOLDER for MVP)
func (s *PayoutService) RetryTransfer(ctx context.Context, transferRequestID string, triggeredBy string) error {
	s.logger.WarnWith("Retry not implemented in MVP",
		"transfer_request_id", transferRequestID,
		"triggered_by", triggeredBy)
	s.logger.DebugWith("TODO: Implement unified retry strategy across payments/payouts")
	// For MVP, manual intervention via ops team
	return errors.New("retry not implemented in MVP - manual intervention required")
}

// MarkCompletedManually manually marks a timeout transfer as completed
// Used when ops team verifies completion externally
func (s *PayoutService) MarkCompletedManually(ctx context.Context, transferRequestID string, utr string, completedBy string, remarks string) error {
	s.logger.InfoWith("MarkCompletedManually called",
		"transfer_request_id", transferRequestID,
		"utr", utr,
		"completed_by", completedBy)

	// Get transfer
	transfer, err := s.transferRepo.GetByID(ctx, transferRequestID)
	if err != nil {
		return fmt.Errorf("failed to get transfer: %w", err)
	}

	// Only allow for timeout transfers
	if transfer.Status != payout.TransferStatusTimeout {
		return fmt.Errorf("only timeout transfers can be manually completed: status=%s", transfer.Status)
	}

	// Mark completed
	now := time.Now()
	if err := s.transferRepo.MarkCompleted(ctx, transferRequestID, utr, now); err != nil {
		return fmt.Errorf("failed to mark completed: %w", err)
	}

	s.logger.InfoWith("Transfer manually completed successfully",
		"transfer_request_id", transferRequestID,
		"utr", utr,
		"completed_by", completedBy)
	s.logger.DebugWith("TODO: Add audit logging for manual completions")
	return nil
}
