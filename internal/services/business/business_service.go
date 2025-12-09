package business

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/core/business"
	coreNotification "xcentro.in/internal/core/notification"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/services/notification"
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
	userService         user.UserService
	paymentRepo         payment.PaymentRepository
	txManager           client.TxManager
	rclient             *redisClient.RedisClient
	verificationService *verification.DocumentVerificationService
	s3Config            *config.S3Config
	externalClients     sharedClients.ExternalClients // Store full clients interface for S3 access
	logger              *logger.Logger
}

// NewBusinessService creates a new business service with dependency injection
func NewBusinessService(
	businessRepo business.BusinessRepository,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	userService user.UserService,
	paymentRepo payment.PaymentRepository,
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
		userService:         userService,
		paymentRepo:         paymentRepo,
		txManager:           clients.GetTxManager(),   // Extract from clients
		rclient:             clients.GetRedisClient(), // Extract from clients
		verificationService: verificationSvc,
		s3Config:            &cfg.S3,
		externalClients:     clients, // Store full clients interface
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

// GetKYBDocumentUploadURL generates a presigned S3 URL for KYB document upload
func (s *BusinessServiceImpl) GetKYBDocumentUploadURL(ctx context.Context, userRoles []*business.BusinessUserRelation) (string, error) {
	// Authorization: User must have exactly one role with KYB initiated
	if !(len(userRoles) == 1 && userRoles[0].KybInitiated) {
		return "", fmt.Errorf("kyb completed or unauthorized")
	}

	// Create S3 client
	fileuploader, err := s.createS3Client()
	if err != nil {
		return "", fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Generate presigned upload URL
	key := fmt.Sprintf("PII/%s/%s", userRoles[0].BID, "business_deed")
	url, _, err := fileuploader.GetPresignedUploadURL(ctx, "test.pdf", "application/pdf", key, 15*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to get presigned upload URL: %w", err)
	}

	return url, nil
}

// ConfirmKYBDocumentUpload confirms the S3 upload and updates the database
func (s *BusinessServiceImpl) ConfirmKYBDocumentUpload(ctx context.Context, userRoles []*business.BusinessUserRelation) (string, error) {
	// Authorization: User must have exactly one role with KYB initiated
	if !(len(userRoles) == 1 && userRoles[0].KybInitiated) {
		return "", fmt.Errorf("kyb completed or unauthorized")
	}

	// Create S3 client
	fileuploader, err := s.createS3Client()
	if err != nil {
		return "", fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Confirm upload and get file URL
	key := fmt.Sprintf("PII/%s/%s", userRoles[0].BID, "business_deed.pdf")
	fileurl, err := fileuploader.ConfirmUpload(ctx, key, 15*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to confirm upload: %w", err)
	}

	// Update database with document key
	err = s.UpdateBusinessDeedDocumentKey(ctx, userRoles[0].BID, key)
	if err != nil {
		return "", fmt.Errorf("failed to update business deed document key: %w", err)
	}

	return fileurl, nil
}

// createS3Client is a helper method to create S3 client from config
func (s *BusinessServiceImpl) createS3Client() (*fileupload.S3Client, error) {
	return fileupload.NewS3ClientFromConfig(*s.s3Config)
}

// ============================================================================
// New Onboarding Flow Methods
// ============================================================================

// InitiateOnboarding creates a new business and verifies GSTIN via AuthBridge
func (s *BusinessServiceImpl) InitiateOnboarding(ctx context.Context, uid string, name string, gstin string) (*business.OnboardingInitResponse, error) {
	s.logger.Infof("Initiating onboarding for user %s with GSTIN %s", uid, gstin)

	// ============================================================================
	// PHASE 1: Validation and External API Calls (OUTSIDE TRANSACTION)
	// ============================================================================

	// Step 1: Validate inputs
	if name == "" || gstin == "" {
		return nil, types.NewInvalidInputError("name or gstin", "both name and GSTIN are required")
	}

	// Step 3: Extract PAN from GSTIN (characters 3-12, positions 2-11 in 0-indexed)
	pan := gstin[2:12]
	s.logger.Infof("Extracted PAN %s from GSTIN %s", pan, gstin)

	// Step 4: Verify GSTIN via AuthBridge (EXTERNAL API - do BEFORE transaction)
	// NOTE: GSTIN verification is optional - if it fails, we proceed with empty data
	var gstDataJSON string
	if s.verificationService != nil {
		transID := uuid.Must(uuid.NewV7()).String()
		gstData, err := s.verificationService.VerifyGSTIN(ctx, gstin, transID)
		if err != nil {
			s.logger.Warnf("GSTIN verification failed: %v (proceeding with empty data)", err)
			gstDataJSON = "{}" // Empty JSON on failure
		} else {
			gstDataJSON, err = verification.ToJSON(gstData.RawResponse)
			if err != nil {
				s.logger.Errorf("Failed to serialize GST data: %v (using empty data)", err)
				gstDataJSON = "{}" // Empty JSON on serialization failure
			}
		}
	} else {
		gstDataJSON = "{}" // Empty JSON if verification service unavailable
	}

	// Step 5: Check if PAN already exists in kyb_data (SMART OPTIMIZATION)
	// If exists, reuse existing PAN data instead of calling verification API
	var itidDataJSON string
	existingKYB, err := s.businessRepo.GetKYBDataByPAN(ctx, pan)

	if err == nil && existingKYB != nil && existingKYB.ITIDData != "" && existingKYB.ITIDData != "{}" {
		// PAN exists with data - reuse it
		s.logger.Infof("PAN %s already exists in kyb_data, reusing existing verification data", pan)
		itidDataJSON = existingKYB.ITIDData
	} else {
		// PAN doesn't exist or has no data - verify it
		s.logger.Infof("PAN %s not found in kyb_data, calling verification API", pan)
		if s.verificationService != nil {
			transID := uuid.Must(uuid.NewV7()).String()
			panData, err := s.verificationService.VerifyPANCOM(ctx, pan, transID)
			if err != nil {
				s.logger.Warnf("PAN verification failed: %v (proceeding with empty data)", err)
				itidDataJSON = "{}" // Empty JSON on failure
			} else {
				itidDataJSON, err = verification.ToJSON(panData)
				if err != nil {
					s.logger.Errorf("Failed to serialize PAN data: %v (using empty data)", err)
					itidDataJSON = "{}" // Empty JSON on serialization failure
				}
			}
		} else {
			itidDataJSON = "{}" // Empty JSON if verification service unavailable
		}
	}

	// ============================================================================
	// PHASE 2: Database Operations (INSIDE TRANSACTION)
	// ============================================================================

	var newBusiness *business.Business

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBusinessRepo := s.businessRepo.WithTx(tx)
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)

		// Step 6: Check if business already exists (CRITICAL READ - must be in transaction to prevent race condition)
		existingBusiness, err := txBusinessRepo.FindByGSTIN(ctx, gstin)
		if err != nil {
			return fmt.Errorf("error checking existing business: %w", err)
		}
		if existingBusiness != nil {
			return types.NewBusinessAlreadyExistsError(gstin)
		}

		// Step 7: Create business with KYB in progress
		newBusiness, err = txBusinessRepo.CreateBusinessByNameAndGSTIN(ctx, name, gstin)
		if err != nil {
			return fmt.Errorf("error creating business: %w", err)
		}

		// Step 8: Update GST data if verification succeeded
		if gstDataJSON != "" {
			if err := txBusinessRepo.UpdateGSTData(ctx, newBusiness.BID, gstDataJSON); err != nil {
				s.logger.Errorf("Failed to update GST data: %v", err)
				// Don't fail transaction for this
			}
		}

		// Step 9: Create/Update KYB data with PAN (income_tax_id)
		// UpdateKYBData already links the PAN to the business table's income_tax_id column
		if pan != "" {
			_, err = txBusinessRepo.UpdateKYBData(ctx, newBusiness.BID, &business.BusinessKYBData{
				ITID:     pan,
				ITIDData: itidDataJSON, // Repository handles empty string → NULL conversion
				CID:      "",
				CIDData:  "",
				DocUrl:   "",
			})
			if err != nil {
				s.logger.Errorf("Failed to update KYB data with PAN: %v", err)
				return fmt.Errorf("error updating KYB data: %w", err)
			}
		}

		// Step 10: Create business-user relation with role and roleClaim as NULL
		err = txBusinessUserRelRepo.AddUserToBusiness(ctx, &business.BusinessUserRelation{
			UserID:       uid,
			BID:          newBusiness.BID,
			KybInitiated: true,
			KycNeeded:    true, // User creating business needs KYC
			Role:         "",   // NULL - will be set after ops verification
			RoleClaim:    "",   // NULL - will be set when user completes KYC
		})
		if err != nil {
			return fmt.Errorf("error creating business-user relation: %w", err)
		}

		return nil // Commit transaction
	})

	if err != nil {
		return nil, err
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

	// ============================================================================
	// PHASE 1: Validation and External API Calls (OUTSIDE TRANSACTION)
	// ============================================================================

	// Step 1: Validate inputs (ITID is optional now, already set in init)
	if req.BID == "" || req.BusinessType == "" {
		return nil, types.NewInvalidInputError("bid or businessType", "bid and businessType are required")
	}

	// Step 2: Trim optional fields
	req.BusinessModel = strings.TrimSpace(req.BusinessModel)
	req.BusinessTurnover = strings.TrimSpace(req.BusinessTurnover)

	// Step 3: Validate business type
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

	// Step 5: Check if CID already exists in business's kyb_data (SMART OPTIMIZATION)
	// If exists, reuse existing CID data instead of calling verification API
	var cidDataJSON string
	if shared.RequiresCIN(req.BusinessType) {
		// Query existing kyb_data for this business
		_, existingKYB, err := s.businessRepo.GetBusinessWithKYBData(ctx, req.BID)
		if err != nil {
			return nil, fmt.Errorf("error fetching business: %w", err)
		}

		// Check if CID already exists with data
		if existingKYB != nil && existingKYB.CID == req.CID && existingKYB.CIDData != "" && existingKYB.CIDData != "{}" {
			// CID exists with data - reuse it
			s.logger.Infof("CIN %s already exists for business %s, reusing existing verification data", req.CID, req.BID)
			cidDataJSON = existingKYB.CIDData
		} else {
			// CID doesn't exist or has no data - verify it
			s.logger.Infof("CIN %s not found for business %s, calling verification API", req.CID, req.BID)
			if s.verificationService != nil {
				transID := uuid.Must(uuid.NewV7()).String()
				cinData, err := s.verificationService.VerifyCIN(ctx, req.CID, transID)
				if err != nil {
					s.logger.Warnf("CIN verification failed: %v (proceeding with empty data)", err)
					cidDataJSON = "{}" // Empty JSON on failure
				} else {
					cidDataJSON, err = verification.ToJSON(cinData.RawResponse)
					if err != nil {
						s.logger.Errorf("Failed to serialize CIN data: %v (using empty data)", err)
						cidDataJSON = "{}" // Empty JSON on serialization failure
					}
				}
			} else {
				cidDataJSON = "{}" // Empty JSON if verification service unavailable
			}
		}
	}

	// ============================================================================
	// PHASE 2: Database Operations (INSIDE TRANSACTION)
	// ============================================================================

	var requiresDeed bool

	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBusinessRepo := s.businessRepo.WithTx(tx)

		// Step 6: Check if KYB already completed (CRITICAL READ - must be in transaction)
		existingBusiness, existingKYBData, err := txBusinessRepo.GetBusinessWithKYBData(ctx, req.BID)
		if err != nil {
			return fmt.Errorf("error fetching business: %w", err)
		}

		if existingBusiness.KybStatus == shared.KYBCompleted || existingBusiness.KybStatus == shared.KYBVerified {
			return types.ErrKYBAlreadyCompleted
		}

		// Step 7: Get existing PAN from business (set during init)
		existingPAN := existingBusiness.ITID
		if existingPAN == "" {
			return types.NewInvalidInputError("itid", "PAN not found. Please reinitiate business onboarding.")
		}

		// Step 8: Update KYB data (keep existing PAN data, add CIN)
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

		_, err = txBusinessRepo.UpdateKYBData(ctx, req.BID, kybUpdate)
		if err != nil {
			return fmt.Errorf("error updating KYB data: %w", err)
		}

		// Step 9: Update business with type, model, turnover, and status
		err = txBusinessRepo.AddKYBSuccessData(ctx, &business.Business{
			BID:              req.BID,
			KybStatus:        shared.KYBCompleted,
			BusinessType:     req.BusinessType,
			BusinessModel:    req.BusinessModel,    // Optional
			BusinessTurnover: req.BusinessTurnover, // Optional
			ITID:             existingPAN,          // Keep existing PAN
		})
		if err != nil {
			return fmt.Errorf("error updating business KYB status: %w", err)
		}

		// Determine if deed upload is required
		requiresDeed = shared.RequiresDeed(req.BusinessType)

		return nil // Commit transaction
	})

	if err != nil {
		return nil, err
	}

	s.logger.Infof("KYB completed successfully for BID: %s with business type: %s", req.BID, req.BusinessType)

	// Business details cache will expire via TTL (10 minutes)

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

// InviteUserToBusiness invites a user to a business with authorization checks and sends invitation email
func (s *BusinessServiceImpl) InviteUserToBusiness(ctx context.Context, bid string, email string, name string, role shared.UserRole, requesterRole shared.UserRole) (*business.InviteUserResponse, error) {
	s.logger.Infof("Inviting user %s to business %s with role %s (requester role: %s)", email, bid, role, requesterRole)

	// Validate role is valid
	validRoles := []shared.UserRole{shared.Owner, shared.Admin, shared.Manager, shared.Auditor}
	isValidRole := false
	for _, validRole := range validRoles {
		if role == validRole {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
		return nil, fmt.Errorf("invalid role. Must be one of: owner, admin, manager, auditor")
	}

	// Authorization: Only admin or owner can invite users
	if requesterRole != shared.Owner && requesterRole != shared.Admin {
		return nil, fmt.Errorf("only business owners and admins can invite users")
	}

	// Admin cannot invite another owner (only owner can invite owner)
	if requesterRole == shared.Admin && role == shared.Owner {
		return nil, fmt.Errorf("only business owners can invite other owners")
	}

	var response *business.InviteUserResponse

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create invited user (always require KYC for invited users)
		createdUser, err := s.userService.CreateInvitedUser(ctx, email, name, true)
		if err != nil {
			s.logger.Errorf("Error creating invited user: %v", err)
			return fmt.Errorf("failed to create invited user: %w", err)
		}

		// Add user to business with specified role
		relation := &business.BusinessUserRelation{
			UserID:       createdUser.Id,
			BID:          bid,
			Role:         role,
			RoleClaim:    role,
			KybInitiated: false,
			KycNeeded:    false, // Always require KYC for invited users in business relation
		}

		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)
		err = txBusinessUserRelRepo.AddUserToBusiness(ctx, relation)
		if err != nil {
			s.logger.Errorf("Error adding user to business: %v", err)
			return fmt.Errorf("failed to add user to business: %w", err)
		}

		// Notify admins and above about new user addition
		if err := notification.TriggerMulticast(ctx, bid, coreNotification.EventUserInvited, "New User Added",
			fmt.Sprintf("A new user has been added to the business with role: %s", role),
			coreNotification.RoleAdmin, nil, coreNotification.EntityReferences{}, []coreNotification.ActionButton{
				{
					Type: "button-positive",
					Text: "View Settings",
					Href: "/settings",
				},
			}); err != nil {
			s.logger.Errorf("Failed to trigger user invited notification: %v", err)
			// Don't fail the operation if notification trigger fails
		}

		response = &business.InviteUserResponse{
			UserID: createdUser.Id,
			Email:  createdUser.Email,
			Name:   createdUser.Name,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Note: Invitation email should be sent by the caller (handler) after this succeeds

	// Invalidate user roles cache (outside transaction)
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(response.UserID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", response.UserID, err)
		}
	}()

	return response, nil
}

// AddUserToBusiness adds a user to a business (for invited users)
func (s *BusinessServiceImpl) AddUserToBusiness(ctx context.Context, relation *business.BusinessUserRelation) error {
	s.logger.Infof("Adding user %s to business %s with role %s", relation.UserID, relation.BID, relation.Role)

	err := s.businessUserRelRepo.AddUserToBusiness(ctx, relation)
	if err != nil {
		return fmt.Errorf("error adding user to business: %w", err)
	}

	// Notify admins and above about new user addition
	if err := notification.TriggerMulticast(ctx, relation.BID, coreNotification.EventUserInvited, "New User Added",
		fmt.Sprintf("A new user has been added to the business with role: %s", relation.Role),
		coreNotification.RoleAdmin, nil, coreNotification.EntityReferences{}, []coreNotification.ActionButton{}); err != nil {
		s.logger.Errorf("Failed to trigger user invited notification: %v", err)
		// Don't fail the operation if notification trigger fails
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
func (s *BusinessServiceImpl) GetBusinessUsers(ctx context.Context, bid string, requesterRole shared.UserRole) ([]*business.BusinessUserDTO, error) {
	s.logger.Infof("Getting users for business %s (requester role: %s)", bid, requesterRole)

	// Authorization: Only admin or owner can list users
	if requesterRole != shared.Owner && requesterRole != shared.Admin {
		return nil, fmt.Errorf("only business owners and admins can list users")
	}

	users, err := s.businessUserRelRepo.GetBusinessUsersWithDetails(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("error getting business users: %w", err)
	}

	return users, nil
}

// UpdateUserRoleInBusiness updates a user's role in a business with authorization checks
func (s *BusinessServiceImpl) UpdateUserRoleInBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole, newRole shared.UserRole) error {
	s.logger.Infof("Updating user %s role in business %s to %s (requester role: %s)", targetUserID, bid, newRole, requesterRole)

	// Validate new role is valid
	validRoles := []shared.UserRole{shared.Owner, shared.Admin, shared.Manager, shared.Auditor}
	isValidRole := false
	for _, validRole := range validRoles {
		if newRole == validRole {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
		return fmt.Errorf("invalid role. Must be one of: owner, admin, manager, auditor")
	}

	// Authorization: Only admin or owner can update users
	if requesterRole != shared.Owner && requesterRole != shared.Admin {
		return fmt.Errorf("only business owners and admins can update user roles")
	}

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Get target user's current role within transaction
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)
		relations, err := txBusinessUserRelRepo.FindByBusiness(ctx, bid)
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
		err = txBusinessUserRelRepo.UpdateUserRole(ctx, targetUserID, bid, newRole)
		if err != nil {
			return fmt.Errorf("error updating user role: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate user roles cache after successful update (outside transaction)
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
func (s *BusinessServiceImpl) RemoveUserFromBusiness(ctx context.Context, bid string, targetUserID string, requesterRole shared.UserRole) error {
	s.logger.Infof("Removing user %s from business %s (requester role: %s)", targetUserID, bid, requesterRole)

	// Authorization: Only admin or owner can remove users
	if requesterRole != shared.Owner && requesterRole != shared.Admin {
		return fmt.Errorf("only business owners and admins can remove users")
	}

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Get target user's role within transaction for authorization check
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)
		relations, err := txBusinessUserRelRepo.FindByBusiness(ctx, bid)
		if err != nil {
			return fmt.Errorf("error finding business relations: %w", err)
		}

		var targetUserRole shared.UserRole
		found := false
		for _, rel := range relations {
			if rel.UserID == targetUserID {
				targetUserRole = rel.Role
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("user not found in business")
		}

		// Authorization check: Admin cannot remove owner
		if requesterRole == shared.Admin && targetUserRole == shared.Owner {
			return fmt.Errorf("admin cannot remove owner from business")
		}

		// Remove user from business
		err = txBusinessUserRelRepo.RemoveUserFromBusiness(ctx, targetUserID, bid)
		if err != nil {
			return fmt.Errorf("error removing user from business: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate user roles cache after successful removal (outside transaction)
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

// GetBusiness retrieves basic business information
func (s *BusinessServiceImpl) GetBusiness(ctx context.Context, bid string) (*business.Business, error) {
	businessData, _, err := s.businessRepo.GetBusinessWithKYBData(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch business: %w", err)
	}

	if businessData == nil {
		return nil, fmt.Errorf("business not found: %s", bid)
	}

	return businessData, nil
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

// GetBusinessOnboardingFlags retrieves the onboarding flags for a business.
// All data is fetched fresh from database - no caching.
// showDashboard is determined solely by payment count (if >= 5 successful payments, flags are implicitly true).
func (s *BusinessServiceImpl) GetBusinessOnboardingFlags(ctx context.Context, bid string) (*business.BusinessOnboardingStatusDTO, error) {
	// Fetch business flags from database
	bus, err := s.businessRepo.FindByID(ctx, bid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch business details: %w", err)
	}

	// Count successful payments (real-time, always fresh)
	// If there are >= 5 successful payments, showDashboard = true
	// (No need to check flags first - if payments exist, flags must be true)
	count, err := s.paymentRepo.CountSuccessfulPayments(ctx, bid)
	if err != nil {
		s.logger.Warnf("Failed to count successful payments for business %s: %v", bid, err)
		// Don't fail the entire request, just keep showDashboard as false
		count = 0
	}

	showDashboard := (count >= 5)

	if showDashboard {
		s.logger.Infof("Dashboard enabled for business %s (successful payments: %d)", bid, count)
	} else {
		s.logger.Debugf("Dashboard not enabled for business %s (successful payments: %d < 5)", bid, count)
	}

	return &business.BusinessOnboardingStatusDTO{
		HasAddedPaymentMethod:  bus.HasAddedPaymentMethod,
		HasAddedVendor:         bus.HasAddedVendor,
		HasCreatedFirstPayment: bus.HasCreatedFirstPayment,
		ShowDashboard:          showDashboard,
	}, nil
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

	invalidationKey := keys.Business.OnboardingStatus(bid)
	if err := s.rclient.Delete(ctx, invalidationKey); err != nil {
		s.logger.Warnf("Failed to invalidate business onboarding status cache for BID %s: %v", bid, err)
	}


	return nil
}

// Ops Dashboard Methods

// ListBusinessesPaginated retrieves paginated list of businesses, optionally filtered by KYB status
func (s *BusinessServiceImpl) ListBusinessesPaginated(ctx context.Context, page int, limit int, kybStatus string) ([]*business.OpsBusinessListItem, int, error) {
	return s.businessRepo.ListBusinessesPaginated(ctx, page, limit, kybStatus)
}

// SearchBusinesses searches businesses by name or GSTIN, optionally filtered by KYB status
func (s *BusinessServiceImpl) SearchBusinesses(ctx context.Context, searchTerm string, kybStatus string) ([]*business.OpsBusinessSearchResult, error) {
	return s.businessRepo.SearchBusinesses(ctx, searchTerm, kybStatus)
}

// GetBusinessDetailsForOps retrieves complete business details for ops dashboard
func (s *BusinessServiceImpl) GetBusinessDetailsForOps(ctx context.Context, bid string) (*business.OpsBusinessDetails, error) {
	details, err := s.businessRepo.GetBusinessDetailsForOps(ctx, bid)
	if err != nil {
		return nil, err
	}

	// Generate presigned URL for business deed document if it exists
	if details.DocPath != "" {
		// Try to get S3 client from externalClients interface
		if s3Getter, ok := s.externalClients.(interface{ GetS3Client() *fileupload.S3Client }); ok {
			s3Client := s3Getter.GetS3Client()
			if s3Client != nil {
				// Generate presigned download URL with 15 minute expiration
				presignedURL, err := s3Client.GetPresignedDownloadURL(ctx, details.DocPath, 15*time.Minute)
				if err != nil {
					s.logger.Warnf("Failed to generate presigned URL for DocPath %s: %v", details.DocPath, err)
				} else {
					details.DocUrl = presignedURL
				}
			}
		}
	}

	return details, nil
}

// ReloadBusinessData reloads verification data for business (GSTIN, ITID, or CID)
func (s *BusinessServiceImpl) ReloadBusinessData(ctx context.Context, bid string, dataType string) error {
	s.logger.Infof("Reloading business data for bid=%s, type=%s", bid, dataType)

	// Get business to fetch required IDs
	bus, err := s.businessRepo.FindByID(ctx, bid)
	if err != nil {
		return fmt.Errorf("business not found: %w", err)
	}

	// Generate transaction ID
	transID := fmt.Sprintf("reload_%s_%s_%d", dataType, bid, time.Now().Unix())

	switch dataType {
	case "gstin":
		// Verify GSTIN and update data
		gstData, err := s.verificationService.VerifyGSTIN(ctx, bus.GSTIN, transID)
		if err != nil {
			return fmt.Errorf("GSTIN verification failed: %w", err)
		}

		// Convert to JSON
		gstDataJSON, err := json.Marshal(gstData.RawResponse)
		if err != nil {
			return fmt.Errorf("failed to marshal GST data: %w", err)
		}

		// Update in database
		return s.businessRepo.UpdateGSTINData(ctx, bid, string(gstDataJSON))

	case "income_tax_id":
		// Verify PAN and update data
		if bus.ITID == "" {
			return fmt.Errorf("income tax ID not set for business")
		}

		panData, err := s.verificationService.VerifyPANCOM(ctx, bus.ITID, transID)
		if err != nil {
			return fmt.Errorf("PAN verification failed: %w", err)
		}

		// Convert to JSON
		panDataJSON, err := json.Marshal(panData.PANCOMResponse)
		if err != nil {
			return fmt.Errorf("failed to marshal PAN data: %w", err)
		}

		// Update in database
		return s.businessRepo.UpdateIncometTaxIDData(ctx, bus.ITID, string(panDataJSON))

	case "company_id":
		// Get CID from KYB data
		_, kybData, err := s.businessRepo.GetBusinessWithKYBData(ctx, bid)
		if err != nil || kybData == nil || kybData.CID == "" {
			return fmt.Errorf("company ID not set for business")
		}

		// Verify CIN and update data
		cinData, err := s.verificationService.VerifyCIN(ctx, kybData.CID, transID)
		if err != nil {
			return fmt.Errorf("CIN verification failed: %w", err)
		}

		// Convert to JSON
		cinDataJSON, err := json.Marshal(cinData.RawResponse)
		if err != nil {
			return fmt.Errorf("failed to marshal CIN data: %w", err)
		}

		// Update in database
		return s.businessRepo.UpdateCompanyIDData(ctx, bus.ITID, string(cinDataJSON))

	default:
		return fmt.Errorf("invalid data type: %s (must be gstin, income_tax_id, or company_id)", dataType)
	}
}

// RejectKYB marks business KYB as rejected, sets owner role to NULL, and clears KYB data
func (s *BusinessServiceImpl) RejectKYB(ctx context.Context, bid string, notes string) error {
	s.logger.Infof("Rejecting KYB for business: %s", bid)

	// Get business to check ITID
	bus, err := s.businessRepo.FindByID(ctx, bid)
	if err != nil {
		return fmt.Errorf("business not found: %w", err)
	}

	// Find user who has kyb_initiated = true for this business
	var userIDToNotify string
	relations, err := s.businessUserRelRepo.FindByBusiness(ctx, bid)
	if err == nil {
		for _, rel := range relations {
			if rel.KybInitiated {
				userIDToNotify = rel.UserID
				break
			}
		}
	}

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBusinessRepo := s.businessRepo.WithTx(tx)
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)

		// 1. Set KYB status to rejected
		if err := txBusinessRepo.SetKYBStatusRejected(ctx, bid); err != nil {
			return err
		}

		// 2. Set owner role to NULL (database NULL)
		if err := txBusinessUserRelRepo.SetOwnerRoleToNull(ctx, bid); err != nil {
			// It's OK if no owner exists
			s.logger.Warnf("No owner found for business %s: %v", bid, err)
		}

		// 3. Clear KYB data
		if bus.ITID != "" {
			if err := txBusinessRepo.ClearKYBData(ctx, bus.ITID); err != nil {
				return err
			}
		}

		s.logger.Infof("KYB rejected successfully for business: %s", bid)
		return nil
	})

	if err != nil {
		return err
	}

	// Invalidate caches after KYB rejection
	go func() {
		bgCtx := context.Background()
		// Invalidate business details cache
		if err := s.rclient.Delete(bgCtx, keys.Business.Details(bid)); err != nil {
			s.logger.Warnf("Failed to invalidate business details cache for BID %s: %v", bid, err)
		}
		// Invalidate owner's user roles cache if an owner was found
		if userIDToNotify != "" {
			invalidationKeys := keys.InvalidateUserRoles(userIDToNotify)
			if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
				s.logger.Warnf("Failed to invalidate user roles cache for user %s after KYB rejection: %v", userIDToNotify, err)
			}
		}
	}()

	// Trigger notification if notes are present and user found
	if notes != "" && userIDToNotify != "" {
		go func() {
			bgCtx := context.Background()
			message := "Your business KYB verification has been rejected."
			if notes != "" {
				message += " Notes: " + notes
			}

			if err := notification.TriggerUnicast(bgCtx, bid, userIDToNotify, "kyb_rejected", "KYB Rejected", message, coreNotification.EntityReferences{}, nil); err != nil {
				s.logger.Errorf("Failed to trigger KYB rejection notification: %v", err)
			}
		}()
	}

	return nil
}

// VerifyKYB marks business KYB as verified and sets owner's KYC status to verified
func (s *BusinessServiceImpl) VerifyKYB(ctx context.Context, bid string, notes string) error {
	s.logger.Infof("Verifying KYB for business: %s", bid)

	var ownerUserID string

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txBusinessRepo := s.businessRepo.WithTx(tx)

		// 1. Set business KYB status to verified
		if err := txBusinessRepo.SetKYBStatusVerified(ctx, bid); err != nil {
			return err
		}

		// 2. Get owner user ID
		var err error
		ownerUserID, err = txBusinessRepo.GetOwnerUserID(ctx, bid)
		if err != nil {
			// It's OK if no owner exists yet
			s.logger.Warnf("No owner found for business %s: %v", bid, err)
			return nil
		}

		// 3. Set owner's KYC status to verified
		txUserRepo := s.userRepo.WithTx(tx)
		if err := txUserRepo.SetKYCStatusVerified(ctx, ownerUserID); err != nil {
			return fmt.Errorf("failed to verify owner KYC: %w", err)
		}

		s.logger.Infof("KYB and owner KYC verified successfully for business: %s", bid)
		return nil
	})

	if err != nil {
		return err
	}

	// Trigger notification if notes are present and owner found
	if notes != "" && ownerUserID != "" {
		go func() {
			bgCtx := context.Background()
			message := "Your business KYB verification has been approved!"
			if notes != "" {
				message += " Notes: " + notes
			}

			if err := notification.TriggerUnicast(bgCtx, bid, ownerUserID, "kyb_verified", "KYB Verified", message, coreNotification.EntityReferences{}, nil); err != nil {
				s.logger.Errorf("Failed to trigger KYB verification notification: %v", err)
			}
		}()
	}

	return nil
}

// OpsUpdateUserRole updates a user's role in a business (ops dashboard endpoint)
func (s *BusinessServiceImpl) OpsUpdateUserRole(ctx context.Context, bid string, userID string, role shared.UserRole) error {
	s.logger.Infof("Ops updating user %s role in business %s to %s", userID, bid, role)

	// Call repository to update role - it will return error if user-business relation doesn't exist
	err := s.businessUserRelRepo.OpsUpdateUserRole(ctx, bid, userID, role)
	if err != nil {
		return err
	}

	// Invalidate user roles cache after successful update
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUserRoles(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user roles cache for user %s: %v", userID, err)
		}
	}()

	s.logger.Infof("Successfully updated user %s role to %s in business %s via ops", userID, role, bid)
	return nil
}

// GetOwnerMobile retrieves the mobile number of the business owner
// This is a convenience method for payment gateways that require owner mobile
// (e.g., JioPay externalId = entityId + ownerMobile)
func (s *BusinessServiceImpl) GetOwnerMobile(ctx context.Context, businessID string) (string, error) {
	// Step 1: Get business owner user ID
	ownerUserID, err := s.businessUserRelRepo.GetBusinessOwnerUserID(ctx, businessID)
	if err != nil {
		return "", fmt.Errorf("failed to get business owner: %w", err)
	}

	// Step 2: Get owner's mobile number
	ownerMobile, err := s.userRepo.GetUserMobileByID(ctx, ownerUserID)
	if err != nil {
		return "", fmt.Errorf("failed to get owner mobile: %w", err)
	}

	return ownerMobile, nil
}
