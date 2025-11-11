package http

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
)

// OpsUserHandler handles ops dashboard user-related endpoints
type OpsUserHandler struct {
	userService user.UserService
	authService auth.AuthService
	logger      *logger.Logger
}

// NewOpsUserHandler creates a new ops user handler instance
func NewOpsUserHandler(userService user.UserService, authService auth.AuthService) *OpsUserHandler {
	return &OpsUserHandler{
		userService: userService,
		authService: authService,
		logger:      logger.GetDefault(),
	}
}

// RegisterOpsUserRoutes registers all ops dashboard user routes
func RegisterOpsUserRoutes(e *echo.Group, userService user.UserService, authService auth.AuthService) {
	handler := NewOpsUserHandler(userService, authService)

	userGroup := e.Group("/users")

	// User search endpoint
	userGroup.GET("/search", handler.SearchUsers)

	// User details endpoint
	userGroup.GET("/:user_id", handler.GetUserDetails)

	// Ops verification endpoints
	userGroup.POST("/:user_id/reload-pan", handler.ReloadUserPANData)
	userGroup.POST("/:user_id/reject-kyc", handler.RejectKYC)
}

// SearchUsers handles GET /users/search?business_id=biz-123&business_id=biz-456&q=john
func (h *OpsUserHandler) SearchUsers(c echo.Context) error {
	// Get business IDs from query params (can have multiple)
	rawBusinessIDs := c.QueryParams()["business_id"]
	if len(rawBusinessIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "At least one business_id is required",
		})
	}

	// Validate number of business IDs (max 20 to prevent abuse)
	if len(rawBusinessIDs) > 20 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Maximum 20 business IDs allowed per request",
		})
	}

	// Trim and validate business IDs
	businessIDs := make([]string, 0, len(rawBusinessIDs))
	for _, bid := range rawBusinessIDs {
		trimmedBid := strings.TrimSpace(bid)
		if trimmedBid == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Business ID cannot be empty",
			})
		}
		businessIDs = append(businessIDs, trimmedBid)
	}

	// Get and trim search term
	searchTerm := strings.TrimSpace(c.QueryParam("q"))
	if searchTerm == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Search term 'q' is required",
		})
	}

	// Validate search term length
	if len(searchTerm) < 2 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Search term must be at least 2 characters",
		})
	}

	// Validate search term length (max 100 chars to prevent abuse)
	if len(searchTerm) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Search term must not exceed 100 characters",
		})
	}

	// Call service layer
	results, err := h.userService.SearchUsersByBusinesses(c.Request().Context(), businessIDs, searchTerm)
	if err != nil {
		h.logger.Errorf("Error searching users: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to search users",
		})
	}

	// Return results (empty array if no results)
	if results == nil {
		results = []*user.OpsUserSearchResult{}
	}

	return c.JSON(http.StatusOK, results)
}

// GetUserDetails handles GET /users/:user_id?bid=biz-123
func (h *OpsUserHandler) GetUserDetails(c echo.Context) error {
	// Get and trim user ID from path parameter
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
		})
	}

	// Get and trim optional business ID from query parameter
	businessID := strings.TrimSpace(c.QueryParam("bid"))

	// Call service layer
	details, err := h.userService.GetUserDetailsForOps(c.Request().Context(), userID, businessID)
	if err != nil {
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "User not found",
			})
		}
		h.logger.Errorf("Error getting user details: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get user details",
		})
	}

	return c.JSON(http.StatusOK, details)
}

// ReloadUserPANData handles POST /users/:user_id/reload-pan?type=pan_com|pan_aadhaar_link
func (h *OpsUserHandler) ReloadUserPANData(c echo.Context) error {
	// Parse and validate user ID
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
		})
	}

	// Parse and validate data type
	dataType := strings.TrimSpace(c.QueryParam("type"))
	if dataType == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Query parameter 'type' is required",
		})
	}

	// Validate allowed types
	allowedTypes := map[string]bool{
		"pan_com":          true,
		"pan_aadhaar_link": true,
	}
	if !allowedTypes[dataType] {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid type. Allowed values: pan_com, pan_aadhaar_link",
		})
	}

	// Call service layer
	if err := h.userService.ReloadUserPANData(c.Request().Context(), userID, dataType); err != nil {
		h.logger.Errorf("Error reloading user PAN data for user %s, type %s: %v", userID, dataType, err)
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to reload user PAN data",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "User PAN data reloaded successfully",
	})
}

// RejectKYC handles POST /users/:user_id/reject-kyc
func (h *OpsUserHandler) RejectKYC(c echo.Context) error {
	// Parse and validate user ID
	userID := strings.TrimSpace(c.Param("user_id"))
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
		})
	}

	// Call service layer
	if err := h.userService.RejectKYC(c.Request().Context(), userID); err != nil {
		h.logger.Errorf("Error rejecting KYC for user %s: %v", userID, err)
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to reject KYC",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "KYC rejected successfully",
	})
}
