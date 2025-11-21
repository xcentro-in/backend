package batch_payment

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/services/batch_payment"
	"xcentro.in/internal/shared/logger"
)

// BatchPaymentHandler handles batch payment HTTP requests
type BatchPaymentHandler struct {
	batchService *batch_payment.BatchPaymentService
	s3Client     *fileupload.S3Client
	logger       *logger.Logger
}

// NewBatchPaymentHandler creates a new batch payment handler
func NewBatchPaymentHandler(batchService *batch_payment.BatchPaymentService, s3Client *fileupload.S3Client) *BatchPaymentHandler {
	return &BatchPaymentHandler{
		batchService: batchService,
		s3Client:     s3Client,
		logger:       logger.GetDefault(),
	}
}

// UploadCSV handles POST /batch_payments/upload - Upload CSV and get preview
func (h *BatchPaymentHandler) UploadCSV(c echo.Context) error {
	h.logger.InfoWith("UploadCSV API called")

	// Get business ID from header
	businessIDStr := c.Request().Header.Get("X-Business-ID")
	if businessIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_business_id",
			"message": "Invalid business_id format",
		})
	}

	// Get user ID from JWT
	userIDStr, ok := c.Get("userId").(string)
	if !ok || userIDStr == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "invalid_user_id",
			"message": "Invalid user ID format",
		})
	}

	// Get uploaded file
	file, err := c.FormFile("csv_file")
	if err != nil {
		h.logger.ErrorWith("Failed to get uploaded file", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_file",
			"message": "CSV file is required",
		})
	}

	// Validate file type
	if file.Header.Get("Content-Type") != "text/csv" && !isSupportedFileExtension(file.Filename) {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_file_type",
			"message": "Only CSV files are supported",
		})
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		h.logger.ErrorWith("Failed to open uploaded file", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "file_error",
			"message": "Failed to process uploaded file",
		})
	}
	defer src.Close()

	h.logger.InfoWith("Processing CSV", "filename", file.Filename, "size", file.Size)

	// Parse and preview
	preview, err := h.batchService.ParseAndPreviewCSV(c.Request().Context(), businessID, userID, file.Filename, src)
	if err != nil {
		h.logger.ErrorWith("Failed to parse CSV", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "csv_parse_error",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("CSV preview generated",
		"batch_id", preview.BatchID,
		"total_rows", preview.TotalRows,
		"matched", preview.MatchedCount,
		"ambiguous", preview.AmbiguousCount,
		"not_found", preview.NotFoundCount)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "CSV uploaded and processed successfully",
		"status":  "success",
		"data":    preview,
	})
}

// ProcessBatch handles POST /batch_payments/:batch_id/process - Process batch payments
func (h *BatchPaymentHandler) ProcessBatch(c echo.Context) error {
	h.logger.InfoWith("ProcessBatch API called")

	// Get business ID from header
	businessIDStr := c.Request().Header.Get("X-Business-ID")
	if businessIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_business_id",
			"message": "Invalid business_id format",
		})
	}

	// Get user ID from JWT
	userIDStr, ok := c.Get("userId").(string)
	if !ok || userIDStr == "" {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "MISSING_USER_ID",
			"message": "User ID not found in token",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{
			"error":   "invalid_user_id",
			"message": "Invalid user ID format",
		})
	}

	// Parse batch ID from path
	batchIDStr := c.Param("batch_id")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_batch_id",
			"message": "Invalid batch ID format",
		})
	}

	// Parse request body
	var req batch_payment.ProcessRequest
	if err := c.Bind(&req); err != nil {
		h.logger.ErrorWith("Error binding request", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "Invalid request body",
		})
	}

	req.BatchID = batchID

	h.logger.DebugWith("Process batch request",
		"batch_id", batchID,
		"ambiguous_mappings", len(req.AmbiguousMappings),
		"skip_rows", len(req.SkipRows))

	// Process batch (creates draft vouchers)
	response, err := h.batchService.ProcessBatch(c.Request().Context(), businessID, userID, &req)
	if err != nil {
		h.logger.ErrorWith("Failed to process batch", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "process_error",
			"message": err.Error(),
		})
	}

	h.logger.InfoWith("Batch vouchers created", "batch_id", batchID, "vouchers_created", response.ProcessedItems)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Batch processed successfully",
		"status":  "success",
		"data":    response,
	})
}

// GetBatchStatus handles GET /batch_payments/:batch_id - Get batch status
func (h *BatchPaymentHandler) GetBatchStatus(c echo.Context) error {
	h.logger.InfoWith("GetBatchStatus API called")

	// Parse batch ID from path
	batchIDStr := c.Param("batch_id")
	batchID, err := uuid.Parse(batchIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_batch_id",
			"message": "Invalid batch ID format",
		})
	}

	// Get batch status
	status, err := h.batchService.GetBatchStatus(c.Request().Context(), batchID)
	if err != nil {
		h.logger.ErrorWith("Failed to get batch status", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "status_error",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Batch status retrieved successfully",
		"status":  "success",
		"data":    status,
	})
}

// GetBatchList handles GET /batch_payments - Get list of batches
func (h *BatchPaymentHandler) GetBatchList(c echo.Context) error {
	h.logger.InfoWith("GetBatchList API called")

	// Get business ID from header
	businessIDStr := c.Request().Header.Get("X-Business-ID")
	if businessIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "MISSING_BUSINESS_ID",
			"message": "X-Business-ID header is required",
		})
	}

	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "invalid_business_id",
			"message": "Invalid business_id format",
		})
	}

	// Get pagination parameters
	page := 1
	pageSize := 20

	if pageStr := c.QueryParam("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr := c.QueryParam("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Get batch list
	list, err := h.batchService.GetBusinessBatches(c.Request().Context(), businessID, page, pageSize)
	if err != nil {
		h.logger.ErrorWith("Failed to get batch list", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "list_error",
			"message": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Batch list retrieved successfully",
		"status":  "success",
		"data":    list,
	})
}

// DownloadSampleCSV handles GET /batch_payments/sample_csv - Download sample CSV template
func (h *BatchPaymentHandler) DownloadSampleCSV(c echo.Context) error {
	h.logger.InfoWith("DownloadSampleCSV API called")

	// Check if S3 client is available
	if h.s3Client == nil {
		h.logger.Error("S3 client not configured for batch payment handler")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "service_unavailable",
			"message": "File download service is not available",
		})
	}

	// TODO: Replace this placeholder with the actual S3 key for your sample CSV file
	// Example: "templates/batch_payment_sample.csv" or "batch-payments/samples/template.csv"
	const sampleCSVS3Key = "PLACEHOLDER_S3_KEY_FOR_SAMPLE_CSV"

	// Generate presigned download URL with 15 minute expiration
	presignedURL, err := h.s3Client.GetPresignedDownloadURL(c.Request().Context(), sampleCSVS3Key, 15*time.Minute)
	if err != nil {
		h.logger.ErrorWith("Failed to generate presigned URL for sample CSV", "error", err, "s3_key", sampleCSVS3Key)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "url_generation_failed",
			"message": "Failed to generate download URL",
		})
	}

	h.logger.InfoWith("Sample CSV presigned URL generated successfully", "s3_key", sampleCSVS3Key)

	return c.JSON(http.StatusOK, map[string]any{
		"message": "Sample CSV download URL generated successfully",
		"status":  "success",
		"data": map[string]any{
			"download_url": presignedURL,
			"expires_in":   900, // 15 minutes in seconds
			"filename":     "batch_payment_sample.csv",
		},
	})
}

// isSupportedFileExtension checks if the file extension is supported
func isSupportedFileExtension(filename string) bool {
	supportedExtensions := []string{".csv", ".CSV"}
	for _, ext := range supportedExtensions {
		if len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext {
			return true
		}
	}
	return false
}

// handleError converts service errors to HTTP responses
func (h *BatchPaymentHandler) handleError(c echo.Context, err error) error {
	h.logger.ErrorWith("Batch payment error", "error", err)

	// Default error
	statusCode := http.StatusInternalServerError
	errorCode := "internal_error"
	message := "An internal error occurred"

	// Check for specific error types
	errStr := err.Error()
	if contains(errStr, "not found") {
		statusCode = http.StatusNotFound
		errorCode = "not_found"
		message = err.Error()
	} else if contains(errStr, "invalid") || contains(errStr, "missing") {
		statusCode = http.StatusBadRequest
		errorCode = "invalid_request"
		message = err.Error()
	}

	return c.JSON(statusCode, map[string]any{
		"error":   errorCode,
		"message": message,
	})
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || fmt.Sprintf("%s", s)[0:len(substr)] == substr)
}
