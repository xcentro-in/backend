package config

import "time"

// MatchingConfig holds tunables for the voucher PO-Invoice matching engine.
//
// These were previously hard-coded constants across matching_engine.go; they
// are lifted here so test runs can override them (e.g. a shorter lock TTL in
// concurrent tests) and so ops can tune variance / cache behaviour without
// rebuilding the binary.
type MatchingConfig struct {
	// MatchLockTTL bounds how long a single matching attempt may hold its
	// per-invoice Redis lock. Longer than any realistic happy-path run; short
	// enough that a crashed worker clears within minutes.
	MatchLockTTL time.Duration

	// POCacheTTL is how long a PO lookup is cached in Redis before expiring.
	// Keep short — balance changes on every successful match, and stale
	// reads would be swallowed by the in-tx re-check but waste a round trip.
	POCacheTTL time.Duration

	// PriceVarianceThreshold is the allowed fractional difference between a
	// PO line item's unit price and an invoice's unit price before we flag a
	// price variance. 0.01 == 1 %.
	PriceVarianceThreshold float64

	// DescriptionSimilarityThreshold is the minimum Jaccard similarity score
	// required to accept a description-based line-item match.
	DescriptionSimilarityThreshold float64

	// HandleTaskTimeout caps how long a single queued matching task may run
	// before its context is cancelled.
	HandleTaskTimeout time.Duration

	// BRPopTimeout is the block duration for BRPOP polling of the matching
	// queue. Longer reduces idle CPU; shorter shortens graceful-shutdown wait.
	BRPopTimeout time.Duration

	// RoundingToleranceRupees is the maximum residual PO balance (in rupees)
	// that is treated as a GST rounding artefact and closed automatically.
	// Indian invoicing commonly produces ₹0–₹1 differences when GST is rounded
	// per-line rather than on the aggregate. Default is ₹2.
	RoundingToleranceRupees float64
}

// Match-method identifiers. Typed constants so handlers / audit readers can
// pattern-match without stringly-typed comparisons.
const (
	MatchMethodSKU         = "SKU_MATCH"
	MatchMethodDescription = "DESCRIPTION_SIMILARITY"
)

// DefaultMatchingConfig returns the production defaults. Tests should call
// this and override only the fields they need to tweak.
func DefaultMatchingConfig() MatchingConfig {
	return MatchingConfig{
		MatchLockTTL:                   2 * time.Minute,
		POCacheTTL:                     5 * time.Minute,
		PriceVarianceThreshold:         0.01,
		DescriptionSimilarityThreshold: 0.70,
		HandleTaskTimeout:              2 * time.Minute,
		BRPopTimeout:                   5 * time.Second,
		RoundingToleranceRupees:        2.0,
	}
}
