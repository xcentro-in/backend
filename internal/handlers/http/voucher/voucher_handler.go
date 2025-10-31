package voucher

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/core/voucher"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/logger"
)

// VoucherHandler handles voucher-related HTTP requests
type VoucherHandler struct {
	voucherService voucherSvc.VoucherService
	logger         *logger.Logger
}

// NewVoucherHandler creates a new voucher handler
func NewVoucherHandler(voucherService voucherSvc.VoucherService, logger *logger.Logger) *VoucherHandler {
	return &VoucherHandler{
		voucherService: voucherService,
		logger:         logger,
	}
}

// CreateDraftVoucher handles POST /vouchers/draft - Create draft voucher
func (h *VoucherHandler) CreateDraftVoucher(c echo.Context) error {
	h.logger.InfoWith("CreateDraftVoucher API called")

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
		h.logger.ErrorWith("Invalid business_id format", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid business_id format",
		})
	}

	// Extract userId from JWT context
	userIDStr, ok := c.Get("userId").(string)
	if !ok || userIDStr == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.ErrorWith("Invalid user_id format", "error", err)
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "invalid_user_id",
			"message": "Invalid user ID format",
		})
	}

	// DEBUG: Print raw request body
	bodyBytes, _ := io.ReadAll(c.Request().Body)
	c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	fmt.Printf("DEBUG - Request Body: %s\n", string(bodyBytes))

	// Parse request body
	var req voucherSvc.CreateDraftVoucherRequest
	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Set businessID and userID from context
	req.BusinessID = businessID
	req.UserID = userID

	h.logger.DebugWith("Draft voucher request",
		"relation_id", req.RelationID,
		"invoice_number", req.InvoiceNumber,
		"net_amount", req.NetAmount)

	// Call service
	h.logger.DebugWith("Calling voucher service")
	resp, err := h.voucherService.CreateDraftVoucher(c.Request().Context(), &req)
	if err != nil {
		h.logger.ErrorWith("Create draft failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Draft voucher created", "voucher_number", resp.VoucherNumber)

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Draft voucher created successfully",
		"status":  "success",
		"data":    resp,
	})
}

// ListVouchers handles GET /vouchers - List vouchers with filters
func (h *VoucherHandler) ListVouchers(c echo.Context) error {
	h.logger.InfoWith("ListVouchers API called")

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

	h.logger.DebugWith("List vouchers request",
		"business_id", businessID,
		"status", filters.Status,
		"page", filters.Page)

	// Call service
	resp, err := h.voucherService.ListVouchersForBusiness(c.Request().Context(), businessID, filters)
	if err != nil {
		h.logger.ErrorWith("List vouchers failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Vouchers listed", "count", len(resp.Vouchers))

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Vouchers retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// GetVoucherDetails handles GET /vouchers/:id - Get voucher details
func (h *VoucherHandler) GetVoucherDetails(c echo.Context) error {
	h.logger.InfoWith("GetVoucherDetails API called")

	// Parse voucher_id from path
	voucherIDStr := c.Param("id")
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_voucher_id",
			"message": "Invalid voucher ID format",
		})
	}

	h.logger.DebugWith("Get voucher details", "voucher_id", voucherID)

	// Call service
	resp, err := h.voucherService.GetVoucherDetails(c.Request().Context(), voucherID)
	if err != nil {
		h.logger.ErrorWith("Get voucher failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Voucher details retrieved", "voucher_number", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher details retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// UpdateVoucher handles PUT /vouchers/:id - Update draft voucher
func (h *VoucherHandler) UpdateVoucher(c echo.Context) error {
	h.logger.InfoWith("UpdateVoucher API called")

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
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Set voucher_id from URL path
	req.VoucherID = voucherID

	h.logger.DebugWith("Update voucher request", "voucher_id", voucherID)

	// Call service
	h.logger.DebugWith("Calling voucher service")
	resp, err := h.voucherService.UpdateVoucher(c.Request().Context(), &req)
	if err != nil {
		h.logger.ErrorWith("Update voucher failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Voucher updated",
		"voucher_number", resp.VoucherNumber,
		"fields_changed", len(resp.UpdatedFields))

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher updated successfully",
		"status":  "success",
		"data":    resp,
	})
}

// FinalizeVoucher handles PUT /vouchers/:id/finalize - Finalize voucher manually
func (h *VoucherHandler) FinalizeVoucher(c echo.Context) error {
	h.logger.InfoWith("FinalizeVoucher API called")

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

	h.logger.DebugWith("Finalize voucher request",
		"voucher_id", voucherID,
		"payment_id", paymentID)

	// Call service
	finalizeReq := &voucherSvc.FinalizeVoucherRequest{
		VoucherID: voucherID,
		PaymentID: paymentID,
	}
	resp, err := h.voucherService.FinalizeVoucher(c.Request().Context(), finalizeReq)
	if err != nil {
		h.logger.ErrorWith("Finalize voucher failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Voucher finalized", "voucher_number", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Voucher finalized successfully",
		"status":  "success",
		"data":    resp,
	})
}

// CreateMultiVouchers handles POST /vouchers/multi - Create multiple vouchers for multi-bill payment
func (h *VoucherHandler) CreateMultiVouchers(c echo.Context) error {
	h.logger.InfoWith("CreateMultiVouchers API called")

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
		h.logger.ErrorWith("Invalid business_id format", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid business_id format",
		})
	}

	// Extract userId from JWT context
	userIDStr, ok := c.Get("userId").(string)
	if !ok || userIDStr == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.ErrorWith("Invalid user_id format", "error", err)
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "invalid_user_id",
			"message": "Invalid user ID format",
		})
	}

	// Parse request body
	var req voucherSvc.CreateMultiVouchersRequest
	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	// Set businessID and userID from context
	req.BusinessID = businessID
	req.UserID = userID

	h.logger.DebugWith("Multi voucher request", "bill_count", len(req.Vouchers))

	// Call service
	resp, err := h.voucherService.CreateMultiVouchers(c.Request().Context(), &req)
	if err != nil {
		h.logger.ErrorWith("Create multi vouchers failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Multi vouchers created", "voucher_count", len(resp.Vouchers))

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Multiple vouchers created successfully",
		"status":  "success",
		"data":    resp,
	})
}

// GetDraftReminders handles GET /vouchers/reminders - Get abandoned draft vouchers
func (h *VoucherHandler) GetDraftReminders(c echo.Context) error {
	h.logger.InfoWith("GetDraftReminders API called")

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

	h.logger.DebugWith("Draft reminders request",
		"business_id", businessID,
		"older_than_hours", olderThanHours)

	// Call service
	resp, err := h.voucherService.GetDraftReminders(c.Request().Context(), req)
	if err != nil {
		h.logger.ErrorWith("Get draft reminders failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Draft reminders found", "count", resp.Count)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Draft reminders retrieved successfully",
		"status":  "success",
		"data":    resp,
	})
}

// RequestCancellation handles POST /vouchers/:id/cancel - Request voucher cancellation
func (h *VoucherHandler) RequestCancellation(c echo.Context) error {
	h.logger.InfoWith("RequestCancellation API called")

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

	h.logger.DebugWith("Cancellation request",
		"voucher_id", voucherID,
		"user_id", userID,
		"reason", req.Reason)

	// Call service
	cancelReq := &voucherSvc.RequestCancellationRequest{
		VoucherID: voucherID,
		UserID:    userID,
		Reason:    req.Reason,
	}
	resp, err := h.voucherService.RequestCancellation(c.Request().Context(), cancelReq)
	if err != nil {
		h.logger.ErrorWith("Request cancellation failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Cancellation requested", "voucher_number", resp.VoucherNumber)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Cancellation request submitted successfully",
		"status":  "success",
		"data":    resp,
	})
}

// ApproveCancellation handles PUT /vouchers/:id/cancel/approve - Approve or reject cancellation
func (h *VoucherHandler) ApproveCancellation(c echo.Context) error {
	h.logger.InfoWith("ApproveCancellation API called")

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

	h.logger.DebugWith("Approve cancellation request",
		"voucher_id", voucherID,
		"user_id", userID,
		"action", req.Action)

	// Call service
	approveReq := &voucherSvc.ApproveCancellationRequest{
		VoucherID: voucherID,
		UserID:    userID,
		Action:    req.Action,
		Remarks:   req.Remarks,
	}
	resp, err := h.voucherService.ApproveCancellation(c.Request().Context(), approveReq)
	if err != nil {
		h.logger.ErrorWith("Approve cancellation failed", "error", err)
		return h.handleError(c, err)
	}

	h.logger.InfoWith("Cancellation processed",
		"action", req.Action,
		"voucher_number", resp.VoucherNumber)

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
	h.logger.InfoWith("GetVouchersByPayee API called")

	// Extract relation_id from path parameter
	relationIDStr := c.Param("relation_id")
	if relationIDStr == "" {
		h.logger.WarnWith("Missing relation_id path parameter")
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "relation_id path parameter is required",
		})
	}

	relationID, err := uuid.Parse(relationIDStr)
	if err != nil {
		h.logger.ErrorWith("Invalid relation_id format", "error", err)
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
		h.logger.ErrorWith("Invalid business_id format", "error", err)
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

	h.logger.DebugWith("Get vouchers by payee request",
		"relation_id", relationID,
		"business_id", businessID,
		"page", page,
		"page_size", pageSize)

	// Call voucher service
	h.logger.DebugWith("Calling voucher service")
	resp, err := h.voucherService.GetVouchersByPayee(c.Request().Context(), &voucherSvc.GetVouchersByPayeeRequest{
		BusinessID: businessID,
		RelationID: relationID,
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		h.logger.ErrorWith("Get vouchers by payee failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "fetch_failed",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Vouchers fetched for payee",
		"count", len(resp.Vouchers),
		"relation_id", relationID)

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

// GetVoucherList handles GET /vouchers/list - Get all bills/vouchers for a business
// Includes payment status (paid, open, overdue) with optional status filtering
func (h *VoucherHandler) GetVoucherList(c echo.Context) error {
	h.logger.InfoWith("GetVoucherList API called")

	// Extract business_id from X-Business-ID header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Extract userId from JWT context for authorization
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	// Parse pagination parameters
	limit, _ := parseIntParam(c.QueryParam("limit"))
	offset, _ := parseIntParam(c.QueryParam("offset"))

	// Parse optional status filter
	var statusFilter *voucher.BillPaymentStatus
	statusParam := c.QueryParam("status")
	if statusParam != "" {
		status := voucher.BillPaymentStatus(statusParam)
		// Validate status
		if status != voucher.BillStatusPaid && status != voucher.BillStatusOpen && status != voucher.BillStatusOverdue {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"error":   "INVALID_STATUS",
				"message": "Status must be one of: paid, open, overdue",
			})
		}
		statusFilter = &status
	}

	h.logger.DebugWith("Fetching voucher list",
		"business_id", businessID,
		"status_filter", statusFilter,
		"limit", limit,
		"offset", offset)

	// Call voucher service (validation happens in service layer)
	response, err := h.voucherService.GetVoucherList(c.Request().Context(), businessID, statusFilter, limit, offset)
	if err != nil {
		h.logger.ErrorWith("Failed to get voucher list", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": "Failed to retrieve voucher list",
		})
	}

	h.logger.InfoWith("Voucher list fetched",
		"count", len(response.Vouchers),
		"total_count", response.TotalCount)

	return c.JSON(http.StatusOK, response)
}

// RequestApproval handles POST /vouchers/:voucher_id/request-approval - Create or update approval
func (h *VoucherHandler) RequestApproval(c echo.Context) error {
	h.logger.InfoWith("RequestApproval API called")

	// Extract voucher_id from URL parameter
	voucherID := c.Param("voucher_id")
	if voucherID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_VOUCHER_ID",
			"message": "Voucher ID is required",
		})
	}

	// Extract userId from JWT context
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	// Extract business_id from header (for validation)
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Parse request body
	var reqBody struct {
		ApproverUserID string `json:"approver_user_id" validate:"required,uuid"`
		Action         string `json:"action" validate:"required,oneof=pending approved rejected request_changes"`
		Remarks        string `json:"remarks"`
	}

	if err := c.Bind(&reqBody); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// Validate action
	action := voucher.ApprovalAction(reqBody.Action)
	if action != voucher.ApprovalActionPending &&
		action != voucher.ApprovalActionApproved &&
		action != voucher.ApprovalActionRejected &&
		action != voucher.ApprovalActionRequestChanges {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_ACTION",
			"message": "Action must be one of: pending, approved, rejected, request_changes",
		})
	}

	h.logger.DebugWith("Processing approval request",
		"voucher_id", voucherID,
		"action", action,
		"requester", userID,
		"approver", reqBody.ApproverUserID)

	// Create service request
	serviceReq := &voucher.VoucherApprovalRequest{
		VoucherID:       voucherID,
		BusinessID:      businessID,
		RequesterUserID: userID,
		ApproverUserID:  reqBody.ApproverUserID,
		Action:          action,
		Remarks:         reqBody.Remarks,
	}

	// Call service
	response, err := h.voucherService.RequestApproval(c.Request().Context(), serviceReq)
	if err != nil {
		h.logger.ErrorWith("Failed to process approval request", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Approval request processed successfully",
		"approval_id", response.ApprovalID,
		"action", response.Action)

	return c.JSON(http.StatusOK, response)
}

// GetApprovalInbox retrieves vouchers for approval inbox filtered by status
func (h *VoucherHandler) GetApprovalInbox(c echo.Context) error {
	h.logger.InfoWith("GetApprovalInbox API called")

	// Extract business_id from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Extract status query parameter (REQUIRED)
	statusParam := c.QueryParam("status")
	if statusParam == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_STATUS",
			"message": "status query parameter is required",
		})
	}

	// Validate status
	status := voucher.ApprovalAction(statusParam)
	if status != voucher.ApprovalActionPending &&
		status != voucher.ApprovalActionApproved &&
		status != voucher.ApprovalActionRejected &&
		status != voucher.ApprovalActionRequestChanges {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_STATUS",
			"message": "status must be one of: pending, approved, rejected, request_changes",
		})
	}

	h.logger.DebugWith("Fetching approval inbox",
		"business_id", businessID,
		"status", status)

	// Create service request
	serviceReq := &voucher.VoucherApprovalInboxRequest{
		BusinessID: businessID,
		Status:     status,
	}

	// Call service
	response, err := h.voucherService.GetApprovalInbox(c.Request().Context(), serviceReq)
	if err != nil {
		h.logger.ErrorWith("Failed to get approval inbox", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Approval inbox retrieved successfully",
		"total_count", response.TotalCount,
		"status", status)

	return c.JSON(http.StatusOK, response)
}

// GetApprovalInboxCounts retrieves counts for each approval status
func (h *VoucherHandler) GetApprovalInboxCounts(c echo.Context) error {
	h.logger.InfoWith("GetApprovalInboxCounts API called")

	// Extract business_id from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	h.logger.DebugWith("Fetching approval inbox counts", "business_id", businessID)

	// Call service
	counts, err := h.voucherService.GetApprovalInboxCounts(c.Request().Context(), businessID)
	if err != nil {
		h.logger.ErrorWith("Failed to get approval inbox counts", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Approval inbox counts retrieved successfully",
		"pending", counts.Pending,
		"approved", counts.Approved,
		"rejected", counts.Rejected,
		"request_changes", counts.RequestChanges)

	return c.JSON(http.StatusOK, counts)
}

// BatchApproveVouchers approves multiple vouchers in a single operation
func (h *VoucherHandler) BatchApproveVouchers(c echo.Context) error {
	h.logger.InfoWith("BatchApproveVouchers API called")

	// Extract user_id from JWT context
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	// Extract business_id from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Parse request body
	var reqBody struct {
		VoucherIDs   []string `json:"voucher_ids" validate:"required,min=1"`
		ApprovedByID string   `json:"approved_by_id" validate:"required,uuid"`
		Remarks      string   `json:"remarks"`
	}

	if err := c.Bind(&reqBody); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_REQUEST",
			"message": "Invalid request body",
		})
	}

	// Validate voucher IDs
	if len(reqBody.VoucherIDs) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_VOUCHER_IDS",
			"message": "At least one voucher_id is required",
		})
	}

	h.logger.DebugWith("Processing batch approval",
		"total_vouchers", len(reqBody.VoucherIDs),
		"approved_by", reqBody.ApprovedByID,
		"requester", userID)

	// Create service request
	serviceReq := &voucher.BatchApprovalRequest{
		VoucherIDs:   reqBody.VoucherIDs,
		ApprovedByID: reqBody.ApprovedByID,
		RequesterID:  userID,
		BusinessID:   businessID,
		Remarks:      reqBody.Remarks,
	}

	// Call service
	response, err := h.voucherService.BatchApproveVouchers(c.Request().Context(), serviceReq)
	if err != nil {
		h.logger.ErrorWith("Failed to batch approve vouchers", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Batch approval completed",
		"total_requested", response.TotalRequested,
		"total_approved", response.TotalApproved,
		"total_failed", response.TotalFailed)

	return c.JSON(http.StatusOK, response)
}

// GetUnpaidBills retrieves unpaid bills with role-based filtering
func (h *VoucherHandler) GetUnpaidBills(c echo.Context) error {
	h.logger.InfoWith("GetUnpaidBills API called")

	// Extract business_id from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Get user roles from middleware context
	ur := c.Get("userRoles")
	if ur == nil {
		return c.JSON(http.StatusForbidden, map[string]any{
			"error":   "FORBIDDEN",
			"message": "No business association found",
		})
	}
	roles := ur.([]*business.BusinessUserRelation)

	// Find the role for the requested business
	var isOwner bool
	for _, rel := range roles {
		if rel.BID == businessID {
			isOwner = (string(rel.Role) == "owner")
			break
		}
	}

	// Parse optional query parameters for pagination
	limit := 50 // Default limit
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	h.logger.DebugWith("Fetching unpaid bills",
		"business_id", businessID,
		"is_owner", isOwner,
		"limit", limit,
		"offset", offset)

	// Create service request
	serviceReq := &voucher.UnpaidBillsRequest{
		BusinessID: businessID,
		Limit:      limit,
		Offset:     offset,
	}

	// Call service
	response, err := h.voucherService.GetUnpaidBills(c.Request().Context(), serviceReq, isOwner)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch unpaid bills", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Unpaid bills fetched successfully",
		"total_count", response.TotalCount,
		"returned_count", len(response.Bills))

	return c.JSON(http.StatusOK, response)
}

// GetVoucherCounts retrieves counts of vouchers by payment status
func (h *VoucherHandler) GetVoucherCounts(c echo.Context) error {
	h.logger.InfoWith("GetVoucherCounts API called")

	// Extract business_id from header
	businessID := c.Request().Header.Get("X-Business-ID")
	if businessID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	// Call service
	counts, err := h.voucherService.GetVoucherCounts(c.Request().Context(), businessID)
	if err != nil {
		h.logger.ErrorWith("Failed to get voucher counts", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Voucher counts fetched successfully",
		"all", counts.All,
		"paid", counts.Paid,
		"open", counts.Open,
		"overdue", counts.Overdue)

	return c.JSON(http.StatusOK, counts)
}

// GetVoucherActivity handles GET /vouchers/:id/activity - Get voucher activity history
func (h *VoucherHandler) GetVoucherActivity(c echo.Context) error {
	h.logger.InfoWith("GetVoucherActivity API called")

	// Extract voucher_id from path parameter
	voucherID := c.Param("id")
	if voucherID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_VOUCHER_ID",
			"message": "voucher_id path parameter is required",
		})
	}

	// Validate UUID format
	if _, err := uuid.Parse(voucherID); err != nil {
		h.logger.ErrorWith("Invalid voucher_id format", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_VOUCHER_ID",
			"message": "voucher_id must be a valid UUID",
		})
	}

	// Call service
	activity, err := h.voucherService.GetVoucherActivity(c.Request().Context(), voucherID)
	if err != nil {
		h.logger.ErrorWith("Failed to get voucher activity", "voucher_id", voucherID, "error", err)

		// Check if voucher not found
		if err.Error() == fmt.Sprintf("voucher not found: %s", voucherID) {
			return c.JSON(http.StatusNotFound, map[string]any{
				"error":   "VOUCHER_NOT_FOUND",
				"message": "Voucher not found",
			})
		}

		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Voucher activity fetched successfully",
		"voucher_id", voucherID,
		"voucher_number", activity.VoucherNumber,
		"total_activities", activity.TotalCount)

	return c.JSON(http.StatusOK, activity)
}

// GetVoucherPaymentDetails handles GET /vouchers/:id/payment - Get payment details for voucher
func (h *VoucherHandler) GetVoucherPaymentDetails(c echo.Context) error {
	h.logger.InfoWith("GetVoucherPaymentDetails API called")

	// Extract voucher_id from path parameter
	voucherID := c.Param("id")
	if voucherID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_VOUCHER_ID",
			"message": "voucher_id path parameter is required",
		})
	}

	// Validate UUID format
	if _, err := uuid.Parse(voucherID); err != nil {
		h.logger.ErrorWith("Invalid voucher_id format", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "INVALID_VOUCHER_ID",
			"message": "voucher_id must be a valid UUID",
		})
	}

	// Call service
	paymentDetails, err := h.voucherService.GetVoucherPaymentDetails(c.Request().Context(), voucherID)
	if err != nil {
		h.logger.ErrorWith("Failed to get voucher payment details", "voucher_id", voucherID, "error", err)

		// Check for specific errors
		errorMsg := err.Error()
		if errorMsg == fmt.Sprintf("payment not found for voucher: %s", voucherID) {
			return c.JSON(http.StatusNotFound, map[string]any{
				"error":   "PAYMENT_NOT_FOUND",
				"message": "No payment linked to this voucher. The voucher may still be in draft status.",
			})
		}

		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "SERVICE_ERROR",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Voucher payment details fetched successfully",
		"voucher_id", voucherID,
		"payment_id", paymentDetails.PaymentID,
		"payment_status", paymentDetails.PaymentStatus,
		"total_amount_paid", paymentDetails.TotalAmountPaid)

	return c.JSON(http.StatusOK, paymentDetails)
}
