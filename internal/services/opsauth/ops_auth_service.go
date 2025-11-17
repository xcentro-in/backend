package opsauth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/cache/redis/keys"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/security"
)

// OpsAuthService implements OAuth2/OIDC authentication for ops dashboard users
type OpsAuthService struct {
	oauth2Config      *oauth2.Config
	oidcVerifier      *oidc.IDTokenVerifier
	jwtConfig         *config.JWTSecretConfig
	oauthConfig       *config.OpsOAuthConfig
	encryptionKey     string
	opsUserRepo       opsauth.OpsUserRepository
	opsSessionRepo    opsauth.OpsSessionRepository
	rclient           *redis.RedisClient
	txManager         client.TxManager
}

// NewOpsAuthService creates a new ops authentication service with OAuth2/OIDC support
func NewOpsAuthService(
	opsUserRepo opsauth.OpsUserRepository,
	opsSessionRepo opsauth.OpsSessionRepository,
	rclient *redis.RedisClient,
	txManager client.TxManager,
	cfg *config.Config,
) (*OpsAuthService, error) {
	// Build Office365 OIDC configuration URL
	// For Microsoft Entra ID (formerly Azure AD), the issuer URL follows this pattern:
	// https://login.microsoftonline.com/{tenant}/v2.0
	issuerURL := fmt.Sprintf("https://login.microsoftonline.com/%s/v2.0", cfg.OpsOAuth.TenantID)

	// Create OIDC provider with timeout context (automatically discovers endpoints via .well-known/openid-configuration)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Configure OAuth2 using discovered endpoints from OIDC provider
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.OpsOAuth.ClientID,
		ClientSecret: cfg.OpsOAuth.ClientSecret,
		RedirectURL:  cfg.OpsOAuth.RedirectURL,
		Endpoint:     provider.Endpoint(), // Use discovered endpoints instead of hardcoded
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "offline_access"},
	}

	// Create OIDC ID token verifier
	oidcVerifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.OpsOAuth.ClientID,
	})

	return &OpsAuthService{
		oauth2Config:      oauth2Config,
		oidcVerifier:      oidcVerifier,
		jwtConfig:         &cfg.JWT,
		oauthConfig:       &cfg.OpsOAuth,
		encryptionKey:     cfg.OpsTokenEncryption.EncryptionKey,
		opsUserRepo:       opsUserRepo,
		opsSessionRepo:    opsSessionRepo,
		rclient:           rclient,
		txManager:         txManager,
	}, nil
}

// GetLoginURL generates the OAuth2 authorization URL for Office365 login
func (s *OpsAuthService) GetLoginURL(state string) string {
	return s.oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// HandleCallback processes the OAuth callback, exchanges code for tokens,
// creates/updates ops user, and returns our internal tokens
func (s *OpsAuthService) HandleCallback(
	ctx context.Context,
	code string,
	state string,
	userAgent string,
) (*opsauth.OAuthCallbackResponse, error) {
	logger.Infof("Processing OAuth callback")

	// Exchange authorization code for tokens
	oauth2Token, err := s.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Extract ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in OAuth2 token response")
	}

	// Verify ID token
	idToken, err := s.oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims from ID token
	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}

	// Validate email and name
	if err := validateEmail(claims.Email); err != nil {
		return nil, fmt.Errorf("invalid email from ID token: %w", err)
	}
	if strings.TrimSpace(claims.Name) == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	// Extract roles from access token (if available)
	roles, err := s.extractRolesFromAccessToken(oauth2Token.AccessToken)
	if err != nil {
		logger.Warnf("Failed to extract roles from access token: %v, using empty roles", err)
		roles = []string{} // Default to empty roles if extraction fails
	}

	// Encrypt OIDC refresh token before storing
	encryptedOIDCRefreshToken, err := crypto.EncryptBankAccountNumber(oauth2Token.RefreshToken, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt OIDC refresh token: %w", err)
	}

	// Use configured refresh token expiry for internal refresh tokens
	// Note: oauth2Token.Expiry refers to OIDC access token expiry (~1 hour),
	// NOT the OIDC refresh token expiry (which is typically much longer)
	// We control our internal refresh token lifetime independently
	refreshTokenExpiry := s.jwtConfig.RefreshTokenExpiry

	// Use transaction to ensure atomicity of user creation + session creation
	var opsUserID string
	var internalAccessToken, internalRefreshToken string

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create or update ops user within transaction
		txOpsUserRepo := s.opsUserRepo.WithTx(tx)
		var createErr error
		opsUserID, _, createErr = txOpsUserRepo.CreateOrUpdateUser(ctx, claims.Email, claims.Name, roles)
		if createErr != nil {
			return fmt.Errorf("failed to create/update ops user: %w", createErr)
		}

		// Generate our internal tokens
		internalAccessToken, createErr = s.generateToken(opsUserID, claims.Email, roles, s.jwtConfig.AccessTokenExpiry)
		if createErr != nil {
			return fmt.Errorf("failed to generate internal access token: %w", createErr)
		}

		internalRefreshToken, createErr = s.generateToken(opsUserID, "", nil, refreshTokenExpiry)
		if createErr != nil {
			return fmt.Errorf("failed to generate internal refresh token: %w", createErr)
		}

		// Hash internal refresh token for storage
		refreshTokenHash := security.HashToken(internalRefreshToken)

		// Store session with encrypted OIDC refresh token within transaction
		expiresAt := time.Now().Add(refreshTokenExpiry)
		txOpsSessionRepo := s.opsSessionRepo.WithTx(tx)
		_, createErr = txOpsSessionRepo.CreateSession(
			ctx,
			opsUserID,
			refreshTokenHash,
			encryptedOIDCRefreshToken,
			expiresAt,
			userAgent,
		)
		if createErr != nil {
			return fmt.Errorf("failed to create ops session: %w", createErr)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	logger.Infof("OAuth callback processed successfully for user: %s", opsUserID)

	return &opsauth.OAuthCallbackResponse{
		AccessToken:  internalAccessToken,
		RefreshToken: internalRefreshToken,
	}, nil
}

// RefreshAccessToken validates internal refresh token and issues new access token
// Also refreshes OIDC tokens with provider and updates roles
func (s *OpsAuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (string, error) {
	logger.Infof("Refreshing access token")

	// Parse and validate internal refresh token
	claims, err := s.parseAndValidateToken(refreshToken)
	if err != nil {
		return "", fmt.Errorf("authentication failed")
	}

	opsUserID := claims.Subject
	refreshTokenHash := security.HashToken(refreshToken)

	// Check if token is revoked in Redis
	revokedKey := keys.User.RevokedTokens(opsUserID)
	isRevoked, err := s.rclient.Client.HExists(ctx, revokedKey, refreshTokenHash).Result()
	if err != nil {
		logger.Warnf("Redis error checking revocation: %v", err)
	} else if isRevoked {
		return "", fmt.Errorf("authentication failed")
	}

	// Get session from database
	session, err := s.opsSessionRepo.GetSessionByTokenHash(ctx, opsUserID, refreshTokenHash)
	if err != nil {
		return "", fmt.Errorf("authentication failed")
	}
	if session == nil {
		return "", fmt.Errorf("authentication failed")
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		return "", fmt.Errorf("authentication failed")
	}

	// Decrypt OIDC refresh token
	oidcRefreshToken, err := crypto.DecryptBankAccountNumber(session.OIDCRefreshToken, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("authentication failed")
	}

	// Refresh OIDC tokens with provider
	tokenSource := s.oauth2Config.TokenSource(ctx, &oauth2.Token{
		RefreshToken: oidcRefreshToken,
	})

	newOAuth2Token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("authentication failed")
	}

	// Extract new roles from refreshed access token
	newRoles, err := s.extractRolesFromAccessToken(newOAuth2Token.AccessToken)
	if err != nil {
		logger.Warnf("Failed to extract roles from refreshed token: %v", err)
		// Get existing roles from user record as fallback
		user, getUserErr := s.opsUserRepo.GetUserByID(ctx, opsUserID)
		if getUserErr == nil && user != nil {
			newRoles = user.Roles
		} else {
			newRoles = []string{}
		}
	}

	// Update user roles if changed
	err = s.opsUserRepo.UpdateUserRoles(ctx, opsUserID, newRoles)
	if err != nil {
		logger.Warnf("Failed to update user roles: %v", err)
	}

	// If we got a new OIDC refresh token, encrypt and update it
	if newOAuth2Token.RefreshToken != "" && newOAuth2Token.RefreshToken != oidcRefreshToken {
		encryptedNewOIDCRefreshToken, err := crypto.EncryptBankAccountNumber(newOAuth2Token.RefreshToken, s.encryptionKey)
		if err != nil {
			logger.Errorf("Failed to encrypt new OIDC refresh token: %v", err)
		} else {
			err = s.opsSessionRepo.UpdateOIDCRefreshToken(ctx, opsUserID, refreshTokenHash, encryptedNewOIDCRefreshToken)
			if err != nil {
				logger.Errorf("Failed to update OIDC refresh token in session: %v", err)
			}
		}
	}

	// Update last used timestamp
	err = s.opsSessionRepo.UpdateLastUsed(ctx, opsUserID, refreshTokenHash)
	if err != nil {
		logger.Warnf("Failed to update last used: %v", err)
	}

	// Get user email for new access token
	user, err := s.opsUserRepo.GetUserByID(ctx, opsUserID)
	if err != nil || user == nil {
		return "", fmt.Errorf("authentication failed")
	}

	// Generate new internal access token with updated roles
	newAccessToken, err := s.generateToken(opsUserID, user.Email, newRoles, s.jwtConfig.AccessTokenExpiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate new access token: %w", err)
	}

	logger.Infof("Access token refreshed successfully for user: %s", opsUserID)
	return newAccessToken, nil
}

// Logout revokes the internal refresh token session
func (s *OpsAuthService) Logout(ctx context.Context, opsUserID, refreshToken string) error {
	refreshTokenHash := security.HashToken(refreshToken)

	// Delete session from database
	err := s.opsSessionRepo.DeleteSession(ctx, opsUserID, refreshTokenHash)
	if err != nil {
		return fmt.Errorf("failed to logout: %w", err)
	}

	// Mark as revoked in Redis cache
	revokedKey := keys.User.RevokedTokens(opsUserID)
	if err := s.rclient.Client.HSet(ctx, revokedKey, refreshTokenHash, "1").Err(); err != nil {
		logger.Errorf("Failed to mark token as revoked in Redis: %v", err)
	}

	// Set TTL on revoked tokens hash (15 minutes)
	if err := s.rclient.Client.Expire(ctx, revokedKey, keys.TTLRevokedToken).Err(); err != nil {
		logger.Errorf("Failed to set TTL on revoked tokens: %v", err)
	}

	logger.Infof("Logout successful for user: %s", opsUserID)
	return nil
}

// ValidateAccessToken validates an access token and returns its claims
func (s *OpsAuthService) ValidateAccessToken(tokenString string) (*opsauth.OpsJWTClaims, error) {
	claims, err := s.parseAndValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Convert to OpsJWTClaims
	opsClaims := &opsauth.OpsJWTClaims{
		Subject:   claims.Subject,
		Email:     claims.Email,
		Roles:     claims.Roles,
		IssuedAt:  claims.IssuedAt.Unix(),
		ExpiresAt: claims.ExpiresAt.Unix(),
	}

	return opsClaims, nil
}

// GetUserProfile retrieves the ops user profile by ID
func (s *OpsAuthService) GetUserProfile(ctx context.Context, opsUserID string) (*opsauth.OpsUserProfileResponse, error) {
	user, err := s.opsUserRepo.GetUserByID(ctx, opsUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return &opsauth.OpsUserProfileResponse{
		ID:     user.ID,
		Name:   user.Name,
		Email:  user.Email,
		Roles:  user.Roles,
		Status: user.Status,
	}, nil
}

// ============================================================================
// Helper Methods
// ============================================================================

// customClaims extends JWT registered claims with email and roles
type customClaims struct {
	jwt.RegisteredClaims
	Email string   `json:"email,omitempty"`
	Roles []string `json:"roles,omitempty"`
}

// generateToken creates an internal JWT token (access or refresh)
// Combines the logic for both access and refresh token generation
func (s *OpsAuthService) generateToken(opsUserID, email string, roles []string, expiry time.Duration) (string, error) {
	claims := customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   opsUserID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email: email,
		Roles: roles,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.jwtConfig.SecretKey))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// parseAndValidateToken parses and validates a JWT token
func (s *OpsAuthService) parseAndValidateToken(tokenString string) (*customClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &customClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtConfig.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// extractRolesFromAccessToken extracts roles from the OIDC access token
// For Microsoft Entra ID, roles are typically in the "roles" claim of the access token
func (s *OpsAuthService) extractRolesFromAccessToken(accessToken string) ([]string, error) {
	// Parse the access token (it's a JWT)
	token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse access token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims format")
	}

	// Extract roles from "roles" claim
	rolesInterface, ok := claims["roles"]
	if !ok {
		// No roles claim, return empty array
		return []string{}, nil
	}

	// Convert to string slice
	rolesArray, ok := rolesInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("roles claim is not an array")
	}

	roles := make([]string, 0, len(rolesArray))
	for _, role := range rolesArray {
		roleStr, ok := role.(string)
		if ok {
			roles = append(roles, roleStr)
		}
	}

	return roles, nil
}

// validateEmail validates email format
func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}
	// Basic email format validation
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		return fmt.Errorf("invalid email format")
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format")
	}
	return nil
}
