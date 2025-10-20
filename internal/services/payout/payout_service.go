package payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
)

// PayoutService orchestrates payout transfer creation and management
type PayoutService struct {
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	mappingRepo     payout.PayoutProviderMappingRepository
	configRepo      payout.PayoutConfigurationRepository
	providerFactory PayoutProviderFactory // Interface to get payout provider
}

// PayoutProviderFactory is an interface to get payout providers
type PayoutProviderFactory interface {
	GetProvider(providerType payout.ProviderType) (payout.PayoutProvider, error)
}

// NewPayoutService creates a new payout service
func NewPayoutService(
	voucherRepo voucher.VoucherRepository,
	transferRepo payout.PayoutTransferRepository,
	mappingRepo payout.PayoutProviderMappingRepository,
	configRepo payout.PayoutConfigurationRepository,
	providerFactory PayoutProviderFactory,
) *PayoutService {
	return &PayoutService{
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		mappingRepo:     mappingRepo,
		configRepo:      configRepo,
		providerFactory: providerFactory,
	}
}

// InitiatePayoutForVoucher creates a transfer request for a voucher
// Determines if auto-approval is needed based on amount
func (s *PayoutService) InitiatePayoutForVoucher(ctx context.Context, voucherID string) (*payout.PayoutTransferRequest, error) {
	fmt.Printf("=== PAYOUT SERVICE: InitiatePayoutForVoucher (VoucherID: %s) ===\n", voucherID)

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
		fmt.Println("✅ Transfer request already exists, returning existing")
		return existing, nil
	}

	// Step 4: Get provider mapping (payee + account → Castler)
	mapping, err := s.mappingRepo.GetActiveMapping(ctx, vch.PayeeID.String(), vch.AccountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get provider mapping for payee: %w", err)
	}

	if mapping.OnboardingStatus != payout.OnboardingStatusActive {
		return nil, fmt.Errorf("payee not onboarded with provider: status=%s", mapping.OnboardingStatus)
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
		fmt.Printf("⚠️  Amount %d > auto-approval limit %d, requires manual approval\n", vch.NetAmount, config.AutoApprovalLimit)
	} else {
		approvedBy = "SYSTEM"
		now := time.Now()
		approvedAt = &now
		fmt.Printf("✅ Amount %d <= auto-approval limit %d, auto-approved\n", vch.NetAmount, config.AutoApprovalLimit)
	}

	// Step 7: Create transfer request
	transferReq := &payout.PayoutTransferRequest{
		TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
		VoucherID:             voucherID,
		Provider:              mapping.Provider,
		ProviderBeneficiaryID: mapping.ProviderBeneficiaryID,
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

	fmt.Printf("✅ Transfer request created: %s (status=%s)\n", transferReq.TransferRequestID, transferReq.Status)
	return transferReq, nil
}

// ApproveTransfer manually approves a pending transfer
func (s *PayoutService) ApproveTransfer(ctx context.Context, transferRequestID string, approvedBy string, remarks string) error {
	fmt.Printf("=== PAYOUT SERVICE: ApproveTransfer (TransferID: %s, ApprovedBy: %s) ===\n", transferRequestID, approvedBy)

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

	fmt.Printf("✅ Transfer approved: %s\n", transferRequestID)
	return nil
}

// CancelTransfer cancels a pending transfer
func (s *PayoutService) CancelTransfer(ctx context.Context, transferRequestID string, cancelledBy string, reason string) error {
	fmt.Printf("=== PAYOUT SERVICE: CancelTransfer (TransferID: %s, CancelledBy: %s) ===\n", transferRequestID, cancelledBy)

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

	fmt.Printf("✅ Transfer cancelled: %s (reason: %s)\n", transferRequestID, reason)
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
	fmt.Println("TODO: Implement retry strategy for transfer:", transferRequestID)
	fmt.Println("TODO: Consider unified retry strategy across payments/payouts")
	// For MVP, manual intervention via ops team
	return errors.New("retry not implemented in MVP - manual intervention required")
}

// MarkCompletedManually manually marks a timeout transfer as completed
// Used when ops team verifies completion externally
func (s *PayoutService) MarkCompletedManually(ctx context.Context, transferRequestID string, utr string, completedBy string, remarks string) error {
	fmt.Printf("=== PAYOUT SERVICE: MarkCompletedManually (TransferID: %s, UTR: %s) ===\n", transferRequestID, utr)

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

	// TODO: Audit log
	fmt.Printf("✅ Transfer manually completed: %s (UTR: %s, CompletedBy: %s)\n", transferRequestID, utr, completedBy)
	return nil
}
