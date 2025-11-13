package workers

import (
	"context"
	"fmt"
	"time"

	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/core/voucher"
	payoutService "xcentro.in/internal/services/payout"
)

// PayoutWorker orchestrates automated payout processing
// Runs periodically to process vouchers ready for payout
type PayoutWorker struct {
	voucherRepo     voucher.VoucherRepository
	transferRepo    payout.PayoutTransferRepository
	mappingRepo     payout.PayoutProviderMappingRepository
	configRepo      payout.PayoutConfigurationRepository
	payoutService   *payoutService.PayoutService
	providerFactory payoutService.PayoutProviderFactory
}

// NewPayoutWorker creates a new payout worker
func NewPayoutWorker(
	voucherRepo voucher.VoucherRepository,
	transferRepo payout.PayoutTransferRepository,
	mappingRepo payout.PayoutProviderMappingRepository,
	configRepo payout.PayoutConfigurationRepository,
	payoutService *payoutService.PayoutService,
	providerFactory payoutService.PayoutProviderFactory,
) *PayoutWorker {
	return &PayoutWorker{
		voucherRepo:     voucherRepo,
		transferRepo:    transferRepo,
		mappingRepo:     mappingRepo,
		configRepo:      configRepo,
		payoutService:   payoutService,
		providerFactory: providerFactory,
	}
}

// Run executes all worker tasks
func (w *PayoutWorker) Run(ctx context.Context) error {
	fmt.Println("=== PAYOUT WORKER: Starting run ===")

	// Task 1: Process vouchers ready for payout
	if err := w.processReadyVouchers(ctx); err != nil {
		fmt.Printf("❌ Error processing ready vouchers: %v\n", err)
	}

	// Task 2: Initiate approved transfers
	if err := w.initiateApprovedTransfers(ctx); err != nil {
		fmt.Printf("❌ Error initiating approved transfers: %v\n", err)
	}

	// Task 3: Flag timeout transfers
	if err := w.flagTimeoutTransfers(ctx); err != nil {
		fmt.Printf("❌ Error flagging timeout transfers: %v\n", err)
	}

	// Task 4: Retry failed transfers (PLACEHOLDER)
	if err := w.retryFailedTransfers(ctx); err != nil {
		fmt.Printf("❌ Error retrying failed transfers: %v\n", err)
	}

	fmt.Println("=== PAYOUT WORKER: Run complete ===")
	return nil
}

// processReadyVouchers finds settled vouchers and creates transfer requests
func (w *PayoutWorker) processReadyVouchers(ctx context.Context) error {
	fmt.Println("\n📋 Task 1: Processing ready vouchers...")

	// Get global config for batch size
	config, err := w.configRepo.GetConfig(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	if !config.WorkerEnabled {
		fmt.Println("⏸️  Worker disabled in configuration")
		return nil
	}

	// Query vouchers ready for payout
	// TODO: Need to add GetVouchersReadyForPayout method to voucher repository
	// For now, using placeholder logic
	fmt.Printf("🔍 Querying vouchers ready for payout (batch_size=%d)...\n", config.BatchSize)

	// Placeholder: Would query:
	// WHERE payment.settlement_status = 'settled'
	//   AND voucher.payment_id IS NOT NULL
	//   AND voucher.payout_status = 'pending'
	//   AND voucher.cancellation_status = 'not_cancelled'
	//   AND NOT EXISTS (SELECT 1 FROM payout_transfer_requests WHERE voucher_id = voucher.voucher_id)
	// LIMIT config.BatchSize

	fmt.Println("TODO: Implement GetVouchersReadyForPayout repository method")
	fmt.Println("ℹ️  Skipping for MVP - manual initiation via API")

	return nil
}

// initiateApprovedTransfers sends approved transfers to Castler
func (w *PayoutWorker) initiateApprovedTransfers(ctx context.Context) error {
	fmt.Println("\n🚀 Task 2: Initiating approved transfers...")

	// Get global config
	config, err := w.configRepo.GetConfig(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Get all transfers with status='initiated' (created but not yet sent to Castler)
	transfers, err := w.transferRepo.GetByStatus(ctx, payout.TransferStatusInitiated, config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get initiated transfers: %w", err)
	}

	if len(transfers) == 0 {
		fmt.Println("✅ No initiated transfers to send to provider")
		return nil
	}

	fmt.Printf("📦 Found %d initiated transfers to send to provider\n", len(transfers))

	// Process each transfer
	for _, transfer := range transfers {
		if err := w.initiateTransfer(ctx, transfer, config); err != nil {
			fmt.Printf("❌ Failed to initiate transfer %s: %v\n", transfer.TransferRequestID, err)
			continue
		}
	}

	return nil
}

// initiateTransfer sends a single transfer to the provider
func (w *PayoutWorker) initiateTransfer(ctx context.Context, transfer *payout.PayoutTransferRequest, config *payout.PayoutConfiguration) error {
	fmt.Printf("  📤 Sending transfer to provider: %s (amount=%d)\n", transfer.TransferRequestID, transfer.Amount)

	// Get provider
	provider, err := w.providerFactory.GetProvider(transfer.Provider)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Call Castler payout API with our transfer_request_id as customerRefId
	createReq := &payout.CreateTransferRequest{
		TransferRequestID:     transfer.TransferRequestID, // Used as customerRefId in Castler
		VoucherID:             transfer.VoucherID,
		ProviderBeneficiaryID: transfer.ProviderBeneficiaryID,
		EscrowID:              transfer.EscrowID, // Castler escrow account ID
		Amount:                transfer.Amount,
		Currency:              transfer.Currency,
		Purpose:               transfer.Purpose,
		Reference:             transfer.Reference,
		Narration:             transfer.Narration,
		TransferMode:          transfer.TransferMode,
	}

	resp, err := provider.CreateTransferRequest(ctx, createReq)
	if err != nil {
		// Log error - ops can manually intervene
		fmt.Printf("  ⚠️  Provider API error: %v\n", err)
		return fmt.Errorf("provider API error: %w", err)
	}

	fmt.Printf("  ✅ Transfer sent to provider: %s (Castler transferId: %s)\n",
		transfer.TransferRequestID, resp.ProviderTransferID)

	// Update transfer status to 'processing' (Castler auto-processes)
	// No separate approval step - Castler confirms via settlement webhook
	now := time.Now()
	if err := w.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusProcessing, now); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Store Castler's provider transfer ID (their transferId)
	if err := w.transferRepo.UpdateProviderTransferID(ctx, transfer.TransferRequestID, resp.ProviderTransferID); err != nil {
		return fmt.Errorf("failed to update provider transfer ID: %w", err)
	}

	// Set webhook timeout (processing_at + timeout_minutes)
	timeoutAt := now.Add(time.Duration(config.WebhookTimeoutMinutes) * time.Minute)
	if err := w.transferRepo.SetWebhookTimeout(ctx, transfer.TransferRequestID, timeoutAt); err != nil {
		return fmt.Errorf("failed to set webhook timeout: %w", err)
	}

	fmt.Printf("  ✅ Transfer initiated successfully (timeout_at: %s)\n", timeoutAt.Format(time.RFC3339))
	return nil
}

// flagTimeoutTransfers marks transfers as timeout if no webhook received
func (w *PayoutWorker) flagTimeoutTransfers(ctx context.Context) error {
	fmt.Println("\n⏰ Task 3: Flagging timeout transfers...")

	// Get global config
	config, err := w.configRepo.GetConfig(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Get transfers that exceeded webhook timeout
	transfers, err := w.transferRepo.GetWebhookTimeouts(ctx, config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get timeout transfers: %w", err)
	}

	if len(transfers) == 0 {
		fmt.Println("✅ No timeout transfers found")
		return nil
	}

	fmt.Printf("⚠️  Found %d transfers that exceeded webhook timeout\n", len(transfers))

	// Flag each as timeout
	for _, transfer := range transfers {
		fmt.Printf("  ⏰ Flagging transfer as timeout: %s (initiated_at=%s)\n",
			transfer.TransferRequestID,
			transfer.InitiatedAt.Format(time.RFC3339))

		// Update status to 'timeout'
		now := time.Now()
		if err := w.transferRepo.UpdateStatus(ctx, transfer.TransferRequestID, payout.TransferStatusTimeout, now); err != nil {
			fmt.Printf("  ❌ Failed to update status: %v\n", err)
			continue
		}

		// TODO: Send alert to ops team for manual investigation
		fmt.Printf("  🚨 TODO: Send alert to ops team for transfer: %s\n", transfer.TransferRequestID)
		fmt.Printf("  ℹ️  Ops team can check payout status with Castler and manually complete if needed\n")

		fmt.Printf("  ✅ Transfer flagged as timeout\n")
	}

	return nil
}

// retryFailedTransfers retries failed transfers (PLACEHOLDER)
func (w *PayoutWorker) retryFailedTransfers(ctx context.Context) error {
	fmt.Println("\n🔄 Task 4: Retrying failed transfers...")
	fmt.Println("TODO: Implement retry strategy with exponential backoff")
	fmt.Println("TODO: Consider unified retry strategy across payments/payouts")
	fmt.Println("ℹ️  For MVP: Ops team handles failed transfers manually")
	return nil
}
