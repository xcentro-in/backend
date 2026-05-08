package voucher

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/config"
	"xcentro.in/internal/shared/logger"
)

// MatchingEngine handles deterministic PO-Invoice matching
type MatchingEngine struct {
	voucherRepo voucher.VoucherRepository
	redisClient *redis.Client
	txManager   client.TxManager
	logger      *logger.Logger
	cfg         config.MatchingConfig
}

// NewMatchingEngine creates a new MatchingEngine instance with production defaults.
func NewMatchingEngine(
	voucherRepo voucher.VoucherRepository,
	redisClient *redis.Client,
	txManager client.TxManager,
	logger *logger.Logger,
) *MatchingEngine {
	return NewMatchingEngineWithConfig(
		voucherRepo, redisClient, txManager, logger, config.DefaultMatchingConfig(),
	)
}

// NewMatchingEngineWithConfig lets tests inject non-default tunables.
func NewMatchingEngineWithConfig(
	voucherRepo voucher.VoucherRepository,
	redisClient *redis.Client,
	txManager client.TxManager,
	logger *logger.Logger,
	cfg config.MatchingConfig,
) *MatchingEngine {
	return &MatchingEngine{
		voucherRepo: voucherRepo,
		redisClient: redisClient,
		txManager:   txManager,
		logger:      logger,
		cfg:         cfg,
	}
}

// acquireMatchLock SetNX-style locks an invoice for matching. Returns (true,
// nil) if we got the lock, (false, nil) if someone else holds it. Any redis
// error falls through with ok=false so callers skip the run.
func (e *MatchingEngine) acquireMatchLock(ctx context.Context, invoiceID uuid.UUID) (bool, error) {
	if e.redisClient == nil {
		return true, nil
	}
	key := fmt.Sprintf("xcentro:match_lock:%s", invoiceID)
	return e.redisClient.SetNX(ctx, key, "1", e.cfg.MatchLockTTL).Result()
}

func (e *MatchingEngine) releaseMatchLock(ctx context.Context, invoiceID uuid.UUID) {
	if e.redisClient == nil {
		return
	}
	e.redisClient.Del(ctx, fmt.Sprintf("xcentro:match_lock:%s", invoiceID))
}

// matchingQueue is the list key consumed by the async matching worker.
// Keeping payloads self-describing with a "kind" tag lets us add new
// task shapes (orphan retry, periodic re-match, etc.) without migrating
// the queue format.
const matchingQueue = "xcentro:matching_tasks"

type matchTask struct {
	Kind       string    `json:"kind"` // "invoice" | "orphan_retry"
	InvoiceID  uuid.UUID `json:"invoice_id,omitempty"`
	PONumber   string    `json:"po_number,omitempty"`
	BusinessID uuid.UUID `json:"business_id,omitempty"`
}

// EnqueueInvoiceMatch pushes a matching task so the worker pool — rather than
// the request goroutine — runs RunMatchingForInvoice. The HTTP handler returns
// as soon as validation persists; matching happens out-of-band.
func (e *MatchingEngine) EnqueueInvoiceMatch(ctx context.Context, invoiceID uuid.UUID) error {
	return e.enqueueTask(ctx, matchTask{Kind: "invoice", InvoiceID: invoiceID})
}

// EnqueueOrphanRetry pushes an orphan-reconciliation task for a newly-arrived PO.
func (e *MatchingEngine) EnqueueOrphanRetry(ctx context.Context, poNumber string, businessID uuid.UUID) error {
	return e.enqueueTask(ctx, matchTask{Kind: "orphan_retry", PONumber: poNumber, BusinessID: businessID})
}

func (e *MatchingEngine) enqueueTask(ctx context.Context, t matchTask) error {
	if e.redisClient == nil {
		return fmt.Errorf("redis client not configured")
	}
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal match task: %w", err)
	}
	return e.redisClient.LPush(ctx, matchingQueue, body).Err()
}

// StartMatchingWorker runs a BRPOP loop on matchingQueue until ctx is done.
// Meant to be launched with go in main. Single-worker is sufficient today —
// per-invoice Redis lock guarantees correctness if we scale to N workers later.
func (e *MatchingEngine) StartMatchingWorker(ctx context.Context) {
	if e.redisClient == nil {
		e.logger.Warn("Matching worker not started: no redis client")
		return
	}
	e.logger.InfoWith("Matching worker started", "queue", matchingQueue)
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := e.redisClient.BRPop(ctx, e.cfg.BRPopTimeout, matchingQueue).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Timeout on empty queue is the common path — BRPop returns redis.Nil.
			continue
		}
		if len(res) < 2 {
			continue
		}
		var t matchTask
		if jsonErr := json.Unmarshal([]byte(res[1]), &t); jsonErr != nil {
			e.logger.ErrorWith("Malformed matching task", "error", jsonErr, "raw", res[1])
			continue
		}
		e.handleTask(ctx, t)
	}
}

func (e *MatchingEngine) handleTask(ctx context.Context, t matchTask) {
	taskCtx, cancel := context.WithTimeout(ctx, e.cfg.HandleTaskTimeout)
	defer cancel()
	switch t.Kind {
	case "invoice":
		if err := e.RunMatchingForInvoice(taskCtx, t.InvoiceID); err != nil {
			e.logger.ErrorWith("Matching failed", "error", err, "invoice_id", t.InvoiceID)
		}
	case "orphan_retry":
		if err := e.RetryOrphanInvoices(taskCtx, t.PONumber, t.BusinessID); err != nil {
			e.logger.ErrorWith("Orphan retry failed", "error", err, "po_number", t.PONumber)
		}
	default:
		e.logger.WarnWith("Unknown matching task kind", "kind", t.Kind)
	}
}

// Match-method identifiers are re-exported from config so the rest of the
// package (and tests) can reference them without importing the config pkg.
const (
	MatchMethodSKU         = config.MatchMethodSKU
	MatchMethodDescription = config.MatchMethodDescription
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
// Concurrency / idempotency contract:
//   - A per-invoice Redis lock ensures only one worker runs the write path at a time.
//   - A second status check inside the lock prevents two successive runs (e.g. a retry
//     racing a newly-arrived orphan sweep) from both decrementing the PO balance and
//     double-incrementing qty_received.
//   - All state mutations are wrapped in a single DB transaction, so a failure
//     half-way through leaves invoice/PO/line-items consistent.
func (e *MatchingEngine) RunMatchingForInvoice(ctx context.Context, invoiceID uuid.UUID) error {
	locked, err := e.acquireMatchLock(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("match lock acquire: %w", err)
	}
	if !locked {
		e.logger.InfoWith("Skipping match: another worker holds the lock", "invoice_id", invoiceID)
		return nil
	}
	defer e.releaseMatchLock(ctx, invoiceID)

	inv, err := e.voucherRepo.GetInvoiceByID(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to load invoice %s: %w", invoiceID, err)
	}
	if inv.MatchStatus != voucher.InvoiceMatchPending {
		e.logger.InfoWith("Invoice already processed", "invoice_id", invoiceID, "status", inv.MatchStatus)
		return nil
	}

	if inv.BusinessID == uuid.Nil {
		return fmt.Errorf("invoice has no business ID")
	}
	businessID := inv.BusinessID

	// po_number may be a comma-separated list when one invoice references
	// multiple POs. Resolve every reference, then match invoice line items
	// against the union — a single invoice line can legitimately belong to
	// any of the referenced POs (many-to-many procurement flow).
	candidates := parsePONumbers(inv.PONumber)
	if len(candidates) == 0 {
		e.logger.InfoWith("Invoice has no PO number", "invoice_id", invoiceID)
		if err := e.voucherRepo.UpdateInvoiceMatchStatus(ctx, invoiceID, voucher.InvoiceMatchOrphan); err != nil {
			return fmt.Errorf("mark orphan: %w", err)
		}
		_ = e.voucherRepo.CreateMatchResult(ctx, &voucher.MatchResult{
			InvoiceID:    invoiceID,
			MatchStatus:  voucher.InvoiceMatchOrphan,
			MatchDetails: map[string]interface{}{"reason": "NO_PO_REFERENCE"},
		})
		return nil
	}

	invItems, err := e.voucherRepo.GetLineItemsByInvoiceID(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to fetch invoice items: %w", err)
	}

	var resolvedPOs []*voucher.PurchaseOrder
	var unionPOItems []voucher.LineItemDetail
	poByItemID := make(map[uuid.UUID]*voucher.PurchaseOrder)

	for _, poNum := range candidates {
		po, err := e.getPOCached(ctx, poNum, businessID)
		if err != nil {
			return fmt.Errorf("failed to lookup PO %q: %w", poNum, err)
		}
		if po == nil {
			continue
		}
		resolvedPOs = append(resolvedPOs, po)
		items, err := e.voucherRepo.GetLineItemsByPurchaseOrderID(ctx, po.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch PO items for %q: %w", poNum, err)
		}
		for i := range items {
			poByItemID[items[i].ID] = po
		}
		unionPOItems = append(unionPOItems, items...)
	}

	if len(resolvedPOs) == 0 {
		e.logger.InfoWith("Matching PO not found (Orphan)", "invoice_id", invoiceID, "po_number", inv.PONumber)
		return e.voucherRepo.UpdateInvoiceMatchStatus(ctx, invoiceID, voucher.InvoiceMatchOrphan)
	}

	lineMatch := e.matchLineItems(invItems, unionPOItems)

	// Attribute each invoice line's value to whichever PO owns the matched
	// PO line item. This per-line attribution is what lets us split the
	// invoice's net across POs when the match genuinely spans multiple orders.
	invByID := make(map[uuid.UUID]*voucher.LineItemDetail, len(invItems))
	for i := range invItems {
		invByID[invItems[i].ID] = &invItems[i]
	}
	perPOAttribution := make(map[uuid.UUID]float64, len(resolvedPOs))
	for _, pair := range lineMatch.MatchedPairs {
		owner := poByItemID[pair.POItemID]
		invLine := invByID[pair.InvoiceItemID]
		if owner == nil || invLine == nil {
			continue
		}
		amt := invLine.TotalAmount
		if amt == 0 {
			amt = invLine.Quantity * invLine.UnitPrice
		}
		perPOAttribution[owner.ID] += amt
	}

	// Primary PO: the one with the largest attributed share. Used for the
	// single po_id FK on invoice and the single PurchaseOrderID on the audit
	// row. Ties broken by input order for determinism.
	primary := resolvedPOs[0]
	var maxAttrib float64
	for _, po := range resolvedPOs {
		if perPOAttribution[po.ID] > maxAttrib {
			maxAttrib = perPOAttribution[po.ID]
			primary = po
		}
	}

	// Amount check fits the invoice against the *combined* remaining balance.
	var totalBalance float64
	for _, po := range resolvedPOs {
		totalBalance += po.RemainingBalance
	}
	amountOk := e.matchAmount(inv.NetAmount, totalBalance)

	referencedPONumbers := make([]string, 0, len(resolvedPOs))
	for _, po := range resolvedPOs {
		referencedPONumbers = append(referencedPONumbers, po.PONumber)
	}

	finalStatus := voucher.InvoiceMatchPending
	details := map[string]interface{}{
		"line_items_match":              lineMatch,
		"po_remaining_balance_at_match": totalBalance,
		"referenced_po_numbers":         referencedPONumbers,
	}
	if !amountOk {
		details["reason"] = "AMOUNT_EXCEEDS_PO_BALANCE"
	}
	if amountOk && lineMatch.FullyMatched {
		finalStatus = voucher.InvoiceMatchMatched
	}

	audit := &voucher.MatchResult{
		InvoiceID:       invoiceID,
		PurchaseOrderID: &primary.ID,
		MatchStatus:     finalStatus,
		MatchDetails:    details,
	}

	return e.applyMatchResult(ctx, inv, resolvedPOs, primary, perPOAttribution, invItems, lineMatch, finalStatus, audit, businessID)
}

// applyMatchResult writes all match mutations inside one tx. A status re-check
// inside the tx is the final line of defence against concurrent runs that
// slipped past the redis lock (e.g. lock TTL expiry mid-flight).
//
// For multi-PO matches, `perPOAttribution` specifies how much of the invoice's
// net each PO absorbs. Per-line amounts often don't sum to the invoice net
// (invoice-level taxes, discounts, rounding), so any delta falls on the primary
// PO. Each PO's balance and status update independently; qty_received updates
// are already per-line so they naturally route to the correct PO.
func (e *MatchingEngine) applyMatchResult(
	ctx context.Context,
	inv *voucher.ExtractedInvoice,
	resolvedPOs []*voucher.PurchaseOrder,
	primary *voucher.PurchaseOrder,
	perPOAttribution map[uuid.UUID]float64,
	invItems []voucher.LineItemDetail,
	lineMatch *LineItemMatchResult,
	finalStatus string,
	audit *voucher.MatchResult,
	businessID uuid.UUID,
) error {
	if e.txManager == nil {
		return fmt.Errorf("matching engine requires txManager")
	}

	return e.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
		txRepo := e.voucherRepo.WithTx(tx)

		// Re-verify status under tx to close the lock-expiry race.
		current, err := txRepo.GetInvoiceByID(ctx, inv.ID)
		if err != nil {
			return fmt.Errorf("reload invoice: %w", err)
		}
		if current.MatchStatus != voucher.InvoiceMatchPending {
			e.logger.InfoWith("Invoice state changed under us; abandoning match",
				"invoice_id", inv.ID, "status", current.MatchStatus)
			return nil
		}

		if finalStatus == voucher.InvoiceMatchMatched {
			if err := txRepo.UpdateInvoiceMatchStatus(ctx, inv.ID, voucher.InvoiceMatchMatched); err != nil {
				return fmt.Errorf("set match status: %w", err)
			}
			if err := txRepo.UpdateInvoicePOID(ctx, inv.ID, primary.ID); err != nil {
				return fmt.Errorf("link po_id: %w", err)
			}

			// Reconcile per-line totals against invoice net; the delta
			// (typically taxes/discounts applied at invoice level) goes to
			// the primary PO so the total decrement equals inv.NetAmount.
			var attributed float64
			for _, amt := range perPOAttribution {
				attributed += amt
			}
			if delta := inv.NetAmount - attributed; delta != 0 {
				perPOAttribution[primary.ID] += delta
			}

			for _, po := range resolvedPOs {
				charge := perPOAttribution[po.ID]
				if charge == 0 {
					continue
				}
				newBalance := po.RemainingBalance - charge
				newPoStatus := voucher.POStatusPartiallyInvoiced
				if newBalance <= e.cfg.RoundingToleranceRupees {
					newPoStatus = voucher.POStatusClosed
					newBalance = 0
				}
				if err := txRepo.UpdatePurchaseOrderBalance(ctx, po.ID, newBalance, newPoStatus); err != nil {
					return fmt.Errorf("update po %s balance: %w", po.ID, err)
				}
			}

			for _, pair := range lineMatch.MatchedPairs {
				var qty float64
				for _, item := range invItems {
					if item.ID == pair.InvoiceItemID {
						qty = item.Quantity
						break
					}
				}
				if qty > 0 {
					if err := txRepo.UpdateLineItemQtyReceived(ctx, pair.POItemID, qty); err != nil {
						return fmt.Errorf("update line qty_received: %w", err)
					}
				}
			}
		} else {
			if err := txRepo.UpdateInvoiceMatchStatus(ctx, inv.ID, voucher.InvoiceMatchMismatch); err != nil {
				return fmt.Errorf("set mismatch status: %w", err)
			}
			audit.MatchStatus = voucher.InvoiceMatchMismatch
		}

		if err := txRepo.CreateMatchResult(ctx, audit); err != nil {
			return fmt.Errorf("create match result: %w", err)
		}

		// Best-effort cache invalidation *after* the tx conceptually commits.
		// Each referenced PO had a cache entry keyed by its own po_number.
		for _, po := range resolvedPOs {
			e.invalidatePOCache(ctx, po.PONumber, businessID)
		}
		return nil
	})
}

// RetryOrphanInvoices attempts to re-match invoices that were orphans when a matching PO is finally available.
func (e *MatchingEngine) RetryOrphanInvoices(ctx context.Context, poNumber string, businessID uuid.UUID) error {
	invoices, err := e.voucherRepo.GetOrphanInvoicesByPONumber(ctx, poNumber, businessID)
	if err != nil {
		return fmt.Errorf("failed to get orphan invoices for PO %s: %w", poNumber, err)
	}

	for _, inv := range invoices {
		e.logger.InfoWith("Retrying orphan invoice", "invoice_id", inv.ID)
		// Reset status to pending so RunMatchingForInvoice will process it
		e.voucherRepo.UpdateInvoiceMatchStatus(ctx, inv.ID, voucher.InvoiceMatchPending)

		if err := e.RunMatchingForInvoice(ctx, inv.ID); err != nil {
			e.logger.ErrorWith("Failed executing retry matching", "invoice_id", inv.ID, "error", err)
		}
	}
	return nil
}

// ---------------------------------------------------------
// Pure Logic Engine
// ---------------------------------------------------------

// matchAmount is pure. Checks if invoice amount can fit in remaining balance.
func (e *MatchingEngine) matchAmount(invoiceNetAmount, poRemainingBalance float64) bool {
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
		poUnitPrice := bestPOItem.UnitPrice
		invUnitPrice := invItem.UnitPrice
		
		if poUnitPrice > 0 { // avoid division by zero
			diff := math.Abs(poUnitPrice - invUnitPrice)
			variance := diff / poUnitPrice
			if variance > e.cfg.PriceVarianceThreshold {
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

// findBestPOLineItem identifies the corresponding PO line item using SKU or Description.
//
// When the invoice SKU matches a PO SKU we take that directly. Otherwise we
// score description candidates by (similarity * 10) + qty-fits + price-fits,
// which disambiguates duplicate-description PO lines (e.g. the same vendor
// shipping the same part across two open POs) by preferring the candidate
// whose remaining qty absorbs the invoice qty and whose unit price is within
// variance. Plain first-wins was the earlier behavior, but that silently
// latched onto whichever duplicate appeared first in the union, producing
// spurious qty/price mismatches when a better-fitting line existed.
func (e *MatchingEngine) findBestPOLineItem(invItem voucher.LineItemDetail, poItems []voucher.LineItemDetail) (*voucher.LineItemDetail, string) {
	if invItem.SKU != "" {
		for i := range poItems {
			if strings.EqualFold(invItem.SKU, poItems[i].SKU) {
				return &poItems[i], MatchMethodSKU
			}
		}
	}

	if invItem.Description == "" {
		return nil, ""
	}

	var best *voucher.LineItemDetail
	bestScore := -1.0
	for i := range poItems {
		poItem := &poItems[i]
		sim := e.calculateDescriptionSimilarity(invItem.Description, poItem.Description)
		if sim < e.cfg.DescriptionSimilarityThreshold {
			continue
		}
		score := sim * 10
		if invItem.Quantity <= (poItem.Quantity - poItem.QtyReceived) {
			score++
		}
		if poItem.UnitPrice > 0 {
			variance := math.Abs(poItem.UnitPrice-invItem.UnitPrice) / poItem.UnitPrice
			if variance <= e.cfg.PriceVarianceThreshold {
				score++
			}
		} else if invItem.UnitPrice == 0 {
			score++
		}
		if score > bestScore {
			best = poItem
			bestScore = score
		}
	}
	if best == nil {
		return nil, ""
	}
	return best, MatchMethodDescription
}

// ---------------------------------------------------------
// Redis PO Cache Helpers
// ---------------------------------------------------------

func poCacheKey(poNumber string, businessID uuid.UUID) string {
	return fmt.Sprintf("xcentro:po_cache:%s:%s", poNumber, businessID)
}

// getPOCached fetches a PO from Redis cache, falling back to the database on a miss.
func (e *MatchingEngine) getPOCached(ctx context.Context, poNumber string, businessID uuid.UUID) (*voucher.PurchaseOrder, error) {
	if e.redisClient != nil {
		key := poCacheKey(poNumber, businessID)
		raw, err := e.redisClient.Get(ctx, key).Bytes()
		if err == nil {
			var po voucher.PurchaseOrder
			if jsonErr := json.Unmarshal(raw, &po); jsonErr == nil {
				return &po, nil
			}
		}
	}

	po, err := e.voucherRepo.GetPurchaseOrderByPONumber(ctx, poNumber, businessID)
	if err != nil || po == nil {
		return po, err
	}

	// Populate cache on miss
	if e.redisClient != nil {
		if raw, jsonErr := json.Marshal(po); jsonErr == nil {
			e.redisClient.Set(ctx, poCacheKey(poNumber, businessID), raw, e.cfg.POCacheTTL)
		}
	}
	return po, nil
}

// invalidatePOCache removes a PO entry from the cache after a balance update.
func (e *MatchingEngine) invalidatePOCache(ctx context.Context, poNumber string, businessID uuid.UUID) {
	if e.redisClient != nil {
		e.redisClient.Del(ctx, poCacheKey(poNumber, businessID))
	}
}

// parsePONumbers splits a potentially comma-separated po_number field into
// individual candidate PO references. OCR/LLM extraction sometimes concatenates
// multiple PO numbers into the single po_number column (e.g. "PO-A, PO-B"), so
// the matching engine must iterate over each token rather than treat the raw
// string as a single key. Input order is preserved and duplicates are dropped.
func parsePONumbers(raw string) []string {
	if raw == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(raw, ",") {
		token := strings.TrimSpace(part)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
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
