package voucher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/providers/fileupload"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// Allowed file extensions and MIME types for document upload
var allowedMIMETypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
}

var allowedExtensions = map[string]bool{
	".pdf":  true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

// Redis queue name must match Python worker config (xcentro:extraction_tasks)
const extractionQueueName = "xcentro:extraction_tasks"

// maxUploadSizeBytes is the maximum allowed file size (10 MB)
const maxUploadSizeBytes = 10 * 1024 * 1024

// UploadDocumentRequest is the input for the UploadDocument service method
type UploadDocumentRequest struct {
	File       *multipart.FileHeader `json:"-"`
	DocType    string                `json:"doc_type"`    // INVOICE or PURCHASE_ORDER
	BusinessID *uuid.UUID            `json:"business_id"` // Optional, for violation tracking
}

// UploadDocumentResponse is the output of a successful document upload
type UploadDocumentResponse struct {
	DocumentID uuid.UUID `json:"document_id"`
	S3Path     string    `json:"s3_path"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
}

// DocumentUploadService handles the ingestion pipeline for voucher documents.
// It orchestrates S3 upload, database persistence, and Redis task dispatch.
type DocumentUploadService struct {
	voucherRepo voucher.VoucherRepository
	s3Client    *fileupload.S3Client
	redisClient *redis.Client
	logger      *logger.Logger
}

// NewDocumentUploadService creates a new DocumentUploadService
func NewDocumentUploadService(
	voucherRepo voucher.VoucherRepository,
	s3Client *fileupload.S3Client,
	redisClient *redis.Client,
	logger *logger.Logger,
) *DocumentUploadService {
	return &DocumentUploadService{
		voucherRepo: voucherRepo,
		s3Client:    s3Client,
		redisClient: redisClient,
		logger:      logger,
	}
}

// UploadDocument handles the full ingestion flow:
// 1-4. Validate file type, size, extension, doc_type
// 5. Compute SHA-256 hash; return existing record if duplicate
// 6. Generate UUIDv7 document ID
// 7. Upload file to S3
// 8. Persist document metadata (with file_hash) to voucher_documents
// 9. Push extraction task to Redis for the Python worker
func (s *DocumentUploadService) UploadDocument(ctx context.Context, req *UploadDocumentRequest) (*UploadDocumentResponse, error) {
	file := req.File

	// 1. Validate file size
	if file.Size > maxUploadSizeBytes {
		return nil, fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes", file.Size, maxUploadSizeBytes)
	}
	if file.Size == 0 {
		return nil, fmt.Errorf("uploaded file is empty")
	}

	// 2. Validate file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedExtensions[ext] {
		return nil, fmt.Errorf("file extension '%s' is not allowed; accepted: .pdf, .jpg, .jpeg, .png, .webp", ext)
	}

	// 3. Validate MIME type
	contentType := file.Header.Get("Content-Type")
	if contentType != "" && !allowedMIMETypes[contentType] {
		return nil, fmt.Errorf("content type '%s' is not allowed; accepted: application/pdf, image/jpeg, image/png, image/webp", contentType)
	}

	// 4. Validate doc_type
	docType := strings.ToUpper(req.DocType)
	if docType == "" {
		docType = string(voucher.DocTypeUnknown)
	}
	if docType != string(voucher.DocTypeInvoice) && docType != string(voucher.DocTypePurchaseOrder) && docType != string(voucher.DocTypeUnknown) {
		return nil, fmt.Errorf("invalid doc_type '%s'; accepted: INVOICE, PURCHASE_ORDER", req.DocType)
	}

	// 5. Compute SHA-256 hash for duplicate detection
	f, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	fileBytes, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read uploaded file: %w", err)
	}
	sum := sha256.Sum256(fileBytes)
	fileHash := hex.EncodeToString(sum[:])

	existing, err := s.voucherRepo.GetDocumentByFileHash(ctx, fileHash)
	if err != nil {
		s.logger.ErrorWith("Failed to check file hash", "error", err)
		return nil, fmt.Errorf("failed to check for duplicate document: %w", err)
	}
	if existing != nil {
		s.logger.InfoWith("Duplicate document detected", "file_hash", fileHash, "existing_doc_id", existing.ID)
		return &UploadDocumentResponse{
			DocumentID: existing.ID,
			S3Path:     existing.S3Path,
			Status:     string(existing.Status),
			Message:    "Duplicate document — returning existing record",
		}, nil
	}

	// 6. Generate document ID (UUIDv7 for time-sortable ordering)
	docID := uuid.Must(uuid.NewV7())

	// 7. Upload file to S3
	s3Key := fileupload.GenerateUploadKey("voucher_documents", file.Filename)
	s.logger.InfoWith("Uploading document to S3",
		"doc_id", docID,
		"s3_key", s3Key,
		"filename", file.Filename,
		"size", file.Size)

	uploadedKey, err := s.s3Client.UploadFile(ctx, file, s3Key)
	if err != nil {
		s.logger.ErrorWith("S3 upload failed", "error", err, "doc_id", docID)
		return nil, fmt.Errorf("failed to upload file to S3: %w", err)
	}

	s.logger.InfoWith("S3 upload successful", "doc_id", docID, "s3_path", uploadedKey)

	// 8. Persist document metadata to database
	doc := &voucher.VoucherDocument{
		ID:       docID,
		S3Path:   uploadedKey,
		DocType:  docType,
		Status:   voucher.DocStatusQueued,
		FileHash: fileHash,
	}

	if err := s.voucherRepo.CreateVoucherDocument(ctx, doc); err != nil {
		s.logger.ErrorWith("Failed to persist document record", "error", err, "doc_id", docID)
		return nil, fmt.Errorf("failed to persist document metadata: %w", err)
	}

	s.logger.InfoWith("Document record persisted", "doc_id", docID, "status", doc.Status)

	// 9. Push extraction task to Redis queue for the Python worker
	taskPayload := map[string]interface{}{
		"document_id": docID.String(),
		"s3_path":     uploadedKey,
		"doc_type":    docType,
	}
	if req.BusinessID != nil {
		taskPayload["business_id"] = req.BusinessID.String()
	}

	taskJSON, err := json.Marshal(taskPayload)
	if err != nil {
		s.logger.ErrorWith("Failed to marshal task payload", "error", err, "doc_id", docID)
		return nil, fmt.Errorf("failed to marshal extraction task: %w", err)
	}

	if err := s.redisClient.LPush(ctx, extractionQueueName, taskJSON).Err(); err != nil {
		s.logger.ErrorWith("Failed to push extraction task to Redis", "error", err, "doc_id", docID)
		// Note: document is already in DB with QUEUED status.
		// A reconciliation sweep could retry this later.
		return nil, fmt.Errorf("failed to enqueue extraction task: %w", err)
	}

	s.logger.InfoWith("Extraction task enqueued", "doc_id", docID, "queue", extractionQueueName)

	return &UploadDocumentResponse{
		DocumentID: docID,
		S3Path:     uploadedKey,
		Status:     string(voucher.DocStatusQueued),
		Message:    "Document uploaded and queued for extraction",
	}, nil
}

// GetPresignedURL generates a short-lived presigned S3 download URL for a document.
func (s *DocumentUploadService) GetPresignedURL(ctx context.Context, docID uuid.UUID) (string, error) {
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		return "", err
	}
	return s.s3Client.GetPresignedDownloadURL(ctx, doc.S3Path, 15*time.Minute)
}
