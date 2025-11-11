package http

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/shared/logger"
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

	businessGroup := e.Group("/businesses")

	// Endpoint 1: Paginated list of businesses
	businessGroup.GET("", handler.ListBusinesses)

	// Endpoint 2: Search businesses
	businessGroup.GET("/search", handler.SearchBusinesses)

	// Endpoint 3: Get single business details
	businessGroup.GET("/:bid", handler.GetBusinessDetails)
}

// PaginationResponse represents pagination metadata
type PaginationResponse struct {
	CurrentPage int `json:"currentPage"`
	PageSize    int `json:"pageSize"`
	TotalItems  int `json:"totalItems"`
	TotalPages  int `json:"totalPages"`
}

// ListBusinessesResponse represents the response for listing businesses
type ListBusinessesResponse struct {
	Data       []*business.OpsBusinessListItem `json:"data"`
	Pagination PaginationResponse              `json:"pagination"`
}

// ListBusinesses handles GET /businesses?page=1&limit=20
func (h *OpsBusinessHandler) ListBusinesses(c echo.Context) error {
	// Parse and validate pagination parameters
	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page < 1 {
		page = 1 // Default to page 1
	}

	limit, err := strconv.Atoi(c.QueryParam("limit"))
	if err != nil || limit < 1 {
		limit = 20 // Default page size
	}

	// Validate max limit
	if limit > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Page size cannot exceed 100",
		})
	}

	// Call service layer
	businesses, totalCount, err := h.businessService.ListBusinessesPaginated(c.Request().Context(), page, limit)
	if err != nil {
		h.logger.Errorf("Error listing businesses: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch businesses",
		})
	}

	// Calculate total pages
	totalPages := (totalCount + limit - 1) / limit

	// Build response
	response := ListBusinessesResponse{
		Data: businesses,
		Pagination: PaginationResponse{
			CurrentPage: page,
			PageSize:    limit,
			TotalItems:  totalCount,
			TotalPages:  totalPages,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// SearchBusinesses handles GET /businesses/search?q=term
func (h *OpsBusinessHandler) SearchBusinesses(c echo.Context) error {
	// Parse and validate search query
	searchTerm := c.QueryParam("q")
	if searchTerm == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Search query parameter 'q' is required",
		})
	}

	// Validate minimum search length
	if len(searchTerm) < 2 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Search term must be at least 2 characters",
		})
	}

	// Call service layer
	results, err := h.businessService.SearchBusinesses(c.Request().Context(), searchTerm)
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
	// Parse and validate business ID
	bid := c.Param("bid")
	if bid == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Business ID is required",
		})
	}

	// Call service layer
	details, err := h.businessService.GetBusinessDetailsForOps(c.Request().Context(), bid)
	if err != nil {
		h.logger.Errorf("Error fetching business details for BID %s: %v", bid, err)
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
