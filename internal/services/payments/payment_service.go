package payments

import (
	"context"
	"fmt"

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
) *PaymentService {
	return &PaymentService{
		orchestrator:    orchestrator,
		paymentRepo:     paymentRepo,
		storedCardRepo:  storedCardRepo,
		bankAccountRepo: bankAccountRepo,
		walletService:   walletService,
		feeCalculator:   feeCalculator,
		txManager:       txManager,
	}
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

	// Step 2: Create initial payment record with 'pending' status
	fmt.Println("💾 Creating initial payment record...")
	p := &payment.Payment{
		UserID:        req.UserID,
		BusinessID:    req.BusinessID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		PaymentMethod: req.PaymentMethod,
		Status:        payment.PaymentStatusPending,
		Description:   req.Description,
		CallbackURL:   req.CallbackURL,
		FeeAmount:     feeCalculation.FeeAmount,
		TotalAmount:   feeCalculation.TotalAmount,
		Metadata:      req.Metadata,
	}

	if err := s.paymentRepo.CreatePayment(ctx, p); err != nil {
		fmt.Printf("❌ Failed to create payment record: %v\n", err)
		return nil, fmt.Errorf("failed to create payment record: %w", err)
	}

	fmt.Printf("✅ Payment record created with ID: %s\n", p.ID)

	// Step 3: Add payment_id to request for orchestrator
	req.PaymentID = p.ID

	// Step 4: Call orchestrator to process payment
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
func (s *PaymentService) TokenizeCard(ctx context.Context, req payment.TokenizeRequest) (*payment.TokenizeResponse, error) {
	fmt.Printf("=== PAYMENT SERVICE: TokenizeCard Called - BusinessID: %s ===\n", req.BusinessID)

	// Call orchestrator for tokenization
	response, err := s.orchestrator.TokenizeCard(ctx, req)
	if err != nil {
		fmt.Printf("❌ Tokenization failed: %v\n", err)
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	fmt.Printf("✅ Card tokenized successfully: %s\n", response.CardID)
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
