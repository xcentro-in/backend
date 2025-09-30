package business_rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MockBankVerificationService provides a mock implementation for bank account verification
// Simulates 3rd party API calls for penny drop verification during development
type MockBankVerificationService struct {
	// Configuration would go here in real implementation
	apiDelay time.Duration // Simulate network delay
}

// NewMockBankVerificationService creates a new mock bank verification service
func NewMockBankVerificationService() BankVerificationService {
	return &MockBankVerificationService{
		apiDelay: 2 * time.Second, // Simulate realistic API response time
	}
}

// GetProviderType returns the verification service provider identifier
func (s *MockBankVerificationService) GetProviderType() BankVerificationProvider {
	return BankVerificationProviderMock
}

// VerifyBankAccount simulates penny drop verification for a bank account
func (s *MockBankVerificationService) VerifyBankAccount(ctx context.Context, request BankVerificationRequest) (*BankVerificationResponse, error) {
	fmt.Println("=== BANK VERIFICATION: VerifyBankAccount Called ===")
	fmt.Printf("📥 Request ID: %s\n", request.RequestID)
	fmt.Printf("🏦 Bank Account: %s\n", s.maskAccountNumber(request.AccountNumber))
	fmt.Printf("🏧 IFSC Code: %s\n", request.IFSCCode)
	fmt.Printf("👤 Account Holder: %s\n", request.AccountHolderName)
	fmt.Printf("🏪 Bank: %s\n", request.BankName)

	// Validate IFSC code format
	if err := s.validateIFSCCode(request.IFSCCode); err != nil {
		fmt.Printf("❌ IFSC validation failed: %v\n", err)
		return nil, err
	}

	// Validate account number
	if err := s.validateAccountNumber(request.AccountNumber); err != nil {
		fmt.Printf("❌ Account number validation failed: %v\n", err)
		return nil, err
	}

	fmt.Println("🔄 Initiating penny drop verification...")
	fmt.Printf("💰 Transferring ₹1 to account for verification...\n")

	// Simulate API call delay
	time.Sleep(s.apiDelay)

	// Simulate verification based on account patterns
	return s.simulateVerificationResponse(request)
}

// simulateVerificationResponse creates mock responses based on account patterns
func (s *MockBankVerificationService) simulateVerificationResponse(request BankVerificationRequest) (*BankVerificationResponse, error) {
	// Use account number patterns to simulate different scenarios
	lastFourDigits := request.AccountNumber[len(request.AccountNumber)-4:]

	switch {
	case strings.HasSuffix(lastFourDigits, "0000"):
		// Simulate account closed
		fmt.Printf("❌ Account closed: %s\n", s.maskAccountNumber(request.AccountNumber))
		return nil, NewBankVerificationError(
			ErrCodeAccountClosed,
			"Bank account is closed",
			"The provided account number is no longer active",
		)

	case strings.HasSuffix(lastFourDigits, "1111"):
		// Simulate invalid account
		fmt.Printf("❌ Invalid account: %s\n", s.maskAccountNumber(request.AccountNumber))
		return nil, NewBankVerificationError(
			ErrCodeInvalidAccountNumber,
			"Invalid account number",
			"The account number does not exist or is invalid",
		)

	case strings.HasSuffix(lastFourDigits, "2222"):
		// Simulate name mismatch
		fmt.Printf("❌ Name mismatch: provided=%s, bank=%s\n", request.AccountHolderName, "Different Name")
		return nil, NewBankVerificationError(
			ErrCodeNameMismatch,
			"Account holder name mismatch",
			"The provided name does not match bank records",
		)

	case strings.HasSuffix(lastFourDigits, "9999"):
		// Simulate API timeout
		fmt.Printf("⏰ API timeout simulation\n")
		return nil, NewBankVerificationError(
			ErrCodeAPITimeout,
			"Bank verification API timeout",
			"The verification request timed out",
		)

	default:
		// Simulate successful verification
		return s.createSuccessfulResponse(request)
	}
}

// createSuccessfulResponse creates a successful verification response
func (s *MockBankVerificationService) createSuccessfulResponse(request BankVerificationRequest) (*BankVerificationResponse, error) {
	// Generate verification details
	verificationAmount := int64(100) // ₹1 in paise
	transactionReference := fmt.Sprintf("UTR%d%s", time.Now().Unix(), uuid.New().String()[:6])

	// For mock, we'll slightly modify the name to simulate bank response
	verifiedName := s.generateVerifiedName(request.AccountHolderName)

	response := &BankVerificationResponse{
		RequestID:            request.RequestID,
		Status:               BankVerificationStatusSuccess,
		VerifiedAccountName:  verifiedName,
		BankName:             s.getBankNameFromIFSC(request.IFSCCode),
		BranchName:           s.getBranchNameFromIFSC(request.IFSCCode),
		IFSCCode:             request.IFSCCode,
		AccountNumber:        s.maskAccountNumber(request.AccountNumber),
		AccountType:          request.AccountType,
		IsAccountActive:      true,
		VerificationAmount:   verificationAmount,
		TransactionReference: transactionReference,
		VerifiedAt:           time.Now(),
		Provider:             BankVerificationProviderMock,
		Metadata: map[string]any{
			"verification_method": "penny_drop",
			"api_response_time":   s.apiDelay.String(),
			"mock_data":          true,
		},
	}

	fmt.Printf("✅ Verification successful!\n")
	fmt.Printf("💳 Verified Name: %s\n", verifiedName)
	fmt.Printf("🏦 Bank: %s\n", response.BankName)
	fmt.Printf("🔗 UTR: %s\n", transactionReference)
	fmt.Printf("💰 Amount: ₹%.2f\n", float64(verificationAmount)/100)
	fmt.Println("=== BANK VERIFICATION: VerifyBankAccount Complete ===")

	return response, nil
}

// Helper methods for validation and simulation

func (s *MockBankVerificationService) validateIFSCCode(ifsc string) error {
	if len(ifsc) != 11 {
		return NewBankVerificationError(
			ErrCodeInvalidIFSC,
			"Invalid IFSC code length",
			"IFSC code must be exactly 11 characters",
		)
	}

	// Basic IFSC format validation (4 letters + 0 + 6 alphanumeric)
	if !strings.HasSuffix(ifsc[4:5], "0") {
		return NewBankVerificationError(
			ErrCodeInvalidIFSC,
			"Invalid IFSC code format",
			"IFSC code format is invalid",
		)
	}

	return nil
}

func (s *MockBankVerificationService) validateAccountNumber(accountNumber string) error {
	if len(accountNumber) < 9 || len(accountNumber) > 18 {
		return NewBankVerificationError(
			ErrCodeInvalidAccountNumber,
			"Invalid account number length",
			"Account number must be between 9 and 18 digits",
		)
	}

	return nil
}

func (s *MockBankVerificationService) maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
}

func (s *MockBankVerificationService) generateVerifiedName(providedName string) string {
	// Simulate bank response which might have slight variations
	// In real life, bank names might have different formatting

	// For mock, convert to uppercase (common bank response format)
	return strings.ToUpper(providedName)
}

func (s *MockBankVerificationService) getBankNameFromIFSC(ifsc string) string {
	// Mock bank name mapping based on IFSC prefixes
	bankCode := strings.ToUpper(ifsc[:4])

	bankMap := map[string]string{
		"HDFC": "HDFC Bank Limited",
		"ICIC": "ICICI Bank Limited",
		"SBIN": "State Bank of India",
		"AXIS": "Axis Bank Limited",
		"UTIB": "Axis Bank Limited", // Axis Bank IFSC variant
		"KKBK": "Kotak Mahindra Bank Limited",
		"YESB": "Yes Bank Limited",
		"IDIB": "Indian Bank",
		"PUNB": "Punjab National Bank",
		"UBIN": "Union Bank of India",
	}

	if bankName, exists := bankMap[bankCode]; exists {
		return bankName
	}

	// Default to a generic name for unknown IFSC codes
	return fmt.Sprintf("%s Bank Limited", bankCode)
}

func (s *MockBankVerificationService) getBranchNameFromIFSC(ifsc string) string {
	// Mock branch name generation
	branchCode := ifsc[5:]

	// Generate a realistic branch name
	locations := []string{"Main Branch", "Corporate Branch", "Business Branch", "Commercial Branch"}
	areas := []string{"Mumbai", "Delhi", "Bangalore", "Chennai", "Hyderabad", "Pune", "Kolkata", "Ahmedabad"}

	// Use branch code to deterministically select location and area
	locationIndex := len(branchCode) % len(locations)
	areaIndex := (len(branchCode) * 2) % len(areas)

	return fmt.Sprintf("%s %s", areas[areaIndex], locations[locationIndex])
}