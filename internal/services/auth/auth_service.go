package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

type AuthService struct {
	config          *config.JWTSecretConfig
	rclient         *redis.RedisClient
	sessionRepo     auth.SessionRepository
	userService     user.UserService
	businessService business.BusinessService
}

func NewAuthService(
	cfg *config.JWTSecretConfig,
	rclient *redis.RedisClient,
	sessionRepo auth.SessionRepository,
	userService user.UserService,
	businessService business.BusinessService,
) *AuthService {
	return &AuthService{
		config:          cfg,
		rclient:         rclient,
		sessionRepo:     sessionRepo,
		userService:     userService,
		businessService: businessService,
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

// HashPassword hashes a password using bcrypt
func (s *AuthService) HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// ComparePassword compares a hashed password with a plaintext password
func (s *AuthService) ComparePassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
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
		return nil, fmt.Errorf("failed to create user")
	}

	// Send email verification (non-blocking)
	go func() {
		bgCtx := context.Background()
		if err := s.userService.SendEmailVerification(bgCtx, createdUser.Id); err != nil {
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

	// Get onboarding status (cached)
	onboardingStatus := s.getOnboardingStatus(ctx, userObj)

	return &auth.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		KycNeeded:    onboardingStatus.KycNeeded,
		KybNeeded:    onboardingStatus.KybNeeded,
	}, nil
}

// getOnboardingStatus determines KYC and KYB status
func (s *AuthService) getOnboardingStatus(ctx context.Context, userObj *user.User) *auth.OnboardingStatusCache {
	// KYC Logic
	kycNeeded := false
	if userObj.KycNeeded {
		if userObj.KycStatus != shared.KYCCompleted && userObj.KycStatus != shared.KYCVerified {
			kycNeeded = true
		}
	}

	// KYB Logic
	kybNeeded := false
	onboardingStatus, err := s.businessService.GetOnboardingStatus(ctx, userObj.Id, nil)
	if err != nil {
		logger.Warnf("Failed to get business onboarding status: %v", err)
		kybNeeded = true
	} else if onboardingStatus != nil && onboardingStatus.KYBInitiated {
		kybNeeded = onboardingStatus.KYBStatus != shared.KYBCompleted && onboardingStatus.KYBStatus != shared.KYBVerified
	} else if onboardingStatus != nil && onboardingStatus.KYBStatus == shared.KYBNotStarted {
		kybNeeded = true
	}

	return &auth.OnboardingStatusCache{
		KycNeeded: kycNeeded,
		KybNeeded: kybNeeded,
	}
}
