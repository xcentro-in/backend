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
		h.logger.DebugWith("Employee onboard request", "full_name", req.Employee.FullName, "display_name", req.DisplayName)
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

	// Create filters for counting (without pagination)
	countFilters := payee.RelationSearchFilters{
		DisplayName: displayName,
		IsActive:    isActive,
		IsFavorite:  isFavorite,
		Tags:        []string{}, // TODO: Parse tags from query if needed
	}

	// Create filters for data fetching (with pagination)
	dataFilters := payee.RelationSearchFilters{
		DisplayName: displayName,
		IsActive:    isActive,
		IsFavorite:  isFavorite,
		Tags:        []string{}, // TODO: Parse tags from query if needed
		Limit:       pageSize,
		Offset:      (page - 1) * pageSize,
	}

	// Call payee service to get total count (without pagination)
	h.logger.DebugWith("Calling payee service for total count")
	totalCount, err := h.payeeService.CountPayeesByBusiness(c.Request().Context(), businessID, countFilters)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch total count", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "count_failed",
			"message": err.Error(),
		})
	}

	// Call payee service to get paginated data
	h.logger.DebugWith("Calling payee service for paginated data")
	relations, err := h.payeeService.GetPayeesByBusiness(c.Request().Context(), businessID, dataFilters)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payees", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	// Convert to response DTO (exclude internal IDs)
	payeeList := make([]*payee.PayeeListItem, len(relations))
	for i, relation := range relations {
		payeeList[i] = &payee.PayeeListItem{
			RelationID:   relation.RelationID,
			PayeeType:    relation.PayeeType,
			DisplayName:  relation.DisplayName,
			ContactEmail: relation.ContactEmail,
			ContactPhone: relation.ContactPhone,
			IsActive:     relation.IsActive,
			IsFavorite:   relation.IsFavorite,
			Tags:         relation.Tags,
			AddedAt:      relation.AddedAt,
			UpdatedAt:    relation.UpdatedAt,
		}
	}

	h.logger.InfoWith("Payees fetched successfully", "count", len(payeeList), "total_count", totalCount, "business_id", businessID)

	return c.JSON(http.StatusOK, map[string]any{
		"message":     "Payees retrieved successfully",
		"status":      "success",
		"total_count": totalCount,
		"page":        page,
		"page_size":   pageSize,
		"data":        payeeList,
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

// GetPayeeAccountDetails handles GET /payees/:relation_id/acc_details - Get lightweight account information
// Returns only last 4 digits of account number, bank name, and IFSC code
func (h *PayeeHandler) GetPayeeAccountDetails(c echo.Context) error {
	h.logger.InfoWith("GetPayeeAccountDetails API called")

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

	// Call payee service to get full details
	details, err := h.payeeService.GetPayeeDetailsByRelation(c.Request().Context(), businessID, relationID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payee details", "error", err)
		return c.JSON(http.StatusNotFound, map[string]any{
			"error":   "not_found",
			"message": err.Error(),
		})
	}

	// Check if bank details exist
	if details.BankDetails == nil {
		h.logger.WarnWith("No bank details available for payee", "relation_id", relationID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"error":   "no_bank_details",
			"message": "Bank details not available for this payee",
		})
	}

	// Extract last 4 digits of account number
	accountNumber := details.BankDetails.AccountNumber
	accountLast4 := ""
	if len(accountNumber) >= 4 {
		accountLast4 = accountNumber[len(accountNumber)-4:]
	}

	// Create lightweight response
	accountDetails := &payee.PayeeAccountDetails{
		AccountNumberLast4: accountLast4,
		BankName:           details.BankDetails.BankName,
		IFSCCode:           details.BankDetails.IFSCCode,
	}

	h.logger.InfoWith("Account details fetched successfully", "relation_id", relationID, "bank_name", accountDetails.BankName)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   accountDetails,
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

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
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

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
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

// GetVendorsForDropdown handles GET /vendors - Get lightweight vendor list for dropdown
// Returns only active vendors with minimal fields for voucher creation UI
func (h *PayeeHandler) GetVendorsForDropdown(c echo.Context) error {
	h.logger.InfoWith("GetVendorsForDropdown API called")

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Business context", "business_id", businessID)

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

// GetEmployeesForList handles GET /employees - Get lightweight employee list for tabulated display
// Returns only active employees with minimal fields
func (h *PayeeHandler) GetEmployeesForList(c echo.Context) error {
	h.logger.InfoWith("GetEmployeesForList API called")

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Business context", "business_id", businessID)

	// Call payee service
	h.logger.DebugWith("Calling payee service")
	resp, err := h.payeeService.GetEmployeesForList(c.Request().Context(), businessID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch employees", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Employees fetched for list", "count", resp.Count)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   resp,
	})
}

// GetRemunerationDocumentUploadURL handles GET /payees/remuneration/upload-url?relation_id=xxx
// Returns a presigned S3 URL for uploading employee remuneration document
func (h *PayeeHandler) GetRemunerationDocumentUploadURL(c echo.Context) error {
	h.logger.InfoWith("GetRemunerationDocumentUploadURL API called")

	// Get relation_id from query parameter
	relationID := c.QueryParam("relation_id")
	if relationID == "" {
		h.logger.WarnWith("Missing relation_id in query parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id query parameter is required",
		})
	}

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Request parameters", "relation_id", relationID, "business_id", businessID)

	// Call payee service to get presigned upload URL
	h.logger.DebugWith("Calling payee service")
	uploadURL, err := h.payeeService.GetRemunerationDocumentUploadURL(c.Request().Context(), businessID, relationID)
	if err != nil {
		h.logger.ErrorWith("Failed to get upload URL", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "upload_url_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Presigned upload URL generated", "relation_id", relationID)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"upload_url": uploadURL,
			"expires_in": "15 minutes",
		},
	})
}

// ConfirmRemunerationDocumentUpload handles GET /payees/remuneration/confirm-upload?relation_id=xxx
// Confirms the S3 upload and updates the database with the document key
func (h *PayeeHandler) ConfirmRemunerationDocumentUpload(c echo.Context) error {
	h.logger.InfoWith("ConfirmRemunerationDocumentUpload API called")

	// Get relation_id from query parameter
	relationID := c.QueryParam("relation_id")
	if relationID == "" {
		h.logger.WarnWith("Missing relation_id in query parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id query parameter is required",
		})
	}

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Request parameters", "relation_id", relationID, "business_id", businessID)

	// Call payee service to confirm upload and get presigned download URL
	h.logger.DebugWith("Calling payee service")
	downloadURL, err := h.payeeService.ConfirmRemunerationDocumentUpload(c.Request().Context(), businessID, relationID)
	if err != nil {
		h.logger.ErrorWith("Failed to confirm upload", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "confirm_upload_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Remuneration document upload confirmed", "relation_id", relationID)

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Document upload confirmed successfully",
		"data": map[string]any{
			"document_url": downloadURL,
			"expires_in":   "15 minutes",
		},
	})
}

// GetRemunerationDocumentDownloadURL handles GET /payees/remuneration/download-url?relation_id=xxx
// Generates a fresh presigned S3 URL for downloading the employee remuneration document
func (h *PayeeHandler) GetRemunerationDocumentDownloadURL(c echo.Context) error {
	h.logger.InfoWith("GetRemunerationDocumentDownloadURL API called")

	// Get relation_id from query parameter
	relationID := c.QueryParam("relation_id")
	if relationID == "" {
		h.logger.WarnWith("Missing relation_id in query parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id query parameter is required",
		})
	}

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		h.logger.WarnWith("Missing X-Business-ID header")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Request parameters", "relation_id", relationID, "business_id", businessID)

	// Call payee service to get presigned download URL
	h.logger.DebugWith("Calling payee service")
	downloadURL, err := h.payeeService.GetRemunerationDocumentDownloadURL(c.Request().Context(), businessID, relationID)
	if err != nil {
		// Only return error for actual failures (not missing document)
		h.logger.ErrorWith("Failed to get download URL", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "download_url_failed",
			"message": err.Error(),
		})
	}

	// Always return 200 with has_document field
	hasDocument := downloadURL != ""

	if hasDocument {
		h.logger.InfoWith("Presigned download URL generated", "relation_id", relationID)
	} else {
		h.logger.DebugWith("No document uploaded for employee", "relation_id", relationID)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"has_document": hasDocument,
			"download_url": downloadURL,
			"expires_in":   "15 minutes",
		},
	})
}
