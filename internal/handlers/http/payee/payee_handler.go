package payee

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payee"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/shared/logger"
)

// PayeeHandler handles payee-related HTTP requests
type PayeeHandler struct {
	payeeService *payeeSvc.PayeeService
	logger       *logger.Logger
}

// NewPayeeHandler creates a new payee handler
func NewPayeeHandler(payeeService *payeeSvc.PayeeService, logger *logger.Logger) *PayeeHandler {
	return &PayeeHandler{
		payeeService: payeeService,
		logger:       logger,
	}
}

// OnboardPayee handles POST /payees - Onboard a new payee (polymorphic)
func (h *PayeeHandler) OnboardPayee(c echo.Context) error {
	h.logger.InfoWith("OnboardPayee API called")

	// Parse polymorphic request body
	var req payee.OnboardPayeeRequest
	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Extract userId from JWT context (set by JWTMiddleware)
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		h.logger.WarnWith("Missing userId in context")
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}
	h.logger.DebugWith("User authenticated", "user_id", userID)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	req.BusinessID = businessID
	req.AddedBy = userID // Set the user who is adding this payee
	h.logger.DebugWith("Business context set", "business_id", businessID)

	// Default to vendor if not specified (backward compatibility)
	if req.PayeeType == "" {
		req.PayeeType = payee.PayeeTypeVendor
	}

	// Validate type-specific data is provided
	switch req.PayeeType {
	case payee.PayeeTypeVendor:
		if req.Vendor == nil {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "invalid_request",
				"message": "vendor data is required for payee_type=vendor",
			})
		}
		h.logger.DebugWith("Vendor onboard request", "gstin", req.Vendor.GSTIN, "display_name", req.DisplayName)
	case payee.PayeeTypeEmployee:
		if req.Employee == nil {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "invalid_request",
				"message": "employee data is required for payee_type=employee",
			})
		}
		h.logger.DebugWith("Employee onboard request", "pan", req.Employee.PAN, "display_name", req.DisplayName)
	case payee.PayeeTypeBiller:
		if req.Biller == nil {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "invalid_request",
				"message": "biller data is required for payee_type=biller",
			})
		}
		h.logger.DebugWith("Biller onboard request", "biller_id", req.Biller.BillerID, "display_name", req.DisplayName)
	case payee.PayeeTypeGST:
		if req.GST == nil {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "invalid_request",
				"message": "gst data is required for payee_type=gst_payment",
			})
		}
		h.logger.DebugWith("GST payment onboard request", "tax_period", req.GST.TaxPeriod, "display_name", req.DisplayName)
	default:
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_payee_type",
			"message": fmt.Sprintf("unsupported payee_type: %s", req.PayeeType),
		})
	}

	// Call payee service to onboard payee
	h.logger.DebugWith("Calling payee service")
	resp, err := h.payeeService.OnboardPayee(c.Request().Context(), req)
	if err != nil {
		h.logger.ErrorWith("Onboarding failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "onboarding_failed",
			"message": err.Error(),
		})
	}

	// Convert to response
	response := map[string]any{
		"relation_id":               resp.RelationID,
		"payee_id":                  resp.PayeeID,
		"account_id":                resp.AccountID,
		"gst_verification_status":   string(resp.GSTVerificationStatus),
		"bank_verification_status":  string(resp.BankVerificationStatus),
		"cross_verification_status": string(resp.CrossVerificationStatus),
		"is_fully_verified":         resp.IsFullyVerified,
		"message":                   resp.Message,
	}

	h.logger.InfoWith("Payee onboarded successfully", "payee_id", resp.PayeeID, "account_id", resp.AccountID)

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Payee onboarded successfully",
		"status":  "success",
		"data":    response,
	})
}

// GetPayees handles GET /payees - List all payees for a business
func (h *PayeeHandler) GetPayees(c echo.Context) error {
	h.logger.InfoWith("GetPayees API called")

	// Parse query parameters
	displayName := c.QueryParam("display_name")
	isActiveStr := c.QueryParam("is_active")
	isFavoriteStr := c.QueryParam("is_favorite")
	pageStr := c.QueryParam("page")
	pageSizeStr := c.QueryParam("page_size")

	// Parse boolean filters
	var isActive, isFavorite *bool
	if isActiveStr != "" {
		val := isActiveStr == "true"
		isActive = &val
	}
	if isFavoriteStr != "" {
		val := isFavoriteStr == "true"
		isFavorite = &val
	}

	// Parse pagination
	page := 1
	pageSize := 10
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	h.logger.DebugWith("Query parameters parsed",
		"display_name", displayName,
		"is_active", isActive,
		"is_favorite", isFavorite,
		"page", page,
		"page_size", pageSize)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	h.logger.DebugWith("Business context", "business_id", businessID)

	// Convert to service filters
	filters := payee.RelationSearchFilters{
		DisplayName: displayName,
		IsActive:    isActive,
		IsFavorite:  isFavorite,
		Tags:        []string{}, // TODO: Parse tags from query if needed
		Limit:       pageSize,
		Offset:      (page - 1) * pageSize,
	}

	// Call payee service
	h.logger.DebugWith("Calling payee service")
	relations, err := h.payeeService.GetPayeesByBusiness(c.Request().Context(), businessID, filters)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payees", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	// Convert to response (simplified - returning relations directly)
	// In production, you might want to enrich with payee and account details
	h.logger.InfoWith("Payees fetched successfully", "count", len(relations), "business_id", businessID)

	return c.JSON(http.StatusOK, map[string]any{
		"message":     "Payees retrieved successfully",
		"status":      "success",
		"total_count": len(relations),
		"page":        page,
		"page_size":   pageSize,
		"data":        relations,
	})
}

// GetPayeeDetails handles GET /payees/details/:relation_id - Get detailed payee information by relation_id
// Returns polymorphic response (vendor/employee/biller) with complete details including bank info
func (h *PayeeHandler) GetPayeeDetails(c echo.Context) error {
	h.logger.InfoWith("GetPayeeDetails API called")

	// Extract relation_id from path parameter
	relationID := c.Param("relation_id")
	if relationID == "" {
		h.logger.WarnWith("Missing relation_id path parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id path parameter is required",
		})
	}

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Request parameters", "relation_id", relationID, "business_id", businessID)

	// Call payee service with business_id validation
	h.logger.DebugWith("Calling payee service")
	details, err := h.payeeService.GetPayeeDetailsByRelation(c.Request().Context(), businessID, relationID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payee details", "error", err)
		return c.JSON(http.StatusNotFound, map[string]any{
			"error":   "not_found",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payee details fetched", "display_name", details.DisplayName, "payee_type", details.PayeeType)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   details,
	})
}

// UpdatePayee handles PUT /payees/:payee_id/accounts/:account_id - Update payee relationship metadata
func (h *PayeeHandler) UpdatePayee(c echo.Context) error {
	h.logger.InfoWith("UpdatePayee API called")

	// Extract path parameters
	payeeID := c.Param("payee_id")
	accountID := c.Param("account_id")

	h.logger.DebugWith("Path parameters", "payee_id", payeeID, "account_id", accountID)

	// Parse request body
	var req struct {
		DisplayName  string   `json:"display_name"`
		ContactEmail string   `json:"contact_email"`
		ContactPhone string   `json:"contact_phone"`
		Notes        string   `json:"notes"`
		IsFavorite   bool     `json:"is_favorite"`
		Tags         []string `json:"tags"`
	}

	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	h.logger.DebugWith("Request body parsed", "display_name", req.DisplayName, "is_favorite", req.IsFavorite)

	// Extract business_id from JWT
	// TODO: Extract from JWT token claims
	businessID := "bid12345"
	h.logger.DebugWith("Business context", "business_id", businessID)

	// Convert to service request
	updateReq := payee.UpdateRelationRequest{
		BusinessID:   businessID,
		PayeeID:      payeeID,
		AccountID:    accountID,
		DisplayName:  req.DisplayName,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Notes:        req.Notes,
		IsFavorite:   req.IsFavorite,
		Tags:         req.Tags,
	}

	// Call payee service
	h.logger.DebugWith("Calling payee service")
	if err := h.payeeService.UpdatePayeeRelation(c.Request().Context(), updateReq); err != nil {
		h.logger.ErrorWith("Update failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "update_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payee updated successfully", "payee_id", payeeID)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Payee updated successfully",
		"status":  "success",
	})
}

// DeactivatePayee handles DELETE /payees/:payee_id/accounts/:account_id - Deactivate a payee (soft delete)
func (h *PayeeHandler) DeactivatePayee(c echo.Context) error {
	h.logger.InfoWith("DeactivatePayee API called")

	// Extract path parameters
	payeeID := c.Param("payee_id")
	accountID := c.Param("account_id")

	h.logger.DebugWith("Path parameters", "payee_id", payeeID, "account_id", accountID)

	// Extract business_id from JWT
	// TODO: Extract from JWT token claims
	businessID := "bid12345"
	h.logger.DebugWith("Business context", "business_id", businessID)

	// Call payee service
	h.logger.DebugWith("Calling payee service")
	if err := h.payeeService.DeactivatePayee(c.Request().Context(), businessID, payeeID, accountID); err != nil {
		h.logger.ErrorWith("Deactivation failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "deactivation_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Payee deactivated successfully", "payee_id", payeeID)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Payee deactivated successfully",
		"status":  "success",
	})
}

// GetVendorsForDropdown handles GET /vendors?bid=xxx - Get lightweight vendor list for dropdown
// Returns only active vendors with minimal fields for voucher creation UI
func (h *PayeeHandler) GetVendorsForDropdown(c echo.Context) error {
	h.logger.InfoWith("GetVendorsForDropdown API called")

	// Extract business_id from query parameter
	businessID := c.QueryParam("bid")
	if businessID == "" {
		h.logger.WarnWith("Missing business_id query parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "business_id (bid) query parameter is required",
		})
	}

	h.logger.DebugWith("Query parameter", "business_id", businessID)

	// Call payee service
	h.logger.DebugWith("Calling payee service")
	resp, err := h.payeeService.GetVendorsForDropdown(c.Request().Context(), businessID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch vendors", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Vendors fetched for dropdown", "count", resp.Count)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   resp,
	})
}
