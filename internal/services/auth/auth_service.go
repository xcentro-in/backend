package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	sharedClients "xcentro.in/internal/shared/clients"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/email"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/security"
)

type AuthService struct {
	config          *config.JWTSecretConfig
	bcryptConfig    *config.BcryptConfig
	rclient         *redis.RedisClient
	sessionRepo     auth.SessionRepository
	userRepo        user.UserRepository
	userService     user.UserService
	businessService business.BusinessService
	emailService    mailers.EmailService
	metrics         metrics.MetricsProvider
	appConfig       *config.Config
	txManager       client.TxManager
}

func NewAuthService(
	sessionRepo auth.SessionRepository,
	userRepo user.UserRepository,
	userService user.UserService,
	businessService business.BusinessService,
	clients sharedClients.ExternalClients,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		config:          &cfg.JWT,                 // Extract JWT config
		bcryptConfig:    &cfg.Bcrypt,              // Extract Bcrypt config
		rclient:         clients.GetRedisClient(), // Extract from clients
		sessionRepo:     sessionRepo,
		userRepo:        userRepo,
		userService:     userService,
		businessService: businessService,
		emailService:    clients.GetEmailService(), // Extract from clients
		metrics:         clients.GetMetrics().(metrics.MetricsProvider),
		appConfig:       cfg,
		txManager:       clients.GetTxManager(), // Extract from clients
	}
}

// generateToken creates a JWT token for the given user with specified expiry duration
func (s *AuthService) generateToken(userID string, expiry time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.config.SecretKey))
	if err != nil {
		logger.Errorf("Failed to sign token: %v", err)
		return "", fmt.Errorf("failed to create token")
	}
	return signedToken, nil
}

// generateAccessToken creates a new short-lived access token for the given user
func (s *AuthService) generateAccessToken(userID string) (string, error) {
	return s.generateToken(userID, s.config.AccessTokenExpiry)
}

// generateRefreshToken creates a new long-lived refresh token for the given user
func (s *AuthService) generateRefreshToken(userID string) (string, error) {
	return s.generateToken(userID, s.config.RefreshTokenExpiry)
}

// CreateTokenPair creates a new access and refresh token pair
func (s *AuthService) CreateTokenPair(ctx context.Context, userID, userAgent string) (accessToken, refreshToken string, err error) {
	// Generate access token (short-lived)
	accessToken, err = s.generateAccessToken(userID)
	if err != nil {
		return "", "", err
	}

	// Generate refresh token (long-lived)
	refreshToken, err = s.generateRefreshToken(userID)
	if err != nil {
		return "", "", err
	}

	// Hash the refresh token for storage
	tokenHash := security.HashToken(refreshToken)

	// Store refresh token session in database
	_, err = s.sessionRepo.CreateSession(
		ctx,
		userID,
		tokenHash,
		time.Now().Add(s.config.RefreshTokenExpiry),
		userAgent,
	)
	if err != nil {
		logger.Errorf("Failed to store session: %v", err)
		return "", "", fmt.Errorf("failed to store session")
	}

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
	tokenHash := security.HashToken(refreshToken)

	// Check if token is revoked in Redis cache first
	revokedKey := keys.User.RevokedTokens(userID)
	isRevoked, err := s.rclient.Client.HExists(ctx, revokedKey, tokenHash).Result()

	if err != nil {
		// Redis is down - fallback to DB check
		logger.Warnf("Redis error checking revocation, falling back to DB: %v", err)

		// Try to get session from DB (if exists = not revoked, if not exists = revoked)
		_, dbErr := s.sessionRepo.GetSessionByTokenHash(ctx, userID, tokenHash)
		if dbErr != nil {
			logger.Errorf("Session not found in DB (token revoked): %v", dbErr)
			return "", fmt.Errorf("refresh token has been revoked")
		}

		// Session exists in DB, so not revoked - update last used timestamp
		if err := s.sessionRepo.UpdateLastUsed(ctx, userID, tokenHash); err != nil {
			logger.Warnf("Failed to update last used: %v", err)
		}

	} else if isRevoked {
		// Token is in revoked set
		logger.Errorf("Refresh token is revoked (cached)")
		return "", fmt.Errorf("refresh token has been revoked")
	} else {
		// Redis is up and token not revoked - update last used timestamp
		if err := s.sessionRepo.UpdateLastUsed(ctx, userID, tokenHash); err != nil {
			logger.Warnf("Failed to update last used: %v", err)
		}
	}

	// Generate new access token
	return s.generateAccessToken(userID)
}

// RevokeRefreshToken revokes a specific refresh token
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

	// Delete from database
	if err := s.sessionRepo.DeleteSession(ctx, userID, tokenHash); err != nil {
		logger.Errorf("Failed to delete session: %v", err)
		return fmt.Errorf("failed to revoke token")
	}

	// Mark as revoked in Redis cache (MUST succeed)
	revokedKey := keys.User.RevokedTokens(userID)
	if err := s.rclient.Client.HSet(ctx, revokedKey, tokenHash, "1").Err(); err != nil {
		logger.Errorf("Failed to mark token as revoked in Redis: %v", err)
		return fmt.Errorf("failed to complete token revocation")
	}

	// Set TTL on the revoked tokens hash (15 minutes)
	if err := s.rclient.Client.Expire(ctx, revokedKey, keys.TTLRevokedToken).Err(); err != nil {
		logger.Errorf("Failed to set TTL on revoked tokens: %v", err)
		return fmt.Errorf("failed to complete token revocation")
	}

	logger.Info("Successfully revoked refresh token for user %s", userID)
	return nil
}

// RevokeAllUserTokens revokes all refresh tokens for a user
func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID string) error {
	// Delete all sessions from database and get their token hashes
	tokenHashes, err := s.sessionRepo.DeleteAllUserSessions(ctx, userID)
	if err != nil {
		logger.Errorf("Failed to delete user sessions: %v", err)
		return fmt.Errorf("failed to revoke tokens")
	}

	if len(tokenHashes) == 0 {
		logger.Info("No tokens to revoke for user %s", userID)
		return nil
	}

	// Mark all tokens as revoked in Redis cache using HSET (MUST succeed)
	revokedKey := keys.User.RevokedTokens(userID)
	tokenMap := make(map[string]interface{})
	for _, tokenHash := range tokenHashes {
		tokenMap[tokenHash] = "1"
	}

	if err := s.rclient.Client.HSet(ctx, revokedKey, tokenMap).Err(); err != nil {
		logger.Errorf("Failed to mark tokens as revoked in Redis: %v", err)
		return fmt.Errorf("failed to complete token revocation")
	}

	// Set TTL on the revoked tokens hash (15 minutes)
	if err := s.rclient.Client.Expire(ctx, revokedKey, keys.TTLRevokedToken).Err(); err != nil {
		logger.Errorf("Failed to set TTL on revoked tokens: %v", err)
		return fmt.Errorf("failed to complete token revocation")
	}

	logger.Info("Successfully revoked %d tokens for user %s", len(tokenHashes), userID)
	return nil
}

// ValidateAccessToken validates an access token and returns its claims
func (s *AuthService) ValidateAccessToken(tokenString string) (*jwt.RegisteredClaims, error) {
	return s.parseAndValidateToken(tokenString)
}

// parseAndValidateToken is a helper to parse and validate any JWT token
func (s *AuthService) parseAndValidateToken(tokenString string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
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

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ChangePassword changes the user's password after verifying the old password.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	logger.Infof("Processing password change for user: %s", userID)

	// Validate new password strength
	if err := security.ValidatePasswordStrength(newPassword); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return err
	}

	// Hash new password upfront
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		logger.Errorf("Failed to hash new password: %v", err)
		return fmt.Errorf("failed to process new password")
	}

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

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

	// Hash password
	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		logger.Errorf("Failed to hash password: %v", err)
		return nil, fmt.Errorf("failed to process password")
	}

	// Create user
	authPayload := auth.AuthPayload{
		Email:    email,
		Password: hashedPassword,
	}

	createdUser, err := s.userService.CreateUser(ctx, authPayload)
	if err != nil {
		logger.Errorf("Failed to create user: %v", err)
		// Return the actual error message (e.g., "user already exists")
		return nil, err
	}

	// Send email verification (non-blocking) - only if user was created successfully
	userID := createdUser.Id // Capture the ID to avoid race conditions
	go func() {
		bgCtx := context.Background()
		if err := s.SendEmailVerification(bgCtx, userID); err != nil {
			logger.Errorf("Failed to send email verification: %v", err)
		}
	}()

	return &auth.SignupResponse{
		UserID:  createdUser.Id,
		Message: "User created successfully. Please check your email for verification link.",
	}, nil
}

// Login handles complete login flow with token pair generation
func (s *AuthService) Login(ctx context.Context, email, password, userAgent string) (*auth.LoginResponse, error) {
	logger.Info("Processing login for email: %s", email)

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

	// Check email verification
	if !userObj.EmailVerified {
		logger.Error("Email not verified for user: %s", userObj.Id)
		return nil, fmt.Errorf("email not verified. Please verify your email before logging in")
	}

	// Verify password
	if err := s.ComparePassword(userObj.PwdHash, password); err != nil {
		logger.Error("Invalid password for user: %s", userObj.Id)
		return nil, fmt.Errorf("invalid email or password")
	}

	// Create token pair
	accessToken, refreshToken, err := s.CreateTokenPair(ctx, userObj.Id, userAgent)
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

	// Generate secure verification token (32 bytes = 256 bits)
	resetToken, err := security.GenerateSecureToken()
	if err != nil {
		logger.Errorf("Failed to generate reset token: %v", err)
		// Return success to prevent information disclosure (for reset type)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash the token before storing in database
	hashedToken := security.HashTokenWithPepper(resetToken, s.bcryptConfig.Pepper)

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
		logger.Errorf("Failed to create password reset code: %v", err)
		// Return success to prevent information disclosure (for reset type)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("failed to create verification code: %w", err)
	}

	// Create password reset URL with token as query parameter
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.appConfig.Frontend.URL, resetToken)

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

// ResetPasswordWithToken resets user's password using verification token
func (s *AuthService) ResetPasswordWithToken(ctx context.Context, token string, newPassword string) error {
	logger.Infof("Resetting password with token")

	if token == "" {
		return fmt.Errorf("reset token is required")
	}

	if newPassword == "" {
		return fmt.Errorf("new password is required")
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

	var userID string

	// Use transaction to ensure atomicity
	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code record using hashed token
		verificationCode, err := txUserRepo.GetVerificationCodeByCode(ctx, hashedToken, shared.PasswordReset)
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

		userID = verificationCode.UserID
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
	verificationURL := fmt.Sprintf("%s/verify-email?token=%s", s.appConfig.Frontend.URL, verificationToken)

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

// VerifyEmailWithToken verifies user's email using verification token
func (s *AuthService) VerifyEmailWithToken(ctx context.Context, token string) error {
	logger.Infof("Verifying email with token")

	if token == "" {
		return fmt.Errorf("verification token is required")
	}

	// Hash the incoming token to compare with stored hash
	hashedToken := security.HashTokenWithPepper(token, s.bcryptConfig.Pepper)

	var userID string

	// Use transaction to ensure atomicity
	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txUserRepo := s.userRepo.WithTx(tx)

		// Get verification code record using hashed token
		verificationCode, err := txUserRepo.GetVerificationCodeByCode(ctx, hashedToken, shared.EmailVerification)
		if err != nil {
			logger.Errorf("Failed to get verification code: %v", err)
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

		userID = verificationCode.UserID
		return nil
	})

	if err != nil {
		return err
	}

	logger.Infof("Email verified successfully for user: %s", userID)
	return nil
}

// ResendEmailVerification resends email verification for a given email
func (s *AuthService) ResendEmailVerification(ctx context.Context, email string) error {
	logger.Infof("Resending email verification for email: %s", email)

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
