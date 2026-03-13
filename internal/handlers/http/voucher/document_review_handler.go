package voucher

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/voucher"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/logger"
)

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

// GetDocumentDraft handles GET /vouchers/:document_id/draft
// Returns the extracted draft data for human review
func (h *DocumentReviewHandler) GetDocumentDraft(c echo.Context) error {
	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	doc, err := h.reviewService.GetDocumentDraft(c.Request().Context(), docID)
	if err != nil {
		h.logger.ErrorWith("GetDocumentDraft failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"document_id":     doc.ID,
			"doc_type":        doc.DocType,
			"document_status": doc.Status,
			"draft_data":      doc.DraftData,
			"validated_data":  doc.ValidatedData,
			"processed_by":    doc.ProcessedBy,
		},
	})
}

// GetDocumentStatus handles GET /vouchers/:document_id/status
// Returns the current processing status of a document
func (h *DocumentReviewHandler) GetDocumentStatus(c echo.Context) error {
	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	status, err := h.reviewService.GetDocumentStatus(c.Request().Context(), docID)
	if err != nil {
		h.logger.ErrorWith("GetDocumentStatus failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": err.Error(),
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
	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
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

	if err := h.reviewService.ValidateDocument(c.Request().Context(), docID, &req); err != nil {
		h.logger.ErrorWith("ValidateDocument failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{
			"status":  "error",
			"error":   "VALIDATION_FAILED",
			"message": err.Error(),
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

// ListDocuments handles GET /vouchers/documents
func (h *DocumentReviewHandler) ListDocuments(c echo.Context) error {
	filter := voucher.DocumentListFilter{
		Page:     h.getQueryParamInt(c, "page", 1),
		PageSize: h.getQueryParamInt(c, "limit", 20),
	}

	if status := c.QueryParam("status"); status != "" {
		docStatus := voucher.DocumentStatus(status)
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
	filter := voucher.ExtractedInvoiceListFilter{
		Page:     h.getQueryParamInt(c, "page", 1),
		PageSize: h.getQueryParamInt(c, "limit", 20),
	}

	if bid := c.QueryParam("business_id"); bid != "" {
		if u, err := uuid.Parse(bid); err == nil {
			filter.BusinessID = &u
		}
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
	filter := voucher.PurchaseOrderListFilter{
		Page:     h.getQueryParamInt(c, "page", 1),
		PageSize: h.getQueryParamInt(c, "limit", 20),
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

func (h *DocumentReviewHandler) getQueryParamInt(c echo.Context, name string, defaultValue int) int {
	val := c.QueryParam(name)
	if val == "" {
		return defaultValue
	}
	var i int
	if _, err := fmt.Sscanf(val, "%d", &i); err != nil {
		return defaultValue
	}
	return i
}
