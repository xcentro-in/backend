package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
	payoutService "xcentro.in/internal/services/payout"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PayoutWorker orchestrates automated payout processing
// Runs periodically to process vouchers ready for payout
type PayoutWorker struct {
	paymentRepo     payment.PaymentRepository
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	stateChangeRepo payment.PaymentStateChangeRepository
	payoutService   *payoutService.PayoutService
	providerFactory payoutService.PayoutProviderFactory
	cfg             *config.Config
	logger          *logger.Logger
}

// NewPayoutWorker creates a new payout worker
func NewPayoutWorker(
	paymentRepo payment.PaymentRepository,
	voucherRepo voucher.VoucherRepository,
	transferRepo payout.PayoutTransferRepository,
	stateChangeRepo payment.PaymentStateChangeRepository,
	payoutService *payoutService.PayoutService,
	providerFactory payoutService.PayoutProviderFactory,
	cfg *config.Config,
) *PayoutWorker {
	return &PayoutWorker{
		paymentRepo:     paymentRepo,
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		stateChangeRepo: stateChangeRepo,
		payoutService:   payoutService,
		providerFactory: providerFactory,
		cfg:             cfg,
		logger:          logger.GetDefault(),
	}
}

// Run executes all worker tasks
func (w *PayoutWorker) Run(ctx context.Context) error {
	w.logger.Info("Payout worker: Starting run")

	// Task 1: Monitor initiated transfers (catch orphaned transfers from async failures)
	if err := w.monitorInitiatedTransfers(ctx); err != nil {
		w.logger.ErrorWith("Task 1 failed: Error monitoring initiated transfers", "error", err)
	}

	// Task 2: Flag timeout transfers (no webhook received after 60 minutes)
	if err := w.flagTimeoutTransfers(ctx); err != nil {
		w.logger.ErrorWith("Task 2 failed: Error flagging timeout transfers", "error", err)
	}

	// Task 3: Retry failed transfers (PLACEHOLDER - future enhancement)
	if err := w.retryFailedTransfers(ctx); err != nil {
		w.logger.ErrorWith("Task 3 failed: Error retrying failed transfers", "error", err)
	}

	// Task 4: Auto-payout for settled payments (main functionality)
	if err := w.ProcessSettledPaymentsForPayout(ctx, w.cfg.Workers.AutoPayout.BatchSize, w.cfg.Workers.AutoPayout.Enabled); err != nil {
		w.logger.ErrorWith("Task 4 failed: Error processing settled payments for payout", "error", err)
	}

	w.logger.Info("Payout worker: Run complete")
	return nil
}

// monitorInitiatedTransfers monitors transfers stuck in 'initiated' status
// Manual payouts process immediately via async goroutines, so this should rarely find anything
// Auto-retries orphaned transfers that failed async processing
func (w *PayoutWorker) monitorInitiatedTransfers(ctx context.Context) error {
	w.logger.Info("Task 1: Monitoring initiated transfers")

	// Use hardcoded batch size
	const batchSize = 50

	// Get all transfers with status='initiated' (should be empty or very few)
	transfers, err := w.transferRepo.GetByStatus(ctx, payout.TransferStatusInitiated, batchSize)
	if err != nil {
		return fmt.Errorf("failed to get initiated transfers: %w", err)
	}

	if len(transfers) == 0 {
		w.logger.Info("No initiated transfers found (as expected)")
		return nil
	}

	// WARNING: Finding initiated transfers indicates async processing failed
	w.logger.WarnWith("Found transfers stuck in initiated status - starting auto-retry",
		"count", len(transfers))

	// Track retry stats
	var successCount, errorCount int

	// Auto-retry each stuck transfer
	for _, transfer := range transfers {
		w.logger.WarnWith("Stuck transfer detected - attempting auto-retry",
			"transfer_request_id", transfer.TransferRequestID,
			"voucher_id", transfer.VoucherID,
			"created_at", transfer.CreatedAt.Format(time.RFC3339))

		// Retry the transfer by calling Castler API via service
		// This completes the work that the crashed goroutine never finished
		if err := w.payoutService.ProcessTransferToCastler(ctx, transfer); err != nil {
			w.logger.ErrorWith("Auto-retry failed for stuck transfer",
				"error", err,
				"transfer_request_id", transfer.TransferRequestID)
			errorCount++
			continue
		}

		w.logger.InfoWith("Auto-retry successful - stuck transfer recovered",
			"transfer_request_id", transfer.TransferRequestID)
		successCount++
	}

	// Log summary
	w.logger.InfoWith("Task 1 complete - initiated transfers recovery",
		"recovered", successCount,
		"failed", errorCount,
		"total", len(transfers))

	return nil
}

// processTransferAsync processes a single transfer in background goroutine
// Calls Castler API via service layer to move transfer from 'initiated' to 'processing' status
func (w *PayoutWorker) processTransferAsync(ctx context.Context, transfer *payout.PayoutTransferRequest) {
	w.logger.InfoWith("Processing transfer async",
		"transfer_request_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID,
		"amount", transfer.Amount)

	// Call Castler API via service (centralizes all API logic + status updates)
	if err := w.payoutService.ProcessTransferToCastler(ctx, transfer); err != nil {
		w.logger.ErrorWith("Async transfer processing failed",
			"error", err,
			"transfer_request_id", transfer.TransferRequestID,
			"voucher_id", transfer.VoucherID)
		// Error already logged and status updated to 'failed' by service
		return
	}

	w.logger.InfoWith("Async transfer processing completed successfully",
		"transfer_request_id", transfer.TransferRequestID,
		"voucher_id", transfer.VoucherID)
}

// flagTimeoutTransfers marks transfers as timeout if no webhook received
func (w *PayoutWorker) flagTimeoutTransfers(ctx context.Context) error {
	w.logger.Info("Task 2: Flagging timeout transfers")

	// Use hardcoded batch size and timeout from config
	const batchSize = 50
	webhookTimeoutMinutes := w.cfg.Payout.WebhookTimeoutMinutes

	// Get transfers that exceeded webhook timeout
	transfers, err := w.transferRepo.GetWebhookTimeouts(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("failed to get timeout transfers: %w", err)
	}

	if len(transfers) == 0 {
		w.logger.Info("No timeout transfers found")
		return nil
	}

	w.logger.WarnWith("Found transfers that exceeded webhook timeout",
		"count", len(transfers))

	// Flag each as timeout
	for _, transfer := range transfers {
		w.logger.WarnWith("Flagging transfer as timeout",
			"transfer_request_id", transfer.TransferRequestID,
			"initiated_at", transfer.InitiatedAt.Format(time.RFC3339))

		// Update status to 'timeout'
		now := time.Now()
		if err := w.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusTimeout, now); err != nil {
			w.logger.ErrorWith("Failed to update transfer status to timeout",
				"error", err,
				"transfer_request_id", transfer.TransferRequestID)
			continue
		}

		// Record activity: Payout timeout
		transferUUID, err := uuid.Parse(transfer.TransferRequestID)
		if err != nil {
			w.logger.WarnWith("Invalid transfer_request_id UUID",
				"error", err,
				"transfer_request_id", transfer.TransferRequestID)
		} else {
			paymentUUID, err := uuid.Parse(transfer.PaymentID)
			if err != nil {
				w.logger.WarnWith("Invalid payment_id UUID",
					"error", err,
					"payment_id", transfer.PaymentID)
			} else {
				_, err = w.stateChangeRepo.Create(ctx, &payment.CreatePaymentStateChangeRequest{
					PaymentID:       paymentUUID,
					TransferID:      &transferUUID,
					Action:          payment.PayoutActionTimeout,
					ActionTimestamp: now,
					ActorType:       payment.ActorTypeSystem,
					ActorName:       "payout_worker",
					PreviousState:   "pending",
					NewState:        "timeout",
					Amount:          transfer.Amount,
					Currency:        transfer.Currency,
					Provider:        string(transfer.Provider),
					Remarks:         fmt.Sprintf("No webhook received after %d minutes - flagged for manual investigation", webhookTimeoutMinutes),
				})
				if err != nil {
					w.logger.WarnWith("Failed to create activity record (timeout)",
						"error", err,
						"transfer_request_id", transfer.TransferRequestID)
				}
			}
		}

		// TODO: Send alert to ops team for manual investigation
		w.logger.WarnWith("TODO: Send alert to ops team - transfer timed out",
			"transfer_request_id", transfer.TransferRequestID,
			"action_required", "check_with_castler")

		w.logger.InfoWith("Transfer flagged as timeout",
			"transfer_request_id", transfer.TransferRequestID)
	}

	return nil
}

// retryFailedTransfers retries failed transfers (PLACEHOLDER - future enhancement)
func (w *PayoutWorker) retryFailedTransfers(ctx context.Context) error {
	w.logger.Info("Task 3: Retrying failed transfers")
	w.logger.Info("TODO: Implement retry strategy with exponential backoff")
	w.logger.Info("TODO: Consider unified retry strategy across payments/payouts")
	w.logger.Info("For MVP: Ops team handles failed transfers manually")
	return nil
}

// ProcessSettledPaymentsForPayout automatically processes settled payments for payout
// This is the auto-payout worker that scans for settled payments and initiates payouts
// after performing mathematical validation
func (w *PayoutWorker) ProcessSettledPaymentsForPayout(ctx context.Context, batchSize int, enabled bool) error {
	w.logger.Info("Task 4: Auto-Payout - Processing settled payments")

	// Check if auto-payout is enabled
	if !enabled {
		w.logger.Info("Auto-payout worker disabled in configuration")
		return nil
	}

	// Query settled payments ready for payout
	w.logger.InfoWith("Querying settled payments ready for payout",
		"batch_size", batchSize)
	payments, err := w.paymentRepo.GetSettledPaymentsReadyForPayout(ctx, batchSize)
	if err != nil {
		return fmt.Errorf("failed to query settled payments: %w", err)
	}

	if len(payments) == 0 {
		w.logger.Info("No settled payments ready for payout")
		return nil
	}

	w.logger.InfoWith("Found settled payments ready for payout",
		"count", len(payments))

	// Track processing stats
	var successCount, skipCount, errorCount int

	// Process each payment
	for i, pmt := range payments {
		w.logger.InfoWith("Processing payment",
			"index", fmt.Sprintf("%d/%d", i+1, len(payments)),
			"payment_id", pmt.ID,
			"amount_inr", fmt.Sprintf("%.2f", float64(pmt.TotalAmount)/100.0))

		// Parse payment ID to UUID
		paymentUUID, err := uuid.Parse(pmt.ID)
		if err != nil {
			w.logger.ErrorWith("Invalid payment ID format",
				"error", err,
				"payment_id", pmt.ID)
			errorCount++
			continue
		}

		// Get vouchers linked to this payment
		vouchers, err := w.voucherRepo.GetByPaymentID(ctx, paymentUUID)
		if err != nil {
			w.logger.ErrorWith("Failed to get vouchers for payment",
				"error", err,
				"payment_id", pmt.ID)
			errorCount++
			continue
		}

		if len(vouchers) == 0 {
			w.logger.WarnWith("No vouchers found for payment, skipping",
				"payment_id", pmt.ID)
			skipCount++
			continue
		}

		w.logger.InfoWith("Found vouchers linked to payment",
			"payment_id", pmt.ID,
			"voucher_count", len(vouchers))

		// Perform mathematical validation
		w.logger.InfoWith("Validating payment amounts",
			"payment_id", pmt.ID)
		isValid, reason, err := payoutService.ValidatePaymentAmountsForPayout(ctx, pmt, vouchers)
		if err != nil {
			w.logger.ErrorWith("Validation error",
				"error", err,
				"payment_id", pmt.ID)
			errorCount++
			continue
		}

		if !isValid {
			w.logger.WarnWith("Validation failed - skipping payment",
				"payment_id", pmt.ID,
				"reason", reason,
				"action_required", "manual_review")
			skipCount++
			continue
		}

		w.logger.InfoWith("Validation passed",
			"payment_id", pmt.ID,
			"reason", reason)

		// Initiate payout via existing PayoutService
		// Pass empty voucher IDs list to process all vouchers, bypass_checks=false
		w.logger.InfoWith("Initiating payout for all vouchers",
			"payment_id", pmt.ID)
		transfers, err := w.payoutService.InitiateManualPayout(ctx, pmt.ID, []string{}, false)
		if err != nil {
			w.logger.ErrorWith("Failed to initiate payout",
				"error", err,
				"payment_id", pmt.ID)
			errorCount++
			continue
		}

		// Log results
		w.logger.InfoWith("Payout transfer requests created",
			"payment_id", pmt.ID,
			"transfers_created", len(transfers))
		for _, transfer := range transfers {
			w.logger.InfoWith("Transfer created",
				"transfer_request_id", transfer.TransferRequestID,
				"voucher_id", transfer.VoucherID,
				"amount_inr", fmt.Sprintf("%.2f", float64(transfer.Amount)/100.0))
		}

		// Trigger async processing for each transfer (call Castler API in background)
		// This completes the payout flow immediately without waiting for Task 1
		w.logger.InfoWith("Triggering async Castler API calls",
			"payment_id", pmt.ID,
			"transfer_count", len(transfers))
		for _, transfer := range transfers {
			// Launch goroutine for each transfer (independent processing)
			go w.processTransferAsync(context.Background(), transfer)
		}

		successCount++
	}

	// Print summary
	w.logger.InfoWith("Auto-Payout Summary",
		"successfully_processed", successCount,
		"skipped", skipCount,
		"errors", errorCount,
		"total", len(payments))

	return nil
}
