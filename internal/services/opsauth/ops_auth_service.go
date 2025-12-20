package opsauth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	redis "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/crypto"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/security"
)

// OpsAuthService implements OAuth2/OIDC authentication for ops dashboard users
type OpsAuthService struct {
	oauth2Config   *oauth2.Config
	oidcVerifier   *oidc.IDTokenVerifier
	jwtConfig      *config.JWTSecretConfig
	oauthConfig    *config.OpsOAuthConfig
	encryptionKey  string
	opsUserRepo    opsauth.OpsUserRepository
	opsSessionRepo opsauth.OpsSessionRepository
	rclient        *redis.RedisClient
	txManager      client.TxManager
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
	// Scopes:
	// - openid: Required for OIDC, provides ID token with sub claim
	// - profile: Required for name and email in ID token
	// - offline_access: Required for refresh tokens
	// - {client_id}/role: Custom API scope for accessing role claims
	roleScope := fmt.Sprintf("%s/role", cfg.OpsOAuth.ClientID)
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.OpsOAuth.ClientID,
		ClientSecret: cfg.OpsOAuth.ClientSecret,
		RedirectURL:  cfg.OpsOAuth.RedirectURL,
		Endpoint:     provider.Endpoint(), // Use discovered endpoints instead of hardcoded
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "offline_access", roleScope},
	}

	// Create OIDC ID token verifier
	oidcVerifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.OpsOAuth.ClientID,
	})

	return &OpsAuthService{
		oauth2Config:   oauth2Config,
		oidcVerifier:   oidcVerifier,
		jwtConfig:      &cfg.JWT,
		oauthConfig:    &cfg.OpsOAuth,
		encryptionKey:  cfg.OpsTokenEncryption.EncryptionKey,
		opsUserRepo:    opsUserRepo,
		opsSessionRepo: opsSessionRepo,
		rclient:        rclient,
		txManager:      txManager,
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

	// Extract specific claims we need from ID token
	var claims struct {
		Sub            string   `json:"sub"`                // Subject - unique Entra user ID (logged but not stored)
		Email          string   `json:"email"`              // Email from email scope
		PreferredEmail string   `json:"preferred_username"` // Fallback for email
		UPN            string   `json:"upn"`                // Another fallback
		Name           string   `json:"name"`               // Name from profile scope
		OID            string   `json:"oid"`                // Object ID from Entra
		Roles          []string `json:"roles"`              // Roles array
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}

	// Log essential ID token claims for debugging
	logger.Infof("ID Token - OID: %s, Email: %s, Name: %s, Roles: %v", claims.OID, claims.Email, claims.Name, claims.Roles)

	// Determine email with fallbacks (profile scope provides email in ID token)
	email := claims.Email
	if email == "" {
		email = claims.PreferredEmail
	}
	if email == "" {
		email = claims.UPN
	}

	// Validate email and name
	if err := validateEmail(email); err != nil {
		return nil, fmt.Errorf("invalid email from ID token: %w", err)
	}
	if strings.TrimSpace(claims.Name) == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	// Extract OID, email, name, and roles from access token (single source of truth)
	oid, accessEmail, accessName, roles, err := s.extractUserDataFromAccessToken(oauth2Token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to extract user data from access token: %w", err)
	}

	// Use access token data as source of truth, fallback to ID token if needed
	if accessEmail == "" {
		accessEmail = email
	}
	if accessName == "" {
		accessName = claims.Name
	}

	logger.Infof("Access Token - OID: %s, Email: %s, Name: %s, Roles: %v", oid, accessEmail, accessName, roles)

	// Encrypt OIDC refresh token before storing
	encryptedOIDCRefreshToken, err := crypto.EncryptBankAccountNumber(oauth2Token.RefreshToken, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt OIDC refresh token: %w", err)
	}

	// Calculate internal access token expiry
	// Use the MINIMUM of: config value OR Microsoft's access token expiry
	// This ensures our internal token never outlives Microsoft's access token
	microsoftTokenExpiry := time.Until(oauth2Token.Expiry)
	configuredExpiry := s.jwtConfig.AccessTokenExpiry
	accessTokenExpiry := configuredExpiry
	if microsoftTokenExpiry < configuredExpiry {
		accessTokenExpiry = microsoftTokenExpiry
		logger.Infof("Using Microsoft token expiry (%v) instead of config (%v)", microsoftTokenExpiry, configuredExpiry)
	}

	// Use configured refresh token expiry for internal refresh tokens
	// Note: oauth2Token.Expiry refers to OIDC access token expiry (~1 hour),
	// NOT the OIDC refresh token expiry (which is typically much longer - 90 days)
	refreshTokenExpiry := s.jwtConfig.RefreshTokenExpiry

	// Generate session ID upfront so it can be included in JWT tokens
	sessionID := uuid.New().String()

	// Use transaction to ensure atomicity of user creation + session creation
	var internalAccessToken, internalRefreshToken string

	err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		// Create or update ops user within transaction using OID as primary key
		txOpsUserRepo := s.opsUserRepo.WithTx(tx)
		var createErr error
		_, _, createErr = txOpsUserRepo.CreateOrUpdateUser(ctx, oid, accessEmail, accessName, roles)
		if createErr != nil {
			return fmt.Errorf("failed to create/update ops user: %w", createErr)
		}

		// Generate our internal tokens (use OID as subject, include session ID)
		// Access token: includes sessionID and roles for authorization
		internalAccessToken, createErr = s.generateToken(oid, sessionID, roles, accessTokenExpiry)
		if createErr != nil {
			return fmt.Errorf("failed to generate internal access token: %w", createErr)
		}

		// Refresh token: minimal claims (OID and sessionID)
		internalRefreshToken, createErr = s.generateToken(oid, sessionID, nil, refreshTokenExpiry)
		if createErr != nil {
			return fmt.Errorf("failed to generate internal refresh token: %w", createErr)
		}

		// Hash internal refresh token for storage
		refreshTokenHash := security.HashToken(internalRefreshToken)

		// Store session with encrypted OIDC refresh token and pre-generated session ID
		expiresAt := time.Now().Add(refreshTokenExpiry)
		txOpsSessionRepo := s.opsSessionRepo.WithTx(tx)
		createErr = txOpsSessionRepo.CreateSession(
			ctx,
			sessionID,
			oid,
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

	logger.Infof("OAuth callback processed successfully for user OID: %s", oid)

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
	sessionID := claims.SessionID
	refreshTokenHash := security.HashToken(refreshToken)

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
		// Check if error is "invalid_grant" (refresh token expired/revoked)
		if strings.Contains(err.Error(), "invalid_grant") || strings.Contains(err.Error(), "AADSTS") {
			logger.Warnf("OIDC refresh token is invalid/expired (invalid_grant): %v", err)
			// Delete the session from database
			if deleteErr := s.opsSessionRepo.DeleteSession(ctx, opsUserID, refreshTokenHash); deleteErr != nil {
				logger.Errorf("Failed to delete expired session: %v", deleteErr)
			}
			// Return specific error for frontend to redirect to login
			return "", fmt.Errorf("REFRESH_TOKEN_EXPIRED")
		}
		logger.Errorf("Failed to refresh OIDC token: %v", err)
		return "", fmt.Errorf("authentication failed")
	}

	// Extract user data from refreshed access token (OID, email, name, roles)
	oid, newEmail, newName, newRoles, err := s.extractUserDataFromAccessToken(newOAuth2Token.AccessToken)
	if err != nil {
		logger.Warnf("Failed to extract user data from refreshed token: %v", err)
		// Get existing data from user record as fallback
		user, getUserErr := s.opsUserRepo.GetUserByID(ctx, opsUserID)
		if getUserErr == nil && user != nil {
			newRoles = user.Roles
			newEmail = user.Email
			newName = user.Name
		} else {
			newRoles = []string{}
		}
	}

	// Verify OID matches (security check)
	if oid != opsUserID {
		logger.Errorf("OID mismatch: token OID=%s, session user=%s", oid, opsUserID)
		return "", fmt.Errorf("authentication failed")
	}

	// Update user email, name, and roles (email can change in Entra)
	if newEmail != "" && newName != "" {
		_, _, err = s.opsUserRepo.CreateOrUpdateUser(ctx, oid, newEmail, newName, newRoles)
		if err != nil {
			logger.Warnf("Failed to update user data: %v", err)
		}
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

	// Calculate internal access token expiry (min of config vs Microsoft's token expiry)
	microsoftTokenExpiry := time.Until(newOAuth2Token.Expiry)
	configuredExpiry := s.jwtConfig.AccessTokenExpiry
	accessTokenExpiry := configuredExpiry
	if microsoftTokenExpiry < configuredExpiry {
		accessTokenExpiry = microsoftTokenExpiry
	}

	// Generate new internal access token with updated roles and session ID
	newAccessToken, err := s.generateToken(oid, sessionID, newRoles, accessTokenExpiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate new access token: %w", err)
	}

	logger.Infof("Access token refreshed successfully for user OID: %s", oid)
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

	logger.Infof("Logout successful for user: %s", opsUserID)
	return nil
}

// ValidateAccessToken validates an access token and returns its claims
func (s *OpsAuthService) ValidateAccessToken(tokenString string) (*opsauth.OpsJWTClaims, error) {
	claims, err := s.parseAndValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Convert to OpsJWTClaims (OID, session ID, and roles)
	opsClaims := &opsauth.OpsJWTClaims{
		Subject:   claims.Subject, // OID from Entra
		SessionID: claims.SessionID,
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

// customClaims extends JWT registered claims with roles only
// For security, only include minimum required data: user ID (sub) and roles
type customClaims struct {
	jwt.RegisteredClaims
	SessionID string   `json:"session_id,omitempty"`
	Roles     []string `json:"roles,omitempty"`
}

// generateToken creates an internal JWT token (access or refresh)
// Only includes: sub (OID), sessionID, roles, exp, iat
// Email/name can be fetched from database if needed
func (s *OpsAuthService) generateToken(opsUserID string, sessionID string, roles []string, expiry time.Duration) (string, error) {
	claims := customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   opsUserID, // OID from Entra
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		SessionID: sessionID,
		Roles:     roles,
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

// extractUserDataFromAccessToken extracts OID, email, name, and roles from the OIDC access token
// For Microsoft Entra ID, this is the single source of truth for user data
func (s *OpsAuthService) extractUserDataFromAccessToken(accessToken string) (oid, email, name string, roles []string, err error) {
	// Parse the access token (it's a JWT)
	token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil {
		return "", "", "", nil, fmt.Errorf("failed to parse access token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", "", nil, fmt.Errorf("invalid claims format")
	}

	// Extract OID (Object ID - unique user identifier in Entra)
	if oidVal, ok := claims["oid"].(string); ok {
		oid = oidVal
	}
	if oid == "" {
		return "", "", "", nil, fmt.Errorf("oid claim not found in access token")
	}

	// Extract email (provided by email scope)
	if emailVal, ok := claims["email"].(string); ok {
		email = emailVal
	}
	if email == "" {
		return "", "", "", nil, fmt.Errorf("email claim not found in access token")
	}

	// Extract name
	if nameVal, ok := claims["name"].(string); ok {
		name = nameVal
	}

	// Extract roles from "roles" claim
	if rolesInterface, ok := claims["roles"]; ok {
		if rolesArray, ok := rolesInterface.([]interface{}); ok {
			roles = make([]string, 0, len(rolesArray))
			for _, role := range rolesArray {
				if roleStr, ok := role.(string); ok {
					roles = append(roles, roleStr)
				}
			}
		}
	}

	// If no roles found, use empty array
	if roles == nil {
		roles = []string{}
	}

	// Log essential access token claims for debugging
	logger.Infof("Access Token - OID: %s, Email: %s, Name: %s, Roles: %v", oid, email, name, roles)

	return oid, email, name, roles, nil
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
