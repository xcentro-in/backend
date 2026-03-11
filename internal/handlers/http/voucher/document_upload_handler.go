package voucher

import (
	"net/http"

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

	// 1. Extract file from multipart form
	file, err := c.FormFile("file")
	if err != nil {
		h.logger.ErrorWith("Failed to read uploaded file", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_FILE",
			"message": "A file must be provided in the 'file' form field",
		})
	}

	// 2. Read optional doc_type form value (defaults to UNKNOWN if not provided)
	docType := c.FormValue("doc_type")

	h.logger.DebugWith("Upload request received",
		"filename", file.Filename,
		"size", file.Size,
		"content_type", file.Header.Get("Content-Type"),
		"doc_type", docType)

	// 3. Build service request
	req := &voucherSvc.UploadDocumentRequest{
		File:    file,
		DocType: docType,
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
