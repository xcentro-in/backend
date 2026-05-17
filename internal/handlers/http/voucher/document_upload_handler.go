package voucher

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/logger"
)

// DocumentUploadHandler handles HTTP requests for voucher document uploads
type DocumentUploadHandler struct {
	uploadService *voucherSvc.DocumentUploadService
	logger        *logger.Logger
}

// NewDocumentUploadHandler creates a new DocumentUploadHandler
func NewDocumentUploadHandler(uploadService *voucherSvc.DocumentUploadService, logger *logger.Logger) *DocumentUploadHandler {
	return &DocumentUploadHandler{
		uploadService: uploadService,
		logger:        logger,
	}
}

// businessIDFromContext extracts the authenticated business UUID set by
// JWTMiddleware via the "bid" claim. Missing or malformed values are treated as a 401.
func businessIDFromContext(c echo.Context) (uuid.UUID, error) {
	raw := c.Get("bid")
	s, ok := raw.(string)
	if !ok || s == "" {
		return uuid.Nil, errors.New("business context missing")
	}
	return uuid.Parse(s)
}

func unauthorizedBusiness(c echo.Context) error {
	return c.JSON(http.StatusUnauthorized, map[string]any{
		"status":  "error",
		"error":   "BUSINESS_CONTEXT_MISSING",
		"message": "Authenticated business context is required",
	})
}

// UploadDocument handles POST /vouchers/upload
// Accepts multipart form data with a "file" field and optional "doc_type" field
func (h *DocumentUploadHandler) UploadDocument(c echo.Context) error {
	h.logger.InfoWith("UploadDocument API called")

	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	file, err := c.FormFile("file")
	if err != nil {
		h.logger.ErrorWith("Failed to read uploaded file", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_FILE",
			"message": "A file must be provided in the 'file' form field",
		})
	}

	docType := c.FormValue("doc_type")

	h.logger.DebugWith("Upload request received",
		"filename", file.Filename,
		"size", file.Size,
		"content_type", file.Header.Get("Content-Type"),
		"doc_type", docType,
		"business_id", businessID)

	req := &voucherSvc.UploadDocumentRequest{
		File:       file,
		DocType:    docType,
		BusinessID: businessID,
	}

	resp, err := h.uploadService.UploadDocument(c.Request().Context(), req)
	if err != nil {
		if errors.Is(err, voucherSvc.ErrDailyUploadQuotaExceeded) {
			return c.JSON(http.StatusTooManyRequests, map[string]any{
				"error":   "QUOTA_EXCEEDED",
				"message": "Daily document upload limit reached. Try again tomorrow.",
			})
		}
		h.logger.ErrorWith("UploadDocument service call failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "UPLOAD_FAILED",
			"message": "Failed to upload document",
		})
	}

	h.logger.InfoWith("Document uploaded successfully",
		"document_id", resp.DocumentID,
		"s3_path", resp.S3Path,
		"is_duplicate", resp.IsDuplicate)

	if resp.IsDuplicate {
		return c.JSON(http.StatusOK, map[string]any{
			"message": "Duplicate document — existing record returned",
			"status":  "success",
			"data":    resp,
		})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Document uploaded and queued for extraction",
		"status":  "success",
		"data":    resp,
	})
}

// respondOwnershipError converts an ErrDocumentNotOwned into a 403, and any
// other error into a generic 400. Internal error details stay in the log.
func (h *DocumentUploadHandler) respondOwnershipError(c echo.Context, err error, op, code string, docID uuid.UUID) error {
	if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
		h.logger.WarnWith(op+": ownership check failed", "doc_id", docID)
		return c.JSON(http.StatusForbidden, map[string]any{
			"status":  "error",
			"error":   "ACCESS_DENIED",
			"message": "Document does not belong to this business",
		})
	}
	h.logger.ErrorWith(op+" failed", "error", err, "doc_id", docID)
	return c.JSON(http.StatusBadRequest, map[string]any{
		"status":  "error",
		"error":   code,
		"message": op + " failed",
	})
}

// RetryDocument handles POST /vouchers/:document_id/retry
// Resets a FAILED document to QUEUED and re-enqueues it for extraction
func (h *DocumentUploadHandler) RetryDocument(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	if err := h.uploadService.RetryDocument(c.Request().Context(), docID, businessID); err != nil {
		return h.respondOwnershipError(c, err, "RetryDocument", "RETRY_FAILED", docID)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Document re-queued for extraction",
	})
}

// DeleteDocument handles DELETE /vouchers/:document_id
// Deletes a FAILED document from the database and S3
func (h *DocumentUploadHandler) DeleteDocument(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	if err := h.uploadService.DeleteDocument(c.Request().Context(), docID, businessID); err != nil {
		return h.respondOwnershipError(c, err, "DeleteDocument", "DELETE_FAILED", docID)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":  "success",
		"message": "Document deleted successfully",
	})
}

// GetDocumentURL handles GET /vouchers/:document_id/url
// Returns a short-lived presigned S3 URL for viewing the document file
func (h *DocumentUploadHandler) GetDocumentURL(c echo.Context) error {
	businessID, err := businessIDFromContext(c)
	if err != nil {
		return unauthorizedBusiness(c)
	}

	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	url, err := h.uploadService.GetPresignedURL(c.Request().Context(), docID, businessID)
	if err != nil {
		if errors.Is(err, voucherSvc.ErrDocumentNotOwned) {
			h.logger.WarnWith("GetDocumentURL: ownership check failed", "doc_id", docID)
			return c.JSON(http.StatusForbidden, map[string]any{
				"status":  "error",
				"error":   "ACCESS_DENIED",
				"message": "Document does not belong to this business",
			})
		}
		h.logger.ErrorWith("GetDocumentURL failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": "Document not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   map[string]any{"url": url},
	})
}
