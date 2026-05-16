package user

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	coreBusiness "xcentro.in/internal/core/business"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/types"
	customValidator "xcentro.in/internal/shared/validator"
)

type UserHandler struct {
	userService         user.UserService
	authService         auth.AuthService
	businessUserRelRepo coreBusiness.BusinessUserRelationRepository
	logger              *logger.Logger
}

func NewUserHandler(userService user.UserService, authService auth.AuthService, businessUserRelRepo coreBusiness.BusinessUserRelationRepository) *UserHandler {
	return &UserHandler{
		userService:         userService,
		authService:         authService,
		businessUserRelRepo: businessUserRelRepo,
		logger:              logger.GetDefault(),
	}
}

func RegisterUserRoutes(e *echo.Group, userService user.UserService, authService auth.AuthService, businessUserRelRepo coreBusiness.BusinessUserRelationRepository, cfg *config.Config, cookieManager *httputil.CookieManager) {
	handler := NewUserHandler(userService, authService, businessUserRelRepo)
	aadhaarHandler := NewAadhaarCallbackHandler(userService, cfg)

	userGroup := e.Group("/user")

	// Protected routes (JWT and CSRF middleware first, then rate limiter can use user ID)
	protectedUser := userGroup.Group("")
	protectedUser.Use(middleware.JWTMiddleware(authService))
	protectedUser.Use(middleware.CSRFMiddleware(cookieManager))
	protectedUser.Use(ratelimiter.MiddlewareDefault())

	// User onboarding endpoints — all exempt from KYC gate so re-KYC flow works.
	noKycUser := userGroup.Group("")
	noKycUser.Use(middleware.JWTMiddlewareNoKYCCheck(authService))
	noKycUser.Use(middleware.CSRFMiddleware(cookieManager))
	noKycUser.Use(ratelimiter.MiddlewareDefault())
	noKycUser.POST("/onboard/init", handler.InitiateOnboarding)
	noKycUser.POST("/onboard/complete", handler.CompleteOnboarding)
	noKycUser.GET("/onboard/status", handler.GetOnboardingStatus)

	// User profile endpoints
	protectedUser.GET("/profile", handler.GetProfile)
	protectedUser.PUT("/profile", handler.UpdateProfile)

	// User roles endpoint
	protectedUser.GET("/roles", handler.GetUserRoles)

	// Public routes (rate limiter first since no JWT)
	publicUser := userGroup.Group("")
	publicUser.Use(ratelimiter.MiddlewareDefault())
	// Aadhaar callback and webhook (public, no auth - for debugging)
	publicUser.GET("/onboard/aadhaar/callback", aadhaarHandler.HandleRedirectCallback)
	publicUser.POST("/onboard/aadhaar/webhook", aadhaarHandler.HandleWebhook)
}

// InitiateOnboarding initiates user onboarding process (identity verification only)
func (h *UserHandler) InitiateOnboarding(c echo.Context) error {
	var dto user.InitiateOnboardingDTO

	// Bind input
	if err := c.Bind(&dto); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid request payload", "data": nil})
	}

	// Validate input
	if err := c.Validate(&dto); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	response, err := h.userService.InitiateOnboarding(
		c.Request().Context(),
		userID,
		dto.Name,
		dto.DOB,
		dto.TaxID,
		dto.NationalID,
	)

	if err != nil {
		h.logger.Errorf("Error initiating onboarding for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeKYCAlreadyCompleted:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeInvalidKYCInput:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeVerificationFailed:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to initiate onboarding"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding initiated successfully",
		"data":    response,
	})
}

// CompleteOnboarding completes the user onboarding after Aadhaar verification
// Supports two flows:
// 1. Create new business: businessID from JWT context (bid) + claimedRole in body
// 2. Join existing business: gstin + claimedRole in body
func (h *UserHandler) CompleteOnboarding(c echo.Context) error {
	var dto user.CompleteOnboardingDTO

	// Bind input
	if err := c.Bind(&dto); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid request payload", "data": nil})
	}

	// Validate input
	if err := c.Validate(&dto); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	// Get businessID from JWT context; fall back to body bid for creator flow
	// where active_bid is not yet set in the DB before KYC completes.
	businessID, _ := c.Get("bid").(string)
	if businessID == "" {
		businessID = dto.BusinessID
	}

	// Call CompleteOnboarding with both businessID and gstin
	// Service will determine which flow based on which parameter is provided
	err := h.userService.CompleteOnboarding(c.Request().Context(), userID, businessID, dto.GSTIN, dto.ClaimedRole)

	if err != nil {
		h.logger.Errorf("Error in CompleteOnboarding for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeAadhaarNotInitiated:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeVerificationFailed:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			case types.ErrCodeInvalidKYCInput:
				return c.JSON(400, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}
		if bizErr, ok := err.(*types.BusinessError); ok {
			switch bizErr.Code {
			case types.ErrCodeBusinessNotFound:
				return c.JSON(404, map[string]interface{}{"error": bizErr.Code, "message": bizErr.Error()})
			case types.ErrCodeUserAlreadyAssociated:
				return c.JSON(409, map[string]interface{}{"error": bizErr.Code, "message": bizErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to complete onboarding"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding completed successfully",
		"data":    map[string]string{"status": "kyc_completed"},
	})
}

// GetOnboardingStatus returns the current onboarding status
func (h *UserHandler) GetOnboardingStatus(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	statusData, err := h.userService.GetOnboardingStatus(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error getting onboarding status for user %s: %v", userID, err)

		// Handle typed errors
		if userErr, ok := err.(*types.UserError); ok {
			switch userErr.Code {
			case types.ErrCodeUserNotFound:
				return c.JSON(404, map[string]interface{}{"error": userErr.Code, "message": userErr.Error()})
			}
		}

		return c.JSON(500, map[string]string{"message": "Failed to get onboarding status"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding status fetched successfully",
		"data":    statusData,
	})
}

// GetProfile retrieves the user's profile information with masked national ID
func (h *UserHandler) GetProfile(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	profile, err := h.userService.GetUserProfile(c.Request().Context(), userID)
	if err != nil {
		h.logger.Errorf("Error getting profile for user %s: %v", userID, err)
		// Check if error is "user not found" - return 404 instead of 500
		if err.Error() == "user not found" || err.Error() == "failed to get user profile: user not found" {
			return c.JSON(404, map[string]string{
				"error":   "USER_NOT_FOUND",
				"message": "User not found - your session may have expired, please login again",
			})
		}
		return c.JSON(500, map[string]string{"message": "Failed to get user profile"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Profile retrieved successfully",
		"data":    profile,
	})
}

// UpdateProfile updates the user's profile information (mobile only)
func (h *UserHandler) UpdateProfile(c echo.Context) error {
	var dto user.UpdateProfileDTO

	// Bind input
	if err := c.Bind(&dto); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]any{"message": "Invalid request payload", "data": nil})
	}

	// Validate input
	if err := c.Validate(&dto); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]any{"message": "Validation failed", "errors": ve.Errors, "data": nil})
		}
		return c.JSON(400, map[string]any{"message": "Validation failed", "data": nil})
	}

	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	err := h.userService.UpdateUserProfile(c.Request().Context(), userID, dto.Mobile)
	if err != nil {
		h.logger.Errorf("Error updating profile for user %s: %v", userID, err)
		return c.JSON(500, map[string]string{"message": "Failed to update user profile"})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Profile updated successfully",
		"data":    nil,
	})
}

// GetUserRoles retrieves the user's business roles
func (h *UserHandler) GetUserRoles(c echo.Context) error {
	userID := c.Get("userId").(string)
	if userID == "" {
		return c.JSON(401, map[string]string{"message": "Unauthorized request"})
	}

	allUserRoles, err := h.businessUserRelRepo.FindByUser(c.Request().Context(), userID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch user roles", "error", err, "user_id", userID)
		return c.JSON(500, map[string]any{"message": "Failed to retrieve user roles"})
	}

	type RoleResponse struct {
		UserID     string `json:"userId"`
		BusinessID string `json:"businessId"`
		Role       string `json:"role"`
		KycNeeded  bool   `json:"kycNeeded"`
	}

	roles := make([]RoleResponse, 0, len(allUserRoles))
	for _, rel := range allUserRoles {
		if rel.Role != "" {
			roles = append(roles, RoleResponse{
				UserID:     rel.UserID,
				BusinessID: rel.BID,
				Role:       string(rel.Role),
				KycNeeded:  rel.KycNeeded,
			})
		}
	}

	return c.JSON(200, map[string]interface{}{
		"message": "User roles retrieved successfully",
		"data":    roles,
	})
}
