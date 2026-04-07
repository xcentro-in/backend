package voucher

import (
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

// UploadDocument handles POST /vouchers/upload
// Accepts multipart form data with a "file" field and optional "doc_type" field
func (h *DocumentUploadHandler) UploadDocument(c echo.Context) error {
	h.logger.InfoWith("UploadDocument API called")

	// 1. Extract business_id from X-Business-ID header (optional for violation tracking)
	businessIDStr := c.Request().Header.Get("X-Business-ID")
	var businessID *uuid.UUID
	if businessIDStr != "" {
		parsed, err := uuid.Parse(businessIDStr)
		if err != nil {
			h.logger.WarnWith("Invalid X-Business-ID header, ignoring", "error", err)
		} else {
			businessID = &parsed
		}
	}

	// 2. Extract file from multipart form
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.ErrorWith("Failed to read uploaded file", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_FILE",
			"message": "A file must be provided in the 'file' form field",
		})
	}

	// 3. Read optional doc_type form value (defaults to UNKNOWN if not provided)
	docType := c.FormValue("doc_type")

	h.logger.DebugWith("Upload request received",
		"filename", file.Filename,
		"size", file.Size,
		"content_type", file.Header.Get("Content-Type"),
		"doc_type", docType,
		"business_id", businessIDStr)

	// 4. Build service request
	req := &voucherSvc.UploadDocumentRequest{
		File:       file,
		DocType:    docType,
		BusinessID: businessID,
	}

	// 4. Call service
	resp, err := h.uploadService.UploadDocument(c.Request().Context(), req)
	if err != nil {
		h.logger.ErrorWith("UploadDocument service call failed", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "UPLOAD_FAILED",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Document uploaded successfully",
		"document_id", resp.DocumentID,
		"s3_path", resp.S3Path)

	return c.JSON(http.StatusCreated, map[string]any{
		"message": "Document uploaded and queued for extraction",
		"status":  "success",
		"data":    resp,
	})
}

// GetDocumentURL handles GET /vouchers/:document_id/url
// Returns a short-lived presigned S3 URL for viewing the document file
func (h *DocumentUploadHandler) GetDocumentURL(c echo.Context) error {
	docIDStr := c.Param("document_id")
	docID, err := uuid.Parse(docIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"error":   "INVALID_DOCUMENT_ID",
			"message": "Invalid document ID format",
		})
	}

	url, err := h.uploadService.GetPresignedURL(c.Request().Context(), docID)
	if err != nil {
		h.logger.ErrorWith("GetDocumentURL failed", "error", err, "doc_id", docID)
		return c.JSON(http.StatusNotFound, map[string]any{
			"status":  "error",
			"error":   "DOCUMENT_NOT_FOUND",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   map[string]any{"url": url},
	})
}
