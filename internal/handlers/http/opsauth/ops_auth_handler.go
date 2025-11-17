package opsauth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/opsauth"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
)

type OpsAuthHandler struct {
	authService   opsauth.OpsAuthService
	cookieManager *httputil.CookieManager
}

func NewOpsAuthHandler(authService opsauth.OpsAuthService, cfg *config.Config) *OpsAuthHandler {
	return &OpsAuthHandler{
		authService:   authService,
		cookieManager: httputil.NewCookieManager(cfg),
	}
}

// HandleLogin initiates the OAuth2 flow
func (h *OpsAuthHandler) HandleLogin(c echo.Context) error {
	logger.Info("Initiating OAuth2 login")

	// Generate random state for CSRF protection
	state, err := generateState()
	if err != nil {
		logger.Errorf("Failed to generate state: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to initiate login", "data": nil})
	}

	// Store state in session cookie for verification
	c.SetCookie(&http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
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

// HandleCallback processes the OAuth2 callback
func (h *OpsAuthHandler) HandleCallback(c echo.Context) error {
	logger.Info("Processing OAuth2 callback")

	// Get code and state from query parameters
	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" {
		logger.Error("Missing authorization code")
		return c.JSON(400, map[string]any{"message": "Missing authorization code", "data": nil})
	}

	// Verify state for CSRF protection
	stateCookie, err := c.Cookie("oauth_state")
	if err != nil || stateCookie.Value != state {
		logger.Errorf("Invalid state parameter: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid state parameter", "data": nil})
	}

	// Clear state cookie
	c.SetCookie(&http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
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

	return c.JSON(200, map[string]any{
		"message": "Login successful",
		"data": map[string]any{
			"accessToken":  response.AccessToken,
			"refreshToken": response.RefreshToken,
		},
	})
}

// HandleRefresh refreshes the access token using the internal refresh token
func (h *OpsAuthHandler) HandleRefresh(c echo.Context) error {
	logger.Info("Refreshing access token")

	// Read refresh token from cookie
	cookie, err := c.Cookie("refresh_token")
	if err != nil {
		logger.Errorf("Refresh token cookie not found: %v", err)
		return c.JSON(401, map[string]any{"message": "Refresh token cookie required", "data": nil})
	}

	refreshToken := cookie.Value
	if refreshToken == "" {
		return c.JSON(401, map[string]any{"message": "Refresh token is empty", "data": nil})
	}

	// Refresh access token
	newAccessToken, err := h.authService.RefreshAccessToken(c.Request().Context(), refreshToken)
	if err != nil {
		logger.Errorf("Token refresh failed: %v", err)
		return c.JSON(401, map[string]any{"message": "Failed to refresh token", "data": nil})
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

// HandleLogout logs out the ops user
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
	cookie, err := c.Cookie("refresh_token")
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

	// Protected routes would require middleware, but logout can work either way
	authGroup.POST("/logout", handler.HandleLogout)
}
