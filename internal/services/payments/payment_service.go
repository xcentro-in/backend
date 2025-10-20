package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payment"
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
	}

	// Inject voucher service into orchestrator (Phase 6)
	if voucherService != nil {
		orchestrator.SetVoucherService(voucherService)
		fmt.Println("✅ Voucher service wired into payment orchestrator")
	}

	return s
}

// ProcessPayment orchestrates the complete payment flow
// Creates payment record, calculates fees, calls orchestrator
func (s *PaymentService) ProcessPayment(ctx context.Context, req payment.PaymentRequest) (*payment.PaymentResponse, error) {
	fmt.Println("=== PAYMENT SERVICE: ProcessPayment Called ===")
	fmt.Printf("Business ID: %s, User ID: %s\n", req.BusinessID, req.UserID)
	fmt.Printf("Amount: %d %s, Method: %s\n", req.Amount, req.Currency, req.PaymentMethod)

	// Step 1: Calculate fees before creating payment record
	fmt.Println("💰 Calculating payment fees...")
	feeCalculation, err := s.feeCalculator.CalculateFee(
		req.Amount,
		req.PaymentMethod,
		getCardTypeFromRequest(&req),
		getCardCategoryFromRequest(&req),
	)
	if err != nil {
		fmt.Printf("❌ Fee calculation failed: %v\n", err)
		return nil, fmt.Errorf("fee calculation failed: %w", err)
	}

	fmt.Printf("💰 Fee calculated: Base: %d, Fee: %d, Total: %d %s\n",
		feeCalculation.BaseAmount, feeCalculation.FeeAmount,
		feeCalculation.TotalAmount, feeCalculation.Currency)

	// Step 2: Generate UUID for payment at application level
	paymentID := uuid.Must(uuid.NewV7()).String()
	fmt.Printf("🆔 Generated Payment ID: %s\n", paymentID)

	// Step 3: Generate callback URL based on calling device
	callbackURL := s.generateCallbackURL(req.CallingDevice, paymentID)
	fmt.Printf("🔗 Generated Callback URL for %s: %s\n", req.CallingDevice, callbackURL)

	// Step 4: Create initial payment record with 'pending' status
	fmt.Println("💾 Creating initial payment record...")
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
		fmt.Printf("❌ Failed to create payment record: %v\n", err)
		return nil, fmt.Errorf("failed to create payment record: %w", err)
	}

	fmt.Printf("✅ Payment record created with ID: %s\n", paymentID)

	// Step 5: Add payment_id to request for orchestrator
	req.PaymentID = paymentID

	// Step 6: Call orchestrator to process payment
	fmt.Println("🔄 Calling Payment Orchestrator...")
	response, err := s.orchestrator.ProcessPayment(ctx, req)

	if err != nil {
		// Orchestrator already updated status to 'failed'
		fmt.Printf("❌ Orchestrator failed: %v\n", err)
		return nil, fmt.Errorf("payment processing failed: %w", err)
	}

	fmt.Printf("✅ Payment processed successfully: %s\n", response.PaymentID)
	fmt.Printf("🏦 Gateway Used: %s\n", response.GatewayUsed)
	fmt.Printf("🌐 Redirect URL: %s\n", response.RedirectURL)
	fmt.Println("=== PAYMENT SERVICE: ProcessPayment Complete ===")

	return response, nil
}

// GetBusinessWallet retrieves wallet for a business
func (s *PaymentService) GetBusinessWallet(businessID string) (*payment.BusinessWallet, error) {
	fmt.Printf("=== PAYMENT SERVICE: GetBusinessWallet Called - BusinessID: %s ===\n", businessID)
	return s.walletService.GetBusinessWallet(businessID)
}

// AddPaymentSource adds a payment source to business wallet
func (s *PaymentService) AddPaymentSource(businessID string, source payment.PaymentSource) error {
	fmt.Printf("=== PAYMENT SERVICE: AddPaymentSource Called - BusinessID: %s, SourceType: %s ===\n",
		businessID, source.GetSourceType())
	return s.walletService.AddPaymentSource(businessID, source)
}

// TokenizeCard tokenizes a card and stores it in the wallet
// Creates a ₹1 payment record for the tokenization transaction
func (s *PaymentService) TokenizeCard(ctx context.Context, req payment.TokenizeRequest) (*payment.TokenizeResponse, error) {
	fmt.Printf("=== PAYMENT SERVICE: TokenizeCard Called - BusinessID: %s ===\n", req.BusinessID)

	// Step 1: Generate UUID for tokenization payment at application level
	paymentID := uuid.Must(uuid.NewV7()).String()
	fmt.Printf("🆔 Generated Payment ID for tokenization: %s\n", paymentID)

	// Step 2: Create payment record for ₹1 tokenization transaction
	fmt.Println("💾 Creating tokenization payment record...")
	p := &payment.Payment{
		ID:            paymentID,
		UserID:        "", // Tokenization is business-level, no specific user
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
		fmt.Printf("❌ Failed to create tokenization payment record: %v\n", err)
		return nil, fmt.Errorf("failed to create tokenization payment record: %w", err)
	}

	fmt.Printf("✅ Tokenization payment record created with ID: %s\n", p.ID)

	// Step 3: Pass payment ID to orchestrator as transaction ID
	req.TransactionID = paymentID

	// Step 4: Call orchestrator for tokenization
	fmt.Println("🔄 Calling Payment Orchestrator for tokenization...")
	response, err := s.orchestrator.TokenizeCard(ctx, req)
	if err != nil {
		// Update payment status to failed
		failureMetadata := map[string]any{"error": err.Error()}
		_ = s.paymentRepo.UpdatePaymentStatus(ctx, paymentID, payment.PaymentStatusFailed, failureMetadata)

		fmt.Printf("❌ Tokenization failed: %v\n", err)
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	// Step 5: Update payment status to success
	successMetadata := map[string]any{
		"card_id":      response.CardID,
		"token":        response.Token,
		"gateway_used": string(response.GatewayUsed),
	}
	_ = s.paymentRepo.UpdatePaymentStatus(ctx, paymentID, payment.PaymentStatusSuccess, successMetadata)

	fmt.Printf("✅ Card tokenized successfully: %s\n", response.CardID)
	fmt.Printf("💳 Payment ID: %s (for webhook matching)\n", paymentID)
	return response, nil
}

// GetFeeRules retrieves fee calculation rules
func (s *PaymentService) GetFeeRules(paymentMethod payment.PaymentMethod) ([]*business_rules.FeeRule, error) {
	fmt.Printf("=== PAYMENT SERVICE: GetFeeRules Called - Method: %s ===\n", paymentMethod)
	return s.feeCalculator.GetFeeRules(paymentMethod)
}

// GetAllFeeRules retrieves all fee calculation rules
func (s *PaymentService) GetAllFeeRules() ([]*business_rules.FeeRule, error) {
	fmt.Println("=== PAYMENT SERVICE: GetAllFeeRules Called ===")
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
