package business_rules

import (
	"fmt"
	"xcentro.in/internal/core/payment"
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
	storedCards map[string][]*payment.StoredCard // businessID -> cards
}

// NewMockWalletService creates a new mock wallet service
func NewMockWalletService() WalletService {
	return &MockWalletService{
		storedCards: make(map[string][]*payment.StoredCard),
	}
}

// GetBusinessWallet retrieves the complete business wallet with all payment sources
// TODO: clean the logic here and move to DB in later phases
// For now, returns mock data along with any real stored cards from tokenization
// some part of it is similar to real life
// need to take down mock data and see how it works
func (ws *MockWalletService) GetBusinessWallet(businessID string) (*payment.BusinessWallet, error) {
	fmt.Println("=== WALLET SERVICE: GetBusinessWallet Called ===")
	fmt.Printf("Business ID: %s\n", businessID)

	// Get real stored cards (from tokenization) and combine with mock data
	realStoredCards := ws.storedCards[businessID]
	if realStoredCards == nil {
		realStoredCards = make([]*payment.StoredCard, 0)
	}

	// Generate mock payment sources
	mockStoredCards := ws.generateMockStoredCards(businessID)

	// Combine real and mock cards
	storedCards := append(realStoredCards, mockStoredCards...)

	bankAccounts := ws.generateMockBankAccounts(businessID)
	upiDetails := ws.generateMockUPIDetails(businessID)
	creditLines := ws.generateMockCreditLines(businessID)

	wallet := &payment.BusinessWallet{
		BusinessID:   businessID,
		StoredCards:  storedCards,
		BankAccounts: bankAccounts,
		UPIDetails:   upiDetails,
		CreditLines:  creditLines,
		LastUpdated:  payment.Now(),
	}

	fmt.Printf("💳 Found %d stored cards (%d real + %d mock)\n",
		len(storedCards), len(realStoredCards), len(mockStoredCards))
	fmt.Printf("🏦 Found %d bank accounts\n", len(bankAccounts))
	fmt.Printf("📱 Found %d UPI details\n", len(upiDetails))
	fmt.Printf("💰 Found %d credit lines\n", len(creditLines))

	fmt.Println("=== WALLET SERVICE: GetBusinessWallet Complete ===")
	return wallet, nil
}

// AddPaymentSource adds a new payment source to the business wallet
func (ws *MockWalletService) AddPaymentSource(businessID string, source payment.PaymentSource) error {
	fmt.Println("=== WALLET SERVICE: AddPaymentSource Called ===")
	fmt.Printf("Business ID: %s\n", businessID)
	fmt.Printf("Source Type: %s\n", source.GetSourceType())
	fmt.Printf("Source ID: %s\n", source.GetSourceID())

	// Store the payment source based on type
	switch source.GetSourceType() {
	case payment.PaymentSourceCard:
		if storedCard, ok := source.(*payment.StoredCard); ok {
			// Initialize the business's card slice if not exists
			if ws.storedCards[businessID] == nil {
				ws.storedCards[businessID] = make([]*payment.StoredCard, 0)
			}

			// Add the card to storage
			ws.storedCards[businessID] = append(ws.storedCards[businessID], storedCard)

			fmt.Printf("💾 Stored card: %s %s ending in %s\n",
				storedCard.CardCategory, storedCard.CardType, storedCard.LastFourDigits)
			fmt.Printf("🎫 Token stored securely (backend only)\n")
		} else {
			return fmt.Errorf("invalid stored card type")
		}

	case payment.PaymentSourceBankAccount:
		// TODO: Implement bank account storage when needed
		fmt.Printf("🏦 Bank account storage not implemented yet\n")

	case payment.PaymentSourceUPI:
		// TODO: Implement UPI storage when needed
		fmt.Printf("📱 UPI storage not implemented yet\n")

	case payment.PaymentSourceCreditLine:
		// TODO: Implement credit line storage when needed
		fmt.Printf("💰 Credit line storage not implemented yet\n")
	}

	fmt.Printf("✅ Payment source added successfully\n")
	fmt.Printf("📋 Source Info: %+v\n", source.GetDisplayInfo())
	fmt.Printf("📊 Total stored cards for business: %d\n", len(ws.storedCards[businessID]))

	fmt.Println("=== WALLET SERVICE: AddPaymentSource Complete ===")
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