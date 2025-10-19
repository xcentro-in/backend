package payee

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/payment"
	payeeSvc "xcentro.in/internal/services/payee"
)

// PayeeHandler handles payee-related HTTP requests
type PayeeHandler struct {
	payeeService *payeeSvc.PayeeService
}

// NewPayeeHandler creates a new payee handler
func NewPayeeHandler(payeeService *payeeSvc.PayeeService) *PayeeHandler {
	return &PayeeHandler{
		payeeService: payeeService,
	}
}

// OnboardPayee handles POST /payees - Onboard a new payee
func (h *PayeeHandler) OnboardPayee(c echo.Context) error {
	fmt.Println("=== HANDLER: OnboardPayee API Called ===")

	// Parse request body (vendor-specific for Phase 1)
	// TODO: Phase 7 - Make this polymorphic to support all payee types
	var req struct {
		PayeeType         string `json:"payee_type"` // "vendor" for now
		GSTIN             string `json:"gstin"`
		AccountNumber     string `json:"account_number"`
		IFSCCode          string `json:"ifsc_code"`
		AccountHolderName string `json:"account_holder_name"`
		BankName          string `json:"bank_name"`
		AccountType       string `json:"account_type"`
		AccountLabel      string `json:"account_label"`
		DisplayName       string `json:"display_name"`
		ContactEmail      string `json:"contact_email"`
		ContactPhone      string `json:"contact_phone"`
		Notes             string `json:"notes"`
	}

	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: PayeeType: %s, GSTIN: %s, Display Name: %s\n", req.PayeeType, req.GSTIN, req.DisplayName)

	// Extract business_id from JWT (TODO: implement JWT extraction)
	// TODO: Extract from JWT token claims
	businessID := "bid12345"
	fmt.Printf("🔐 Business ID (from JWT): %s\n", businessID)

	// Default to vendor if not specified (backward compatibility)
	payeeType := req.PayeeType
	if payeeType == "" {
		payeeType = "vendor"
	}

	// Convert to service request with polymorphic structure
	serviceReq := payee.OnboardPayeeRequest{
		BusinessID:   businessID,
		PayeeType:    payee.PayeeType(payeeType),
		DisplayName:  req.DisplayName,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Notes:        req.Notes,
		Vendor: &payee.OnboardVendorData{
			GSTIN:             req.GSTIN,
			AccountNumber:     req.AccountNumber,
			IFSCCode:          req.IFSCCode,
			AccountHolderName: req.AccountHolderName,
			BankName:          req.BankName,
			AccountType:       payment.BankAccountType(req.AccountType),
			AccountLabel:      req.AccountLabel,
		},
	}

	// Call payee service to onboard payee
	fmt.Println("🔄 Calling Payee Service...")
	resp, err := h.payeeService.OnboardPayee(c.Request().Context(), serviceReq)
	if err != nil {
		fmt.Printf("❌ Onboarding error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "onboarding_failed",
			"message": err.Error(),
		})
	}

	// Convert to response
	response := map[string]any{
		"payee_id":                  resp.PayeeID,
		"account_id":                resp.AccountID,
		"gst_verification_status":   string(resp.GSTVerificationStatus),
		"bank_verification_status":  string(resp.BankVerificationStatus),
		"cross_verification_status": string(resp.CrossVerificationStatus),
		"is_fully_verified":         resp.IsFullyVerified,
		"message":                   resp.Message,
	}

	fmt.Printf("✅ Payee onboarded successfully: PayeeID=%s, AccountID=%s\n", resp.PayeeID, resp.AccountID)
	fmt.Println("=== HANDLER: OnboardPayee Complete ===")

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Payee onboarded successfully",
		"status":  "success",
		"data":    response,
	})
}

// GetPayees handles GET /payees - List all payees for a business
func (h *PayeeHandler) GetPayees(c echo.Context) error {
	fmt.Println("=== HANDLER: GetPayees API Called ===")

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

	fmt.Printf("📥 Query: display_name=%s, is_active=%v, is_favorite=%v, page=%d, page_size=%d\n",
		displayName, isActive, isFavorite, page, pageSize)

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}
	fmt.Printf("🔐 Business ID (from header): %s\n", businessID)

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
	fmt.Println("🔄 Calling Payee Service...")
	relations, err := h.payeeService.GetPayeesByBusiness(c.Request().Context(), businessID, filters)
	if err != nil {
		fmt.Printf("❌ Fetch error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	// Convert to response (simplified - returning relations directly)
	// In production, you might want to enrich with payee and account details
	fmt.Printf("✅ Fetched %d payees for business %s\n", len(relations), businessID)
	fmt.Println("=== HANDLER: GetPayees Complete ===")

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
	fmt.Println("=== HANDLER: GetPayeeDetails API Called ===")

	// Extract relation_id from path parameter
	relationID := c.Param("relation_id")
	if relationID == "" {
		fmt.Println("❌ Missing relation_id path parameter")
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

	fmt.Printf("📥 Path param: relation_id=%s, Business ID (from header): %s\n", relationID, businessID)

	// Call payee service with business_id validation
	fmt.Println("🔄 Calling Payee Service...")
	details, err := h.payeeService.GetPayeeDetailsByRelation(c.Request().Context(), businessID, relationID)
	if err != nil {
		fmt.Printf("❌ Fetch error: %v\n", err)
		return c.JSON(http.StatusNotFound, map[string]any{
			"error":   "not_found",
			"message": err.Error(),
		})
	}

	fmt.Printf("✅ Payee details fetched: %s (Type: %s)\n", details.DisplayName, details.PayeeType)
	fmt.Println("=== HANDLER: GetPayeeDetails Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   details,
	})
}

// UpdatePayee handles PUT /payees/:payee_id/accounts/:account_id - Update payee relationship metadata
func (h *PayeeHandler) UpdatePayee(c echo.Context) error {
	fmt.Println("=== HANDLER: UpdatePayee API Called ===")

	// Extract path parameters
	payeeID := c.Param("payee_id")
	accountID := c.Param("account_id")

	fmt.Printf("📥 Path params: payee_id=%s, account_id=%s\n", payeeID, accountID)

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
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	fmt.Printf("📥 Request: display_name=%s, is_favorite=%v\n", req.DisplayName, req.IsFavorite)

	// Extract business_id from JWT
	// TODO: Extract from JWT token claims
	businessID := "bid12345"
	fmt.Printf("🔐 Business ID (from JWT): %s\n", businessID)

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
	fmt.Println("🔄 Calling Payee Service...")
	if err := h.payeeService.UpdatePayeeRelation(c.Request().Context(), updateReq); err != nil {
		fmt.Printf("❌ Update error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "update_failed",
			"message": err.Error(),
		})
	}

	fmt.Printf("✅ Payee updated successfully: %s\n", payeeID)
	fmt.Println("=== HANDLER: UpdatePayee Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Payee updated successfully",
		"status":  "success",
	})
}

// DeactivatePayee handles DELETE /payees/:payee_id/accounts/:account_id - Deactivate a payee (soft delete)
func (h *PayeeHandler) DeactivatePayee(c echo.Context) error {
	fmt.Println("=== HANDLER: DeactivatePayee API Called ===")

	// Extract path parameters
	payeeID := c.Param("payee_id")
	accountID := c.Param("account_id")

	fmt.Printf("📥 Path params: payee_id=%s, account_id=%s\n", payeeID, accountID)

	// Extract business_id from JWT
	// TODO: Extract from JWT token claims
	businessID := "bid12345"
	fmt.Printf("🔐 Business ID (from JWT): %s\n", businessID)

	// Call payee service
	fmt.Println("🔄 Calling Payee Service...")
	if err := h.payeeService.DeactivatePayee(c.Request().Context(), businessID, payeeID, accountID); err != nil {
		fmt.Printf("❌ Deactivation error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "deactivation_failed",
			"message": err.Error(),
		})
	}

	fmt.Printf("✅ Payee deactivated successfully: %s\n", payeeID)
	fmt.Println("=== HANDLER: DeactivatePayee Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Payee deactivated successfully",
		"status":  "success",
	})
}

// GetVendorsForDropdown handles GET /vendors?bid=xxx - Get lightweight vendor list for dropdown
// Returns only active vendors with minimal fields for voucher creation UI
func (h *PayeeHandler) GetVendorsForDropdown(c echo.Context) error {
	fmt.Println("=== HANDLER: GetVendorsForDropdown API Called ===")

	// Extract business_id from query parameter
	businessID := c.QueryParam("bid")
	if businessID == "" {
		fmt.Println("❌ Missing business_id query parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "business_id (bid) query parameter is required",
		})
	}

	fmt.Printf("📥 Query param: bid=%s\n", businessID)

	// Call payee service
	fmt.Println("🔄 Calling Payee Service...")
	resp, err := h.payeeService.GetVendorsForDropdown(c.Request().Context(), businessID)
	if err != nil {
		fmt.Printf("❌ Error fetching vendors: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	fmt.Printf("✅ Found %d vendors for dropdown\n", resp.Count)
	fmt.Println("=== HANDLER: GetVendorsForDropdown Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   resp,
	})
}
