package user

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"xcentro.in/internal/adapters/database/postgres/client"
	userRepo "xcentro.in/internal/adapters/database/postgres/user"
	"xcentro.in/internal/adapters/providers/authbridge"
	"xcentro.in/internal/core/auth"
	businessRepo "xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// UserServiceImpl provides business logic for user operations
type UserServiceImpl struct {
	userRepo            user.UserRepository
	businessUserRelRepo businessRepo.BusinessUserRelationRepository
	txManager           client.TxManager
	logger              *logger.Logger
}

// NewUserService creates a new user service with dependency injection
func NewUserService(
	userRepo *userRepo.PgUserRepository,
	businessUserRelRepo businessRepo.BusinessUserRelationRepository,
	txManager client.TxManager,
) user.UserService {
	return &UserServiceImpl{
		userRepo:            userRepo,
		businessUserRelRepo: businessUserRelRepo,
		txManager:           txManager,
		logger:              logger.GetDefault(),
	}
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
			return nil
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

// GetUserByEmail retrieves a user by email
func (s *UserServiceImpl) GetUserByEmail(ctx context.Context, email string) (*user.User, error) {
	s.logger.Info("Getting user by email: %s", email)

	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		s.logger.Error("Failed to get user by email: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *UserServiceImpl) GetUserByID(ctx context.Context, userID string) (*user.User, error) {
	s.logger.Info("Getting user by ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user by ID: %v", err)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// UpdateUserProfile updates user profile information in a transaction
func (s *UserServiceImpl) UpdateUserProfile(ctx context.Context, userID string, mobile, taxID string) error {
	s.logger.Info("Updating user profile for ID: %s", userID)

	return s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if mobile != "" {
			if err := txUserRepo.UpdateUserMobile(ctx, userID, mobile); err != nil {
				return fmt.Errorf("failed to update mobile: %w", err)
			}
		}

		if taxID != "" {
			if err := txUserRepo.UpdateUserTaxID(ctx, userID, taxID); err != nil {
				return fmt.Errorf("failed to update tax ID: %w", err)
			}
		}

		return nil
	})
}

// UpdateKYCStatus updates user's KYC status
func (s *UserServiceImpl) UpdateKYCStatus(ctx context.Context, userID string, status shared.KYCStatus) error {
	s.logger.Info("Updating KYC status for user ID: %s to status: %v", userID, status)

	return s.txManager.WithTx(context.Background(), func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, status); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		return nil
	})
}

// InitiateKYC initiates KYC process by setting claimed role and creating identity records
func (s *UserServiceImpl) InitiateKYC(ctx context.Context, userID string, claimedRole shared.UserRole, taxID, nationalID string) error {
	s.logger.Info("Initiating KYC for user ID: %s with claimed role: %v", userID, claimedRole)

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	if taxID == "" {
		return fmt.Errorf("tax ID is required for KYC")
	}

	// Check current KYC status - should not allow if already completed or verified
	currentUser, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user information: %w", err)
	}

	if currentUser.KycStatus == shared.KYCCompleted || currentUser.KycStatus == shared.KYCVerified {
		return fmt.Errorf("KYC already completed or verified")
	}

	// Validate that owner/admin roles require KYC
	if claimedRole != shared.Owner && claimedRole != shared.Admin {
		return fmt.Errorf("KYC is only required for Owner and Admin roles")
	}

	return s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)

		// Create or update user identity
		identity := &user.UserIdentity{
			TaxID:          taxID,
			TaxIDData:      "{}", // Empty JSON initially
			NationalID:     nationalID,
			NationalIDData: "{}", // Empty JSON initially
		}

		if err := txUserRepo.UpsertUserIdentity(ctx, identity); err != nil {
			return fmt.Errorf("failed to create user identity: %w", err)
		}

		// Update user to reference the identity and set KYC status
		if err := txUserRepo.UpdateUserTaxID(ctx, userID, taxID); err != nil {
			return fmt.Errorf("failed to link user to identity: %w", err)
		}

		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCInProgress); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		// Find user's business relationships and update role claims
		relations, err := txBusinessUserRelRepo.FindByUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to find user business relations: %w", err)
		}

		// Update role claim in all business relationships
		for _, relation := range relations {
			if err := txBusinessUserRelRepo.UpdateUserRole(ctx, userID, relation.BID, claimedRole); err != nil {
				return fmt.Errorf("failed to update role claim for business %s: %w", relation.BID, err)
			}
		}

		s.logger.Info("KYC initiated successfully for user %s", userID)
		return nil
	})
}

// GetUserKYCStatus returns user KYC status and identity data as JSON (similar to business service)
func (s *UserServiceImpl) GetUserKYCStatus(ctx context.Context, userID string) (*user.UserResponseDTO, error) {
	s.logger.Info("Getting KYC status for user ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	userData, identityData, err := s.userRepo.GetUserKYCStatus(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user KYC status: %w", err)
	}

	if userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Check if KYC is in appropriate state (not completed or verified)
	if userData.KycStatus == shared.KYCVerified || userData.KycStatus == shared.KYCCompleted {
		return nil, fmt.Errorf("KYC already completed and verified")
	}

	type response struct {
		User     *user.User         `json:"user"`
		Identity *user.UserIdentity `json:"identity"`
	}

	// Ensure we always return a valid (possibly empty) identity object
	var identity *user.UserIdentity
	if identityData != nil {
		identity = identityData
	} else {
		identity = &user.UserIdentity{
			TaxID:          "",
			TaxIDData:      "{}",
			NationalID:     "",
			NationalIDData: "{}",
		}
	}

	resp := &user.UserResponseDTO{
		Id:             userData.Id,
		Email:          userData.Email,
		Mobile:         userData.Mobile,
		TaxID:          identity.TaxID,
		NationalID:     identity.NationalID,
		KycStatus:      userData.KycStatus,
		EmailVerified:  userData.EmailVerified,
		MobileVerified: userData.MobileVerified,
	}

	return resp, nil
}

// VerifyEmail verifies user's email using verification code
func (s *UserServiceImpl) VerifyEmail(ctx context.Context, userID, code string) error {
	s.logger.Info("Verifying email for user ID: %s", userID)

	verified, err := s.userRepo.VerifyVerificationCode(ctx, userID, code, shared.EmailVerification)
	if err != nil {
		return fmt.Errorf("failed to verify email code: %w", err)
	}

	if !verified {
		return fmt.Errorf("invalid or expired verification code")
	}

	s.logger.Info("Email verified successfully for user %s", userID)
	return nil
}

// VerifyMobile verifies user's mobile using verification code
func (s *UserServiceImpl) VerifyMobile(ctx context.Context, userID, code string) error {
	s.logger.Info("Verifying mobile for user ID: %s", userID)

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	if code == "" {
		return fmt.Errorf("verification code is required")
	}

	verified, err := s.userRepo.VerifyVerificationCode(ctx, userID, code, shared.MobileVerification)
	if err != nil {
		return fmt.Errorf("failed to verify mobile code: %w", err)
	}

	if !verified {
		return fmt.Errorf("invalid or expired verification code")
	}

	s.logger.Info("Mobile verified successfully for user %s", userID)
	return nil
}

// SendVerificationCode creates and stores a verification code (integration with external services would be separate)
func (s *UserServiceImpl) SendVerificationCode(ctx context.Context, userID string, codeType shared.VerificationType) error {
	s.logger.Info("Sending verification code for user ID: %s, type: %v", userID, codeType)

	// Get user to validate
	userData, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || userData == nil {
		return fmt.Errorf("user not found")
	}

	// Generate a simple 6-digit code (in production, use crypto/rand)
	code := fmt.Sprintf("%06d", 123456) // Placeholder - replace with proper code generation

	verificationCode := &user.UserVerificationCodes{
		UserID:        userID,
		Code:          code,
		CodeType:      codeType,
		ExpiresAt:     "2025-10-01T00:00:00Z", // Placeholder - calculate proper expiry
		CreatedAt:     "2025-09-30T00:00:00Z", // Placeholder - use NOW()
		Used:          false,
		RetryAttempts: 0,
	}

	if err := s.userRepo.CreateVerificationCode(ctx, verificationCode); err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// TODO: Integrate with SMS/Email service to actually send the code
	s.logger.Info("Verification code created (not sent - integration needed) for user %s", userID)
	return nil
}

// ClaimBusinessRole allows user to claim a role in a business they initiated KYB for
func (s *UserServiceImpl) ClaimBusinessRole(ctx context.Context, userID, businessID string, role shared.UserRole) error {
	s.logger.Info("User %s claiming role %v in business %s", userID, role, businessID)

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	if businessID == "" {
		return fmt.Errorf("business ID is required")
	}

	if role == "" {
		return fmt.Errorf("role is required")
	}

	// Check if user has initiated KYB for this business
	relations, err := s.businessUserRelRepo.FindByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check business relations: %w", err)
	}

	var targetRelation *businessRepo.BusinessUserRelation
	for _, rel := range relations {
		if rel.BID == businessID && rel.KybInitiated {
			targetRelation = rel
			break
		}
	}

	if targetRelation == nil {
		return fmt.Errorf("user not associated with this business")
	}

	if !targetRelation.KybInitiated {
		return fmt.Errorf("KYB not initiated for this business")
	}

	// Update role claim
	if err := s.businessUserRelRepo.UpdateUserRole(ctx, userID, businessID, role); err != nil {
		return fmt.Errorf("failed to update role claim: %w", err)
	}

	s.logger.Info("Role claim updated successfully for user %s in business %s", userID, businessID)
	return nil
}

// GenerateAadhaarLink generates an Aadhaar verification link and stores transaction ID
func (s *UserServiceImpl) GenerateAadhaarLink(ctx context.Context, userID string) (*user.AadhaarLinkResponse, error) {
	s.logger.Info("Generating Aadhaar link for user ID: %s", userID)

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	// Get user data to ensure user exists and has tax ID
	userData, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || userData == nil {
		return nil, fmt.Errorf("user not found")
	}

	if userData.TaxID == "" {
		return nil, fmt.Errorf("user must have tax ID (PAN) before Aadhaar verification")
	}

	// Check if user already has KYC completed/verified
	if userData.KycStatus == shared.KYCCompleted || userData.KycStatus == shared.KYCVerified {
		return nil, fmt.Errorf("KYC already completed or verified")
	}

	var result *user.AadhaarLinkResponse

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Create authbridge client
		authClient, err := s.createAuthBridgeClient()
		if err != nil {
			return fmt.Errorf("failed to create authentication client: %w", err)
		}

		// Generate unique transaction ID for this request
		transactionID := fmt.Sprintf("aadhaar_%s_%d", userID, time.Now().Unix())

		// Create Aadhaar link request (empty callback and redirect URLs for now)
		linkRequest := authbridge.NewAadhaarLinkRequest(transactionID, "", "")

		// Call authbridge API
		response, err := authClient.SearchDocument(linkRequest)
		if err != nil {
			return fmt.Errorf("failed to generate Aadhaar link: %w", err)
		}

		// Check response status
		status, ok := response["status"].(float64)
		if !ok || status != 1 {
			return fmt.Errorf("Aadhaar link generation failed: invalid response status")
		}

		// Extract URL from response
		data, ok := response["data"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid response format: missing data")
		}

		url, ok := data["url"].(string)
		if !ok || url == "" {
			return fmt.Errorf("invalid response format: missing or empty URL")
		}

		// Extract transaction ID from response
		tsTransID, ok := response["ts_trans_id"].(string)
		if !ok || tsTransID == "" {
			return fmt.Errorf("invalid response format: missing transaction ID")
		}

		// Store transaction ID in database
		if err := txUserRepo.UpdateAadhaarTransactionID(ctx, userData.TaxID, tsTransID); err != nil {
			return fmt.Errorf("failed to store transaction ID: %w", err)
		}

		s.logger.Info("Aadhaar link generated successfully for user %s, transaction ID: %s", userID, tsTransID)

		result = &user.AadhaarLinkResponse{
			URL:   url,
			TxnID: tsTransID,
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to generate Aadhaar link: %v", err)
		return nil, err
	}

	return result, nil
}

// CompleteAadhaarVerification completes the Aadhaar verification process
func (s *UserServiceImpl) CompleteAadhaarVerification(ctx context.Context, userID string) error {
	s.logger.Info("Completing Aadhaar verification for user ID: %s", userID)

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	// Get user data
	userData, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil || userData == nil {
		return fmt.Errorf("user not found")
	}

	if userData.TaxID == "" {
		return fmt.Errorf("user must have tax ID (PAN) configured")
	}

	return s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get stored transaction ID
		tsTransID, err := txUserRepo.GetAadhaarTransactionID(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to get transaction ID: %w", err)
		}

		// Create authbridge client
		authClient, err := s.createAuthBridgeClient()
		if err != nil {
			return fmt.Errorf("failed to create authentication client: %w", err)
		}

		// Create status request
		statusRequest := authbridge.NewAadhaarStatusRequest(tsTransID)

		// Call authbridge API for status
		response, err := authClient.SearchDocument(statusRequest)
		if err != nil {
			return fmt.Errorf("failed to get Aadhaar verification status: %w", err)
		}

		// Convert entire response to JSON and store it
		responseJSON, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("failed to marshal response data: %w", err)
		}

		// Update national ID data with the response
		if err := txUserRepo.UpdateNationalIDData(ctx, userData.TaxID, string(responseJSON)); err != nil {
			return fmt.Errorf("failed to update national ID data: %w", err)
		}

		s.logger.Info("Aadhaar verification completed successfully for user %s", userID)
		return nil
	})
}

// createAuthBridgeClient is a helper method to create authbridge client
func (s *UserServiceImpl) createAuthBridgeClient() (*authbridge.AuthBridgeClient, error) {
	return authbridge.NewClient()
}
