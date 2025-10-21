package auth

import (
	"context"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
)

func placeHolderFn(c echo.Context) error { return c.String(200, "Not Implemented") }

// AuthHandler handles HTTP requests for authentication
// Delegates all business logic to AuthService
type AuthHandler struct {
	authService   auth.AuthService
	businessService business.BusinessService
	cookieManager *httputil.CookieManager
}

func NewAuthHandler(authService auth.AuthService, businessService business.BusinessService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:   authService,
		businessService: businessService,
		cookieManager: httputil.NewCookieManager(cfg),
	}
}

func RegisterAuthRoutes(e *echo.Group, authService auth.AuthService, businessService business.BusinessService, cfg *config.Config) {
	authHandler := NewAuthHandler(authService, businessService, cfg)

	authGroup := e.Group("/auth")

	// Public routes
	authGroup.POST("/login", authHandler.HandleLogin)
	authGroup.POST("/signup", authHandler.HandleSignup)
	authGroup.POST("/refresh", authHandler.HandleRefreshToken)
	authGroup.POST("/logout", authHandler.HandleLogout)
	authGroup.GET("/forgot-password", authHandler.HandleForgotPassword)
	authGroup.POST("/reset-password", authHandler.HandleResetPassword)

	// Protected routes (require JWT)
	protectedAuth := authGroup.Group("")
	protectedAuth.Use(middleware.JWTMiddleware(authService))
	protectedAuth.POST("/revoke", authHandler.HandleRevokeToken)
	protectedAuth.POST("/revoke-all", authHandler.HandleRevokeAllTokens)
	protectedAuth.POST("/change-password", authHandler.HandleChangePassword)
	protectedAuth.GET("/onboarding-status", authHandler.HandleGetBusinessOnboardingFlags)

	// OAuth sub group
	oauthGroup := authGroup.Group("/Oauth")
	oauthGroup.GET("/redirecturl", placeHolderFn)
	oauthGroup.GET("/callback", placeHolderFn)
}

// HandleSignup processes user registration requests
// Handler responsibility: Input validation and response formatting only
func (ah AuthHandler) HandleSignup(c echo.Context) error {
	var signupReq auth.AuthPayload

	// Bind and validate input
	if err := c.Bind(&signupReq); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if signupReq.Email == "" || signupReq.Password == "" {
		return c.JSON(400, map[string]any{"message": "Email and password are required", "data": nil})
	}

	// Delegate to service
	response, err := ah.authService.Signup(c.Request().Context(), signupReq.Email, signupReq.Password)
	if err != nil {
		logger.Errorf("Signup failed: %v", err)
		// Return 400 for validation errors
		if err.Error() == "password must be at least 8 characters long" ||
			err.Error() == "password must contain at least one uppercase letter, one lowercase letter, one number, and one special character" {
			return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
		}
		// Return 409 for duplicate user
		if err.Error() == "user already exists" {
			return c.JSON(409, map[string]any{"message": "User with this email already exists", "data": nil})
		}
		// Return 500 for actual server errors
		return c.JSON(500, map[string]any{"message": "Failed to create user", "data": nil})
	}

	return c.JSON(201, map[string]any{
		"message": response.Message,
		"data":    map[string]string{"userId": response.UserID},
	})
}

// HandleLogin processes user login requests
// Handler responsibility: Input validation and response formatting only
// Returns access token, refresh token, and onboarding flags
func (ah AuthHandler) HandleLogin(c echo.Context) error {
	var loginReq auth.AuthPayload

	// Bind and validate input
	if err := c.Bind(&loginReq); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if loginReq.Email == "" || loginReq.Password == "" {
		return c.JSON(400, map[string]any{"message": "Email and password are required", "data": nil})
	}

	// Get user agent from request
	userAgent := c.Request().UserAgent()

	// Delegate to service
	response, err := ah.authService.Login(c.Request().Context(), loginReq.Email, loginReq.Password, userAgent)
	if err != nil {
		logger.Errorf("Login failed: %v", err)
		// Return 401 for authentication failures
		if err.Error() == "email not verified. Please verify your email before logging in" ||
			err.Error() == "invalid email or password" {
			return c.JSON(401, map[string]any{"message": err.Error(), "data": nil})
		}
		return c.JSON(500, map[string]any{"message": "Login failed", "data": nil})
	}

	// Set authentication cookies
	ah.cookieManager.SetAccessTokenCookie(c, response.AccessToken)
	ah.cookieManager.SetRefreshTokenCookie(c, response.RefreshToken)

	return c.JSON(200, map[string]any{
		"message": "Login successful",
		"data": map[string]any{
			"accessToken":  response.AccessToken,
			"refreshToken": response.RefreshToken,
		},
	})
}

// HandleRefreshToken processes refresh token requests
func (ah AuthHandler) HandleRefreshToken(c echo.Context) error {
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

	// Delegate to service
	accessToken, err := ah.authService.RefreshAccessToken(c.Request().Context(), refreshToken)
	if err != nil {
		logger.Errorf("Token refresh failed: %v", err)
		return c.JSON(401, map[string]any{"message": err.Error(), "data": nil})
	}

	// Set new access token cookie
	ah.cookieManager.SetAccessTokenCookie(c, accessToken)

	return c.JSON(200, map[string]any{
		"message": "Token refreshed successfully",
		"data": map[string]any{
			"accessToken": accessToken,
		},
	})
}

// HandleRevokeToken revokes a specific refresh token
func (ah AuthHandler) HandleRevokeToken(c echo.Context) error {
	// Get userID from context (set by middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	var req auth.RevokeTokenRequest

	// Bind and validate input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if req.RefreshToken == "" {
		return c.JSON(400, map[string]any{"message": "Refresh token is required", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.RevokeRefreshToken(c.Request().Context(), userID, req.RefreshToken); err != nil {
		logger.Errorf("Token revocation failed: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to revoke token", "data": nil})
	}

	// Clear refresh token cookie
	ah.cookieManager.ClearRefreshTokenCookie(c)

	return c.JSON(200, map[string]any{
		"message": "Token revoked successfully",
		"data":    nil,
	})
}

// HandleRevokeAllTokens revokes all refresh tokens for the current user
func (ah AuthHandler) HandleRevokeAllTokens(c echo.Context) error {
	// Get userID from context (set by middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.RevokeAllUserTokens(c.Request().Context(), userID); err != nil {
		logger.Errorf("Token revocation failed: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to revoke tokens", "data": nil})
	}

	// Clear both access and refresh token cookies
	ah.cookieManager.ClearAllAuthCookies(c)

	return c.JSON(200, map[string]any{
		"message": "All tokens revoked successfully",
		"data":    nil,
	})
}

// HandleLogout logs out the user by clearing authentication cookies and revoking the refresh token
// Cookie clearing is synchronous (fast), token deletion is asynchronous (background cleanup)
func (ah AuthHandler) HandleLogout(c echo.Context) error {
	logger.Info("Processing logout request")

	// Try to read refresh token from cookie for async deletion
	cookie, err := c.Cookie("refresh_token")
	if err == nil && cookie != nil && cookie.Value != "" {
		refreshToken := cookie.Value

		// Launch async goroutine for token deletion (non-blocking)
		go func() {
			bgCtx := context.Background()

			// Validate the refresh token and extract user ID
			claims, err := ah.authService.ValidateAccessToken(refreshToken)
			if err == nil && claims != nil && claims.Subject != "" {
				userID := claims.Subject

				// Delete the specific token from database (keeps table clean)
				if err := ah.authService.RevokeRefreshToken(bgCtx, userID, refreshToken); err != nil {
					logger.Warnf("Failed to revoke refresh token during logout (async): %v", err)
				} else {
					logger.Infof("Refresh token revoked successfully for user: %s (async)", userID)
				}
			} else {
				logger.Warnf("Invalid or expired refresh token during logout (async): %v", err)
			}
		}()
	} else {
		logger.Info("No refresh token found in cookies during logout")
	}

	// Clear both access and refresh token cookies (synchronous - always fast)
	ah.cookieManager.ClearAllAuthCookies(c)

	// Return immediately (fast logout response)
	return c.JSON(200, map[string]any{
		"message": "Logged out successfully",
		"data":    nil,
	})
}

// HandleForgotPassword processes forgot password requests
// Sends a password reset email with token link
func (ah AuthHandler) HandleForgotPassword(c echo.Context) error {
	// Get email from query parameter
	email := c.QueryParam("email")
	if email == "" {
		return c.JSON(400, map[string]any{"message": "Email is required", "data": nil})
	}

	// Delegate to service (always returns success to prevent email enumeration)
	if err := ah.authService.SendPasswordResetEmail(c.Request().Context(), email, "reset"); err != nil {
		logger.Errorf("Failed to send password reset email: %v", err)
		// Still return success to user to prevent email enumeration
	}

	return c.JSON(200, map[string]any{
		"message": "If the email exists, a password reset link has been sent",
		"data":    nil,
	})
}

// HandleResetPassword processes password reset with token
func (ah AuthHandler) HandleResetPassword(c echo.Context) error {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}

	// Bind and validate input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if req.Token == "" || req.NewPassword == "" {
		return c.JSON(400, map[string]any{"message": "Token and new password are required", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.ResetPasswordWithToken(c.Request().Context(), req.Token, req.NewPassword); err != nil {
		logger.Errorf("Password reset failed: %v", err)
		return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Password has been reset successfully. All existing sessions have been invalidated.",
		"data":    nil,
	})
}

// HandleChangePassword processes requests to change the user's password
func (ah AuthHandler) HandleChangePassword(c echo.Context) error {
	// Get userID from context (set by middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}

	// Bind and validate input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		return c.JSON(400, map[string]any{"message": "Old and new passwords are required", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.ChangePassword(c.Request().Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		logger.Errorf("Password change failed: %v", err)
		// Return 401 for authentication failures (wrong old password)
		if err.Error() == "invalid old password" {
			return c.JSON(401, map[string]any{"message": "Current password is incorrect", "data": nil})
		}
		// Return 400 for validation errors
		if err.Error() == "new password cannot be the same as old password" ||
			err.Error() == "password must be at least 8 characters long" ||
			err.Error() == "password must contain at least one uppercase letter, one lowercase letter, one number, and one special character" {
			return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
		}
		// Return 500 for actual server errors
		return c.JSON(500, map[string]any{"message": "Failed to change password", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Password changed successfully. All existing sessions have been invalidated.",
		"data":    nil,
	})
}

// HandleGetBusinessOnboardingFlags retrieves the onboarding status flags for a business.
func (ah AuthHandler) HandleGetBusinessOnboardingFlags(c echo.Context) error {
	// Get businessID from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	status, err := ah.businessService.GetBusinessOnboardingFlags(c.Request().Context(), businessID)
	if err != nil {
		logger.Errorf("Failed to get business onboarding flags: %v", err)
		return c.JSON(500, map[string]any{"message": "Failed to retrieve onboarding status", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Business onboarding status retrieved successfully",
		"data":    status,
	})
}
