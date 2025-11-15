package business

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/shared"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/validator"
)

// OpsBusinessHandler handles ops dashboard business-related endpoints
type OpsBusinessHandler struct {
	businessService business.BusinessService
	authService     auth.AuthService
	logger          *logger.Logger
}

// NewOpsBusinessHandler creates a new ops business handler instance
func NewOpsBusinessHandler(businessService business.BusinessService, authService auth.AuthService) *OpsBusinessHandler {
	return &OpsBusinessHandler{
		businessService: businessService,
		authService:     authService,
		logger:          logger.GetDefault(),
	}
}

// RegisterOpsBusinessRoutes registers all ops dashboard business routes
func RegisterOpsBusinessRoutes(e *echo.Group, businessService business.BusinessService, authService auth.AuthService) {
	handler := NewOpsBusinessHandler(businessService, authService)

	businessGroup := e.Group("/business")

	// Endpoint 1: Paginated list of businesses
	businessGroup.GET("", handler.ListBusinesses)

	// Endpoint 2: Search businesses
	businessGroup.GET("/search", handler.SearchBusinesses)

	// Endpoint 3: Get single business details
	businessGroup.GET("/:bid", handler.GetBusinessDetails)

	// Ops verification endpoints
	businessGroup.POST("/:bid/reload-data", handler.ReloadBusinessData)
	businessGroup.POST("/:bid/kyb-action", handler.KYBAction)
	businessGroup.POST("/:bid/user-action", handler.UpdateUserRole)
}

// ListBusinesses handles GET /businesses?page=1&limit=20
func (h *OpsBusinessHandler) ListBusinesses(c echo.Context) error {
	var req business.OpsListBusinessesRequest

	// Bind query parameters
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request parameters",
		})
	}

	// Set defaults
	if req.Page == 0 {
		req.Page = 1
	}
	if req.Limit == 0 {
		req.Limit = 20
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
	businesses, totalCount, err := h.businessService.ListBusinessesPaginated(c.Request().Context(), req.Page, req.Limit, req.KYBStatus)
	if err != nil {
		h.logger.Errorf("Error listing businesses: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch businesses",
		})
	}

	// Calculate total pages
	totalPages := (totalCount + req.Limit - 1) / req.Limit

	// Build response
	response := business.OpsListBusinessesResponse{
		Data: businesses,
		Pagination: business.PaginationResponse{
			CurrentPage: req.Page,
			PageSize:    req.Limit,
			TotalItems:  totalCount,
			TotalPages:  totalPages,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// SearchBusinesses handles GET /businesses/search?q=term
func (h *OpsBusinessHandler) SearchBusinesses(c echo.Context) error {
	var req business.OpsSearchBusinessesRequest

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
	results, err := h.businessService.SearchBusinesses(c.Request().Context(), req.SearchTerm, req.KYBStatus)
	if err != nil {
		h.logger.Errorf("Error searching businesses: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to search businesses",
		})
	}

	// Return results (empty array if no matches)
	if results == nil {
		results = []*business.OpsBusinessSearchResult{}
	}

	return c.JSON(http.StatusOK, results)
}

// GetBusinessDetails handles GET /businesses/:bid
func (h *OpsBusinessHandler) GetBusinessDetails(c echo.Context) error {
	var req business.OpsGetBusinessDetailsRequest

	// Bind path parameter
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
	details, err := h.businessService.GetBusinessDetailsForOps(c.Request().Context(), req.BusinessID)
	if err != nil {
		h.logger.Errorf("Error fetching business details for BID %s: %v", req.BusinessID, err)
		if err.Error() == "business not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Business not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch business details",
		})
	}

	return c.JSON(http.StatusOK, details)
}

// ReloadBusinessData handles POST /businesses/:bid/reload-data?type=gstin|income_tax_id|company_id
func (h *OpsBusinessHandler) ReloadBusinessData(c echo.Context) error {
	var req business.OpsReloadBusinessDataRequest

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
	if err := h.businessService.ReloadBusinessData(c.Request().Context(), req.BusinessID, req.DataType); err != nil {
		h.logger.Errorf("Error reloading business data for BID %s, type %s: %v", req.BusinessID, req.DataType, err)
		if err.Error() == "business not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Business not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to reload business data",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Business data reloaded successfully",
	})
}

// KYBAction handles POST /businesses/:bid/kyb-action?action=reject|verify
func (h *OpsBusinessHandler) KYBAction(c echo.Context) error {
	var req business.OpsKYBActionRequest

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

	// Call service layer based on action
	var err error
	var successMessage string

	switch req.Action {
	case "reject":
		err = h.businessService.RejectKYB(c.Request().Context(), req.BusinessID)
		successMessage = "KYB rejected successfully"
	case "verify":
		err = h.businessService.VerifyKYB(c.Request().Context(), req.BusinessID)
		successMessage = "KYB verified successfully"
	}

	// Handle service errors
	if err != nil {
		h.logger.Errorf("Error performing KYB action '%s' for BID %s: %v", req.Action, req.BusinessID, err)
		if err.Error() == "business not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Business not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to perform KYB action",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": successMessage,
	})
}

// UpdateUserRole handles POST /businesses/:bid/user-action?user_id=xxx&action=set_owner|set_admin|set_manager|set_auditor
func (h *OpsBusinessHandler) UpdateUserRole(c echo.Context) error {
	var req business.OpsUpdateUserRoleRequest

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

	// Map action to role
	var role shared.UserRole
	switch req.Action {
	case "set_owner":
		role = shared.Owner
	case "set_admin":
		role = shared.Admin
	case "set_manager":
		role = shared.Manager
	case "set_auditor":
		role = shared.Auditor
	}

	// Call service layer
	err := h.businessService.OpsUpdateUserRole(c.Request().Context(), req.BusinessID, req.UserID, role)
	if err != nil {
		h.logger.Errorf("Error updating user role for user %s in business %s: %v", req.UserID, req.BusinessID, err)
		if err.Error() == "business-user relation not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "User not associated with business",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user role",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "User role updated successfully",
	})
}
