package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// PaymentService orchestrates payment operations with proper layering
type PaymentService struct {
	orchestrator    payment.PaymentOrchestrator
	paymentRepo     payment.PaymentRepository
	storedCardRepo  payment.StoredCardRepository
	bankAccountRepo payment.BankAccountRepository
	walletService   business_rules.WalletService
	feeCalculator   business_rules.FeeCalculator
	txManager       client.TxManager
	voucherService  interface{} // voucher.VoucherService, using interface{} to avoid circular import (Phase 6)
	voucherRepo     interface{} // voucher.VoucherRepository, using interface{} to avoid circular import
	businessService business.BusinessService
	config          *config.Config // Application configuration (server URL, etc.)
	logger          *logger.Logger
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
	businessService business.BusinessService,
	cfg *config.Config, // Application configuration
	logger *logger.Logger,
) *PaymentService {
	s := &PaymentService{
		orchestrator:    orchestrator,
		paymentRepo:     paymentRepo,
		storedCardRepo:  storedCardRepo,
		bankAccountRepo: bankAccountRepo,
		walletService:   walletService,
		feeCalculator:   feeCalculator,
		txManager:       txManager,
		voucherService:  voucherService,
		voucherRepo:     voucherRepo,
		businessService: businessService,
		config:          cfg,
		logger:          logger,
	}

	// NOTE: Voucher finalization now handled directly in service layer within transaction
	// All payment and voucher operations are wrapped in a single transaction

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

		// Sub-step 1: Create payment record within transaction
		s.logger.DebugWith("Creating payment record within transaction")
		p := &payment.Payment{
			ID:            paymentID,
			UserID:        req.UserID,
			BusinessID:    req.BusinessID,
			Amount:        req.Amount,
			Currency:      req.Currency,
			PaymentMethod: req.PaymentMethod,
			PaymentType:   payment.PaymentTypePayment,
			Status:        payment.PaymentStatusPending,
			Description:   req.Description,
			CallbackURL:   callbackURL,
			FeeAmount:     feeCalculation.FeeAmount,
			TotalAmount:   feeCalculation.TotalAmount,
			Metadata:      req.Metadata,
		}

		if err := txPaymentRepo.CreatePayment(txCtx, p); err != nil {
			s.logger.ErrorWith("Failed to create payment record", "error", err)
			return fmt.Errorf("failed to create payment record: %w", err)
		}
		s.logger.DebugWith("Payment record created", "payment_id", paymentID)

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

		// Sub-step 4: Finalize vouchers if provided (within same transaction)
		if len(req.VoucherIDs) > 0 && s.voucherRepo != nil && s.voucherService != nil {
			s.logger.InfoWith("Finalizing vouchers within transaction", "count", len(req.VoucherIDs))

			// Type assert to get WithTx method
			type voucherRepoInterface interface {
				WithTx(tx interface{}) interface{}
			}
			if vr, ok := s.voucherRepo.(voucherRepoInterface); ok {
				txVoucherRepo := vr.WithTx(tx)

				// Type assert to call FinalizeVoucher method
				type voucherRepoFinalizer interface {
					FinalizeVoucher(ctx context.Context, voucherID uuid.UUID, paymentID uuid.UUID) error
				}
				if finalizer, ok := txVoucherRepo.(voucherRepoFinalizer); ok {
					paymentUUID, _ := uuid.Parse(paymentID)
					for _, voucherIDStr := range req.VoucherIDs {
						voucherUUID, parseErr := uuid.Parse(voucherIDStr)
						if parseErr != nil {
							s.logger.ErrorWith("Invalid voucher ID format", "voucher_id", voucherIDStr, "error", parseErr)
							return fmt.Errorf("invalid voucher ID: %w", parseErr)
						}

						if finalizeErr := finalizer.FinalizeVoucher(txCtx, voucherUUID, paymentUUID); finalizeErr != nil {
							s.logger.ErrorWith("Failed to finalize voucher", "voucher_id", voucherIDStr, "error", finalizeErr)
							return fmt.Errorf("failed to finalize voucher %s: %w", voucherIDStr, finalizeErr)
						}
						s.logger.DebugWith("Voucher finalized", "voucher_id", voucherIDStr)
					}
					s.logger.InfoWith("All vouchers finalized successfully", "count", len(req.VoucherIDs))
				}
			}
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
		go func(bizID string) {
			bgCtx := context.Background()
			flags, err := s.businessService.GetBusinessOnboardingFlags(bgCtx, bizID)
			if err == nil && !flags.HasCreatedFirstPayment {
				_ = s.businessService.UpdateOnboardingStatus(bgCtx, bizID, "has_created_first_payment", true)
				s.logger.DebugWith("Onboarding flag updated",
					"business_id", bizID,
					"flag", "has_created_first_payment")
			}
		}(req.BusinessID)
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
