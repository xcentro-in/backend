package payment

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/services/payments"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/validator"
)

// OpsCollectionHandler handles ops dashboard collection-related endpoints
type OpsCollectionHandler struct {
	collectionService *payments.OpsCollectionService
	logger            *logger.Logger
}

// NewOpsCollectionHandler creates a new ops collection handler instance
func NewOpsCollectionHandler(collectionService *payments.OpsCollectionService) *OpsCollectionHandler {
	return &OpsCollectionHandler{
		collectionService: collectionService,
		logger:            logger.GetDefault(),
	}
}

// RegisterOpsCollectionRoutes registers all ops dashboard collection routes
func RegisterOpsCollectionRoutes(e *echo.Group, collectionService *payments.OpsCollectionService) {
	handler := NewOpsCollectionHandler(collectionService)

	collectionGroup := e.Group("/collections")

	// Endpoint: Paginated list of collections
	collectionGroup.GET("", handler.ListCollections)
	// Endpoint: Get collection details by payment ID
	collectionGroup.GET("/:payment_id", handler.GetCollectionDetails)
}

// ListCollections handles GET /collections?page=1&limit=20&status=pending|payoutReady|failed
func (h *OpsCollectionHandler) ListCollections(c echo.Context) error {
	var req payment.OpsListCollectionsRequest

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
	response, err := h.collectionService.ListCollections(c.Request().Context(), req.Page, req.Limit, string(req.Status))
	if err != nil {
		h.logger.Errorf("Error listing collections: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch collections",
		})
	}

	return c.JSON(http.StatusOK, response)
}

// GetCollectionDetails handles GET /collections/:payment_id
func (h *OpsCollectionHandler) GetCollectionDetails(c echo.Context) error {
	paymentID := c.Param("payment_id")
	if paymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "payment_id is required",
		})
	}

	// Call service layer
	details, err := h.collectionService.GetCollectionDetails(c.Request().Context(), paymentID)
	if err != nil {
		h.logger.Errorf("Error getting collection details: %v", err)
		if err.Error() == "payment not found" {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "Collection not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch collection details",
		})
	}

	return c.JSON(http.StatusOK, details)
}
