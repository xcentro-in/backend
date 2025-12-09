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
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PayoutService orchestrates payout transfer creation and management
type PayoutService struct {
	paymentRepo     payment.PaymentRepository
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	bankAccountRepo payee.PayeeBankAccountRepository
	stateChangeRepo payment.PaymentStateChangeRepository
	providerFactory PayoutProviderFactory // Interface to get payout provider
	config          *config.Config        // Application config (for retry, timeout, RTGS threshold)
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
	stateChangeRepo payment.PaymentStateChangeRepository,
	providerFactory PayoutProviderFactory,
	cfg *config.Config,
) *PayoutService {
	return &PayoutService{
		paymentRepo:     paymentRepo,
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		bankAccountRepo: bankAccountRepo,
		stateChangeRepo: stateChangeRepo,
		providerFactory: providerFactory,
		config:          cfg,
		logger:          logger.GetDefault(),
	}
}

// determineTransferMode selects transfer mode based on amount
// NEFT: ≤ ₹2,00,000 (20000000 paise)
// RTGS: > ₹2,00,000
// IMPS: Future (on-demand payouts, post-MVP)
func (s *PayoutService) determineTransferMode(amountInPaise int64) string {
	if amountInPaise > s.config.Payout.RTGSThresholdAmount {
		return "RTGS"
	}
	return "NEFT"
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
			// Check if already paid (has existing active transfer)
			existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
			if err == nil && existing != nil {
				// Check status
				if existing.Status == payout.TransferStatusCompleted {
					s.logger.DebugWith("Skipping voucher - already completed",
						"voucher_id", v.VoucherID.String(),
						"transfer_id", existing.TransferRequestID)
					continue
				}
				if existing.Status == payout.TransferStatusInitiated || existing.Status == payout.TransferStatusProcessing {
					s.logger.DebugWith("Skipping voucher - transfer already in progress",
						"voucher_id", v.VoucherID.String(),
						"transfer_id", existing.TransferRequestID,
						"status", existing.Status)
					continue
				}
				// If failed or timeout, allow retry (will be handled in create loop below)
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
				// Check existing transfer
				existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
				if err == nil && existing != nil {
					if existing.Status == payout.TransferStatusCompleted {
						return nil, fmt.Errorf("voucher %s already completed (transfer_id: %s)",
							v.VoucherID.String(), existing.TransferRequestID)
					}
					if existing.Status == payout.TransferStatusInitiated || existing.Status == payout.TransferStatusProcessing {
						return nil, fmt.Errorf("voucher %s already in progress (transfer_id: %s, status: %s)",
							v.VoucherID.String(), existing.TransferRequestID, existing.Status)
					}
					// If failed or timeout, allow retry (will be handled in create loop below)
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

	// Step 4: Create transfer requests for each voucher
	var transfers []*payout.PayoutTransferRequest

	for _, vch := range targetVouchers {
		// Check retry count (all attempts including soft deleted)
		retryCount, err := s.transferRepo.GetRetryCount(ctx, vch.VoucherID.String())
		if err != nil {
			s.logger.WarnWith("Failed to get retry count, assuming 0", "voucher_id", vch.VoucherID, "error", err)
			retryCount = 0
		}

		// Enforce max retry limit
		if retryCount >= s.config.Payout.MaxRetryAttempts {
			return nil, fmt.Errorf("max retries exceeded (%d/%d) for voucher %s",
				retryCount, s.config.Payout.MaxRetryAttempts, vch.VoucherID)
		}

		// If this is a retry, soft delete the old failed/timeout transfer
		if retryCount > 0 {
			existing, _ := s.transferRepo.GetByVoucherID(ctx, vch.VoucherID.String())
			if existing != nil && !existing.IsDeleted {
				s.logger.InfoWith("Soft deleting old transfer for retry",
					"old_transfer_id", existing.TransferRequestID,
					"old_status", existing.Status,
					"retry_attempt", retryCount+1)

				if err := s.transferRepo.SoftDelete(ctx, existing.TransferRequestID, "retry"); err != nil {
					return nil, fmt.Errorf("failed to soft delete old transfer: %w", err)
				}
			}
		}

		// Get Castler beneficiary ID (lightweight query - no decryption)
		castlerBeneficiaryID, err := s.bankAccountRepo.GetCastlerBeneficiaryID(ctx, vch.AccountID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to get Castler beneficiary ID for voucher %s: %w", vch.VoucherID.String(), err)
		}

		// Determine transfer mode based on amount
		transferMode := s.determineTransferMode(vch.NetAmount)

		// Create NEW transfer request (fresh UUID for retry)
		transferReq := &payout.PayoutTransferRequest{
			TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
			PaymentID:             paymentID,
			VoucherID:             vch.VoucherID.String(),
			Provider:              payout.ProviderCastler, // Always Castler for now
			ProviderBeneficiaryID: castlerBeneficiaryID,
			EscrowID:              "", // Populated from Castler client config (not needed here)
			Amount:                vch.NetAmount,
			Currency:              vch.Currency,
			Purpose:               "vendor_payment",
			Reference:             vch.InvoiceNumber,     // Invoice number (used as customerRefId in Castler)
			Narration:             fmt.Sprintf("Payment for %s (attempt %d)", vch.InvoiceNumber, retryCount+1),
			TransferMode:          transferMode,          // NEFT (≤₹2L) or RTGS (>₹2L)
			Status:                payout.TransferStatusInitiated,
			RetryCount:            retryCount,  // Store current retry count
			MaxRetries:            s.config.Payout.MaxRetryAttempts,
			IsDeleted:             false,
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

// ProcessTransferToCastler calls Castler API to process a transfer request
// Moves transfer from 'initiated' to 'processing' (success) or 'failed' (error)
// This centralizes the Castler API call logic - used by both handler and worker
func (s *PayoutService) ProcessTransferToCastler(ctx context.Context, transfer *payout.PayoutTransferRequest) error {
	s.logger.InfoWith("Processing transfer to Castler",
		"transfer_request_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID,
		"amount", transfer.Amount)

	// Parse UUIDs for activity tracking
	transferUUID, err := uuid.Parse(transfer.TransferRequestID)
	if err != nil {
		return fmt.Errorf("invalid transfer_request_id UUID: %w", err)
	}

	paymentUUID, err := uuid.Parse(transfer.PaymentID)
	if err != nil {
		return fmt.Errorf("invalid payment_id UUID: %w", err)
	}

	// Record activity: Payout initiated
	now := time.Now()
	_, err = s.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionInitiated,
		ActionTimestamp: now,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_service",
		PreviousState:   "initiated",
		NewState:        "processing",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"transfer_mode": transfer.TransferMode,
			"purpose":       transfer.Purpose,
		},
		Remarks: fmt.Sprintf("Payout processing started for transfer %s", transfer.TransferRequestID),
	})
	if err != nil {
		s.logger.WarnWith("Failed to create activity record (initiated)",
			"error", err,
			"transfer_request_id", transfer.TransferRequestID)
	}

	// Get provider
	provider, err := s.providerFactory.GetProvider(transfer.Provider)
	if err != nil {
		s.logger.ErrorWith("Failed to get provider", "error", err)
		s.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Failed to get provider: %v", err))
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Call Castler payout API
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
		s.logger.ErrorWith("Castler API error", "error", err, "transfer_request_id", transfer.TransferRequestID)
		s.markTransferFailed(ctx, transfer, transferUUID, paymentUUID, fmt.Sprintf("Castler API error: %v", err))
		return fmt.Errorf("castler API error: %w", err)
	}

	s.logger.InfoWith("Transfer sent to Castler successfully",
		"transfer_request_id", transfer.TransferRequestID,
		"provider_transfer_id", resp.ProviderTransferID)

	// Update status to 'processing'
	processingTime := time.Now()
	if err := s.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusProcessing, processingTime); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Store Castler's provider transfer ID
	if err := s.transferRepo.UpdateProviderTransferID(ctx, transfer.TransferRequestID, resp.ProviderTransferID); err != nil {
		return fmt.Errorf("failed to update provider transfer ID: %w", err)
	}

	// Record activity: Payout processing
	_, err = s.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
		PaymentID:       paymentUUID,
		TransferID:      &transferUUID,
		Action:          payment.PayoutActionProcessing,
		ActionTimestamp: processingTime,
		ActorType:       payment.ActorTypeSystem,
		ActorName:       "payout_service",
		PreviousState:   "initiated",
		NewState:        "processing",
		Amount:          transfer.Amount,
		Currency:        transfer.Currency,
		Provider:        string(transfer.Provider),
		Metadata: map[string]any{
			"provider_transfer_id": resp.ProviderTransferID,
			"transfer_mode":        transfer.TransferMode,
		},
		Remarks: fmt.Sprintf("Castler API success - now processing (transferId: %s)", resp.ProviderTransferID),
	})
	if err != nil {
		s.logger.WarnWith("Failed to create activity record (processing)",
			"error", err,
			"transfer_request_id", transfer.TransferRequestID)
	}

	// Set webhook timeout
	webhookTimeoutMinutes := s.config.Payout.WebhookTimeoutMinutes
	timeoutAt := processingTime.Add(time.Duration(webhookTimeoutMinutes) * time.Minute)
	if err := s.transferRepo.SetWebhookTimeout(ctx, transfer.TransferRequestID, timeoutAt); err != nil {
		return fmt.Errorf("failed to set webhook timeout: %w", err)
	}

	s.logger.InfoWith("Transfer processing completed successfully",
		"transfer_request_id", transfer.TransferRequestID,
		"timeout_at", timeoutAt.Format(time.RFC3339))

	return nil
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

	// Step 5: Determine transfer mode based on amount
	transferMode := s.determineTransferMode(vch.NetAmount)

	// Step 6: Create transfer request (simplified - no approval workflow)
	transferReq := &payout.PayoutTransferRequest{
		TransferRequestID:     uuid.Must(uuid.NewV7()).String(),
		PaymentID:             vch.PaymentID.String(),
		VoucherID:             voucherID,
		Provider:              payout.ProviderCastler, // Always Castler for now
		ProviderBeneficiaryID: castlerBeneficiaryID,
		EscrowID:              "", // Populated from Castler client config (not needed here)
		Amount:                vch.NetAmount,
		Currency:              vch.Currency,
		Purpose:               "vendor_payment",
		Reference:             vch.InvoiceNumber,     // Invoice number (used as customerRefId in Castler)
		Narration:             fmt.Sprintf("Payment for %s", vch.InvoiceNumber),
		TransferMode:          transferMode,          // NEFT (≤₹2L) or RTGS (>₹2L)
		Status:                payout.TransferStatusInitiated,
		RetryCount:            0,
		MaxRetries:            s.config.Payout.MaxRetryAttempts,
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
	timeoutAt := pendingTime.Add(time.Duration(s.config.Payout.WebhookTimeoutMinutes) * time.Minute)
	if err := s.transferRepo.SetWebhookTimeout(ctx, transfer.TransferRequestID, timeoutAt); err != nil {
		s.logger.ErrorWith("Failed to set webhook timeout", "error", err)
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
