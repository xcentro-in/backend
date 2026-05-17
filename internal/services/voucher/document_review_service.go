package voucher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// HITL regen thresholds — tuneable knobs for the template-degradation feedback
// loop. We require a minimum sample size before regenerating to avoid acting
// on a single bad doc, and a correction ratio that signals the template is
// systemically wrong (not just an unusual document).
const (
	templateRegenLookback        = 30 * 24 * time.Hour
	templateRegenMinTotalDocs    = 5
	templateRegenCorrectionRatio = 0.6 // ≥60% of validated docs had corrections
)

// templateDegraded returns true when a template's recent correction history
// indicates it is producing wrong values often enough to warrant regeneration.
// Pulled out of ValidateDocument so the threshold logic can be unit-tested in
// isolation from the repository and redis machinery.
func templateDegraded(corrected, total int) bool {
	if total < templateRegenMinTotalDocs {
		return false
	}
	return float64(corrected)/float64(total) >= templateRegenCorrectionRatio
}

// diffDraftData returns one DocumentCorrection per field whose JSON
// representation changed between before and after. Fields that are identical
// (or present only in after due to the merge) but unchanged are skipped.
func diffDraftData(before, after map[string]interface{}) []voucher.DocumentCorrection {
	var diffs []voucher.DocumentCorrection
	for key, newVal := range after {
		oldVal := before[key]
		oldJSON, _ := json.Marshal(oldVal)
		newJSON, _ := json.Marshal(newVal)
		if string(oldJSON) != string(newJSON) {
			diffs = append(diffs, voucher.DocumentCorrection{
				Field:    key,
				OldValue: oldVal,
				NewValue: newVal,
			})
		}
	}
	return diffs
}

// draftHash returns a deterministic hash of a draft_data map so we can detect
// human edits without the false-positive landmines of reflect.DeepEqual on
// JSON-round-tripped data (where int/float64 and nil/absent fields differ).
func draftHash(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

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

// AssertDocumentOwnership returns ErrDocumentNotOwned when the document
// does not belong to the given business. Legacy rows with a nil bid fail closed.
func (s *DocumentReviewService) AssertDocumentOwnership(ctx context.Context, docID, businessID uuid.UUID) error {
	owner, err := s.voucherRepo.GetVoucherDocumentBusinessID(ctx, docID)
	if err != nil {
		return err
	}
	if owner == uuid.Nil || owner != businessID {
		return ErrDocumentNotOwned
	}
	return nil
}

// AssertInvoiceOwnership returns ErrDocumentNotOwned when the invoice does not
// belong to the given business. Rows with a nil bid fail closed.
func (s *DocumentReviewService) AssertInvoiceOwnership(ctx context.Context, invoiceID, businessID uuid.UUID) error {
	owner, err := s.voucherRepo.GetInvoiceBusinessID(ctx, invoiceID)
	if err != nil {
		return err
	}
	if owner == uuid.Nil || owner != businessID {
		return ErrDocumentNotOwned
	}
	return nil
}

// AssertPOOwnership returns ErrDocumentNotOwned when the purchase order does not
// belong to the given business. Rows with a nil bid fail closed.
func (s *DocumentReviewService) AssertPOOwnership(ctx context.Context, poID, businessID uuid.UUID) error {
	owner, err := s.voucherRepo.GetPurchaseOrderBusinessID(ctx, poID)
	if err != nil {
		return err
	}
	if owner == uuid.Nil || owner != businessID {
		return ErrDocumentNotOwned
	}
	return nil
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
	UserID    string                 `json:"-"` // set by handler from JWT; never from request body
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

	// 3. Merge: start from the worker-generated draft_data, apply only the
	// human's explicit field corrections on top. This prevents a client from
	// submitting a wholly fabricated payload — they can correct values the LLM
	// extracted but cannot inject fields the worker never produced.
	mergedData := make(map[string]interface{}, len(doc.DraftData)+len(req.DraftData))
	for k, v := range doc.DraftData {
		mergedData[k] = v
	}
	for k, v := range req.DraftData {
		mergedData[k] = v
	}
	// business_id is already stamped by the handler from the JWT claim;
	// re-stamp from the doc's own bid so the DB value is always authoritative.
	if doc.BusinessID != nil {
		mergedData["business_id"] = doc.BusinessID.String()
	}

	// 4. Persist merged data and set status to VALIDATED
	if err := s.voucherRepo.UpdateVoucherDocumentValidation(ctx, docID, voucher.DocStatusValidated, mergedData); err != nil {
		s.logger.ErrorWith("Failed to update document validation", "error", err, "doc_id", docID)
		return err
	}

	s.logger.InfoWith("Document validated successfully", "doc_id", docID)

	// 5. Record which fields the human changed (audit trail for compliance).
	if corrections := diffDraftData(doc.DraftData, mergedData); len(corrections) > 0 && req.UserID != "" {
		if err := s.voucherRepo.InsertDocumentCorrections(ctx, docID, req.UserID, corrections); err != nil {
			// Non-fatal: log but don't fail the validation.
			s.logger.ErrorWith("Failed to write document corrections audit log", "error", err, "doc_id", docID)
		}
	}

	// 6. Route to the correct typed table using the doc_type recorded by the
	// worker — never from the client payload.
	docType := doc.DocType

	switch docType {
	case string(voucher.DocTypePurchaseOrder):
		if poID, err := s.voucherRepo.CreatePurchaseOrder(ctx, docID, mergedData); err != nil {
			s.logger.ErrorWith("Failed to persist purchase order", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist purchase order: %w", err)
		} else {
			s.logger.InfoWith("Purchase order persisted", "doc_id", docID, "po_id", poID)

			// Enqueue orphan-retry for the async worker instead of blocking the
			// request on a detached goroutine.
			poNumber, _ := mergedData["po_number"].(string)
			businessIDStr, _ := mergedData["business_id"].(string)
			if poNumber != "" && businessIDStr != "" && s.matchingEngine != nil {
				if bid, parseErr := uuid.Parse(businessIDStr); parseErr == nil {
					if err := s.matchingEngine.EnqueueOrphanRetry(ctx, poNumber, bid); err != nil {
						s.logger.ErrorWith("Failed to enqueue orphan retry", "error", err, "po_number", poNumber)
					}
				}
			}
		}

	case string(voucher.DocTypeInvoice):
		if invoiceID, err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, mergedData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		} else {
			s.logger.InfoWith("Extracted invoice persisted", "doc_id", docID, "invoice_id", invoiceID)

			if s.matchingEngine != nil {
				if err := s.matchingEngine.EnqueueInvoiceMatch(ctx, invoiceID); err != nil {
					s.logger.ErrorWith("Failed to enqueue invoice match", "error", err, "invoice_id", invoiceID)
				}
			}
		}

	default:
		// For UNKNOWN doc_type, default to invoice
		s.logger.WarnWith("Unknown doc_type, defaulting to invoice", "doc_type", docType, "doc_id", docID)
		if invoiceID, err := s.voucherRepo.CreateInvoiceFromExtraction(ctx, docID, mergedData); err != nil {
			s.logger.ErrorWith("Failed to persist extracted invoice (default)", "error", err, "doc_id", docID)
			return fmt.Errorf("validated but failed to persist extracted invoice: %w", err)
		} else {
			if s.matchingEngine != nil {
				if err := s.matchingEngine.EnqueueInvoiceMatch(ctx, invoiceID); err != nil {
					s.logger.ErrorWith("Failed to enqueue invoice match (default)", "error", err, "invoice_id", invoiceID)
				}
			}
		}
	}

	// 6. Enqueue template generation task. Three triggers:
	//   a) LLM extracted (no template yet exists for this vendor)
	//   b) Human corrected the worker's draft (the existing template, if any,
	//      missed something that's worth re-learning)
	//   c) HITL feedback loop: the template that produced this doc has been
	//      systematically wrong over the last lookback window — regenerate.
	// Hash comparison avoids reflect.DeepEqual's false positives on JSON-round-tripped data
	// (float64 vs int, absent vs nil, etc.).
	humanCorrected := draftHash(doc.DraftData) != draftHash(mergedData)
	degraded := false
	if doc.I2DTemplate != "" {
		corrected, total, err := s.voucherRepo.GetTemplateCorrectionRate(
			ctx, doc.I2DTemplate, templateRegenLookback,
		)
		if err != nil {
			s.logger.WarnWith("Could not query template correction rate",
				"error", err, "template", doc.I2DTemplate)
		} else if templateDegraded(corrected, total) {
			degraded = true
			s.logger.InfoWith("event=template.regen.triggered",
				"template", doc.I2DTemplate,
				"corrected", corrected,
				"total", total,
				"ratio", float64(corrected)/float64(total),
			)
		}
	}

	// Force regen when:
	//   a) the template is statistically degraded (repeated corrections), or
	//   b) this specific document was extracted by a template but the user had
	//      to correct the values — meaning the template was wrong for this doc.
	//      Without force_regen the worker skips because a template already exists.
	templateWasWrong := humanCorrected && doc.ProcessedBy == "INVOICE2DATA"
	forceRegen := degraded || templateWasWrong

	if doc.ProcessedBy == "LLM" || humanCorrected || degraded {
		taskPayload := map[string]interface{}{
			"document_id": docID.String(),
			"task_type":   "generate_template",
			"force_regen": forceRegen,
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
					"reason_degraded", degraded,
					"force_regen", forceRegen,
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
