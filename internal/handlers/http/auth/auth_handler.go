package auth

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
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
	cookieManager *httputil.CookieManager
}

func NewAuthHandler(authService auth.AuthService, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:   authService,
		cookieManager: httputil.NewCookieManager(cfg),
	}
}

func RegisterAuthRoutes(e *echo.Group, authService auth.AuthService, cfg *config.Config) {
	authHandler := NewAuthHandler(authService, cfg)

	authGroup := e.Group("/auth")

	// Public routes
	authGroup.POST("/login", authHandler.HandleLogin)
	authGroup.POST("/signup", authHandler.HandleSignup)
	authGroup.POST("/refresh", authHandler.HandleRefreshToken)
	authGroup.POST("/logout", authHandler.HandleLogout)

	// Protected routes (require JWT)
	protectedAuth := authGroup.Group("")
	protectedAuth.Use(middleware.JWTMiddleware(authService))
	protectedAuth.POST("/revoke", authHandler.HandleRevokeToken)
	protectedAuth.POST("/revoke-all", authHandler.HandleRevokeAllTokens)

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
		return c.JSON(500, map[string]any{"message": err.Error(), "data": nil})
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
			"kycNeeded":    response.KycNeeded,
			"kybNeeded":    response.KybNeeded,
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

// HandleLogout logs out the user by clearing authentication cookies
// This is a client-side logout that doesn't revoke tokens from the database
func (ah AuthHandler) HandleLogout(c echo.Context) error {
	logger.Info("Processing logout request")

	// Clear both access and refresh token cookies
	ah.cookieManager.ClearAllAuthCookies(c)

	return c.JSON(200, map[string]any{
		"message": "Logged out successfully",
		"data":    nil,
	})
}
