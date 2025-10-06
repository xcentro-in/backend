package business

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/services/verification"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
)

// BusinessServiceImpl provides business logic for business operations
type BusinessServiceImpl struct {
	businessRepo        business.BusinessRepository
	businessUserRelRepo business.BusinessUserRelationRepository
	userRepo            user.UserRepository
	txManager           client.TxManager
	rclient             *redis.RedisClient
	verificationService *verification.DocumentVerificationService
	logger              *logger.Logger
}

// NewBusinessService creates a new business service with dependency injection
func NewBusinessService(
	businessRepo business.BusinessRepository,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	txManager client.TxManager,
	rclient *redis.RedisClient,
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
		txManager:           txManager,
		rclient:             rclient,
		verificationService: verificationSvc,
		logger:              logger.GetDefault(),
	}
}

func (s *BusinessServiceImpl) GetUserRolesMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID := c.Get("userId").(string)
			rel, err := s.businessUserRelRepo.FindByUser(c.Request().Context(), userID)
			if err != nil {
				s.logger.Errorf("%v", err)
			}
			if len(rel) > 0 {
				c.Set("userRoles", rel)
			}
			return next(c)
		}
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

	return nil
}

// ============================================================================
// New Onboarding Flow Methods
// ============================================================================

// ValidateOnboardingEligibility checks if a user can create a new business
func (s *BusinessServiceImpl) ValidateOnboardingEligibility(ctx context.Context, uid string) error {
	// Use optimized count query instead of fetching all relations
	count, err := s.userRepo.CountBusinessRelations(ctx, uid)
	if err != nil {
		return fmt.Errorf("error checking user business relations: %w", err)
	}

	if count > 0 {
		s.logger.Warnf("User %s already associated with %d business(es) - KYB forbidden for invited users", uid, count)
		return types.ErrUserAlreadyAssociated
	}

	return nil
}

// InitiateOnboarding creates a new business and verifies GSTIN via AuthBridge
func (s *BusinessServiceImpl) InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*business.OnboardingInitResponse, error) {
	s.logger.Infof("Initiating onboarding for user %s with GSTIN %s", uid, gstin)

	// Step 1: Validate user eligibility
	if err := s.ValidateOnboardingEligibility(ctx, uid); err != nil {
		return nil, err
	}

	// Step 2: Validate inputs
	if name == "" || gstin == "" {
		return nil, types.NewInvalidInputError("name or gstin", "both name and GSTIN are required")
	}

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
		transID := uuid.New().String()
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

	// Step 7: Create business-user relation with kyb_initiated = true
	err = s.businessUserRelRepo.AddUserToBusiness(ctx, &business.BusinessUserRelation{
		UserID:       uid,
		BID:          newBusiness.BID,
		KybInitiated: true,
		Role:         shared.Support,
		RoleClaim:    shared.Support,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating business-user relation: %w", err)
	}

	// Step 8: Update Redis cache
	cmd := s.rclient.Client.HSet(ctx, "xcentro:user:verification:"+uid, "is_kyb_initiated", true)
	if err := cmd.Err(); err != nil {
		s.logger.Errorf("Error setting Redis cache: %v", err)
	}

	s.logger.Infof("Business onboarding initiated successfully for BID: %s", newBusiness.BID)

	return &business.OnboardingInitResponse{
		BID:       newBusiness.BID,
		Name:      newBusiness.Name,
		GSTIN:     newBusiness.GSTIN,
		KYBStatus: shared.KYBInProgress,
		Message:   "Business created successfully. Please complete KYB details.",
	}, nil
}

// CompleteKYB completes the KYB process with business type, PAN, and optionally CIN
func (s *BusinessServiceImpl) CompleteKYB(ctx context.Context, uid string, req *business.CompleteKYBRequest) (*business.CompleteKYBResponse, error) {
	s.logger.Infof("Completing KYB for user %s, BID %s", uid, req.BID)

	// Step 1: Validate inputs
	if req.BID == "" || req.ITID == "" || req.BusinessType == "" {
		return nil, types.NewInvalidInputError("bid, itid, or businessType", "all required fields must be provided")
	}

	// Step 2: Validate business type
	if !shared.IsValidBusinessType(req.BusinessType) {
		return nil, types.NewInvalidBusinessTypeError(string(req.BusinessType))
	}

	// Step 3: Check if CIN is required but missing
	if shared.RequiresCIN(req.BusinessType) && req.CID == "" {
		return nil, types.NewInvalidInputError("cid", fmt.Sprintf("CIN is required for business type: %s", req.BusinessType))
	}

	// Step 4: Verify user is authorized for this business
	relations, err := s.businessUserRelRepo.FindByUser(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error checking user authorization: %w", err)
	}

	authorized := false
	kybInitiated := false
	for _, rel := range relations {
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
	existingBusiness, _, err := s.businessRepo.GetBusinessWithKYBData(ctx, req.BID)
	if err != nil {
		return nil, fmt.Errorf("error fetching business: %w", err)
	}

	if existingBusiness.KybStatus == shared.KYBCompleted || existingBusiness.KybStatus == shared.KYBVerified {
		return nil, types.ErrKYBAlreadyCompleted
	}

	// Step 6: Verify PAN via AuthBridge
	var itidDataJSON string
	if s.verificationService != nil {
		transID := uuid.New().String()
		panData, err := s.verificationService.VerifyPANCOM(ctx, req.ITID, transID)
		if err != nil {
			s.logger.Errorf("PAN verification failed: %v", err)
			return nil, fmt.Errorf("PAN verification failed: %w", err)
		}

		itidDataJSON, err = verification.ToJSON(panData)
		if err != nil {
			s.logger.Errorf("Failed to serialize PAN data: %v", err)
		}
	}

	// Step 7: Verify CIN if required
	var cidDataJSON string
	if shared.RequiresCIN(req.BusinessType) && s.verificationService != nil {
		transID := uuid.New().String()
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

	// Step 8: Update KYB data
	_, err = s.businessRepo.UpdateKYBData(ctx, req.BID, &business.BusinessKYBData{
		ITID:     req.ITID,
		ITIDData: itidDataJSON,
		CID:      req.CID,
		CIDData:  cidDataJSON,
		DocUrl:   "", // Will be updated when deed is uploaded
	})
	if err != nil {
		return nil, fmt.Errorf("error updating KYB data: %w", err)
	}

	// Step 9: Update business with type and status
	err = s.businessRepo.AddKYBSuccessData(ctx, &business.Business{
		BID:          req.BID,
		KybStatus:    shared.KYBCompleted,
		BusinessType: req.BusinessType,
		ITID:         req.ITID,
	})
	if err != nil {
		return nil, fmt.Errorf("error updating business KYB status: %w", err)
	}

	s.logger.Infof("KYB completed successfully for BID: %s", req.BID)

	// Step 10: Determine if deed upload is required
	requiresDeed := shared.RequiresDeed(req.BusinessType)

	return &business.CompleteKYBResponse{
		BID:                req.BID,
		KYBStatus:          shared.KYBCompleted,
		RequiresDeedUpload: requiresDeed,
		Message:            "KYB details saved successfully",
	}, nil
}

// GetOnboardingStatus retrieves the current KYB onboarding status for a user
func (s *BusinessServiceImpl) GetOnboardingStatus(ctx context.Context, uid string) (*business.OnboardingStatusResponse, error) {
	s.logger.Infof("Fetching onboarding status for user %s", uid)

	// Get user's business relations
	relations, err := s.businessUserRelRepo.FindByUser(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error fetching user relations: %w", err)
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

	s.logger.Infof("Successfully removed user %s from business %s", targetUserID, bid)
	return nil
}
