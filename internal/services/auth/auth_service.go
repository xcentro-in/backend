package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/providers/mailers"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/metrics"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	sharedClients "xcentro.in/internal/shared/clients"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
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
	tokenHash := hashToken(refreshToken)

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
	tokenHash := hashToken(refreshToken)

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
	tokenHash := hashToken(refreshToken)

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

// hashToken creates a SHA-256 hash of the token for storage
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// validatePasswordStrength validates password strength requirements
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	var (
		hasUpper   = false
		hasLower   = false
		hasNumber  = false
		hasSpecial = false
	)

	for _, char := range password {
		switch {
		case 'A' <= char && char <= 'Z':
			hasUpper = true
		case 'a' <= char && char <= 'z':
			hasLower = true
		case '0' <= char && char <= '9':
			hasNumber = true
		case char == '!' || char == '@' || char == '#' || char == '$' || char == '%' || char == '^' || char == '&' || char == '*' || char == '(' || char == ')' || char == '-' || char == '_' || char == '+' || char == '=' || char == '{' || char == '}' || char == '[' || char == ']' || char == '|' || char == '\\' || char == ':' || char == ';' || char == '"' || char == '\'' || char == '<' || char == '>' || char == ',' || char == '.' || char == '?' || char == '/':
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	}

	return nil
}

// ChangePassword changes the user's password after verifying the old password.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	logger.Infof("Processing password change for user: %s", userID)

	// Validate new password strength
	if err := validatePasswordStrength(newPassword); err != nil {
		logger.Errorf("Password validation failed: %v", err)
		return err
	}

	// Get user by ID
	userObj, err := s.userService.GetUserByID(ctx, userID)
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

	// Hash new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		logger.Errorf("Failed to hash new password: %v", err)
		return fmt.Errorf("failed to process new password")
	}

	// Update user password hash
	if err := s.userRepo.UpdateUserPasswordHash(ctx, userID, hashedPassword); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
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
	if err := validatePasswordStrength(password); err != nil {
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
		if err := s.userService.SendEmailVerification(bgCtx, userID); err != nil {
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
func (s *AuthService) SendPasswordResetEmail(ctx context.Context, email string, emailType string) error {
	logger.Infof("Password reset/invitation requested for email: %s, type: %s", email, emailType)

	if email == "" {
		// Still return nil to prevent enumeration
		return nil
	}

	// Get user details - if not found, silently return success for reset type
	userObj, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil || userObj == nil {
		logger.Infof("Password reset/invitation requested for non-existent email: %s", email)
		// SECURITY: Return success even if user doesn't exist (for reset type only)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("user not found")
	}

	// Generate secure verification token (32 bytes = 256 bits)
	resetToken, err := generateSecureToken()
	if err != nil {
		logger.Errorf("Failed to generate reset token: %v", err)
		// Return success to prevent information disclosure (for reset type)
		if emailType == "reset" {
			return nil
		}
		return fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash the token before storing in database
	hashedToken := hashPasswordResetToken(resetToken)

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
	resetURL := fmt.Sprintf("http://localhost:8080/reset-password?token=%s", resetToken)

	// Generate email content based on type
	var subject, htmlBody, textBody string
	if emailType == "invite" {
		subject = "Welcome to Xcentro - Set Your Password"
		htmlBody = s.generateInvitationEmailHTML(userObj.Name, resetURL)
		textBody = s.generateInvitationEmailText(userObj.Name, resetURL)
	} else {
		subject = "Reset your Xcentro password"
		htmlBody = s.generatePasswordResetEmailHTML(resetURL)
		textBody = s.generatePasswordResetEmailText(resetURL)
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

// generatePasswordResetEmailHTML generates HTML content for password reset email
func (s *AuthService) generatePasswordResetEmailHTML(resetURL string) string {
	return fmt.Sprintf(`
		<html>
		<body>
			<h2>Password Reset - Xcentro</h2>
			<p>Hi there,</p>
			<p>We received a request to reset your password. Please click the link below to reset your password:</p>
			<p><a href="%s" style="background-color: #007bff; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px;">Reset Password</a></p>
			<p>Or copy and paste this link in your browser:</p>
			<p>%s</p>
			<p>This link will expire in 1 hour.</p>
			<p>If you didn't request a password reset, please ignore this email and your password will remain unchanged.</p>
			<p><strong>Security Note:</strong> Never share this link with anyone. Xcentro staff will never ask for your reset link.</p>
			<p>Best regards,<br>Xcentro Team</p>
		</body>
		</html>
	`, resetURL, resetURL)
}

// generatePasswordResetEmailText generates plain text content for password reset email
func (s *AuthService) generatePasswordResetEmailText(resetURL string) string {
	return fmt.Sprintf(`
		Password Reset - Xcentro

		Hi there,

		We received a request to reset your password. Please click the link below to reset your password:

		%s

		This link will expire in 1 hour.

		If you didn't request a password reset, please ignore this email and your password will remain unchanged.

		Security Note: Never share this link with anyone. Xcentro staff will never ask for your reset link.

		Best regards,
		Xcentro Team
	`, resetURL)
}

// generateInvitationEmailHTML generates HTML content for user invitation email
func (s *AuthService) generateInvitationEmailHTML(name, resetURL string) string {
	return fmt.Sprintf(`
		<html>
		<body>
			<h2>Welcome to Xcentro!</h2>
			<p>Hi %s,</p>
			<p>You have been invited to join a business on the Xcentro platform.</p>
			<p>To get started, please click the link below to set your password:</p>
			<p><a href="%s" style="background-color: #28a745; color: white; padding: 10px 20px; text-decoration: none; border-radius: 5px;">Set Your Password</a></p>
			<p>Or copy and paste this link in your browser:</p>
			<p>%s</p>
			<p>This link will expire in 1 hour.</p>
			<p><strong>What's next?</strong></p>
			<ul>
				<li>Set your password using the link above</li>
				<li>Log in to your Xcentro account</li>
				<li>Complete your profile and KYC verification</li>
				<li>Start managing business operations</li>
			</ul>
			<p><strong>Security Note:</strong> Never share this link with anyone. Xcentro staff will never ask for your invitation link.</p>
			<p>If you have any questions, feel free to reach out to our support team.</p>
			<p>Best regards,<br>Xcentro Team</p>
		</body>
		</html>
	`, name, resetURL, resetURL)
}

// generateInvitationEmailText generates plain text content for user invitation email
func (s *AuthService) generateInvitationEmailText(name, resetURL string) string {
	return fmt.Sprintf(`
		Welcome to Xcentro!

		Hi %s,

		You have been invited to join a business on the Xcentro platform.

		To get started, please click the link below to set your password:

		%s

		This link will expire in 1 hour.

		What's next?
		- Set your password using the link above
		- Log in to your Xcentro account
		- Complete your profile and KYC verification
		- Start managing business operations

		Security Note: Never share this link with anyone. Xcentro staff will never ask for your invitation link.

		If you have any questions, feel free to reach out to our support team.

		Best regards,
		Xcentro Team
	`, name, resetURL)
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

	// Hash the incoming token to compare with stored hash
	hashedToken := hashPasswordResetToken(token)

	// Get verification code record using hashed token
	verificationCode, err := s.userRepo.GetVerificationCodeByCode(ctx, hashedToken, shared.PasswordReset)
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

	// Hash the new password with pepper
	pepperedPassword := newPassword + s.bcryptConfig.Pepper
	hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(pepperedPassword), s.bcryptConfig.Cost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	hashedPassword := string(hashedPasswordBytes)

	// Mark verification code as used (token is automatically erased after use)
	if err := s.userRepo.MarkVerificationCodeAsUsed(ctx, verificationCode.ID); err != nil {
		return fmt.Errorf("failed to mark reset code as used: %w", err)
	}

	// Update user password hash
	if err := s.userRepo.UpdateUserPasswordHash(ctx, verificationCode.UserID, hashedPassword); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	logger.Infof("Password reset successfully for user: %s", verificationCode.UserID)

	// Invalidate all refresh tokens for security (done after password update)
	logger.Infof("Invalidating all refresh tokens for user: %s", verificationCode.UserID)

	if revokeErr := s.RevokeAllUserTokens(ctx, verificationCode.UserID); revokeErr != nil {
		logger.Errorf("Failed to revoke all tokens for user %s: %v", verificationCode.UserID, revokeErr)
		// Don't fail the password reset if token revocation fails, just log it
	} else {
		logger.Infof("All refresh tokens invalidated for user: %s", verificationCode.UserID)
	}

	return nil
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken() (string, error) {
	// Generate 32 random bytes (256 bits)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(randomBytes), nil
}

// hashPasswordResetToken creates a SHA-256 hash of the token for secure database storage
func hashPasswordResetToken(token string) string {
	// Use application-wide salt from environment/config
	appSalt := "xcentro_password_reset_salt_2024"

	// Combine token with application salt
	saltedToken := fmt.Sprintf("%s_%s_%s", appSalt, token, "xcentro")

	// Create SHA-256 hash
	hash := sha256.Sum256([]byte(saltedToken))

	// Return hex-encoded hash
	return hex.EncodeToString(hash[:])
}
