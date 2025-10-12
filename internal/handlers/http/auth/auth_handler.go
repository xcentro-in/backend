package auth

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/shared/logger"
)

func placeHolderFn(c echo.Context) error { return c.String(200, "This is a placeholder function") }

// AuthHandler handles HTTP requests for authentication
// Delegates all business logic to AuthService
type AuthHandler struct {
	authService auth.AuthService
}

func NewAuthHandler(authService auth.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func RegisterAuthRoutes(e *echo.Group, authService auth.AuthService) {
	authHandler := NewAuthHandler(authService)

	authGroup := e.Group("/auth")
	/**
	  can add middleware here for auth group and its subgroups
	**/
	authGroup.POST("/login", authHandler.HandleLogin)
	authGroup.POST("/signup", authHandler.HandleSignup)

	// Sub group is prefixed with /auth
	oauthGroup := authGroup.Group("/Oauth")
	/**
	  can add middleware here for Oauth group and its subgroups
	**/
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
// Returns minimal response: token, kycNeeded, kybNeeded
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

	// Delegate to service
	response, err := ah.authService.Login(c.Request().Context(), loginReq.Email, loginReq.Password)
	if err != nil {
		logger.Errorf("Login failed: %v", err)
		// Return 401 for authentication failures
		if err.Error() == "email not verified. Please verify your email before logging in" ||
			err.Error() == "invalid email or password" {
			return c.JSON(401, map[string]any{"message": err.Error(), "data": nil})
		}
		return c.JSON(500, map[string]any{"message": "Login failed", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Login successful",
		"data": map[string]any{
			"token":     response.Token,
			"kycNeeded": response.KycNeeded,
			"kybNeeded": response.KybNeeded,
		},
	})
}
