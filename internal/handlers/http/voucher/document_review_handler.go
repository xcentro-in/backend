package voucher

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/voucher"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/logger"
)

// Pagination bounds shared by every list endpoint on this handler.
const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// validDocumentStatuses enumerates the legal values for the ?status= filter.
// Keeping it a map so handlers can reject unknown input before it hits SQL.
var validDocumentStatuses = map[voucher.DocumentStatus]bool{
	voucher.DocStatusQueued:         true,
	voucher.DocStatusOCRProcessing:  true,
	voucher.DocStatusPendingLLM:     true,
	voucher.DocStatusLLMProcessing:  true,
	voucher.DocStatusReviewRequired: true,
	voucher.DocStatusValidated:      true,
	voucher.DocStatusFailed:         true,
}

// DocumentReviewHandler handles HTTP requests for the HITL document review flow
type DocumentReviewHandler struct {
	reviewService *voucherSvc.DocumentReviewService
	logger        *logger.Logger
}

// NewDocumentReviewHandler creates a new DocumentReviewHandler
func NewDocumentReviewHandler(reviewService *voucherSvc.DocumentReviewService, logger *logger.Logger) *DocumentReviewHandler {
	return &DocumentReviewHandler{
		reviewService: reviewService,
		logger:        logger,
	}
}

// parseDocID parses the :document_id path param or writes a 400 and returns false.
func parseDocID(c echo.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("document_id"))
	if err != nil {
		_ = c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
		return uuid.Nil, false
	}
	return id, true
}

// paginationFromQuery extracts page/limit and clamps them to sane bounds so
// a malicious or buggy client can't pull the whole table with ?limit=100000.
func paginationFromQuery(c echo.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ = strconv.Atoi(c.QueryParam("limit"))
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func forbidden(c echo.Context) error {
	return c.JSON(http.StatusForbidden, map[string]any{
		"status":  "error",
		"error":   "ACCESS_DENIED",
		"message": "Document does not belong to this business",
	})
}

// GetDocumentDraft handles GET /vouchers/:document_id/draft
// Returns the extracted draft data for human review
func (h *DocumentReviewHandler) GetDocumentDraft(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	docID, ok := parseDocID(c)
	if !ok {
		return nil
	}

	if err := h.reviewService.AssertDocumentOwnership(c.Request().Context(), docID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("GetDocumentDraft ownership check failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	result, err := h.reviewService.GetDocumentDraftWithResolution(c.Request().Context(), docID, businessID.String())
	if err != nil {
		h.logger.ErrorWith("GetDocumentDraft failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	doc := result.Document
	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"document_id":         doc.ID,
			"doc_type":            doc.DocType,
			"document_status":     doc.Status,
			"draft_data":          doc.DraftData,
			"validated_data":      doc.ValidatedData,
			"processed_by":        doc.ProcessedBy,
			"payee_resolution":    result.PayeeResolution,
			"business_resolution": result.BusinessResolution,
		},
	})
}

// GetDocumentStatus handles GET /vouchers/:document_id/status
// Returns the current processing status of a document
func (h *DocumentReviewHandler) GetDocumentStatus(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	docID, ok := parseDocID(c)
	if !ok {
		return nil
	}

	if err := h.reviewService.AssertDocumentOwnership(c.Request().Context(), docID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("GetDocumentStatus ownership check failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	status, err := h.reviewService.GetDocumentStatus(c.Request().Context(), docID)
	if err != nil {
		h.logger.ErrorWith("GetDocumentStatus failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"document_status": status,
		},
	})
}

// ValidateDocument handles POST /vouchers/:document_id/validate
// Accepts corrected fields from the user and marks the document as VALIDATED
func (h *DocumentReviewHandler) ValidateDocument(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	docID, ok := parseDocID(c)
	if !ok {
		return nil
	}

	if err := h.reviewService.AssertDocumentOwnership(c.Request().Context(), docID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("ValidateDocument ownership check failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	var req voucherSvc.ValidateDocumentRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_REQUEST_BODY",
			"message": "Failed to parse request body",
		})
	}

	if req.DraftData == nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "MISSING_DRAFT_DATA",
			"message": "draft_data field is required",
		})
	}

	// Stamp the authenticated user and business from JWT — never trust client-supplied values.
	req.UserID, _ = c.Get("userId").(string)
	req.DraftData["business_id"] = businessID.String()

	if err := h.reviewService.ValidateDocument(c.Request().Context(), docID, &req); err != nil {
		h.logger.ErrorWith("ValidateDocument failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{
			"status":  "error",
			"error":   "VALIDATION_FAILED",
			"message": "Failed to validate document",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Document validated successfully",
		"data": map[string]any{
			"document_status": "VALIDATED",
		},
	})
}

// GetInvoiceLineItems handles GET /vouchers/invoices/:invoice_id/line-items
// Returns typed line items for a given extracted invoice.
func (h *DocumentReviewHandler) GetInvoiceLineItems(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	invoiceIDStr := c.Param("invoice_id")
	invoiceID, err := uuid.Parse(invoiceIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_INVOICE_ID",
			"message": "Invalid invoice ID format",
		})
	}

	if err := h.reviewService.AssertInvoiceOwnership(c.Request().Context(), invoiceID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("GetInvoiceLineItems ownership check failed", "error", err, "invoice_id", invoiceID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "INVOICE_NOT_FOUND",
			"message": "Invoice not found",
		})
	}

	items, err := h.reviewService.GetInvoiceLineItems(c.Request().Context(), invoiceID)
	if err != nil {
		h.logger.ErrorWith("GetInvoiceLineItems failed", "error", err, "invoice_id", invoiceID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to retrieve line items",
		})
	}

	type lineItemResponse struct {
		ID          string  `json:"id"`
		SKU         string  `json:"sku"`
		Description string  `json:"description"`
		Quantity    float64 `json:"quantity"`
		UnitPrice   float64 `json:"unit_price"`
		TaxRate     float64 `json:"tax_rate"`
		TaxAmount   float64 `json:"tax_amount"`
		TotalAmount float64 `json:"total_amount"`
	}
	resp := make([]lineItemResponse, len(items))
	for i, item := range items {
		resp[i] = lineItemResponse{
			ID:          item.ID.String(),
			SKU:         item.SKU,
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
			TaxRate:     item.TaxRate,
			TaxAmount:   item.TaxAmount,
			TotalAmount: item.TotalAmount,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   resp,
	})
}

// GetPurchaseOrderLineItems handles GET /vouchers/purchase-orders/:po_id/line-items
// Returns typed line items for a given purchase order.
func (h *DocumentReviewHandler) GetPurchaseOrderLineItems(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	poIDStr := c.Param("po_id")
	poID, err := uuid.Parse(poIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_PO_ID",
			"message": "Invalid purchase order ID format",
		})
	}

	if err := h.reviewService.AssertPOOwnership(c.Request().Context(), poID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("GetPurchaseOrderLineItems ownership check failed", "error", err, "po_id", poID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "PO_NOT_FOUND",
			"message": "Purchase order not found",
		})
	}

	items, err := h.reviewService.GetPurchaseOrderLineItems(c.Request().Context(), poID)
	if err != nil {
		h.logger.ErrorWith("GetPurchaseOrderLineItems failed", "error", err, "po_id", poID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to retrieve line items",
		})
	}

	type lineItemResponse struct {
		ID          string  `json:"id"`
		SKU         string  `json:"sku"`
		Description string  `json:"description"`
		Quantity    float64 `json:"quantity"`
		QtyReceived float64 `json:"qty_received"`
		UnitPrice   float64 `json:"unit_price"`
		TaxRate     float64 `json:"tax_rate"`
		TaxAmount   float64 `json:"tax_amount"`
		TotalAmount float64 `json:"total_amount"`
	}
	resp := make([]lineItemResponse, len(items))
	for i, item := range items {
		resp[i] = lineItemResponse{
			ID:          item.ID.String(),
			SKU:         item.SKU,
			Description: item.Description,
			Quantity:    item.Quantity,
			QtyReceived: item.QtyReceived,
			UnitPrice:   item.UnitPrice,
			TaxRate:     item.TaxRate,
			TaxAmount:   item.TaxAmount,
			TotalAmount: item.TotalAmount,
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   resp,
	})
}

// GetMatchResult handles GET /vouchers/invoices/:invoice_id/match-result
// Returns the deterministic matching outcome for a given invoice
func (h *DocumentReviewHandler) GetMatchResult(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	invoiceIDStr := c.Param("invoice_id")
	invoiceID, err := uuid.Parse(invoiceIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_INVOICE_ID",
			"message": "Invalid invoice ID format",
		})
	}

	if err := h.reviewService.AssertInvoiceOwnership(c.Request().Context(), invoiceID, businessID); err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			return forbidden(c)
		}
		h.logger.ErrorWith("GetMatchResult ownership check failed", "error", err, "invoice_id", invoiceID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "INVOICE_NOT_FOUND",
			"message": "Invoice not found",
		})
	}

	results, err := h.reviewService.GetMatchResult(c.Request().Context(), invoiceID)
	if err != nil {
		h.logger.ErrorWith("GetMatchResult failed", "error", err, "invoice_id", invoiceID)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to retrieve match results",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   results,
	})
}

// ListMatchResults handles GET /vouchers/match-results
// Returns paginated joined view of match_results + invoice + purchase_orders for a business.
func (h *DocumentReviewHandler) ListMatchResults(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	page, pageSize := paginationFromQuery(c)

	bid := businessID
	filter := voucher.MatchResultListFilter{
		BusinessID: &bid,
		Page:       page,
		PageSize:   pageSize,
	}

	res, err := h.reviewService.ListMatchResults(c.Request().Context(), filter)
	if err != nil {
		h.logger.ErrorWith("ListMatchResults failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to list match results",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   res,
	})
}

// ListDocuments handles GET /vouchers/documents
func (h *DocumentReviewHandler) ListDocuments(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	page, pageSize := paginationFromQuery(c)

	bid := businessID
	filter := voucher.DocumentListFilter{
		BusinessID: &bid,
		Page:       page,
		PageSize:   pageSize,
	}

	if raw := c.QueryParam("status"); raw != "" {
		docStatus := voucher.DocumentStatus(raw)
		if !validDocumentStatuses[docStatus] {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"status":  "error",
				"error":   "INVALID_STATUS",
				"message": "status must be one of QUEUED, OCR_PROCESSING, PENDING_LLM, LLM_PROCESSING, REVIEW_REQUIRED, VALIDATED, FAILED",
			})
		}
		filter.Status = &docStatus
	}

	res, err := h.reviewService.ListDocuments(c.Request().Context(), filter)
	if err != nil {
		h.logger.ErrorWith("ListDocuments failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to list documents",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   res,
	})
}

// ListInvoices handles GET /vouchers/invoices
func (h *DocumentReviewHandler) ListInvoices(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	page, pageSize := paginationFromQuery(c)

	bid := businessID
	filter := voucher.ExtractedInvoiceListFilter{
		BusinessID: &bid,
		Page:       page,
		PageSize:   pageSize,
	}

	res, err := h.reviewService.ListInvoices(c.Request().Context(), filter)
	if err != nil {
		h.logger.ErrorWith("ListInvoices failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to list invoices",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   res,
	})
}

// ListPurchaseOrders handles GET /vouchers/purchase-orders
func (h *DocumentReviewHandler) ListPurchaseOrders(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}
	page, pageSize := paginationFromQuery(c)

	bid := businessID
	filter := voucher.PurchaseOrderListFilter{
		BusinessID: &bid,
		Page:       page,
		PageSize:   pageSize,
	}

	if docID := c.QueryParam("document_id"); docID != "" {
		if u, err := uuid.Parse(docID); err == nil {
			filter.DocumentID = &u
		}
	}

	res, err := h.reviewService.ListPurchaseOrders(c.Request().Context(), filter)
	if err != nil {
		h.logger.ErrorWith("ListPurchaseOrders failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "INTERNAL_SERVER_ERROR",
			"message": "Failed to list purchase orders",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   res,
	})
}

// QuickCreateVendor handles POST /vouchers/vendors/quick-create
// Creates a minimal vendor payee for the document review context (no bank account / GSTIN verification).
func (h *DocumentReviewHandler) QuickCreateVendor(c echo.Context) error {
	userID, ok := c.Get("userId").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"status":  "error",
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	var body struct {
		DisplayName  string `json:"display_name"`
		GSTIN        string `json:"gstin"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_REQUEST_BODY",
			"message": "Failed to parse request body",
		})
	}
	if body.DisplayName == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "MISSING_DISPLAY_NAME",
			"message": "display_name is required",
		})
	}

	resp, err := h.reviewService.QuickCreateVendor(c.Request().Context(), voucherSvc.QuickCreateVendorRequest{
		BusinessID:   businessID.String(),
		AddedBy:      userID,
		DisplayName:  body.DisplayName,
		GSTIN:        body.GSTIN,
		ContactEmail: body.ContactEmail,
		ContactPhone: body.ContactPhone,
	})
	if err != nil {
		h.logger.ErrorWith("QuickCreateVendor failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"error":   "CREATE_FAILED",
			"message": "Failed to create vendor",
		})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"status": "success",
		"data": map[string]any{
			"payee_id":    resp.PayeeID,
			"relation_id": resp.RelationID,
		},
	})
}
