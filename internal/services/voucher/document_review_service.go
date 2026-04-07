package voucher

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// DocumentReviewService handles the HITL document review workflow.
// It orchestrates fetching draft data, checking status, and validating
// documents with corrected fields before persisting to typed tables.
type DocumentReviewService struct {
	voucherRepo               voucher.VoucherRepository
	resolutionService         *PayeeResolutionService
	matchingEngine            *MatchingEngine
	redisClient               *redis.Client
	payeeMasterRepo           payee.PayeeMasterRepository
	payeeVendorRepo           payee.PayeeVendorRepository
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository
	txManager                 client.TxManager
	logger                    *logger.Logger
}

// QuickCreateVendorRequest holds the minimal data needed for a voucher-context vendor creation.
type QuickCreateVendorRequest struct {
	BusinessID   string
	AddedBy      string
	DisplayName  string
	GSTIN        string
	ContactEmail string
	ContactPhone string
}

// QuickCreateVendorResponse is returned after a successful quick vendor creation.
type QuickCreateVendorResponse struct {
	RelationID string `json:"relation_id"`
	PayeeID    string `json:"payee_id"`
}

// NewDocumentReviewService creates a new DocumentReviewService
func NewDocumentReviewService(
	voucherRepo voucher.VoucherRepository,
	vendorRepo payee.PayeeVendorRepository,
	redisClient *redis.Client,
	matchingEngine *MatchingEngine,
	payeeMasterRepo payee.PayeeMasterRepository,
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository,
	txManager client.TxManager,
	logger *logger.Logger,
) *DocumentReviewService {
	var resolutionSvc *PayeeResolutionService
	if vendorRepo != nil {
		resolutionSvc = NewPayeeResolutionService(vendorRepo, logger)
	}
	return &DocumentReviewService{
		voucherRepo:               voucherRepo,
		resolutionService:         resolutionSvc,
		matchingEngine:            matchingEngine,
		redisClient:               redisClient,
		payeeMasterRepo:           payeeMasterRepo,
		payeeVendorRepo:           vendorRepo,
		businessPayeeRelationRepo: businessPayeeRelationRepo,
		txManager:                 txManager,
		logger:                    logger,
	}
}

// QuickCreateVendor creates a minimal vendor payee for the voucher document review context.
// It does NOT perform GSTIN or bank account verification — the payee is created as pending
// and bank account details can be added later through the full payee onboarding flow.
func (s *DocumentReviewService) QuickCreateVendor(ctx context.Context, req QuickCreateVendorRequest) (*QuickCreateVendorResponse, error) {
	if req.BusinessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name is required")
	}

	// If GSTIN provided and a vendor with this GSTIN already exists, reuse that payee.
	var existingPayeeID string
	if req.GSTIN != "" && len(req.GSTIN) == 15 && s.payeeVendorRepo != nil {
		existing, err := s.payeeVendorRepo.FindByGSTIN(ctx, req.GSTIN)
		if err == nil && existing != nil {
			existingPayeeID = existing.PayeeID
			s.logger.InfoWith("Reusing existing vendor for quick create", "payee_id", existingPayeeID, "gstin", req.GSTIN)
		}
	}

	// If an existing payee was found, check whether a relation already exists for this business.
	// If so, return it immediately without creating anything.
	if existingPayeeID != "" {
		existingRelation, err := s.businessPayeeRelationRepo.FindRelationByBusinessAndPayee(ctx, req.BusinessID, existingPayeeID)
		if err != nil {
			s.logger.ErrorWith("QuickCreateVendor: failed to check existing relation", "error", err)
		} else if existingRelation != nil {
			s.logger.InfoWith("QuickCreateVendor: relation already exists, reusing", "relation_id", existingRelation.RelationID)
			return &QuickCreateVendorResponse{
				RelationID: existingRelation.RelationID,
				PayeeID:    existingPayeeID,
			}, nil
		}
	}

	var payeeID, relationID string

	err := s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txMasterRepo := s.payeeMasterRepo.WithTx(tx)
		txVendorRepo := s.payeeVendorRepo.WithTx(tx)
		txRelationRepo := s.businessPayeeRelationRepo.WithTx(tx)

		if existingPayeeID == "" {
			// Create payee master
			master := &payee.PayeeMaster{
				PayeeType: payee.PayeeTypeVendor,
				IsActive:  true,
			}
			if err := txMasterRepo.Create(ctx, master); err != nil {
				return fmt.Errorf("failed to create payee master: %w", err)
			}
			payeeID = master.PayeeID

			// Create minimal vendor record (no GSTIN verification)
			vendor := &payee.PayeeVendor{
				PayeeID:               payeeID,
				GSTIN:                 req.GSTIN,
				GSTVerificationStatus: payee.VerificationStatusPending,
			}
			if err := txVendorRepo.Create(ctx, vendor); err != nil {
				return fmt.Errorf("failed to create vendor: %w", err)
			}
		} else {
			payeeID = existingPayeeID
		}

		// Create relation WITHOUT account_id (NULL — bank account to be added later)
		relation := &payee.BusinessPayeeRelation{
			BusinessID:   req.BusinessID,
			PayeeID:      payeeID,
			AccountID:    "", // intentionally empty → stored as NULL
			DisplayName:  req.DisplayName,
			ContactEmail: req.ContactEmail,
			ContactPhone: req.ContactPhone,
			IsActive:     true,
			IsFavorite:   false,
			Tags:         []string{},
			AddedBy:      req.AddedBy,
		}
		if err := txRelationRepo.CreateRelation(ctx, relation); err != nil {
			return fmt.Errorf("failed to create relation: %w", err)
		}
		relationID = relation.RelationID
		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.InfoWith("Quick vendor creation successful", "payee_id", payeeID, "relation_id", relationID)
	return &QuickCreateVendorResponse{
		RelationID: relationID,
		PayeeID:    payeeID,
	}, nil
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

// DocumentDraftWithResolution is the enriched response returned by GetDocumentDraftWithResolution
type DocumentDraftWithResolution struct {
	Document           *voucher.VoucherDocument `json:"document"`
	PayeeResolution    *PayeeResolution         `json:"payee_resolution"`
	BusinessResolution *BusinessResolution      `json:"business_resolution"`
}

// GetDocumentDraftWithResolution fetches draft data and performs payee + business resolution
func (s *DocumentReviewService) GetDocumentDraftWithResolution(ctx context.Context, docID uuid.UUID, headerBusinessID string) (*DocumentDraftWithResolution, error) {
	doc, err := s.voucherRepo.GetVoucherDocumentByID(ctx, docID)
	if err != nil {
		s.logger.ErrorWith("Failed to get document draft", "error", err, "doc_id", docID)
		return nil, err
	}

	result := &DocumentDraftWithResolution{Document: doc}

	if s.resolutionService != nil && doc.DraftData != nil {
		vendorName, _ := doc.DraftData["vendor_name"].(string)
		vendorGSTIN, _ := doc.DraftData["vendor_gstin"].(string)
		buyerGSTIN, _ := doc.DraftData["buyer_gstin"].(string)

		result.PayeeResolution = s.resolutionService.ResolvePayee(ctx, vendorName, vendorGSTIN)

		result.BusinessResolution = s.resolutionService.ResolveBusiness(
			ctx,
			headerBusinessID,
			buyerGSTIN,
			s.voucherRepo.GetBusinessIDByGSTIN,
		)
	} else {
		result.PayeeResolution = &PayeeResolution{Status: PayeeResolutionNotFound}
		result.BusinessResolution = &BusinessResolution{ResolvedVia: "unresolved"}
		if headerBusinessID != "" {
			result.BusinessResolution = &BusinessResolution{
				BusinessID:  headerBusinessID,
				ResolvedVia: "header",
			}
		}
	}

	return result, nil
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
		if poID, err := s.voucherRepo.CreatePurchaseOrder(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist purchase order", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist purchase order: %w", err)
		} else {
			s.logger.InfoWith("Purchase order persisted", "doc_id", docID, "po_id", poID)
			
			// Trigger orphan invoice reconciliation matching
			poNumber, _ := req.DraftData["po_number"].(string)
			businessIDStr, _ := req.DraftData["business_id"].(string)
			if poNumber != "" && businessIDStr != "" && s.matchingEngine != nil {
				if bid, err := uuid.Parse(businessIDStr); err == nil {
					go func() {
						matchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						if err := s.matchingEngine.RetryOrphanInvoices(matchCtx, poNumber, bid); err != nil {
							s.logger.ErrorWith("Orphan reconciliation failed", "error", err, "po_number", poNumber)
						}
					}()
				}
			}
		}

	case string(voucher.DocTypeInvoice):
		if invoiceID, err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		} else {
			s.logger.InfoWith("Extracted invoice persisted", "doc_id", docID, "invoice_id", invoiceID)
			
			// Trigger invoice-PO matching
			if s.matchingEngine != nil {
				go func() {
					matchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := s.matchingEngine.RunMatchingForInvoice(matchCtx, invoiceID); err != nil {
						s.logger.ErrorWith("Matching engine failed for invoice", "error", err, "invoice_id", invoiceID)
					}
				}()
			}
		}

	default:
		// For UNKNOWN doc_type, default to invoice
		s.logger.WarnWith("Unknown doc_type, defaulting to invoice", "doc_type", docType, "doc_id", docID)
		if invoiceID, err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, req.DraftData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice (default)", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		} else {
			// Trigger invoice-PO matching
			if s.matchingEngine != nil {
				go func() {
					matchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := s.matchingEngine.RunMatchingForInvoice(matchCtx, invoiceID); err != nil {
						s.logger.ErrorWith("Matching engine failed for invoice (default)", "error", err, "invoice_id", invoiceID)
					}
				}()
			}
		}
	}

	// 5. Enqueue template generation task if processed by LLM or human corrected the data
	humanCorrected := !reflect.DeepEqual(doc.DraftData, req.DraftData)
	if doc.ProcessedBy == "LLM" || humanCorrected {
		taskPayload := map[string]interface{}{
			"document_id": docID.String(),
			"task_type":   "generate_template",
		}

		if taskJSON, err := json.Marshal(taskPayload); err != nil {
			s.logger.ErrorWith("Failed to marshal template generation task payload", "error", err, "doc_id", docID)
		} else {
			if err := s.redisClient.LPush(ctx, "xcentro:template_generation_tasks", taskJSON).Err(); err != nil {
				s.logger.ErrorWith("Failed to push template generation task to Redis", "error", err, "doc_id", docID)
			} else {
				s.logger.InfoWith("Template generation task enqueued",
					"doc_id", docID,
					"reason_llm", doc.ProcessedBy == "LLM",
					"reason_corrected", humanCorrected,
				)
			}
		}
	}

	return nil
}

// GetInvoiceLineItems retrieves all line items for a given invoice.
func (s *DocumentReviewService) GetInvoiceLineItems(ctx context.Context, invoiceID uuid.UUID) ([]voucher.LineItemDetail, error) {
	items, err := s.voucherRepo.GetLineItemsByInvoiceID(ctx, invoiceID)
	if err != nil {
		s.logger.ErrorWith("Failed to get invoice line items", "error", err, "invoice_id", invoiceID)
		return nil, err
	}
	return items, nil
}

// GetPurchaseOrderLineItems retrieves all line items for a given purchase order.
func (s *DocumentReviewService) GetPurchaseOrderLineItems(ctx context.Context, poID uuid.UUID) ([]voucher.LineItemDetail, error) {
	items, err := s.voucherRepo.GetLineItemsByPurchaseOrderID(ctx, poID)
	if err != nil {
		s.logger.ErrorWith("Failed to get PO line items", "error", err, "po_id", poID)
		return nil, err
	}
	return items, nil
}

// GetMatchResult retrieves all match result records for a given invoice.
func (s *DocumentReviewService) GetMatchResult(ctx context.Context, invoiceID uuid.UUID) ([]*voucher.MatchResult, error) {
	results, err := s.voucherRepo.GetMatchResultsByInvoiceID(ctx, invoiceID)
	if err != nil {
		s.logger.ErrorWith("Failed to get match results", "error", err, "invoice_id", invoiceID)
		return nil, err
	}
	return results, nil
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

// ListMatchResults retrieves a paginated joined view of match results for the Match Results tab.
func (s *DocumentReviewService) ListMatchResults(ctx context.Context, filter voucher.MatchResultListFilter) (*voucher.PaginatedResponse[*voucher.MatchResultSummary], error) {
	items, total, err := s.voucherRepo.ListMatchResults(ctx, filter)
	if err != nil {
		s.logger.ErrorWith("Failed to list match results", "error", err)
		return nil, err
	}

	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	return &voucher.PaginatedResponse[*voucher.MatchResultSummary]{
		Items:      items,
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
