package auth

import (
	"context"
	"strings"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	customValidator "xcentro.in/internal/shared/validator"
)

func placeHolderFn(c echo.Context) error { return c.String(200, "Not Implemented") }

// AuthHandler handles HTTP requests for authentication
// Delegates all business logic to AuthService
type AuthHandler struct {
	authService     auth.AuthService
	businessService business.BusinessService
	cookieManager   *httputil.CookieManager
}

func NewAuthHandler(authService auth.AuthService, businessService business.BusinessService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:     authService,
		businessService: businessService,
		cookieManager:   httputil.NewCookieManager(cfg),
	}
}

func RegisterAuthRoutes(e *echo.Group, authService auth.AuthService, businessService business.BusinessService, cfg *config.Config, cookieManager *httputil.CookieManager) {
	authHandler := NewAuthHandler(authService, businessService, cfg)

	authGroup := e.Group("/auth")

	// Public routes (rate limiter runs first since no JWT)
	publicAuth := authGroup.Group("")
	publicAuth.Use(ratelimiter.MiddlewareDefault())
	publicAuth.POST("/login", authHandler.HandleLogin, middleware.TurnstileMiddleware())
	publicAuth.POST("/signup", authHandler.HandleSignup, middleware.TurnstileMiddleware())
	publicAuth.POST("/refresh", authHandler.HandleRefreshToken)
	publicAuth.POST("/logout", authHandler.HandleLogout)
	publicAuth.GET("/forgot-password", authHandler.HandleForgotPassword)
	publicAuth.POST("/reset-password", authHandler.HandleResetPassword, middleware.TurnstileMiddleware())
	publicAuth.POST("/reset-password-with-otp", authHandler.HandleResetPasswordWithOTP, middleware.TurnstileMiddleware())
	publicAuth.POST("/send-invite-otp", authHandler.HandleSendInviteOTP, middleware.TurnstileMiddleware())
	publicAuth.POST("/verify-email", authHandler.HandleVerifyEmail, middleware.TurnstileMiddleware())
	publicAuth.POST("/verify-email-with-mobile", authHandler.HandleVerifyEmailWithMobile, middleware.TurnstileMiddleware())
	publicAuth.POST("/verify-mobile-otp", authHandler.HandleVerifyMobileOTP, middleware.TurnstileMiddleware())
	publicAuth.POST("/resend-verification", authHandler.HandleResendEmailVerification, middleware.TurnstileMiddleware())

	// Protected routes (JWT and CSRF middleware first, then rate limiter can use user ID)
	protectedAuth := authGroup.Group("")
	protectedAuth.Use(middleware.JWTMiddleware(authService))
	protectedAuth.Use(middleware.CSRFMiddleware(cookieManager))
	protectedAuth.Use(ratelimiter.MiddlewareDefault())
	protectedAuth.POST("/revoke", authHandler.HandleRevokeToken)
	protectedAuth.POST("/revoke-all", authHandler.HandleRevokeAllTokens)
	protectedAuth.POST("/generate-mobile-otp", authHandler.HandleGenerateMobileOTP, middleware.TurnstileMiddleware())
	protectedAuth.POST("/change-password", authHandler.HandleChangePassword, middleware.TurnstileMiddleware())
	protectedAuth.POST("/send-mobile-update-otp", authHandler.HandleSendMobileUpdateOTP, middleware.TurnstileMiddleware())
	protectedAuth.POST("/verify-mobile-update", authHandler.HandleVerifyMobileUpdate, middleware.TurnstileMiddleware())
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

	// Bind input
	if err := c.Bind(&signupReq); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&signupReq); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
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

	// Bind input
	if err := c.Bind(&loginReq); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&loginReq); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Get user agent from request
	userAgent := c.Request().UserAgent()

	// Get device type from request (defaults to "web" if not provided)
	deviceType := loginReq.DeviceType
	if deviceType == "" {
		deviceType = "web"
	}

	// TODO: Support mobile device type in future - currently only "web" is supported
	if deviceType != "web" {
		logger.Warnf("Unsupported device type '%s' requested, rejecting", deviceType)
		return c.JSON(400, map[string]any{
			"message": "Only 'web' device type is currently supported",
			"data":    nil,
		})
	}

	// Delegate to service
	response, err := ah.authService.Login(c.Request().Context(), loginReq.Email, loginReq.Password, userAgent, deviceType)
	if err != nil {
		logger.Errorf("Login failed: %v", err)
		// Return 401 for authentication failures
		if err.Error() == "account verification incomplete. Please verify both your email and mobile number" ||
			err.Error() == "invalid email or password" {
			return c.JSON(401, map[string]any{"message": err.Error(), "data": nil})
		}
		return c.JSON(500, map[string]any{"message": "Login failed", "data": nil})
	}

	// Set authentication cookies
	ah.cookieManager.SetAccessTokenCookie(c, response.AccessToken)
	ah.cookieManager.SetRefreshTokenCookie(c, response.RefreshToken)

	// Extract session ID from access token to generate CSRF token
	claims, err := ah.authService.ValidateAccessToken(response.AccessToken)
	if err != nil {
		logger.Errorf("Failed to validate access token for CSRF generation: %v", err)
		// Don't fail login, CSRF will be validated on next request
	} else {
		// Generate and set CSRF token cookie
		csrfToken := ah.cookieManager.GenerateCSRFToken(claims.SessionID)
		ah.cookieManager.SetCSRFTokenCookie(c, csrfToken)
	}

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

	refreshToken := strings.TrimSpace(cookie.Value)
	if refreshToken == "" {
		return c.JSON(401, map[string]any{"message": "Refresh token is empty", "data": nil})
	}

	// Validate token format (minimal sanity check - service layer handles full validation)
	if len(refreshToken) < 20 {
		logger.Errorf("Refresh token too short: %d characters", len(refreshToken))
		return c.JSON(401, map[string]any{"message": "Invalid refresh token format", "data": nil})
	}

	// Delegate to service
	accessToken, err := ah.authService.RefreshAccessToken(c.Request().Context(), refreshToken)
	if err != nil {
		logger.Errorf("Token refresh failed: %v", err)
		return c.JSON(401, map[string]any{"message": "Failed to refresh token", "data": nil})
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

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
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
	email := strings.TrimSpace(c.QueryParam("email"))
	if email == "" {
		return c.JSON(400, map[string]any{"message": "Email is required", "data": nil})
	}

	// Validate email format
	if v, ok := c.Echo().Validator.(*customValidator.CustomValidator); ok {
		if err := v.Var(email, "required,email"); err != nil {
			logger.Errorf("Invalid email format: %s", email)
			return c.JSON(400, map[string]any{"message": "Invalid email format", "data": nil})
		}
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
	var req auth.ResetPasswordRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.ResetPasswordWithToken(c.Request().Context(), req.UserID, req.Token, req.Signature, req.NewPassword); err != nil {
		logger.Errorf("Password reset failed: %v", err)
		return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Password has been reset successfully. All existing sessions have been invalidated.",
		"data":    nil,
	})
}

// HandleGenerateMobileOTP generates and sends OTP to user's registered mobile number
func (ah AuthHandler) HandleGenerateMobileOTP(c echo.Context) error {
	// Get userID from context (set by JWT middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	// Delegate to service (no request body needed, uses userID from JWT)
	if err := ah.authService.GenerateMobileOTP(c.Request().Context(), userID); err != nil {
		logger.Errorf("Failed to generate mobile OTP for user %s: %v", userID, err)

		// Return 404 if user doesn't have mobile number
		if err.Error() == "user does not have a mobile number registered" {
			return c.JSON(404, map[string]any{"message": "Mobile number not registered. Please update your profile.", "data": nil})
		}

		// Return 500 for other errors
		return c.JSON(500, map[string]any{"message": "Failed to send OTP", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "OTP sent successfully to your registered mobile number",
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

	var req auth.ChangePasswordRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.ChangePassword(c.Request().Context(), userID, req.OldPassword, req.NewPassword, req.OTP); err != nil {
		logger.Errorf("Password change failed: %v", err)
		// Return 401 for authentication failures (wrong old password)
		if err.Error() == "invalid old password" {
			return c.JSON(401, map[string]any{"message": "Current password is incorrect", "data": nil})
		}
		// Return 400 for OTP errors
		if err.Error() == "invalid or expired OTP" || err.Error() == "OTP already used" || err.Error() == "OTP has expired" {
			return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
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
	businessID := strings.TrimSpace(c.Request().Header.Get("X-Business-ID"))
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Validate UUID format
	if v, ok := c.Echo().Validator.(*customValidator.CustomValidator); ok {
		if err := v.Var(businessID, "required,uuid"); err != nil {
			logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
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

// HandleVerifyEmail verifies user's email using verification token
func (ah AuthHandler) HandleVerifyEmail(c echo.Context) error {
	var req auth.VerifyEmailRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid request payload", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	if err := ah.authService.VerifyEmailWithToken(c.Request().Context(), req.UserID, req.Token, req.Signature); err != nil {
		logger.Errorf("Error verifying email with token: %v", err)

		// Handle specific error cases
		switch err.Error() {
		case "verification token not found":
			return c.JSON(404, map[string]string{"message": "Invalid verification token"})
		case "verification token already used":
			return c.JSON(400, map[string]string{"message": "Verification token has already been used"})
		case "verification token has expired":
			return c.JSON(400, map[string]string{"message": "Verification token has expired"})
		case "invalid or tampered verification link":
			return c.JSON(400, map[string]string{"message": "Invalid or tampered verification link"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to verify email"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Email verified successfully. You can now log in.",
		"data":    nil,
	})
}

// HandleResendEmailVerification resends email verification token
func (ah AuthHandler) HandleResendEmailVerification(c echo.Context) error {
	var req auth.ResendVerificationRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid request payload", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	if err := ah.authService.ResendEmailVerification(c.Request().Context(), req.Email); err != nil {
		logger.Errorf("Error resending email verification for %s: %v", req.Email, err)

		// Handle specific error cases
		switch err.Error() {
		case "user not found":
			return c.JSON(404, map[string]string{"message": "User not found"})
		case "email already verified":
			return c.JSON(400, map[string]string{"message": "Email is already verified"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to resend verification email"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Verification email sent successfully. Please check your inbox.",
		"data":    nil,
	})
}

// ============================================================================
// Mobile Verification Handlers (Phase 2)
// ============================================================================

// HandleVerifyEmailWithMobile verifies email and captures mobile number for OTP
func (ah AuthHandler) HandleVerifyEmailWithMobile(c echo.Context) error {
	var req auth.VerifyEmailWithMobileRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]string{"message": "Invalid input provided"})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed"})
	}

	// Call service
	if err := ah.authService.VerifyEmailWithMobile(c.Request().Context(), req.UserID, req.Token, req.Signature, req.MobileNumber); err != nil {
		logger.Errorf("Error verifying email with mobile: %v", err)

		// Handle specific error cases
		switch {
		case err.Error() == "verification token not found":
			return c.JSON(404, map[string]string{"message": "Invalid verification token"})
		case err.Error() == "verification token already used":
			return c.JSON(400, map[string]string{"message": "Verification token has already been used"})
		case err.Error() == "verification token has expired":
			return c.JSON(400, map[string]string{"message": "Verification token has expired"})
		case err.Error() == "invalid or tampered verification link":
			return c.JSON(400, map[string]string{"message": "Invalid or tampered verification link"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to verify email and send OTP"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Email verified successfully. OTP sent to your mobile number.",
		"data":    nil,
	})
}

// HandleVerifyMobileOTP verifies the mobile OTP and marks mobile as verified
func (ah AuthHandler) HandleVerifyMobileOTP(c echo.Context) error {
	var req auth.VerifyMobileOTPRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]string{"message": "Invalid input provided"})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed"})
	}

	// Call service
	if err := ah.authService.VerifyMobileOTP(c.Request().Context(), req.UserID, req.OTP); err != nil {
		logger.Errorf("Error verifying mobile OTP: %v", err)

		// Handle specific error cases
		switch {
		case err.Error() == "invalid or expired OTP":
			return c.JSON(400, map[string]string{"message": "Invalid or expired OTP"})
		case err.Error() == "OTP already used":
			return c.JSON(400, map[string]string{"message": "OTP has already been used"})
		case err.Error() == "OTP has expired":
			return c.JSON(400, map[string]string{"message": "OTP has expired"})
		default:
			return c.JSON(500, map[string]string{"message": "Failed to verify mobile OTP"})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Mobile number verified successfully. You can now log in.",
		"data":    nil,
	})
}

// HandleResetPasswordWithOTP resets password with mobile OTP verification (for invited users)
// HandleSendInviteOTP sends OTP to invited user's registered mobile number
func (ah AuthHandler) HandleSendInviteOTP(c echo.Context) error {
	var req auth.SendInviteOTPRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.SendInviteOTP(c.Request().Context(), req.UserID, req.Token, req.Signature); err != nil {
		logger.Errorf("Failed to send invite OTP for user %s: %v", req.UserID, err)

		// Return 403 for invalid/tampered link
		if err.Error() == "invalid or tampered invitation link" {
			return c.JSON(403, map[string]any{"message": "Invalid invitation link", "data": nil})
		}

		// Return 404 if user doesn't have mobile number
		if err.Error() == "user does not have a mobile number registered" {
			return c.JSON(404, map[string]any{"message": "Mobile number not found for this user", "data": nil})
		}

		// Return 500 for other errors
		return c.JSON(500, map[string]any{"message": "Failed to send OTP", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "OTP sent successfully to your registered mobile number",
		"data":    nil,
	})
}

func (ah AuthHandler) HandleResetPasswordWithOTP(c echo.Context) error {
	var req auth.ResetPasswordWithMobileOTPRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.ResetPasswordWithMobileOTP(c.Request().Context(), req.UserID, req.Token, req.Signature, req.OTP, req.NewPassword); err != nil {
		logger.Errorf("Password reset with OTP failed: %v", err)
		return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Password reset and mobile verified successfully. All existing sessions have been invalidated.",
		"data":    nil,
	})
}

// HandleSendMobileUpdateOTP sends OTP to new mobile number for update
func (ah AuthHandler) HandleSendMobileUpdateOTP(c echo.Context) error {
	// Get userID from context (set by JWT middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	var req auth.SendMobileUpdateOTPRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service (reuses existing SendMobileVerificationOTP)
	if err := ah.authService.SendMobileVerificationOTP(c.Request().Context(), userID, req.MobileNumber); err != nil {
		logger.Errorf("Failed to send mobile update OTP for user %s: %v", userID, err)
		return c.JSON(500, map[string]any{"message": "Failed to send OTP", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "OTP sent successfully to the new mobile number",
		"data":    nil,
	})
}

// HandleVerifyMobileUpdate verifies OTP and updates mobile number
func (ah AuthHandler) HandleVerifyMobileUpdate(c echo.Context) error {
	// Get userID from context (set by JWT middleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(401, map[string]any{"message": "Unauthorized", "data": nil})
	}

	var req auth.VerifyMobileUpdateRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid input provided", "data": nil})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	// Delegate to service
	if err := ah.authService.VerifyAndUpdateMobile(c.Request().Context(), userID, req.MobileNumber, req.OTP); err != nil {
		logger.Errorf("Failed to verify and update mobile for user %s: %v", userID, err)

		// Return 400 for OTP errors
		if err.Error() == "invalid or expired OTP" || err.Error() == "OTP already used" || err.Error() == "OTP has expired" {
			return c.JSON(400, map[string]any{"message": err.Error(), "data": nil})
		}

		// Return 500 for other errors
		return c.JSON(500, map[string]any{"message": "Failed to update mobile number", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Mobile number updated and verified successfully",
		"data":    nil,
	})
}
