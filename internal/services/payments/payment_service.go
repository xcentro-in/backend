package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	paymentdb "xcentro.in/internal/adapters/database/postgres/payment"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PaymentService orchestrates payment operations with proper layering
type PaymentService struct {
	orchestrator         payment.PaymentOrchestrator
	paymentRepo          payment.PaymentRepository
	storedCardRepo       payment.StoredCardRepository
	bankAccountRepo      payment.BankAccountRepository
	walletService        business_rules.WalletService
	feeCalculator        business_rules.FeeCalculator
	txManager            client.TxManager
	voucherService       interface{}                                 // voucher.VoucherService, using interface{} to avoid circular import (Phase 6)
	voucherRepo          interface{}                                 // voucher.VoucherRepository, using interface{} to avoid circular import
	pendingVoucherRepo   *paymentdb.PaymentVoucherPendingRepository // Concrete type for temporary voucher-payment linkage
	businessService      business.BusinessService
	config               *config.Config // Application configuration (server URL, etc.)
	logger               *logger.Logger
}

// NewPaymentService creates a new payment service with all dependencies
func NewPaymentService(
	orchestrator payment.PaymentOrchestrator,
	paymentRepo payment.PaymentRepository,
	storedCardRepo payment.StoredCardRepository,
	bankAccountRepo payment.BankAccountRepository,
	walletService business_rules.WalletService,
	feeCalculator business_rules.FeeCalculator,
	txManager client.TxManager,
	voucherService interface{}, // Phase 6: Voucher service for payment integration
	voucherRepo interface{}, // voucher.VoucherRepository for transactional operations
	pendingVoucherRepo interface{}, // PaymentVoucherPendingRepository for temporary voucher-payment linkage
	businessService business.BusinessService,
	cfg *config.Config, // Application configuration
	logger *logger.Logger,
) *PaymentService {
	s := &PaymentService{
		orchestrator:       orchestrator,
		paymentRepo:        paymentRepo,
		storedCardRepo:     storedCardRepo,
		bankAccountRepo:    bankAccountRepo,
		walletService:      walletService,
		feeCalculator:      feeCalculator,
		txManager:          txManager,
		voucherService:     voucherService,
		voucherRepo:        voucherRepo,
		pendingVoucherRepo: pendingVoucherRepo.(*paymentdb.PaymentVoucherPendingRepository),
		businessService:    businessService,
		config:             cfg,
		logger:             logger,
	}

	// NOTE: Voucher finalization happens in callback handler (when payment succeeds)
	// During payment initiation, we only store pending voucher-payment relationships
	// This ensures vouchers are only finalized when payment actually succeeds

	return s
}

// ProcessPayment orchestrates the complete payment flow with transactional atomicity
// All payment and voucher operations are wrapped in a single database transaction
func (s *PaymentService) ProcessPayment(ctx context.Context, req payment.PaymentRequest) (*payment.PaymentResponse, error) {
	s.logger.InfoWith("Starting payment processing",
		"business_id", req.BusinessID,
		"user_id", req.UserID,
		"amount", req.Amount,
		"currency", req.Currency,
		"payment_method", req.PaymentMethod)

	// Step 1: Calculate fees before transaction (read-only operation)
	s.logger.DebugWith("Calculating payment fees")
	feeCalculation, err := s.feeCalculator.CalculateFee(
		req.Amount,
		req.PaymentMethod,
		getCardTypeFromRequest(&req),
		getCardCategoryFromRequest(&req),
	)
	if err != nil {
		s.logger.ErrorWith("Fee calculation failed", "error", err)
		return nil, fmt.Errorf("fee calculation failed: %w", err)
	}

	s.logger.DebugWith("Fee calculation completed",
		"base_amount", feeCalculation.BaseAmount,
		"fee_amount", feeCalculation.FeeAmount,
		"total_amount", feeCalculation.TotalAmount,
		"currency", feeCalculation.Currency)

	// Step 2: Generate payment ID and callback URL (no DB operations)
	paymentID := uuid.Must(uuid.NewV7()).String()
	s.logger.DebugWith("Generated payment ID", "payment_id", paymentID)

	callbackURL := s.generateCallbackURL(req.CallingDevice, paymentID)
	s.logger.DebugWith("Generated callback URL",
		"calling_device", req.CallingDevice,
		"callback_url", callbackURL)

	// Step 3: Wrap all database operations in a transaction
	// This ensures atomicity: either everything succeeds or everything rolls back
	var response *payment.PaymentResponse
	err = s.txManager.WithTx(ctx, func(txCtx context.Context, tx client.Tx) error {
		s.logger.DebugWith("Starting transactional payment processing", "payment_id", paymentID)

		// Create transactional repository instances
		txPaymentRepo := s.paymentRepo.WithTx(tx)
		txStoredCardRepo := s.storedCardRepo.WithTx(tx)

		// Sub-step 0.5: For tokenization payments, create stored_card record with partial data (Phase 1)
		// This allows us to track the card even before tokenization completes
		var storedCardID *string
		if req.PaymentType == payment.PaymentTypeTokenization && req.CardDetails != nil {
			s.logger.InfoWith("TOKENIZATION: Creating stored_card record with partial data (Phase 1)")

			// Generate card ID
			cardID := uuid.Must(uuid.NewV7()).String()

			// Create stored_card with partial data (no token yet)
			storedCard := &payment.StoredCard{
				CardID:         cardID,
				BusinessID:     req.BusinessID,
				LastFourDigits: req.CardDetails.LastFourDigits,
				CardType:       req.CardDetails.CardType,
				CardCategory:   req.CardDetails.CardCategory,
				ExpiryMonth:    req.CardDetails.ExpiryMonth,
				ExpiryYear:     req.CardDetails.ExpiryYear,
				CardHolderName: req.CardDetails.HolderName,
				Token:          "", // Will be filled in Phase 2 (after tokenization)
				IssuerBank:     "", // Will be filled in Phase 2
				IsActive:       false, // Not active until tokenization succeeds
				GatewayUsed:    "", // Will be updated in Phase 2 after gateway selection
				AddedAt:        time.Now(),
				Metadata: map[string]any{
					"status": "pending_tokenization",
					"phase":  "1_partial_data",
				},
			}

			if err := txStoredCardRepo.CreateStoredCard(txCtx, storedCard); err != nil {
				s.logger.ErrorWith("Failed to create stored_card record", "error", err)
				return fmt.Errorf("failed to create stored_card record: %w", err)
			}

			storedCardID = &cardID
			s.logger.InfoWith("TOKENIZATION: Stored card created with partial data",
				"card_id", cardID,
				"last_four", storedCard.LastFourDigits,
				"card_type", storedCard.CardType,
				"card_category", storedCard.CardCategory)
		}

		// Sub-step 1: Create payment record within transaction
		s.logger.DebugWith("Creating payment record within transaction")

		// Calculate GST breakdown (18% GST on convenience fee)
		convenienceFee := feeCalculation.FeeAmount
		convenienceFeeGST := int64(float64(convenienceFee) * 0.18)
		totalFee := convenienceFee + convenienceFeeGST

		p := &payment.Payment{
			ID:               paymentID,
			UserID:           req.UserID,
			BusinessID:       req.BusinessID,
			Amount:           req.Amount,
			Currency:         req.Currency,
			PaymentMethod:    req.PaymentMethod,
			PaymentType:      payment.PaymentTypePayment,
			Status:           payment.PaymentStatusPending,
			Description:      req.Description,
			CallbackURL:      callbackURL,
			FeeAmount:        totalFee,              // Total fee (base + GST)
			ConvenienceFee:   convenienceFee,        // Base convenience fee before GST
			ConvenienceFeeGST: convenienceFeeGST,    // 18% GST on convenience fee
			TotalAmount:      req.Amount + totalFee, // Base amount + total fee
			Metadata:         req.Metadata,
		}

		// Set payment source details for different payment methods
		// CRITICAL: These fields must be set for payment_source tracking to work
		if req.PaymentType == payment.PaymentTypeTokenization && storedCardID != nil {
			// For tokenization payments, link to the stored_card we just created
			cardSourceType := payment.PaymentSourceCard
			p.PaymentSourceID = storedCardID
			p.PaymentSourceType = &cardSourceType
			s.logger.InfoWith("TOKENIZATION: Payment linked to stored_card",
				"payment_id", paymentID,
				"stored_card_id", *storedCardID,
				"source_type", "card")
		} else if req.PaymentMethod == payment.PaymentMethodTokenCard {
			if req.TokenCardDetails != nil && req.TokenCardDetails.CardID != "" {
				cardSourceType := payment.PaymentSourceCard
				p.PaymentSourceID = &req.TokenCardDetails.CardID
				p.PaymentSourceType = &cardSourceType
				s.logger.InfoWith("Payment source set for tokenized card payment",
					"payment_id", paymentID,
					"card_id", req.TokenCardDetails.CardID,
					"source_type", "card")
			} else {
				// This is a critical error - tokenized payment without card_id
				s.logger.ErrorWith("CRITICAL: token_card payment missing TokenCardDetails or CardID",
					"payment_id", paymentID,
					"payment_method", req.PaymentMethod,
					"has_token_details", req.TokenCardDetails != nil)
				return fmt.Errorf("token_card payment requires TokenCardDetails with valid CardID")
			}
		} else if req.PaymentMethod == payment.PaymentMethodNetBanking && req.NetBankingDetails != nil {
			// Note: For net_banking, we may not have a payment_source_id unless it's from saved account
			// This is expected behavior for one-time bank account usage
			s.logger.DebugWith("Net banking payment without payment_source (one-time use)",
				"payment_id", paymentID,
				"bank_name", req.NetBankingDetails.BankName)
		} else {
			// For card, upi, wallet methods without source tracking
			s.logger.DebugWith("Payment created without payment_source tracking",
				"payment_id", paymentID,
				"payment_method", req.PaymentMethod)
		}

		if err := txPaymentRepo.CreatePayment(txCtx, p); err != nil {
			s.logger.ErrorWith("Failed to create payment record", "error", err)
			return fmt.Errorf("failed to create payment record: %w", err)
		}
		s.logger.InfoWith("Payment record created in database",
			"payment_id", paymentID,
			"payment_method", req.PaymentMethod,
			"has_source_id", p.PaymentSourceID != nil,
			"has_source_type", p.PaymentSourceType != nil)

		// Sub-step 2: Call orchestrator (gateway communication - may fail)
		req.PaymentID = paymentID
		req.CallbackURL = callbackURL
		s.logger.DebugWith("Calling payment orchestrator")
		resp, err := s.orchestrator.ProcessPayment(txCtx, req)
		if err != nil {
			// Gateway call failed - transaction will rollback, payment record will not persist
			s.logger.ErrorWith("Payment orchestrator failed - transaction will rollback", "error", err)
			return fmt.Errorf("payment processing failed: %w", err)
		}
		s.logger.DebugWith("Orchestrator returned successfully", "gateway_used", resp.GatewayUsed)

		// Sub-step 3: Update payment with gateway response
		s.logger.DebugWith("Updating payment with gateway response")
		if err := txPaymentRepo.UpdateGatewayPaymentID(txCtx, paymentID, resp.GatewayPaymentID, resp.RedirectURL); err != nil {
			s.logger.ErrorWith("Failed to update gateway payment ID", "error", err)
			return fmt.Errorf("failed to update gateway payment ID: %w", err)
		}
		s.logger.DebugWith("Payment updated with gateway details")

		// Sub-step 3.5: For tokenization payments, store gateway metadata (contains card details for immature PGs)
		if req.PaymentType == payment.PaymentTypeTokenization && resp.Metadata != nil {
			// Merge gateway metadata into payment metadata
			if p.Metadata == nil {
				p.Metadata = make(map[string]any)
			}
			for k, v := range resp.Metadata {
				p.Metadata[k] = v
			}
			s.logger.DebugWith("Stored gateway metadata for tokenization", "keys", len(resp.Metadata))
		}

		// Sub-step 4: Store pending voucher relationships (within same transaction)
		// Vouchers will be finalized later in callback handler when payment succeeds
		if len(req.VoucherIDs) > 0 && s.pendingVoucherRepo != nil {
			s.logger.InfoWith("Storing pending voucher relationships", "count", len(req.VoucherIDs))

			// Create transactional repository - no reflection needed!
			txPendingRepo := s.pendingVoucherRepo.WithTx(tx)

			// Parse payment ID
			paymentUUID, parseErr := uuid.Parse(paymentID)
			if parseErr != nil {
				s.logger.ErrorWith("Invalid payment ID format", "payment_id", paymentID, "error", parseErr)
				return fmt.Errorf("invalid payment ID: %w", parseErr)
			}

			// Parse voucher IDs
			voucherUUIDs := make([]uuid.UUID, 0, len(req.VoucherIDs))
			for _, voucherIDStr := range req.VoucherIDs {
				voucherUUID, parseErr := uuid.Parse(voucherIDStr)
				if parseErr != nil {
					s.logger.ErrorWith("Invalid voucher ID format", "voucher_id", voucherIDStr, "error", parseErr)
					return fmt.Errorf("invalid voucher ID: %w", parseErr)
				}
				voucherUUIDs = append(voucherUUIDs, voucherUUID)
			}

			// Store pending voucher relationships
			s.logger.DebugWith("Calling StorePendingVouchers", "payment_id", paymentID, "voucher_count", len(voucherUUIDs))
			if storeErr := txPendingRepo.StorePendingVouchers(txCtx, paymentUUID, voucherUUIDs); storeErr != nil {
				s.logger.ErrorWith("Failed to store pending vouchers", "error", storeErr)
				return fmt.Errorf("failed to store pending vouchers: %w", storeErr)
			}
			s.logger.InfoWith("Pending vouchers stored successfully", "count", len(voucherUUIDs))
		}

		// Sub-step 5: Store response for return (transaction successful)
		response = resp
		s.logger.InfoWith("Transaction completed successfully", "payment_id", paymentID)
		return nil // Commit transaction
	})

	// Check transaction result
	if err != nil {
		// Transaction rolled back - no data persisted
		s.logger.ErrorWith("Payment processing transaction failed - all changes rolled back", "error", err)
		return nil, err
	}

	// Success - all changes committed
	s.logger.InfoWith("Payment processed successfully",
		"payment_id", response.PaymentID,
		"gateway_used", response.GatewayUsed)
	s.logger.DebugWith("Redirect URL generated", "redirect_url", response.RedirectURL)

	// Update onboarding flag (non-blocking, first-time only, outside transaction)
	if s.businessService != nil {
		flags, err := s.businessService.GetBusinessOnboardingFlags(ctx, req.BusinessID)
		if err == nil && !flags.HasCreatedFirstPayment {
			_ = s.businessService.UpdateOnboardingStatus(ctx, req.BusinessID, "has_created_first_payment", true)
			s.logger.DebugWith("Onboarding flag updated",
				"business_id", req.BusinessID,
				"flag", "has_created_first_payment")
		}
	}

	s.logger.InfoWith("Payment processing completed", "payment_id", paymentID)

	return response, nil
}

// GetBusinessWallet retrieves wallet for a business
func (s *PaymentService) GetBusinessWallet(ctx context.Context, businessID string) (*payment.BusinessWallet, error) {
	s.logger.DebugWith("Fetching business wallet", "business_id", businessID)
	return s.walletService.GetBusinessWallet(ctx, businessID)
}

// AddPaymentSource adds a payment source to business wallet
func (s *PaymentService) AddPaymentSource(ctx context.Context, businessID string, source payment.PaymentSource) error {
	s.logger.DebugWith("Adding payment source to wallet",
		"business_id", businessID,
		"source_type", source.GetSourceType())
	return s.walletService.AddPaymentSource(ctx, businessID, source)
}

// TokenizeCard tokenizes a card and stores it in the wallet
// Creates a ₹1 payment record for the tokenization transaction
func (s *PaymentService) TokenizeCard(ctx context.Context, req payment.TokenizeRequest) (*payment.TokenizeResponse, error) {
	s.logger.InfoWith("Starting card tokenization", "business_id", req.BusinessID)

	// Step 1: Generate UUID for tokenization payment at application level
	paymentID := uuid.Must(uuid.NewV7()).String()
	s.logger.DebugWith("Generated payment ID for tokenization", "payment_id", paymentID)

	// Step 2: Create payment record for ₹1 tokenization transaction
	s.logger.DebugWith("Creating tokenization payment record")
	p := &payment.Payment{
		ID:            paymentID,
		UserID:        req.UserID,
		BusinessID:    req.BusinessID,
		Amount:        100, // ₹1.00 in paise (tokenization charge)
		Currency:      payment.CurrencyINR,
		PaymentMethod: payment.PaymentMethodCard,
		PaymentType:   payment.PaymentTypeTokenization, // Mark as tokenization payment
		Status:        payment.PaymentStatusPending,
		Description:   "Card tokenization (₹1 verification charge)",
		CallbackURL:   "",  // No callback URL needed for tokenization
		FeeAmount:     0,   // No fee for tokenization
		TotalAmount:   100, // ₹1.00 total
		Metadata: map[string]any{
			"card_type":     string(req.CardDetails.CardType),
			"card_category": string(req.CardDetails.CardCategory),
			"last_four":     req.CardDetails.LastFourDigits,
		},
	}

	if err := s.paymentRepo.CreatePayment(ctx, p); err != nil {
		s.logger.ErrorWith("Failed to create tokenization payment record", "error", err)
		return nil, fmt.Errorf("failed to create tokenization payment record: %w", err)
	}

	s.logger.InfoWith("Tokenization payment record created", "payment_id", p.ID)

	// Step 3: Pass payment ID to orchestrator as transaction ID
	req.TransactionID = paymentID

	// Step 4: Call orchestrator for tokenization
	s.logger.DebugWith("Calling payment orchestrator for tokenization")
	response, err := s.orchestrator.TokenizeCard(ctx, req)
	if err != nil {
		// Update payment status to failed
		failureMetadata := map[string]any{"error": err.Error()}
		_ = s.paymentRepo.UpdatePaymentStatus(ctx, paymentID, payment.PaymentStatusFailed, failureMetadata)

		s.logger.ErrorWith("Card tokenization failed", "error", err)
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	// Step 5: Update payment status to success
	successMetadata := map[string]any{
		"card_id":      response.CardID,
		"token":        response.Token,
		"gateway_used": string(response.GatewayUsed),
	}
	_ = s.paymentRepo.UpdatePaymentStatus(ctx, paymentID, payment.PaymentStatusSuccess, successMetadata)

	s.logger.InfoWith("Card tokenized successfully",
		"card_id", response.CardID,
		"payment_id", paymentID)
	return response, nil
}

// GetFeeRules retrieves fee calculation rules
func (s *PaymentService) GetFeeRules(paymentMethod payment.PaymentMethod) ([]*business_rules.FeeRule, error) {
	s.logger.DebugWith("Fetching fee rules", "payment_method", paymentMethod)
	return s.feeCalculator.GetFeeRules(paymentMethod)
}

// GetAllFeeRules retrieves all fee calculation rules
func (s *PaymentService) GetAllFeeRules() ([]*business_rules.FeeRule, error) {
	s.logger.DebugWith("Fetching all fee rules")
	return s.feeCalculator.GetAllFeeRules()
}

// CalculateFee calculates fee for a payment amount
func (s *PaymentService) CalculateFee(amount int64, paymentMethod payment.PaymentMethod, cardType payment.CardType, cardCategory payment.CardCategory) (*business_rules.FeeCalculation, error) {
	return s.feeCalculator.CalculateFee(amount, paymentMethod, cardType, cardCategory)
}

// Helper functions

func getCardTypeFromRequest(req *payment.PaymentRequest) payment.CardType {
	if req.CardDetails != nil {
		return req.CardDetails.CardType
	}
	if req.TokenCardDetails != nil {
		// TODO: Lookup card type from stored card
		return ""
	}
	return ""
}

func getCardCategoryFromRequest(req *payment.PaymentRequest) payment.CardCategory {
	if req.CardDetails != nil {
		return req.CardDetails.CardCategory
	}
	if req.TokenCardDetails != nil {
		// TODO: Lookup card category from stored card
		return ""
	}
	return ""
}

// generateCallbackURL generates the appropriate callback URL based on calling device
// Returns user-facing HTML page URLs for web/POS and deep links for mobile
func (s *PaymentService) generateCallbackURL(device payment.CallingDevice, paymentID string) string {
	serverURL := s.config.Server.URL

	switch device {
	case payment.CallingDeviceWeb:
		// Web callback - HTML page showing payment status
		return fmt.Sprintf("%s/v1/process_payment/callback_page?transaction_id=%s&source=web", serverURL, paymentID)

	case payment.CallingDeviceMobile:
		// Mobile callback - deep link to mobile app (handled separately by app)
		return fmt.Sprintf("%s/v1/process_payment/callback?payment_id=%s&source=mobile", serverURL, paymentID)

	case payment.CallingDevicePOS:
		// POS callback - HTML page showing payment status
		return fmt.Sprintf("%s/v1/process_payment/callback_page?transaction_id=%s&source=pos", serverURL, paymentID)

	default:
		// Default to web callback with HTML page
		return fmt.Sprintf("%s/v1/process_payment/callback_page?transaction_id=%s&source=web", serverURL, paymentID)
	}
}

// GetPaymentList retrieves paginated payment list for a business
func (s *PaymentService) GetPaymentList(ctx context.Context, businessID string, limit, offset int) (*payment.PaymentListResponse, error) {
	s.logger.InfoWith("Fetching payment list",
		"business_id", businessID,
		"limit", limit,
		"offset", offset)

	// Validate business ID
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Set default pagination if not provided
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100 // Max 100 items per page
	}
	if offset < 0 {
		offset = 0
	}

	// Fetch payment list from repository
	items, totalCount, err := s.paymentRepo.GetPaymentListForBusiness(ctx, businessID, limit, offset)
	if err != nil {
		s.logger.ErrorWith("failed to get payment list",
			"error", err,
			"business_id", businessID)
		return nil, fmt.Errorf("failed to get payment list: %w", err)
	}

	s.logger.InfoWith("Successfully retrieved payment list",
		"business_id", businessID,
		"count", len(items),
		"total_count", totalCount)

	return &payment.PaymentListResponse{
		Payments:   items,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// GetPaymentByID retrieves detailed payment information by payment ID
func (s *PaymentService) GetPaymentByID(ctx context.Context, paymentID string, businessID string) (*payment.Payment, error) {
	s.logger.InfoWith("Fetching payment by ID",
		"payment_id", paymentID,
		"business_id", businessID)

	// Validate payment ID
	if paymentID == "" {
		return nil, fmt.Errorf("payment_id is required")
	}

	// Validate business ID
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Fetch payment from repository
	paymentRecord, err := s.paymentRepo.GetPaymentByID(ctx, paymentID)
	if err != nil {
		s.logger.ErrorWith("failed to get payment by ID",
			"error", err,
			"payment_id", paymentID)
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	// Check if payment exists
	if paymentRecord == nil {
		s.logger.WarnWith("payment not found",
			"payment_id", paymentID)
		return nil, fmt.Errorf("payment not found")
	}

	// Verify payment belongs to the business (authorization check)
	if paymentRecord.BusinessID != businessID {
		s.logger.WarnWith("payment does not belong to business",
			"payment_id", paymentID,
			"requested_business_id", businessID,
			"actual_business_id", paymentRecord.BusinessID)
		return nil, fmt.Errorf("payment not found")
	}

	s.logger.InfoWith("Successfully retrieved payment",
		"payment_id", paymentID,
		"business_id", businessID,
		"status", paymentRecord.Status,
		"amount", paymentRecord.Amount)

	return paymentRecord, nil
}

// RetryPayment retries a failed or pending payment with the same payment source and vouchers
// Maximum 1 retry allowed per payment, within 7 days of original payment
func (s *PaymentService) RetryPayment(ctx context.Context, originalPaymentID string, callingDevice payment.CallingDevice) (*payment.PaymentResponse, error) {
	s.logger.InfoWith("Starting payment retry",
		"original_payment_id", originalPaymentID,
		"calling_device", callingDevice)

	// Step 1: Validate and fetch original payment
	originalPayment, err := s.paymentRepo.GetPaymentByID(ctx, originalPaymentID)
	if err != nil {
		s.logger.ErrorWith("Failed to fetch original payment", "error", err, "payment_id", originalPaymentID)
		return nil, fmt.Errorf("failed to fetch original payment: %w", err)
	}
	if originalPayment == nil {
		s.logger.WarnWith("Original payment not found", "payment_id", originalPaymentID)
		return nil, fmt.Errorf("payment not found: %s", originalPaymentID)
	}

	s.logger.DebugWith("Original payment fetched",
		"payment_id", originalPaymentID,
		"status", originalPayment.Status,
		"created_at", originalPayment.CreatedAt,
		"payment_method", originalPayment.PaymentMethod)

	// Step 2: Validate payment status
	if originalPayment.Status == payment.PaymentStatusSuccess {
		return nil, fmt.Errorf("cannot retry successful payment")
	}

	// Step 3: Validate retry window (7 days)
	maxRetryWindow := 7 * 24 * time.Hour
	timeSinceCreation := time.Since(originalPayment.CreatedAt)
	if timeSinceCreation > maxRetryWindow {
		s.logger.WarnWith("Payment retry window expired",
			"payment_id", originalPaymentID,
			"created_at", originalPayment.CreatedAt,
			"days_since_creation", timeSinceCreation.Hours()/24)
		return nil, fmt.Errorf("retry window expired (max 7 days)")
	}

	// Step 4: Check max 1 retry (if RetryCount is set and > 0, already retried)
	if originalPayment.RetryCount != nil && *originalPayment.RetryCount > 0 {
		return nil, fmt.Errorf("payment already has a retry (max 1 retry allowed)")
	}

	// Step 5: Fetch pending vouchers
	paymentUUID, err := uuid.Parse(originalPaymentID)
	if err != nil {
		s.logger.ErrorWith("Invalid payment ID format", "error", err, "payment_id", originalPaymentID)
		return nil, fmt.Errorf("invalid payment ID format: %w", err)
	}

	voucherIDs, err := s.pendingVoucherRepo.GetPendingVouchers(ctx, paymentUUID)
	if err != nil {
		s.logger.ErrorWith("Failed to fetch pending vouchers", "error", err, "payment_id", originalPaymentID)
		return nil, fmt.Errorf("failed to fetch vouchers: %w", err)
	}
	if len(voucherIDs) == 0 {
		s.logger.WarnWith("No vouchers found for payment", "payment_id", originalPaymentID)
		return nil, fmt.Errorf("no vouchers found for payment %s (payment may have already been finalized by another payment)", originalPaymentID)
	}

	s.logger.InfoWith("Fetched pending vouchers for retry",
		"payment_id", originalPaymentID,
		"voucher_count", len(voucherIDs))

	// Step 6: Validate vouchers are still in draft status (if voucher repo available)
	if s.voucherRepo != nil {
		if err := s.validateVouchersAreInDraft(ctx, voucherIDs); err != nil {
			s.logger.WarnWith("Voucher validation failed", "error", err, "payment_id", originalPaymentID)
			return nil, err
		}
	}

	// Step 7: Build retry payment request
	voucherIDStrings := make([]string, len(voucherIDs))
	for i, vID := range voucherIDs {
		voucherIDStrings[i] = vID.String()
	}

	retryRequest := payment.PaymentRequest{
		UserID:        originalPayment.UserID,
		BusinessID:    originalPayment.BusinessID,
		Amount:        originalPayment.Amount, // Base amount (fees will be recalculated)
		Currency:      originalPayment.Currency,
		PaymentMethod: originalPayment.PaymentMethod,
		Description:   fmt.Sprintf("Retry of payment %s", originalPaymentID),
		CallingDevice: callingDevice,
		VoucherIDs:    voucherIDStrings,
		Metadata: map[string]any{
			"retry_of":             originalPaymentID,
			"retry_count":          1, // Always 1 since max 1 retry allowed
			"retry_reason":         "user_initiated",
			"original_created_at":  originalPayment.CreatedAt.Format(time.RFC3339),
			"original_status":      string(originalPayment.Status),
			"original_gateway":     string(originalPayment.GatewayUsed),
		},
	}

	// Set payment source details based on payment method
	if originalPayment.PaymentMethod == payment.PaymentMethodTokenCard {
		if originalPayment.PaymentSourceID == nil {
			s.logger.ErrorWith("CRITICAL: Original token_card payment missing payment_source_id",
				"payment_id", originalPaymentID)
			return nil, fmt.Errorf("original payment missing payment source information")
		}
		retryRequest.TokenCardDetails = &payment.TokenCardDetails{
			CardID: *originalPayment.PaymentSourceID,
		}
		s.logger.InfoWith("Set token card details for retry",
			"card_id", *originalPayment.PaymentSourceID)
	}

	// Step 8: Process retry payment through normal flow
	s.logger.InfoWith("Processing retry payment",
		"original_payment_id", originalPaymentID,
		"amount", retryRequest.Amount,
		"voucher_count", len(retryRequest.VoucherIDs))

	retryResponse, err := s.ProcessPayment(ctx, retryRequest)
	if err != nil {
		s.logger.ErrorWith("Retry payment processing failed", "error", err, "original_payment_id", originalPaymentID)
		return nil, fmt.Errorf("retry payment failed: %w", err)
	}

	// Step 9: Update retry metadata columns on the new payment
	// This ensures retry_of_payment_id and retry_count are set in database columns (not just metadata JSON)
	s.logger.DebugWith("Updating retry metadata columns", "retry_payment_id", retryResponse.PaymentID)
	if err := s.paymentRepo.UpdateRetryMetadata(ctx, retryResponse.PaymentID, originalPaymentID, 1); err != nil {
		// Non-fatal error - payment was created and processed, but metadata update failed
		s.logger.ErrorWith("Failed to update retry metadata (non-fatal)", "error", err,
			"retry_payment_id", retryResponse.PaymentID,
			"original_payment_id", originalPaymentID)
		// Continue anyway - the retry info is in the metadata JSON field
	}

	s.logger.InfoWith("Payment retry successful",
		"original_payment_id", originalPaymentID,
		"retry_payment_id", retryResponse.PaymentID,
		"gateway_used", retryResponse.GatewayUsed,
		"retry_metadata_updated", true)

	// Add retry metadata to response
	if retryResponse.Metadata == nil {
		retryResponse.Metadata = make(map[string]any)
	}
	retryResponse.Metadata["is_retry"] = true
	retryResponse.Metadata["retry_of"] = originalPaymentID
	retryResponse.Metadata["retry_count"] = 1

	return retryResponse, nil
}

// validateVouchersAreInDraft checks if vouchers are still in draft status
func (s *PaymentService) validateVouchersAreInDraft(ctx context.Context, voucherIDs []uuid.UUID) error {
	// Type assert voucherRepo to access GetByID method
	type voucherGetter interface {
		GetByID(ctx context.Context, voucherID uuid.UUID) (interface{}, error)
	}

	repo, ok := s.voucherRepo.(voucherGetter)
	if !ok {
		s.logger.WarnWith("Voucher repository doesn't implement GetByID, skipping validation")
		return nil
	}

	for _, voucherID := range voucherIDs {
		voucherInterface, err := repo.GetByID(ctx, voucherID)
		if err != nil {
			s.logger.ErrorWith("Failed to fetch voucher", "error", err, "voucher_id", voucherID)
			return fmt.Errorf("failed to fetch voucher %s: %w", voucherID, err)
		}
		if voucherInterface == nil {
			return fmt.Errorf("voucher %s not found", voucherID)
		}

		// Try to extract status from voucher
		type statusGetter interface {
			GetStatus() string
		}

		if v, ok := voucherInterface.(statusGetter); ok {
			status := v.GetStatus()
			if status != "draft" {
				s.logger.WarnWith("Voucher not in draft status",
					"voucher_id", voucherID,
					"status", status)
				return fmt.Errorf("voucher %s is not in draft status (current: %s). It may have been paid by another payment", voucherID, status)
			}
		} else {
			s.logger.DebugWith("Unable to check voucher status via interface",
				"voucher_id", voucherID)
		}
	}

	s.logger.DebugWith("All vouchers validated for retry", "count", len(voucherIDs))
	return nil
}
