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
// Determines if auto-approval is needed based on amount
func (s *PayoutService) InitiatePayoutForVoucher(ctx context.Context, voucherID string) (*payout.PayoutTransferRequest, error) {
	s.logger.InfoWith("InitiatePayoutForVoucher called", "voucher_id", voucherID)

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

	if voucher.PayoutStatus(vch.PayoutStatus) != voucher.PayoutStatusPending {
		return nil, fmt.Errorf("voucher already has payout_status: %s", vch.PayoutStatus)
	}

	if vch.CancellationStatus != voucher.CancellationStatusNotCancelled {
		return nil, fmt.Errorf("voucher is cancelled: %s", vch.CancellationStatus)
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

	// Step 5: Get configuration for auto-approval logic
	config, err := s.configRepo.GetConfig(ctx, nil) // Use global config for MVP
	if err != nil {
		return nil, fmt.Errorf("failed to get payout configuration: %w", err)
	}

	// Step 6: Determine if needs approval
	requiresApproval := !config.ShouldAutoApprove(vch.NetAmount)
	status := payout.TransferStatusApproved
	approvalMode := payout.ApprovalModeAuto
	var approvedBy string
	var approvedAt *time.Time

	if requiresApproval {
		status = payout.TransferStatusPendingApproval
		approvalMode = payout.ApprovalModeManual
		s.logger.WarnWith("Amount exceeds auto-approval limit, requires manual approval",
			"amount", vch.NetAmount,
			"auto_approval_limit", config.AutoApprovalLimit)
	} else {
		approvedBy = "SYSTEM"
		now := time.Now()
		approvedAt = &now
		s.logger.InfoWith("Amount within auto-approval limit, auto-approved",
			"amount", vch.NetAmount,
			"auto_approval_limit", config.AutoApprovalLimit)
	}

	// Step 7: Create transfer request
	transferReq := &payout.PayoutTransferRequest{
		TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
		VoucherID:             voucherID,
		Provider:              payout.ProviderCastler, // Always Castler for now
		ProviderBeneficiaryID: bankAccount.CastlerBeneficiaryID,
		Amount:                vch.NetAmount,
		Currency:              vch.Currency,
		Purpose:               "vendor_payment", // TODO: Make configurable
		Reference:             vch.VoucherNumber,
		Narration:             fmt.Sprintf("Payment for %s", vch.VoucherNumber),
		TransferMode:          config.PreferredTransferMode,
		RequiresApproval:      requiresApproval,
		ApprovalMode:          approvalMode,
		ApprovedBy:            approvedBy,
		ApprovedAt:            approvedAt,
		Status:                status,
		RetryCount:            0,
		MaxRetries:            config.MaxRetryAttempts,
		CreatedAt:             time.Now(),
	}

	// Step 8: Save transfer request
	if err := s.transferRepo.Create(ctx, transferReq); err != nil {
		return nil, fmt.Errorf("failed to create transfer request: %w", err)
	}

	s.logger.InfoWith("Transfer request created successfully",
		"transfer_request_id", transferReq.TransferRequestID,
		"status", transferReq.Status,
		"amount", transferReq.Amount)
	return transferReq, nil
}

// ApproveTransfer manually approves a pending transfer
func (s *PayoutService) ApproveTransfer(ctx context.Context, transferRequestID string, approvedBy string, remarks string) error {
	s.logger.InfoWith("ApproveTransfer called",
		"transfer_request_id", transferRequestID,
		"approved_by", approvedBy)

	// Get transfer
	transfer, err := s.transferRepo.GetByID(ctx, transferRequestID)
	if err != nil {
		return fmt.Errorf("failed to get transfer: %w", err)
	}

	// Validate status
	if transfer.Status != payout.TransferStatusPendingApproval {
		return fmt.Errorf("transfer not pending approval: status=%s", transfer.Status)
	}

	// Approve
	now := time.Now()
	if err := s.transferRepo.ApproveTransfer(ctx, transferRequestID, approvedBy, payout.ApprovalModeManual, now); err != nil {
		return fmt.Errorf("failed to approve transfer: %w", err)
	}

	s.logger.InfoWith("Transfer approved successfully", "transfer_request_id", transferRequestID)
	return nil
}

// CancelTransfer cancels a pending transfer
func (s *PayoutService) CancelTransfer(ctx context.Context, transferRequestID string, cancelledBy string, reason string) error {
	s.logger.InfoWith("CancelTransfer called",
		"transfer_request_id", transferRequestID,
		"cancelled_by", cancelledBy,
		"reason", reason)

	// Get transfer
	transfer, err := s.transferRepo.GetByID(ctx, transferRequestID)
	if err != nil {
		return fmt.Errorf("failed to get transfer: %w", err)
	}

	// Validate can be cancelled
	if transfer.Status == payout.TransferStatusInitiated ||
		transfer.Status == payout.TransferStatusProcessing ||
		transfer.Status == payout.TransferStatusCompleted {
		return fmt.Errorf("cannot cancel transfer in status: %s", transfer.Status)
	}

	// Cancel
	now := time.Now()
	if err := s.transferRepo.UpdateStatus(ctx, transferRequestID, payout.TransferStatusCancelled, now); err != nil {
		return fmt.Errorf("failed to cancel transfer: %w", err)
	}

	s.logger.InfoWith("Transfer cancelled successfully",
		"transfer_request_id", transferRequestID,
		"reason", reason)
	return nil
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

	// Update voucher payout status
	if err := s.voucherRepo.UpdatePayoutStatus(ctx, transfer.VoucherID, voucher.PayoutStatusCompleted, utr); err != nil {
		return fmt.Errorf("failed to update voucher status: %w", err)
	}

	s.logger.InfoWith("Transfer manually completed successfully",
		"transfer_request_id", transferRequestID,
		"utr", utr,
		"completed_by", completedBy)
	s.logger.DebugWith("TODO: Add audit logging for manual completions")
	return nil
}
