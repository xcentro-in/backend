package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
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
	businessService business.BusinessService
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
	businessService business.BusinessService,
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
		businessService: businessService,
		logger:          logger,
	}

	// Inject voucher service into orchestrator (Phase 6)
	if voucherService != nil {
		orchestrator.SetVoucherService(voucherService)
		logger.Info("Voucher service wired into payment orchestrator")
	}

	return s
}

// ProcessPayment orchestrates the complete payment flow
// Creates payment record, calculates fees, calls orchestrator
func (s *PaymentService) ProcessPayment(ctx context.Context, req payment.PaymentRequest) (*payment.PaymentResponse, error) {
	s.logger.InfoWith("Starting payment processing",
		"business_id", req.BusinessID,
		"user_id", req.UserID,
		"amount", req.Amount,
		"currency", req.Currency,
		"payment_method", req.PaymentMethod)

	// Step 1: Calculate fees before creating payment record
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

	// Step 2: Generate UUID for payment at application level
	paymentID := uuid.Must(uuid.NewV7()).String()
	s.logger.DebugWith("Generated payment ID", "payment_id", paymentID)

	// Step 3: Generate callback URL based on calling device
	callbackURL := s.generateCallbackURL(req.CallingDevice, paymentID)
	s.logger.DebugWith("Generated callback URL",
		"calling_device", req.CallingDevice,
		"callback_url", callbackURL)

	// Step 4: Create initial payment record with 'pending' status
	s.logger.DebugWith("Creating initial payment record")
	p := &payment.Payment{
		ID:            paymentID,
		UserID:        req.UserID,
		BusinessID:    req.BusinessID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		PaymentMethod: req.PaymentMethod,
		PaymentType:   payment.PaymentTypePayment, // Regular payment
		Status:        payment.PaymentStatusPending,
		Description:   req.Description,
		CallbackURL:   callbackURL,
		FeeAmount:     feeCalculation.FeeAmount,
		TotalAmount:   feeCalculation.TotalAmount,
		Metadata:      req.Metadata,
	}

	if err := s.paymentRepo.CreatePayment(ctx, p); err != nil {
		s.logger.ErrorWith("Failed to create payment record", "error", err)
		return nil, fmt.Errorf("failed to create payment record: %w", err)
	}

	s.logger.InfoWith("Payment record created", "payment_id", paymentID)

	// Step 5: Add payment_id to request for orchestrator
	req.PaymentID = paymentID

	// Step 6: Call orchestrator to process payment
	s.logger.DebugWith("Calling payment orchestrator")
	response, err := s.orchestrator.ProcessPayment(ctx, req)

	if err != nil {
		// Orchestrator already updated status to 'failed'
		s.logger.ErrorWith("Payment orchestrator failed", "error", err)
		return nil, fmt.Errorf("payment processing failed: %w", err)
	}

	s.logger.InfoWith("Payment processed successfully",
		"payment_id", response.PaymentID,
		"gateway_used", response.GatewayUsed)
	s.logger.DebugWith("Redirect URL generated", "redirect_url", response.RedirectURL)

	// Update onboarding flag (non-blocking, first-time only)
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
func (s *PaymentService) GetBusinessWallet(businessID string) (*payment.BusinessWallet, error) {
	s.logger.DebugWith("Fetching business wallet", "business_id", businessID)
	return s.walletService.GetBusinessWallet(businessID)
}

// AddPaymentSource adds a payment source to business wallet
func (s *PaymentService) AddPaymentSource(businessID string, source payment.PaymentSource) error {
	s.logger.DebugWith("Adding payment source to wallet",
		"business_id", businessID,
		"source_type", source.GetSourceType())
	return s.walletService.AddPaymentSource(businessID, source)
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
func (s *PaymentService) generateCallbackURL(device payment.CallingDevice, paymentID string) string {
	// TODO: Make base URLs configurable via environment variables
	baseURL := "https://api.xcentro.in" // Production base URL

	switch device {
	case payment.CallingDeviceWeb:
		// Web callback - redirect to web application
		return fmt.Sprintf("%s/v1/process_payment/callback?payment_id=%s&source=web", baseURL, paymentID)

	case payment.CallingDeviceMobile:
		// Mobile callback - deep link to mobile app
		return fmt.Sprintf("%s/v1/process_payment/callback?payment_id=%s&source=mobile", baseURL, paymentID)

	case payment.CallingDevicePOS:
		// POS callback - return to POS application
		return fmt.Sprintf("%s/v1/process_payment/callback?payment_id=%s&source=pos", baseURL, paymentID)

	default:
		// Default to web callback
		return fmt.Sprintf("%s/v1/process_payment/callback?payment_id=%s&source=web", baseURL, paymentID)
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
