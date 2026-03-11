package voucher

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
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
