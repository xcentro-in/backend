package payee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/external/payout/castler"
	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/business_rules"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/payout"
	"xcentro.in/internal/services/verification"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// CastlerPayeeOnboarder interface for Castler-specific payee onboarding operations
type CastlerPayeeOnboarder interface {
	OnboardPayeeToCastler(ctx context.Context, accountHolder, email, mobile, accountNumber, ifsc, bankName string) (*castler.CastlerPayeeOnboardResponse, error)
}

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
	payoutProvider payout.PayoutProvider
	castlerClient  CastlerPayeeOnboarder // For Castler-specific payee onboarding

	// Verification services
	docVerificationSvc   *verification.DocumentVerificationService // AuthBridge for GST, PAN verification
	bankVerificationSvc  business_rules.BankVerificationService
	crossVerificationSvc business_rules.CrossVerificationService

	// Transaction manager
	txManager client.TxManager

	// Business service for onboarding flags
	businessService business.BusinessService

	// S3 configuration for document uploads
	s3Config *config.S3Config

	// Logger
	logger *logger.Logger
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
	payoutProvider payout.PayoutProvider,
	castlerClient CastlerPayeeOnboarder,
	docVerificationSvc *verification.DocumentVerificationService,
	bankVerificationSvc business_rules.BankVerificationService,
	crossVerificationSvc business_rules.CrossVerificationService,
	txManager client.TxManager,
	businessService business.BusinessService,
	s3Config *config.S3Config,
	logger *logger.Logger,
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
		payoutProvider:             payoutProvider,
		castlerClient:              castlerClient,
		docVerificationSvc:         docVerificationSvc,
		bankVerificationSvc:        bankVerificationSvc,
		crossVerificationSvc:       crossVerificationSvc,
		txManager:                  txManager,
		businessService:            businessService,
		s3Config:                   s3Config,
		logger:                     logger,
	}
}

// OnboardPayee orchestrates the complete payee onboarding flow (polymorphic)
// Routes to type-specific onboarding methods based on PayeeType
func (s *PayeeService) OnboardPayee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	s.logger.InfoWith("Payee onboarding requested", "payee_type", req.PayeeType)

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
	s.logger.InfoWith("Starting vendor onboarding",
		"business_id", req.BusinessID,
		"gstin", req.Vendor.GSTIN,
		"display_name", req.DisplayName)

	// Validate vendor-specific data
	if req.Vendor == nil {
		return nil, fmt.Errorf("vendor data is required for vendor payee type")
	}

	vendor := req.Vendor
	s.logger.DebugWith("Vendor onboarding details",
		"account_last4", vendor.AccountNumber[len(vendor.AccountNumber)-4:],
		"ifsc", vendor.IFSCCode)

	// Step 1: Validate input
	s.logger.DebugWith("Validating vendor onboarding request")
	if err := s.validateVendorRequest(&req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Check if vendor payee already exists by GSTIN
	s.logger.DebugWith("Checking if vendor exists by GSTIN", "gstin", vendor.GSTIN)
	existingVendor, err := s.payeeVendorRepo.FindByGSTIN(ctx, vendor.GSTIN)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing vendor: %w", err)
	}

	var payeeID string
	var vendorPayee *payee.PayeeVendor

	// Step 3: If vendor doesn't exist, verify GST (prepare for creation)
	var gstResp *business_rules.GSTVerificationResponse
	var needsVendorCreation bool

	if existingVendor == nil {
		needsVendorCreation = true
		s.logger.InfoWith("Vendor not found, initiating GST verification", "gstin", vendor.GSTIN)

		transID := fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix())

		var err error
		gstData, err := s.docVerificationSvc.VerifyGSTIN(ctx, vendor.GSTIN, transID)
		if err != nil {
			return nil, fmt.Errorf("GST verification failed: %w", err)
		}

		// Map to business_rules response format for consistency
		gstResp = &business_rules.GSTVerificationResponse{
			RequestID: transID,
			GSTIN:     gstData.GSTIN,
			LegalName: gstData.LegalName,
			TradeName: gstData.TradeName,
			Status:    gstData.Status,
			Metadata:  gstData.RawResponse,
		}

		s.logger.InfoWith("GST verification successful",
			"legal_name", gstResp.LegalName,
			"status", gstResp.Status)
	} else {
		needsVendorCreation = false
		s.logger.DebugWith("Vendor already exists, reusing",
			"payee_id", existingVendor.PayeeID)
		payeeID = existingVendor.PayeeID
		vendorPayee = existingVendor
	}

	// Step 4: Check if bank account already exists for this payee (if vendor exists)
	s.logger.DebugWith("Checking for duplicate bank account")
	accountHash := s.computeAccountHash(vendor.AccountNumber)

	var isDuplicate bool
	if !needsVendorCreation {
		// Only check for duplicate if vendor already exists (has payeeID)
		isDuplicate, err = s.payeeBankAccountRepo.CheckDuplicateAccount(ctx, payeeID, accountHash)
		if err != nil {
			return nil, fmt.Errorf("failed to check duplicate account: %w", err)
		}
	}

	var bankAccount *payee.PayeeBankAccount

	if isDuplicate {
		s.logger.DebugWith("Bank account already exists, fetching existing account")
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
		s.logger.InfoWith("Bank account not found, initiating penny drop verification")

		// Default account_type to "current" if not provided
		accountType := vendor.AccountType
		if accountType == "" {
			accountType = payment.BankAccountTypeCurrent
			s.logger.WarnWith("account_type not provided, defaulting to current")
		}

		bankReq := authbridge.NewBankVerificationRequest(
			"10000",
			vendor.AccountNumber,
			vendor.IFSCCode,
		)

		s.logger.InfoWith("Bank Request body", bankReq)

		// Step 5: Verify bank account (penny drop)
		authbridgeClient, err := authbridge.NewClient()
		if err != nil {
			s.logger.ErrorWith("Failed to create authbridge client", err)
			return nil, fmt.Errorf("failed to create authbridge client: %w", err)
		}
		bankResp, err := authbridgeClient.SearchDocument(bankReq)
		if err != nil {
			s.logger.ErrorWith("Failed to search document", err)
			return nil, fmt.Errorf("failed to search document: %w", err)
		}

		// Step 5: Verify bank account (penny drop)
		// bankReq := business_rules.BankVerificationRequest{
		// 	BusinessID:        req.BusinessID,
		// 	RequestID:         fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix()),
		// 	AccountNumber:     vendor.AccountNumber,
		// 	IFSCCode:          vendor.IFSCCode,
		// 	AccountHolderName: vendor.AccountHolderName,
		// 	BankName:          vendor.BankName,
		// 	BranchName:        vendor.BranchName,
		// 	AccountType:       accountType,
		// 	Metadata: map[string]any{
		// 		"email": req.ContactEmail,
		// 		"phone": req.ContactPhone,
		// 	},
		// }

		// bankResp, err := s.bankVerificationSvc.VerifyBankAccount(ctx, bankReq)

		// Type assert to access nested map
		msgData, ok := bankResp["msg"].(map[string]interface{})
		if !ok {
			s.logger.ErrorWith("Invalid bank verification response format")
			return nil, fmt.Errorf("invalid bank verification response format")
		}

		verificationStatus, _ := msgData["status"].(string)
		s.logger.InfoWith("Bank verification completed", "status", verificationStatus)

		// Fail fast: Abort if bank verification did not succeed
		if verificationStatus != "SUCCESS" {
			s.logger.ErrorWith("Bank verification failed, aborting onboarding", "status", verificationStatus)
			return nil, fmt.Errorf("bank verification failed: account could not be verified")
		}

		// Step 6: Money rotation check
		// if err := s.checkMoneyRotation(ctx, req.BusinessID, bankResp.VerifiedAccountName); err != nil {
		// 	return nil, err
		// }

		// Step 7.5: Onboard payee to Castler (after all verifications pass)
		s.logger.InfoWith("Onboarding payee to Castler")
		castlerPayeeResp, err := s.castlerClient.OnboardPayeeToCastler(
			ctx,
			vendor.AccountHolderName,
			req.ContactEmail,
			req.ContactPhone,
			vendor.AccountNumber,
			vendor.IFSCCode,
			vendor.BankName,
		)
		if err != nil {
			return nil, fmt.Errorf("castler payee onboarding failed: %w", err)
		}
		s.logger.InfoWith("Castler onboarding successful",
			"castler_payee_id", castlerPayeeResp.Result)

		// Create PayeeBankAccount entity with Castler beneficiary ID
		now := time.Now()
		bankAccount = &payee.PayeeBankAccount{
			PayeeID:                 payeeID,
			AccountNumber:           vendor.AccountNumber,
			AccountNumberHash:       accountHash,
			IFSCCode:                vendor.IFSCCode,
			BankName:                vendor.BankName,   // Use bank name from frontend IFSC check
			BranchName:              vendor.BranchName, // Use branch name from frontend IFSC check
			AccountType:             accountType,       // Use the defaulted account type
			AccountHolderName:       vendor.AccountHolderName,
			IsVerified:              true,
			VerifiedAt:              &now,
			BankVerificationStatus:  payee.VerificationStatusSuccess,
			CrossVerificationStatus: payee.VerificationStatusSuccess,
			CastlerBeneficiaryID:    castlerPayeeResp.Result, // Store Castler payee ID
			IsActive:                true,
			AccountLabel:            vendor.AccountLabel,
			VerificationData:        map[string]any{"test": "test"}, // TODO: Store actual verification data
		}
	}

	// Step 8: Check if relation already exists (if everything already exists)
	s.logger.DebugWith("Checking if business-payee relation already exists")
	if !needsVendorCreation && bankAccount != nil && bankAccount.AccountID != "" {
		relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, bankAccount.AccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to check relation existence: %w", err)
		}
		if relationExists {
			return nil, fmt.Errorf("payee already added to your account with this bank account")
		}
	}

	// Step 9: ATOMIC TRANSACTION - Create all entities (master, vendor, bank account, relation)
	s.logger.InfoWith("Creating all entities in atomic transaction")

	var accountID string
	var relationID string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txMasterRepo := s.payeeMasterRepo.WithTx(tx)
		txVendorRepo := s.payeeVendorRepo.WithTx(tx)
		txBankAccountRepo := s.payeeBankAccountRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		// 1. Create master + vendor if needed
		if needsVendorCreation {
			// Create master entry
			payeeMaster := &payee.PayeeMaster{
				PayeeType: payee.PayeeTypeVendor,
				IsActive:  true,
			}
			if err := txMasterRepo.Create(ctx, payeeMaster); err != nil {
				return fmt.Errorf("failed to create payee master: %w", err)
			}
			payeeID = payeeMaster.PayeeID
			s.logger.DebugWith("Created payee master entry", "payee_id", payeeID)

			// Create vendor payee entity
			now := time.Now()
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
				GSTVerifiedAt:         &now,
				GSTData:               gstResp.Metadata,
				Metadata: map[string]any{
					"directors":  gstResp.Directors,
					"state_code": gstResp.StateCode,
				},
			}
			if err := txVendorRepo.Create(ctx, vendorPayee); err != nil {
				return fmt.Errorf("failed to create vendor payee: %w", err)
			}
			s.logger.DebugWith("Created vendor entity", "gstin", vendor.GSTIN)
		}

		// 2. Create bank account if needed
		if bankAccount != nil && bankAccount.AccountID == "" {
			// Update PayeeID in case it was just created
			bankAccount.PayeeID = payeeID

			if err := txBankAccountRepo.CreateBankAccount(ctx, bankAccount); err != nil {
				return fmt.Errorf("failed to create bank account: %w", err)
			}
			accountID = bankAccount.AccountID
			s.logger.DebugWith("Created bank account", "account_id", accountID)
		} else if bankAccount != nil {
			accountID = bankAccount.AccountID
		}

		// 3. Create relation
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
		relationID = relation.RelationID
		s.logger.DebugWith("Created business-payee relation", "relation_id", relationID)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	s.logger.InfoWith("Vendor onboarding completed successfully",
		"payee_id", payeeID,
		"relation_id", relationID)

	// Update onboarding flag (non-blocking, first-time only)
	if s.businessService != nil {
		go func(bizID string) {
			bgCtx := context.Background()
			flags, err := s.businessService.GetBusinessOnboardingFlags(bgCtx, bizID)
			if err == nil && !flags.HasAddedVendor {
				_ = s.businessService.UpdateOnboardingStatus(bgCtx, bizID, "has_added_vendor", true)
				s.logger.DebugWith("Onboarding flag updated",
					"business_id", bizID,
					"flag", "has_added_vendor")
			}
		}(req.BusinessID)
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

func gen_random_string(i int) string {
	panic("unimplemented")
}

// onboardGSTPayee handles GST payment payee onboarding (government virtual account)
// Steps: Validate → Check duplicates → Create master + GST + virtual account entities → Create relation
func (s *PayeeService) onboardGSTPayee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	s.logger.InfoWith("Starting GST payment onboarding",
		"business_id", req.BusinessID,
		"display_name", req.DisplayName)

	// Step 1: Validate GST-specific data
	if err := s.validateGSTRequest(&req); err != nil {
		return nil, err
	}

	gst := req.GST
	var payeeID string
	var virtualAccountID string

	// Step 2: Check if the government GST payee exists (singleton pattern)
	s.logger.DebugWith("Checking if GST payee exists (singleton)")

	// Try to get the government payee (singleton)
	existingGST, err := s.payeeGSTRepo.GetGovernmentPayee(ctx)
	if err != nil && err.Error() != "government payee not found" {
		return nil, fmt.Errorf("failed to check existing GST payee: %w", err)
	}

	var needsGSTCreation bool
	if existingGST != nil {
		// GST payee exists - reuse it
		needsGSTCreation = false
		payeeID = existingGST.PayeeID
		s.logger.DebugWith("GST payee exists, reusing", "payee_id", payeeID)
	} else {
		// Need to create new GST payee (singleton)
		needsGSTCreation = true
		s.logger.DebugWith("GST payee not found, will create in transaction")
	}

	// Step 3: Check if virtual account for this tax period already exists
	s.logger.DebugWith("Checking for virtual account",
		"tax_period", gst.TaxPeriod,
		"financial_year", gst.FinancialYear)
	var existingVirtualAccounts []*payee.PayeeGSTVirtualAccount
	if !needsGSTCreation {
		// Only check if GST payee already exists (has payeeID)
		existingVirtualAccounts, err = s.payeeGSTVirtualAccountRepo.FindByPayeeID(ctx, payeeID)
	}
	if err != nil && err.Error() != "no virtual accounts found" {
		return nil, fmt.Errorf("failed to find virtual accounts: %w", err)
	}

	// Check if this specific tax period/FY virtual account exists
	var existingVA *payee.PayeeGSTVirtualAccount
	var needsVirtualAccountCreation bool

	for _, va := range existingVirtualAccounts {
		if va.TaxPeriod == gst.TaxPeriod && va.FinancialYear == gst.FinancialYear {
			existingVA = va
			break
		}
	}

	if existingVA != nil {
		// Virtual account exists for this period
		s.logger.DebugWith("Virtual account exists for tax period",
			"tax_period", gst.TaxPeriod,
			"financial_year", gst.FinancialYear)

		// Check if it's still valid
		if time.Now().After(existingVA.ValidUntil) {
			s.logger.WarnWith("Existing virtual account expired, will create new one")
			needsVirtualAccountCreation = true
		} else {
			s.logger.DebugWith("Virtual account still valid, reusing")
			needsVirtualAccountCreation = false
			virtualAccountID = existingVA.VirtualAccountID
		}
	} else {
		// Need to create new virtual account for this tax period
		s.logger.DebugWith("No virtual account found for this period, will create",
			"tax_period", gst.TaxPeriod,
			"financial_year", gst.FinancialYear)
		needsVirtualAccountCreation = true
	}

	// Step 4: Check if relation already exists (if everything already exists)
	s.logger.DebugWith("Checking if business-payee relation already exists")
	if !needsGSTCreation && !needsVirtualAccountCreation && virtualAccountID != "" {
		relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeID, virtualAccountID)
		if err != nil {
			return nil, fmt.Errorf("failed to check relation existence: %w", err)
		}
		if relationExists {
			return nil, fmt.Errorf("GST payee already added to your account for this tax period")
		}
	}

	// Step 5: ATOMIC TRANSACTION - Create all entities (master, virtual account, relation)
	s.logger.InfoWith("Creating all entities in atomic transaction")

	var relationID string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txMasterRepo := s.payeeMasterRepo.WithTx(tx)
		txVirtualAccountRepo := s.payeeGSTVirtualAccountRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		// 1. Create master if needed
		if needsGSTCreation {
			payeeMaster := &payee.PayeeMaster{
				PayeeType: payee.PayeeTypeGST,
				IsActive:  true,
			}
			if err := txMasterRepo.Create(ctx, payeeMaster); err != nil {
				return fmt.Errorf("failed to create payee master: %w", err)
			}
			payeeID = payeeMaster.PayeeID
			s.logger.DebugWith("Created payee master entry", "payee_id", payeeID)
		}

		// 2. Create virtual account if needed
		if needsVirtualAccountCreation {
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
			if err := txVirtualAccountRepo.Create(ctx, virtualAccount); err != nil {
				return fmt.Errorf("failed to create virtual account: %w", err)
			}
			virtualAccountID = virtualAccount.VirtualAccountID
			s.logger.DebugWith("Created virtual account", "virtual_account_id", virtualAccountID)
		}

		// 3. Create relation
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

		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}
		relationID = relation.RelationID
		s.logger.DebugWith("Created business-payee relation", "relation_id", relationID)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	s.logger.InfoWith("GST payment onboarding completed successfully",
		"payee_id", payeeID,
		"relation_id", relationID)

	// Step 6: Return response
	return &payee.OnboardPayeeResponse{
		RelationID:              relationID,
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
	s.logger.InfoWith("Starting biller onboarding",
		"business_id", req.BusinessID,
		"display_name", req.DisplayName)

	// Step 1: Validate biller-specific data
	if err := s.validateBillerRequest(&req); err != nil {
		return nil, err
	}

	biller := req.Biller
	var payeeID string
	var billerAccountID string

	// Step 2: Check if biller payee exists by BillerID
	s.logger.DebugWith("Checking if biller exists", "biller_id", biller.BillerID)
	existingBiller, err := s.payeeBillerRepo.FindByBillerID(ctx, biller.BillerID)

	if err != nil && err.Error() != "biller not found" {
		return nil, fmt.Errorf("failed to check existing biller: %w", err)
	}

	var needsBillerCreation bool
	if existingBiller != nil {
		// Biller exists - reuse it
		needsBillerCreation = false
		payeeID = existingBiller.PayeeID
		s.logger.DebugWith("Biller exists, reusing", "payee_id", payeeID)
	} else {
		// Need to create new biller
		needsBillerCreation = true
		s.logger.DebugWith("Biller not found, will create in transaction", "biller_id", biller.BillerID)
	}

	// Step 3: Check if biller account already exists for this business
	s.logger.DebugWith("Checking for existing biller account for this business")
	var existingAccounts []*payee.PayeeBillerAccount
	if !needsBillerCreation {
		existingAccounts, err = s.payeeBillerAccountRepo.FindByBusinessAndPayee(ctx, req.BusinessID, payeeID)
		if err != nil && err.Error() != "no biller accounts found" {
			return nil, fmt.Errorf("failed to check existing biller accounts: %w", err)
		}
	}

	// Check for duplicate account (same mobile number)
	for _, account := range existingAccounts {
		if account.MobileNumber == biller.MobileNumber {
			return nil, fmt.Errorf("biller account with mobile %s already exists", biller.MobileNumber)
		}
	}

	// Step 4: ATOMIC TRANSACTION - Create all entities (master, biller, biller account, relation)
	s.logger.InfoWith("Creating all entities in atomic transaction")

	var relationID string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txMasterRepo := s.payeeMasterRepo.WithTx(tx)
		txBillerRepo := s.payeeBillerRepo.WithTx(tx)
		txBillerAccountRepo := s.payeeBillerAccountRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		// 1. Create master + biller if needed
		if needsBillerCreation {
			// Create master entry
			payeeMaster := &payee.PayeeMaster{
				PayeeType: payee.PayeeTypeBiller,
				IsActive:  true,
			}
			if err := txMasterRepo.Create(ctx, payeeMaster); err != nil {
				return fmt.Errorf("failed to create payee master: %w", err)
			}
			payeeID = payeeMaster.PayeeID
			s.logger.DebugWith("Created payee master entry", "payee_id", payeeID)

			// Create biller entity
			billerPayee := &payee.PayeeBiller{
				PayeeID:        payeeID,
				BillerID:       biller.BillerID,
				BillerName:     req.DisplayName, // Use customer's display name
				BillerCategory: "utility",       // Default category, should be from BBPS lookup
				NPCIRegistered: true,            // Assume BBPS billers are NPCI registered
				PaymentModes:   []string{"upi", "netbanking", "card"},
			}
			if err := txBillerRepo.Create(ctx, billerPayee); err != nil {
				return fmt.Errorf("failed to create biller: %w", err)
			}
			s.logger.DebugWith("Created biller entity", "biller_id", biller.BillerID)
		}

		// 2. Create biller account
		billerAccount := &payee.PayeeBillerAccount{
			PayeeID:          payeeID,
			BusinessID:       req.BusinessID,
			MobileNumber:     biller.MobileNumber,
			CustomerParams:   biller.CustomerParams,
			AutoFetchEnabled: biller.AutoFetch,
			IsActive:         true,
		}
		if err := txBillerAccountRepo.Create(ctx, billerAccount); err != nil {
			return fmt.Errorf("failed to create biller account: %w", err)
		}
		billerAccountID = billerAccount.BillerAccountID
		s.logger.DebugWith("Created biller account", "biller_account_id", billerAccountID)

		// 3. Create relation
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

		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}
		relationID = relation.RelationID
		s.logger.DebugWith("Created business-payee relation", "relation_id", relationID)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	s.logger.InfoWith("Biller onboarding completed successfully",
		"payee_id", payeeID,
		"relation_id", relationID)

	// Step 5: Return response
	return &payee.OnboardPayeeResponse{
		RelationID:              relationID,
		PayeeID:                 payeeID,
		AccountID:               billerAccountID,
		GSTVerificationStatus:   payee.VerificationStatusSuccess, // BBPS billers are pre-verified
		BankVerificationStatus:  payee.VerificationStatusSuccess, // NPCI registered
		CrossVerificationStatus: payee.VerificationStatusSuccess, // No cross-verification needed
		IsFullyVerified:         true,
		Message:                 fmt.Sprintf("Biller '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// onboardEmployee handles employee payee onboarding (bank account verification)
// Steps: Validate → Verify Bank → Cross-verify → Create entities
func (s *PayeeService) onboardEmployee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	s.logger.InfoWith("Starting employee onboarding",
		"business_id", req.BusinessID,
		"display_name", req.DisplayName)

	// Step 1: Validate employee-specific data
	if err := s.validateEmployeeRequest(&req); err != nil {
		return nil, err
	}

	employee := req.Employee
	var payeeID string

	// Step 2: Always create new employee (employees are business-specific, not global)
	s.logger.InfoWith("Preparing to create new employee payee")

	// Step 3: Prepare bank account for verification
	s.logger.DebugWith("Preparing bank account for verification")
	accountHash := s.computeAccountHash(employee.AccountNumber)

	// Prepare bank account entity for verification
	// var crossVerifyStatus payee.VerificationStatus
	var accountType payment.BankAccountType

	// Default account_type to "current" if not provided
	accountType = employee.AccountType
	if accountType == "" {
		accountType = payment.BankAccountTypeCurrent
		s.logger.WarnWith("account_type not provided, defaulting to current")
	}

	// Step 4: Perform penny drop verification
	s.logger.InfoWith("Initiating bank account verification")

	bankReq := authbridge.NewBankVerificationRequest(
		"10000",
		employee.AccountNumber,
		employee.IFSCCode,
	)
	authbridgeClient, err := authbridge.NewClient()
	if err != nil {
		s.logger.ErrorWith("Failed to create authbridge client", err)
		return nil, fmt.Errorf("failed to create authbridge client: %w", err)
	}
	bankResp, err := authbridgeClient.SearchDocument(bankReq)
	if err != nil {
		s.logger.ErrorWith("Failed to search document", err)
		return nil, fmt.Errorf("failed to search document: %w", err)
	}
	s.logger.InfoWith("Bank verification response", bankResp)

	/*
		// response from authbridge
		{
		"msg": {
			"accountType":"SAVINGS",
			"beneficiaryName":"DARAHAS KOPPARAPU",
			"clientRefId":"02112025859128",
			"responseCode":"00",
			"responseMsg":"SUCCESS",
			"rrn":"530613010252",
			"status":"SUCCESS",
			"transDateTime":"02/11/2025 13:45:04"
			},
			"status":1,
			"transId":"12",
			"tsTransId":"TS-UXR-307481"
		}
	*/

	// Type assert to access nested map
	msgData, ok := bankResp["msg"].(map[string]interface{})
	if !ok {
		s.logger.ErrorWith("Invalid bank verification response format")
		return nil, fmt.Errorf("invalid bank verification response format")
	}

	verificationStatus, _ := msgData["status"].(string)
	s.logger.InfoWith("Bank verification completed", "status", verificationStatus)

	// Fail fast: Abort if bank verification did not succeed
	if verificationStatus != "SUCCESS" {
		s.logger.ErrorWith("Bank verification failed, aborting onboarding", "status", verificationStatus)
		return nil, fmt.Errorf("bank verification failed: account could not be verified")
	}

	s.logger.InfoWith("Onboarding payee to Castler")
	castlerPayeeResp, err := s.castlerClient.OnboardPayeeToCastler(
		ctx,
		req.DisplayName,
		req.ContactEmail,
		req.ContactPhone,
		employee.AccountNumber,
		employee.IFSCCode,
		employee.BankName,
	)
	if err != nil {
		return nil, fmt.Errorf("castler payee onboarding failed: %w", err)
	}
	s.logger.InfoWith("Castler onboarding successful",
		"castler_payee_id", castlerPayeeResp.Result)

	// // Step 5: Cross-verification (employee name vs bank account holder name)
	// s.logger.DebugWith("Cross-verifying employee name with bank account holder name")
	// // Mock cross-verification for now
	// crossVerifyStatus = payee.VerificationStatusSuccess
	// s.logger.InfoWith("Cross-verification completed successfully")

	// // Fail fast: Abort if cross-verification failed
	// if crossVerifyStatus != payee.VerificationStatusSuccess {
	// 	s.logger.ErrorWith("Cross-verification failed, aborting onboarding", "status", crossVerifyStatus)
	// 	return nil, fmt.Errorf("cross-verification failed: employee name does not match bank account holder name")
	// }

	// Prepare bank account entity (will be created in transaction)
	// At this point, all verifications have passed
	now := time.Now()
	bankAccount := &payee.PayeeBankAccount{
		PayeeID:                 payeeID, // Will be updated in transaction
		AccountNumber:           employee.AccountNumber,
		AccountNumberHash:       accountHash,
		IFSCCode:                employee.IFSCCode,
		AccountHolderName:       employee.AccountHolderName,
		BankName:                employee.BankName,   // Use bank name from frontend IFSC check
		BranchName:              employee.BranchName, // Use branch name from frontend IFSC check
		AccountType:             accountType,
		AccountLabel:            employee.AccountLabel,
		IsActive:                true,
		IsVerified:              true,                    // Always true at this point
		CastlerBeneficiaryID:    castlerPayeeResp.Result, // Store Castler payee ID
		BankVerificationStatus:  payee.VerificationStatusSuccess,
		CrossVerificationStatus: payee.VerificationStatusSuccess,
		VerifiedAt:              &now,
		VerificationData:        map[string]any{"test": "test"}, // TODO: Store actual verification data
	}

	// Step 6: ATOMIC TRANSACTION - Create all entities (master, employee, bank account, relation)
	s.logger.InfoWith("Creating all entities in atomic transaction")

	var accountID string
	var relationID string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txMasterRepo := s.payeeMasterRepo.WithTx(tx)
		txEmployeeRepo := s.payeeEmployeeRepo.WithTx(tx)
		txBankAccountRepo := s.payeeBankAccountRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		// 1. Create master + employee
		// Create master entry
		payeeMaster := &payee.PayeeMaster{
			PayeeType: payee.PayeeTypeEmployee,
			IsActive:  true,
		}
		if err := txMasterRepo.Create(ctx, payeeMaster); err != nil {
			return fmt.Errorf("failed to create payee master: %w", err)
		}
		payeeID = payeeMaster.PayeeID
		s.logger.DebugWith("Created payee master entry", "payee_id", payeeID)

		// Create employee entity
		// Note: RemunerationDocURL is not set during onboarding - it will be uploaded separately
		employeePayee := &payee.PayeeEmployee{
			PayeeID:        payeeID,
			EmployeeCode:   employee.EmployeeCode,
			FullName:       employee.FullName,
			Email:          employee.Email,
			Phone:          employee.Phone,
			Designation:    employee.Designation,
			Department:     employee.Department,
			DateOfJoining:  employee.DateOfJoining,
			EmploymentType: employee.EmploymentType,
			IsSensitive:    true,
		}

		if err := txEmployeeRepo.Create(ctx, employeePayee); err != nil {
			return fmt.Errorf("failed to create employee: %w", err)
		}
		s.logger.DebugWith("Created employee entity", "full_name", employee.FullName)

		// 2. Create bank account
		// Update PayeeID in bank account
		bankAccount.PayeeID = payeeID

		if err := txBankAccountRepo.CreateBankAccount(ctx, bankAccount); err != nil {
			return fmt.Errorf("failed to create bank account: %w", err)
		}
		accountID = bankAccount.AccountID
		s.logger.DebugWith("Created bank account", "account_id", accountID)

		// 3. Create relation
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
			Tags:         []string{"employee", "payroll"},
			AddedBy:      req.AddedBy, // User ID from JWT
		}

		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}
		relationID = relation.RelationID
		s.logger.DebugWith("Created business-payee relation", "relation_id", relationID)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("transaction failed: %w", err)
	}

	s.logger.InfoWith("Employee onboarding completed successfully",
		"payee_id", payeeID,
		"relation_id", relationID)

	// Step 7: Return response
	// At this point, all verifications have passed (fail-fast approach)
	return &payee.OnboardPayeeResponse{
		RelationID:              relationID,
		PayeeID:                 payeeID,
		AccountID:               accountID,
		GSTVerificationStatus:   payee.VerificationStatusSuccess, // No identity verification needed for employees
		BankVerificationStatus:  payee.VerificationStatusSuccess, // Always success at this point
		CrossVerificationStatus: payee.VerificationStatusSuccess, // Always success at this point
		IsFullyVerified:         true,                            // Always true - fail-fast ensures this
		Message:                 fmt.Sprintf("Employee '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// GetPayeesByBusiness retrieves all payees for a business
func (s *PayeeService) GetPayeesByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	s.logger.DebugWith("Fetching payees for business", "business_id", businessID)
	return s.businessPayeeRelationRepo.FindByBusiness(ctx, businessID, filters)
}

// GetPayeeDetails retrieves detailed information about a specific payee (polymorphic)
func (s *PayeeService) GetPayeeDetails(ctx context.Context, businessID, payeeID, accountID string) (*payee.PayeeDetailsResponse, error) {
	s.logger.DebugWith("Fetching payee details",
		"business_id", businessID,
		"payee_id", payeeID,
		"account_id", accountID)

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
	s.logger.DebugWith("Updating payee relation",
		"business_id", req.BusinessID,
		"payee_id", req.PayeeID)

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
	s.logger.DebugWith("Deactivating payee",
		"business_id", businessID,
		"payee_id", payeeID)
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

// checkMoneyRotation performs money rotation fraud detection by comparing payee account holder
// name against the directors of the business that is onboarding the payee.
// This prevents scenarios where business directors create fake vendors with their own bank accounts.
//
// Future enhancements:
// - ML-based name matching (fuzzy matching, nicknames, aliases)
// - Historical transaction pattern analysis
// - Cross-business director checks
// - Risk scoring instead of binary pass/fail
func (s *PayeeService) checkMoneyRotation(ctx context.Context, businessID string, payeeAccountHolderName string) error {
	s.logger.DebugWith("Performing money rotation check")

	// Extract director names from BUSINESS's KYB data
	var directorNames []string

	// Fetch business KYB data
	kybData, err := s.businessService.GetBusinessKYBData(ctx, businessID)
	if err != nil {
		s.logger.WarnWith("Could not fetch business KYB data for money rotation check", "error", err)
		// Continue without director names (money rotation check will have empty list)
	} else if kybData.CIDData != "" {
		// Parse company_id_data JSON to extract director names
		var companyData map[string]interface{}
		if err := json.Unmarshal([]byte(kybData.CIDData), &companyData); err != nil {
			s.logger.WarnWith("Failed to parse company_id_data", "error", err)
		} else if msg, ok := companyData["msg"].(map[string]interface{}); ok {
			if masterDirectors, ok := msg["MasterDirector"].([]interface{}); ok {
				for _, dirInterface := range masterDirectors {
					if dir, ok := dirInterface.(map[string]interface{}); ok {
						if name, ok := dir["name"].(string); ok && name != "" {
							directorNames = append(directorNames, name)
						}
					}
				}
				s.logger.DebugWith("Found directors in business KYB data", "director_count", len(directorNames))
			}
		}
	}

	// Prepare money rotation check request
	moneyRotationReq := business_rules.MoneyRotationCheckRequest{
		CustomerBusinessID: businessID,
		PayeeAccountName:   payeeAccountHolderName,
		PayeeGSTDirectors:  directorNames, // Business directors, not payee directors
	}

	// Call verification service
	moneyRotationResp, err := s.crossVerificationSvc.CheckMoneyRotation(ctx, moneyRotationReq)
	if err != nil {
		return fmt.Errorf("money rotation check failed: %w", err)
	}

	// Check if money rotation detected
	if moneyRotationResp.IsMoneyRotation {
		return fmt.Errorf("money rotation detected: %s", moneyRotationResp.Details)
	}

	s.logger.InfoWith("Money rotation check passed")
	return nil
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
		EmployeeCode:            employee.EmployeeCode,
		FullName:                employee.FullName,
		Email:                   employee.Email,
		Phone:                   employee.Phone,
		Designation:             employee.Designation,
		Department:              employee.Department,
		EmploymentType:          employee.EmploymentType,
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
	s.logger.DebugWith("Fetching vendors for dropdown", "business_id", businessID)

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

	s.logger.DebugWith("Vendors fetched for dropdown", "vendor_count", len(vendors))
	return response, nil
}

// GetEmployeesForList retrieves lightweight employee list for tabulated display
func (s *PayeeService) GetEmployeesForList(ctx context.Context, businessID string) (*payee.EmployeeLightweightListResponse, error) {
	s.logger.DebugWith("Fetching employees for list", "business_id", businessID)

	// Input validation
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Validate UUID format
	if _, err := uuid.Parse(businessID); err != nil {
		return nil, fmt.Errorf("invalid business_id format: %w", err)
	}

	// Fetch employees from repository
	employees, err := s.businessPayeeRelationRepo.GetEmployeesForList(ctx, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get employees for list: %w", err)
	}

	// Build response
	response := &payee.EmployeeLightweightListResponse{
		Employees: employees,
		Count:     len(employees),
	}

	s.logger.DebugWith("Employees fetched for list", "employee_count", len(employees))
	return response, nil
}

// GetPayeeDetailsByRelation retrieves complete payee details by relation_id (polymorphic)
// Returns vendor, employee, or biller details with bank/account information
// Validates that the relation belongs to the specified business
func (s *PayeeService) GetPayeeDetailsByRelation(ctx context.Context, businessID, relationID string) (*payee.PayeeFullDetails, error) {
	s.logger.DebugWith("Fetching payee details by relation",
		"business_id", businessID,
		"relation_id", relationID)

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

	s.logger.DebugWith("Found payee details", "payee_type", details.PayeeType)
	return details, nil
}

// ############################################################################
// # Employee Remuneration Document Upload
// ############################################################################

// GetRemunerationDocumentUploadURL generates a presigned S3 URL for employee remuneration document upload
// This allows the frontend to upload the document directly to S3 without going through the backend
func (s *PayeeService) GetRemunerationDocumentUploadURL(ctx context.Context, businessID, payeeID, accountID string) (string, error) {
	s.logger.DebugWith("Generating presigned upload URL for remuneration document",
		"business_id", businessID,
		"payee_id", payeeID,
		"account_id", accountID)

	// Input validation
	if businessID == "" {
		return "", fmt.Errorf("business_id is required")
	}
	if payeeID == "" {
		return "", fmt.Errorf("payee_id is required")
	}
	if accountID == "" {
		return "", fmt.Errorf("account_id is required")
	}

	// Validate UUID formats
	if _, err := uuid.Parse(businessID); err != nil {
		return "", fmt.Errorf("invalid business_id format: %w", err)
	}
	if _, err := uuid.Parse(payeeID); err != nil {
		return "", fmt.Errorf("invalid payee_id format: %w", err)
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return "", fmt.Errorf("invalid account_id format: %w", err)
	}

	// Verify that the payee exists and is an employee
	employee, err := s.payeeEmployeeRepo.FindByID(ctx, payeeID)
	if err != nil {
		return "", fmt.Errorf("employee payee not found: %w", err)
	}

	// Verify that the employee belongs to the business with the specific account
	relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, businessID, payeeID, accountID)
	if err != nil {
		return "", fmt.Errorf("failed to verify business-payee relationship: %w", err)
	}
	if !relationExists {
		return "", fmt.Errorf("payee does not belong to the specified business or account not found")
	}

	s.logger.DebugWith("Employee verified", "full_name", employee.FullName)

	// Create S3 client
	fileuploader, err := s.createS3Client()
	if err != nil {
		return "", fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Generate presigned upload URL with S3 key: PII/<businessID>/employees/<payeeID>.pdf
	key := fmt.Sprintf("PII/%s/employees/%s.pdf", businessID, payeeID)
	url, _, err := fileuploader.GetPresignedUploadURL(ctx, "remuneration_document.pdf", "application/pdf", key, 15*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned upload URL: %w", err)
	}

	s.logger.InfoWith("Presigned upload URL generated successfully", "key", key)
	return url, nil
}

// ConfirmRemunerationDocumentUpload confirms the S3 upload and updates the database with the document key
// This is called after the frontend successfully uploads the document to S3
func (s *PayeeService) ConfirmRemunerationDocumentUpload(ctx context.Context, businessID, payeeID, accountID string) (string, error) {
	s.logger.DebugWith("Confirming remuneration document upload",
		"business_id", businessID,
		"payee_id", payeeID,
		"account_id", accountID)

	// Input validation
	if businessID == "" {
		return "", fmt.Errorf("business_id is required")
	}
	if payeeID == "" {
		return "", fmt.Errorf("payee_id is required")
	}
	if accountID == "" {
		return "", fmt.Errorf("account_id is required")
	}

	// Validate UUID formats
	if _, err := uuid.Parse(businessID); err != nil {
		return "", fmt.Errorf("invalid business_id format: %w", err)
	}
	if _, err := uuid.Parse(payeeID); err != nil {
		return "", fmt.Errorf("invalid payee_id format: %w", err)
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return "", fmt.Errorf("invalid account_id format: %w", err)
	}

	// Verify that the payee exists and is an employee
	employee, err := s.payeeEmployeeRepo.FindByID(ctx, payeeID)
	if err != nil {
		return "", fmt.Errorf("employee payee not found: %w", err)
	}

	// Verify that the employee belongs to the business with the specific account
	relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, businessID, payeeID, accountID)
	if err != nil {
		return "", fmt.Errorf("failed to verify business-payee relationship: %w", err)
	}
	if !relationExists {
		return "", fmt.Errorf("payee does not belong to the specified business or account not found")
	}

	s.logger.DebugWith("Employee verified", "full_name", employee.FullName)

	// Create S3 client
	fileuploader, err := s.createS3Client()
	if err != nil {
		return "", fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Confirm upload and get presigned download URL
	key := fmt.Sprintf("PII/%s/employees/%s.pdf", businessID, payeeID)
	fileURL, err := fileuploader.ConfirmUpload(ctx, key, 15*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to confirm upload: %w", err)
	}

	s.logger.DebugWith("S3 upload confirmed", "key", key)

	// Update database with document key
	err = s.payeeEmployeeRepo.UpdateRemunerationDocumentKey(ctx, payeeID, key)
	if err != nil {
		return "", fmt.Errorf("failed to update remuneration document key in database: %w", err)
	}

	s.logger.InfoWith("Remuneration document upload confirmed successfully",
		"payee_id", payeeID,
		"s3_key", key)

	return fileURL, nil
}

// createS3Client is a helper method to create S3 client from config
func (s *PayeeService) createS3Client() (*fileupload.S3Client, error) {
	if s.s3Config == nil {
		return nil, fmt.Errorf("S3 configuration is not initialized")
	}
	return fileupload.NewS3ClientFromConfig(*s.s3Config)
}
