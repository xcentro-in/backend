package voucher

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/voucher"
	voucherSvc "xcentro.in/internal/services/voucher"
)

// VoucherHandler handles voucher-related HTTP requests
type VoucherHandler struct {
	voucherService voucherSvc.VoucherService
}

// NewVoucherHandler creates a new voucher handler
func NewVoucherHandler(voucherService voucherSvc.VoucherService) *VoucherHandler {
	return &VoucherHandler{
		voucherService: voucherService,
	}
}

// CreateDraftVoucher handles POST /vouchers/draft - Create draft voucher
func (h *VoucherHandler) CreateDraftVoucher(c echo.Context) error {
	fmt.Println("=== HANDLER: CreateDraftVoucher API Called ===")

	// Parse request body
	var req voucherSvc.CreateDraftVoucherRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// TODO: Extract business_id and user_id from JWT
	// For now using hardcoded values (Phase 6 will integrate JWT)
	if req.BusinessID == uuid.Nil {
		// TODO: Extract from JWT token claims
		req.BusinessID = uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder
	}
	if req.UserID == uuid.Nil {
		// TODO: Extract from JWT token claims
		req.UserID = uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder
	}

	fmt.Printf("📥 Request: RelationID: %s, Invoice: %s, Amount: ₹%.2f\n",
		req.RelationID, req.InvoiceNumber, float64(req.NetAmount)/100)

	// Call service
	fmt.Println("🔄 Calling Voucher Service...")
	resp, err := h.voucherService.CreateDraftVoucher(c.Request().Context(), &req)
	if err != nil {
		fmt.Printf("❌ Create draft error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Draft voucher created: %s\n", resp.VoucherNumber)

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Draft voucher created successfully",
		"status":  "success",
		"data":    resp,
	})
}

// ListVouchers handles GET /vouchers - List vouchers with filters
func (h *VoucherHandler) ListVouchers(c echo.Context) error {
	fmt.Println("=== HANDLER: ListVouchers API Called ===")

	// TODO: Extract business_id from JWT
	businessIDStr := c.QueryParam("business_id")
	if businessIDStr == "" {
		// TODO: Extract from JWT
		businessIDStr = "00000000-0000-0000-0000-000000000000" // Placeholder
	}
	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_business_id",
			"message": "Invalid business ID format",
		})
	}

	// Parse filters from query params
	filters := h.parseFilters(c)

	fmt.Printf("📥 Business ID: %s, Filters: Status=%v, Page=%d\n",
		businessID, filters.Status, filters.Page)

	// Call service
	resp, err := h.voucherService.ListVouchersForBusiness(c.Request().Context(), businessID, filters)
	if err != nil {
		fmt.Printf("❌ List vouchers error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Found %d vouchers\n", len(resp.Vouchers))

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Vouchers retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// GetVoucherDetails handles GET /vouchers/:id - Get voucher details
func (h *VoucherHandler) GetVoucherDetails(c echo.Context) error {
	fmt.Println("=== HANDLER: GetVoucherDetails API Called ===")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	fmt.Printf("📥 Voucher ID: %s\n", voucherID)

	// Call service
	resp, err := h.voucherService.GetVoucherDetails(c.Request().Context(), voucherID)
	if err != nil {
		fmt.Printf("❌ Get voucher error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Voucher details retrieved: %s\n", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher details retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// UpdateVoucher handles PUT /vouchers/:id - Update draft voucher
func (h *VoucherHandler) UpdateVoucher(c echo.Context) error {
	fmt.Println("=== HANDLER: UpdateVoucher API Called ===")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	// Parse request body
	var req voucherSvc.UpdateVoucherRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Set voucher_id from URL path
	req.VoucherID = voucherID

	fmt.Printf("📥 Voucher ID: %s, Updating fields...\n", voucherID)

	// Call service
	fmt.Println("🔄 Calling Voucher Service...")
	resp, err := h.voucherService.UpdateVoucher(c.Request().Context(), &req)
	if err != nil {
		fmt.Printf("❌ Update voucher error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Voucher updated: %s (%d fields changed)\n", resp.VoucherNumber, len(resp.UpdatedFields))

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher updated successfully",
		"status":  "success",
		"data":    resp,
	})
}

// FinalizeVoucher handles PUT /vouchers/:id/finalize - Finalize voucher manually
func (h *VoucherHandler) FinalizeVoucher(c echo.Context) error {
	fmt.Println("=== HANDLER: FinalizeVoucher API Called ===")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	// Parse request body
	var req struct {
		PaymentID string `json:"payment_id"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	paymentID, err := uuid.Parse(req.PaymentID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_payment_id",
			"message": "Invalid payment ID format",
		})
	}

	fmt.Printf("📥 Voucher ID: %s, Payment ID: %s\n", voucherID, paymentID)

	// Call service
	finalizeReq := &voucherSvc.FinalizeVoucherRequest{
		VoucherID: voucherID,
		PaymentID: paymentID,
	}
	resp, err := h.voucherService.FinalizeVoucher(c.Request().Context(), finalizeReq)
	if err != nil {
		fmt.Printf("❌ Finalize voucher error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Voucher finalized: %s\n", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher finalized successfully",
		"status":  "success",
		"data":    resp,
	})
}

// CreateMultiVouchers handles POST /vouchers/multi - Create multiple vouchers for multi-bill payment
func (h *VoucherHandler) CreateMultiVouchers(c echo.Context) error {
	fmt.Println("=== HANDLER: CreateMultiVouchers API Called ===")

	// Parse request body
	var req voucherSvc.CreateMultiVouchersRequest
	if err := c.Bind(&req); err != nil {
		fmt.Printf("❌ Error binding request: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// TODO: Extract business_id and user_id from JWT
	if req.BusinessID == uuid.Nil {
		req.BusinessID = uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder
	}
	if req.UserID == uuid.Nil {
		req.UserID = uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder
	}

	fmt.Printf("📥 Request: %d bills to process\n", len(req.Vouchers))

	// Call service
	resp, err := h.voucherService.CreateMultiVouchers(c.Request().Context(), &req)
	if err != nil {
		fmt.Printf("❌ Create multi vouchers error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Created %d vouchers\n", len(resp.Vouchers))

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Multiple vouchers created successfully",
		"status":  "success",
		"data":    resp,
	})
}

// GetDraftReminders handles GET /vouchers/reminders - Get abandoned draft vouchers
func (h *VoucherHandler) GetDraftReminders(c echo.Context) error {
	fmt.Println("=== HANDLER: GetDraftReminders API Called ===")

	// TODO: Extract business_id from JWT
	businessIDStr := c.QueryParam("business_id")
	if businessIDStr == "" {
		businessIDStr = "00000000-0000-0000-0000-000000000000" // Placeholder
	}
	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_business_id",
			"message": "Invalid business ID format",
		})
	}

	// Parse older_than parameter (default: 24 hours)
	olderThanHours := 24
	if hoursStr := c.QueryParam("older_than_hours"); hoursStr != "" {
		fmt.Sscanf(hoursStr, "%d", &olderThanHours)
	}

	req := &voucherSvc.GetDraftRemindersRequest{
		BusinessID: businessID,
		OlderThan:  time.Duration(olderThanHours) * time.Hour,
	}

	fmt.Printf("📥 Business ID: %s, Older than: %d hours\n", businessID, olderThanHours)

	// Call service
	resp, err := h.voucherService.GetDraftReminders(c.Request().Context(), req)
	if err != nil {
		fmt.Printf("❌ Get draft reminders error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Found %d draft reminders\n", resp.Count)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Draft reminders retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// RequestCancellation handles POST /vouchers/:id/cancel - Request voucher cancellation
func (h *VoucherHandler) RequestCancellation(c echo.Context) error {
	fmt.Println("=== HANDLER: RequestCancellation API Called ===")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	// Parse request body
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Validate reason
	if req.Reason == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "missing_reason",
			"message": "Cancellation reason is required",
		})
	}

	// TODO: Extract user_id from JWT
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder

	fmt.Printf("📥 Voucher ID: %s, User ID: %s, Reason: %s\n", voucherID, userID, req.Reason)

	// Call service
	cancelReq := &voucherSvc.RequestCancellationRequest{
		VoucherID: voucherID,
		UserID:    userID,
		Reason:    req.Reason,
	}
	resp, err := h.voucherService.RequestCancellation(c.Request().Context(), cancelReq)
	if err != nil {
		fmt.Printf("❌ Request cancellation error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Cancellation requested for voucher: %s\n", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Cancellation request submitted successfully",
		"status":  "success",
		"data":    resp,
	})
}

// ApproveCancellation handles PUT /vouchers/:id/cancel/approve - Approve or reject cancellation
func (h *VoucherHandler) ApproveCancellation(c echo.Context) error {
	fmt.Println("=== HANDLER: ApproveCancellation API Called ===")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	// Parse request body
	var req struct {
		Action  string `json:"action"`  // "approve" or "reject"
		Remarks string `json:"remarks"` // Optional remarks
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Validate action
	if req.Action != "approve" && req.Action != "reject" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_action",
			"message": "Action must be 'approve' or 'reject'",
		})
	}

	// TODO: Extract user_id from JWT (operations/finance user)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000000") // Placeholder

	fmt.Printf("📥 Voucher ID: %s, User ID: %s, Action: %s\n", voucherID, userID, req.Action)

	// Call service
	approveReq := &voucherSvc.ApproveCancellationRequest{
		VoucherID: voucherID,
		UserID:    userID,
		Action:    req.Action,
		Remarks:   req.Remarks,
	}
	resp, err := h.voucherService.ApproveCancellation(c.Request().Context(), approveReq)
	if err != nil {
		fmt.Printf("❌ Approve cancellation error: %v\n", err)
		return h.handleError(c, err)
	}

	fmt.Printf("✅ Cancellation %s for voucher: %s\n", req.Action, resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": fmt.Sprintf("Cancellation %s successfully", req.Action),
		"status":  "success",
		"data":    resp,
	})
}

// Helper methods

// parseFilters parses voucher filters from query parameters
func (h *VoucherHandler) parseFilters(c echo.Context) voucher.VoucherFilters {
	filters := voucher.VoucherFilters{
		Page:  1,
		Limit: 50,
	}

	// Status filter
	if statusStr := c.QueryParam("status"); statusStr != "" {
		status := voucher.VoucherStatus(statusStr)
		filters.Status = &status
	}

	// ERP sync status filter
	if erpStatusStr := c.QueryParam("erp_sync_status"); erpStatusStr != "" {
		erpStatus := voucher.ERPSyncStatus(erpStatusStr)
		filters.ERPSyncStatus = &erpStatus
	}

	// Date range filters
	if dateFromStr := c.QueryParam("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse("2006-01-02", dateFromStr); err == nil {
			filters.DateFrom = &dateFrom
		}
	}
	if dateToStr := c.QueryParam("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse("2006-01-02", dateToStr); err == nil {
			filters.DateTo = &dateTo
		}
	}

	// Fiscal year filter
	if fiscalYear := c.QueryParam("fiscal_year"); fiscalYear != "" {
		filters.FiscalYear = &fiscalYear
	}

	// Payee ID filter
	if payeeIDStr := c.QueryParam("payee_id"); payeeIDStr != "" {
		if payeeID, err := uuid.Parse(payeeIDStr); err == nil {
			filters.PayeeID = &payeeID
		}
	}

	// Expense category filter
	if category := c.QueryParam("expense_category"); category != "" {
		filters.ExpenseCategory = &category
	}

	// Invoice number filter
	if invoiceNumber := c.QueryParam("invoice_number"); invoiceNumber != "" {
		filters.InvoiceNumber = &invoiceNumber
	}

	// Pagination
	if pageStr := c.QueryParam("page"); pageStr != "" {
		fmt.Sscanf(pageStr, "%d", &filters.Page)
	}
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &filters.Limit)
	}

	// Sorting
	if sortBy := c.QueryParam("sort_by"); sortBy != "" {
		filters.SortBy = sortBy
	}
	if sortOrder := c.QueryParam("sort_order"); sortOrder != "" {
		filters.SortOrder = sortOrder
	}

	return filters
}

// handleError handles service errors and returns appropriate HTTP responses
func (h *VoucherHandler) handleError(c echo.Context, err error) error {
	// Map domain errors to HTTP status codes
	switch err {
	case voucher.ErrVoucherNotFound:
		return c.JSON(http.StatusNotFound, map[string]any{
			"error":   "voucher_not_found",
			"message": "Voucher not found",
		})
	case voucher.ErrVoucherNotDraft:
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "voucher_not_draft",
			"message": "Voucher is not in draft status",
		})
	case voucher.ErrVoucherAlreadyFinalized:
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "voucher_already_finalized",
			"message": "Voucher is already finalized",
		})
	case voucher.ErrVoucherAlreadyExists:
		return c.JSON(http.StatusConflict, map[string]any{
			"error":   "voucher_already_exists",
			"message": "Voucher with this number already exists",
		})
	case voucher.ErrInvalidBusinessID, voucher.ErrInvalidPayeeID, voucher.ErrInvalidAccountID:
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_data",
			"message": err.Error(),
		})
	case voucher.ErrAmountMismatch:
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "amount_mismatch",
			"message": "Amount calculation mismatch",
		})
	default:
		// Internal server error for unexpected errors
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal_server_error",
			"message": err.Error(),
		})
	}
}

// GetVouchersByPayee handles GET /vouchers/payee/:relation_id - Get all vouchers for a specific payee
func (h *VoucherHandler) GetVouchersByPayee(c echo.Context) error {
	fmt.Println("=== HANDLER: GetVouchersByPayee API Called ===")

	// Extract relation_id from path parameter
	relationIDStr := c.Param("relation_id")
	if relationIDStr == "" {
		fmt.Println("❌ Missing relation_id path parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id path parameter is required",
		})
	}

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		fmt.Printf("❌ Invalid relation_id format: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid relation_id format",
		})
	}

	// Extract business_id from X-Business-ID header
	businessIDStr := c.Request().Header.Get("X-Business-ID")
	if businessIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		fmt.Printf("❌ Invalid business_id format: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid business_id format",
		})
	}

	// Parse pagination query parameters
	page := 1
	pageSize := 10

	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := parseIntParam(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := c.QueryParam("page_size"); pageSizeStr != "" {
		if ps, err := parseIntParam(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	fmt.Printf("📥 Path param: relation_id=%s, Business ID (from header): %s, page=%d, page_size=%d\n",
		relationID, businessID, page, pageSize)

	// Call voucher service
	fmt.Println("🔄 Calling Voucher Service...")
	resp, err := h.voucherService.GetVouchersByPayee(c.Request().Context(), &voucherSvc.GetVouchersByPayeeRequest{
		BusinessID: businessID,
		RelationID: relationID,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		fmt.Printf("❌ Fetch error: %v\n", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	fmt.Printf("✅ Fetched %d vouchers for relation %s\n", len(resp.Vouchers), relationID)
	fmt.Println("=== HANDLER: GetVouchersByPayee Complete ===")

	return c.JSON(http.StatusOK, map[string]any{
		"status":     "success",
		"vouchers":   resp.Vouchers,
		"pagination": resp.Pagination,
		"summary":    resp.Summary,
	})
}

// Helper function to parse integer query parameters
func parseIntParam(s string) (int, error) {
	return strconv.Atoi(s)
}
