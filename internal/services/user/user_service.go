package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"xcentro.in/internal/shared/config"
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
}

// NewUserService creates a new user service with dependency injection
func NewUserService(
	userRepo *userRepo.PgUserRepository,
	businessRepo businessRepo.BusinessRepository,
	businessUserRelRepo businessRepo.BusinessUserRelationRepository,
	txManager client.TxManager,
	emailService mailers.EmailService,
	verificationService *verification.DocumentVerificationService,
	config *config.Config,
	rclient *redisClient.RedisClient,
) user.UserService {
	return &UserServiceImpl{
		userRepo:            userRepo,
		businessRepo:        businessRepo,
		businessUserRelRepo: businessUserRelRepo,
		txManager:           txManager,
		emailService:        emailService,
		verificationService: verificationService,
		config:              config,
		rclient:             rclient,
		logger:              logger.GetDefault(),
	}
}

func (s *UserServiceImpl) JoinBusinessByGSTIN(ctx context.Context, userID string, gstin string, claimedRole shared.UserRole) error {
	s.logger.Info("User %s attempting to join business with GSTIN %s", userID, gstin)

	// 1. Find business by GSTIN
	bus, err := s.businessRepo.FindByGSTIN(ctx, gstin)
	if err != nil {
		return fmt.Errorf("error finding business by GSTIN: %w", err)
	}
	if bus == nil {
		return types.ErrBusinessNotFound
	}

	// 2. Check if user is already associated
	relations, err := s.businessUserRelRepo.FindByUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("error checking for existing user-business relation: %w", err)
	}
	for _, rel := range relations {
		if rel.BID == bus.BID {
			return types.ErrUserAlreadyAssociated
		}
	}

	// 3. Create new association
	relation := &businessRepo.BusinessUserRelation{
		UserID:       userID,
		BID:          bus.BID,
		Role:         "support", // Using claimedRole for both
		RoleClaim:    claimedRole,
		KybInitiated: false, // User is joining, not initiating
	}

	err = s.businessUserRelRepo.AddUserToBusiness(ctx, relation)
	if err != nil {
		return fmt.Errorf("error adding user to business: %w", err)
	}

	s.logger.Infof("User %s successfully joined business %s (BID: %s) with role claim %s", userID, bus.Name, bus.BID, claimedRole)

	return nil
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
		// Mask national ID - show only last 4 characters
		profile.MaskedNationalID = s.maskNationalID(identityData.NationalID)
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

// maskNationalID masks the national ID showing only the last 4 characters
func (s *UserServiceImpl) maskNationalID(nationalID string) string {
	if nationalID == "" {
		return ""
	}
	if len(nationalID) <= 4 {
		return nationalID
	}
	lastFour := nationalID[len(nationalID)-4:]
	maskedPart := ""
	for i := 0; i < len(nationalID)-4; i++ {
		maskedPart += "*"
	}
	return maskedPart + lastFour
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

	// Invalidate user cache after successful update
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUser(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user cache: %v", err)
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

	// Invalidate user cache after successful update
	go func() {
		bgCtx := context.Background()
		invalidationKeys := keys.InvalidateUser(userID)
		if err := s.rclient.Delete(bgCtx, invalidationKeys...); err != nil {
			s.logger.Warnf("Failed to invalidate user cache: %v", err)
		}
	}()

	return nil
}

// SendEmailVerification sends email verification token to user's email
func (s *UserServiceImpl) SendEmailVerification(ctx context.Context, userID string) error {
	s.logger.Info("Sending email verification for user: %s", userID)

	// Get user details
	userObj, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if userObj == nil {
		return fmt.Errorf("user not found")
	}

	if userObj.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Generate secure verification token (32 bytes = 256 bits)
	verificationToken, err := s.generateSecureToken()
	if err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Hash the token before storing in database (SECURITY: Never store plaintext tokens)
	hashedToken := s.hashToken(verificationToken)

	// Calculate proper timestamps
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour) // 24 hour expiry for email verification

	// Create verification code record with 24 hour expiry
	verificationCode := &user.UserVerificationCodes{
		UserID:        userID,
		Code:          hashedToken, // Store hashed version, not plaintext
		CodeType:      shared.EmailVerification,
		ExpiresAt:     expiresAt.Format(time.RFC3339),
		CreatedAt:     now.Format(time.RFC3339),
		Used:          false,
		RetryAttempts: 0,
	}

	// Store verification code in database
	if err := s.userRepo.CreateVerificationCode(ctx, verificationCode); err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// Create verification URL
	verificationURL := fmt.Sprintf("http://localhost:8888/v1/user/verify-email?token=%s", verificationToken)

	// Prepare email content
	htmlBody := fmt.Sprintf(`
		<html>
		<body>
			<h2>Email Verification - Xcentro</h2>
			<p>Hi there,</p>
			<p>Thank you for signing up with Xcentro. Please click the link below to verify your email address:</p>
			<p><a href="%s" style="background-color: #007bff; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px;">Verify Email</a></p>
			<p>Or copy and paste this link in your browser:</p>
			<p>%s</p>
			<p>This link will expire in 24 hours.</p>
			<p>If you didn't create an account with us, please ignore this email.</p>
			<p>Best regards,<br>Xcentro Team</p>
		</body>
		</html>
	`, verificationURL, verificationURL)

	textBody := fmt.Sprintf(`
		Email Verification - Xcentro
		
		Hi there,
		
		Thank you for signing up with Xcentro. Please click the link below to verify your email address:
		
		%s
		
		This link will expire in 24 hours.
		
		If you didn't create an account with us, please ignore this email.
		
		Best regards,
		Xcentro Team
	`, verificationURL)

	// Create email message
	emailMsg := &mailers.EmailMessage{
		To:       []string{userObj.Email},
		From:     s.config.SES.DefaultFromEmail,
		FromName: s.config.SES.DefaultFromName,
		Subject:  "Verify your Xcentro account",
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		s.logger.Errorf("Failed to send verification email to %s: %v", userObj.Email, err)
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	s.logger.Info("Verification email sent successfully to %s, Message ID: %s", userObj.Email, messageID)

	return nil
}

// VerifyEmailWithToken verifies user's email using verification token
func (s *UserServiceImpl) VerifyEmailWithToken(ctx context.Context, token string) error {
	s.logger.Info("Verifying email with token")

	if token == "" {
		return fmt.Errorf("verification token is required")
	}

	return s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Hash the incoming token to compare with stored hash
		hashedToken := s.hashToken(token)

		// Get verification code record using hashed token
		verificationCode, err := txUserRepo.GetVerificationCodeByCode(ctx, hashedToken, shared.EmailVerification)
		if err != nil {
			s.logger.Errorf("Failed to get verification code: %v", err)
			return fmt.Errorf("invalid or expired verification token")
		}

		if verificationCode == nil {
			return fmt.Errorf("verification token not found")
		}

		if verificationCode.Used {
			return fmt.Errorf("verification token already used")
		}

		// Check if token is expired
		expiresAt, err := time.Parse(time.RFC3339, verificationCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(expiresAt) {
			return fmt.Errorf("verification token has expired")
		}

		// Mark verification code as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark verification code as used: %w", err)
		}

		// Update user email verification status
		if err := txUserRepo.UpdateUserEmailVerification(ctx, verificationCode.UserID, true); err != nil {
			return fmt.Errorf("failed to update user email verification: %w", err)
		}

		s.logger.Info("Email verified successfully for user: %s", verificationCode.UserID)
		return nil
	})
}

// generateSecureToken generates a cryptographically secure random token
func (s *UserServiceImpl) generateSecureToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Convert to hex string
	return hex.EncodeToString(randomBytes), nil
}

// hashToken creates a SHA-256 hash of the token for secure database storage
// SECURITY: This prevents token compromise if database is breached
func (s *UserServiceImpl) hashToken(token string) string {
	// Use application-wide salt from environment/config
	appSalt := "xcentro_email_verification_salt_2024"

	// Combine token with application salt
	saltedToken := fmt.Sprintf("%s_%s_%s", appSalt, token, "xcentro")

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(saltedToken))

	// Return hex-encoded hash
	return hex.EncodeToString(hash[:])
}

// ResendEmailVerification resends email verification for a given email
func (s *UserServiceImpl) ResendEmailVerification(ctx context.Context, email string) error {
	s.logger.Info("Resending email verification for email: %s", email)

	if email == "" {
		return fmt.Errorf("email is required")
	}

	// Get user details
	userObj, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if userObj == nil {
		return fmt.Errorf("user not found")
	}

	if userObj.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Send verification email
	return s.SendEmailVerification(ctx, userObj.Id)
}

// InitiateOnboarding initiates user onboarding with tax ID and national ID verification
// User KYC is independent of business - this only verifies identity
func (s *UserServiceImpl) InitiateOnboarding(
	ctx context.Context,
	userID, name, dob, taxID, nationalID string,
) (*user.InitiateOnboardingResponseDTO, error) {
	s.logger.Info("Initiating onboarding for user %s", userID)

	// Input validation
	if userID == "" {
		return nil, types.NewInvalidKYCInputError("userID", "required")
	}
	if name == "" || dob == "" || taxID == "" || nationalID == "" {
		return nil, types.NewInvalidKYCInputError("user details", "name, dob, taxID, and nationalID are required")
	}

	// Get current user
	currentUser, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, types.ErrUserNotFound
	}

	// Check if KYC already completed or verified
	if currentUser.KycStatus == shared.KYCCompleted || currentUser.KycStatus == shared.KYCVerified {
		return nil, types.ErrKYCAlreadyCompleted
	}

	var response *user.InitiateOnboardingResponseDTO

	// Execute all operations in a transaction
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Step 1: Verify PAN via PAN_COM
		s.logger.Info("Verifying PAN (tax ID) for user %s", userID)
		panComTxnID := fmt.Sprintf("pancom_%s_%d", userID, time.Now().Unix())
		panComResult, err := s.verificationService.VerifyPANCOM(ctx, taxID, panComTxnID)
		if err != nil {
			return types.NewVerificationFailedError("Tax ID", err.Error())
		}

		// Step 2: Verify PAN-Aadhaar link
		s.logger.Info("Verifying Tax ID-National ID link for user %s", userID)
		panAadhaarTxnID := fmt.Sprintf("panaadhaar_%s_%d", userID, time.Now().Unix())
		panAadhaarResult, err := s.verificationService.VerifyPANAadhaar(ctx, taxID, nationalID, panAadhaarTxnID)
		if err != nil {
			return types.NewVerificationFailedError("Tax ID-National ID link", err.Error())
		}

		// Step 3: Combine verification data
		taxIDVerificationData := &user.UserTaxIDVerificationData{
			PANCOMData:     panComResult.PANCOMResponse,
			PANAadhaarLink: panAadhaarResult,
		}

		taxIDDataJSON, err := json.Marshal(taxIDVerificationData)
		if err != nil {
			return fmt.Errorf("failed to marshal tax ID verification data: %w", err)
		}

		// Step 4: Create or update user identity
		identity := &user.UserIdentity{
			TaxID:          taxID,
			TaxIDData:      string(taxIDDataJSON),
			NationalID:     nationalID,
			NationalIDData: "{}", // Will be populated after Aadhaar verification
			NTxnID:         "",   // Will be set when generating Aadhaar link
		}

		if err := txUserRepo.UpsertUserIdentity(ctx, identity); err != nil {
			return fmt.Errorf("failed to create user identity: %w", err)
		}

		// Step 5: Update user name, dob, and tax_id reference
		if err := txUserRepo.UpdateUserNameAndDOB(ctx, userID, name, dob); err != nil {
			return fmt.Errorf("failed to update user name and DOB: %w", err)
		}

		if err := txUserRepo.UpdateUserTaxID(ctx, userID, taxID); err != nil {
			return fmt.Errorf("failed to link user to identity: %w", err)
		}

		// Step 6: Set KYC status to in_progress
		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCInProgress); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		// Step 7: Generate Aadhaar DigiLocker link
		s.logger.Info("Generating Aadhaar verification link for user %s", userID)

		// Create authbridge client for Aadhaar link generation
		authClient, err := authbridge.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create authentication client: %w", err)
		}

		// Generate unique transaction ID for Aadhaar
		aadhaarTxnID := fmt.Sprintf("aadhaar_%s_%d", userID, time.Now().Unix())
		linkRequest := authbridge.NewAadhaarLinkRequest(aadhaarTxnID, "", "http://localhost:8888/v1/callback")

		// Call authbridge API
		aadhaarResponse, err := authClient.SearchDocument(linkRequest)
		if err != nil {
			return fmt.Errorf("failed to generate Aadhaar link: %w", err)
		}

		// Extract URL and transaction ID from response
		status, ok := aadhaarResponse["status"].(float64)
		if !ok || status != 1 {
			return fmt.Errorf("aadhaar link generation failed: invalid response status")
		}

		data, ok := aadhaarResponse["data"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid aadhaar response format: missing data")
		}

		aadhaarURL, ok := data["url"].(string)
		if !ok || aadhaarURL == "" {
			return fmt.Errorf("invalid aadhaar response format: missing or empty URL")
		}

		tsTransID, ok := aadhaarResponse["ts_trans_id"].(string)
		if !ok || tsTransID == "" {
			return fmt.Errorf("invalid aadhaar response format: missing transaction ID")
		}

		// Step 8: Store Aadhaar transaction ID
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
// Supports two flows:
// 1. Create new business: businessID (from header) + claimedRole
// 2. Join existing business: gstin + claimedRole
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

	// Check if user is invited (has existing business relations)
	relationCount, err := s.userRepo.CountBusinessRelations(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check business relations: %w", err)
	}

	isInvitedUser := relationCount > 0
	var targetBusinessID string

	// Determine which flow: create new business OR join existing business
	if !isInvitedUser {
		// User is completing KYC and needs to associate with a business
		if claimedRole == "" {
			return types.NewInvalidKYCInputError("claimedRole", "required")
		}

		if businessID != "" {
			// Flow 1: Create new business (businessID from header)
			targetBusinessID = businessID
			s.logger.Info("User %s completing KYC to create/own business %s", userID, businessID)
		} else if gstin != "" {
			// Flow 2: Join existing business (gstin from body)
			s.logger.Info("User %s completing KYC to join business with GSTIN %s", userID, gstin)

			// Find business by GSTIN
			business, err := s.businessRepo.FindByGSTIN(ctx, gstin)
			if err != nil {
				return fmt.Errorf("error finding business by GSTIN: %w", err)
			}
			if business == nil {
				return types.ErrBusinessNotFound
			}
			targetBusinessID = business.BID

			// Create business relation for joining user
			relation := &businessRepo.BusinessUserRelation{
				UserID:       userID,
				BID:          targetBusinessID,
				Role:         "support",
				RoleClaim:    claimedRole,
				KybInitiated: false, // User is joining, not creating
			}

			err = s.businessUserRelRepo.AddUserToBusiness(ctx, relation)
			if err != nil {
				return fmt.Errorf("error adding user to business: %w", err)
			}
			s.logger.Info("User %s successfully joined business %s (GSTIN: %s)", userID, targetBusinessID, gstin)
		} else {
			return types.NewInvalidKYCInputError("businessID or gstin", "one of businessID or gstin is required")
		}
	} else {
		// Invited user - already has business relation, no need for businessID or gstin
		s.logger.Info("User %s is invited, skipping business association", userID)
	}

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)
		txBusinessUserRelRepo := s.businessUserRelRepo.WithTx(tx)

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

		if err := txUserRepo.UpdateNationalIDData(ctx, userData.TaxID, string(responseJSON)); err != nil {
			return fmt.Errorf("failed to update national ID data: %w", err)
		}

		// Step 5: Set KYC status to completed
		if err := txUserRepo.UpdateUserKYCStatus(ctx, userID, shared.KYCCompleted); err != nil {
			return fmt.Errorf("failed to update KYC status: %w", err)
		}

		// Step 6: Update role_claim ONLY for non-invited users who created a business
		if !isInvitedUser && businessID != "" && targetBusinessID != "" {
			// Only update role_claim if user is creating a business (not joining via GSTIN)
			if err := txBusinessUserRelRepo.UpdateRoleClaim(ctx, userID, targetBusinessID, claimedRole); err != nil {
				return fmt.Errorf("failed to update role claim for business %s: %w", targetBusinessID, err)
			}
			s.logger.Info("Onboarding completed successfully - role claim %s set for user %s in business %s", claimedRole, userID, targetBusinessID)
		} else if !isInvitedUser && gstin != "" {
			s.logger.Info("Onboarding completed successfully for user %s joining business via GSTIN %s", userID, gstin)
		} else {
			s.logger.Info("Onboarding completed successfully for invited user %s - skipping role claim", userID)
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

	// Restrict access after KYC is completed or verified
	if userData.KycStatus == shared.KYCCompleted || userData.KycStatus == shared.KYCVerified {
		return nil, fmt.Errorf("onboarding status is only available during onboarding process")
	}

	// Prepare response DTO
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

// SendPasswordResetEmail sends password reset token to user's email
// SECURITY: Always returns success to prevent email enumeration
func (s *UserServiceImpl) SendPasswordResetEmail(ctx context.Context, email string) error {
	s.logger.Info("Password reset requested for email: %s", email)

	if email == "" {
		// Still return nil to prevent enumeration
		return nil
	}

	// Get user details - if not found, silently return success
	userObj, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil || userObj == nil {
		s.logger.Info("Password reset requested for non-existent email: %s", email)
		// SECURITY: Return success even if user doesn't exist
		return nil
	}

	// Generate secure verification token (32 bytes = 256 bits)
	resetToken, err := s.generateSecureToken()
	if err != nil {
		s.logger.Errorf("Failed to generate reset token: %v", err)
		// Return success to prevent information disclosure
		return nil
	}

	// Hash the token before storing in database
	hashedToken := s.hashToken(resetToken)

	// Calculate timestamps - 1 hour expiry for password reset
	now := time.Now()
	expiresAt := now.Add(1 * time.Hour)

	// Create verification code record
	verificationCode := &user.UserVerificationCodes{
		UserID:        userObj.Id,
		Code:          hashedToken,
		CodeType:      shared.PasswordReset,
		ExpiresAt:     expiresAt.Format(time.RFC3339),
		CreatedAt:     now.Format(time.RFC3339),
		Used:          false,
		RetryAttempts: 0,
	}

	// Store verification code in database
	if err := s.userRepo.CreateVerificationCode(ctx, verificationCode); err != nil {
		s.logger.Errorf("Failed to create password reset code: %v", err)
		// Return success to prevent information disclosure
		return nil
	}

	// Prepare email content with just the token
	htmlBody := fmt.Sprintf(`
		<html>
		<body>
			<h2>Password Reset - Xcentro</h2>
			<p>Hi there,</p>
			<p>We received a request to reset your password. Please use the following verification code:</p>
			<p style="font-size: 20px; font-weight: bold; background-color: #f0f0f0; padding: 10px; border-radius: 5px; display: inline-block;">%s</p>
			<p>This code will expire in 1 hour.</p>
			<p>If you didn't request a password reset, please ignore this email and your password will remain unchanged.</p>
			<p><strong>Security Note:</strong> Never share this code with anyone. Xcentro staff will never ask for your verification code.</p>
			<p>Best regards,<br>Xcentro Team</p>
		</body>
		</html>
	`, resetToken)

	textBody := fmt.Sprintf(`
		Password Reset - Xcentro

		Hi there,

		We received a request to reset your password. Please use the following verification code:

		%s

		This code will expire in 1 hour.

		If you didn't request a password reset, please ignore this email and your password will remain unchanged.

		Security Note: Never share this code with anyone. Xcentro staff will never ask for your verification code.

		Best regards,
		Xcentro Team
	`, resetToken)

	// Create email message
	emailMsg := &mailers.EmailMessage{
		To:       []string{userObj.Email},
		From:     s.config.SES.DefaultFromEmail,
		FromName: s.config.SES.DefaultFromName,
		Subject:  "Reset your Xcentro password",
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		s.logger.Errorf("Failed to send password reset email to %s: %v", userObj.Email, err)
		// Return success to prevent information disclosure
		return nil
	}

	s.logger.Info("Password reset email sent successfully to %s, Message ID: %s", userObj.Email, messageID)
	return nil
}

// ResetPasswordWithToken resets user's password using verification token
func (s *UserServiceImpl) ResetPasswordWithToken(ctx context.Context, token string, newPasswordHash string) error {
	s.logger.Info("Resetting password with token")

	if token == "" {
		return fmt.Errorf("reset token is required")
	}

	if newPasswordHash == "" {
		return fmt.Errorf("new password is required")
	}

	return s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Hash the incoming token to compare with stored hash
		hashedToken := s.hashToken(token)

		// Get verification code record using hashed token
		verificationCode, err := txUserRepo.GetVerificationCodeByCode(ctx, hashedToken, shared.PasswordReset)
		if err != nil {
			s.logger.Errorf("Failed to get password reset code: %v", err)
			return fmt.Errorf("invalid or expired reset token")
		}

		if verificationCode == nil {
			return fmt.Errorf("reset token not found")
		}

		if verificationCode.Used {
			return fmt.Errorf("reset token already used")
		}

		// Check if token is expired
		expiresAt, err := time.Parse(time.RFC3339, verificationCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(expiresAt) {
			return fmt.Errorf("reset token has expired")
		}

		// Mark verification code as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark reset code as used: %w", err)
		}

		// Update user password hash
		if err := txUserRepo.UpdateUserPasswordHash(ctx, verificationCode.UserID, newPasswordHash); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		s.logger.Info("Password reset successfully for user: %s", verificationCode.UserID)
		return nil
	})
}

// generateRandomPassword generates a cryptographically secure random password
func (s *UserServiceImpl) generateRandomPassword() (string, error) {
	const (
		length      = 16
		lowerChars  = "abcdefghijklmnopqrstuvwxyz"
		upperChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digitChars  = "0123456789"
		symbolChars = "!@#$%^&*"
		allChars    = lowerChars + upperChars + digitChars + symbolChars
	)

	password := make([]byte, length)

	// Ensure at least one character from each category
	categories := []string{lowerChars, upperChars, digitChars, symbolChars}
	for i := 0; i < 4; i++ {
		category := categories[i]
		randomIndex := make([]byte, 1)
		if _, err := rand.Read(randomIndex); err != nil {
			return "", fmt.Errorf("failed to generate random byte: %w", err)
		}
		password[i] = category[int(randomIndex[0])%len(category)]
	}

	// Fill remaining characters randomly from all characters
	for i := 4; i < length; i++ {
		randomIndex := make([]byte, 1)
		if _, err := rand.Read(randomIndex); err != nil {
			return "", fmt.Errorf("failed to generate random byte: %w", err)
		}
		password[i] = allChars[int(randomIndex[0])%len(allChars)]
	}

	// Shuffle the password to randomize character positions
	for i := length - 1; i > 0; i-- {
		randomIndex := make([]byte, 1)
		if _, err := rand.Read(randomIndex); err != nil {
			return "", fmt.Errorf("failed to generate random byte: %w", err)
		}
		j := int(randomIndex[0]) % (i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// CreateInvitedUser creates a new user for business invitation (email-first to avoid deadlock)
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

	// Generate random password
	randomPassword, err := s.generateRandomPassword()
	if err != nil {
		return nil, fmt.Errorf("failed to generate random password: %w", err)
	}

	// IMPORTANT: Send email FIRST before creating user (to avoid deadlock)
	htmlBody := fmt.Sprintf(`
		<html>
		<body>
			<h2>Welcome to Xcentro!</h2>
			<p>Hi %s,</p>
			<p>You have been invited to join a business on Xcentro platform.</p>
			<p>Your temporary login credentials are:</p>
			<p><strong>Email:</strong> %s</p>
			<p><strong>Temporary Password:</strong> <code style="background-color: #f0f0f0; padding: 5px 10px; border-radius: 3px;">%s</code></p>
			<p><strong>⚠️ IMPORTANT:</strong> Please reset your password immediately after logging in for security purposes.</p>
			<p>If you didn't expect this invitation, please contact the business administrator.</p>
			<p>Best regards,<br>Xcentro Team</p>
		</body>
		</html>
	`, name, email, randomPassword)

	textBody := fmt.Sprintf(`
		Welcome to Xcentro!

		Hi %s,

		You have been invited to join a business on Xcentro platform.

		Your temporary login credentials are:
		Email: %s
		Temporary Password: %s

		⚠️ IMPORTANT: Please reset your password immediately after logging in for security purposes.

		If you didn't expect this invitation, please contact the business administrator.

		Best regards,
		Xcentro Team
	`, name, email, randomPassword)

	emailMsg := &mailers.EmailMessage{
		To:       []string{email},
		From:     s.config.SES.DefaultFromEmail,
		FromName: s.config.SES.DefaultFromName,
		Subject:  "You've been invited to Xcentro",
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		s.logger.Errorf("Failed to send invitation email to %s: %v", email, err)
		return nil, fmt.Errorf("failed to send invitation email: %w", err)
	}

	s.logger.Info("Invitation email sent successfully to %s, Message ID: %s", email, messageID)

	// Hash the password
	hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	hashedPassword := string(hashedPasswordBytes)

	// Create user with hashed password
	createdUser, err := s.userRepo.CreateInvitedUser(ctx, email, hashedPassword, name, kycNeeded)
	if err != nil {
		return nil, fmt.Errorf("failed to create invited user: %w", err)
	}

	s.logger.Info("Invited user created successfully with ID: %s", createdUser.Id)
	return createdUser, nil
}
