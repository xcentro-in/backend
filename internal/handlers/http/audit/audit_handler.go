package audit

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/services/audit"
)

// AuditHandler handles audit trail HTTP requests
type AuditHandler struct {
	auditService *audit.AuditService
}

// NewAuditHandler creates a new audit handler
func NewAuditHandler(auditService *audit.AuditService) *AuditHandler {
	return &AuditHandler{
		auditService: auditService,
	}
}

// GetBusinessAuditTrail handles GET /v1/audit/business/:business_id
func (h *AuditHandler) GetBusinessAuditTrail(c echo.Context) error {
	businessID := c.Param("business_id")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "Business ID is required",
		})
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	// Get audit trail
	trail, err := h.auditService.GetTrailByBusinessID(c.Request().Context(), businessID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch audit trail",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Audit trail retrieved successfully",
		"data": map[string]any{
			"business_id": businessID,
			"entries":     trail,
			"limit":       limit,
			"offset":      offset,
		},
	})
}

// GetEntityAuditTrail handles GET /v1/audit/:entity_type/:entity_id
// Supports: payment, voucher, payee, user, wallet, payout
func (h *AuditHandler) GetEntityAuditTrail(c echo.Context) error {
	entityType := c.Param("entity_type")
	entityID := c.Param("entity_id")

	if entityType == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_ENTITY_TYPE",
			"message": "Entity type is required (payment, voucher, payee, user, wallet, payout)",
		})
	}

	if entityID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_ENTITY_ID",
			"message": "Entity ID is required",
		})
	}

	// Parse pagination parameters
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50 // Default limit
	}

	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if offset < 0 {
		offset = 0
	}

	// Get audit trail for the entity
	trail, err := h.auditService.GetTrailByEntityID(c.Request().Context(), entityType, entityID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "FETCH_FAILED",
			"message": "Failed to fetch audit trail",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Audit trail retrieved successfully",
		"data": map[string]any{
			"entity_type": entityType,
			"entity_id":   entityID,
			"entries":     trail,
			"limit":       limit,
			"offset":      offset,
		},
	})
}
