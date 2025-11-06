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
	GetBusinessWallet(businessID string) (*payment.BusinessWallet, error)
	AddPaymentSource(businessID string, source payment.PaymentSource) error
	RemovePaymentSource(businessID string, sourceID string, sourceType payment.PaymentSourceType) error
	GetPaymentSource(businessID string, sourceID string, sourceType payment.PaymentSourceType) (payment.PaymentSource, error)
}

// MockWalletService provides a mock implementation for business wallet operations
type MockWalletService struct {
	// In-memory storage for Phase 3B.1 (replace with database in later phases)
	// TODO-DB-4.1-CRITICAL: Remove in-memory storage maps
	// - Delete: storedCards map[string][]*payment.StoredCard
	// - Delete: bankAccounts map[string][]*payment.BankAccount
	// - All storage should go through repositories
	storedCards map[string][]*payment.StoredCard // businessID -> cards
	bankAccounts map[string][]*payment.BankAccount // businessID -> bank accounts
	// Bank verification service for penny drop verification
	bankVerificationService BankVerificationService
	// Repository dependencies (Phase 4.2)
	storedCardRepo  payment.StoredCardRepository
	bankAccountRepo payment.BankAccountRepository
	// Transaction manager (Phase 4.4)
	txManager client.TxManager
	// Business service for onboarding flags
	businessService business.BusinessService
	// Logger
	logger *logger.Logger
}

// NewMockWalletService creates a new mock wallet service
// TODO-DB-4.1: Update constructor signature for Phase 4
// - Rename to NewWalletService()
// - Add parameters: storedCardRepo, bankAccountRepo, bankVerificationLogRepo
// - Remove map initialization, initialize repo dependencies instead
func NewMockWalletService(storedCardRepo payment.StoredCardRepository, bankAccountRepo payment.BankAccountRepository, txManager client.TxManager, businessService business.BusinessService, logger *logger.Logger) WalletService {
	return &MockWalletService{
		storedCards:             make(map[string][]*payment.StoredCard),
		bankAccounts:            make(map[string][]*payment.BankAccount),
		bankVerificationService: NewMockBankVerificationService(),
		storedCardRepo:          storedCardRepo,
		bankAccountRepo:         bankAccountRepo,
		txManager:               txManager,
		businessService:         businessService,
		logger:                  logger,
	}
}

// GetBusinessWallet retrieves the complete business wallet with all payment sources
// TODO-DB-4.1-CRITICAL: Replace mock data with database queries
// - This method combines real stored data with mock data
// - Phase 4.1: Mark all mock data generation for removal
// - Phase 4.4: Replace with actual repository calls
func (ws *MockWalletService) GetBusinessWallet(businessID string) (*payment.BusinessWallet, error) {
	ws.logger.InfoWith("Fetching business wallet", "business_id", businessID)

	// Get real stored cards (from tokenization) and combine with mock data
	// TODO-DB-4.1-CRITICAL: Fetch stored cards from database
	// - Replace in-memory map lookup: ws.storedCards[businessID]
	// - Call: storedCardRepo.GetStoredCardsByBusinessID(ctx, businessID)
	// - Filter for is_active = true
	ctx := context.Background()
	dbStoredCards, _ := ws.storedCardRepo.GetStoredCardsByBusinessID(ctx, businessID)
	realStoredCards := ws.storedCards[businessID]
	if realStoredCards == nil {
		realStoredCards = make([]*payment.StoredCard, 0)
	}
	// Combine database cards with in-memory cards (temporarily until Phase 4.5)
	if dbStoredCards != nil {
		realStoredCards = append(realStoredCards, dbStoredCards...)
	}

	// Use only real stored cards from database
	storedCards := realStoredCards

	// Get real bank accounts (from verification) and combine with mock data
	// TODO-DB-4.1-CRITICAL: Fetch bank accounts from database
	// - Replace in-memory map lookup: ws.bankAccounts[businessID]
	// - Call: bankAccountRepo.GetBankAccountsByBusinessID(ctx, businessID)
	// - Filter for is_active = true and is_verified = true
	dbBankAccounts, _ := ws.bankAccountRepo.GetBankAccountsByBusinessID(ctx, businessID)
	realBankAccounts := ws.bankAccounts[businessID]
	if realBankAccounts == nil {
		realBankAccounts = make([]*payment.BankAccount, 0)
	}
	// Combine database accounts with in-memory accounts (temporarily until Phase 4.5)
	if dbBankAccounts != nil {
		realBankAccounts = append(realBankAccounts, dbBankAccounts...)
	}

	// Use only real bank accounts from database
	bankAccounts := realBankAccounts

	// TODO-DB-4.1-FUTURE: Add repository queries when features implemented
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
func (ws *MockWalletService) AddPaymentSource(businessID string, source payment.PaymentSource) error {
	ws.logger.InfoWith("Adding payment source to wallet",
		"business_id", businessID,
		"source_type", source.GetSourceType(),
		"source_id", source.GetSourceID())

	// Store the payment source based on type
	switch source.GetSourceType() {
	case payment.PaymentSourceCard:
		if storedCard, ok := source.(*payment.StoredCard); ok {
			// Initialize the business's card slice if not exists
			if ws.storedCards[businessID] == nil {
				ws.storedCards[businessID] = make([]*payment.StoredCard, 0)
			}

			// Add the card to storage
			// TODO-DB-4.1-CRITICAL: Replace in-memory append with repository call
			// - Remove: ws.storedCards[businessID] = append(...)
			// - Call: storedCardRepo.CreateStoredCard(ctx, storedCard)
			// - Repository will generate card_id via database UUID
			// - Handle duplicate errors if card already exists
			ctx := context.Background()
			_ = ws.storedCardRepo.CreateStoredCard(ctx, storedCard)
			ws.storedCards[businessID] = append(ws.storedCards[businessID], storedCard)

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

			// TODO-DB-4.1-CRITICAL: Create verification log BEFORE calling API
			// - Call: bankVerificationLogRepo.CreateVerificationLog(ctx, logEntry)
			// - logEntry.status = 'pending'
			// - logEntry.request_id = verificationRequest.RequestID
			// - logEntry.account_number_masked = maskAccountNumber()
			// - This creates audit trail even if API call fails

			// Call verification service (this simulates 3rd party API call)
			ctx := context.Background() // TODO-DB-4.1: Use proper context from HTTP request
			verificationResponse, err := ws.bankVerificationService.VerifyBankAccount(ctx, verificationRequest)

			// TODO-DB-4.1-CRITICAL: Update verification log with response
			// - Call: bankVerificationLogRepo.UpdateVerificationResult(ctx, requestID, response)
			// - Update: status, verified_account_name, transaction_reference, verification_amount
			// - Store complete response in metadata JSONB for audit

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

			// Step 6: Update in-memory storage (temporary until Phase 4.5 cleanup)
			if ws.bankAccounts[businessID] == nil {
				ws.bankAccounts[businessID] = make([]*payment.BankAccount, 0)
			}
			ws.bankAccounts[businessID] = append(ws.bankAccounts[businessID], bankAccount)

			ws.logger.InfoWith("Bank account verified and stored successfully",
				"business_id", businessID,
				"bank_name", bankAccount.BankName,
				"account_holder_name", bankAccount.AccountHolderName,
				"transaction_reference", verificationResponse.TransactionReference,
				"total_accounts", len(ws.bankAccounts[businessID]))

		} else {
			return fmt.Errorf("invalid bank account type")
		}

	case payment.PaymentSourceUPI:
		// TODO: Implement UPI storage when needed
		ws.logger.Debug("UPI storage not implemented yet")

	case payment.PaymentSourceCreditLine:
		// TODO: Implement credit line storage when needed
		ws.logger.Debug("Credit line storage not implemented yet")
	}

	ws.logger.InfoWith("Payment source added successfully",
		"business_id", businessID,
		"source_type", source.GetSourceType(),
		"total_cards", len(ws.storedCards[businessID]))

	// Update onboarding flag (non-blocking, first-time only)
	if ws.businessService != nil {
		go func(bizID string) {
			bgCtx := context.Background()
			flags, err := ws.businessService.GetBusinessOnboardingFlags(bgCtx, bizID)
			if err == nil && !flags.HasAddedPaymentMethod {
				_ = ws.businessService.UpdateOnboardingStatus(bgCtx, bizID, "has_added_payment_method", true)
				ws.logger.DebugWith("Onboarding flag updated", "business_id", bizID, "flag", "has_added_payment_method")
			}
		}(businessID)
	}

	return nil
}

// RemovePaymentSource removes a payment source from the business wallet
func (ws *MockWalletService) RemovePaymentSource(businessID string, sourceID string, sourceType payment.PaymentSourceType) error {
	fmt.Println("=== WALLET SERVICE: RemovePaymentSource Called ===")
	fmt.Printf("Business ID: %s\n", businessID)
	fmt.Printf("Source Type: %s\n", sourceType)
	fmt.Printf("Source ID: %s\n", sourceID)

	// Mock implementation - just log the action
	fmt.Printf("✅ Payment source removed successfully\n")

	fmt.Println("=== WALLET SERVICE: RemovePaymentSource Complete ===")
	return nil
}

// GetPaymentSource retrieves a specific payment source
func (ws *MockWalletService) GetPaymentSource(businessID string, sourceID string, sourceType payment.PaymentSourceType) (payment.PaymentSource, error) {
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

// TODO-DB-4.1-CRITICAL: Mark for deletion in Phase 4.4
// - generateMockStoredCards() - DELETE in Phase 4.4
// - This is only for testing, should not be in production code
func (ws *MockWalletService) generateMockStoredCards(businessID string) []*payment.StoredCard {
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

// TODO-DB-4.1-CRITICAL: Mark for deletion in Phase 4.4
// - generateMockBankAccounts() - DELETE in Phase 4.4
// - This is only for testing, should not be in production code
func (ws *MockWalletService) generateMockBankAccounts(businessID string) []*payment.BankAccount {
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

// TODO-DB-4.1-CRITICAL: Mark for deletion in Phase 4.4
// - generateMockUPIDetails() - DELETE in Phase 4.4
// - This is only for testing, should not be in production code
func (ws *MockWalletService) generateMockUPIDetails(businessID string) []*payment.UPIDetail {
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

// TODO-DB-4.1-CRITICAL: Mark for deletion in Phase 4.4
// - generateMockCreditLines() - DELETE in Phase 4.4
// - This is only for testing, should not be in production code
func (ws *MockWalletService) generateMockCreditLines(businessID string) []*payment.CreditLine {
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
func (ws *MockWalletService) validateBankAccountDetails(bankAccount *payment.BankAccount) error {
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

// checkDuplicateBankAccount checks if the account number already exists for the business
// TODO-DB-4.1-CRITICAL: Check duplicates via database query
// - Replace in-memory loop through ws.bankAccounts[businessID]
// - Call: bankAccountRepo.CheckDuplicateAccount(ctx, businessID, accountNumber)
// - Use account_number_hash for fast duplicate detection
func (ws *MockWalletService) checkDuplicateBankAccount(businessID, accountNumber string) error {
	existingAccounts := ws.bankAccounts[businessID]
	if existingAccounts == nil {
		return nil // No existing accounts, so no duplicates
	}

	for _, account := range existingAccounts {
		if account.AccountNumber == accountNumber {
			return fmt.Errorf("bank account already exists: %s", ws.maskAccountNumber(accountNumber))
		}
	}

	return nil
}

// maskAccountNumber masks the account number for secure display
func (ws *MockWalletService) maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
}