package opsauth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
)

// OpsAuthHandler handles OAuth2/OIDC authentication requests for ops dashboard
type OpsAuthHandler struct {
	authService   opsauth.OpsAuthService
	cookieManager *httputil.CookieManager
	frontendURL   string
}

// NewOpsAuthHandler creates a new ops authentication handler
func NewOpsAuthHandler(authService opsauth.OpsAuthService, cfg *config.Config) *OpsAuthHandler {
	return &OpsAuthHandler{
		authService:   authService,
		cookieManager: httputil.NewOpsCookieManager(cfg),
		frontendURL:   cfg.Frontend.URL,
	}
}

// HandleLogin initiates the OAuth2 flow by generating authorization URL
// GET /ops/v1/auth/login
func (h *OpsAuthHandler) HandleLogin(c echo.Context) error {
	logger.Info("Initiating OAuth2 login")

	// Generate random state for CSRF protection
	state, err := generateState()
	if err != nil {
		logger.Errorf("Failed to generate state: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to initiate login", "data": nil})
	}

	// Store state in session cookie for verification (path restricted to auth endpoints)
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/ops/v1/auth",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	// Get authorization URL
	authURL := h.authService.GetLoginURL(state)

	return c.JSON(200, map[string]any{
		"message": "Redirect to Office365 login",
		"data": map[string]string{
			"authUrl": authURL,
		},
	})
}

// HandleCallback processes the OAuth2 callback from Office365
// GET /ops/v1/auth/callback?code=xxx&state=xxx
func (h *OpsAuthHandler) HandleCallback(c echo.Context) error {
	logger.Info("Processing OAuth2 callback")

	// Get authorization code (will be validated by Entra ID when exchanged for tokens)
	code := strings.TrimSpace(c.QueryParam("code"))
	if code == "" {
		logger.Error("Missing authorization code")
		return c.JSON(400, map[string]any{"message": "Missing authorization code", "data": nil})
	}

	// Get and validate state parameter
	state := strings.TrimSpace(c.QueryParam("state"))
	if state == "" {
		logger.Error("Missing state parameter")
		return c.JSON(400, map[string]any{"message": "Missing state parameter", "data": nil})
	}

	// Validate state length (CSRF protection requires sufficient entropy)
	if len(state) < 32 || len(state) > 500 {
		logger.Errorf("Invalid state parameter length: %d", len(state))
		return c.JSON(400, map[string]any{"message": "Invalid state parameter format", "data": nil})
	}

	// Verify state for CSRF protection
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		logger.Errorf("Invalid state parameter - CSRF check failed: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid state parameter - CSRF check failed", "data": nil})
	}

	// Clear state cookie
	c.SetCookie(&http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/ops/v1/auth",
		MaxAge: -1,
	})

	// Get user agent
	userAgent := c.Request().UserAgent()

	// Handle callback and exchange code for tokens
	response, err := h.authService.HandleCallback(c.Request().Context(), code, state, userAgent)
	if err != nil {
		logger.Errorf("OAuth callback failed: %v", err)
		return c.JSON(500, map[string]any{"message": "Authentication failed", "data": nil})
	}

	// Set authentication cookies
	h.cookieManager.SetAccessTokenCookie(c, response.AccessToken)
	h.cookieManager.SetRefreshTokenCookie(c, response.RefreshToken)

	// Extract session ID from access token to generate CSRF token
	claims, err := h.authService.ValidateAccessToken(response.AccessToken)
	if err != nil {
		logger.Errorf("Failed to validate access token for CSRF generation: %v", err)
		// Don't fail authentication, CSRF will be validated on next request
	} else {
		// Generate and set CSRF token cookie
		csrfToken := h.cookieManager.GenerateCSRFToken(claims.SessionID)
		h.cookieManager.SetCSRFTokenCookie(c, csrfToken)
	}

	// Redirect to ops dashboard root page
	logger.Info("OAuth callback successful, redirecting to ops dashboard")
	return c.Redirect(http.StatusFound, h.frontendURL)
}

// HandleRefresh refreshes the access token using the internal refresh token
// POST /ops/v1/auth/refresh
func (h *OpsAuthHandler) HandleRefresh(c echo.Context) error {
	logger.Info("Refreshing access token")

	// Read refresh token from cookie
	cookie, err := c.Cookie("ops_refresh_token")
	if err != nil {
		logger.Errorf("Refresh token cookie not found: %v", err)
		return c.JSON(401, map[string]any{
			"error":   "MISSING_REFRESH_TOKEN",
			"message": "Refresh token cookie required",
		})
	}

	refreshToken := strings.TrimSpace(cookie.Value)
	if refreshToken == "" {
		return c.JSON(401, map[string]any{
			"error":   "MISSING_REFRESH_TOKEN",
			"message": "Refresh token is empty",
		})
	}

	// Validate token format (minimal sanity check - service layer handles full validation)
	if len(refreshToken) < 20 {
		logger.Errorf("Refresh token too short: %d characters", len(refreshToken))
		return c.JSON(401, map[string]any{
			"error":   "INVALID_REFRESH_TOKEN",
			"message": "Invalid refresh token format",
		})
	}

	// Refresh access token
	newAccessToken, err := h.authService.RefreshAccessToken(c.Request().Context(), refreshToken)
	if err != nil {
		logger.Errorf("Token refresh failed: %v", err)
		// Check if it's the specific REFRESH_TOKEN_EXPIRED error
		if err.Error() == "REFRESH_TOKEN_EXPIRED" {
			return c.JSON(401, map[string]any{
				"error":   "REFRESH_TOKEN_EXPIRED",
				"message": "Refresh token has expired or is invalid. Please login again.",
			})
		}
		// Generic auth failure
		return c.JSON(401, map[string]any{
			"error":   "AUTHENTICATION_FAILED",
			"message": "Failed to refresh access token. Please login again.",
		})
	}

	// Set new access token cookie
	h.cookieManager.SetAccessTokenCookie(c, newAccessToken)

	return c.JSON(200, map[string]any{
		"message": "Token refreshed successfully",
		"data": map[string]any{
			"accessToken": newAccessToken,
		},
	})
}

// HandleLogout logs out the ops user and revokes the session
// POST /ops/v1/auth/logout
func (h *OpsAuthHandler) HandleLogout(c echo.Context) error {
	logger.Info("Processing logout request")

	// Get ops user ID from context (set by middleware)
	opsUserID, ok := c.Get("opsUserId").(string)
	if !ok || opsUserID == "" {
		// If no user ID, just clear cookies and return success
		h.cookieManager.ClearAllAuthCookies(c)
		return c.JSON(200, map[string]any{
			"message": "Logged out successfully",
			"data":    nil,
		})
	}

	// Try to read refresh token from cookie
	cookie, err := c.Cookie("ops_refresh_token")
	if err == nil && cookie != nil && cookie.Value != "" {
		refreshToken := cookie.Value

		// Revoke the session
		if err := h.authService.Logout(c.Request().Context(), opsUserID, refreshToken); err != nil {
			logger.Warnf("Failed to revoke session during logout: %v", err)
		}
	}

	// Clear authentication cookies
	h.cookieManager.ClearAllAuthCookies(c)

	return c.JSON(200, map[string]any{
		"message": "Logged out successfully",
		"data":    nil,
	})
}

// ============================================================================
// Helper Functions
// ============================================================================

// generateState generates a random state string for CSRF protection
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// RegisterOpsAuthRoutes registers ops authentication routes
func RegisterOpsAuthRoutes(e *echo.Group, authService opsauth.OpsAuthService, cfg *config.Config) {
	handler := NewOpsAuthHandler(authService, cfg)

	authGroup := e.Group("/auth")

	// Public routes (no authentication required)
	authGroup.GET("/login", handler.HandleLogin)
	authGroup.GET("/callback", handler.HandleCallback)
	authGroup.POST("/refresh", handler.HandleRefresh)

	// Logout can work either way (with or without authentication)
	authGroup.POST("/logout", handler.HandleLogout)
}
