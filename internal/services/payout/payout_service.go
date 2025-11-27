package payout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// PayoutService orchestrates payout transfer creation and management
type PayoutService struct {
	paymentRepo     payment.PaymentRepository
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	bankAccountRepo payee.PayeeBankAccountRepository
	configRepo      payout.PayoutConfigurationRepository
	stateChangeRepo payment.PaymentStateChangeRepository
	providerFactory PayoutProviderFactory // Interface to get payout provider
	logger          *logger.Logger
}

// PayoutProviderFactory is an interface to get payout providers
type PayoutProviderFactory interface {
	GetProvider(providerType payout.ProviderType) (payout.PayoutProvider, error)
}

// NewPayoutService creates a new payout service
func NewPayoutService(
	paymentRepo payment.PaymentRepository,
	voucherRepo voucher.VoucherRepository,
	transferRepo payout.PayoutTransferRepository,
	bankAccountRepo payee.PayeeBankAccountRepository,
	configRepo payout.PayoutConfigurationRepository,
	stateChangeRepo payment.PaymentStateChangeRepository,
	providerFactory PayoutProviderFactory,
) *PayoutService {
	return &PayoutService{
		paymentRepo:     paymentRepo,
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		bankAccountRepo: bankAccountRepo,
		configRepo:      configRepo,
		stateChangeRepo: stateChangeRepo,
		providerFactory: providerFactory,
		logger:          logger.GetDefault(),
	}
}

// InitiateManualPayout creates payout transfer requests for a payment
// New flow: payment_id (required) + voucher_ids[] (optional) + bypassSettlementCheck (optional)
// If voucher_ids empty: processes all unpaid vouchers for the payment
// If bypassSettlementCheck = false (default): requires payment.settlement_status = 'settled'
// If bypassSettlementCheck = true: allows payout even if not settled (ops override for emergencies)
// Returns all created transfer requests (status='initiated')
func (s *PayoutService) InitiateManualPayout(ctx context.Context, paymentID string, voucherIDs []string, bypassSettlementCheck bool) ([]*payout.PayoutTransferRequest, error) {
	s.logger.InfoWith("InitiateManualPayout called",
		"payment_id", paymentID,
		"voucher_ids", voucherIDs,
		"bypass_settlement_check", bypassSettlementCheck)

	// Step 1: Parse payment ID
	paymentUUID, err := uuid.Parse(paymentID)
	if err != nil {
		return nil, fmt.Errorf("invalid payment_id format: %w", err)
	}

	// Step 2: Get payment details for settlement validation
	pmt, err := s.paymentRepo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	if pmt == nil {
		return nil, fmt.Errorf("payment not found: %s", paymentID)
	}

	// Step 3: Validate settlement status (unless bypassed by ops)
	if !bypassSettlementCheck && pmt.SettlementStatus != "settled" {
		s.logger.WarnWith("Payout rejected - payment not settled",
			"payment_id", paymentID,
			"settlement_status", pmt.SettlementStatus)
		return nil, fmt.Errorf(
			"payment not settled (current status: %s). Settlement must complete before payout can be initiated. "+
				"Use bypass_checks=true in request if you need to override this (ops discretion only)",
			pmt.SettlementStatus,
		)
	}

	if bypassSettlementCheck {
		s.logger.WarnWith("⚠️  SETTLEMENT CHECK BYPASSED - Manual override by ops",
			"payment_id", paymentID,
			"settlement_status", pmt.SettlementStatus)
	} else {
		s.logger.InfoWith("✅ Settlement check passed",
			"payment_id", paymentID,
			"settlement_status", pmt.SettlementStatus)
	}

	// Step 4: Get all vouchers for this payment
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

// PayoutInitiationResult represents the result of initiating a payout
type PayoutInitiationResult struct {
	PaymentID     string                    `json:"payment_id"`
	TransferCount int                       `json:"transfer_count"`
	Transfers     []PayoutTransferDetail    `json:"transfers"` // Detailed transfer info with voucher mapping
	TotalAmount   int64                     `json:"total_amount"`
	Currency      string                    `json:"currency"`
	Status        string                    `json:"status"`
	Message       string                    `json:"message"`
}

// PayoutTransferDetail represents details of a single transfer in the response
type PayoutTransferDetail struct {
	TransferID string `json:"transfer_id"`
	VoucherID  string `json:"voucher_id"`
	Amount     int64  `json:"amount"`
	Status     string `json:"status"`
}

// InitiateAndProcessPayout creates transfer requests and immediately processes them async
// This is the main entry point for manual payout initiation from ops dashboard
func (s *PayoutService) InitiateAndProcessPayout(
	ctx context.Context,
	paymentID string,
	voucherIDs []string,
	initiatedBy string,
	bypassChecks bool,
) (*PayoutInitiationResult, error) {
	s.logger.InfoWith("InitiateAndProcessPayout called",
		"payment_id", paymentID,
		"voucher_count", len(voucherIDs),
		"initiated_by", initiatedBy,
		"bypass_checks", bypassChecks)

	// Step 1: Create transfer requests with status='initiated'
	transfers, err := s.InitiateManualPayout(ctx, paymentID, voucherIDs, bypassChecks)
	if err != nil {
		return nil, fmt.Errorf("failed to create transfer requests: %w", err)
	}

	s.logger.InfoWith("Transfer requests created", "count", len(transfers))

	// Step 2: Spawn async processing for all transfers
	go s.processTransfersAsync(context.Background(), transfers)

	// Step 3: Build result with detailed transfer info
	transferDetails := make([]PayoutTransferDetail, len(transfers))
	totalAmount := int64(0)
	for i, t := range transfers {
		transferDetails[i] = PayoutTransferDetail{
			TransferID: t.TransferRequestID,
			VoucherID:  t.VoucherID,
			Amount:     t.Amount,
			Status:     string(t.Status), // "initiated"
		}
		totalAmount += t.Amount
	}

	return &PayoutInitiationResult{
		PaymentID:     paymentID,
		TransferCount: len(transfers),
		Transfers:     transferDetails,
		TotalAmount:   totalAmount,
		Currency:      "INR",
		Status:        "initiated",
		Message:       "Payout initiated - processing in background",
	}, nil
}

// processTransfersAsync processes multiple transfers in parallel (background goroutines)
func (s *PayoutService) processTransfersAsync(ctx context.Context, transfers []*payout.PayoutTransferRequest) {
	s.logger.InfoWith("processTransfersAsync started", "count", len(transfers))

	// Launch goroutine for each transfer
	for _, transfer := range transfers {
		go s.processSingleTransfer(ctx, transfer)
	}

	s.logger.InfoWith("All transfer goroutines launched", "count", len(transfers))
}

// processSingleTransfer processes a single transfer: calls provider API, updates status, logs activity
func (s *PayoutService) processSingleTransfer(ctx context.Context, transfer *payout.PayoutTransferRequest) {
	s.logger.InfoWith("Processing transfer",
		"transfer_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID,
		"amount", transfer.Amount)

	// Parse UUIDs for activity tracking
	transferUUID, err := uuid.Parse(transfer.TransferRequestID)
	if err != nil {
		s.logger.ErrorWith("Invalid transfer_request_id UUID", "error", err)
		return
	}

	paymentUUID, err := uuid.Parse(transfer.PaymentID)
	if err != nil {
		s.logger.ErrorWith("Invalid payment_id UUID", "error", err)
		return
	}

	// Log: Payout initiated
	s.logPayoutInitiated(ctx, paymentUUID, transferUUID, transfer)

	// Get provider
	provider, err := s.providerFactory.GetProvider(transfer.Provider)
	if err != nil {
		s.logger.ErrorWith("Failed to get provider", "error", err)
		s.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Failed to get provider: %v", err))
		return
	}

	// Call provider API
	createReq := &payout.CreateTransferRequest{
		TransferRequestID:     transfer.TransferRequestID,
		VoucherID:             transfer.VoucherID,
		ProviderBeneficiaryID: transfer.ProviderBeneficiaryID,
		EscrowID:              transfer.EscrowID,
		Amount:                transfer.Amount,
		Currency:              transfer.Currency,
		Purpose:               transfer.Purpose,
		Reference:             transfer.Reference,
		Narration:             transfer.Narration,
		TransferMode:          transfer.TransferMode,
	}

	resp, err := provider.CreateTransferRequest(ctx, createReq)
	if err != nil {
		s.logger.ErrorWith("Provider API error", "error", err)
		s.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Provider API error: %v", err))
		return
	}

	s.logger.InfoWith("Provider API success",
		"transfer_id", transfer.TransferRequestID,
		"provider_transfer_id", resp.ProviderTransferID)

	// Update status to 'pending'
	pendingTime := time.Now()
	if err := s.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusPending, pendingTime); err != nil {
		s.logger.ErrorWith("Failed to update status to pending", "error", err)
		return
	}

	// Store provider transfer ID
	if err := s.transferRepo.UpdateProviderTransferID(ctx, transfer.TransferRequestID, resp.ProviderTransferID); err != nil {
		s.logger.ErrorWith("Failed to update provider transfer ID", "error", err)
		return
	}

	// Log: Payout pending
	s.logPayoutPending(ctx, paymentUUID, transferUUID, transfer, resp.ProviderTransferID, pendingTime)

	// Set webhook timeout
	config, err := s.configRepo.GetConfig(ctx, nil)
	if err == nil {
		timeoutAt := pendingTime.Add(time.Duration(config.WebhookTimeoutMinutes) * time.Minute)
		if err := s.transferRepo.SetWebhookTimeout(ctx, transfer.TransferRequestID, timeoutAt); err != nil {
			s.logger.ErrorWith("Failed to set webhook timeout", "error", err)
		}
	}

	s.logger.InfoWith("Transfer processed successfully",
		"transfer_id", transfer.TransferRequestID,
		"status", "pending")
}

// logPayoutInitiated logs payout_initiated activity
func (s *PayoutService) logPayoutInitiated(ctx context.Context, paymentID, transferID uuid.UUID, transfer *payout.PayoutTransferRequest) {
	_, err := s.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		TransferID:      &transferID,
		Action:          payment.PayoutActionInitiated,
		ActionTimestamp: time.Now(),
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_service",
		NewState:        "initiated",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"transfer_mode": transfer.TransferMode,
			"purpose":       transfer.Purpose,
		},
		Remarks: fmt.Sprintf("Manual payout initiated for transfer %s", transfer.TransferRequestID),
	})
	if err != nil {
		s.logger.ErrorWith("Failed to log payout initiated", "error", err)
	}
}

// logPayoutPending logs payout_processing activity when API succeeds
func (s *PayoutService) logPayoutPending(
	ctx context.Context,
	paymentID, transferID uuid.UUID,
	transfer *payout.PayoutTransferRequest,
	providerTransferID string,
	timestamp time.Time,
) {
	_, err := s.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentID,
		TransferID:      &transferID,
		Action:          payment.PayoutActionProcessing,
		ActionTimestamp: timestamp,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_service",
		PreviousState:   "initiated",
		NewState:        "pending",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"provider_transfer_id": providerTransferID,
			"transfer_mode":        transfer.TransferMode,
		},
		Remarks: fmt.Sprintf("Provider API success - now pending settlement (Provider transferId: %s)", providerTransferID),
	})
	if err != nil {
		s.logger.ErrorWith("Failed to log payout pending", "error", err)
	}
}

// markTransferFailed updates transfer status and logs failure activity
func (s *PayoutService) markTransferFailed(
	ctx context.Context,
	transfer *payout.PayoutTransferRequest,
	transferUUID, paymentUUID uuid.UUID,
	reason string,
) {
	failedTime := time.Now()

	// Update status
	if err := s.transferRepo.MarkFailed(ctx, transfer.TransferRequestID, reason, failedTime); err != nil {
		s.logger.ErrorWith("Failed to mark transfer as failed", "error", err)
		return
	}

	// Log activity
	_, err := s.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionFailed,
		ActionTimestamp: failedTime,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_service",
		PreviousState:   "initiated",
		NewState:        "failed",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"failure_reason": reason,
		},
		Remarks: fmt.Sprintf("Payout failed: %s", reason),
	})
	if err != nil {
		s.logger.ErrorWith("Failed to log payout failure", "error", err)
	}

	s.logger.ErrorWith("Transfer marked as failed",
		"transfer_id", transfer.TransferRequestID,
		"reason", reason)
}
