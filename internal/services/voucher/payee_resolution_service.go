package voucher

import (
	"context"
	"fmt"

	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/shared/logger"
)

// PayeeResolutionStatus indicates the outcome of a payee resolution attempt
type PayeeResolutionStatus string

const (
	PayeeResolutionVerified PayeeResolutionStatus = "VERIFIED"
	PayeeResolutionNotFound PayeeResolutionStatus = "NOT_FOUND"
)

// PayeeResolutionConfidence indicates how confident the match is
type PayeeResolutionConfidence string

const (
	PayeeResolutionConfidenceHigh PayeeResolutionConfidence = "HIGH"
	PayeeResolutionConfidenceLow  PayeeResolutionConfidence = "LOW"
)

// PayeeResolution holds the result of attempting to resolve a payee from extracted data
type PayeeResolution struct {
	PayeeID       string                    `json:"payee_id"`
	Status        PayeeResolutionStatus     `json:"status"`
	Confidence    PayeeResolutionConfidence `json:"confidence"`
	MatchedVia    string                    `json:"matched_via"` // "gstin" or "name_fuzzy"
	SuggestedData map[string]any            `json:"suggested_data,omitempty"`
}

// BusinessResolution holds the result of resolving the business_id
type BusinessResolution struct {
	BusinessID  string `json:"business_id"`
	ResolvedVia string `json:"resolved_via"` // "header", "gstin_match", or "unresolved"
}

// PayeeResolutionService resolves payees and businesses from extracted document data
type PayeeResolutionService struct {
	vendorRepo payee.PayeeVendorRepository
	logger     *logger.Logger
}

// NewPayeeResolutionService creates a new PayeeResolutionService
func NewPayeeResolutionService(
	vendorRepo payee.PayeeVendorRepository,
	logger *logger.Logger,
) *PayeeResolutionService {
	return &PayeeResolutionService{
		vendorRepo: vendorRepo,
		logger:     logger,
	}
}

// ResolvePayee attempts to match a vendor from the extracted vendor_name and vendor_gstin.
// 1. Exact match on GSTIN → HIGH confidence
// 2. Fuzzy match on vendor_name via ILIKE → LOW confidence
// 3. No match → NOT_FOUND
func (s *PayeeResolutionService) ResolvePayee(ctx context.Context, vendorName, gstin string) *PayeeResolution {
	// Step 1: Try exact GSTIN match
	if gstin != "" {
		vendor, err := s.vendorRepo.FindByGSTIN(ctx, gstin)
		if err != nil {
			s.logger.WarnWith("Payee GSTIN lookup error", "gstin", gstin, "error", err)
		}
		if vendor != nil {
			s.logger.InfoWith("Payee resolved via GSTIN",
				"gstin", gstin, "payee_id", vendor.PayeeID, "legal_name", vendor.LegalName)
			return &PayeeResolution{
				PayeeID:    vendor.PayeeID,
				Status:     PayeeResolutionVerified,
				Confidence: PayeeResolutionConfidenceHigh,
				MatchedVia: "gstin",
				SuggestedData: map[string]any{
					"legal_name":  vendor.LegalName,
					"gst_status":  vendor.GSTStatus,
					"vendor_gstin": vendor.GSTIN,
				},
			}
		}
		s.logger.InfoWith("No payee found for GSTIN, falling back to name match", "gstin", gstin)
	}

	// Step 2: Fuzzy name match via ILIKE on legal_name
	if vendorName != "" {
		results, err := s.vendorRepo.Search(ctx, payee.VendorSearchFilters{
			LegalName: vendorName,
			Limit:     1,
		})
		if err != nil {
			s.logger.WarnWith("Payee name search error", "vendor_name", vendorName, "error", err)
		}
		if len(results) > 0 {
			matched := results[0]
			s.logger.InfoWith("Payee resolved via fuzzy name match",
				"vendor_name", vendorName, "payee_id", matched.PayeeID, "legal_name", matched.LegalName)
			return &PayeeResolution{
				PayeeID:    matched.PayeeID,
				Status:     PayeeResolutionVerified,
				Confidence: PayeeResolutionConfidenceLow,
				MatchedVia: "name_fuzzy",
				SuggestedData: map[string]any{
					"legal_name":  matched.LegalName,
					"gst_status":  matched.GSTStatus,
					"vendor_gstin": matched.GSTIN,
				},
			}
		}
		s.logger.InfoWith("No payee found via name match", "vendor_name", vendorName)
	}

	// Step 3: No match found
	s.logger.InfoWith("Payee resolution: NOT_FOUND", "vendor_name", vendorName, "gstin", gstin)
	return &PayeeResolution{
		Status:     PayeeResolutionNotFound,
		Confidence: PayeeResolutionConfidenceLow,
		MatchedVia: "",
	}
}

// ResolveBusiness determines the business_id for a document.
// Priority: explicit header > extracted buyer_gstin match against xcentro.business table
func (s *PayeeResolutionService) ResolveBusiness(ctx context.Context, headerBusinessID string, extractedBuyerGSTIN string, businessLookup func(ctx context.Context, gstin string) (string, error)) *BusinessResolution {
	// Priority 1: Explicit header
	if headerBusinessID != "" {
		s.logger.InfoWith("Business resolved via header", "business_id", headerBusinessID)
		return &BusinessResolution{
			BusinessID:  headerBusinessID,
			ResolvedVia: "header",
		}
	}

	// Priority 2: Match buyer_gstin from extraction against business table
	if extractedBuyerGSTIN != "" && businessLookup != nil {
		bid, err := businessLookup(ctx, extractedBuyerGSTIN)
		if err != nil {
			s.logger.WarnWith("Business GSTIN lookup failed", "buyer_gstin", extractedBuyerGSTIN, "error", err)
		}
		if bid != "" {
			s.logger.InfoWith("Business resolved via buyer GSTIN match",
				"buyer_gstin", extractedBuyerGSTIN, "business_id", bid)
			return &BusinessResolution{
				BusinessID:  bid,
				ResolvedVia: "gstin_match",
			}
		}
	}

	s.logger.InfoWith("Business resolution: unresolved",
		"header_business_id", headerBusinessID, "buyer_gstin", extractedBuyerGSTIN)
	return &BusinessResolution{
		ResolvedVia: "unresolved",
	}
}

// GetBusinessIDByGSTIN is a helper that can be used as the businessLookup parameter.
// It queries xcentro.business WHERE gstin = ? and returns the bid.
// This should be a thin wrapper over a real DB query in the voucher repository.
func GetBusinessIDByGSTINQuery(ctx context.Context, gstin string) (string, error) {
	// This is a placeholder — the actual implementation is in the repository.
	// The caller should provide the concrete function from the repository layer.
	return "", fmt.Errorf("not implemented: provide business lookup via repository")
}
