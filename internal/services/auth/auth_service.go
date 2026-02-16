package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/adapters/providers/sms"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	sharedClients "xcentro.in/internal/shared/clients"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/email"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/security"
)

type AuthService struct {
	config              *config.JWTSecretConfig
	bcryptConfig        *config.BcryptConfig
	rclient             *redis.RedisClient
	sessionRepo         auth.SessionRepository
	deviceVersionRepo   auth.DeviceVersionRepository
	businessUserRelRepo business.BusinessUserRelationRepository
	userRepo            user.UserRepository
	userService         user.UserService
	businessService     business.BusinessService
	emailService        mailers.EmailService
	smsService          sms.SMSService
	smsConfig           *config.SMSConfig
	metrics             metrics.MetricsProvider
	appConfig           *config.Config
	txManager           client.TxManager
	rateLimiter         *ratelimiter.RateLimiter
}

func NewAuthService(
	sessionRepo auth.SessionRepository,
	deviceVersionRepo auth.DeviceVersionRepository,
	businessUserRelRepo business.BusinessUserRelationRepository,
	userRepo user.UserRepository,
	userService user.UserService,
	businessService business.BusinessService,
	clients sharedClients.ExternalClients,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		config:              &cfg.JWT,                 // Extract JWT config
		bcryptConfig:        &cfg.Bcrypt,              // Extract Bcrypt config
		rclient:             clients.GetRedisClient(), // Extract from clients
		sessionRepo:         sessionRepo,
		deviceVersionRepo:   deviceVersionRepo,
		businessUserRelRepo: businessUserRelRepo,
		userRepo:            userRepo,
		userService:         userService,
		businessService:     businessService,
		emailService:        clients.GetEmailService(), // Extract from clients
		smsService:          clients.GetSMSService(),   // Extract from clients
		smsConfig:           &cfg.SMS,                  // Extract SMS config
		metrics:             clients.GetMetrics().(metrics.MetricsProvider),
		appConfig:           cfg,
		txManager:           clients.GetTxManager(), // Extract from clients
		rateLimiter:         ratelimiter.GetInstance(),
	}
}

// generateToken creates a JWT token with device information and session ID
func (s *AuthService) generateToken(userID string, sessionID string, deviceType string, deviceVersion int, expiry time.Duration) (string, error) {
	claims := auth.CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		SessionID:     sessionID,
		DeviceType:    deviceType,
		DeviceVersion: deviceVersion,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.config.SecretKey))
	if err != nil {
		logger.Errorf("Failed to sign token: %v", err)
		return "", fmt.Errorf("failed to create token")
	}
	return signedToken, nil
}

// generateAccessToken creates a new short-lived access token with device information and session ID
func (s *AuthService) generateAccessToken(userID string, sessionID string, deviceType string, deviceVersion int) (string, error) {
	return s.generateToken(userID, sessionID, deviceType, deviceVersion, s.config.AccessTokenExpiry)
}

// generateRefreshToken creates a new long-lived refresh token with device information and session ID
func (s *AuthService) generateRefreshToken(userID string, sessionID string, deviceType string, deviceVersion int) (string, error) {
	return s.generateToken(userID, sessionID, deviceType, deviceVersion, s.config.RefreshTokenExpiry)
}

// CreateTokenPair creates a new access and refresh token pair with device session management
func (s *AuthService) CreateTokenPair(ctx context.Context, userID, userAgent string, deviceType string) (accessToken, refreshToken string, err error) {
	// Default to web if no device type provided (backward compatibility)
	if deviceType == "" {
		deviceType = string(shared.DeviceWeb)
	}

	// Validate device type
	if !shared.ValidateDeviceType(deviceType) {
		return "", "", fmt.Errorf("invalid device type: %s", deviceType)
	}

	var currentVersion int
	var maxSessions int

	// Get max sessions based on device type
	switch deviceType {
	case string(shared.DeviceWeb):
		maxSessions = s.config.MaxWebSessions
	case string(shared.DeviceMobile):
		maxSessions = s.config.MaxMobileSessions
	default:
		maxSessions = 3 // Fallback
	}

	// Generate session ID upfront so it can be included in JWT tokens
	sessionID := uuid.New().String()

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txSessionRepo := s.sessionRepo.WithTx(tx)
		txDeviceVersionRepo := s.deviceVersionRepo.WithTx(tx)

		// Count existing sessions for this device type
		sessionCount, err := txSessionRepo.CountSessionsByDevice(ctx, userID, deviceType)
		if err != nil {
			return fmt.Errorf("failed to count sessions: %w", err)
		}

		// If max sessions reached, increment version and delete oldest session
		if sessionCount >= maxSessions {
			logger.Infof("Max sessions (%d) reached for user %s device %s, incrementing version", maxSessions, userID, deviceType)

			// Increment device version
			newVersion, err := txDeviceVersionRepo.IncrementDeviceVersion(ctx, userID, deviceType)
			if err != nil {
				return fmt.Errorf("failed to increment device version: %w", err)
			}
			currentVersion = newVersion

			// Delete oldest session
			if err := txSessionRepo.DeleteOldestSessionByDevice(ctx, userID, deviceType); err != nil {
				return fmt.Errorf("failed to delete oldest session: %w", err)
			}
		} else {
			// Get current version (will be 1 if not exists)
			currentVersion, err = txDeviceVersionRepo.GetDeviceVersion(ctx, userID, deviceType)
			if err != nil {
				return fmt.Errorf("failed to get device version: %w", err)
			}
		}

		// Generate tokens with device information and session ID
		accessToken, err = s.generateAccessToken(userID, sessionID, deviceType, currentVersion)
		if err != nil {
			return err
		}

		refreshToken, err = s.generateRefreshToken(userID, sessionID, deviceType, currentVersion)
		if err != nil {
			return err
		}

		// Hash the refresh token for storage
		tokenHash := security.HashToken(refreshToken)

		// Store refresh token session in database with device info and pre-generated session ID
		err = txSessionRepo.CreateSession(
			ctx,
			sessionID,
			userID,
			tokenHash,
			time.Now().Add(s.config.RefreshTokenExpiry),
			userAgent,
			deviceType,
			currentVersion,
		)
		if err != nil {
			logger.Errorf("Failed to store session: %v", err)
			return fmt.Errorf("failed to store session")
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	// Invalidate device version cache in Redis (outside transaction, fire-and-forget)
	go func() {
		bgCtx := context.Background()
		cacheKey := keys.User.DeviceVersion(userID, deviceType)
		if err := s.rclient.Client.Del(bgCtx, cacheKey).Err(); err != nil {
			logger.Warnf("Failed to invalidate device version cache: %v", err)
		}
	}()

	return accessToken, refreshToken, nil
}

// RefreshAccessToken validates refresh token and issues a new access token
func (s *AuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (string, error) {
	// Parse refresh token to extract userID (JWT expiry is validated here automatically)
	claims, err := s.parseAndValidateToken(refreshToken)
	if err != nil {
		logger.Errorf("Invalid refresh token: %v", err)
		return "", fmt.Errorf("invalid refresh token")
	}

	userID := claims.Subject
	sessionID := claims.SessionID
	deviceType := claims.DeviceType
	deviceVersion := claims.DeviceVersion
	tokenHash := security.HashToken(refreshToken)

	// Validate device version with Redis/DB
	cacheKey := keys.User.DeviceVersion(userID, deviceType)

	// Try Redis first
	cachedVersion, err := s.rclient.Client.Get(ctx, cacheKey).Int()
	if err == nil {
		// Version found in cache
		if cachedVersion != deviceVersion {
			logger.Warnf("Device version mismatch for user %s device %s: token=%d, current=%d", userID, deviceType, deviceVersion, cachedVersion)
			return "", fmt.Errorf("session invalidated")
		}
	} else {
		// Redis miss - fallback to DB
		dbVersion, err := s.deviceVersionRepo.GetDeviceVersion(ctx, userID, deviceType)
		if err != nil {
			logger.Errorf("Failed to get device version from DB: %v", err)
			return "", fmt.Errorf("failed to validate session")
		}

		if dbVersion != deviceVersion {
			logger.Warnf("Device version mismatch for user %s device %s: token=%d, current=%d", userID, deviceType, deviceVersion, dbVersion)
			return "", fmt.Errorf("session invalidated")
		}

		// Cache the version for future requests
		go func() {
			bgCtx := context.Background()
			if err := s.rclient.Client.Set(bgCtx, cacheKey, dbVersion, keys.TTLDeviceVersion).Err(); err != nil {
				logger.Warnf("Failed to cache device version: %v", err)
			}
		}()
	}

	// Check if session exists in DB (if deleted, token is effectively revoked)
	_, err = s.sessionRepo.GetSessionByTokenHash(ctx, userID, tokenHash)
	if err != nil {
		logger.Errorf("Session not found in DB (token revoked or expired): %v", err)
		return "", fmt.Errorf("refresh token has been revoked")
	}

	// Update last used timestamp
	if err := s.sessionRepo.UpdateLastUsed(ctx, userID, tokenHash); err != nil {
		logger.Warnf("Failed to update last used: %v", err)
	}

	// Generate new access token with same device info and session ID
	return s.generateAccessToken(userID, sessionID, deviceType, deviceVersion)
}

// RevokeRefreshToken revokes a specific refresh token (logout from one session)
// Simply deletes the session from DB - version check in middleware will handle validation
func (s *AuthService) RevokeRefreshToken(ctx context.Context, userID, refreshToken string) error {
	// Parse token to validate it and verify it belongs to the user
	claims, err := s.parseAndValidateToken(refreshToken)
	if err != nil {
		logger.Errorf("Invalid refresh token: %v", err)
		return fmt.Errorf("invalid refresh token")
	}

	// Verify token belongs to this user
	if claims.Subject != userID {
		logger.Errorf("Token does not belong to user %s", userID)
		return fmt.Errorf("unauthorized token revocation")
	}

	// Hash the token
	tokenHash := security.HashToken(refreshToken)

	// Delete session from database - this is sufficient, no need for revocation lists
	// RefreshAccessToken will check if session exists in DB when Redis is down
	if err := s.sessionRepo.DeleteSession(ctx, userID, tokenHash); err != nil {
		logger.Errorf("Failed to delete session: %v", err)
		return fmt.Errorf("failed to revoke token")
	}

	logger.Infof("Successfully revoked refresh token for user %s", userID)
	return nil
}

// RevokeAllUserTokens revokes all refresh tokens for a user by incrementing device versions
// This efficiently invalidates ALL tokens across ALL devices (web + mobile) without maintaining revocation lists
func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID string) error {
	logger.Infof("Invalidating all tokens for user %s by bumping device versions", userID)

	// Use transaction to atomically increment versions for both device types and delete sessions
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txDeviceVersionRepo := s.deviceVersionRepo.WithTx(tx)
		txSessionRepo := s.sessionRepo.WithTx(tx)

		// Increment version for WEB device type
		webVersion, err := txDeviceVersionRepo.IncrementDeviceVersion(ctx, userID, string(shared.DeviceWeb))
		if err != nil {
			return fmt.Errorf("failed to increment web device version: %w", err)
		}
		logger.Infof("Bumped web device version to %d for user %s", webVersion, userID)

		// Increment version for MOBILE device type
		mobileVersion, err := txDeviceVersionRepo.IncrementDeviceVersion(ctx, userID, string(shared.DeviceMobile))
		if err != nil {
			return fmt.Errorf("failed to increment mobile device version: %w", err)
		}
		logger.Infof("Bumped mobile device version to %d for user %s", mobileVersion, userID)

		// Delete all sessions from database
		deletedCount, err := txSessionRepo.DeleteAllUserSessions(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to delete user sessions: %w", err)
		}
		logger.Infof("Deleted %d sessions for user %s", len(deletedCount), userID)

		return nil
	})

	if err != nil {
		logger.Errorf("Failed to revoke all tokens for user %s: %v", userID, err)
		return fmt.Errorf("failed to revoke tokens")
	}

	// Invalidate device version cache for both device types (fire-and-forget)
	go func() {
		bgCtx := context.Background()
		webKey := keys.User.DeviceVersion(userID, string(shared.DeviceWeb))
		mobileKey := keys.User.DeviceVersion(userID, string(shared.DeviceMobile))

		if err := s.rclient.Client.Del(bgCtx, webKey, mobileKey).Err(); err != nil {
			logger.Warnf("Failed to invalidate device version cache: %v", err)
		}
	}()

	logger.Infof("Successfully invalidated all tokens for user %s across all devices", userID)
	return nil
}

// ValidateAccessToken validates an access token and returns its claims
func (s *AuthService) ValidateAccessToken(tokenString string) (*auth.CustomClaims, error) {
	return s.parseAndValidateToken(tokenString)
}

// ValidateAccessTokenWithVersion validates an access token and checks device version
// This is used by middleware to ensure tokens are not invalidated
func (s *AuthService) ValidateAccessTokenWithVersion(ctx context.Context, tokenString string) (*auth.CustomClaims, error) {
	// Parse and validate token structure
	claims, err := s.parseAndValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	userID := claims.Subject
	deviceType := claims.DeviceType
	deviceVersion := claims.DeviceVersion

	// Validate device version with Redis/DB
	cacheKey := keys.User.DeviceVersion(userID, deviceType)

	// Try Redis first
	cachedVersion, err := s.rclient.Client.Get(ctx, cacheKey).Int()
	if err == nil {
		// Version found in cache
		if cachedVersion != deviceVersion {
			logger.Warnf("Device version mismatch for user %s device %s: token=%d, current=%d", userID, deviceType, deviceVersion, cachedVersion)
			return nil, fmt.Errorf("SESSION_INVALIDATED")
		}
	} else {
		// Redis miss - fallback to DB
		dbVersion, err := s.deviceVersionRepo.GetDeviceVersion(ctx, userID, deviceType)
		if err != nil {
			logger.Errorf("Failed to get device version from DB: %v", err)
			return nil, fmt.Errorf("failed to validate session")
		}

		if dbVersion != deviceVersion {
			logger.Warnf("Device version mismatch for user %s device %s: token=%d, current=%d", userID, deviceType, deviceVersion, dbVersion)
			return nil, fmt.Errorf("SESSION_INVALIDATED")
		}

		// Cache the version for future requests
		go func() {
			bgCtx := context.Background()
			if err := s.rclient.Client.Set(bgCtx, cacheKey, dbVersion, keys.TTLDeviceVersion).Err(); err != nil {
				logger.Warnf("Failed to cache device version: %v", err)
			}
		}()
	}

	return claims, nil
}

// parseAndValidateToken is a helper to parse and validate any JWT token
func (s *AuthService) parseAndValidateToken(tokenString string) (*auth.CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &auth.CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing algorithm
		if token.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*auth.CustomClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ChangePassword changes the user's password after verifying OTP and old password.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword, otp string) error {
	logger.Infof("Processing password change for user: %s", userID)

	if userID == "" || oldPassword == "" || newPassword == "" || otp == "" {
		return fmt.Errorf("all fields are required")
	}

	// Validate new password strength
	if err := security.ValidatePasswordStrength(newPassword); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return err
	}

	// Hash new password and OTP upfront
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		logger.Errorf("Failed to hash new password: %v", err)
		return fmt.Errorf("failed to process new password")
	}
	hashedOTP := security.HashOTP(otp)

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Verify OTP first
		verificationCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedOTP, shared.MobileVerification)
		if err != nil {
			logger.Errorf("Failed to get mobile verification code: %v", err)
			return fmt.Errorf("invalid or expired OTP")
		}

		if verificationCode.Used {
			return fmt.Errorf("OTP already used")
		}

		// Check OTP expiry
		expiresAt, err := time.Parse(time.RFC3339, verificationCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(expiresAt) {
			return fmt.Errorf("OTP has expired")
		}

		// Mark OTP as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark OTP as used: %w", err)
		}

		// Get user by ID within transaction
		userObj, err := txUserRepo.GetUserByID(ctx, userID)
		if err != nil {
			logger.Errorf("Failed to get user by ID: %v", err)
			return fmt.Errorf("user not found")
		}
		if userObj == nil {
			logger.Error("User not found for ID: %s", userID)
			return fmt.Errorf("user not found")
		}

		// Verify old password
		if err := s.ComparePassword(userObj.PwdHash, oldPassword); err != nil {
			logger.Error("Invalid old password for user: %s", userObj.Id)
			return fmt.Errorf("invalid old password")
		}

		// Check if new password is same as old password
		if err := s.ComparePassword(userObj.PwdHash, newPassword); err == nil {
			logger.Error("New password is same as old password for user: %s", userObj.Id)
			return fmt.Errorf("new password cannot be the same as old password")
		}

		// Update user password hash
		if err := txUserRepo.UpdateUserPasswordHash(ctx, userID, hashedPassword); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Password changed successfully for user: %s", userID)

	// Invalidate all refresh tokens for security
	logger.Infof("Invalidating all refresh tokens for user: %s", userID)
	if revokeErr := s.RevokeAllUserTokens(ctx, userID); revokeErr != nil {
		logger.Errorf("Failed to revoke all tokens for user %s: %v", userID, revokeErr)
		// Don't fail the password change if token revocation fails, just log it
	} else {
		logger.Infof("All refresh tokens invalidated for user: %s", userID)
	}

	return nil
}

// HashPassword hashes a password using bcrypt with pepper
func (s *AuthService) HashPassword(password string) (string, error) {
	// Add pepper to password before hashing
	pepperedPassword := password + s.bcryptConfig.Pepper
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(pepperedPassword), s.bcryptConfig.Cost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// ComparePassword compares a hashed password with a plaintext password
func (s *AuthService) ComparePassword(hashedPassword, password string) error {
	// Add pepper to password before comparing
	pepperedPassword := password + s.bcryptConfig.Pepper
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(pepperedPassword))
}

// GetRedisClient returns the Redis client
func (s *AuthService) GetRedisClient() *redis.RedisClient {
	return s.rclient
}

// Signup handles complete user registration flow
func (s *AuthService) Signup(ctx context.Context, email, password string) (*auth.SignupResponse, error) {
	logger.Info("Processing signup for email: %s", email)

	// Validate input
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Validate password strength
	if err := security.ValidatePasswordStrength(password); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return nil, err
	}

	// Rate limit check FIRST (prevent spam signups)
	rateLimitKey := fmt.Sprintf("xcentro:rl:email:%s|signup", email)
	result, err := s.rateLimiter.CheckLimitWithKey(ctx, rateLimitKey, "signup")
	if err == nil && !result.Allowed {
		return nil, fmt.Errorf("too many signup attempts. Please wait %s seconds before trying again", result.RetryAfterSecs)
	}

	// Check if user already exists (security: prevent enumeration)
	existingUser, err := s.userRepo.GetUserByEmail(ctx, email)

	if err == nil && existingUser != nil {
		// User exists - handle based on verification status
		if existingUser.EmailVerified {
			// Verified user - silent success (prevent enumeration attack)
			logger.Infof("Signup attempt for verified email (silent): %s", email)
			return &auth.SignupResponse{
				UserID:  existingUser.Id,
				Message: "Account created successfully. Please check your email for verification link.",
			}, nil
		}

		// Unverified user - resend verification email
		logger.Infof("Signup attempt for unverified email (resend): %s", email)
		go func() {
			bgCtx := context.Background()
			if err := s.SendEmailVerification(bgCtx, existingUser.Id); err != nil {
				logger.Errorf("Failed to resend email verification: %v", err)
			}
		}()

		return &auth.SignupResponse{
			UserID:  existingUser.Id,
			Message: "Account created successfully. Please check your email for verification link.",
		}, nil
	}

	// New user - proceed with creation
	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		logger.Errorf("Failed to hash password: %v", err)
		return nil, fmt.Errorf("failed to process password")
	}

	authPayload := auth.AuthPayload{
		Email:    email,
		Password: hashedPassword,
	}

	createdUser, err := s.userService.CreateUser(ctx, authPayload)
	if err != nil {
		logger.Errorf("Failed to create user: %v", err)
		return nil, err
	}

	// Send email verification (non-blocking)
	userID := createdUser.Id
	go func() {
		bgCtx := context.Background()
		if err := s.SendEmailVerification(bgCtx, userID); err != nil {
			logger.Errorf("Failed to send email verification: %v", err)
		}
	}()

	return &auth.SignupResponse{
		UserID:  createdUser.Id,
		Message: "Account created successfully. Please check your email for verification link.",
	}, nil
}

// Login handles complete login flow with token pair generation
func (s *AuthService) Login(ctx context.Context, email, password, userAgent, deviceType string) (*auth.LoginResponse, error) {
	logger.Info("Processing login for email: %s, device: %s", email, deviceType)

	// Validate input
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Get user by email (cached)
	userObj, err := s.userService.GetUserByEmail(ctx, email)
	if err != nil {
		logger.Errorf("Failed to get user by email: %v", err)
		return nil, fmt.Errorf("invalid email or password")
	}
	if userObj == nil {
		logger.Error("User not found for email: %s", email)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Check email and mobile verification
	if !userObj.EmailVerified || !userObj.MobileVerified {
		logger.Error("Account verification incomplete for user: %s (email: %v, mobile: %v)", userObj.Id, userObj.EmailVerified, userObj.MobileVerified)
		return nil, fmt.Errorf("account verification incomplete. Please verify both your email and mobile number")
	}

	// Verify password
	if err := s.ComparePassword(userObj.PwdHash, password); err != nil {
		logger.Error("Invalid password for user: %s", userObj.Id)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Create token pair with device information
	accessToken, refreshToken, err := s.CreateTokenPair(ctx, userObj.Id, userAgent, deviceType)
	if err != nil {
		logger.Errorf("Failed to create token pair: %v", err)
		return nil, fmt.Errorf("failed to create authentication tokens")
	}

	s.metrics.Counter(ctx, "auth.loginsucess", 1, metrics.MetricLabels{"Test": "Check"})

	return &auth.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil

}

// ============================================================================
// Password Reset Methods
// ============================================================================

// SendPasswordResetEmail sends password reset or invitation token to user's email
// emailType can be "reset" or "invite"
// SECURITY: Always returns success to prevent email enumeration (for reset type)
func (s *AuthService) SendPasswordResetEmail(ctx context.Context, emailAddr string, emailType string) error {
	logger.Infof("Password reset/invitation requested for email: %s, type: %s", emailAddr, emailType)

	if emailAddr == "" {
		// Still return nil to prevent enumeration
		return nil
	}

	// Get user details - if not found, silently return success for reset type
	userObj, err := s.userRepo.GetUserByEmail(ctx, emailAddr)
	if err != nil || userObj == nil {
		logger.Infof("Password reset/invitation requested for non-existent email: %s", emailAddr)
		// SECURITY: Return success even if user doesn't exist (for reset type only)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("user not found")
	}

	// Use transaction to atomically invalidate old tokens and create new one
	// SECURITY: Prevents multiple valid reset tokens from existing simultaneously
	var resetToken string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Invalidate all previous password reset tokens for this user
		// SECURITY: Only the latest reset link should be valid
		if err := txUserRepo.InvalidatePreviousVerificationCodes(ctx, userObj.Id, shared.PasswordReset); err != nil {
			logger.Errorf("Failed to invalidate previous reset tokens: %v", err)
			// Return success to prevent information disclosure (for reset type)
			if emailType == "reset" {
				return nil
			}
			return fmt.Errorf("failed to invalidate previous tokens: %w", err)
		}

		// Generate secure verification token (32 bytes = 256 bits)
		var genErr error
		resetToken, genErr = security.GenerateSecureToken()
		if genErr != nil {
			logger.Errorf("Failed to generate reset token: %v", genErr)
			// Return success to prevent information disclosure (for reset type)
			if emailType == "reset" {
				return nil
			}
			return fmt.Errorf("failed to generate token: %w", genErr)
		}

		// Hash the token before storing in database
		hashedToken := security.HashTokenWithPepper(resetToken, s.bcryptConfig.Pepper)

		// Calculate timestamps - 1 hour expiry for password reset (IMPORTANT: Use UTC)
		now := time.Now().UTC()
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

		logger.Debugf("Creating password reset code: user_id=%s, expires_at=%s (UTC), duration=1h",
			userObj.Id, expiresAt.Format(time.RFC3339))

		// Store verification code in database (within transaction)
		if err := txUserRepo.CreateVerificationCode(ctx, verificationCode); err != nil {
			logger.Errorf("Failed to create password reset code: %v", err)
			// Return success to prevent information disclosure (for reset type)
			if emailType == "reset" {
				return nil
			}
			return fmt.Errorf("failed to create verification code: %w", err)
		}

		return nil
	})

	if err != nil {
		// Transaction failed - return success for reset type (enumeration protection)
		if emailType == "reset" {
			return nil
		}
		return err
	}

	// Transaction succeeded, now send email
	// Generate HMAC signature for uid:token
	signature := security.GenerateHMACSignature(userObj.Id, resetToken, s.config.SecretKey)

	// Create password reset URL with uid, token, and sig as query parameters
	resetURL := fmt.Sprintf("%s/reset-password?uid=%s&token=%s&sig=%s",
		s.appConfig.Frontend.URL, userObj.Id, resetToken, signature)

	// Render email templates using the email renderer
	renderer := email.GetRenderer()
	var subject, htmlBody, textBody string
	var renderErr error

	if emailType == "invite" {
		subject = "Welcome to xcentro - Set Your Password"
		htmlBody, textBody, renderErr = renderer.RenderInvitation(userObj.Name, resetURL)
	} else {
		subject = "Reset your xcentro password"
		htmlBody, textBody, renderErr = renderer.RenderPasswordReset(resetURL)
	}

	if renderErr != nil {
		logger.Errorf("Failed to render email templates: %v", renderErr)
		// Return success to prevent information disclosure (for reset type)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("failed to render email templates: %w", renderErr)
	}

	// Create email message
	emailMsg := &mailers.EmailMessage{
		To:       []string{userObj.Email},
		From:     s.appConfig.SES.DefaultFromEmail,
		FromName: s.appConfig.SES.DefaultFromName,
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		logger.Errorf("Failed to send email to %s: %v", userObj.Email, err)
		// Return success to prevent information disclosure (for reset type)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("failed to send email: %w", err)
	}

	logger.Infof("Email sent successfully to %s, Message ID: %s, Type: %s", userObj.Email, messageID, emailType)
	return nil
}

// ResendInviteEmail resends invitation email to a user (admin function)
// Unlike SendPasswordResetEmail, this method:
// - Validates user hasn't already activated (password not set)
// - Invalidates previous invite tokens (in transaction)
// - Does NOT fail-open (returns actual errors for admin feedback)
func (s *AuthService) ResendInviteEmail(ctx context.Context, emailAddr string) error {
	logger.Infof("Resending invitation email to: %s", emailAddr)

	if emailAddr == "" {
		return fmt.Errorf("email address is required")
	}

	// Get user details
	userObj, err := s.userRepo.GetUserByEmail(ctx, emailAddr)
	if err != nil || userObj == nil {
		logger.Errorf("User not found for email: %s", emailAddr)
		return fmt.Errorf("user not found")
	}

	// Check if user has already activated account (password is set)
	if userObj.PwdHash != "" {
		logger.Warnf("Cannot resend invite to activated user: %s", emailAddr)
		return fmt.Errorf("user has already activated their account")
	}

	// Use transaction to atomically invalidate old tokens and create new one
	var resetToken string
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Invalidate all previous password reset/invite tokens for this user
		if err := txUserRepo.InvalidatePreviousVerificationCodes(ctx, userObj.Id, shared.PasswordReset); err != nil {
			logger.Errorf("Failed to invalidate previous invite tokens: %v", err)
			return fmt.Errorf("failed to invalidate previous tokens: %w", err)
		}

		// Generate secure verification token (32 bytes = 256 bits)
		var genErr error
		resetToken, genErr = security.GenerateSecureToken()
		if genErr != nil {
			logger.Errorf("Failed to generate reset token: %v", genErr)
			return fmt.Errorf("failed to generate token: %w", genErr)
		}

		// Hash the token before storing in database
		hashedToken := security.HashTokenWithPepper(resetToken, s.bcryptConfig.Pepper)

		// Calculate timestamps - 1 hour expiry for invitation (IMPORTANT: Use UTC)
		now := time.Now().UTC()
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

		logger.Debugf("Creating invite code: user_id=%s, expires_at=%s (UTC), duration=1h",
			userObj.Id, expiresAt.Format(time.RFC3339))

		// Store verification code in database (within transaction)
		if err := txUserRepo.CreateVerificationCode(ctx, verificationCode); err != nil {
			logger.Errorf("Failed to create invite code: %v", err)
			return fmt.Errorf("failed to create verification code: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Transaction succeeded, now send email
	// Generate HMAC signature for uid:token
	signature := security.GenerateHMACSignature(userObj.Id, resetToken, s.config.SecretKey)

	// Create invitation URL with uid, token, and sig as query parameters
	inviteURL := fmt.Sprintf("%s/set-password?uid=%s&token=%s&sig=%s",
		s.appConfig.Frontend.URL, userObj.Id, resetToken, signature)

	// Render invitation email templates
	renderer := email.GetRenderer()
	subject := "Welcome to xcentro - Set Your Password"
	htmlBody, textBody, renderErr := renderer.RenderInvitation(userObj.Name, inviteURL)

	if renderErr != nil {
		logger.Errorf("Failed to render invitation email templates: %v", renderErr)
		return fmt.Errorf("failed to render email templates: %w", renderErr)
	}

	// Create email message
	emailMsg := &mailers.EmailMessage{
		To:       []string{userObj.Email},
		From:     s.appConfig.SES.DefaultFromEmail,
		FromName: s.appConfig.SES.DefaultFromName,
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		logger.Errorf("Failed to send invitation email to %s: %v", userObj.Email, err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	logger.Infof("Invitation email resent successfully to %s, Message ID: %s", userObj.Email, messageID)
	return nil
}

// ResetPasswordWithToken resets user's password using verification token with HMAC signature validation
func (s *AuthService) ResetPasswordWithToken(ctx context.Context, userID, token, signature, newPassword string) error {
	logger.Infof("Resetting password with token for user: %s", userID)

	if userID == "" || token == "" || signature == "" {
		return fmt.Errorf("userID, token, and signature are required")
	}

	if newPassword == "" {
		return fmt.Errorf("new password is required")
	}

	// Validate HMAC signature first (prevents tampering)
	if !security.ValidateHMACSignature(userID, token, signature, s.config.SecretKey) {
		logger.Errorf("Invalid HMAC signature for user: %s", userID)
		return fmt.Errorf("invalid or tampered reset link")
	}

	// Validate new password strength
	if err := security.ValidatePasswordStrength(newPassword); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return err
	}

	// Hash the incoming token to compare with stored hash
	hashedToken := security.HashTokenWithPepper(token, s.bcryptConfig.Pepper)

	// Hash the new password using existing HashPassword method
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		logger.Errorf("Failed to hash new password: %v", err)
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code record using userID and hashed token (faster with userID index)
		verificationCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedToken, shared.PasswordReset)
		if err != nil {
			logger.Errorf("Failed to get password reset code: %v", err)
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

		// Mark verification code as used (token is automatically erased after use)
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark reset code as used: %w", err)
		}

		// Update user password hash
		if err := txUserRepo.UpdateUserPasswordHash(ctx, verificationCode.UserID, hashedPassword); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Password reset successfully for user: %s", userID)

	// Invalidate all refresh tokens for security (done after password update)
	logger.Infof("Invalidating all refresh tokens for user: %s", userID)

	if revokeErr := s.RevokeAllUserTokens(ctx, userID); revokeErr != nil {
		logger.Errorf("Failed to revoke all tokens for user %s: %v", userID, revokeErr)
		// Don't fail the password reset if token revocation fails, just log it
	} else {
		logger.Infof("All refresh tokens invalidated for user: %s", userID)
	}

	return nil
}

// ============================================================================
// Email Verification Methods
// ============================================================================

// SendEmailVerification sends email verification token to user's email
func (s *AuthService) SendEmailVerification(ctx context.Context, userID string) error {
	logger.Infof("Sending email verification for user: %s", userID)

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
	verificationToken, err := security.GenerateSecureToken()
	if err != nil {
		return fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Hash the token before storing in database (SECURITY: Never store plaintext tokens)
	hashedToken := security.HashTokenWithPepper(verificationToken, s.bcryptConfig.Pepper)

	// Calculate proper timestamps (IMPORTANT: Use UTC to avoid timezone issues)
	now := time.Now().UTC()
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

	logger.Debugf("Creating email verification code: user_id=%s, expires_at=%s (UTC), duration=24h",
		userID, expiresAt.Format(time.RFC3339))

	// Atomically invalidate old codes and create new one (SECURITY: prevents race conditions)
	if err := s.userRepo.CreateVerificationCodeWithInvalidation(ctx, verificationCode); err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// Generate HMAC signature for uid:token
	signature := security.GenerateHMACSignature(userID, verificationToken, s.config.SecretKey)

	// Create verification URL with uid, token, and sig
	verificationURL := fmt.Sprintf("%s/verify-email?uid=%s&token=%s&sig=%s",
		s.appConfig.Frontend.URL, userID, verificationToken, signature)

	// Render email templates using the email renderer
	renderer := email.GetRenderer()
	htmlBody, textBody, err := renderer.RenderEmailVerification(verificationURL)
	if err != nil {
		logger.Errorf("Failed to render email verification templates: %v", err)
		return fmt.Errorf("failed to render email templates: %w", err)
	}

	// Create email message
	emailMsg := &mailers.EmailMessage{
		To:       []string{userObj.Email},
		From:     s.appConfig.SES.DefaultFromEmail,
		FromName: s.appConfig.SES.DefaultFromName,
		Subject:  "Verify your Xcentro account",
		HTMLBody: htmlBody,
		TextBody: textBody,
	}

	// Send email
	messageID, err := s.emailService.SendEmail(ctx, emailMsg)
	if err != nil {
		logger.Errorf("Failed to send verification email to %s: %v", userObj.Email, err)
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	logger.Infof("Verification email sent successfully to %s, Message ID: %s", userObj.Email, messageID)

	return nil
}

// VerifyEmailWithToken verifies user's email using verification token with HMAC signature validation
// ResendEmailVerification resends email verification for a given email
func (s *AuthService) ResendEmailVerification(ctx context.Context, email string) error {
	logger.Infof("Resending email verification for email: %s", email)

	if email == "" {
		return fmt.Errorf("email is required")
	}

	// Rate limit check (email-scoped, fail open for UX)
	rateLimitKey := fmt.Sprintf("xcentro:rl:email:%s|resend_email_verification", email)
	result, err := s.rateLimiter.CheckLimitWithKey(ctx, rateLimitKey, "resend_email_verification")

	if err != nil {
		logger.Errorf("Rate limit check failed: %v", err)
		// Fail open for UX (allow if Redis unavailable)
	} else if !result.Allowed {
		return fmt.Errorf("too many resend requests. Please wait %s seconds before trying again", result.RetryAfterSecs)
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

	// Send verification email (will invalidate old codes)
	return s.SendEmailVerification(ctx, userObj.Id)
}

// ResendMobileOTP resends OTP during signup flow (public endpoint, uses email to lookup user)
func (s *AuthService) ResendMobileOTP(ctx context.Context, email string) error {
	logger.Infof("Resending mobile OTP for email: %s", email)

	if email == "" {
		return fmt.Errorf("email is required")
	}

	// Rate limit check (email-scoped, fail closed for SMS cost control)
	rateLimitKey := fmt.Sprintf("xcentro:rl:email:%s|resend_mobile_otp", email)
	result, err := s.rateLimiter.CheckLimitWithKey(ctx, rateLimitKey, "resend_mobile_otp")

	if err != nil || !result.Allowed {
		if err != nil {
			return fmt.Errorf("rate limit check failed, please try again later")
		}
		return fmt.Errorf("too many resend requests. Please wait %s seconds before trying again", result.RetryAfterSecs)
	}

	// Get user by email
	userObj, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil || userObj == nil {
		return fmt.Errorf("user not found")
	}

	if userObj.MobileVerified {
		return fmt.Errorf("mobile already verified")
	}

	if userObj.Mobile == "" {
		return fmt.Errorf("user does not have a mobile number registered")
	}

	// NOTE: Code invalidation is handled atomically in SendMobileVerificationOTP

	// Increment resend attempts (analytics tracking)
	_ = s.userRepo.IncrementResendAttempts(ctx, userObj.Id, shared.MobileVerification)

	// Send new OTP (with retry logic and atomic code invalidation)
	return s.SendMobileVerificationOTP(ctx, userObj.Id, userObj.Mobile)
}

// ResendInviteOTP resends OTP for invited users with rate limiting and signature validation
func (s *AuthService) ResendInviteOTP(ctx context.Context, userID, token, signature string) error {
	logger.Infof("Resending invite OTP for user: %s", userID)

	// Validate HMAC signature first (ensures legitimate invite link)
	if !security.ValidateHMACSignature(userID, token, signature, s.config.SecretKey) {
		logger.Errorf("Invalid HMAC signature for user: %s", userID)
		return fmt.Errorf("invalid or tampered invitation link")
	}

	// Rate limit check (user-scoped, reuses same policy as mobile OTP)
	rateLimitKey := fmt.Sprintf("xcentro:rl:user:%s|resend_mobile_otp", userID)
	result, err := s.rateLimiter.CheckLimitWithKey(ctx, rateLimitKey, "resend_mobile_otp")

	if err != nil || !result.Allowed {
		if err != nil {
			return fmt.Errorf("rate limit check failed, please try again later")
		}
		return fmt.Errorf("too many resend requests. Please wait %s seconds before trying again", result.RetryAfterSecs)
	}

	// Fetch user to get mobile number
	usr, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to fetch user: %w", err)
	}

	if usr.Mobile == "" {
		return fmt.Errorf("user does not have a mobile number registered")
	}

	// NOTE: Code invalidation is handled atomically in SendMobileVerificationOTP

	// Increment resend attempts (analytics tracking)
	_ = s.userRepo.IncrementResendAttempts(ctx, userID, shared.MobileVerification)

	// Generate and send new OTP (with retry logic and atomic code invalidation)
	return s.SendMobileVerificationOTP(ctx, userID, usr.Mobile)
}

// ============================================================================
// Mobile Verification Methods (Phase 2)
// ============================================================================

// SendMobileVerificationOTP generates and sends an OTP to user's mobile number
func (s *AuthService) SendMobileVerificationOTP(ctx context.Context, userID, mobileNumber string) error {
	logger.Infof("Sending mobile verification OTP to user: %s, mobile: %s", userID, mobileNumber)

	if userID == "" || mobileNumber == "" {
		return fmt.Errorf("userID and mobileNumber are required")
	}

	// Generate OTP
	otp, err := security.GenerateOTP(s.smsConfig.OTPLength)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash OTP for storage
	hashedOTP := security.HashOTP(otp)

	// Store OTP in database with expiry
	verificationCode := &user.UserVerificationCodes{
		UserID:        userID,
		Code:          hashedOTP,
		CodeType:      shared.MobileVerification,
		ExpiresAt:     time.Now().Add(s.smsConfig.OTPExpiry).Format(time.RFC3339),
		CreatedAt:     time.Now().Format(time.RFC3339),
		Used:          false,
		RetryAttempts: 0,
	}

	// Atomically invalidate old codes and create new one (SECURITY: prevents race conditions)
	if err := s.userRepo.CreateVerificationCodeWithInvalidation(ctx, verificationCode); err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// Send OTP via SMS (retry logic now built into Fast2SMS library)
	var messageID string
	var sendErr error

	if fast2smsClient, ok := s.smsService.(*sms.Fast2SMSClient); ok {
		messageID, sendErr = fast2smsClient.SendOTPSMS(ctx, mobileNumber, otp, "signup")
	} else {
		// Fallback to generic SendSMS (for other providers - no retry)
		smsMessage := &sms.SMSMessage{
			To:   mobileNumber,
			Body: otp,
		}
		messageID, sendErr = s.smsService.SendSMS(ctx, smsMessage)
	}

	if sendErr != nil {
		logger.Errorf("Failed to send SMS OTP: %v", sendErr)
		return fmt.Errorf("failed to send SMS: %w", sendErr)
	}

	logger.Infof("SMS OTP sent successfully to %s, Message ID: %s", mobileNumber, messageID)
	return nil
}

// GenerateMobileOTP fetches user's mobile number and sends OTP (generic method for protected routes)
func (s *AuthService) GenerateMobileOTP(ctx context.Context, userID string) error {
	logger.Infof("Generating mobile OTP for user: %s", userID)

	if userID == "" {
		return fmt.Errorf("userID is required")
	}

	// Rate limit check (user-scoped for authenticated users)
	rateLimitKey := fmt.Sprintf("xcentro:rl:user:%s|resend_mobile_otp", userID)
	result, err := s.rateLimiter.CheckLimitWithKey(ctx, rateLimitKey, "resend_mobile_otp")

	if err != nil || !result.Allowed {
		if err != nil {
			return fmt.Errorf("rate limit check failed, please try again later")
		}
		return fmt.Errorf("too many resend requests. Please wait %s seconds before trying again", result.RetryAfterSecs)
	}

	// Fetch user to get mobile number
	usr, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to fetch user: %w", err)
	}

	if usr.Mobile == "" {
		return fmt.Errorf("user does not have a mobile number registered")
	}

	// NOTE: Code invalidation is handled atomically in SendMobileVerificationOTP

	// Increment resend attempts (analytics tracking)
	_ = s.userRepo.IncrementResendAttempts(ctx, userID, shared.MobileVerification)

	// Send OTP (with retry logic and atomic code invalidation)
	return s.SendMobileVerificationOTP(ctx, userID, usr.Mobile)
}

// SendInviteOTP sends OTP for invited user (validates reset token signature first)
func (s *AuthService) SendInviteOTP(ctx context.Context, userID, token, signature string) error {
	logger.Infof("Sending invite OTP for user: %s", userID)

	if userID == "" || token == "" || signature == "" {
		return fmt.Errorf("userID, token, and signature are required")
	}

	// Validate HMAC signature to ensure legitimate invite link
	if !security.ValidateHMACSignature(userID, token, signature, s.config.SecretKey) {
		logger.Errorf("Invalid HMAC signature for user: %s", userID)
		return fmt.Errorf("invalid or tampered invitation link")
	}

	// Fetch user to get mobile number
	usr, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to fetch user: %w", err)
	}

	if usr.Mobile == "" {
		return fmt.Errorf("user does not have a mobile number registered")
	}

	// Generate OTP
	otp, err := security.GenerateOTP(s.smsConfig.OTPLength)
	if err != nil {
		return fmt.Errorf("failed to generate OTP: %w", err)
	}

	// Hash OTP for storage
	hashedOTP := security.HashOTP(otp)

	// Store OTP in database with expiry
	verificationCode := &user.UserVerificationCodes{
		UserID:        userID,
		Code:          hashedOTP,
		CodeType:      shared.MobileVerification,
		ExpiresAt:     time.Now().Add(s.smsConfig.OTPExpiry).Format(time.RFC3339),
		CreatedAt:     time.Now().Format(time.RFC3339),
		Used:          false,
		RetryAttempts: 0,
	}

	// Atomically invalidate old codes and create new one (SECURITY: prevents race conditions)
	if err := s.userRepo.CreateVerificationCodeWithInvalidation(ctx, verificationCode); err != nil {
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// Send OTP via SMS using Fast2SMS (password reset template)
	var messageID string
	if fast2smsClient, ok := s.smsService.(*sms.Fast2SMSClient); ok {
		messageID, err = fast2smsClient.SendOTPSMS(ctx, usr.Mobile, otp, "password_reset")
		if err != nil {
			logger.Errorf("Failed to send SMS OTP: %v", err)
			return fmt.Errorf("failed to send SMS: %w", err)
		}
	} else {
		// Fallback to generic SendSMS
		smsMessage := &sms.SMSMessage{
			To:   usr.Mobile,
			Body: otp,
		}
		messageID, err = s.smsService.SendSMS(ctx, smsMessage)
		if err != nil {
			logger.Errorf("Failed to send SMS OTP: %v", err)
			return fmt.Errorf("failed to send SMS: %w", err)
		}
	}

	logger.Infof("Invite OTP sent successfully to %s, Message ID: %s", usr.Mobile, messageID)
	return nil
}

// VerifyEmailWithMobile verifies email with token/signature and captures mobile number for OTP
func (s *AuthService) VerifyEmailWithMobile(ctx context.Context, userID, token, signature, mobileNumber string) error {
	logger.Infof("Verifying email with mobile for user: %s", userID)

	if userID == "" || token == "" || signature == "" || mobileNumber == "" {
		return fmt.Errorf("userID, token, signature, and mobileNumber are required")
	}

	// Validate HMAC signature first
	if !security.ValidateHMACSignature(userID, token, signature, s.config.SecretKey) {
		logger.Errorf("Invalid HMAC signature for user: %s", userID)
		return fmt.Errorf("invalid or tampered verification link")
	}

	// Hash the incoming token
	hashedToken := security.HashTokenWithPepper(token, s.bcryptConfig.Pepper)

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code
		verificationCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedToken, shared.EmailVerification)
		if err != nil {
			logger.Errorf("Failed to get verification code: %v", err)
			return fmt.Errorf("invalid or expired verification token")
		}

		if verificationCode.Used {
			return fmt.Errorf("verification token already used")
		}

		// Check expiry
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
		if err := txUserRepo.UpdateUserEmailVerification(ctx, userID, true); err != nil {
			return fmt.Errorf("failed to update email verification: %w", err)
		}

		// Update user mobile number
		if err := txUserRepo.UpdateUserMobile(ctx, userID, mobileNumber); err != nil {
			return fmt.Errorf("failed to update mobile number: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Send mobile OTP (outside transaction)
	if err := s.SendMobileVerificationOTP(ctx, userID, mobileNumber); err != nil {
		logger.Errorf("Failed to send mobile OTP after email verification: %v", err)
		// Don't fail the whole operation - email is already verified and mobile is saved
		return fmt.Errorf("email verified but failed to send mobile OTP: %w", err)
	}

	logger.Infof("Email verified and mobile OTP sent successfully for user: %s", userID)
	return nil
}

// VerifyMobileOTP verifies the mobile OTP and marks mobile as verified
func (s *AuthService) VerifyMobileOTP(ctx context.Context, userID, otp string) error {
	logger.Infof("Verifying mobile OTP for user: %s", userID)

	if userID == "" || otp == "" {
		return fmt.Errorf("userID and OTP are required")
	}

	// Hash the incoming OTP
	hashedOTP := security.HashOTP(otp)

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code
		verificationCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedOTP, shared.MobileVerification)
		if err != nil {
			logger.Errorf("Failed to get mobile verification code: %v", err)
			return fmt.Errorf("invalid or expired OTP")
		}

		if verificationCode.Used {
			return fmt.Errorf("OTP already used")
		}

		// Check expiry
		expiresAt, err := time.Parse(time.RFC3339, verificationCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(expiresAt) {
			return fmt.Errorf("OTP has expired")
		}

		// Mark verification code as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark OTP as used: %w", err)
		}

		// Update user mobile verification status
		if err := txUserRepo.UpdateUserMobileVerification(ctx, userID, true); err != nil {
			return fmt.Errorf("failed to update mobile verification: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Mobile verified successfully for user: %s", userID)
	return nil
}

// ResetPasswordWithMobileOTP resets password and verifies mobile OTP for invited users
func (s *AuthService) ResetPasswordWithMobileOTP(ctx context.Context, userID, token, signature, otp, newPassword string) error {
	logger.Infof("Resetting password with mobile OTP for user: %s", userID)

	if userID == "" || token == "" || signature == "" || otp == "" || newPassword == "" {
		return fmt.Errorf("all fields are required")
	}

	// Validate HMAC signature
	if !security.ValidateHMACSignature(userID, token, signature, s.config.SecretKey) {
		logger.Errorf("Invalid HMAC signature for user: %s", userID)
		return fmt.Errorf("invalid or tampered reset link")
	}

	// Validate password strength
	if err := security.ValidatePasswordStrength(newPassword); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return err
	}

	// Hash tokens and OTP
	hashedToken := security.HashTokenWithPepper(token, s.bcryptConfig.Pepper)
	hashedOTP := security.HashOTP(otp)
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		logger.Errorf("Failed to hash new password: %v", err)
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Verify password reset token
		resetCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedToken, shared.PasswordReset)
		if err != nil {
			logger.Errorf("Failed to get password reset code: %v", err)
			return fmt.Errorf("invalid or expired reset token")
		}

		if resetCode.Used {
			return fmt.Errorf("reset token already used")
		}

		resetExpiresAt, err := time.Parse(time.RFC3339, resetCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}
		resetExpiresAt = resetExpiresAt.UTC() // Ensure UTC timezone

		nowUTC := time.Now().UTC()
		if nowUTC.After(resetExpiresAt) {
			logger.Warnf("Password reset token expired: user_id=%s, expires_at=%s, current_time=%s, age=%v",
				userID, resetExpiresAt.Format(time.RFC3339), nowUTC.Format(time.RFC3339), nowUTC.Sub(resetExpiresAt))
			return fmt.Errorf("reset token has expired")
		}

		logger.Debugf("Password reset token valid: user_id=%s, expires_at=%s, current_time=%s, remaining=%v",
			userID, resetExpiresAt.Format(time.RFC3339), nowUTC.Format(time.RFC3339), resetExpiresAt.Sub(nowUTC))

		// Verify mobile OTP
		otpCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedOTP, shared.MobileVerification)
		if err != nil {
			logger.Errorf("Failed to get mobile OTP: %v", err)
			return fmt.Errorf("invalid or expired OTP")
		}

		if otpCode.Used {
			return fmt.Errorf("OTP already used")
		}

		otpExpiresAt, err := time.Parse(time.RFC3339, otpCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(otpExpiresAt) {
			return fmt.Errorf("OTP has expired")
		}

		// Mark both codes as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, resetCode.ID); err != nil {
			return fmt.Errorf("failed to mark reset code as used: %w", err)
		}

		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, otpCode.ID); err != nil {
			return fmt.Errorf("failed to mark OTP as used: %w", err)
		}

		// Update password
		if err := txUserRepo.UpdateUserPasswordHash(ctx, userID, hashedPassword); err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}

		// Mark mobile as verified
		if err := txUserRepo.UpdateUserMobileVerification(ctx, userID, true); err != nil {
			return fmt.Errorf("failed to update mobile verification: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Password reset and mobile verified successfully for user: %s", userID)

	// Invalidate all refresh tokens for security
	logger.Infof("Invalidating all refresh tokens for user: %s", userID)
	if revokeErr := s.RevokeAllUserTokens(ctx, userID); revokeErr != nil {
		logger.Errorf("Failed to revoke all tokens for user %s: %v", userID, revokeErr)
		// Don't fail the password reset if token revocation fails
	} else {
		logger.Infof("All refresh tokens revoked for user: %s", userID)
	}

	return nil
}

// VerifyAndUpdateMobile verifies OTP and updates user's mobile number
func (s *AuthService) VerifyAndUpdateMobile(ctx context.Context, userID, newMobile, otp string) error {
	logger.Infof("Verifying OTP and updating mobile for user: %s", userID)

	if userID == "" || newMobile == "" || otp == "" {
		return fmt.Errorf("userID, mobileNumber, and OTP are required")
	}

	// Hash the incoming OTP
	hashedOTP := security.HashOTP(otp)

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code
		verificationCode, err := txUserRepo.GetVerificationCodeByUserIDAndCode(ctx, userID, hashedOTP, shared.MobileVerification)
		if err != nil {
			logger.Errorf("Failed to get mobile verification code: %v", err)
			return fmt.Errorf("invalid or expired OTP")
		}

		if verificationCode.Used {
			return fmt.Errorf("OTP already used")
		}

		// Check expiry
		expiresAt, err := time.Parse(time.RFC3339, verificationCode.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid expiration time format")
		}

		if time.Now().After(expiresAt) {
			return fmt.Errorf("OTP has expired")
		}

		// Mark verification code as used
		if err := txUserRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
			return fmt.Errorf("failed to mark OTP as used: %w", err)
		}

		// Update user mobile number
		if err := txUserRepo.UpdateUserMobile(ctx, userID, newMobile); err != nil {
			return fmt.Errorf("failed to update mobile number: %w", err)
		}

		// Mark mobile as verified
		if err := txUserRepo.UpdateUserMobileVerification(ctx, userID, true); err != nil {
			return fmt.Errorf("failed to update mobile verification: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Mobile number updated and verified successfully for user: %s", userID)
	return nil
}
