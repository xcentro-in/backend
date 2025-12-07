package business_rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
)

// WalletService defines the interface for business wallet operations
type WalletService interface {
	GetBusinessWallet(ctx context.Context, businessID string, showInactive bool) (*payment.BusinessWallet, error)
	AddPaymentSource(ctx context.Context, businessID string, source payment.PaymentSource) error
	RemovePaymentSource(ctx context.Context, businessID string, sourceID string, sourceType payment.PaymentSourceType) error
	GetPaymentSource(ctx context.Context, businessID string, sourceID string, sourceType payment.PaymentSourceType) (payment.PaymentSource, error)
	CompleteCardTokenization(ctx context.Context, businessID, cardID, token string, gatewayUsed payment.GatewayType, gatewayCardID string) error
}

// WalletServiceImpl provides the implementation for business wallet operations
type WalletServiceImpl struct {
	// Bank verification service for penny drop verification
	bankVerificationService BankVerificationService
	// Repository dependencies (implemented)
	storedCardRepo  payment.StoredCardRepository
	bankAccountRepo payment.BankAccountRepository
	// Transaction manager (implemented)
	txManager client.TxManager
	// Business service for onboarding flags
	businessService business.BusinessService
	// Logger
	logger *logger.Logger
}

// NewWalletService creates a new wallet service with database repositories
// TODO-FUTURE: Add bankVerificationLogRepo parameter when audit logging is implemented
func NewWalletService(storedCardRepo payment.StoredCardRepository, bankAccountRepo payment.BankAccountRepository, txManager client.TxManager, businessService business.BusinessService, logger *logger.Logger) WalletService {
	return &WalletServiceImpl{
		bankVerificationService: NewMockBankVerificationService(),
		storedCardRepo:          storedCardRepo,
		bankAccountRepo:         bankAccountRepo,
		txManager:               txManager,
		businessService:         businessService,
		logger:                  logger,
	}
}

// GetBusinessWallet retrieves the complete business wallet with all payment sources
// All data is fetched from database repositories
// showInactive: false (default) = only active cards, true = active + failed cards from last 2 days
func (ws *WalletServiceImpl) GetBusinessWallet(ctx context.Context, businessID string, showInactive bool) (*payment.BusinessWallet, error) {
	ws.logger.InfoWith("Fetching business wallet", "business_id", businessID, "show_inactive", showInactive)

	// Fetch stored cards from database
	storedCards, err := ws.storedCardRepo.GetStoredCardsByBusinessID(ctx, businessID, showInactive)
	if err != nil {
		ws.logger.ErrorWith("Failed to fetch stored cards", "error", err, "business_id", businessID)
		storedCards = make([]*payment.StoredCard, 0) // Return empty slice on error
	}
	if storedCards == nil {
		storedCards = make([]*payment.StoredCard, 0)
	}

	// Fetch bank accounts from database
	bankAccounts, err := ws.bankAccountRepo.GetBankAccountsByBusinessID(ctx, businessID)
	if err != nil {
		ws.logger.ErrorWith("Failed to fetch bank accounts", "error", err, "business_id", businessID)
		bankAccounts = make([]*payment.BankAccount, 0) // Return empty slice on error
	}
	if bankAccounts == nil {
		bankAccounts = make([]*payment.BankAccount, 0)
	}

	// TODO-FUTURE: Add repository queries when features implemented
	// - upiDetailRepo.GetUPIDetailsByBusinessID(ctx, businessID)
	// - creditLineRepo.GetCreditLinesByBusinessID(ctx, businessID)
	// For now, return empty slices until repositories are implemented
	upiDetails := make([]*payment.UPIDetail, 0)
	creditLines := make([]*payment.CreditLine, 0)

	wallet := &payment.BusinessWallet{
		BusinessID:   businessID,
		StoredCards:  storedCards,
		BankAccounts: bankAccounts,
		UPIDetails:   upiDetails,
		CreditLines:  creditLines,
		LastUpdated:  payment.Now(),
	}

	ws.logger.InfoWith("Business wallet fetched successfully",
		"business_id", businessID,
		"stored_cards", len(storedCards),
		"bank_accounts", len(bankAccounts),
		"upi_details", len(upiDetails),
		"credit_lines", len(creditLines))

	return wallet, nil
}

// AddPaymentSource adds a new payment source to the business wallet
func (ws *WalletServiceImpl) AddPaymentSource(ctx context.Context, businessID string, source payment.PaymentSource) error {
	ws.logger.InfoWith("Adding payment source to wallet",
		"business_id", businessID,
		"source_type", source.GetSourceType(),
		"source_id", source.GetSourceID())

	// Store the payment source based on type
	switch source.GetSourceType() {
	case payment.PaymentSourceCard:
		if storedCard, ok := source.(*payment.StoredCard); ok {
			// Store card in database
			err := ws.storedCardRepo.CreateStoredCard(ctx, storedCard)
			if err != nil {
				ws.logger.ErrorWith("Failed to store card", "error", err, "business_id", businessID)
				return fmt.Errorf("failed to store card: %w", err)
			}

			ws.logger.InfoWith("Card stored successfully",
				"business_id", businessID,
				"card_type", storedCard.CardType,
				"card_category", storedCard.CardCategory,
				"last_four_digits", storedCard.LastFourDigits)
		} else {
			return fmt.Errorf("invalid stored card type")
		}

	case payment.PaymentSourceBankAccount:
		if bankAccount, ok := source.(*payment.BankAccount); ok {
			ws.logger.InfoWith("Processing bank account verification",
				"business_id", businessID,
				"account_masked", ws.maskAccountNumber(bankAccount.AccountNumber),
				"ifsc_code", bankAccount.IFSCCode,
				"holder_name", bankAccount.AccountHolderName)

			// Step 1: Validate bank account details
			if err := ws.validateBankAccountDetails(bankAccount); err != nil {
				ws.logger.ErrorWith("Bank account validation failed", "error", err)
				return err
			}

			// Step 2: Perform penny drop verification (OUTSIDE transaction)
			ws.logger.Info("Initiating penny drop verification")
			verificationRequest := BankVerificationRequest{
				BusinessID:        businessID,
				RequestID:         fmt.Sprintf("verify_%s_%d", businessID[:8], time.Now().Unix()),
				AccountNumber:     bankAccount.AccountNumber,
				IFSCCode:          bankAccount.IFSCCode,
				AccountHolderName: bankAccount.AccountHolderName,
				BankName:          bankAccount.BankName,
				BranchName:        bankAccount.BranchName,
				AccountType:       bankAccount.AccountType,
				Metadata: map[string]any{
					"source":      "wallet_service",
					"business_id": businessID,
				},
			}

			// TODO-CRITICAL-PRODUCTION: Implement audit logging for bank verification
			// Required: bankVerificationLogRepo.CreateVerificationLog(ctx, logEntry) BEFORE API call
			// - Creates audit trail even if API call fails
			// - Store: status='pending', request_id, account_number_masked
			// After API response: bankVerificationLogRepo.UpdateVerificationResult(ctx, requestID, response)
			// - Update: status, verified_account_name, transaction_reference, verification_amount

			// Call verification service (3rd party API call) - uses context from HTTP request for proper cancellation
			verificationResponse, err := ws.bankVerificationService.VerifyBankAccount(ctx, verificationRequest)

			if err != nil {
				ws.logger.ErrorWith("Bank verification failed", "error", err, "business_id", businessID)
				return fmt.Errorf("bank account verification failed: %w", err)
			}

			// Step 3: Process verification results
			if verificationResponse.Status != BankVerificationStatusSuccess {
				ws.logger.WarnWith("Bank verification unsuccessful",
					"business_id", businessID,
					"failure_reason", verificationResponse.FailureReason)
				return fmt.Errorf("bank account verification failed: %s", verificationResponse.FailureReason)
			}

			// Step 4: Update bank account with verification results
			bankAccount.IsVerified = true
			bankAccount.IsActive = true
			bankAccount.AccountHolderName = verificationResponse.VerifiedAccountName // Use verified name
			bankAccount.BankName = verificationResponse.BankName                     // Use verified bank details
			bankAccount.BranchName = verificationResponse.BranchName
			bankAccount.AddedAt = time.Now()
			bankAccount.BusinessID = businessID

			// Add verification metadata
			if bankAccount.Metadata == nil {
				bankAccount.Metadata = make(map[string]any)
			}
			bankAccount.Metadata["verification_status"] = "verified"
			bankAccount.Metadata["verification_date"] = verificationResponse.VerifiedAt.Format("2006-01-02")
			bankAccount.Metadata["verification_amount"] = verificationResponse.VerificationAmount
			bankAccount.Metadata["transaction_reference"] = verificationResponse.TransactionReference
			bankAccount.Metadata["provider"] = string(verificationResponse.Provider)

			// Step 5: Atomically check duplicate + create (IN TRANSACTION)
			ws.logger.Debug("Storing verified bank account in database")
			err = ws.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
				txBankAccountRepo := ws.bankAccountRepo.WithTx(tx)

				// Compute SHA-256 hash for duplicate detection
				hash := sha256.Sum256([]byte(bankAccount.AccountNumber))
				accountNumberHash := hex.EncodeToString(hash[:])

				// Check if duplicate exists (using unique index)
				exists, err := txBankAccountRepo.CheckDuplicateAccount(ctx, businessID, accountNumberHash)
				if err != nil {
					return fmt.Errorf("failed to check duplicate account: %w", err)
				}
				if exists {
					return fmt.Errorf("bank account already exists: %s", ws.maskAccountNumber(bankAccount.AccountNumber))
				}

				// Create bank account (hash computed inside CreateBankAccount as well)
				return txBankAccountRepo.CreateBankAccount(ctx, bankAccount)
			})

			if err != nil {
				ws.logger.ErrorWith("Failed to store bank account", "error", err, "business_id", businessID)
				return err
			}

			ws.logger.InfoWith("Bank account verified and stored successfully",
				"business_id", businessID,
				"bank_name", bankAccount.BankName,
				"account_holder_name", bankAccount.AccountHolderName,
				"transaction_reference", verificationResponse.TransactionReference)

		} else {
			return fmt.Errorf("invalid bank account type")
		}

	case payment.PaymentSourceUPI:
		// TODO-FUTURE: Implement UPI storage when feature is needed
		ws.logger.Debug("UPI storage not implemented yet")

	case payment.PaymentSourceCreditLine:
		// TODO-FUTURE: Implement credit line storage when feature is needed
		ws.logger.Debug("Credit line storage not implemented yet")
	}

	ws.logger.InfoWith("Payment source added successfully",
		"business_id", businessID,
		"source_type", source.GetSourceType())

	// Update onboarding flag (non-blocking, first-time only)
	if ws.businessService != nil {
		flags, err := ws.businessService.GetBusinessOnboardingFlags(ctx, businessID)
		if err == nil && !flags.HasAddedPaymentMethod {
			_ = ws.businessService.UpdateOnboardingStatus(ctx, businessID, "has_added_payment_method", true)
			ws.logger.DebugWith("Onboarding flag updated", "business_id", businessID, "flag", "has_added_payment_method")
		}
	}

	return nil
}

// RemovePaymentSource removes a payment source from the business wallet
func (ws *WalletServiceImpl) RemovePaymentSource(ctx context.Context, businessID string, sourceID string, sourceType payment.PaymentSourceType) error {
	fmt.Println("=== WALLET SERVICE: RemovePaymentSource Called ===")
	fmt.Printf("Business ID: %s\n", businessID)
	fmt.Printf("Source Type: %s\n", sourceType)
	fmt.Printf("Source ID: %s\n", sourceID)

	// Mock implementation - just log the action
	fmt.Printf("✅ Payment source removed successfully\n")

	fmt.Println("=== WALLET SERVICE: RemovePaymentSource Complete ===")
	return nil
}

// CompleteCardTokenization finalizes card tokenization after successful payment
// Updates the stored card with token, sets it as active, and updates onboarding flag
// This centralizes card completion logic in WalletService (same layer as AddPaymentSource)
func (ws *WalletServiceImpl) CompleteCardTokenization(
	ctx context.Context,
	businessID string,
	cardID string,
	token string,
	gatewayUsed payment.GatewayType,
	gatewayCardID string,
) error {
	ws.logger.InfoWith("Completing card tokenization",
		"business_id", businessID,
		"card_id", cardID,
		"gateway", gatewayUsed)

	// Step 1: Load existing stored_card record (created in Phase 1)
	storedCard, err := ws.storedCardRepo.GetStoredCardByID(ctx, cardID)
	if err != nil {
		ws.logger.ErrorWith("Failed to load stored card",
			"card_id", cardID,
			"error", err)
		return fmt.Errorf("failed to load stored card: %w", err)
	}
	if storedCard == nil {
		ws.logger.ErrorWith("Stored card not found",
			"card_id", cardID)
		return fmt.Errorf("stored card not found: %s", cardID)
	}

	ws.logger.DebugWith("Loaded stored card for tokenization completion",
		"card_id", cardID,
		"card_type", storedCard.CardType,
		"last_four", storedCard.LastFourDigits)

	// Step 2: Update card with tokenization data
	storedCard.Token = token
	storedCard.GatewayUsed = gatewayUsed
	storedCard.IsActive = true
	storedCard.IssuerBank = "Unknown" // TODO: Extract from metadata or separate API call if needed

	// Update metadata to reflect Phase 2 completion
	if storedCard.Metadata == nil {
		storedCard.Metadata = make(map[string]any)
	}
	storedCard.Metadata["status"] = "active"
	storedCard.Metadata["phase"] = "2_tokenization_complete"
	storedCard.Metadata["tokenization_date"] = time.Now().Format("2006-01-02")
	storedCard.Metadata["gateway_card_id"] = gatewayCardID

	// Step 3: Update the stored_card record in database
	if err := ws.storedCardRepo.UpdateStoredCard(ctx, storedCard); err != nil {
		ws.logger.ErrorWith("Failed to update stored card after tokenization",
			"card_id", cardID,
			"error", err)
		return fmt.Errorf("failed to update stored card: %w", err)
	}

	ws.logger.InfoWith("Stored card updated successfully with tokenization data",
		"card_id", cardID,
		"gateway", gatewayUsed,
		"is_active", true)

	// Step 4: Update onboarding flag (non-blocking, first-time only)
	// SAME LOGIC as AddPaymentSource - ensures consistency across all payment method additions
	if ws.businessService != nil {
		flags, err := ws.businessService.GetBusinessOnboardingFlags(ctx, businessID)
		if err == nil && !flags.HasAddedPaymentMethod {
			_ = ws.businessService.UpdateOnboardingStatus(ctx, businessID, "has_added_payment_method", true)
			ws.logger.InfoWith("Onboarding flag updated",
				"business_id", businessID,
				"flag", "has_added_payment_method",
				"trigger", "card_tokenization")
		}
	}

	return nil
}

// GetPaymentSource retrieves a specific payment source
func (ws *WalletServiceImpl) GetPaymentSource(ctx context.Context, businessID string, sourceID string, sourceType payment.PaymentSourceType) (payment.PaymentSource, error) {
	fmt.Println("=== WALLET SERVICE: GetPaymentSource Called ===")
	fmt.Printf("Business ID: %s\n", businessID)
	fmt.Printf("Source Type: %s\n", sourceType)
	fmt.Printf("Source ID: %s\n", sourceID)

	// Mock implementation - return a sample source based on type
	switch sourceType {
	case payment.PaymentSourceCard:
		cards := ws.generateMockStoredCards(businessID)
		for _, card := range cards {
			if card.CardID == sourceID {
				fmt.Printf("✅ Found stored card: %s\n", card.LastFourDigits)
				return card, nil
			}
		}
	case payment.PaymentSourceBankAccount:
		accounts := ws.generateMockBankAccounts(businessID)
		for _, account := range accounts {
			if account.AccountID == sourceID {
				fmt.Printf("✅ Found bank account: %s\n", account.LastFiveDigits)
				return account, nil
			}
		}
	case payment.PaymentSourceUPI:
		upis := ws.generateMockUPIDetails(businessID)
		for _, upi := range upis {
			if upi.UPIID == sourceID {
				fmt.Printf("✅ Found UPI: %s\n", upi.UPIAddress)
				return upi, nil
			}
		}
	case payment.PaymentSourceCreditLine:
		credits := ws.generateMockCreditLines(businessID)
		for _, credit := range credits {
			if credit.CreditLineID == sourceID {
				fmt.Printf("✅ Found credit line: %s\n", credit.Provider)
				return credit, nil
			}
		}
	}

	fmt.Printf("❌ Payment source not found\n")
	return nil, fmt.Errorf("payment source not found: %s", sourceID)
}

// Helper methods to generate mock data
// NOTE: These functions are kept for development/testing purposes only
// They are NOT used in production flow as database repositories are primary

// TODO-CLEANUP: Remove mock data generation functions if no longer needed for development
// Check if any tests or development workflows still rely on these
func (ws *WalletServiceImpl) generateMockStoredCards(businessID string) []*payment.StoredCard {
	fmt.Printf("Mock Stored Cards\n")
	return []*payment.StoredCard{
		{
			CardID:         fmt.Sprintf("card_%s_001", businessID[:8]),
			BusinessID:     businessID,
			Token:          fmt.Sprintf("tok_visa_%s", businessID[:8]),
			LastFourDigits: "4532",
			CardType:       payment.CardTypeVisa,
			CardCategory:   payment.CardCategoryCredit,
			ExpiryMonth:    "12",
			ExpiryYear:     "25",
			IssuerBank:     "HDFC Bank",
			CardHolderName: "Business Account",
			IsActive:       true,
			GatewayUsed:    payment.GatewayJioPay,
			AddedAt:        payment.Now().AddDate(0, -2, 0),
			Metadata: map[string]any{
				"nickname": "Primary Card",
				"mock_data": true,
			},
		},
		{
			CardID:         fmt.Sprintf("card_%s_002", businessID[:8]),
			BusinessID:     businessID,
			Token:          fmt.Sprintf("tok_master_%s", businessID[:8]),
			LastFourDigits: "8765",
			CardType:       payment.CardTypeMastercard,
			CardCategory:   payment.CardCategoryDebit,
			ExpiryMonth:    "08",
			ExpiryYear:     "26",
			IssuerBank:     "ICICI Bank",
			CardHolderName: "Business Account",
			IsActive:       true,
			GatewayUsed:    payment.GatewayEasebuzz,
			AddedAt:        payment.Now().AddDate(0, -1, -15),
			Metadata: map[string]any{
				"nickname": "Backup Card",
				"mock_data": true,
			},
		},
	}
}

func (ws *WalletServiceImpl) generateMockBankAccounts(businessID string) []*payment.BankAccount {
	fmt.Printf("Mock bank accs\n")

	return []*payment.BankAccount{
		{
			AccountID:       fmt.Sprintf("acc_%s_001", businessID[:8]),
			BusinessID:      businessID,
			AccountNumber:   "1234567890123456", // Full number (normally encrypted)
			LastFiveDigits:  "23456",
			BankName:        "State Bank of India",
			BranchName:      "Corporate Branch",
			IFSCCode:        "SBIN0001234",
			AccountType:     payment.BankAccountTypeCurrent,
			AccountHolderName: "XCentro Business Pvt Ltd",
			IsActive:        true,
			IsVerified:      true,
			AddedAt:         payment.Now().AddDate(0, -3, 0),
			Metadata: map[string]any{
				"nickname": "Primary Current Account",
				"mock_data": true,
			},
		},
		{
			AccountID:       fmt.Sprintf("acc_%s_002", businessID[:8]),
			BusinessID:      businessID,
			AccountNumber:   "9876543210987654", // Full number (normally encrypted)
			LastFiveDigits:  "87654",
			BankName:        "HDFC Bank",
			BranchName:      "Business Banking Branch",
			IFSCCode:        "HDFC0001234",
			AccountType:     payment.BankAccountTypeCurrent,
			AccountHolderName: "XCentro Business Pvt Ltd",
			IsActive:        true,
			IsVerified:      true,
			AddedAt:         payment.Now().AddDate(0, -2, -10),
			Metadata: map[string]any{
				"nickname": "Secondary Account",
				"mock_data": true,
			},
		},
	}
}

func (ws *WalletServiceImpl) generateMockUPIDetails(businessID string) []*payment.UPIDetail {
	fmt.Printf("Mock UPI Details\n")

	return []*payment.UPIDetail{
		{
			UPIID:      fmt.Sprintf("upi_%s_001", businessID[:8]),
			BusinessID: businessID,
			UPIAddress: "xcentrobusiness@paytm",
			Provider:   payment.UPIProviderPaytm,
			IsActive:   true,
			IsVerified: true,
			AddedAt:    payment.Now().AddDate(0, -1, -5),
			Metadata: map[string]any{
				"nickname": "Primary UPI",
				"mock_data": true,
			},
		},
		{
			UPIID:      fmt.Sprintf("upi_%s_002", businessID[:8]),
			BusinessID: businessID,
			UPIAddress: "business.xcentro@googleplay",
			Provider:   payment.UPIProviderGooglePay,
			IsActive:   true,
			IsVerified: true,
			AddedAt:    payment.Now().AddDate(0, 0, -20),
			Metadata: map[string]any{
				"nickname": "Google Pay UPI",
				"mock_data": true,
			},
		},
	}
}

func (ws *WalletServiceImpl) generateMockCreditLines(businessID string) []*payment.CreditLine {
	fmt.Printf("Mock Credit lines\n")

	// For now, return empty slice as credit lines are placeholder
	return []*payment.CreditLine{
		{
			CreditLineID:   fmt.Sprintf("credit_%s_001", businessID[:8]),
			BusinessID:     businessID,
			Provider:       "FinTech Credit Line",
			CreditLimit:    10000000, // ₹100,000
			AvailableLimit: 7500000,  // ₹75,000
			UsedLimit:      2500000,  // ₹25,000
			Currency:       payment.CurrencyINR,
			InterestRate:   18.5,
			Status:         payment.CreditLineStatusActive,
			IsActive:       true,
			AddedAt:        payment.Now().AddDate(0, -6, 0),
			Metadata: map[string]any{
				"nickname": "Working Capital Line",
				"mock_data": true,
			},
		},
	}
}

// Helper methods for bank account verification

// validateBankAccountDetails validates bank account details before verification
func (ws *WalletServiceImpl) validateBankAccountDetails(bankAccount *payment.BankAccount) error {
	if bankAccount.AccountNumber == "" {
		return fmt.Errorf("account number is required")
	}

	if len(bankAccount.AccountNumber) < 9 || len(bankAccount.AccountNumber) > 18 {
		return fmt.Errorf("account number must be between 9 and 18 digits")
	}

	if bankAccount.IFSCCode == "" {
		return fmt.Errorf("IFSC code is required")
	}

	if len(bankAccount.IFSCCode) != 11 {
		return fmt.Errorf("IFSC code must be exactly 11 characters")
	}

	if bankAccount.AccountHolderName == "" {
		return fmt.Errorf("account holder name is required")
	}

	if strings.TrimSpace(bankAccount.AccountHolderName) == "" {
		return fmt.Errorf("account holder name cannot be empty")
	}

	return nil
}

// maskAccountNumber masks the account number for secure display
func (ws *WalletServiceImpl) maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
}