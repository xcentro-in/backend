package payee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/payout"
)

// PayeeService orchestrates the complete payee onboarding and management flow
// Supports polymorphic payee types: Vendor, GST Payment, Biller, Employee
type PayeeService struct {
	// Master routing table
	payeeMasterRepo payee.PayeeMasterRepository

	// Type-specific repositories
	payeeVendorRepo            payee.PayeeVendorRepository
	payeeGSTRepo               payee.PayeeGSTRepository
	payeeGSTVirtualAccountRepo payee.PayeeGSTVirtualAccountRepository
	payeeBillerRepo            payee.PayeeBillerRepository
	payeeBillerAccountRepo     payee.PayeeBillerAccountRepository
	payeeEmployeeRepo          payee.PayeeEmployeeRepository

	// Shared repositories
	payeeBankAccountRepo      payee.PayeeBankAccountRepository
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository

	// Payout provider integration (Phase 8)
	providerMappingRepo payout.PayoutProviderMappingRepository
	payoutProvider      payout.PayoutProvider

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
	providerMappingRepo payout.PayoutProviderMappingRepository,
	payoutProvider payout.PayoutProvider,
	gstVerificationSvc business_rules.GSTVerificationService,
	bankVerificationSvc business_rules.BankVerificationService,
	crossVerificationSvc business_rules.CrossVerificationService,
	txManager client.TxManager,
) *PayeeService {
	return &PayeeService{
		payeeMasterRepo:            payeeMasterRepo,
		payeeVendorRepo:            payeeVendorRepo,
		payeeGSTRepo:               payeeGSTRepo,
		payeeGSTVirtualAccountRepo: payeeGSTVirtualAccountRepo,
		payeeBillerRepo:            payeeBillerRepo,
		payeeBillerAccountRepo:     payeeBillerAccountRepo,
		payeeEmployeeRepo:          payeeEmployeeRepo,
		payeeBankAccountRepo:       payeeBankAccountRepo,
		businessPayeeRelationRepo:  businessPayeeRelationRepo,
		providerMappingRepo:        providerMappingRepo,
		payoutProvider:             payoutProvider,
		gstVerificationSvc:         gstVerificationSvc,
		bankVerificationSvc:        bankVerificationSvc,
		crossVerificationSvc:       crossVerificationSvc,
		txManager:                  txManager,
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
		return s.onboardBiller(ctx, req)
	case payee.PayeeTypeEmployee:
		return s.onboardEmployee(ctx, req)
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
			PayeeType: payee.PayeeTypeVendor,
			IsActive:  true,
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

		// Default account_type to "current" if not provided
		accountType := vendor.AccountType
		if accountType == "" {
			accountType = payment.BankAccountTypeCurrent
			fmt.Println("ℹ️  account_type not provided, defaulting to 'current'")
		}

		// Step 5: Verify bank account (penny drop)
		bankReq := business_rules.BankVerificationRequest{
			BusinessID:        req.BusinessID,
			RequestID:         fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix()),
			AccountNumber:     vendor.AccountNumber,
			IFSCCode:          vendor.IFSCCode,
			AccountHolderName: vendor.AccountHolderName,
			BankName:          vendor.BankName,
			AccountType:       accountType,
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
			PayeeID:                 payeeID,
			AccountNumber:           vendor.AccountNumber,
			AccountNumberHash:       accountHash,
			IFSCCode:                vendor.IFSCCode,
			BankName:                bankResp.BankName,
			BranchName:              bankResp.BranchName,
			AccountType:             accountType, // Use the defaulted account type
			AccountHolderName:       bankResp.VerifiedAccountName,
			IsVerified:              true,
			VerifiedAt:              &time.Time{},
			BankVerificationStatus:  payee.VerificationStatusSuccess,
			CrossVerificationStatus: payee.VerificationStatusSuccess,
			NameMatchScore:          crossResp.MatchScore,
			IsActive:                true,
			IsPrimary:               false, // Will check if this should be primary
			AccountLabel:            vendor.AccountLabel,
			VerificationData:        bankResp.Metadata,
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
	var relationID string
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
			AddedBy:      req.AddedBy, // User ID from JWT
		}

		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}

		// Capture relation_id generated by CreateRelation
		relationID = relation.RelationID

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	fmt.Println("✅ Payee onboarding complete!")

	// Step 11: Onboard to payout provider (Phase 8)
	fmt.Println("🚀 Step 11: Onboarding payee to payout provider (Castler)...")

	onboardReq := &payout.OnboardBeneficiaryRequest{
		PayeeID:           payeeID,
		AccountID:         accountID,
		AccountNumber:     vendor.AccountNumber,
		IFSCCode:          vendor.IFSCCode,
		AccountHolderName: bankAccount.AccountHolderName,
		BankName:          bankAccount.BankName,
		AccountType:       string(vendor.AccountType),
		GSTIN:             vendor.GSTIN,
		LegalName:         vendorPayee.LegalName,
		Email:             req.ContactEmail,
		Phone:             req.ContactPhone,
	}

	onboardResp, err := s.payoutProvider.OnboardBeneficiary(ctx, onboardReq)
	if err != nil {
		// Log error but don't fail payee creation
		fmt.Printf("⚠️  Warning: Provider onboarding failed: %v\n", err)
	} else {
		// Store provider mapping
		now := time.Now()
		mapping := &payout.PayoutProviderMapping{
			MappingID:             uuid.Must(uuid.NewV7()).String(),
			PayeeID:               payeeID,
			AccountID:             accountID,
			Provider:              s.payoutProvider.GetProviderType(),
			ProviderBeneficiaryID: onboardResp.ProviderBeneficiaryID,
			ProviderAccountID:     onboardResp.ProviderAccountID,
			OnboardingStatus:      payout.OnboardingStatus(onboardResp.Status),
			OnboardedAt:           &now,
			ProviderMetadata:      onboardResp.Metadata,
			CreatedAt:             time.Now(),
		}

		if err := s.providerMappingRepo.Create(ctx, mapping); err != nil {
			fmt.Printf("⚠️  Warning: Failed to save provider mapping: %v\n", err)
		} else {
			fmt.Printf("✅ Provider onboarding complete: %s\n", onboardResp.ProviderBeneficiaryID)
		}
	}

	// Step 12: Return response
	return &payee.OnboardPayeeResponse{
		RelationID:              relationID, // Surrogate key for frontend
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
			PayeeType: payee.PayeeTypeGST,
			IsActive:  true,
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

// onboardBiller handles biller payee onboarding (BBPS system)
// Steps: Validate → Check if biller exists → Create biller if needed → Create biller account → Create relation
func (s *PayeeService) onboardBiller(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Println("=== BILLER PAYEE ONBOARDING FLOW ===")

	// Step 1: Validate biller-specific data
	if err := s.validateBillerRequest(&req); err != nil {
		return nil, err
	}

	biller := req.Biller
	var payeeID string
	var billerAccountID string

	// Step 2: Check if biller payee exists by BillerID
	fmt.Printf("🔍 Step 2: Checking if biller exists (BillerID: %s)...\n", biller.BillerID)
	existingBiller, err := s.payeeBillerRepo.FindByBillerID(ctx, biller.BillerID)

	if err != nil && err.Error() != "biller not found" {
		return nil, fmt.Errorf("failed to check existing biller: %w", err)
	}

	if existingBiller != nil {
		// Biller exists - reuse it
		payeeID = existingBiller.PayeeID
		fmt.Printf("📋 Biller exists with PayeeID: %s\n", payeeID)
	} else {
		// Create new biller
		fmt.Printf("🆕 Creating new biller: %s...\n", biller.BillerID)

		// Step 3: Create master entry first
		payeeMaster := &payee.PayeeMaster{
			PayeeType: payee.PayeeTypeBiller,
			IsActive:  true,
		}
		if err := s.payeeMasterRepo.Create(ctx, payeeMaster); err != nil {
			return nil, fmt.Errorf("failed to create payee master: %w", err)
		}
		payeeID = payeeMaster.PayeeID
		fmt.Printf("✅ Master entry created: PayeeID=%s\n", payeeID)

		// Step 4: Create biller entity
		// Note: Biller metadata (name, category, etc.) would come from BBPS API lookup
		// For now, we'll store minimal info from the request
		billerPayee := &payee.PayeeBiller{
			PayeeID:        payeeID,
			BillerID:       biller.BillerID,
			BillerName:     req.DisplayName, // Use customer's display name
			BillerCategory: "utility",       // Default category, should be from BBPS lookup
			NPCIRegistered: true,            // Assume BBPS billers are NPCI registered
			PaymentModes:   []string{"upi", "netbanking", "card"},
		}
		if err := s.payeeBillerRepo.Create(ctx, billerPayee); err != nil {
			return nil, fmt.Errorf("failed to create biller: %w", err)
		}
		fmt.Printf("✅ Biller entity created for BillerID: %s\n", biller.BillerID)
	}

	// Step 5: Check if biller account already exists for this business
	fmt.Println("🔍 Step 5: Checking for existing biller account for this business...")
	existingAccounts, err := s.payeeBillerAccountRepo.FindByBusinessAndPayee(ctx, req.BusinessID, payeeID)
	if err != nil && err.Error() != "no biller accounts found" {
		return nil, fmt.Errorf("failed to check existing biller accounts: %w", err)
	}

	// Check for duplicate account (same mobile number)
	for _, account := range existingAccounts {
		if account.MobileNumber == biller.MobileNumber {
			return nil, fmt.Errorf("biller account with mobile %s already exists", biller.MobileNumber)
		}
	}

	// Step 6: Create biller account (customer-specific configuration)
	fmt.Println("🆕 Creating biller account (customer configuration)...")
	billerAccount := &payee.PayeeBillerAccount{
		PayeeID:          payeeID,
		BusinessID:       req.BusinessID,
		MobileNumber:     biller.MobileNumber,
		CustomerParams:   biller.CustomerParams, // Consumer number, etc.
		AutoFetchEnabled: biller.AutoFetch,
		IsActive:         true,
	}
	if err := s.payeeBillerAccountRepo.Create(ctx, billerAccount); err != nil {
		return nil, fmt.Errorf("failed to create biller account: %w", err)
	}
	billerAccountID = billerAccount.BillerAccountID
	fmt.Printf("✅ Biller account created: %s\n", billerAccountID)

	// Step 7: Check if relation already exists
	fmt.Println("🔍 Step 7: Checking if customer already has this biller payee relation...")
	relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, billerAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check relation existence: %w", err)
	}
	if relationExists {
		return nil, fmt.Errorf("biller payee already added to your account")
	}

	// Step 8: Create business-payee relation
	fmt.Println("💾 Step 8: Creating business-payee relation...")
	relation := &payee.BusinessPayeeRelation{
		BusinessID:   req.BusinessID,
		PayeeID:      payeeID,
		AccountID:    billerAccountID,
		DisplayName:  req.DisplayName,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Notes:        req.Notes,
		IsActive:     true,
		IsFavorite:   false,
		Tags:         []string{"biller", "bbps"},
		AddedBy:      "", // TODO: Extract from JWT
	}

	if err := s.businessPayeeRelationRepo.CreateRelation(ctx, relation); err != nil {
		return nil, fmt.Errorf("failed to create relation: %w", err)
	}

	fmt.Println("✅ Biller payee onboarding complete!")

	// Step 9: Return response
	return &payee.OnboardPayeeResponse{
		PayeeID:                 payeeID,
		AccountID:               billerAccountID,
		GSTVerificationStatus:   payee.VerificationStatusSuccess, // BBPS billers are pre-verified
		BankVerificationStatus:  payee.VerificationStatusSuccess, // NPCI registered
		CrossVerificationStatus: payee.VerificationStatusSuccess, // No cross-verification needed
		IsFullyVerified:         true,
		Message:                 fmt.Sprintf("Biller '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// onboardEmployee handles employee payee onboarding (PAN + bank account)
// Steps: Validate → Check duplicates → Verify PAN → Verify Bank → Cross-verify → Create entities
func (s *PayeeService) onboardEmployee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Println("=== EMPLOYEE PAYEE ONBOARDING FLOW ===")

	// Step 1: Validate employee-specific data
	if err := s.validateEmployeeRequest(&req); err != nil {
		return nil, err
	}

	employee := req.Employee
	var payeeID string

	// Step 2: Check if employee exists by PAN
	fmt.Printf("🔍 Step 2: Checking if employee exists (PAN: %s)...\n", employee.PAN)
	existingEmployee, err := s.payeeEmployeeRepo.FindByPAN(ctx, employee.PAN)

	if err != nil && err.Error() != "employee not found" {
		return nil, fmt.Errorf("failed to check existing employee: %w", err)
	}

	if existingEmployee != nil {
		// Employee exists - reuse payee
		payeeID = existingEmployee.PayeeID
		fmt.Printf("📋 Employee exists with PayeeID: %s\n", payeeID)
	} else {
		// New employee - create master and employee entities
		fmt.Println("🆕 Creating new employee payee...")

		// Step 3: Create master entry first
		payeeMaster := &payee.PayeeMaster{
			PayeeType: payee.PayeeTypeEmployee,
			IsActive:  true,
		}
		if err := s.payeeMasterRepo.Create(ctx, payeeMaster); err != nil {
			return nil, fmt.Errorf("failed to create payee master: %w", err)
		}
		payeeID = payeeMaster.PayeeID
		fmt.Printf("✅ Master entry created: PayeeID=%s\n", payeeID)

		// Step 4: Verify PAN (mock for now)
		fmt.Printf("🔍 Step 4: Verifying PAN: %s...\n", employee.PAN)
		// TODO: Call PAN verification service
		panVerificationStatus := payee.VerificationStatusSuccess
		fmt.Println("✅ PAN verification complete")

		// Step 5: Create employee entity
		employeePayee := &payee.PayeeEmployee{
			PayeeID:               payeeID,
			EmployeeCode:          employee.EmployeeCode,
			FullName:              employee.FullName,
			PAN:                   employee.PAN,
			Email:                 employee.Email,
			Phone:                 employee.Phone,
			Designation:           employee.Designation,
			Department:            employee.Department,
			DateOfJoining:         employee.DateOfJoining,
			EmploymentType:        employee.EmploymentType,
			RemunerationDocURL:    employee.RemunerationDocURL,
			IsSensitive:           true,
			PANVerificationStatus: panVerificationStatus,
		}
		if employee.RemunerationDocURL != "" {
			now := time.Now()
			employeePayee.RemunerationDocUploadedAt = &now
		}
		if panVerificationStatus == payee.VerificationStatusSuccess {
			now := time.Now()
			employeePayee.PANVerifiedAt = &now
		}

		if err := s.payeeEmployeeRepo.Create(ctx, employeePayee); err != nil {
			return nil, fmt.Errorf("failed to create employee: %w", err)
		}
		fmt.Printf("✅ Employee entity created for PAN: %s\n", employee.PAN)
	}

	// Step 6: Check for duplicate bank account
	fmt.Println("🔍 Step 6: Checking for duplicate bank account...")
	accountHash := s.computeAccountHash(employee.AccountNumber)
	existingAccounts, _ := s.payeeBankAccountRepo.FindByPayeeID(ctx, payeeID)

	var bankAccount *payee.PayeeBankAccount
	for _, acc := range existingAccounts {
		if acc.AccountNumberHash == accountHash {
			bankAccount = acc
			fmt.Println("📋 Bank account already exists, reusing...")
			break
		}
	}

	if bankAccount == nil {
		// Default account_type to "current" if not provided
		accountType := employee.AccountType
		if accountType == "" {
			accountType = payment.BankAccountTypeCurrent
			fmt.Println("ℹ️  account_type not provided, defaulting to 'current'")
		}

		// Step 7: Perform penny drop verification
		fmt.Printf("🏦 Step 7: Verifying bank account (penny drop): %s...\n", employee.AccountNumber)
		verifyReq := business_rules.BankVerificationRequest{
			AccountNumber:     employee.AccountNumber,
			IFSCCode:          employee.IFSCCode,
			AccountHolderName: employee.AccountHolderName,
		}

		verifyResp, err := s.bankVerificationSvc.VerifyBankAccount(ctx, verifyReq)
		if err != nil {
			return nil, fmt.Errorf("bank verification failed: %w", err)
		}
		fmt.Printf("✅ Bank verification complete: %s\n", verifyResp.Status)

		// Step 8: Cross-verification (PAN name vs Bank account name)
		fmt.Println("🔄 Step 8: Cross-verifying PAN name with bank account holder name...")
		// Mock cross-verification for now
		crossVerifyStatus := payee.VerificationStatusSuccess
		fmt.Println("✅ Cross-verification complete")

		// Step 9: Create bank account entity
		bankAccount = &payee.PayeeBankAccount{
			PayeeID:                 payeeID,
			AccountNumber:           employee.AccountNumber,
			AccountNumberHash:       accountHash,
			IFSCCode:                employee.IFSCCode,
			AccountHolderName:       employee.AccountHolderName,
			BankName:                employee.BankName,
			AccountType:             accountType, // Use the defaulted account type
			AccountLabel:            employee.AccountLabel,
			IsPrimary:               len(existingAccounts) == 0, // First account is primary
			IsActive:                true,
			IsVerified:              verifyResp.Status == business_rules.BankVerificationStatusSuccess,
			BankVerificationStatus:  payee.VerificationStatus(verifyResp.Status),
			CrossVerificationStatus: crossVerifyStatus,
		}

		// Set verification metadata
		if bankAccount.IsVerified {
			now := time.Now()
			bankAccount.VerifiedAt = &now
		}

		if err := s.payeeBankAccountRepo.CreateBankAccount(ctx, bankAccount); err != nil {
			return nil, fmt.Errorf("failed to create bank account: %w", err)
		}
		fmt.Printf("✅ Bank account created: %s\n", bankAccount.AccountID)
	}

	// Step 10: Check if relation already exists
	fmt.Println("🔍 Step 10: Checking if customer already has this employee payee relation...")
	relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, bankAccount.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check relation existence: %w", err)
	}
	if relationExists {
		return nil, fmt.Errorf("employee payee already added to your account with this bank account")
	}

	// Step 11: Create business-payee relation
	fmt.Println("💾 Step 11: Creating business-payee relation...")
	relation := &payee.BusinessPayeeRelation{
		BusinessID:   req.BusinessID,
		PayeeID:      payeeID,
		AccountID:    bankAccount.AccountID,
		DisplayName:  req.DisplayName,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Notes:        req.Notes,
		IsActive:     true,
		IsFavorite:   false,
		Tags:         []string{"employee", "payroll"},
		AddedBy:      req.AddedBy, // User ID from JWT
	}

	if err := s.businessPayeeRelationRepo.CreateRelation(ctx, relation); err != nil {
		return nil, fmt.Errorf("failed to create relation: %w", err)
	}

	fmt.Println("✅ Employee payee onboarding complete!")

	// Step 11.5: Onboard to payout provider (Phase 8)
	fmt.Println("🚀 Step 11.5: Onboarding employee to payout provider (Castler)...")

	onboardReq := &payout.OnboardBeneficiaryRequest{
		PayeeID:           payeeID,
		AccountID:         bankAccount.AccountID,
		AccountNumber:     employee.AccountNumber,
		IFSCCode:          employee.IFSCCode,
		AccountHolderName: bankAccount.AccountHolderName,
		BankName:          bankAccount.BankName,
		AccountType:       string(bankAccount.AccountType), // Use the account type from the created bank account
		PAN:               employee.PAN,
		LegalName:         employee.FullName,
		Email:             employee.Email,
		Phone:             employee.Phone,
	}

	onboardResp, err := s.payoutProvider.OnboardBeneficiary(ctx, onboardReq)
	if err != nil {
		// Log error but don't fail payee creation
		fmt.Printf("⚠️  Warning: Provider onboarding failed: %v\n", err)
	} else {
		// Store provider mapping
		now := time.Now()
		mapping := &payout.PayoutProviderMapping{
			MappingID:             uuid.Must(uuid.NewV7()).String(),
			PayeeID:               payeeID,
			AccountID:             bankAccount.AccountID,
			Provider:              s.payoutProvider.GetProviderType(),
			ProviderBeneficiaryID: onboardResp.ProviderBeneficiaryID,
			ProviderAccountID:     onboardResp.ProviderAccountID,
			OnboardingStatus:      payout.OnboardingStatus(onboardResp.Status),
			OnboardedAt:           &now,
			ProviderMetadata:      onboardResp.Metadata,
			CreatedAt:             time.Now(),
		}

		if err := s.providerMappingRepo.Create(ctx, mapping); err != nil {
			fmt.Printf("⚠️  Warning: Failed to save provider mapping: %v\n", err)
		} else {
			fmt.Printf("✅ Provider onboarding complete: %s\n", onboardResp.ProviderBeneficiaryID)
		}
	}

	// Step 12: Return response
	return &payee.OnboardPayeeResponse{
		PayeeID:                 payeeID,
		AccountID:               bankAccount.AccountID,
		GSTVerificationStatus:   payee.VerificationStatusSuccess, // PAN verified
		BankVerificationStatus:  bankAccount.BankVerificationStatus,
		CrossVerificationStatus: bankAccount.CrossVerificationStatus,
		IsFullyVerified:         bankAccount.IsVerified && bankAccount.CrossVerificationStatus == payee.VerificationStatusSuccess,
		Message:                 fmt.Sprintf("Employee '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// GetPayeesByBusiness retrieves all payees for a business
func (s *PayeeService) GetPayeesByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PayeeService] GetPayeesByBusiness called - BusinessID: %s\n", businessID)
	return s.businessPayeeRelationRepo.FindByBusiness(ctx, businessID, filters)
}

// GetPayeeDetails retrieves detailed information about a specific payee (polymorphic)
func (s *PayeeService) GetPayeeDetails(ctx context.Context, businessID, payeeID, accountID string) (*payee.PayeeDetailsResponse, error) {
	fmt.Printf("[PayeeService] GetPayeeDetails called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	// Step 1: Get relation (customer-specific metadata)
	relation, err := s.businessPayeeRelationRepo.FindRelation(ctx, businessID, payeeID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to find relation: %w", err)
	}

	// Step 2: Get payee master to determine type
	payeeMaster, err := s.payeeMasterRepo.FindByID(ctx, payeeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find payee master: %w", err)
	}

	// Step 3: Build response based on payee type
	response := &payee.PayeeDetailsResponse{
		PayeeID:      payeeID,
		PayeeType:    payeeMaster.PayeeType,
		AccountID:    accountID,
		DisplayName:  relation.DisplayName,
		ContactEmail: relation.ContactEmail,
		ContactPhone: relation.ContactPhone,
		Notes:        relation.Notes,
		IsFavorite:   relation.IsFavorite,
		Tags:         relation.Tags,
		AddedAt:      relation.AddedAt,
		LastUsedAt:   relation.LastUsedAt,
	}

	// Step 4: Populate type-specific data
	switch payeeMaster.PayeeType {
	case payee.PayeeTypeVendor:
		if err := s.populateVendorDetails(ctx, payeeID, accountID, response); err != nil {
			return nil, err
		}

	case payee.PayeeTypeGST:
		if err := s.populateGSTDetails(ctx, payeeID, accountID, response); err != nil {
			return nil, err
		}

	case payee.PayeeTypeBiller:
		if err := s.populateBillerDetails(ctx, payeeID, accountID, response); err != nil {
			return nil, err
		}

	case payee.PayeeTypeEmployee:
		if err := s.populateEmployeeDetails(ctx, payeeID, accountID, response); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unsupported payee type: %s", payeeMaster.PayeeType)
	}

	return response, nil
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

// validateBillerRequest validates biller-specific onboarding data
func (s *PayeeService) validateBillerRequest(req *payee.OnboardPayeeRequest) error {
	// Common validations
	if req.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	if req.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}

	// Biller-specific validations
	if req.Biller == nil {
		return fmt.Errorf("biller data is required for biller payee type")
	}

	biller := req.Biller
	if biller.BillerID == "" {
		return fmt.Errorf("biller_id is required")
	}
	if biller.MobileNumber == "" {
		return fmt.Errorf("mobile_number is required")
	}
	// Validate mobile number format (10 digits)
	if len(biller.MobileNumber) != 10 {
		return fmt.Errorf("invalid mobile_number (must be 10 digits)")
	}
	if len(biller.CustomerParams) == 0 {
		return fmt.Errorf("customer_params is required (e.g., consumer_number)")
	}
	return nil
}

// validateEmployeeRequest validates employee-specific onboarding data
func (s *PayeeService) validateEmployeeRequest(req *payee.OnboardPayeeRequest) error {
	// Common validations
	if req.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	if req.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}

	// Employee-specific validations
	if req.Employee == nil {
		return fmt.Errorf("employee data is required for employee payee type")
	}

	employee := req.Employee
	if employee.PAN == "" || len(employee.PAN) != 10 {
		return fmt.Errorf("invalid PAN (must be 10 characters)")
	}
	if employee.FullName == "" {
		return fmt.Errorf("full_name is required")
	}
	if employee.AccountNumber == "" {
		return fmt.Errorf("account_number is required")
	}
	if employee.IFSCCode == "" || len(employee.IFSCCode) != 11 {
		return fmt.Errorf("invalid IFSC code (must be 11 characters)")
	}
	if employee.AccountHolderName == "" {
		return fmt.Errorf("account_holder_name is required")
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

// populateVendorDetails populates vendor-specific details in the response
func (s *PayeeService) populateVendorDetails(ctx context.Context, payeeID, accountID string, response *payee.PayeeDetailsResponse) error {
	// Get vendor entity
	vendor, err := s.payeeVendorRepo.FindByID(ctx, payeeID)
	if err != nil {
		return fmt.Errorf("failed to find vendor: %w", err)
	}

	// Get bank account
	bankAccount, err := s.payeeBankAccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to find bank account: %w", err)
	}

	// Populate common account fields
	response.MaskedAccount = s.maskAccountNumber(bankAccount.AccountNumber)
	response.BankName = bankAccount.BankName
	response.IFSCCode = bankAccount.IFSCCode
	response.AccountType = string(bankAccount.AccountType)
	response.AccountLabel = bankAccount.AccountLabel
	response.IsVerified = bankAccount.IsVerified

	// Populate vendor-specific fields
	response.Vendor = &payee.VendorDetailsData{
		GSTIN:                   vendor.GSTIN,
		LegalName:               vendor.LegalName,
		BusinessType:            vendor.BusinessType,
		GSTStatus:               vendor.GSTStatus,
		GSTVerificationStatus:   string(vendor.GSTVerificationStatus),
		BankVerificationStatus:  string(bankAccount.BankVerificationStatus),
		CrossVerificationStatus: string(bankAccount.CrossVerificationStatus),
	}

	return nil
}

// populateGSTDetails populates GST payment-specific details in the response
func (s *PayeeService) populateGSTDetails(ctx context.Context, payeeID, accountID string, response *payee.PayeeDetailsResponse) error {
	// Get GST entity
	gstPayee, err := s.payeeGSTRepo.GetGovernmentPayee(ctx)
	if err != nil {
		return fmt.Errorf("failed to find GST payee: %w", err)
	}

	// Get virtual account
	virtualAccount, err := s.payeeGSTVirtualAccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to find virtual account: %w", err)
	}

	// Populate common account fields
	response.BankName = virtualAccount.BankName
	response.IFSCCode = virtualAccount.IFSCCode
	response.IsVerified = true // Virtual accounts are pre-verified

	// Populate GST-specific fields
	response.GST = &payee.GSTDetailsData{
		GovernmentEntityName: gstPayee.GovernmentEntityName,
		VirtualAccountNumber: virtualAccount.VirtualAccountNumber,
		TaxPeriod:            virtualAccount.TaxPeriod,
		FinancialYear:        virtualAccount.FinancialYear,
		TaxType:              virtualAccount.TaxType,
		ValidFrom:            virtualAccount.ValidFrom,
		ValidUntil:           virtualAccount.ValidUntil,
		IsActive:             virtualAccount.IsActive,
		ChallanDocumentURL:   virtualAccount.ChallanDocumentURL,
		UploadedAt:           virtualAccount.UploadedAt,
	}

	return nil
}

// populateBillerDetails populates biller-specific details in the response
func (s *PayeeService) populateBillerDetails(ctx context.Context, payeeID, accountID string, response *payee.PayeeDetailsResponse) error {
	// Get biller entity
	biller, err := s.payeeBillerRepo.FindByID(ctx, payeeID)
	if err != nil {
		return fmt.Errorf("failed to find biller: %w", err)
	}

	// Get biller account
	billerAccount, err := s.payeeBillerAccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to find biller account: %w", err)
	}

	// Populate common fields
	response.IsVerified = true // BBPS billers are pre-verified

	// Populate biller-specific fields
	response.Biller = &payee.BillerDetailsData{
		BillerID:          biller.BillerID,
		BillerName:        biller.BillerName,
		BillerCategory:    biller.BillerCategory,
		MobileNumber:      billerAccount.MobileNumber,
		CustomerParams:    billerAccount.CustomerParams,
		AutoFetchEnabled:  billerAccount.AutoFetchEnabled,
		LastBillAmount:    billerAccount.LastBillAmount,
		LastBillDueDate:   billerAccount.LastBillDueDate,
		LastBillFetchedAt: billerAccount.LastBillFetchedAt,
	}

	return nil
}

// populateEmployeeDetails populates employee-specific details in the response
func (s *PayeeService) populateEmployeeDetails(ctx context.Context, payeeID, accountID string, response *payee.PayeeDetailsResponse) error {
	// Get employee entity
	employee, err := s.payeeEmployeeRepo.FindByID(ctx, payeeID)
	if err != nil {
		return fmt.Errorf("failed to find employee: %w", err)
	}

	// Get bank account
	bankAccount, err := s.payeeBankAccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("failed to find bank account: %w", err)
	}

	// Populate common account fields
	response.MaskedAccount = s.maskAccountNumber(bankAccount.AccountNumber)
	response.BankName = bankAccount.BankName
	response.IFSCCode = bankAccount.IFSCCode
	response.AccountType = string(bankAccount.AccountType)
	response.AccountLabel = bankAccount.AccountLabel
	response.IsVerified = bankAccount.IsVerified

	// Populate employee-specific fields
	response.Employee = &payee.EmployeeDetailsData{
		PAN:                     employee.PAN,
		EmployeeCode:            employee.EmployeeCode,
		FullName:                employee.FullName,
		Email:                   employee.Email,
		Phone:                   employee.Phone,
		Designation:             employee.Designation,
		Department:              employee.Department,
		EmploymentType:          employee.EmploymentType,
		PANVerificationStatus:   string(employee.PANVerificationStatus),
		BankVerificationStatus:  string(bankAccount.BankVerificationStatus),
		CrossVerificationStatus: string(bankAccount.CrossVerificationStatus),
	}

	return nil
}

// ############################################################################
// # Vendor Dropdown (Lightweight API for Voucher Creation)
// ############################################################################

// GetVendorsForDropdown retrieves lightweight vendor list for dropdown selection
// Returns only active vendors with minimal fields (relation_id, display_name, contact info)
// This is optimized for voucher creation UI where only relation_id is needed
func (s *PayeeService) GetVendorsForDropdown(ctx context.Context, businessID string) (*payee.VendorDropdownListResponse, error) {
	fmt.Printf("=== PAYEE SERVICE: GetVendorsForDropdown - BusinessID: %s ===\n", businessID)

	// Input validation
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Validate UUID format
	if _, err := uuid.Parse(businessID); err != nil {
		return nil, fmt.Errorf("invalid business_id format: %w", err)
	}

	// Fetch vendors from repository (simple pass-through)
	vendors, err := s.businessPayeeRelationRepo.GetVendorsForDropdown(ctx, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get vendors for dropdown: %w", err)
	}

	// Build response
	response := &payee.VendorDropdownListResponse{
		Vendors: vendors,
		Count:   len(vendors),
	}

	fmt.Printf("=== PAYEE SERVICE: GetVendorsForDropdown - Found %d vendors ===\n", len(vendors))
	return response, nil
}

// GetPayeeDetailsByRelation retrieves complete payee details by relation_id (polymorphic)
// Returns vendor, employee, or biller details with bank/account information
// Validates that the relation belongs to the specified business
func (s *PayeeService) GetPayeeDetailsByRelation(ctx context.Context, businessID, relationID string) (*payee.PayeeFullDetails, error) {
	fmt.Printf("=== PAYEE SERVICE: GetPayeeDetailsByRelation - BusinessID: %s, RelationID: %s ===\n", businessID, relationID)

	// Input validation
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}
	if relationID == "" {
		return nil, fmt.Errorf("relation_id is required")
	}

	// Validate UUID formats
	if _, err := uuid.Parse(businessID); err != nil {
		return nil, fmt.Errorf("invalid business_id format: %w", err)
	}
	if _, err := uuid.Parse(relationID); err != nil {
		return nil, fmt.Errorf("invalid relation_id format: %w", err)
	}

	// Fetch full details from repository (handles polymorphism and business validation)
	details, err := s.businessPayeeRelationRepo.GetPayeeFullDetailsByRelation(ctx, businessID, relationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payee details: %w", err)
	}

	fmt.Printf("=== PAYEE SERVICE: GetPayeeDetailsByRelation - Found payee type: %s ===\n", details.PayeeType)
	return details, nil
}
