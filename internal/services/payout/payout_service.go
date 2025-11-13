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

// InitiateManualPayout creates payout transfer requests for a payment
// New flow: payment_id (required) + voucher_ids[] (optional)
// If voucher_ids empty: processes all unpaid vouchers for the payment
// Returns all created transfer requests (status='initiated')
func (s *PayoutService) InitiateManualPayout(ctx context.Context, paymentID string, voucherIDs []string) ([]*payout.PayoutTransferRequest, error) {
	s.logger.InfoWith("InitiateManualPayout called", "payment_id", paymentID, "voucher_ids", voucherIDs)

	// Step 1: Parse payment ID
	paymentUUID, err := uuid.Parse(paymentID)
	if err != nil {
		return nil, fmt.Errorf("invalid payment_id format: %w", err)
	}

	// Step 2: Get all vouchers for this payment
	allVouchers, err := s.voucherRepo.GetByPaymentID(ctx, paymentUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vouchers for payment: %w", err)
	}

	if len(allVouchers) == 0 {
		return nil, fmt.Errorf("no vouchers found for payment_id: %s", paymentID)
	}

	s.logger.DebugWith("Found vouchers for payment", "count", len(allVouchers))

	// Step 3: Filter vouchers based on voucher_ids[] (if provided)
	var targetVouchers []*voucher.Voucher
	if len(voucherIDs) == 0 {
		// Empty list → process all unpaid vouchers
		s.logger.DebugWith("No voucher_ids specified - processing all unpaid vouchers")
		for _, v := range allVouchers {
			// Check if already paid (has existing transfer)
			existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
			if err == nil && existing != nil {
				s.logger.DebugWith("Skipping voucher - already has transfer",
					"voucher_id", v.VoucherID.String(),
					"transfer_id", existing.TransferRequestID,
					"status", existing.Status)
				continue
			}
			targetVouchers = append(targetVouchers, v)
		}
	} else {
		// Specific voucher_ids provided → process only those
		s.logger.DebugWith("Processing specific voucher_ids", "count", len(voucherIDs))
		voucherIDSet := make(map[string]bool)
		for _, vid := range voucherIDs {
			voucherIDSet[vid] = true
		}

		for _, v := range allVouchers {
			if voucherIDSet[v.VoucherID.String()] {
				// Idempotency check
				existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
				if err == nil && existing != nil {
					return nil, fmt.Errorf("voucher %s already has transfer (transfer_id: %s, status: %s)",
						v.VoucherID.String(), existing.TransferRequestID, existing.Status)
				}
				targetVouchers = append(targetVouchers, v)
			}
		}

		if len(targetVouchers) != len(voucherIDs) {
			return nil, fmt.Errorf("some voucher_ids not found or not linked to payment_id: %s", paymentID)
		}
	}

	if len(targetVouchers) == 0 {
		return nil, fmt.Errorf("no vouchers to process (all already have transfers)")
	}

	s.logger.InfoWith("Target vouchers identified", "count", len(targetVouchers))

	// Step 4: Get configuration
	config, err := s.configRepo.GetConfig(ctx, nil) // Use global config for MVP
	if err != nil {
		return nil, fmt.Errorf("failed to get payout configuration: %w", err)
	}

	// Step 5: Create transfer requests for each voucher
	var transfers []*payout.PayoutTransferRequest

	for _, vch := range targetVouchers {
		// Get Castler beneficiary ID (lightweight query - no decryption)
		castlerBeneficiaryID, err := s.bankAccountRepo.GetCastlerBeneficiaryID(ctx, vch.AccountID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to get Castler beneficiary ID for voucher %s: %w", vch.VoucherID.String(), err)
		}

		// Create transfer request
		transferReq := &payout.PayoutTransferRequest{
			TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
			PaymentID:             paymentID,
			VoucherID:             vch.VoucherID.String(),
			Provider:              payout.ProviderCastler, // Always Castler for now
			ProviderBeneficiaryID: castlerBeneficiaryID,
			EscrowID:              "", // TODO: Get from config/environment
			Amount:                vch.NetAmount,
			Currency:              vch.Currency,
			Purpose:               "vendor_payment",
			Reference:             vch.VoucherNumber,
			Narration:             fmt.Sprintf("Payment for %s", vch.VoucherNumber),
			TransferMode:          config.PreferredTransferMode,
			Status:                payout.TransferStatusInitiated,
			RetryCount:            0,
			MaxRetries:            config.MaxRetryAttempts,
			CreatedAt:             time.Now(),
		}

		// Save transfer request
		if err := s.transferRepo.Create(ctx, transferReq); err != nil {
			return nil, fmt.Errorf("failed to create transfer for voucher %s: %w", vch.VoucherID.String(), err)
		}

		s.logger.DebugWith("Transfer request created",
			"transfer_request_id", transferReq.TransferRequestID,
			"voucher_id", vch.VoucherID.String(),
			"amount", transferReq.Amount)

		transfers = append(transfers, transferReq)
	}

	s.logger.InfoWith("Transfer requests created successfully", "count", len(transfers))
	return transfers, nil
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

	// Step 4: Get Castler beneficiary ID (lightweight query - no decryption)
	castlerBeneficiaryID, err := s.bankAccountRepo.GetCastlerBeneficiaryID(ctx, vch.AccountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get Castler beneficiary ID: %w", err)
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
		ProviderBeneficiaryID: castlerBeneficiaryID,
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
