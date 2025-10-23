package business

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/services/verification"
	sharedClients "xcentro.in/internal/shared/clients"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
)

// BusinessServiceImpl provides business logic for business operations
type BusinessServiceImpl struct {
	businessRepo        business.BusinessRepository
	businessUserRelRepo business.BusinessUserRelationRepository
	userRepo            user.UserRepository
	txManager           client.TxManager
	rclient             *redisClient.RedisClient
	verificationService *verification.DocumentVerificationService
	logger              *logger.Logger
}

// NewBusinessService creates a new business service with dependency injection
func NewBusinessService(
	businessRepo business.BusinessRepository,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	clients sharedClients.ExternalClients,
	cfg *config.Config,
) business.BusinessService {
	// Initialize verification service (handle error in production)
	verificationSvc, err := verification.NewDocumentVerificationService()
	if err != nil {
		logger.GetDefault().Warnf("Failed to initialize verification service: %v", err)
	}

	return &BusinessServiceImpl{
		businessRepo:        businessRepo,
		businessUserRelRepo: businessUserRelRepo,
		userRepo:            userRepo,
		txManager:           clients.GetTxManager(),   // Extract from clients
		rclient:             clients.GetRedisClient(), // Extract from clients
		verificationService: verificationSvc,
		logger:              logger.GetDefault(),
	}
}

func (s *BusinessServiceImpl) UpdateBusinessRoleClaim(ctx context.Context, uid, bid string, roleClaim shared.UserRole) error {
	if uid == "" || bid == "" {
		return fmt.Errorf("user ID and business ID are required")
	}

	err := s.businessUserRelRepo.UpdateUserRole(ctx, uid, bid, roleClaim)
	if err != nil {
		return fmt.Errorf("error updating role claim: %w", err)
	}

	// Invalidate user roles cache after successful update
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(uid)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", uid, err)
		}
	}()

	return nil
}

func (s *BusinessServiceImpl) UpdateBusinessDeedDocumentKey(ctx context.Context, bid string, key string) error {
	if bid == "" || key == "" {
		return fmt.Errorf("business ID and document key are required")
	}

	err := s.businessRepo.UpdateBusinessDeedDocumentKey(ctx, bid, key)
	if err != nil {
		return fmt.Errorf("error updating business deed document path: %w", err)
	}

	// Business details cache will expire via TTL (10 minutes)
	return nil
}

// ============================================================================
// New Onboarding Flow Methods
// ============================================================================

// InitiateOnboarding creates a new business and verifies GSTIN via AuthBridge
func (s *BusinessServiceImpl) InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*business.OnboardingInitResponse, error) {
	s.logger.Infof("Initiating onboarding for user %s with GSTIN %s", uid, gstin)

	// Step 2: Validate inputs
	if name == "" || gstin == "" {
		return nil, types.NewInvalidInputError("name or gstin", "both name and GSTIN are required")
	}

	// Step 2.1: Validate GSTIN format (must be exactly 15 characters)
	if len(gstin) != 15 {
		return nil, types.NewInvalidInputError("gstin", "GSTIN must be exactly 15 characters")
	}

	// Step 2.2: Extract PAN from GSTIN (characters 3-12, positions 2-11 in 0-indexed)
	pan := gstin[2:12]
	s.logger.Infof("Extracted PAN %s from GSTIN %s", pan, gstin)

	// Step 3: Check if business already exists
	existingBusiness, err := s.businessRepo.FindByGSTIN(ctx, gstin)
	if err != nil {
		return nil, fmt.Errorf("error checking existing business: %w", err)
	}
	if existingBusiness != nil {
		return nil, types.NewBusinessAlreadyExistsError(gstin)
	}

	// Step 4: Verify GSTIN via AuthBridge
	var gstDataJSON string
	if s.verificationService != nil {
		transID := uuid.Must(uuid.NewV7()).String()
		gstData, err := s.verificationService.VerifyGSTIN(ctx, gstin, transID)
		if err != nil {
			s.logger.Errorf("GSTIN verification failed: %v", err)
			return nil, fmt.Errorf("GSTIN verification failed: %w", err)
		}

		gstDataJSON, err = verification.ToJSON(gstData.RawResponse)
		if err != nil {
			s.logger.Errorf("Failed to serialize GST data: %v", err)
		}
	}

	// Step 4.1: Verify PAN via AuthBridge PAN_COM
	var itidDataJSON string
	if s.verificationService != nil {
		transID := uuid.Must(uuid.NewV7()).String()
		panData, err := s.verificationService.VerifyPANCOM(ctx, pan, transID)
		if err != nil {
			s.logger.Errorf("PAN verification failed: %v", err)
			return nil, fmt.Errorf("PAN verification failed: %w", err)
		}

		itidDataJSON, err = verification.ToJSON(panData)
		if err != nil {
			s.logger.Errorf("Failed to serialize PAN data: %v", err)
		}
	}

	// Step 5: Create business with KYB in progress
	newBusiness, err := s.businessRepo.CreateBusinessByNameAndGSTIN(ctx, name, gstin)
	if err != nil {
		return nil, fmt.Errorf("error creating business: %w", err)
	}

	// Step 6: Update GST data if verification succeeded
	if gstDataJSON != "" {
		if err := s.businessRepo.UpdateGSTData(ctx, newBusiness.BID, gstDataJSON); err != nil {
			s.logger.Errorf("Failed to update GST data: %v", err)
		}
	}

	// Step 6.1: Create/Update KYB data with PAN (income_tax_id)
	if pan != "" {
		_, err = s.businessRepo.UpdateKYBData(ctx, newBusiness.BID, &business.BusinessKYBData{
			ITID:     pan,
			ITIDData: itidDataJSON, // Repository handles empty string → NULL conversion
			CID:      "",
			CIDData:  "",
			DocUrl:   "",
		})
		if err != nil {
			s.logger.Errorf("Failed to update KYB data with PAN: %v", err)
			return nil, fmt.Errorf("error updating KYB data: %w", err)
		}

		// Link the PAN to business
		err = s.businessRepo.AddKYBSuccessData(ctx, &business.Business{
			BID:  newBusiness.BID,
			ITID: pan,
		})
		if err != nil {
			s.logger.Errorf("Failed to link PAN to business: %v", err)
		}
	}

	// Step 7: Create business-user relation with role and roleClaim as NULL
	err = s.businessUserRelRepo.AddUserToBusiness(ctx, &business.BusinessUserRelation{
		UserID:       uid,
		BID:          newBusiness.BID,
		KybInitiated: true,
		KycNeeded:    true, // User creating business needs KYC
		Role:         "",   // NULL - will be set after ops verification
		RoleClaim:    "",   // NULL - will be set when user completes KYC
	})
	if err != nil {
		return nil, fmt.Errorf("error creating business-user relation: %w", err)
	}

	s.logger.Infof("Business onboarding initiated successfully for BID: %s with PAN: %s", newBusiness.BID, pan)

	return &business.OnboardingInitResponse{
		BID:       newBusiness.BID,
		Name:      newBusiness.Name,
		GSTIN:     newBusiness.GSTIN,
		KYBStatus: shared.KYBInProgress,
		Message:   "Business created successfully. PAN extracted and verified. Please complete KYB details.",
	}, nil
}

// CompleteKYB completes the KYB process with business type and optionally CIN
// PAN (ITID) is already set during InitiateOnboarding
func (s *BusinessServiceImpl) CompleteKYB(ctx context.Context, uid string, userRoles []*business.BusinessUserRelation, req *business.CompleteKYBRequest) (*business.CompleteKYBResponse, error) {
	s.logger.Infof("Completing KYB for user %s, BID %s", uid, req.BID)

	// Step 1: Validate inputs (ITID is optional now, already set in init)
	if req.BID == "" || req.BusinessType == "" {
		return nil, types.NewInvalidInputError("bid or businessType", "bid and businessType are required")
	}

	// Step 2: Validate business type
	if !shared.IsValidBusinessType(req.BusinessType) {
		return nil, types.NewInvalidBusinessTypeError(string(req.BusinessType))
	}

	// Step 3: Check if CIN is required but missing
	if shared.RequiresCIN(req.BusinessType) && req.CID == "" {
		return nil, types.NewInvalidInputError("cid", fmt.Sprintf("CIN is required for business type: %s", req.BusinessType))
	}

	// Step 4: Verify user is authorized for this business (using middleware-injected roles)
	authorized := false
	kybInitiated := false
	for _, rel := range userRoles {
		if rel.BID == req.BID {
			authorized = true
			kybInitiated = rel.KybInitiated
			break
		}
	}

	if !authorized {
		return nil, types.ErrUnauthorizedKYB
	}

	if !kybInitiated {
		return nil, types.ErrKYBNotInitiated
	}

	// Step 5: Check if KYB already completed
	existingBusiness, existingKYBData, err := s.businessRepo.GetBusinessWithKYBData(ctx, req.BID)
	if err != nil {
		return nil, fmt.Errorf("error fetching business: %w", err)
	}

	if existingBusiness.KybStatus == shared.KYBCompleted || existingBusiness.KybStatus == shared.KYBVerified {
		return nil, types.ErrKYBAlreadyCompleted
	}

	// Step 5.1: Get existing PAN from business (set during init)
	existingPAN := existingBusiness.ITID
	if existingPAN == "" {
		return nil, types.NewInvalidInputError("itid", "PAN not found. Please reinitiate business onboarding.")
	}

	// Step 6: Verify CIN if required
	var cidDataJSON string
	if shared.RequiresCIN(req.BusinessType) && s.verificationService != nil {
		transID := uuid.Must(uuid.NewV7()).String()
		cinData, err := s.verificationService.VerifyCIN(ctx, req.CID, transID)
		if err != nil {
			s.logger.Errorf("CIN verification failed: %v", err)
			return nil, fmt.Errorf("CIN verification failed: %w", err)
		}

		cidDataJSON, err = verification.ToJSON(cinData.RawResponse)
		if err != nil {
			s.logger.Errorf("Failed to serialize CIN data: %v", err)
		}
	}

	// Step 7: Update KYB data (keep existing PAN data, add CIN)
	kybUpdate := &business.BusinessKYBData{
		ITID:    existingPAN, // Keep existing PAN
		CID:     req.CID,     // Update CIN
		CIDData: cidDataJSON, // Update CIN data
	}

	// Preserve existing data if available
	if existingKYBData != nil {
		kybUpdate.ITIDData = existingKYBData.ITIDData
		kybUpdate.DocUrl = existingKYBData.DocUrl
	}

	_, err = s.businessRepo.UpdateKYBData(ctx, req.BID, kybUpdate)
	if err != nil {
		return nil, fmt.Errorf("error updating KYB data: %w", err)
	}

	// Step 8: Update business with type and status
	err = s.businessRepo.AddKYBSuccessData(ctx, &business.Business{
		BID:          req.BID,
		KybStatus:    shared.KYBCompleted,
		BusinessType: req.BusinessType,
		ITID:         existingPAN, // Keep existing PAN
	})
	if err != nil {
		return nil, fmt.Errorf("error updating business KYB status: %w", err)
	}

	s.logger.Infof("KYB completed successfully for BID: %s with business type: %s", req.BID, req.BusinessType)

	// Business details cache will expire via TTL (10 minutes)

	// Determine if deed upload is required
	requiresDeed := shared.RequiresDeed(req.BusinessType)

	return &business.CompleteKYBResponse{
		BID:                req.BID,
		KYBStatus:          shared.KYBCompleted,
		RequiresDeedUpload: requiresDeed,
		Message:            "KYB details saved successfully",
	}, nil
}

// GetOnboardingStatus retrieves the current KYB onboarding status for a user
func (s *BusinessServiceImpl) GetOnboardingStatus(ctx context.Context, uid string, userRoles []*business.BusinessUserRelation) (*business.OnboardingStatusResponse, error) {
	s.logger.Infof("Fetching onboarding status for user %s", uid)

	// If roles not provided by middleware, fetch from DB (e.g., during login flow)
	var relations []*business.BusinessUserRelation
	if userRoles == nil {
		var err error
		relations, err = s.businessUserRelRepo.FindByUser(ctx, uid)
		if err != nil {
			return nil, fmt.Errorf("error fetching user relations: %w", err)
		}
	} else {
		relations = userRoles
	}

	// If no relations, user hasn't started onboarding
	if len(relations) == 0 {
		return &business.OnboardingStatusResponse{
			KYBStatus:    shared.KYBNotStarted,
			KYBInitiated: false,
		}, nil
	}

	// Get the first (and should be only) business relation
	rel := relations[0]

	// SECURITY: Only return onboarding status for business where user initiated KYB
	// Invited users (kyb_initiated=false) should not see onboarding details
	if !rel.KybInitiated {
		s.logger.Warnf("User %s attempted to access onboarding status for business %s where they are invited (not initiator)", uid, rel.BID)
		return &business.OnboardingStatusResponse{
			KYBStatus:    shared.KYBNotStarted,
			KYBInitiated: false,
		}, nil
	}

	// Get business and KYB data
	bus, kybData, err := s.businessRepo.GetBusinessWithKYBData(ctx, rel.BID)
	if err != nil {
		return nil, fmt.Errorf("error fetching business data: %w", err)
	}

	// SECURITY: If KYB is completed or verified, only return status (no sensitive data)
	if bus.KybStatus == shared.KYBCompleted || bus.KybStatus == shared.KYBVerified {
		return &business.OnboardingStatusResponse{
			KYBStatus: bus.KybStatus,
		}, nil
	}

	// KYB in progress - return full details for resume capability
	response := &business.OnboardingStatusResponse{
		BID:          bus.BID,
		Name:         bus.Name,
		GSTIN:        bus.GSTIN,
		KYBStatus:    bus.KybStatus,
		KYBInitiated: rel.KybInitiated,
		BusinessType: bus.BusinessType,
	}

	// Add KYB data if available
	if kybData != nil {
		response.ITID = kybData.ITID
		response.CID = kybData.CID
		response.HasDeedUploaded = kybData.DocUrl != ""
	}

	// Determine if deed upload is required
	if bus.BusinessType != "" {
		response.RequiresDeedUpload = shared.RequiresDeed(bus.BusinessType)
	}

	return response, nil
}

// AddUserToBusiness adds a user to a business (for invited users)
func (s *BusinessServiceImpl) AddUserToBusiness(ctx context.Context, relation *business.BusinessUserRelation) error {
	s.logger.Infof("Adding user %s to business %s with role %s", relation.UserID, relation.BID, relation.Role)

	err := s.businessUserRelRepo.AddUserToBusiness(ctx, relation)
	if err != nil {
		return fmt.Errorf("error adding user to business: %w", err)
	}

	// Invalidate user roles cache after successful addition
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(relation.UserID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", relation.UserID, err)
		}
	}()

	return nil
}

// GetBusinessUsers retrieves all users associated with a business
func (s *BusinessServiceImpl) GetBusinessUsers(ctx context.Context, bid string) ([]*business.BusinessUserDTO, error) {
	s.logger.Infof("Getting users for business %s", bid)

	users, err := s.businessUserRelRepo.GetBusinessUsersWithDetails(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("error getting business users: %w", err)
	}

	return users, nil
}

// UpdateUserRoleInBusiness updates a user's role in a business with authorization checks
func (s *BusinessServiceImpl) UpdateUserRoleInBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole, newRole shared.UserRole) error {
	s.logger.Infof("Updating user %s role in business %s to %s (requester role: %s)", targetUserID, bid, newRole, requesterRole)

	// Get target user's current role
	relations, err := s.businessUserRelRepo.FindByBusiness(ctx, bid)
	if err != nil {
		return fmt.Errorf("error finding business relations: %w", err)
	}

	var targetUserCurrentRole shared.UserRole
	found := false
	for _, rel := range relations {
		if rel.UserID == targetUserID {
			targetUserCurrentRole = rel.Role
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("user not found in business")
	}

	// Authorization check: Admin cannot change owner's role
	if requesterRole == shared.Admin && targetUserCurrentRole == shared.Owner {
		return fmt.Errorf("admin cannot change owner's role")
	}

	// Authorization check: Admin cannot promote someone to owner
	if requesterRole == shared.Admin && newRole == shared.Owner {
		return fmt.Errorf("admin cannot promote users to owner role")
	}

	// Update the role
	err = s.businessUserRelRepo.UpdateUserRole(ctx, targetUserID, bid, newRole)
	if err != nil {
		return fmt.Errorf("error updating user role: %w", err)
	}

	// Invalidate user roles cache after successful update
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(targetUserID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", targetUserID, err)
		}
	}()

	s.logger.Infof("Successfully updated user %s role to %s in business %s", targetUserID, newRole, bid)
	return nil
}

// RemoveUserFromBusiness removes a user from a business with authorization checks
func (s *BusinessServiceImpl) RemoveUserFromBusiness(ctx context.Context, bid string, targetUserID string, targetUserRole shared.UserRole, requesterRole shared.UserRole) error {
	s.logger.Infof("Removing user %s from business %s (requester role: %s, target role: %s)", targetUserID, bid, requesterRole, targetUserRole)

	// Authorization check: Admin cannot remove owner
	if requesterRole == shared.Admin && targetUserRole == shared.Owner {
		return fmt.Errorf("admin cannot remove owner from business")
	}

	// Remove user from business
	err := s.businessUserRelRepo.RemoveUserFromBusiness(ctx, targetUserID, bid)
	if err != nil {
		return fmt.Errorf("error removing user from business: %w", err)
	}

	// Invalidate user roles cache after successful removal
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(targetUserID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", targetUserID, err)
		}
	}()

	s.logger.Infof("Successfully removed user %s from business %s", targetUserID, bid)
	return nil
}

// GetBusinessDetails retrieves business details for a given business ID (cached)
// Uses middleware-injected user role to avoid DB query
func (s *BusinessServiceImpl) GetBusinessDetails(ctx context.Context, bid string, userRole shared.UserRole) (*business.BusinessDetailsResponse, error) {
	// Try cache first
	cacheKey := keys.Business.Details(bid)
	var cachedResponse business.BusinessDetailsResponse
	err := s.rclient.GetJSON(ctx, cacheKey, &cachedResponse)

	if err == nil {
		// Cache hit - inject user's role (role is user-specific, not cached)
		cachedResponse.UserRole = userRole
		s.logger.Infof("Business details cache hit for BID: %s", bid)
		return &cachedResponse, nil
	}

	// Cache miss - fetch from database
	bus, err := s.businessRepo.FindByID(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch business details: %w", err)
	}

	// Prepare response
	response := &business.BusinessDetailsResponse{
		BID:          bus.BID,
		Name:         bus.Name,
		GSTIN:        bus.GSTIN,
		PAN:          bus.ITID,
		BusinessType: bus.BusinessType,
		UserRole:     userRole,
	}

	// Cache response asynchronously (without user role - that's injected per-user)
	go func() {
		bgCtx := context.Background()
		cacheData := &business.BusinessDetailsResponse{
			BID:          response.BID,
			Name:         response.Name,
			GSTIN:        response.GSTIN,
			PAN:          response.PAN,
			BusinessType: response.BusinessType,
			// UserRole is NOT cached - it's per-user
		}
		if err := s.rclient.SetJSON(bgCtx, cacheKey, cacheData, keys.TTLBusinessDetails); err != nil {
			s.logger.Warnf("Failed to cache business details: %v", err)
		}
	}()

	return response, nil
}

// GetBusinessKYBData retrieves the KYB data for a business including company director information
// This is used for money rotation checks during payee onboarding
func (s *BusinessServiceImpl) GetBusinessKYBData(ctx context.Context, bid string) (*business.BusinessKYBData, error) {
	// Fetch business with KYB data from repository
	_, kybData, err := s.businessRepo.GetBusinessWithKYBData(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch business KYB data: %w", err)
	}

	if kybData == nil {
		return nil, fmt.Errorf("KYB data not found for business: %s", bid)
	}

	return kybData, nil
}

// GetBusinessOnboardingFlags retrieves the onboarding flags for a business (cached).
func (s *BusinessServiceImpl) GetBusinessOnboardingFlags(ctx context.Context, bid string) (*business.BusinessOnboardingStatusDTO, error) {
	// Try cache first
	cacheKey := keys.Business.OnboardingStatus(bid)
	var cachedStatus business.BusinessOnboardingStatusDTO
	err := s.rclient.GetJSON(ctx, cacheKey, &cachedStatus)

	if err == nil {
		s.logger.Infof("Business onboarding status cache hit for BID: %s", bid)
		return &cachedStatus, nil
	}

	// Cache miss - fetch from database
	bus, err := s.businessRepo.FindByID(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch business details: %w", err)
	}

	// Prepare response
	status := &business.BusinessOnboardingStatusDTO{
		HasCompletedOnboarding: bus.HasCompletedOnboarding,
		HasAddedPaymentMethod:  bus.HasAddedPaymentMethod,
		HasAddedVendor:         bus.HasAddedVendor,
		HasCreatedFirstPayment: bus.HasCreatedFirstPayment,
	}

	// Cache response asynchronously (with no TTL)
	go func() {
		bgCtx := context.Background()
		if err := s.rclient.SetJSON(bgCtx, cacheKey, status, 0); err != nil {
			s.logger.Warnf("Failed to cache business onboarding status: %v", err)
		}
	}()

	return status, nil
}

// UpdateOnboardingStatus updates a specific onboarding flag for a business and invalidates the cache.
func (s *BusinessServiceImpl) UpdateOnboardingStatus(ctx context.Context, bid string, flagName string, value bool) error {
	if bid == "" || flagName == "" {
		return fmt.Errorf("business ID and flag name are required")
	}

	// Update the database
	err := s.businessRepo.UpdateOnboardingFlag(ctx, bid, flagName, value)
	if err != nil {
		return fmt.Errorf("error updating onboarding flag in DB: %w", err)
	}

	// Invalidate the cache asynchronously
	go func() {
		bgCtx := context.Background()
		invalidationKey := keys.Business.OnboardingStatus(bid)
		if err := s.rclient.Delete(bgCtx, invalidationKey); err != nil {
			s.logger.Warnf("Failed to invalidate business onboarding status cache for BID %s: %v", bid, err)
		}
	}()

	return nil
}
