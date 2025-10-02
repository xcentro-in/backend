package payee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payee"
)

// PayeeService orchestrates the complete payee onboarding and management flow
// Supports polymorphic payee types: Vendor, GST Payment, Biller, Employee
type PayeeService struct {
	// Master routing table
	payeeMasterRepo payee.PayeeMasterRepository

	// Type-specific repositories
	payeeVendorRepo              payee.PayeeVendorRepository
	payeeGSTRepo                 payee.PayeeGSTRepository
	payeeGSTVirtualAccountRepo   payee.PayeeGSTVirtualAccountRepository
	payeeBillerRepo              payee.PayeeBillerRepository
	payeeBillerAccountRepo       payee.PayeeBillerAccountRepository
	payeeEmployeeRepo            payee.PayeeEmployeeRepository

	// Shared repositories
	payeeBankAccountRepo      payee.PayeeBankAccountRepository
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository

	// Verification services
	gstVerificationSvc   business_rules.GSTVerificationService
	bankVerificationSvc  business_rules.BankVerificationService
	crossVerificationSvc business_rules.CrossVerificationService

	// Transaction manager
	txManager client.TxManager
}

// NewPayeeService creates a new payee service with all dependencies
func NewPayeeService(
	payeeMasterRepo payee.PayeeMasterRepository,
	payeeVendorRepo payee.PayeeVendorRepository,
	payeeGSTRepo payee.PayeeGSTRepository,
	payeeGSTVirtualAccountRepo payee.PayeeGSTVirtualAccountRepository,
	payeeBillerRepo payee.PayeeBillerRepository,
	payeeBillerAccountRepo payee.PayeeBillerAccountRepository,
	payeeEmployeeRepo payee.PayeeEmployeeRepository,
	payeeBankAccountRepo payee.PayeeBankAccountRepository,
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository,
	gstVerificationSvc business_rules.GSTVerificationService,
	bankVerificationSvc business_rules.BankVerificationService,
	crossVerificationSvc business_rules.CrossVerificationService,
	txManager client.TxManager,
) *PayeeService {
	return &PayeeService{
		payeeMasterRepo:              payeeMasterRepo,
		payeeVendorRepo:              payeeVendorRepo,
		payeeGSTRepo:                 payeeGSTRepo,
		payeeGSTVirtualAccountRepo:   payeeGSTVirtualAccountRepo,
		payeeBillerRepo:              payeeBillerRepo,
		payeeBillerAccountRepo:       payeeBillerAccountRepo,
		payeeEmployeeRepo:            payeeEmployeeRepo,
		payeeBankAccountRepo:         payeeBankAccountRepo,
		businessPayeeRelationRepo:    businessPayeeRelationRepo,
		gstVerificationSvc:           gstVerificationSvc,
		bankVerificationSvc:          bankVerificationSvc,
		crossVerificationSvc:         crossVerificationSvc,
		txManager:                    txManager,
	}
}

// OnboardPayee orchestrates the complete payee onboarding flow (polymorphic)
// Routes to type-specific onboarding methods based on PayeeType
func (s *PayeeService) OnboardPayee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Printf("=== PAYEE SERVICE: OnboardPayee Called (Type: %s) ===\n", req.PayeeType)

	// Route based on payee type
	switch req.PayeeType {
	case payee.PayeeTypeVendor:
		return s.onboardVendor(ctx, req)
	case payee.PayeeTypeGST:
		return s.onboardGSTPayee(ctx, req)
	case payee.PayeeTypeBiller:
		return nil, fmt.Errorf("Biller onboarding not yet implemented (Phase 4)")
	case payee.PayeeTypeEmployee:
		return nil, fmt.Errorf("Employee onboarding not yet implemented (Phase 5)")
	default:
		return nil, fmt.Errorf("unsupported payee type: %s", req.PayeeType)
	}
}

// onboardVendor handles vendor payee onboarding (GSTIN + bank account)
// Steps: Validate → Check duplicates → Verify GST → Verify Bank → Cross-verify → Money rotation check → Create entities
func (s *PayeeService) onboardVendor(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Println("=== PAYEE SERVICE: onboardVendor Called ===")

	// Validate vendor-specific data
	if req.Vendor == nil {
		return nil, fmt.Errorf("vendor data is required for vendor payee type")
	}

	vendor := req.Vendor
	fmt.Printf("Business ID: %s, GSTIN: %s, Display Name: %s\n", req.BusinessID, vendor.GSTIN, req.DisplayName)
	fmt.Printf("Account Number: ****%s, IFSC: %s\n", vendor.AccountNumber[len(vendor.AccountNumber)-4:], vendor.IFSCCode)

	// Step 1: Validate input
	fmt.Println("✅ Step 1: Validating input...")
	if err := s.validateVendorRequest(&req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Check if vendor payee already exists by GSTIN
	fmt.Println("🔍 Step 2: Checking if vendor payee already exists (by GSTIN)...")
	existingVendor, err := s.payeeVendorRepo.FindByGSTIN(ctx, vendor.GSTIN)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing vendor: %w", err)
	}

	var payeeID string
	var vendorPayee *payee.PayeeVendor

	// Step 3: If vendor doesn't exist, verify GST and create
	if existingVendor == nil {
		fmt.Println("🆕 Step 3: Vendor not found, verifying GST and creating new vendor payee...")

		gstReq := business_rules.GSTVerificationRequest{
			GSTIN:     vendor.GSTIN,
			RequestID: fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix()),
		}

		gstResp, err := s.gstVerificationSvc.VerifyGSTIN(ctx, gstReq)
		if err != nil {
			return nil, fmt.Errorf("GST verification failed: %w", err)
		}

		fmt.Printf("✅ GST verified: %s (%s)\n", gstResp.LegalName, gstResp.Status)

		// Create master entry first
		payeeMaster := &payee.PayeeMaster{
			PayeeType:   payee.PayeeTypeVendor,
			DisplayName: req.DisplayName,
			IsActive:    true,
		}

		if err := s.payeeMasterRepo.Create(ctx, payeeMaster); err != nil {
			return nil, fmt.Errorf("failed to create payee master: %w", err)
		}
		payeeID = payeeMaster.PayeeID

		// Create vendor payee entity
		vendorPayee = &payee.PayeeVendor{
			PayeeID:          payeeID,
			GSTIN:            vendor.GSTIN,
			LegalName:        gstResp.LegalName,
			BusinessType:     gstResp.BusinessType,
			RegistrationDate: &gstResp.RegistrationDate,
			GSTStatus:        gstResp.Status,
			PrimaryAddress: payee.Address{
				BuildingName: gstResp.Address.BuildingName,
				Street:       gstResp.Address.Street,
				City:         gstResp.Address.City,
				State:        gstResp.Address.State,
				Pincode:      gstResp.Address.Pincode,
			},
			GSTVerificationStatus: payee.VerificationStatusSuccess,
			GSTVerifiedAt:         &time.Time{},
			GSTData:               gstResp.Metadata,
			Metadata: map[string]any{
				"directors":  gstResp.Directors,
				"state_code": gstResp.StateCode,
			},
		}
		*vendorPayee.GSTVerifiedAt = time.Now()

		if err := s.payeeVendorRepo.Create(ctx, vendorPayee); err != nil {
			return nil, fmt.Errorf("failed to create vendor payee: %w", err)
		}

		fmt.Printf("✅ Vendor payee created with ID: %s\n", payeeID)
	} else {
		fmt.Printf("♻️  Step 3: Vendor already exists (PayeeID: %s), reusing...\n", existingVendor.PayeeID)
		payeeID = existingVendor.PayeeID
		vendorPayee = existingVendor
	}

	// Step 4: Check if bank account already exists for this payee
	fmt.Println("🔍 Step 4: Checking for duplicate bank account...")
	accountHash := s.computeAccountHash(vendor.AccountNumber)
	isDuplicate, err := s.payeeBankAccountRepo.CheckDuplicateAccount(ctx, payeeID, accountHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate account: %w", err)
	}

	var bankAccount *payee.PayeeBankAccount

	if isDuplicate {
		fmt.Println("♻️  Bank account already exists for this payee, fetching...")
		// Find the existing account
		accounts, err := s.payeeBankAccountRepo.FindByPayeeID(ctx, payeeID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch existing accounts: %w", err)
		}
		for _, acc := range accounts {
			if acc.AccountNumberHash == accountHash {
				bankAccount = acc
				break
			}
		}
		if bankAccount == nil {
			return nil, fmt.Errorf("duplicate account detected but not found")
		}
	} else {
		fmt.Println("🆕 Bank account not found, verifying via penny drop...")

		// Step 5: Verify bank account (penny drop)
		bankReq := business_rules.BankVerificationRequest{
			BusinessID:        req.BusinessID,
			RequestID:         fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix()),
			AccountNumber:     vendor.AccountNumber,
			IFSCCode:          vendor.IFSCCode,
			AccountHolderName: vendor.AccountHolderName,
			BankName:          vendor.BankName,
			AccountType:       vendor.AccountType,
		}

		bankResp, err := s.bankVerificationSvc.VerifyBankAccount(ctx, bankReq)
		if err != nil {
			return nil, fmt.Errorf("bank verification failed: %w", err)
		}

		if bankResp.Status != business_rules.BankVerificationStatusSuccess {
			return nil, fmt.Errorf("bank verification failed: %s", bankResp.Status)
		}

		fmt.Printf("✅ Bank verified: %s at %s\n", bankResp.VerifiedAccountName, bankResp.BankName)

		// Step 6: Cross-verify GST name vs Bank name
		fmt.Println("🔗 Step 6: Cross-verifying GST name vs Bank account name...")
		crossReq := business_rules.CrossVerificationRequest{
			GSTIN:           vendor.GSTIN,
			GSTLegalName:    vendorPayee.LegalName,
			BankAccountName: bankResp.VerifiedAccountName,
			BankIFSC:        vendor.IFSCCode,
		}

		crossResp, err := s.crossVerificationSvc.VerifyGSTBankMatch(ctx, crossReq)
		if err != nil {
			return nil, fmt.Errorf("cross-verification failed: %w", err)
		}

		if crossResp.RecommendedAction == "reject" {
			return nil, fmt.Errorf("cross-verification failed: name mismatch (score: %.2f)", crossResp.MatchScore)
		}

		fmt.Printf("✅ Cross-verification passed: Match score %.2f (%s)\n", crossResp.MatchScore, crossResp.RecommendedAction)

		// Step 7: Money rotation check (placeholder for now)
		fmt.Println("💰 Step 7: Checking for money rotation...")

		// Extract director names from GST data
		var directorNames []string
		if vendorPayee.GSTData != nil {
			if directors, ok := vendorPayee.GSTData["directors"].([]business_rules.GSTDirector); ok {
				for _, dir := range directors {
					directorNames = append(directorNames, dir.Name)
				}
			}
		}

		moneyRotationReq := business_rules.MoneyRotationCheckRequest{
			CustomerBusinessID: req.BusinessID,
			PayeeAccountName:   bankResp.VerifiedAccountName,
			PayeeGSTDirectors:  directorNames,
		}

		moneyRotationResp, err := s.crossVerificationSvc.CheckMoneyRotation(ctx, moneyRotationReq)
		if err != nil {
			return nil, fmt.Errorf("money rotation check failed: %w", err)
		}

		if moneyRotationResp.IsMoneyRotation {
			return nil, fmt.Errorf("money rotation detected: %s", moneyRotationResp.Details)
		}

		fmt.Println("✅ Money rotation check passed")

		// Create PayeeBankAccount entity
		bankAccount = &payee.PayeeBankAccount{
			PayeeID:                  payeeID,
			AccountNumber:            vendor.AccountNumber,
			AccountNumberHash:        accountHash,
			IFSCCode:                 vendor.IFSCCode,
			BankName:                 bankResp.BankName,
			BranchName:               bankResp.BranchName,
			AccountType:              vendor.AccountType,
			AccountHolderName:        bankResp.VerifiedAccountName,
			IsVerified:               true,
			VerifiedAt:               &time.Time{},
			BankVerificationStatus:   payee.VerificationStatusSuccess,
			CrossVerificationStatus:  payee.VerificationStatusSuccess,
			NameMatchScore:           crossResp.MatchScore,
			IsActive:                 true,
			IsPrimary:                false, // Will check if this should be primary
			AccountLabel:             vendor.AccountLabel,
			VerificationData:         bankResp.Metadata,
		}
		*bankAccount.VerifiedAt = time.Now()
	}

	// Step 8: Check if this should be the primary account
	if bankAccount.AccountID == "" { // New account being created
		existingAccounts, _ := s.payeeBankAccountRepo.FindByPayeeID(ctx, payeeID)
		if len(existingAccounts) == 0 {
			bankAccount.IsPrimary = true
			fmt.Println("📌 Setting as primary account (first account for this payee)")
		}
	}

	// Step 9: Check if relation already exists
	fmt.Println("🔍 Step 9: Checking if customer already has this payee-account relation...")
	if bankAccount.AccountID != "" { // Account exists
		relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, bankAccount.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to check relation existence: %w", err)
		}
		if relationExists {
			return nil, fmt.Errorf("payee already added to your account with this bank account")
		}
	}

	// Step 10: Atomically create account + relation in transaction
	fmt.Println("💾 Step 10: Creating bank account and relation (in transaction)...")

	var accountID string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBankAccountRepo := s.payeeBankAccountRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		// Create bank account if new
		if bankAccount.AccountID == "" {
			if err := txBankAccountRepo.CreateBankAccount(ctx, bankAccount); err != nil {
				return fmt.Errorf("failed to create bank account: %w", err)
			}
			accountID = bankAccount.AccountID
		} else {
			accountID = bankAccount.AccountID
		}

		// Create relation
		relation := &payee.BusinessPayeeRelation{
			BusinessID:   req.BusinessID,
			PayeeID:      payeeID,
			AccountID:    accountID,
			DisplayName:  req.DisplayName,
			ContactEmail: req.ContactEmail,
			ContactPhone: req.ContactPhone,
			Notes:        req.Notes,
			IsActive:     true,
			IsFavorite:   false,
			Tags:         []string{},
			AddedBy:      "", // TODO: Extract from JWT
		}

		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	fmt.Println("✅ Payee onboarding complete!")

	// Step 11: Return response
	return &payee.OnboardPayeeResponse{
		PayeeID:                 payeeID,
		AccountID:               accountID,
		GSTVerificationStatus:   vendorPayee.GSTVerificationStatus,
		BankVerificationStatus:  bankAccount.BankVerificationStatus,
		CrossVerificationStatus: bankAccount.CrossVerificationStatus,
		IsFullyVerified:         true,
		Message:                 fmt.Sprintf("Payee '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// onboardGSTPayee handles GST payment payee onboarding (government virtual account)
// Steps: Validate → Check duplicates → Create master + GST + virtual account entities → Create relation
func (s *PayeeService) onboardGSTPayee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Println("=== GST PAYEE ONBOARDING FLOW ===")

	// Step 1: Validate GST-specific data
	if err := s.validateGSTRequest(&req); err != nil {
		return nil, err
	}

	gst := req.GST
	var payeeID string
	var virtualAccountID string

	// Step 2: Check if the government GST payee exists (singleton pattern)
	fmt.Println("🔍 Step 2: Checking if GST payee exists (singleton)...")

	// Try to get the government payee (singleton)
	existingGST, err := s.payeeGSTRepo.GetGovernmentPayee(ctx)
	if err != nil && err.Error() != "government payee not found" {
		return nil, fmt.Errorf("failed to check existing GST payee: %w", err)
	}

	if existingGST != nil {
		// GST payee exists - reuse it
		payeeID = existingGST.PayeeID
		fmt.Printf("📋 GST payee exists with PayeeID: %s\n", payeeID)
	} else {
		// Create new GST payee (singleton)
		fmt.Println("🆕 Creating new GST payee (singleton)...")

		// Step 3: Create master entry first
		payeeMaster := &payee.PayeeMaster{
			PayeeType:   payee.PayeeTypeGST,
			DisplayName: "GST Portal", // Standard name
			IsActive:    true,
		}
		if err := s.payeeMasterRepo.Create(ctx, payeeMaster); err != nil {
			return nil, fmt.Errorf("failed to create payee master: %w", err)
		}
		payeeID = payeeMaster.PayeeID
		fmt.Printf("✅ Master entry created: PayeeID=%s\n", payeeID)

		// Step 4: Create GST payee entity (no Create method - will be created by repository setup)
		// The GST payee entity creation happens in database migration/setup
		fmt.Println("✅ GST payee entity will use singleton pattern")
	}

	// Step 5: Check if virtual account for this tax period already exists
	fmt.Printf("🔍 Step 5: Checking for virtual account (tax period: %s, FY: %s)...\n", gst.TaxPeriod, gst.FinancialYear)
	existingVirtualAccounts, err := s.payeeGSTVirtualAccountRepo.FindByPayeeID(ctx, payeeID)
	if err != nil && err.Error() != "no virtual accounts found" {
		return nil, fmt.Errorf("failed to find virtual accounts: %w", err)
	}

	// Check if this specific tax period/FY virtual account exists
	var existingVA *payee.PayeeGSTVirtualAccount
	for _, va := range existingVirtualAccounts {
		if va.TaxPeriod == gst.TaxPeriod && va.FinancialYear == gst.FinancialYear {
			existingVA = va
			break
		}
	}

	if existingVA != nil {
		// Virtual account exists for this period
		fmt.Printf("📋 Virtual account exists for %s/%s\n", gst.TaxPeriod, gst.FinancialYear)

		// Check if it's still valid
		if time.Now().After(existingVA.ValidUntil) {
			fmt.Println("⚠️ Existing virtual account expired, creating new one...")
			// Create new virtual account for this period
			newVirtualAccount := &payee.PayeeGSTVirtualAccount{
				PayeeID:              payeeID,
				VirtualAccountNumber: gst.VirtualAccountNumber,
				IFSCCode:             gst.IFSCCode,
				BankName:             gst.BankName,
				TaxPeriod:            gst.TaxPeriod,
				FinancialYear:        gst.FinancialYear,
				TaxType:              gst.TaxType,
				ValidFrom:            gst.ValidFrom,
				ValidUntil:           gst.ValidUntil,
				IsActive:             true,
				ChallanDocumentURL:   gst.ChallanDocumentURL,
			}
			if err := s.payeeGSTVirtualAccountRepo.Create(ctx, newVirtualAccount); err != nil {
				return nil, fmt.Errorf("failed to create new virtual account: %w", err)
			}
			virtualAccountID = newVirtualAccount.VirtualAccountID
		} else {
			fmt.Println("✅ Virtual account still valid, reusing...")
			virtualAccountID = existingVA.VirtualAccountID
		}
	} else {
		// Create new virtual account for this tax period
		fmt.Printf("🆕 Creating new virtual account for %s/%s...\n", gst.TaxPeriod, gst.FinancialYear)
		virtualAccount := &payee.PayeeGSTVirtualAccount{
			PayeeID:              payeeID,
			VirtualAccountNumber: gst.VirtualAccountNumber,
			IFSCCode:             gst.IFSCCode,
			BankName:             gst.BankName,
			TaxPeriod:            gst.TaxPeriod,
			FinancialYear:        gst.FinancialYear,
			TaxType:              gst.TaxType,
			ValidFrom:            gst.ValidFrom,
			ValidUntil:           gst.ValidUntil,
			IsActive:             true,
			ChallanDocumentURL:   gst.ChallanDocumentURL,
		}
		if err := s.payeeGSTVirtualAccountRepo.Create(ctx, virtualAccount); err != nil {
			return nil, fmt.Errorf("failed to create virtual account: %w", err)
		}
		virtualAccountID = virtualAccount.VirtualAccountID
		fmt.Printf("✅ Virtual account created: %s\n", virtualAccountID)
	}

	// Step 6: Check if relation already exists
	fmt.Println("🔍 Step 6: Checking if customer already has this GST payee relation...")
	relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, virtualAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check relation existence: %w", err)
	}
	if relationExists {
		return nil, fmt.Errorf("GST payee already added to your account for this tax period")
	}

	// Step 7: Create business-payee relation
	fmt.Println("💾 Step 7: Creating business-payee relation...")
	relation := &payee.BusinessPayeeRelation{
		BusinessID:   req.BusinessID,
		PayeeID:      payeeID,
		AccountID:    virtualAccountID,
		DisplayName:  req.DisplayName,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Notes:        req.Notes,
		IsActive:     true,
		IsFavorite:   false,
		Tags:         []string{"gst", "tax"},
		AddedBy:      "", // TODO: Extract from JWT
	}

	if err := s.businessPayeeRelationRepo.CreateRelation(ctx, relation); err != nil {
		return nil, fmt.Errorf("failed to create relation: %w", err)
	}

	fmt.Println("✅ GST payee onboarding complete!")

	// Step 8: Return response
	return &payee.OnboardPayeeResponse{
		PayeeID:                 payeeID,
		AccountID:               virtualAccountID,
		GSTVerificationStatus:   payee.VerificationStatusSuccess, // Government entity pre-verified
		BankVerificationStatus:  payee.VerificationStatusSuccess, // Virtual account from challan pre-verified
		CrossVerificationStatus: payee.VerificationStatusSuccess, // No cross-verification needed for govt
		IsFullyVerified:         true,                            // Virtual accounts from challan are pre-verified
		Message:                 fmt.Sprintf("GST payee '%s' successfully onboarded for %s/%s", req.DisplayName, gst.TaxPeriod, gst.FinancialYear),
	}, nil
}

// GetPayeesByBusiness retrieves all payees for a business
func (s *PayeeService) GetPayeesByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PayeeService] GetPayeesByBusiness called - BusinessID: %s\n", businessID)
	return s.businessPayeeRelationRepo.FindByBusiness(ctx, businessID, filters)
}

// GetPayeeDetails retrieves detailed information about a specific payee
// TODO: Phase 6 - Implement polymorphic GetPayeeDetails
func (s *PayeeService) GetPayeeDetails(ctx context.Context, businessID, payeeID, accountID string) (*payee.PayeeDetailsResponse, error) {
	return nil, fmt.Errorf("GetPayeeDetails not yet implemented for polymorphic payees (Phase 6)")
}

// UpdatePayeeRelation updates customer-specific metadata for a payee
func (s *PayeeService) UpdatePayeeRelation(ctx context.Context, req payee.UpdateRelationRequest) error {
	fmt.Printf("[PayeeService] UpdatePayeeRelation called - BusinessID: %s, PayeeID: %s\n",
		req.BusinessID, req.PayeeID)

	relation, err := s.businessPayeeRelationRepo.FindRelation(ctx, req.BusinessID, req.PayeeID, req.AccountID)
	if err != nil {
		return fmt.Errorf("failed to find relation: %w", err)
	}

	// Update fields
	relation.DisplayName = req.DisplayName
	relation.ContactEmail = req.ContactEmail
	relation.ContactPhone = req.ContactPhone
	relation.Notes = req.Notes
	relation.IsFavorite = req.IsFavorite
	relation.Tags = req.Tags

	return s.businessPayeeRelationRepo.UpdateRelation(ctx, relation)
}

// DeactivatePayee deactivates a payee relation (soft delete)
func (s *PayeeService) DeactivatePayee(ctx context.Context, businessID, payeeID, accountID string) error {
	fmt.Printf("[PayeeService] DeactivatePayee called - BusinessID: %s, PayeeID: %s\n", businessID, payeeID)
	return s.businessPayeeRelationRepo.DeleteRelation(ctx, businessID, payeeID, accountID)
}

// Helper methods

// validateVendorRequest validates vendor-specific onboarding data
func (s *PayeeService) validateVendorRequest(req *payee.OnboardPayeeRequest) error {
	// Common validations
	if req.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	if req.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}

	// Vendor-specific validations
	if req.Vendor == nil {
		return fmt.Errorf("vendor data is required for vendor payee type")
	}

	vendor := req.Vendor
	if vendor.GSTIN == "" || len(vendor.GSTIN) != 15 {
		return fmt.Errorf("invalid GSTIN (must be 15 characters)")
	}
	if vendor.AccountNumber == "" {
		return fmt.Errorf("account_number is required")
	}
	if vendor.IFSCCode == "" || len(vendor.IFSCCode) != 11 {
		return fmt.Errorf("invalid IFSC code (must be 11 characters)")
	}
	if vendor.AccountHolderName == "" {
		return fmt.Errorf("account_holder_name is required")
	}
	return nil
}

// validateGSTRequest validates GST payment-specific onboarding data
func (s *PayeeService) validateGSTRequest(req *payee.OnboardPayeeRequest) error {
	// Common validations
	if req.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	if req.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}

	// GST-specific validations
	if req.GST == nil {
		return fmt.Errorf("gst data is required for GST payee type")
	}

	gst := req.GST
	if gst.VirtualAccountNumber == "" {
		return fmt.Errorf("virtual_account_number is required")
	}
	if gst.IFSCCode == "" || len(gst.IFSCCode) != 11 {
		return fmt.Errorf("invalid IFSC code (must be 11 characters)")
	}
	if gst.BankName == "" {
		return fmt.Errorf("bank_name is required")
	}
	if gst.TaxPeriod == "" {
		return fmt.Errorf("tax_period is required (e.g., '2024-03')")
	}
	if gst.FinancialYear == "" {
		return fmt.Errorf("financial_year is required (e.g., '2024-25')")
	}
	if gst.ValidFrom.IsZero() {
		return fmt.Errorf("valid_from date is required")
	}
	if gst.ValidUntil.IsZero() {
		return fmt.Errorf("valid_until date is required")
	}
	if gst.ChallanDocumentURL == "" {
		return fmt.Errorf("challan_document_url is required")
	}
	return nil
}

func (s *PayeeService) computeAccountHash(accountNumber string) string {
	hash := sha256.Sum256([]byte(accountNumber))
	return hex.EncodeToString(hash[:])
}

func (s *PayeeService) maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 5 {
		return accountNumber
	}
	return "****" + accountNumber[len(accountNumber)-5:]
}
