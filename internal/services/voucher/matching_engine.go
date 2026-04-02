package voucher

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/logger"
)

// MatchingEngine handles deterministic PO-Invoice matching
type MatchingEngine struct {
	voucherRepo voucher.VoucherRepository
	redisClient *redis.Client // Reserved for distributed locking if workers scale
	logger      *logger.Logger
}

// NewMatchingEngine creates a new MatchingEngine instance
func NewMatchingEngine(
	voucherRepo voucher.VoucherRepository,
	redisClient *redis.Client,
	logger *logger.Logger,
) *MatchingEngine {
	return &MatchingEngine{
		voucherRepo: voucherRepo,
		redisClient: redisClient,
		logger:      logger,
	}
}

// matchTypes
const (
	MatchMethodSKU         = "SKU_MATCH"
	MatchMethodDescription = "DESCRIPTION_SIMILARITY"
	PriceVarianceThreshold = 0.01 // 1%
)

// MatchedPair represents a matched invoice-PO line item pair
type MatchedPair struct {
	InvoiceItemID uuid.UUID `json:"invoice_item_id"`
	POItemID      uuid.UUID `json:"po_item_id"`
	Method        string    `json:"method"`
}

// LineItemMatchResult holds the result of a line item comparison
type LineItemMatchResult struct {
	FullyMatched     bool            `json:"fully_matched"`
	MatchedPairs     []MatchedPair   `json:"matched_pairs"`
	QtyMismatches    []uuid.UUID     `json:"qty_mismatches"`
	PriceVariances   []uuid.UUID     `json:"price_variances"`
	UnmatchedInvoice []uuid.UUID     `json:"unmatched_invoice"`
}

// ---------------------------------------------------------
// Orchestration Logic
// ---------------------------------------------------------

// RunMatchingForInvoice orchestrates the matching process for a single invoice.
// It is intended to be called asynchronously after validation.
func (e *MatchingEngine) RunMatchingForInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	// We do NOT use e.voucherRepo.WithTx(tx) immediately because matching might fail safely.
	// We will just do a standard flow. In a fully robust system we'd SELECT FOR UPDATE the PO.
	
	// 1. Fetch Invoice
	inv, err := e.voucherRepo.GetInvoiceByID(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to load invoice %s: %w", invoiceID, err)
	}

	// Idempotency check
	if inv.MatchStatus != voucher.InvoiceMatchPending {
		e.logger.InfoWith("Invoice already processed", "invoice_id", invoiceID, "status", inv.MatchStatus)
		return nil
	}

	// 2. Extract PO Number & Business
	if inv.PONumber == "" {
		e.logger.InfoWith("Invoice has no PO number", "invoice_id", invoiceID)
		matchResult := &voucher.MatchResult{
			InvoiceID:   invoiceID,
			MatchStatus: voucher.InvoiceMatchPending,
			MatchDetails: map[string]interface{}{"reason": "NO_PO_REFERENCE"},
		}
		_ = e.voucherRepo.CreateMatchResult(ctx, matchResult)
		return nil
	}

	var businessID uuid.UUID
	if inv.BusinessID != uuid.Nil {
		businessID = inv.BusinessID
	} else {
		return fmt.Errorf("invoice has no business ID")
	}

	// 3. Find PO
	po, err := e.voucherRepo.GetPurchaseOrderByPONumber(ctx, inv.PONumber, businessID)
	if err != nil {
		return fmt.Errorf("failed to lookup PO: %w", err)
	}

	if po == nil {
		e.logger.InfoWith("Matching PO not found (Orphan)", "invoice_id", invoiceID, "po_number", inv.PONumber)
		e.voucherRepo.UpdateInvoiceMatchStatus(ctx, invoiceID, voucher.InvoiceMatchOrphan)
		return nil // Successfully marked as orphan
	}

	// 4. Fetch line items
	invItems, err := e.voucherRepo.GetLineItemsByInvoiceID(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to fetch invoice items: %w", err)
	}

	poItems, err := e.voucherRepo.GetLineItemsByPurchaseOrderID(ctx, po.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch PO items: %w", err)
	}

	// 5. Run Pure Matching Logic
	amountOk := e.matchAmount(inv.NetAmount, po.RemainingBalance)
	lineMatch := e.matchLineItems(invItems, poItems)

	// Determine final status
	finalStatus := voucher.InvoiceMatchPending
	details := map[string]interface{}{}

	if !amountOk {
		details["reason"] = "AMOUNT_EXCEEDS_PO_BALANCE"
	}
	if !lineMatch.FullyMatched {
		details["line_items_match"] = lineMatch
	}

	// If header amount checks out, and we either have no line items to compare OR line items fully match
	if amountOk && lineMatch.FullyMatched {
		finalStatus = voucher.InvoiceMatchMatched
	}

	// 6. Apply Results
	audit := &voucher.MatchResult{
		InvoiceID:       invoiceID,
		PurchaseOrderID: &po.ID,
		MatchStatus:     finalStatus,
		MatchDetails:    details,
	}

	if finalStatus == voucher.InvoiceMatchMatched {
		// Update Invoice
		err = e.voucherRepo.UpdateInvoiceMatchStatus(ctx, invoiceID, voucher.InvoiceMatchMatched)
		if err != nil {
			return err
		}

		// Update PO Balance
		newBalance := po.RemainingBalance - inv.NetAmount
		newPoStatus := voucher.POStatusPartiallyInvoiced
		if newBalance <= 0 {
			newPoStatus = voucher.POStatusClosed
		}
		err = e.voucherRepo.UpdatePurchaseOrderBalance(ctx, po.ID, newBalance, newPoStatus)
		if err != nil {
			return err
		}

		// Update Line Items Qty
		for _, pair := range lineMatch.MatchedPairs {
			// Find the invoice item to get its quantity
			var matchedQty float64
			for _, item := range invItems {
				if item.ID == pair.InvoiceItemID {
					matchedQty = item.Quantity
					break
				}
			}
			if matchedQty > 0 {
				e.voucherRepo.UpdateLineItemQtyReceived(ctx, pair.POItemID, matchedQty)
			}
		}
	} else {
		// Just update the status to PENDING_MATCH (retaining it in review queue essentially)
		e.voucherRepo.UpdateInvoiceMatchStatus(ctx, invoiceID, voucher.InvoiceMatchPending)
	}

	// Always document the result
	e.voucherRepo.CreateMatchResult(ctx, audit)
	return nil
}

// RetryOrphanInvoices attempts to re-match invoices that were orphans when a matching PO is finally available.
func (e *MatchingEngine) RetryOrphanInvoices(ctx context.Context, poNumber string, businessID uuid.UUID) error {
	invoices, err := e.voucherRepo.GetOrphanInvoicesByPONumber(ctx, poNumber)
	if err != nil {
		return fmt.Errorf("failed to get orphan invoices for PO %s: %w", poNumber, err)
	}

	for _, inv := range invoices {
		// Ensure business matches
		if inv.BusinessID == businessID {
			e.logger.InfoWith("Retrying orphan invoice", "invoice_id", inv.ID)
			// Reset status to pending so RunMatchingForInvoice will process it
			e.voucherRepo.UpdateInvoiceMatchStatus(ctx, inv.ID, voucher.InvoiceMatchPending)
			
			// Detached matching run per invoice
			if err := e.RunMatchingForInvoice(ctx, inv.ID); err != nil {
				e.logger.ErrorWith("Failed executing retry matching", "invoice_id", inv.ID, "error", err)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------
// Pure Logic Engine
// ---------------------------------------------------------

// matchAmount is pure. Checks if invoice amount can fit in remaining balance.
func (e *MatchingEngine) matchAmount(invoiceNetAmount, poRemainingBalance float64) bool {
	// Allow a very tiny floating point epsilon margin if needed, but standard <= is fine for mostly integer representations
	return invoiceNetAmount <= poRemainingBalance
}

// matchLineItems compares invoice lines against PO lines
func (e *MatchingEngine) matchLineItems(invItems []voucher.LineItemDetail, poItems []voucher.LineItemDetail) *LineItemMatchResult {
	result := &LineItemMatchResult{
		FullyMatched:     true,
		MatchedPairs:     []MatchedPair{},
		QtyMismatches:    []uuid.UUID{},
		PriceVariances:   []uuid.UUID{},
		UnmatchedInvoice: []uuid.UUID{},
	}

	// If no items on either side, technically nothing to match line-item wise.
	// We consider it a "full match" at the header level.
	if len(invItems) == 0 || len(poItems) == 0 {
		return result
	}

	for _, invItem := range invItems {
		bestPOItem, method := e.findBestPOLineItem(invItem, poItems)

		if bestPOItem == nil {
			result.UnmatchedInvoice = append(result.UnmatchedInvoice, invItem.ID)
			result.FullyMatched = false
			continue
		}

		// We have a match, let's validate qty and price
		result.MatchedPairs = append(result.MatchedPairs, MatchedPair{
			InvoiceItemID: invItem.ID,
			POItemID:      bestPOItem.ID,
			Method:        method,
		})

		// Qty Check: Invoice Qty cannot exceed remaining PO Qty
		remainingPOQty := bestPOItem.Quantity - bestPOItem.QtyReceived
		if invItem.Quantity > remainingPOQty {
			result.QtyMismatches = append(result.QtyMismatches, invItem.ID)
			result.FullyMatched = false
		}

		// Price Check: 1% variance allowed
		poUnitPrice := float64(bestPOItem.UnitPrice)
		invUnitPrice := float64(invItem.UnitPrice)
		
		if poUnitPrice > 0 { // avoid division by zero
			diff := math.Abs(poUnitPrice - invUnitPrice)
			variance := diff / poUnitPrice
			if variance > PriceVarianceThreshold {
				result.PriceVariances = append(result.PriceVariances, invItem.ID)
				result.FullyMatched = false
			}
		} else if invUnitPrice > 0 {
			// PO unit price is 0 but invoice is not - definite variance
			result.PriceVariances = append(result.PriceVariances, invItem.ID)
			result.FullyMatched = false
		}
	}

	return result
}

// findBestPOLineItem identifies the corresponding PO line item using SKU or Description
func (e *MatchingEngine) findBestPOLineItem(invItem voucher.LineItemDetail, poItems []voucher.LineItemDetail) (*voucher.LineItemDetail, string) {
	// 1. Try exact SKU match first
	if invItem.SKU != "" {
		for _, poItem := range poItems {
			if strings.EqualFold(invItem.SKU, poItem.SKU) {
				return &poItem, MatchMethodSKU
			}
		}
	}

	// 2. Try Description semantic match (fallback)
	if invItem.Description != "" {
		var bestMatch *voucher.LineItemDetail
		highestSim := 0.0

		for _, poItem := range poItems {
			sim := e.calculateDescriptionSimilarity(invItem.Description, poItem.Description)
			if sim > highestSim && sim >= 0.70 { // 70% threshold
				highestSim = sim
				bestMatch = &poItem
			}
		}

		if bestMatch != nil {
			return bestMatch, MatchMethodDescription
		}
	}

	return nil, ""
}

// calculateDescriptionSimilarity computes a basic Jaccard index similarity score between two texts.
// Could be enhanced with Levenshtein for typos.
func (e *MatchingEngine) calculateDescriptionSimilarity(a, b string) float64 {
	aWords := strings.Fields(strings.ToLower(a))
	bWords := strings.Fields(strings.ToLower(b))

	if len(aWords) == 0 || len(bWords) == 0 {
		return 0.0
	}

	aMap := make(map[string]bool)
	for _, w := range aWords {
		aMap[w] = true
	}

	intersection := 0
	bMap := make(map[string]bool)
	for _, w := range bWords {
		if aMap[w] && !bMap[w] { // avoid counting duplicate b-words twice in intersection
			intersection++
		}
		bMap[w] = true
	}

	union := len(aMap) + len(bMap) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}
