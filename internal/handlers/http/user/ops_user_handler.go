package user

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/user"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/validator"
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

	userGroup := e.Group("/user")

	// User search endpoint
	userGroup.GET("/search", handler.SearchUsers)

	// User details endpoint
	userGroup.GET("/:user_id", handler.GetUserDetails)

	// Ops verification endpoints
	userGroup.POST("/:user_id/reload-pan", handler.ReloadUserPANData)
	userGroup.POST("/:user_id/kyc-action", handler.KYCAction)
}

// SearchUsers handles GET /users/search?business_id=biz-123&business_id=biz-456&q=john
func (h *OpsUserHandler) SearchUsers(c echo.Context) error {
	var req user.OpsUserSearchRequest

	// Bind query parameters
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request parameters",
		})
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Call service layer
	results, err := h.userService.SearchUsersByBusinesses(c.Request().Context(), req.BusinessIDs, req.SearchTerm)
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
	var req user.OpsUserDetailsRequest

	// Bind path and query parameters
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request parameters",
		})
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Call service layer
	details, err := h.userService.GetUserDetailsForOps(c.Request().Context(), req.UserID, req.BusinessID)
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
	var req user.OpsReloadUserPANRequest

	// Bind path parameters
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid path parameters",
		})
	}

	// Bind query parameters
	if err := (&echo.DefaultBinder{}).BindQueryParams(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid query parameters",
		})
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Call service layer
	if err := h.userService.ReloadUserPANData(c.Request().Context(), req.UserID, req.DataType); err != nil {
		h.logger.Errorf("Error reloading user PAN data for user %s, type %s: %v", req.UserID, req.DataType, err)
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

// KYCAction handles POST /users/:user_id/kyc-action?action=reject
func (h *OpsUserHandler) KYCAction(c echo.Context) error {
	var req user.OpsKYCActionRequest

	// Bind path parameters
	if err := (&echo.DefaultBinder{}).BindPathParams(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid path parameters",
		})
	}

	// Bind query parameters
	if err := (&echo.DefaultBinder{}).BindQueryParams(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid query parameters",
		})
	}

	// Bind JSON body (notes field)
	if err := (&echo.DefaultBinder{}).BindBody(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Trim and validate
	validator.TrimStrings(&req)
	if err := c.Validate(&req); err != nil {
		if ve, ok := err.(*validator.ValidationError); ok {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error":  "Validation failed",
				"errors": ve.Errors,
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}

	// Call service layer based on action
	var err error
	var successMessage string

	switch req.Action {
	case "reject":
		err = h.userService.RejectKYC(c.Request().Context(), req.UserID, req.Notes)
		successMessage = "KYC rejected successfully"
	}

	// Handle service errors
	if err != nil {
		h.logger.Errorf("Error performing KYC action '%s' for user %s: %v", req.Action, req.UserID, err)
		if err.Error() == "user not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "User not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to perform KYC action",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": successMessage,
	})
}
