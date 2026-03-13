package voucher

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// DocumentReviewService handles the HITL document review workflow.
// It orchestrates fetching draft data, checking status, and validating
// documents with corrected fields before persisting to typed tables.
type DocumentReviewService struct {
	voucherRepo voucher.VoucherRepository
	logger      *logger.Logger
}

// NewDocumentReviewService creates a new DocumentReviewService
func NewDocumentReviewService(
	voucherRepo voucher.VoucherRepository,
	logger *logger.Logger,
) *DocumentReviewService {
	return &DocumentReviewService{
		voucherRepo: voucherRepo,
		logger:      logger,
	}
}

// GetDocumentDraft fetches the extracted draft data for a document
func (s *DocumentReviewService) GetDocumentDraft(ctx context.Context, docID uuid.UUID) (*voucher.VoucherDocument, error) {
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		s.logger.ErrorWith("Failed to get document draft", "error", err, "doc_id", docID)
		return nil, err
	}

	return doc, nil
}

// GetDocumentStatus fetches just the processing status for a document
func (s *DocumentReviewService) GetDocumentStatus(ctx context.Context, docID uuid.UUID) (voucher.DocumentStatus, error) {
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		s.logger.ErrorWith("Failed to get document status", "error", err, "doc_id", docID)
		return "", err
	}

	return doc.Status, nil
}

// ValidateDocumentRequest is the input for the ValidateDocument method
type ValidateDocumentRequest struct {
	DraftData map[string]interface{} `json:"draft_data"`
}

// ValidateDocument accepts corrected fields, updates the document to VALIDATED,
// and persists the data to the appropriate typed table (extracted_invoices or purchase_orders)
func (s *DocumentReviewService) ValidateDocument(ctx context.Context, docID uuid.UUID, req *ValidateDocumentRequest) error {
	// 1. Fetch the current document
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		s.logger.ErrorWith("Failed to get document for validation", "error", err, "doc_id", docID)
		return err
	}

	// 2. Ensure document is in REVIEW_REQUIRED status
	if doc.Status != voucher.DocStatusReviewRequired {
		return fmt.Errorf("document is not in REVIEW_REQUIRED status; current status: %s", doc.Status)
	}

	// 3. Update draft data and set status to VALIDATED
	if err := s.voucherRepo.UpdateVoucherDocumentValidation(ctx, docID, voucher.DocStatusValidated, req.DraftData); err != nil {
		s.logger.ErrorWith("Failed to update document validation", "error", err, "doc_id", docID)
		return err
	}

	s.logger.InfoWith("Document validated successfully", "doc_id", docID)

	// 4. Persist to typed table based on doc_type
	docType := doc.DocType
	// Allow override from the draft_data if present
	if dt, ok := req.DraftData["doc_type"].(string); ok && dt != "" {
		docType = dt
	}

	switch docType {
	case string(voucher.DocTypePurchaseOrder):
		if err := s.voucherRepo.CreatePurchaseOrder(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist purchase order", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist purchase order: %w", err)
		}
		s.logger.InfoWith("Purchase order persisted", "doc_id", docID)

	case string(voucher.DocTypeInvoice):
		if err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		}
		s.logger.InfoWith("Extracted invoice persisted", "doc_id", docID)

	default:
		// For UNKNOWN doc_type, default to invoice
		s.logger.WarnWith("Unknown doc_type, defaulting to invoice", "doc_type", docType, "doc_id", docID)
		if err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice (default)", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		}
	}

	return nil
}

// ListDocuments retrieves a paginated list of voucher documents
func (s *DocumentReviewService) ListDocuments(ctx context.Context, filter voucher.DocumentListFilter) (*voucher.PaginatedResponse[*voucher.VoucherDocument], error) {
	docs, total, err := s.voucherRepo.ListVoucherDocuments(ctx, filter)
	if err != nil {
		s.logger.ErrorWith("Failed to list documents", "error", err)
		return nil, err
	}

	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	return &voucher.PaginatedResponse[*voucher.VoucherDocument]{
		Items:      docs,
		Total:      total,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalPages: calculateTotalPages(total, pageSize),
	}, nil
}

// ListInvoices retrieves a paginated list of extracted invoices
func (s *DocumentReviewService) ListInvoices(ctx context.Context, filter voucher.ExtractedInvoiceListFilter) (*voucher.PaginatedResponse[*voucher.ExtractedInvoice], error) {
	invs, total, err := s.voucherRepo.ListExtractedInvoices(ctx, filter)
	if err != nil {
		s.logger.ErrorWith("Failed to list invoices", "error", err)
		return nil, err
	}

	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	return &voucher.PaginatedResponse[*voucher.ExtractedInvoice]{
		Items:      invs,
		Total:      total,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalPages: calculateTotalPages(total, pageSize),
	}, nil
}

// ListPurchaseOrders retrieves a paginated list of purchase orders
func (s *DocumentReviewService) ListPurchaseOrders(ctx context.Context, filter voucher.PurchaseOrderListFilter) (*voucher.PaginatedResponse[*voucher.PurchaseOrder], error) {
	pos, total, err := s.voucherRepo.ListPurchaseOrders(ctx, filter)
	if err != nil {
		s.logger.ErrorWith("Failed to list purchase orders", "error", err)
		return nil, err
	}

	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	return &voucher.PaginatedResponse[*voucher.PurchaseOrder]{
		Items:      pos,
		Total:      total,
		Page:       filter.Page,
		PageSize:   pageSize,
		TotalPages: calculateTotalPages(total, pageSize),
	}, nil
}

func calculateTotalPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		pages++
	}
	return pages
}
