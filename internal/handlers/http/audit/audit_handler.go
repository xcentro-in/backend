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

// GetPaymentAuditTrail handles GET /v1/audit/payment/:payment_id
func (h *AuditHandler) GetPaymentAuditTrail(c echo.Context) error {
	paymentID := c.Param("payment_id")
	if paymentID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_PAYMENT_ID",
			"message": "Payment ID is required",
		})
	}

	// Get business ID from header or context
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Get audit trail
	trail, err := h.auditService.GetTrailByPaymentID(c.Request().Context(), businessID, paymentID)
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
			"payment_id":  paymentID,
			"entries":     trail,
		},
	})
}
