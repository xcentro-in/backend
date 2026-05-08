package voucher

import (
	"testing"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/shared/config"
)

func newTestEngine() *MatchingEngine {
	return &MatchingEngine{cfg: config.DefaultMatchingConfig()}
}

func TestMatchAmount(t *testing.T) {
	e := newTestEngine()
	cases := []struct {
		name    string
		invoice float64
		balance float64
		want    bool
	}{
		{"invoice below balance", 500_00, 1000_00, true},
		{"invoice equals balance", 1000_00, 1000_00, true},
		{"invoice exceeds balance by one paisa", 1000_01, 1000_00, false},
		{"zero invoice always fits", 0, 0, true},
		{"negative balance never fits positive invoice", 1, -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := e.matchAmount(tc.invoice, tc.balance); got != tc.want {
				t.Errorf("matchAmount(%v, %v) = %v, want %v", tc.invoice, tc.balance, got, tc.want)
			}
		})
	}
}

func TestCalculateDescriptionSimilarity(t *testing.T) {
	e := newTestEngine()

	if s := e.calculateDescriptionSimilarity("A4 Printer Paper 500 sheets", "A4 Printer Paper 500 sheets"); s != 1.0 {
		t.Errorf("identical strings should yield 1.0, got %v", s)
	}
	if s := e.calculateDescriptionSimilarity("", "anything"); s != 0.0 {
		t.Errorf("empty input should yield 0.0, got %v", s)
	}
	if s := e.calculateDescriptionSimilarity("apple", "banana"); s != 0.0 {
		t.Errorf("fully disjoint words should yield 0.0, got %v", s)
	}
	// Case-insensitive token overlap — "A4 printer paper" vs "a4 PRINTER PAPER" → full overlap.
	if s := e.calculateDescriptionSimilarity("A4 printer paper", "a4 PRINTER PAPER"); s != 1.0 {
		t.Errorf("case-insensitive match should yield 1.0, got %v", s)
	}
	// Partial overlap: {"a4","paper"} vs {"a4","box"} → 1 common / 3 union = 0.333…
	sim := e.calculateDescriptionSimilarity("a4 paper", "a4 box")
	if sim <= 0.0 || sim >= 1.0 {
		t.Errorf("partial overlap should be between 0 and 1, got %v", sim)
	}
}

func TestFindBestPOLineItemSKUPreferredOverDescription(t *testing.T) {
	e := newTestEngine()
	skuMatch := voucher.LineItemDetail{ID: uuid.New(), SKU: "SKU-1", Description: "Totally different wording"}
	descMatch := voucher.LineItemDetail{ID: uuid.New(), SKU: "SKU-2", Description: "A4 printer paper 500 sheets"}

	inv := voucher.LineItemDetail{SKU: "SKU-1", Description: "A4 printer paper 500 sheets"}
	got, method := e.findBestPOLineItem(inv, []voucher.LineItemDetail{descMatch, skuMatch})
	if got == nil || got.ID != skuMatch.ID {
		t.Fatalf("expected SKU match to win; got=%v", got)
	}
	if method != MatchMethodSKU {
		t.Errorf("expected method=%q, got %q", MatchMethodSKU, method)
	}
}

func TestFindBestPOLineItemDescriptionFallback(t *testing.T) {
	e := newTestEngine()
	poItem := voucher.LineItemDetail{ID: uuid.New(), Description: "A4 printer paper 500 sheets"}
	noMatch := voucher.LineItemDetail{ID: uuid.New(), Description: "Stapler pin box"}

	inv := voucher.LineItemDetail{Description: "a4 printer paper 500 sheets"}
	got, method := e.findBestPOLineItem(inv, []voucher.LineItemDetail{noMatch, poItem})
	if got == nil || got.ID != poItem.ID {
		t.Fatalf("expected description match; got=%v", got)
	}
	if method != MatchMethodDescription {
		t.Errorf("expected method=%q, got %q", MatchMethodDescription, method)
	}
}

func TestFindBestPOLineItemDescriptionTiePrefersQtyAndPriceFit(t *testing.T) {
	// Two PO lines with identical descriptions — the old first-wins rule would
	// latch onto whichever one appeared first in the union, even if the other
	// was a better fit on qty+price. The ranker should prefer the one that
	// absorbs the invoice qty and matches price.
	e := newTestEngine()
	firstButWrong := voucher.LineItemDetail{
		ID: uuid.New(), SKU: "KNK-6087",
		Description: "Aluminium Sheet 1mmx1mx2m",
		Quantity:    7, UnitPrice: 1090,
	}
	secondAndBetter := voucher.LineItemDetail{
		ID: uuid.New(), SKU: "XKF-7290",
		Description: "Aluminium Sheet 1mmx1mx2m",
		Quantity:    30, UnitPrice: 4818,
	}
	inv := voucher.LineItemDetail{
		Description: "Aluminium Sheet 1mmx1mx2m",
		Quantity:    30, UnitPrice: 4818,
	}
	got, method := e.findBestPOLineItem(inv, []voucher.LineItemDetail{firstButWrong, secondAndBetter})
	if got == nil || got.ID != secondAndBetter.ID {
		t.Fatalf("expected better-fitting PO line to win; got %+v", got)
	}
	if method != MatchMethodDescription {
		t.Errorf("expected method=%q, got %q", MatchMethodDescription, method)
	}
}

func TestFindBestPOLineItemBelowThresholdReturnsNil(t *testing.T) {
	e := newTestEngine()
	poItems := []voucher.LineItemDetail{
		{ID: uuid.New(), Description: "Stapler pins box of 1000"},
	}
	inv := voucher.LineItemDetail{Description: "A4 printer paper"}
	got, method := e.findBestPOLineItem(inv, poItems)
	if got != nil {
		t.Errorf("expected no match below threshold; got %v method=%s", got, method)
	}
}

func TestMatchLineItemsFullMatch(t *testing.T) {
	e := newTestEngine()
	poID := uuid.New()
	invID := uuid.New()
	po := []voucher.LineItemDetail{{ID: poID, SKU: "SKU-1", Quantity: 10, QtyReceived: 0, UnitPrice: 10000}}
	inv := []voucher.LineItemDetail{{ID: invID, SKU: "SKU-1", Quantity: 5, UnitPrice: 10000}}

	res := e.matchLineItems(inv, po)
	if !res.FullyMatched {
		t.Errorf("expected FullyMatched; got %+v", res)
	}
	if len(res.MatchedPairs) != 1 {
		t.Fatalf("expected 1 matched pair; got %d", len(res.MatchedPairs))
	}
	if len(res.QtyMismatches)+len(res.PriceVariances)+len(res.UnmatchedInvoice) != 0 {
		t.Errorf("expected no mismatches; got %+v", res)
	}
}

func TestMatchLineItemsQtyExceedsRemaining(t *testing.T) {
	e := newTestEngine()
	po := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 10, QtyReceived: 8, UnitPrice: 10000}}
	inv := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 5, UnitPrice: 10000}}

	res := e.matchLineItems(inv, po)
	if res.FullyMatched {
		t.Errorf("qty > remaining should break full match")
	}
	if len(res.QtyMismatches) != 1 {
		t.Errorf("expected 1 qty mismatch; got %d", len(res.QtyMismatches))
	}
}

func TestMatchLineItemsPriceVariance(t *testing.T) {
	e := newTestEngine()
	// 5% price hike — well beyond the 1% default threshold.
	po := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 10, UnitPrice: 10000}}
	inv := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 5, UnitPrice: 10500}}

	res := e.matchLineItems(inv, po)
	if res.FullyMatched {
		t.Errorf("price variance > threshold should break full match")
	}
	if len(res.PriceVariances) != 1 {
		t.Errorf("expected 1 price variance; got %d", len(res.PriceVariances))
	}
}

func TestMatchLineItemsPriceWithinThreshold(t *testing.T) {
	e := newTestEngine()
	// 0.5% — within the 1% default threshold.
	po := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 10, UnitPrice: 10000}}
	inv := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 5, UnitPrice: 10050}}

	res := e.matchLineItems(inv, po)
	if !res.FullyMatched {
		t.Errorf("price within threshold should keep full match; got %+v", res)
	}
}

func TestMatchLineItemsUnmatchedInvoiceLine(t *testing.T) {
	e := newTestEngine()
	po := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-1", Quantity: 10, UnitPrice: 10000}}
	inv := []voucher.LineItemDetail{{ID: uuid.New(), SKU: "SKU-99", Description: "gibberish nobody wrote", Quantity: 5, UnitPrice: 10000}}

	res := e.matchLineItems(inv, po)
	if res.FullyMatched {
		t.Errorf("unmatched invoice line should break full match")
	}
	if len(res.UnmatchedInvoice) != 1 {
		t.Errorf("expected 1 unmatched invoice line; got %d", len(res.UnmatchedInvoice))
	}
}

func TestMatchLineItemsEmptySidesAreHeaderMatch(t *testing.T) {
	e := newTestEngine()
	res := e.matchLineItems(nil, nil)
	if !res.FullyMatched {
		t.Errorf("no line items on either side should be a header-level full match")
	}
}

func TestParsePONumbers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "PO-A", []string{"PO-A"}},
		{"single with surrounding spaces", "  PO-A  ", []string{"PO-A"}},
		{"two comma-separated", "PO-A,PO-B", []string{"PO-A", "PO-B"}},
		{"two with spaces (regression case)", "PO-2025-1797-002, PO-2026-9367-001", []string{"PO-2025-1797-002", "PO-2026-9367-001"}},
		{"trailing comma", "PO-A,", []string{"PO-A"}},
		{"leading comma", ",PO-A", []string{"PO-A"}},
		{"duplicates deduped, order preserved", "PO-A, PO-B, PO-A", []string{"PO-A", "PO-B"}},
		{"interior empty tokens dropped", "PO-A,,PO-B", []string{"PO-A", "PO-B"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePONumbers(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePONumbers(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parsePONumbers(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}
