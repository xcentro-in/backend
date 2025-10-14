package business

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/middleware"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
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

func RegisterBusinessRoutes(e *echo.Group, businessService business.BusinessService, authService auth.AuthService, userService user.UserService, s3Config *config.S3Config, businessUserRelRepo business.BusinessUserRelationRepository, rclient *redisClient.RedisClient) {
	handler := NewBusinessHandler(businessService, authService, userService, s3Config)

	businessGroup := e.Group("/business")
	// Apply middleware to the business group
	businessGroup.Use(middleware.JWTMiddleware(authService))
	businessGroup.Use(middleware.BusinessRolesMiddleware(businessUserRelRepo, rclient))

	// Business details route
	businessGroup.GET("", handler.GetBusinessDetails)

	// Onboarding routes
	onboardingGroup := businessGroup.Group("/onboard")
	onboardingGroup.POST("/init", handler.InitiateOnboarding)
	onboardingGroup.POST("/complete", handler.CompleteOnboarding)
	onboardingGroup.GET("/status", handler.GetOnboardingStatus)
	onboardingGroup.GET("/upload", handler.UploadKYBDocuments)
	onboardingGroup.POST("/confirm-upload", handler.ConfirmKYBDocumentsUpload)

	// Invite user route
	businessGroup.POST("/invite-user", handler.InviteUser)

	// User management routes
	businessGroup.GET("/users", handler.ListBusinessUsers)
	businessGroup.POST("/users/update", handler.UpdateBusinessUser)
	businessGroup.DELETE("/users", handler.RemoveBusinessUser)
}

func (h *BusinessHandler) UploadKYBDocuments(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	if !(len(roles) == 1 && roles[0].KybInitiated) {
		return c.JSON(403, map[string]interface{}{"message": "kyb completed or unauthorized", "data": nil})
	}

	fileuploader, err := fileupload.NewS3ClientFromConfig(*h.S3Config)
	if err != nil {
		h.logger.Errorf("Error creating S3 client: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create S3 client"})
	}

	key := fmt.Sprintf("PII/%s/%s", roles[0].BID, "business_deed")
	// right now i think file name and content type are redundant as we are using presigned urls
	// will remove them later
	url, _, err := fileuploader.GetPresignedUploadURL(c.Request().Context(), "test.pdf", "application/pdf", key, 15*time.Minute)
	if err != nil {
		h.logger.Errorf("Error getting presigned upload URL: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to get presigned upload URL"})
	}

	return c.JSON(200, map[string]interface{}{"message": "Fetched presigned upload URL successfully", "data": map[string]interface{}{"url": url}})
}

func (h *BusinessHandler) ConfirmKYBDocumentsUpload(c echo.Context) error {
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{"message": "Create a business first", "data": nil})
	}
	roles := ur.([]*business.BusinessUserRelation)

	if !(len(roles) == 1 && roles[0].KybInitiated) {
		return c.JSON(403, map[string]interface{}{"message": "kyb completed or unauthorized", "data": nil})
	}

	fileuploader, err := fileupload.NewS3ClientFromConfig(*h.S3Config)
	if err != nil {
		h.logger.Errorf("Error creating S3 client: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to create S3 client"})
	}

	key := fmt.Sprintf("PII/%s/%s", roles[0].BID, "business_deed.pdf")
	fileurl, err := fileuploader.ConfirmUpload(c.Request().Context(), key, 15*time.Minute)
	if err != nil {
		h.logger.Errorf("Error confirming upload: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to confirm upload"})
	}

	err = h.businessService.UpdateBusinessDeedDocumentKey(c.Request().Context(), roles[0].BID, key)
	if err != nil {
		h.logger.Errorf("Error updating business deed document key: %v", err)
		return c.JSON(500, map[string]string{"message": "Failed to update business deed document key"})
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
	type InitRequest struct {
		Name  string `json:"name"`
		GSTIN string `json:"gstin"`
	}

	var req InitRequest
	if err := c.Bind(&req); err != nil {
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
	if err := c.Bind(&req); err != nil {
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

	// Get businessID from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	req.BID = businessID

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
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
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
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for this specific business
	var userRole shared.UserRole
	var hasAccess bool
	for _, rel := range roles {
		if rel.BID == businessID {
			userRole = rel.Role
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "You don't have access to this business",
		})
	}

	// Get business details from service (using middleware-injected role)
	details, err := h.businessService.GetBusinessDetails(c.Request().Context(), businessID, userRole)
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
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Parse request
	type InviteRequest struct {
		Email     string          `json:"email"`
		Name      string          `json:"name"`
		Role      shared.UserRole `json:"role"`
		KycNeeded bool            `json:"kycNeeded"`
	}

	var req InviteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if req.Email == "" || req.Name == "" || req.Role == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Email, name, and role are required",
		})
	}

	// Validate role is valid
	validRoles := []shared.UserRole{shared.Owner, shared.Admin, shared.Finance, shared.Support}
	isValidRole := false
	for _, validRole := range validRoles {
		if req.Role == validRole {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid role. Must be one of: owner, admin, finance, support",
		})
	}

	// Get user ID from context (for authentication check)
	userIDRaw := c.Get("userId")
	if userIDRaw == nil {
		return c.JSON(401, map[string]interface{}{
			"error":   "UNAUTHORIZED",
			"message": "User not authenticated",
		})
	}

	// Get user roles
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for this specific business
	var userRole shared.UserRole
	var hasAccess bool
	for _, rel := range roles {
		if rel.BID == businessID {
			userRole = rel.Role
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "You don't have access to this business",
		})
	}

	// Only admin or owner can invite users
	if userRole != shared.Owner && userRole != shared.Admin {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "Only business owners and admins can invite users",
		})
	}

	// Admin cannot invite another owner (only owner can invite owner)
	if userRole == shared.Admin && req.Role == shared.Owner {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "Only business owners can invite other owners",
		})
	}

	// Create invited user
	createdUser, err := h.userService.CreateInvitedUser(c.Request().Context(), req.Email, req.Name, req.KycNeeded)
	if err != nil {
		h.logger.Errorf("Error creating invited user: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": fmt.Sprintf("Failed to create invited user: %v", err),
		})
	}

	// Add user to business with specified role
	relation := &business.BusinessUserRelation{
		UserID:       createdUser.Id,
		BID:          businessID,
		Role:         req.Role,
		RoleClaim:    req.Role,
		KybInitiated: false,
	}

	err = h.businessService.AddUserToBusiness(c.Request().Context(), relation)
	if err != nil {
		h.logger.Errorf("Error adding user to business: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to add user to business",
		})
	}

	return c.JSON(201, map[string]interface{}{
		"message": "User invited successfully. Invitation email sent.",
		"data": map[string]interface{}{
			"userId": createdUser.Id,
			"email":  createdUser.Email,
			"name":   createdUser.Name,
		},
	})
}

// ListBusinessUsers lists all users associated with a business (only admin/owner can access)
func (h *BusinessHandler) ListBusinessUsers(c echo.Context) error {
	// Read BusinessID from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Get user roles from context
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for this specific business
	var userRole shared.UserRole
	var hasAccess bool
	for _, rel := range roles {
		if rel.BID == businessID {
			userRole = rel.Role
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "You don't have access to this business",
		})
	}

	// Only admin or owner can list users
	if userRole != shared.Owner && userRole != shared.Admin {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "Only business owners and admins can list users",
		})
	}

	// Get business users
	users, err := h.businessService.GetBusinessUsers(c.Request().Context(), businessID)
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
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Parse request
	type UpdateUserRequest struct {
		UserID  string          `json:"userId"`
		NewRole shared.UserRole `json:"newRole"`
	}

	var req UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if req.UserID == "" || req.NewRole == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "userId and newRole are required",
		})
	}

	// Validate role is valid
	validRoles := []shared.UserRole{shared.Owner, shared.Admin, shared.Finance, shared.Support}
	isValidRole := false
	for _, validRole := range validRoles {
		if req.NewRole == validRole {
			isValidRole = true
			break
		}
	}
	if !isValidRole {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid role. Must be one of: owner, admin, finance, support",
		})
	}

	// Get user roles from context
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for this specific business
	var userRole shared.UserRole
	var hasAccess bool
	for _, rel := range roles {
		if rel.BID == businessID {
			userRole = rel.Role
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "You don't have access to this business",
		})
	}

	// Only admin or owner can update users
	if userRole != shared.Owner && userRole != shared.Admin {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "Only business owners and admins can update user roles",
		})
	}

	// Update user role (service handles additional authorization checks)
	err := h.businessService.UpdateUserRoleInBusiness(c.Request().Context(), businessID, req.UserID, userRole, req.NewRole)
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
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Parse request
	type RemoveUserRequest struct {
		UserID string `json:"userId"`
	}

	var req RemoveUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "Invalid request payload",
		})
	}

	// Validate input
	if req.UserID == "" {
		return c.JSON(400, map[string]interface{}{
			"error":   "INVALID_INPUT",
			"message": "userId is required",
		})
	}

	// Get user roles from context
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for this specific business and get target user's role
	var requesterRole shared.UserRole
	var hasAccess bool
	for _, rel := range roles {
		if rel.BID == businessID {
			requesterRole = rel.Role
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "You don't have access to this business",
		})
	}

	// Only admin or owner can remove users
	if requesterRole != shared.Owner && requesterRole != shared.Admin {
		return c.JSON(403, map[string]interface{}{
			"error":   "FORBIDDEN",
			"message": "Only business owners and admins can remove users",
		})
	}

	// Get target user's role
	businessUsers, err := h.businessService.GetBusinessUsers(c.Request().Context(), businessID)
	if err != nil {
		h.logger.Errorf("Error getting business users: %v", err)
		return c.JSON(500, map[string]interface{}{
			"error":   "INTERNAL_ERROR",
			"message": "Failed to get business users",
		})
	}

	var targetUserRole shared.UserRole
	found := false
	for _, u := range businessUsers {
		if u.UserID == req.UserID {
			targetUserRole = u.Role
			found = true
			break
		}
	}

	if !found {
		return c.JSON(404, map[string]interface{}{
			"error":   "NOT_FOUND",
			"message": "User not found in business",
		})
	}

	// Remove user from business (service handles additional authorization checks)
	err = h.businessService.RemoveUserFromBusiness(c.Request().Context(), businessID, req.UserID, targetUserRole, requesterRole)
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

	// Generic error
	return c.JSON(500, map[string]interface{}{
		"error":   "INTERNAL_ERROR",
		"message": err.Error(),
	})
}
