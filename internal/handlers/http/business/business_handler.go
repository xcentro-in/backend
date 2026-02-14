package business

import (
	"context"
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/types"
	customValidator "xcentro.in/internal/shared/validator"
)

type BusinessHandler struct {
	businessService business.BusinessService
	authService     auth.AuthService
	userService     user.UserService
	S3Config        *config.S3Config
	logger          *logger.Logger
}

func NewBusinessHandler(businessService business.BusinessService, authService auth.AuthService, userService user.UserService, s3Config *config.S3Config) *BusinessHandler {
	return &BusinessHandler{
		businessService: businessService,
		authService:     authService,
		userService:     userService,
		S3Config:        s3Config,
		logger:          logger.GetDefault(),
	}
}

// getRequesterRoleForBusiness extracts the requester's role for a specific business from middleware context
func (h *BusinessHandler) getRequesterRoleForBusiness(c echo.Context, businessID string) (shared.UserRole, error) {
	ur := c.Get("userRoles")
	if ur == nil {
		return "", fmt.Errorf("no business association found")
	}
	roles := ur.([]*business.BusinessUserRelation)

	for _, rel := range roles {
		if rel.BID == businessID {
			return rel.Role, nil
		}
	}

	return "", fmt.Errorf("you don't have access to this business")
}

func RegisterBusinessRoutes(e *echo.Group, businessService business.BusinessService, authService auth.AuthService, userService user.UserService, s3Config *config.S3Config, businessUserRelRepo business.BusinessUserRelationRepository, rclient *redisClient.RedisClient, cookieManager *httputil.CookieManager) {
	handler := NewBusinessHandler(businessService, authService, userService, s3Config)

	businessGroup := e.Group("/business")
	// Apply middleware to the business group (JWT, CSRF, rate limiter, then business roles)
	businessGroup.Use(middleware.JWTMiddleware(authService))
	businessGroup.Use(middleware.CSRFMiddleware(cookieManager))
	businessGroup.Use(ratelimiter.MiddlewareDefault())
	businessGroup.Use(middleware.BusinessRolesMiddleware(businessUserRelRepo, rclient))

	// Business details route
	businessGroup.GET("", handler.GetBusinessDetails)

	// Onboarding routes
	onboardingGroup := businessGroup.Group("/onboard")
	onboardingGroup.POST("/init", handler.InitiateOnboarding, middleware.TurnstileMiddleware())
	onboardingGroup.POST("/complete", handler.CompleteOnboarding, middleware.TurnstileMiddleware())
	onboardingGroup.GET("/status", handler.GetOnboardingStatus)
	onboardingGroup.GET("/upload", handler.UploadKYBDocuments)
	onboardingGroup.POST("/confirm-upload", handler.ConfirmKYBDocumentsUpload, middleware.TurnstileMiddleware())

	// Invite user route
	businessGroup.POST("/invite-user", handler.InviteUser, middleware.TurnstileMiddleware())

	// User management routes
	businessGroup.GET("/users", handler.ListBusinessUsers)
	// TODO: fix APi signature, why to pass ID in bodoy? - should be passed in params
	// TODO: chose a consistent APi signature when dealing with resources
	businessGroup.POST("/users/update", handler.UpdateBusinessUser, middleware.TurnstileMiddleware())
	businessGroup.DELETE("/users", handler.RemoveBusinessUser, middleware.TurnstileMiddleware())
}

func (h *BusinessHandler) UploadKYBDocuments(c echo.Context) error {
	// Get user roles from middleware
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Get presigned URL from service (service handles authorization and S3 operations)
	url, err := h.businessService.GetKYBDocumentUploadURL(c.Request().Context(), roles)
	if err != nil {
		h.logger.Errorf("Error getting upload URL: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to generate upload URL",
		})
	}

	return c.JSON(200, map[string]interface{}{"message": "Fetched presigned upload URL successfully", "data": map[string]interface{}{"url": url}})
}

func (h *BusinessHandler) ConfirmKYBDocumentsUpload(c echo.Context) error {
	// Get user roles from middleware
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Confirm upload (service handles authorization, S3 operations, and DB update)
	fileurl, err := h.businessService.ConfirmKYBDocumentUpload(c.Request().Context(), roles)
	if err != nil {
		h.logger.Errorf("Error confirming upload: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to confirm document upload",
		})
	}

	return c.JSON(200, map[string]interface{}{"message": "Confirmed KYB documents upload successfully", "data": map[string]interface{}{"url": fileurl}})
}

// ============================================================================
// New Onboarding Handlers
// ============================================================================

// InitiateOnboarding creates a new business and starts the KYB process
func (h *BusinessHandler) InitiateOnboarding(c echo.Context) error {
	// Check if user already has business relations
	ur := c.Get("userRoles")
	if ur != nil {
		roles := ur.([]*business.BusinessUserRelation)
		if len(roles) > 0 {
			return c.JSON(403, map[string]interface{}{
				"error":   "USER_ALREADY_ASSOCIATED",
				"message": "User is already associated with a business",
			})
		}
	}

	// Parse request
	var req business.InitiateOnboardingRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	// Get user ID from context
	userIDRaw := c.Get("userId")
	if userIDRaw == nil {
		return c.JSON(401, map[string]interface{}{
			"error":   "UNAUTHORIZED",
			"message": "User not authenticated",
		})
	}
	userID := userIDRaw.(string)

	// Initiate onboarding
	response, err := h.businessService.InitiateOnboarding(c.Request().Context(), userID, req.Name, req.GSTIN)
	if err != nil {
		h.logger.Errorf("Error initiating onboarding: %v", err)

		// Return appropriate error response based on error type
		return h.handleBusinessError(c, err)
	}

	return c.JSON(201, map[string]interface{}{
		"message": "Business created successfully",
		"data":    response,
	})
}

// CompleteOnboarding completes the KYB process with business details
func (h *BusinessHandler) CompleteOnboarding(c echo.Context) error {
	// Parse request
	var req business.CompleteKYBRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Get user ID from context
	userIDRaw := c.Get("userId")
	if userIDRaw == nil {
		return c.JSON(401, map[string]interface{}{
			"error":   "UNAUTHORIZED",
			"message": "User not authenticated",
		})
	}
	userID := userIDRaw.(string)

	// Get businessID from header and set it BEFORE validation
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	req.BID = businessID

	// Auto-populate ITID from business data if not provided
	if req.ITID == "" {
		businessData, err := h.businessService.GetBusiness(c.Request().Context(), businessID)
		if err != nil {
			h.logger.Errorf("Failed to fetch business data: %v", err)
			return c.JSON(400, map[string]interface{}{
				"error":   "BUSINESS_NOT_FOUND",
				"message": "Business not found or not accessible",
			})
		}
		if businessData.ITID == "" {
			h.logger.Errorf("Business %s has no ITID - initiate onboarding first", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INCOMPLETE_ONBOARDING",
				"message": "Business onboarding not initiated - please call initiate endpoint first",
			})
		}
		req.ITID = businessData.ITID
		h.logger.Infof("Auto-populated ITID %s from business data for BID %s", req.ITID, businessID)
	}

	// Validate input (after BID and ITID are set)
	if err := c.Validate(&req); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	// Get user roles from middleware context
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	userRoles := ur.([]*business.BusinessUserRelation)

	// Complete KYB (passing middleware-injected roles)
	response, err := h.businessService.CompleteKYB(c.Request().Context(), userID, userRoles, &req)
	if err != nil {
		h.logger.Errorf("Error completing KYB: %v", err)
		return h.handleBusinessError(c, err)
	}

	return c.JSON(200, map[string]interface{}{
		"message": "KYB completed successfully",
		"data":    response,
	})
}

// GetOnboardingStatus retrieves the current onboarding status for the user
func (h *BusinessHandler) GetOnboardingStatus(c echo.Context) error {
	// Get user ID from context
	userIDRaw := c.Get("userId")
	if userIDRaw == nil {
		return c.JSON(401, map[string]interface{}{
			"error":   "UNAUTHORIZED",
			"message": "User not authenticated",
		})
	}
	userID := userIDRaw.(string)

	// Get user roles from middleware context
	ur := c.Get("userRoles")
	var userRoles []*business.BusinessUserRelation
	if ur != nil {
		userRoles = ur.([]*business.BusinessUserRelation)
	}

	// Get onboarding status (passing middleware-injected roles)
	status, err := h.businessService.GetOnboardingStatus(c.Request().Context(), userID, userRoles)
	if err != nil {
		h.logger.Errorf("Error fetching onboarding status: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to fetch onboarding status",
		})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Onboarding status retrieved successfully",
		"data":    status,
	})
}

// GetBusinessDetails retrieves business details for display in UI
func (h *BusinessHandler) GetBusinessDetails(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
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
			h.logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
	}

	// Get requester's role for this business
	requesterRole, err := h.getRequesterRoleForBusiness(c, businessID)
	if err != nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": err.Error(),
		})
	}

	// Get business details from service
	details, err := h.businessService.GetBusinessDetails(c.Request().Context(), businessID, requesterRole)
	if err != nil {
		h.logger.Errorf("Error fetching business details: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to fetch business details",
		})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Business details retrieved successfully",
		"data":    details,
	})
}

// InviteUser invites a user to a business (only admin/owner can invite)
func (h *BusinessHandler) InviteUser(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
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
			h.logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
	}

	// Parse request
	var req business.InviteUserRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	// Get requester's role for this business
	requesterRole, err := h.getRequesterRoleForBusiness(c, businessID)
	if err != nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": err.Error(),
		})
	}

	// Call service to invite user (service handles all authorization and validation)
	response, err := h.businessService.InviteUserToBusiness(c.Request().Context(), businessID, req.Email, req.Name, req.Mobile, req.Role, requesterRole)
	if err != nil {
		h.logger.Errorf("Error inviting user: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": err.Error(),
		})
	}

	// Send invitation email using auth service (after successful DB operations)
	// User will request OTP separately using the invite link
	go func() {
		bgCtx := context.Background()

		// Send password reset email
		if err := h.authService.SendPasswordResetEmail(bgCtx, req.Email, "invite"); err != nil {
			h.logger.Errorf("Error sending invitation email: %v", err)
			h.logger.Warnf("User %s created but invitation email failed to send", response.UserID)
		} else {
			h.logger.Infof("Invitation email sent successfully to %s", req.Email)
		}
	}()

	return c.JSON(201, map[string]interface{}{
		"message": "User invited successfully. Invitation email sent.",
		"data":    response,
	})
}

// ListBusinessUsers lists all users associated with a business (only admin/owner can access)
func (h *BusinessHandler) ListBusinessUsers(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
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
			h.logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
	}

	// Get requester's role for this business
	requesterRole, err := h.getRequesterRoleForBusiness(c, businessID)
	if err != nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": err.Error(),
		})
	}

	// Get business users (service handles authorization)
	users, err := h.businessService.GetBusinessUsers(c.Request().Context(), businessID, requesterRole)
	if err != nil {
		h.logger.Errorf("Error getting business users: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to get business users",
		})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "Business users retrieved successfully",
		"data":    users,
	})
}

// UpdateBusinessUser updates a user's role in a business (only admin/owner can update)
func (h *BusinessHandler) UpdateBusinessUser(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
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
			h.logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
	}

	// Parse request
	var req business.UpdateBusinessUserRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	// Get requester's role for this business
	requesterRole, err := h.getRequesterRoleForBusiness(c, businessID)
	if err != nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": err.Error(),
		})
	}

	// Update user role (service handles all authorization and validation)
	err = h.businessService.UpdateUserRoleInBusiness(c.Request().Context(), businessID, req.UserID, requesterRole, req.NewRole)
	if err != nil {
		h.logger.Errorf("Error updating user role: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": err.Error(),
		})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "User role updated successfully",
	})
}

// RemoveBusinessUser removes a user from a business (only admin/owner can remove)
func (h *BusinessHandler) RemoveBusinessUser(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
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
			h.logger.Errorf("Invalid business ID format: %s", businessID)
			return c.JSON(400, map[string]interface{}{
				"error":   "INVALID_BUSINESS_ID",
				"message": "X-Business-ID must be a valid UUID",
			})
		}
	}

	// Parse request
	var req business.RemoveBusinessUserRequest

	// Bind input
	if err := c.Bind(&req); err != nil {
		h.logger.Errorf("Invalid input: %v", err)
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if err := c.Validate(&req); err != nil {
		h.logger.Errorf("Validation failed: %v", err)
		if ve, ok := err.(*customValidator.ValidationError); ok {
			return c.JSON(400, map[string]interface{}{
				"error":   "VALIDATION_FAILED",
				"message": "Validation failed",
				"errors":  ve.Errors,
			})
		}
		return c.JSON(400, map[string]interface{}{
			"error":   "VALIDATION_FAILED",
			"message": "Validation failed",
		})
	}

	// Get requester's role for this business
	requesterRole, err := h.getRequesterRoleForBusiness(c, businessID)
	if err != nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": err.Error(),
		})
	}

	// Remove user from business (service handles all authorization checks and fetches target role)
	err = h.businessService.RemoveUserFromBusiness(c.Request().Context(), businessID, req.UserID, requesterRole)
	if err != nil {
		h.logger.Errorf("Error removing user from business: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": err.Error(),
		})
	}

	return c.JSON(200, map[string]interface{}{
		"message": "User removed from business successfully",
	})
}

// handleBusinessError handles typed business errors and returns appropriate HTTP responses
func (h *BusinessHandler) handleBusinessError(c echo.Context, err error) error {
	// Check if it's a BusinessError type
	if bizErr, ok := err.(*types.BusinessError); ok {
		switch bizErr.Code {
		case types.ErrCodeUserAlreadyAssociated:
			return c.JSON(403, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
			})
		case types.ErrCodeBusinessAlreadyExists:
			return c.JSON(409, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
				"details": bizErr.Details,
			})
		case types.ErrCodeUnauthorizedKYB, types.ErrCodeKYBNotInitiated:
			return c.JSON(403, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
			})
		case types.ErrCodeKYBAlreadyCompleted:
			return c.JSON(400, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
			})
		case types.ErrCodeInvalidBusinessType, types.ErrCodeInvalidInput:
			return c.JSON(400, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
				"details": bizErr.Details,
			})
		default:
			return c.JSON(500, map[string]interface{}{
				"error":   bizErr.Code,
				"message": bizErr.Message,
			})
		}
	}

	// Generic error - log the actual error for debugging
	h.logger.Errorf("Unexpected error: %v", err)
	return c.JSON(500, map[string]interface{}{
		"error":   "INTERNAL_ERROR",
		"message": "An internal server error occurred",
	})
}
