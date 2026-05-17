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

// dailyUploadLimitPerBusiness is the maximum number of documents a single business
// may upload per UTC calendar day. At typical LLM token costs (~15k tokens/doc)
// this maps to roughly 1.5M tokens/day — a practical cost ceiling before we have
// a richer per-business billing model.
const dailyUploadLimitPerBusiness = 100

// ErrDailyUploadQuotaExceeded is returned when a business has hit its daily document cap.
var ErrDailyUploadQuotaExceeded = fmt.Errorf("daily document upload quota exceeded; try again tomorrow")

// UploadDocumentRequest is the input for the UploadDocument service method
type UploadDocumentRequest struct {
	File       *multipart.FileHeader `json:"-"`
	DocType    string                `json:"doc_type"`    // INVOICE or PURCHASE_ORDER
	BusinessID uuid.UUID             `json:"business_id"` // authenticated owner — required
}

// UploadDocumentResponse is the output of a successful document upload
type UploadDocumentResponse struct {
	DocumentID  uuid.UUID `json:"document_id"`
	S3Path      string    `json:"s3_path"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	IsDuplicate bool      `json:"is_duplicate"`
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
	if req.BusinessID == uuid.Nil {
		return nil, fmt.Errorf("business_id is required")
	}
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
		// Only surface a duplicate to the same business that originally uploaded it —
		// otherwise return the hash collision as if it were a fresh upload so the new
		// tenant gets their own record and doesn't learn about another business's doc.
		if existing.BusinessID != nil && *existing.BusinessID == req.BusinessID {
			s.logger.InfoWith("Duplicate document detected", "file_hash", fileHash, "existing_doc_id", existing.ID)
			return &UploadDocumentResponse{
				DocumentID:  existing.ID,
				S3Path:      existing.S3Path,
				Status:      string(existing.Status),
				Message:     "Duplicate document — returning existing record",
				IsDuplicate: true,
			}, nil
		}
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
	bid := req.BusinessID
	doc := &voucher.VoucherDocument{
		ID:         docID,
		BusinessID: &bid,
		S3Path:     uploadedKey,
		DocType:    docType,
		Status:     voucher.DocStatusQueued,
		FileHash:   fileHash,
	}

	if err := s.voucherRepo.CreateVoucherDocument(ctx, doc); err != nil {
		s.logger.ErrorWith("Failed to persist document record", "error", err, "doc_id", docID)
		return nil, fmt.Errorf("failed to persist document metadata: %w", err)
	}

	s.logger.InfoWith("Document record persisted", "doc_id", docID, "status", doc.Status)

	// 9. Enforce per-business daily upload quota before enqueuing LLM work.
	// Duplicate uploads never reach this point (returned early), so the counter
	// only increments for genuinely new documents.
	if err := s.checkAndIncrementDailyQuota(ctx, req.BusinessID); err != nil {
		return nil, err
	}

	// 10. Push extraction task to Redis queue for the Python worker.
	// If enqueue fails, the doc stays in QUEUED and is picked up by the reconciliation
	// sweeper (ReconcileQueuedDocuments) — the request still succeeds so the user sees
	// their upload persisted.
	if err := s.enqueueExtraction(ctx, docID, uploadedKey, docType, &bid); err != nil {
		s.logger.WarnWith("Extraction enqueue failed; left to reconciliation sweeper",
			"error", err, "doc_id", docID)
	} else {
		s.logger.InfoWith("Extraction task enqueued", "doc_id", docID, "queue", extractionQueueName)
	}

	return &UploadDocumentResponse{
		DocumentID: docID,
		S3Path:     uploadedKey,
		Status:     string(voucher.DocStatusQueued),
		Message:    "Document uploaded and queued for extraction",
	}, nil
}

// enqueueExtraction marshals and LPUSHes a fresh extraction task. Shared by upload,
// retry, and the reconciliation sweeper.
func (s *DocumentUploadService) enqueueExtraction(ctx context.Context, docID uuid.UUID, s3Path, docType string, businessID *uuid.UUID) error {
	payload := map[string]interface{}{
		"document_id": docID.String(),
		"s3_path":     s3Path,
		"doc_type":    docType,
	}
	if businessID != nil {
		payload["business_id"] = businessID.String()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal extraction task: %w", err)
	}
	if err := s.redisClient.LPush(ctx, extractionQueueName, body).Err(); err != nil {
		return fmt.Errorf("redis lpush: %w", err)
	}
	return nil
}

// ErrDocumentNotOwned is returned when a caller tries to act on a document belonging
// to a different business.
var ErrDocumentNotOwned = fmt.Errorf("document does not belong to this business")

// checkAndIncrementDailyQuota atomically increments a per-business upload counter
// for today (UTC) and returns ErrDailyUploadQuotaExceeded if the daily cap is hit.
// The key expires after 48 hours so no manual cleanup is needed.
func (s *DocumentUploadService) checkAndIncrementDailyQuota(ctx context.Context, businessID uuid.UUID) error {
	today := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("xcentro:daily_uploads:%s:%s", businessID, today)

	count, err := s.redisClient.Incr(ctx, key).Result()
	if err != nil {
		// Non-fatal: if Redis is down, let the upload proceed to avoid blocking users.
		s.logger.WarnWith("Daily quota Redis check failed — allowing upload", "error", err, "business_id", businessID)
		return nil
	}

	// Set TTL only on the first increment so we don't extend it on every upload.
	if count == 1 {
		s.redisClient.Expire(ctx, key, 48*time.Hour)
	}

	if count > dailyUploadLimitPerBusiness {
		// Roll back the increment so the count accurately reflects successful uploads.
		s.redisClient.Decr(ctx, key)
		s.logger.WarnWith("Daily upload quota exceeded", "business_id", businessID, "count", count, "limit", dailyUploadLimitPerBusiness)
		return ErrDailyUploadQuotaExceeded
	}
	return nil
}

// assertOwnership checks that the document is owned by the caller's business.
// Legacy rows with a nil bid fail closed.
func (s *DocumentUploadService) assertOwnership(ctx context.Context, docID, businessID uuid.UUID) error {
	owner, err := s.voucherRepo.GetVoucherDocumentBusinessID(ctx, docID)
	if err != nil {
		return err
	}
	if owner == uuid.Nil || owner != businessID {
		return ErrDocumentNotOwned
	}
	return nil
}

// RetryDocument resets a FAILED document to QUEUED and re-enqueues the extraction task.
func (s *DocumentUploadService) RetryDocument(ctx context.Context, docID, businessID uuid.UUID) error {
	if err := s.assertOwnership(ctx, docID, businessID); err != nil {
		return err
	}

	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to load document %s: %w", docID, err)
	}

	if doc.Status != voucher.DocStatusFailed {
		return fmt.Errorf("only FAILED documents can be retried, current status: %s", doc.Status)
	}

	if err := s.voucherRepo.UpdateVoucherDocumentStatus(ctx, docID, voucher.DocStatusQueued); err != nil {
		return fmt.Errorf("failed to reset document status: %w", err)
	}

	if err := s.enqueueExtraction(ctx, docID, doc.S3Path, doc.DocType, doc.BusinessID); err != nil {
		s.logger.ErrorWith("Failed to re-enqueue extraction task", "error", err, "doc_id", docID)
		return fmt.Errorf("failed to enqueue extraction task: %w", err)
	}

	s.logger.InfoWith("Document retried successfully", "doc_id", docID)
	return nil
}

// DeleteDocument removes a document from the database and S3.
// Any owned document may be deleted regardless of its current status, so users
// can clean up stuck or broken records (e.g. QUEUED with a missing S3 object).
func (s *DocumentUploadService) DeleteDocument(ctx context.Context, docID, businessID uuid.UUID) error {
	if err := s.assertOwnership(ctx, docID, businessID); err != nil {
		return err
	}

	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to load document %s: %w", docID, err)
	}

	// Delete from S3 — if it fails (e.g. NoSuchKey for a partially-uploaded file)
	// we still remove the DB row. Orphan objects are cleaned up by a lifecycle policy.
	if doc.S3Path != "" {
		if err := s.s3Client.DeleteObject(ctx, doc.S3Path); err != nil {
			s.logger.WarnWith("S3 object delete failed (may already be absent)", "error", err, "s3_path", doc.S3Path)
		}
	}

	if err := s.voucherRepo.DeleteVoucherDocument(ctx, docID); err != nil {
		return fmt.Errorf("failed to delete document record: %w", err)
	}

	s.logger.InfoWith("Document deleted successfully", "doc_id", docID, "s3_path", doc.S3Path)
	return nil
}

// GetPresignedURL generates a short-lived presigned S3 download URL for a document.
func (s *DocumentUploadService) GetPresignedURL(ctx context.Context, docID, businessID uuid.UUID) (string, error) {
	if err := s.assertOwnership(ctx, docID, businessID); err != nil {
		return "", err
	}
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		return "", err
	}
	return s.s3Client.GetPresignedDownloadURL(ctx, doc.S3Path, 15*time.Minute)
}

// Reconcile loop cadence — tuned so transient Redis outages of a few minutes
// heal before users notice stuck documents, without hammering the DB.
const (
	reconcileInterval  = 2 * time.Minute
	reconcileStaleness = 5 * time.Minute
	reconcileBatch     = 100
)

// StartReconcileLoop runs ReconcileQueuedDocuments on a fixed interval until
// ctx is done. Launched once at startup alongside the matching worker.
func (s *DocumentUploadService) StartReconcileLoop(ctx context.Context) {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	s.logger.InfoWith("Document reconcile loop started",
		"interval", reconcileInterval, "staleness", reconcileStaleness)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.ReconcileQueuedDocuments(ctx, reconcileStaleness, reconcileBatch)
			if err != nil {
				s.logger.ErrorWith("Reconcile sweep failed", "error", err)
				continue
			}
			if n > 0 {
				s.logger.InfoWith("Reconcile sweep re-enqueued documents", "count", n)
			}
		}
	}
}

// ReconcileQueuedDocuments re-enqueues extraction tasks for documents stuck in
// QUEUED status past `staleness`. Intended to be invoked by a periodic job so that
// transient Redis outages during upload do not permanently drop tasks.
func (s *DocumentUploadService) ReconcileQueuedDocuments(ctx context.Context, staleness time.Duration, limit int) (int, error) {
	docs, err := s.voucherRepo.ListQueuedDocumentsOlderThan(ctx, staleness, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, doc := range docs {
		if err := s.enqueueExtraction(ctx, doc.ID, doc.S3Path, doc.DocType, doc.BusinessID); err != nil {
			s.logger.WarnWith("Reconciliation re-enqueue failed", "error", err, "doc_id", doc.ID)
			continue
		}
		recovered++
	}
	return recovered, nil
}
