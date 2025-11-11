package http

import (
	"net/http"

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
}

// SearchUsers handles GET /users/search?business_id=biz-123&business_id=biz-456&q=john
func (h *OpsUserHandler) SearchUsers(c echo.Context) error {
	// Get business IDs from query params (can have multiple)
	businessIDs := c.QueryParams()["business_id"]
	if len(businessIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "At least one business_id is required",
		})
	}

	// Get search term
	searchTerm := c.QueryParam("q")
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
