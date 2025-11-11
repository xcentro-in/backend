package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/auth"
	businessRepo "xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/services/verification"
	sharedClients "xcentro.in/internal/shared/clients"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
)

// UserServiceImpl provides business logic for user operations
type UserServiceImpl struct {
	userRepo            user.UserRepository
	businessRepo        businessRepo.BusinessRepository
	businessUserRelRepo businessRepo.BusinessUserRelationRepository
	txManager           client.TxManager
	emailService        mailers.EmailService
	verificationService *verification.DocumentVerificationService
	config              *config.Config
	rclient             *redisClient.RedisClient
	logger              *logger.Logger
	// metrics can be added here when needed: metrics metrics.MetricsProvider
}

// NewUserService creates a new user service with dependency injection
// Accept repositories directly (already created), clients for extracting what we need, and config
func NewUserService(
	userRepo *userRepo.PgUserRepository,
	businessRepo businessRepo.BusinessRepository,
	businessUserRelRepo businessRepo.BusinessUserRelationRepository,
	verificationService *verification.DocumentVerificationService,
	clients sharedClients.ExternalClients, // Extract needed clients from here
	cfg *config.Config,
) user.UserService {
	return &UserServiceImpl{
		userRepo:            userRepo,
		businessRepo:        businessRepo,
		businessUserRelRepo: businessUserRelRepo,
		txManager:           clients.GetTxManager(),    // Extract from clients
		emailService:        clients.GetEmailService(), // Extract from clients
		verificationService: verificationService,
		config:              cfg,
		rclient:             clients.GetRedisClient(), // Extract from clients
		logger:              logger.GetDefault(),
		// When needed: metrics: clients.GetMetrics().(metrics.MetricsProvider),
	}
}

// maskAadhaar masks Aadhaar number, showing only last 4 digits
// Example: "123456789012" -> "********9012"
func maskAadhaar(aadhaar string) string {
	if len(aadhaar) <= 4 {
		return aadhaar // Don't mask if too short
	}
	lastFour := aadhaar[len(aadhaar)-4:]
	masked := ""
	for i := 0; i < len(aadhaar)-4; i++ {
		masked += "*"
	}
	return masked + lastFour
}

// CreateUser creates a new user with proper validation and transaction handling
func (s *UserServiceImpl) CreateUser(ctx context.Context, authPayload auth.AuthPayload) (*user.User, error) {
	s.logger.Info("Creating user with email: %s", authPayload.Email)

	// Validate input
	if authPayload.Email == "" {
		return nil, fmt.Errorf("email is required")
	}

	var createdUser *user.User

	// Use transaction to ensure data consistency
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create repository instance that uses the transaction
		txUserRepo := s.userRepo.WithTx(tx)

		// Check if user already exists
		existingUser, err := txUserRepo.GetUserByEmail(ctx, authPayload.Email)
		if err == nil && existingUser != nil {
			s.logger.Errorf("user with email %s already exists", authPayload.Email)
			return types.ErrUserAlreadyExists
		}

		if err != nil {
			s.logger.Errorf("failed to check existing user: %v", err)
			return err
		}

		// Create new user
		createdUser, err = txUserRepo.CreateUser(ctx, authPayload)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		s.logger.Info("User created successfully with ID: %s", createdUser.Id)
		return nil
	})

	if err != nil {
		s.logger.Error("Failed to create user: %v", err)
		return nil, err
	}

	return createdUser, nil
}

// GetUserByEmail retrieves a user by email (simple DB fetch - email column has index)
func (s *UserServiceImpl) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	s.logger.Info("Getting user by email: %s", email)

	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	userObj, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		s.logger.Error("Failed to get user by email: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return userObj, nil
}

// GetUserByID retrieves a user by ID (simple DB fetch)
func (s *UserServiceImpl) GetUserByID(ctx context.Context, userID string) (*user.User, error) {
	s.logger.Info("Getting user by ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	userFromDB, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user by ID: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return userFromDB, nil
}

// GetUserProfile retrieves user profile with masked national ID (cached)
func (s *UserServiceImpl) GetUserProfile(ctx context.Context, userID string) (*user.UserProfileDTO, error) {
	s.logger.Info("Getting user profile for ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Try cache first
	cacheKey := keys.User.Profile(userID)
	var cachedProfile user.UserProfileDTO
	err := s.rclient.GetJSON(ctx, cacheKey, &cachedProfile)

	if err == nil {
		// Cache hit
		s.logger.Info("User profile cache hit for ID: %s", userID)
		return &cachedProfile, nil
	}

	// Cache miss - fetch from database
	userData, identityData, err := s.userRepo.GetUserKYCStatus(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user profile: %v", err)
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Create profile DTO
	profile := &user.UserProfileDTO{
		UserID:           userData.Id,
		Email:            userData.Email,
		Name:             userData.Name,
		DOB:              userData.DOB,
		Mobile:           userData.Mobile,
		TaxID:            "",
		MaskedNationalID: "",
	}

	// Add identity data if available
	if identityData != nil {
		profile.TaxID = identityData.TaxID
		// National ID is already masked at DB level - no need to re-mask
		profile.MaskedNationalID = identityData.NationalID
	}

	// Cache profile asynchronously (fire-and-forget)
	go func() {
		bgCtx := context.Background()
		if err := s.rclient.SetJSON(bgCtx, cacheKey, profile, keys.TTLUserProfile); err != nil {
			s.logger.Warnf("Failed to cache user profile: %v", err)
		}
	}()

	return profile, nil
}

// UpdateUserProfile updates user profile information in a transaction (mobile only)
func (s *UserServiceImpl) UpdateUserProfile(ctx context.Context, userID string, mobile string) error {
	s.logger.Info("Updating user profile for ID: %s", userID)

	if mobile == "" {
		return fmt.Errorf("mobile is required")
	}

	err := s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if err := txUserRepo.UpdateUserMobile(ctx, userID, mobile); err != nil {
			return fmt.Errorf("failed to update mobile: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate only the user profile cache, as roles are not affected.
	go func() {
		bgCtx := context.Background()
		invalidationKey := keys.User.Profile(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKey); err != nil {
			s.logger.Warnf("Failed to invalidate user profile cache: %v", err)
		}
	}()
	return nil
}

// UpdateKYCStatus updates user's KYC status
func (s *UserServiceImpl) UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus) error {
	s.logger.Info("Updating KYC status for user ID: %s to status: %v", userID, status)

	err := s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, status); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate only the user profile cache, as roles are not affected by a KYC status change.
	go func() {
		bgCtx := context.Background()
		invalidationKey := keys.User.Profile(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKey); err != nil {
			s.logger.Warnf("Failed to invalidate user profile cache: %v", err)
		}
	}()

	return nil
}

// InitiateOnboarding initiates user onboarding with tax ID and national ID verification
// User KYC is independent of business - this only verifies identity
func (s *UserServiceImpl) InitiateOnboarding(
	ctx context.Context,
	userID, name, dob, taxID, nationalID string,
) (*user.InitiateOnboardingResponseDTO, error) {
	s.logger.Info("Initiating onboarding for user %s", userID)

	// ============================================================================
	// PHASE 1: Validation and External API Calls (OUTSIDE TRANSACTION)
	// ============================================================================

	// Step 1: Input validation
	if userID == "" {
		return nil, types.NewInvalidKYCInputError("userID", "required")
	}
	if name == "" || dob == "" || taxID == "" || nationalID == "" {
		return nil, types.NewInvalidKYCInputError("user details", "name, dob, taxID, and nationalID are required")
	}

	// Step 2: Get current user (non-critical read - outside transaction)
	currentUser, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, types.ErrUserNotFound
	}

	// Step 3: Check if KYC already completed or verified
	if currentUser.KycStatus == shared.KYCCompleted || currentUser.KycStatus == shared.KYCVerified {
		return nil, types.ErrKYCAlreadyCompleted
	}

	// Step 4: Check if PAN already exists and reuse verification data (SMART OPTIMIZATION)
	// If PAN exists with valid data, reuse PANCOMData; otherwise call verification API
	var panComResultMap map[string]interface{}
	existingIdentity, err := s.userRepo.GetUserIdentity(ctx, taxID)

	// Check if we can reuse existing PAN verification data
	canReusePAN := false
	if err == nil && existingIdentity != nil && existingIdentity.TaxIDData != "" && existingIdentity.TaxIDData != "{}" {
		// PAN exists with data - try to extract PANCOMData
		var existingTaxIDData user.UserTaxIDVerificationData
		if err := json.Unmarshal([]byte(existingIdentity.TaxIDData), &existingTaxIDData); err == nil {
			if len(existingTaxIDData.PANCOMData) > 0 {
				// Valid PANCOMData found - reuse it
				s.logger.Infof("PAN %s already exists with valid data, reusing existing verification data", taxID)
				panComResultMap = existingTaxIDData.PANCOMData
				canReusePAN = true
			}
		}
	}

	// If we can't reuse, call verification API
	if !canReusePAN {
		s.logger.Infof("PAN %s not found or has no valid data, calling verification API", taxID)
		panComTxnID := fmt.Sprintf("pancom_%s_%d", userID, time.Now().Unix())
		panComResult, err := s.verificationService.VerifyPANCOM(ctx, taxID, panComTxnID)
		if err != nil {
			s.logger.Warnf("PAN verification failed for user %s: %v (proceeding with empty data)", userID, err)
			panComResultMap = map[string]interface{}{} // Empty JSON on failure
		} else {
			panComResultMap = panComResult.PANCOMResponse
		}
	}

	// Step 5: Verify PAN-Aadhaar link (EXTERNAL API - do BEFORE transaction)
	// NOTE: PAN-Aadhaar verification is optional - if it fails, we proceed with empty data
	s.logger.Info("Verifying Tax ID-National ID link for user %s", userID)
	panAadhaarTxnID := fmt.Sprintf("panaadhaar_%s_%d", userID, time.Now().Unix())
	var panAadhaarResultMap map[string]interface{}
	panAadhaarResult, err := s.verificationService.VerifyPANAadhaar(ctx, taxID, nationalID, panAadhaarTxnID)
	if err != nil {
		s.logger.Warnf("PAN-Aadhaar link verification failed for user %s: %v (proceeding with empty data)", userID, err)
		panAadhaarResultMap = map[string]interface{}{} // Empty JSON on failure
	} else {
		panAadhaarResultMap = panAadhaarResult
	}

	// Step 6: Combine verification data
	taxIDVerificationData := &user.UserTaxIDVerificationData{
		PANCOMData:     panComResultMap,
		PANAadhaarLink: panAadhaarResultMap,
	}

	taxIDDataJSON, err := json.Marshal(taxIDVerificationData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tax ID verification data: %w", err)
	}

	// Step 7: Generate Aadhaar DigiLocker link (EXTERNAL API - do BEFORE transaction)
	s.logger.Info("Generating Aadhaar verification link for user %s", userID)

	authClient, err := authbridge.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create authentication client: %w", err)
	}

	aadhaarTxnID := fmt.Sprintf("aadhaar_%s_%d", userID, time.Now().Unix())
	webhookURL := fmt.Sprintf("%s/v1/user/onboard/aadhaar/webhook", s.config.Server.URL)
	redirectURL := fmt.Sprintf("%s/v1/user/onboard/aadhaar/callback", s.config.Server.URL)
	linkRequest := authbridge.NewAadhaarLinkRequest(aadhaarTxnID, webhookURL, redirectURL)

	aadhaarResponse, err := authClient.SearchDocument(linkRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Aadhaar link: %w", err)
	}

	// Step 8: Extract URL and transaction ID from response
	status, ok := aadhaarResponse["status"].(float64)
	if !ok || status != 1 {
		return nil, fmt.Errorf("aadhaar link generation failed: invalid response status")
	}

	data, ok := aadhaarResponse["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid aadhaar response format: missing data")
	}

	aadhaarURL, ok := data["url"].(string)
	if !ok || aadhaarURL == "" {
		return nil, fmt.Errorf("invalid aadhaar response format: missing or empty URL")
	}

	tsTransID, ok := aadhaarResponse["ts_trans_id"].(string)
	if !ok || tsTransID == "" {
		return nil, fmt.Errorf("invalid aadhaar response format: missing transaction ID")
	}

	// ============================================================================
	// PHASE 2: Database Operations (INSIDE TRANSACTION)
	// ============================================================================

	var response *user.InitiateOnboardingResponseDTO

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Step 9: Create or update user identity
		// Mask Aadhaar number before storing (only last 4 digits visible)
		maskedAadhaar := maskAadhaar(nationalID)

		identity := &user.UserIdentity{
			TaxID:          taxID,
			TaxIDData:      string(taxIDDataJSON),
			NationalID:     maskedAadhaar, // Store masked Aadhaar
			NationalIDData: "{}",          // Will be populated after Aadhaar verification
			NTxnID:         "",            // Will be set below
		}

		if err := txUserRepo.UpsertUserIdentity(ctx, identity); err != nil {
			return fmt.Errorf("failed to create user identity: %w", err)
		}

		// Step 10: Update user name, dob, and tax_id reference
		if err := txUserRepo.UpdateUserNameAndDOB(ctx, userID, name, dob); err != nil {
			return fmt.Errorf("failed to update user name and DOB: %w", err)
		}

		if err := txUserRepo.UpdateUserTaxID(ctx, userID, taxID); err != nil {
			return fmt.Errorf("failed to link user to identity: %w", err)
		}

		// Step 11: Set KYC status to in_progress
		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCInProgress); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		// Step 12: Store Aadhaar transaction ID
		if err := txUserRepo.UpdateAadhaarTransactionID(ctx, taxID, tsTransID); err != nil {
			return fmt.Errorf("failed to store Aadhaar transaction ID: %w", err)
		}

		s.logger.Info("Onboarding initiated successfully for user %s", userID)

		// Prepare response
		response = &user.InitiateOnboardingResponseDTO{
			Status:      shared.KYCInProgress,
			AadhaarLink: aadhaarURL,
			TxnID:       tsTransID,
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to initiate onboarding: %v", err)
		return nil, err
	}

	return response, nil
}

// CompleteOnboarding completes the user onboarding after Aadhaar verification
// Supports three flows:
// 1. Invited user (no businessID/gstin): Complete KYC for all business relations
// 2. Creator flow (businessID/gstin + role claim): Complete KYC and set role claim
// 3. Invited user for specific business: Complete KYC for specific business relation
func (s *UserServiceImpl) CompleteOnboarding(ctx context.Context, userID, businessID, gstin string, claimedRole shared.UserRole) error {
	s.logger.Info("Completing onboarding for user %s with businessID=%s, gstin=%s, role=%s", userID, businessID, gstin, claimedRole)

	// Input validation
	if userID == "" {
		return types.NewInvalidKYCInputError("userID", "required")
	}

	// Get user data
	userData, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || userData == nil {
		return types.ErrUserNotFound
	}

	// Validate user has initiated onboarding
	if userData.KycStatus != shared.KYCInProgress {
		return fmt.Errorf("onboarding not initiated - please call initiate onboarding first")
	}

	if userData.TaxID == "" {
		return fmt.Errorf("user must have tax ID configured")
	}

	// Fetch all business relations for the user
	userRelations, err := s.businessUserRelRepo.FindByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to fetch user business relations: %w", err)
	}

	// Execute onboarding based on flow type
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)

		// Step 1: Complete Aadhaar verification (common for all flows)
		if err := s.completeAadhaarVerification(ctx, tx, userID, userData.TaxID); err != nil {
			return err
		}

		// Step 2: Handle business relation updates based on flow type
		if businessID == "" && gstin == "" {
			// Flow 1: Invited user (no specific business) - update all relations
			invited := false
			for _, rel := range userRelations {
				if rel.Role != "" {
					invited = true
					break
				}
			}
			if !invited {
				return types.NewInvalidKYCInputError("businessID or gstin", "invited user must have a business relation with a role")
			}

			s.logger.Info("User %s is invited, completing KYC for all business relations", userID)
			for _, rel := range userRelations {
				if err := txBusinessUserRelRepo.UpdateKycNeeded(ctx, userID, rel.BID, false); err != nil {
					s.logger.Errorf("Failed to update kycNeeded for user %s in business %s: %v", userID, rel.BID, err)
					return fmt.Errorf("failed to update kycNeeded for business %s: %w", rel.BID, err)
				}
			}
			s.logger.Info("Onboarding completed for invited user %s - role already assigned, kycNeeded=false", userID)

		} else {
			// Flow 2 & 3: Business-specific flows
			if claimedRole == "" {
				return types.NewInvalidKYCInputError("claimedRole", "required")
			}

			// Resolve target business ID
			var targetBusinessID string
			if businessID != "" {
				targetBusinessID = businessID
			} else if gstin != "" {
				business, err := s.businessRepo.FindByGSTIN(ctx, gstin)
				if err != nil {
					return fmt.Errorf("error finding business by GSTIN: %w", err)
				}
				if business == nil {
					return types.ErrBusinessNotFound
				}
				targetBusinessID = business.BID
			}

			// Find the business relation
			var rel *businessRepo.BusinessUserRelation
			for _, r := range userRelations {
				if r.BID == targetBusinessID {
					rel = r
					break
				}
			}

			if rel == nil {
				return fmt.Errorf("user-business relation not found for business %s", targetBusinessID)
			}

			if rel.KybInitiated && rel.Role == "" && rel.RoleClaim == "" {
				// Flow 2: Creator flow (user created the business)
				s.logger.Info("User %s is business creator for %s", userID, targetBusinessID)

				if err := txBusinessUserRelRepo.UpdateUserRole(ctx, userID, targetBusinessID, shared.None); err != nil {
					return fmt.Errorf("failed to update role to 'none' for business %s: %w", targetBusinessID, err)
				}
				if err := txBusinessUserRelRepo.UpdateRoleClaim(ctx, userID, targetBusinessID, claimedRole); err != nil {
					return fmt.Errorf("failed to update role claim for business %s: %w", targetBusinessID, err)
				}
				if err := txBusinessUserRelRepo.UpdateKycNeeded(ctx, userID, targetBusinessID, false); err != nil {
					return fmt.Errorf("failed to update kycNeeded for business %s: %w", targetBusinessID, err)
				}
				s.logger.Info("Onboarding completed - role='none', roleClaim='%s', kycNeeded=false for user %s in business %s (pending ops approval)", claimedRole, userID, targetBusinessID)

			} else if rel.Role != "" {
				// Flow 3: Invited user for specific business
				s.logger.Info("User %s is invited to business %s", userID, targetBusinessID)

				if err := txBusinessUserRelRepo.UpdateKycNeeded(ctx, userID, targetBusinessID, false); err != nil {
					s.logger.Errorf("Failed to update kycNeeded for user %s in business %s: %v", userID, targetBusinessID, err)
					return fmt.Errorf("failed to update kycNeeded for business %s: %w", targetBusinessID, err)
				}
				s.logger.Info("Onboarding completed for invited user %s in business %s - kycNeeded=false", userID, targetBusinessID)

			} else {
				return fmt.Errorf("ambiguous user-business relation for user %s and business %s", userID, targetBusinessID)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate user cache after successful KYC completion
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUser(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user cache after KYC completion: %v", err)
		}
	}()

	return nil
}

// completeAadhaarVerification handles the common Aadhaar verification steps
func (s *UserServiceImpl) completeAadhaarVerification(ctx context.Context, tx client.Tx, userID, taxID string) error {
	txUserRepo := s.userRepo.WithTx(tx)

	// Step 1: Get stored Aadhaar transaction ID
	tsTransID, err := txUserRepo.GetAadhaarTransactionID(ctx, userID)
	if err != nil {
		return types.ErrAadhaarNotInitiated
	}

	// Step 2: Create authbridge client
	authClient, err := authbridge.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create authentication client: %w", err)
	}

	// Step 3: Call Aadhaar status API
	s.logger.Info("Fetching Aadhaar verification status for user %s", userID)
	statusRequest := authbridge.NewAadhaarStatusRequest(tsTransID)
	aadhaarStatusResponse, err := authClient.SearchDocument(statusRequest)
	if err != nil {
		return types.NewVerificationFailedError("Aadhaar status", err.Error())
	}

	// Step 4: Convert response to JSON and store in national_id_data
	responseJSON, err := json.Marshal(aadhaarStatusResponse)
	if err != nil {
		return fmt.Errorf("failed to marshal Aadhaar response: %w", err)
	}

	if err := txUserRepo.UpdateNationalIDData(ctx, taxID, string(responseJSON)); err != nil {
		return fmt.Errorf("failed to update national ID data: %w", err)
	}

	// Step 5: Set KYC status to completed
	if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCCompleted); err != nil {
		return fmt.Errorf("failed to update KYC status: %w", err)
	}

	return nil
}

// GetOnboardingStatus returns the current onboarding status for a user
func (s *UserServiceImpl) GetOnboardingStatus(ctx context.Context, userID string) (*user.OnboardingStatusDTO, error) {
	s.logger.Info("Getting onboarding status for user %s", userID)

	if userID == "" {
		return nil, types.NewInvalidKYCInputError("userID", "required")
	}

	userData, identityData, err := s.userRepo.GetUserKYCStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user onboarding status: %w", err)
	}

	if userData == nil {
		return nil, types.ErrUserNotFound
	}

	// SECURITY: If KYC is completed or verified, only return status (no sensitive data)
	if userData.KycStatus == shared.KYCCompleted || userData.KycStatus == shared.KYCVerified {
		return &user.OnboardingStatusDTO{
			KycStatus: userData.KycStatus,
		}, nil
	}

	// KYC in progress - return full details for resume capability
	response := &user.OnboardingStatusDTO{
		UserID:         userData.Id,
		Email:          userData.Email,
		Name:           userData.Name,
		DOB:            userData.DOB,
		Mobile:         userData.Mobile,
		TaxID:          "",
		NationalID:     "",
		KycStatus:      userData.KycStatus,
		EmailVerified:  userData.EmailVerified,
		MobileVerified: userData.MobileVerified,
	}

	// Add identity data if available
	if identityData != nil {
		response.TaxID = identityData.TaxID
		response.NationalID = identityData.NationalID
	}

	return response, nil
}

// ProcessAadhaarRedirectCallback processes the Aadhaar redirect callback with authentication
// Returns error if verification fails or if KYC is already completed
func (s *UserServiceImpl) ProcessAadhaarRedirectCallback(ctx context.Context, userID, encryptedResponse string) error {
	s.logger.Info("Processing Aadhaar redirect callback for user %s", userID)

	// Decrypt the response
	decryptedResponse, responseData, err := s.decryptAndParseAadhaarResponse(encryptedResponse)
	if err != nil {
		return err
	}

	// Check status first (must be 1 for success)
	status, ok := responseData["status"].(float64)
	if !ok || status != 1 {
		errorMsg := "Verification failed"
		if msg, exists := responseData["message"].(string); exists {
			errorMsg = msg
		}
		s.logger.Error("Aadhaar verification failed for user %s: status=%v, message=%s", userID, status, errorMsg)
		return fmt.Errorf("aadhaar verification failed: %s", errorMsg)
	}

	// Extract ts_trans_id
	tsTransID, ok := responseData["ts_trans_id"].(string)
	if !ok || tsTransID == "" {
		s.logger.Error("Missing ts_trans_id in callback response for user %s", userID)
		return fmt.Errorf("missing transaction ID in response")
	}

	// Get user details
	userObj, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user details for user %s: %v", userID, err)
		return fmt.Errorf("failed to get user details: %w", err)
	}

	// Check if KYC is already completed or verified
	if userObj.KycStatus == shared.KYCCompleted || userObj.KycStatus == shared.KYCVerified {
		s.logger.Info("KYC already completed for user %s, current status: %s", userID, userObj.KycStatus)
		return fmt.Errorf("kyc_already_completed")
	}

	// Verify transaction ID matches
	if err := s.verifyAadhaarTransactionID(ctx, userObj.TaxID, tsTransID); err != nil {
		s.logger.Error("Transaction ID mismatch for user %s: %v", userID, err)
		return fmt.Errorf("invalid transaction reference: %w", err)
	}

	// Update user identity with Aadhaar data
	if err := s.updateAadhaarVerificationData(ctx, userObj.TaxID, userID, decryptedResponse); err != nil {
		return err
	}

	s.logger.Info("Aadhaar verification completed successfully for user %s", userID)
	return nil
}

// ProcessAadhaarWebhook processes the Aadhaar webhook without authentication
// Always returns nil to acknowledge receipt (errors are logged only)
func (s *UserServiceImpl) ProcessAadhaarWebhook(ctx context.Context, encryptedResponse string) error {
	s.logger.Info("Processing Aadhaar webhook")

	// Decrypt the response
	decryptedResponse, responseData, err := s.decryptAndParseAadhaarResponse(encryptedResponse)
	if err != nil {
		s.logger.Error("Failed to decrypt/parse webhook response: %v", err)
		return nil // Return nil to acknowledge receipt
	}

	// Check status first (must be 1 for success)
	status, ok := responseData["status"].(float64)
	if !ok || status != 1 {
		errorMsg := "Verification failed"
		if msg, exists := responseData["message"].(string); exists {
			errorMsg = msg
		}
		s.logger.Error("Aadhaar verification failed in webhook: status=%v, message=%s", status, errorMsg)
		return nil // Return nil to acknowledge receipt
	}

	// Extract ts_trans_id
	tsTransID, ok := responseData["ts_trans_id"].(string)
	if !ok || tsTransID == "" {
		s.logger.Error("Missing ts_trans_id in webhook response")
		return nil
	}

	// Find user identity by transaction ID
	identity, err := s.userRepo.GetUserIdentityByTxnID(ctx, tsTransID)
	if err != nil {
		s.logger.Error("Failed to find user identity by txn_id %s: %v", tsTransID, err)
		return nil
	}

	// Get user by tax ID
	userObj, err := s.userRepo.GetUserByTaxID(ctx, identity.TaxID)
	if err != nil {
		s.logger.Error("Failed to get user by tax_id %s: %v", identity.TaxID, err)
		return nil
	}

	// Check if KYC is already completed (idempotent)
	if userObj.KycStatus == shared.KYCCompleted || userObj.KycStatus == shared.KYCVerified {
		s.logger.Info("KYC already completed for user %s, current status: %s", userObj.Id, userObj.KycStatus)
		return nil
	}

	// Update user identity with Aadhaar data
	if err := s.updateAadhaarVerificationData(ctx, identity.TaxID, userObj.Id, decryptedResponse); err != nil {
		s.logger.Error("Failed to update Aadhaar verification data in webhook: %v", err)
		return nil
	}

	s.logger.Info("Aadhaar verification completed successfully via webhook for user %s", userObj.Id)
	return nil
}

// decryptAndParseAadhaarResponse decrypts and parses the AuthBridge response
func (s *UserServiceImpl) decryptAndParseAadhaarResponse(encryptedResponse string) (string, map[string]interface{}, error) {
	// Decrypt response
	decryptedResponse, err := crypto.DecryptAES256CBC(encryptedResponse, s.config.AuthBridge.Password)
	if err != nil {
		s.logger.Error("Failed to decrypt Aadhaar response: %v", err)
		return "", nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Parse decrypted response
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(decryptedResponse), &responseData); err != nil {
		s.logger.Error("Failed to parse decrypted response: %v", err)
		return "", nil, fmt.Errorf("invalid response format: %w", err)
	}

	return decryptedResponse, responseData, nil
}

// verifyAadhaarTransactionID verifies that the transaction ID matches
func (s *UserServiceImpl) verifyAadhaarTransactionID(ctx context.Context, taxID, tsTransID string) error {
	identity, err := s.userRepo.GetUserIdentityByTaxID(ctx, taxID)
	if err != nil {
		return fmt.Errorf("failed to get user identity: %w", err)
	}

	if identity.NTxnID != tsTransID {
		return fmt.Errorf("transaction ID mismatch: expected %s, got %s", identity.NTxnID, tsTransID)
	}

	return nil
}

// updateAadhaarVerificationData updates the user identity with Aadhaar verification data and KYC status
// Uses transaction to ensure atomicity - both updates must succeed or both fail
func (s *UserServiceImpl) updateAadhaarVerificationData(ctx context.Context, taxID, userID, responseJSON string) error {
	// Use transaction to ensure both updates are atomic
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Update national_id_data with full response
		if err := txUserRepo.UpdateNationalIDData(ctx, taxID, responseJSON); err != nil {
			s.logger.Error("Failed to update national ID data: %v", err)
			return fmt.Errorf("failed to save verification data: %w", err)
		}

		// Update KYC status to verified
		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCVerified); err != nil {
			s.logger.Error("Failed to update KYC status: %v", err)
			return fmt.Errorf("failed to update verification status: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// CreateInvitedUser creates a new user for business invitation
// Note: This no longer sends emails - caller should use auth service for password reset/invitation emails
// User is created with empty password hash - they MUST use the invitation link to set their password
func (s *UserServiceImpl) CreateInvitedUser(ctx context.Context, email, name string, kycNeeded bool) (*user.User, error) {
	s.logger.Info("Creating invited user with email: %s", email)

	// Input validation
	if email == "" || name == "" {
		return nil, fmt.Errorf("email and name are required")
	}

	// Check if user already exists
	existingUser, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		s.logger.Errorf("failed to check existing user: %v", err)
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return nil, fmt.Errorf("user with email %s already exists", email)
	}

	// Create empty password hash (user cannot login, must use invitation link to set password)
	// This is more efficient and semantically correct than generating a random password
	hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(""), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	hashedPassword := string(hashedPasswordBytes)

	// Create user with empty password hash
	createdUser, err := s.userRepo.CreateInvitedUser(ctx, email, hashedPassword, name, kycNeeded)
	if err != nil {
		return nil, fmt.Errorf("failed to create invited user: %w", err)
	}

	s.logger.Info("Invited user created successfully with ID: %s", createdUser.Id)
	return createdUser, nil
}

// SearchUsersByBusinesses searches users by name or email within specified businesses (ops dashboard)
func (s *UserServiceImpl) SearchUsersByBusinesses(ctx context.Context, businessIDs []string, searchTerm string) ([]*user.OpsUserSearchResult, error) {
	return s.userRepo.SearchUsersByBusinesses(ctx, businessIDs, searchTerm)
}

// GetUserDetailsForOps retrieves detailed user information with identity data (ops dashboard)
// If businessID is provided, also fetches role information for that business
func (s *UserServiceImpl) GetUserDetailsForOps(ctx context.Context, userID string, businessID string) (*user.OpsUserDetails, error) {
	return s.userRepo.GetUserDetailsForOps(ctx, userID, businessID)
}
