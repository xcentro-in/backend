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
type PayeeService struct {
	payeeBusinessRepo         payee.PayeeBusinessRepository
	payeeBankAccountRepo      payee.PayeeBankAccountRepository
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository
	gstVerificationSvc        business_rules.GSTVerificationService
	bankVerificationSvc       business_rules.BankVerificationService
	crossVerificationSvc      business_rules.CrossVerificationService
	txManager                 client.TxManager
}

// NewPayeeService creates a new payee service with all dependencies
func NewPayeeService(
	payeeBusinessRepo payee.PayeeBusinessRepository,
	payeeBankAccountRepo payee.PayeeBankAccountRepository,
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository,
	gstVerificationSvc business_rules.GSTVerificationService,
	bankVerificationSvc business_rules.BankVerificationService,
	crossVerificationSvc business_rules.CrossVerificationService,
	txManager client.TxManager,
) *PayeeService {
	return &PayeeService{
		payeeBusinessRepo:         payeeBusinessRepo,
		payeeBankAccountRepo:      payeeBankAccountRepo,
		businessPayeeRelationRepo: businessPayeeRelationRepo,
		gstVerificationSvc:        gstVerificationSvc,
		bankVerificationSvc:       bankVerificationSvc,
		crossVerificationSvc:      crossVerificationSvc,
		txManager:                 txManager,
	}
}

// OnboardPayee orchestrates the complete payee onboarding flow
// Steps: Validate → Check duplicates → Verify GST → Verify Bank → Cross-verify → Money rotation check → Create entities
func (s *PayeeService) OnboardPayee(ctx context.Context, req payee.OnboardPayeeRequest) (*payee.OnboardPayeeResponse, error) {
	fmt.Println("=== PAYEE SERVICE: OnboardPayee Called ===")
	fmt.Printf("Business ID: %s, GSTIN: %s, Display Name: %s\n", req.BusinessID, req.GSTIN, req.DisplayName)
	fmt.Printf("Account Number: ****%s, IFSC: %s\n", req.AccountNumber[len(req.AccountNumber)-4:], req.IFSCCode)

	// Step 1: Validate input
	fmt.Println("✅ Step 1: Validating input...")
	if err := s.validateOnboardRequest(&req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Step 2: Check if payee already exists by GSTIN
	fmt.Println("🔍 Step 2: Checking if payee already exists (by GSTIN)...")
	existingPayee, err := s.payeeBusinessRepo.FindByGSTIN(ctx, req.GSTIN)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing payee: %w", err)
	}

	var payeeBusiness *payee.PayeeBusiness

	// Step 3: If payee doesn't exist, verify GST and create
	if existingPayee == nil {
		fmt.Println("🆕 Step 3: Payee not found, verifying GST and creating new payee business...")

		gstReq := business_rules.GSTVerificationRequest{
			GSTIN:     req.GSTIN,
			RequestID: fmt.Sprintf("onboard_%s_%d", req.BusinessID, time.Now().Unix()),
		}

		gstResp, err := s.gstVerificationSvc.VerifyGSTIN(ctx, gstReq)
		if err != nil {
			return nil, fmt.Errorf("GST verification failed: %w", err)
		}

		fmt.Printf("✅ GST verified: %s (%s)\n", gstResp.LegalName, gstResp.Status)

		// Create PayeeBusiness entity
		payeeBusiness = &payee.PayeeBusiness{
			GSTIN:                 req.GSTIN,
			LegalName:             gstResp.LegalName,
			BusinessType:          gstResp.BusinessType,
			RegistrationDate:      &gstResp.RegistrationDate,
			GSTStatus:             gstResp.Status,
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
		*payeeBusiness.GSTVerifiedAt = time.Now()

		if err := s.payeeBusinessRepo.CreatePayeeBusiness(ctx, payeeBusiness); err != nil {
			return nil, fmt.Errorf("failed to create payee business: %w", err)
		}

		fmt.Printf("✅ Payee business created with ID: %s\n", payeeBusiness.PayeeID)
	} else {
		fmt.Printf("♻️  Step 3: Payee already exists (PayeeID: %s), reusing...\n", existingPayee.PayeeID)
		payeeBusiness = existingPayee
	}

	// Step 4: Check if bank account already exists for this payee
	fmt.Println("🔍 Step 4: Checking for duplicate bank account...")
	accountHash := s.computeAccountHash(req.AccountNumber)
	isDuplicate, err := s.payeeBankAccountRepo.CheckDuplicateAccount(ctx, payeeBusiness.PayeeID, accountHash)
	if err != nil {
		return nil, fmt.Errorf("failed to check duplicate account: %w", err)
	}

	var bankAccount *payee.PayeeBankAccount

	if isDuplicate {
		fmt.Println("♻️  Bank account already exists for this payee, fetching...")
		// Find the existing account
		accounts, err := s.payeeBankAccountRepo.FindByPayeeID(ctx, payeeBusiness.PayeeID)
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
			AccountNumber:     req.AccountNumber,
			IFSCCode:          req.IFSCCode,
			AccountHolderName: req.AccountHolderName,
			BankName:          req.BankName,
			AccountType:       req.AccountType,
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
			GSTIN:           req.GSTIN,
			GSTLegalName:    payeeBusiness.LegalName,
			BankAccountName: bankResp.VerifiedAccountName,
			BankIFSC:        req.IFSCCode,
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
		if payeeBusiness.GSTData != nil {
			if directors, ok := payeeBusiness.GSTData["directors"].([]business_rules.GSTDirector); ok {
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
			PayeeID:                  payeeBusiness.PayeeID,
			AccountNumber:            req.AccountNumber,
			AccountNumberHash:        accountHash,
			IFSCCode:                 req.IFSCCode,
			BankName:                 bankResp.BankName,
			BranchName:               bankResp.BranchName,
			AccountType:              req.AccountType,
			AccountHolderName:        bankResp.VerifiedAccountName,
			IsVerified:               true,
			VerifiedAt:               &time.Time{},
			BankVerificationStatus:   payee.VerificationStatusSuccess,
			CrossVerificationStatus:  payee.VerificationStatusSuccess,
			NameMatchScore:           crossResp.MatchScore,
			IsActive:                 true,
			IsPrimary:                false, // Will check if this should be primary
			AccountLabel:             req.AccountLabel,
			VerificationData:         bankResp.Metadata,
		}
		*bankAccount.VerifiedAt = time.Now()
	}

	// Step 8: Check if this should be the primary account
	if bankAccount.AccountID == "" { // New account being created
		existingAccounts, _ := s.payeeBankAccountRepo.FindByPayeeID(ctx, payeeBusiness.PayeeID)
		if len(existingAccounts) == 0 {
			bankAccount.IsPrimary = true
			fmt.Println("📌 Setting as primary account (first account for this payee)")
		}
	}

	// Step 9: Check if relation already exists
	fmt.Println("🔍 Step 9: Checking if customer already has this payee-account relation...")
	if bankAccount.AccountID != "" { // Account exists
		relationExists, err := s.businessPayeeRelationRepo.RelationExists(ctx, req.BusinessID, payeeBusiness.PayeeID, bankAccount.AccountID)
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
			PayeeID:      payeeBusiness.PayeeID,
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
		PayeeID:                 payeeBusiness.PayeeID,
		AccountID:               accountID,
		GSTVerificationStatus:   payeeBusiness.GSTVerificationStatus,
		BankVerificationStatus:  bankAccount.BankVerificationStatus,
		CrossVerificationStatus: bankAccount.CrossVerificationStatus,
		IsFullyVerified:         true,
		Message:                 fmt.Sprintf("Payee '%s' successfully onboarded", req.DisplayName),
	}, nil
}

// GetPayeesByBusiness retrieves all payees for a business
func (s *PayeeService) GetPayeesByBusiness(ctx context.Context, businessID string, filters payee.RelationSearchFilters) ([]*payee.BusinessPayeeRelation, error) {
	fmt.Printf("[PayeeService] GetPayeesByBusiness called - BusinessID: %s\n", businessID)
	return s.businessPayeeRelationRepo.FindByBusiness(ctx, businessID, filters)
}

// GetPayeeDetails retrieves detailed information about a specific payee
func (s *PayeeService) GetPayeeDetails(ctx context.Context, businessID, payeeID, accountID string) (*payee.PayeeDetailsResponse, error) {
	fmt.Printf("[PayeeService] GetPayeeDetails called - BusinessID: %s, PayeeID: %s, AccountID: %s\n",
		businessID, payeeID, accountID)

	// Get relation
	relation, err := s.businessPayeeRelationRepo.FindRelation(ctx, businessID, payeeID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to find relation: %w", err)
	}

	// Get payee business
	payeeBusiness, err := s.payeeBusinessRepo.FindByID(ctx, payeeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find payee business: %w", err)
	}

	// Get bank account
	bankAccount, err := s.payeeBankAccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to find bank account: %w", err)
	}

	// Build response
	return &payee.PayeeDetailsResponse{
		PayeeID:                 payeeBusiness.PayeeID,
		GSTIN:                   payeeBusiness.GSTIN,
		LegalName:               payeeBusiness.LegalName,
		BusinessType:            payeeBusiness.BusinessType,
		GSTStatus:               payeeBusiness.GSTStatus,
		AccountID:               bankAccount.AccountID,
		MaskedAccount:           s.maskAccountNumber(bankAccount.AccountNumber),
		BankName:                bankAccount.BankName,
		IFSCCode:                bankAccount.IFSCCode,
		AccountType:             string(bankAccount.AccountType),
		AccountLabel:            bankAccount.AccountLabel,
		IsVerified:              bankAccount.IsVerified,
		DisplayName:             relation.DisplayName,
		ContactEmail:            relation.ContactEmail,
		ContactPhone:            relation.ContactPhone,
		Notes:                   relation.Notes,
		IsFavorite:              relation.IsFavorite,
		Tags:                    relation.Tags,
		GSTVerificationStatus:   string(payeeBusiness.GSTVerificationStatus),
		BankVerificationStatus:  string(bankAccount.BankVerificationStatus),
		CrossVerificationStatus: string(bankAccount.CrossVerificationStatus),
		AddedAt:                 relation.AddedAt,
		LastUsedAt:              relation.LastUsedAt,
	}, nil
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

func (s *PayeeService) validateOnboardRequest(req *payee.OnboardPayeeRequest) error {
	if req.BusinessID == "" {
		return fmt.Errorf("business_id is required")
	}
	if req.GSTIN == "" || len(req.GSTIN) != 15 {
		return fmt.Errorf("invalid GSTIN (must be 15 characters)")
	}
	if req.AccountNumber == "" {
		return fmt.Errorf("account_number is required")
	}
	if req.IFSCCode == "" || len(req.IFSCCode) != 11 {
		return fmt.Errorf("invalid IFSC code (must be 11 characters)")
	}
	if req.AccountHolderName == "" {
		return fmt.Errorf("account_holder_name is required")
	}
	if req.DisplayName == "" {
		return fmt.Errorf("display_name is required")
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
