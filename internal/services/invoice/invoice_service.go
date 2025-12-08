package invoice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"xcentro.in/internal/core/invoice"
	"xcentro.in/internal/shared/logger"
)

const (
	DefaultGSTINSupplier = "27AATCM5865F1ZU" // XCentro GSTIN
	DefaultDescription   = "Convenience Fee"
	DefaultSACCode       = "997159"
	DefaultGSTRate       = 18.0
)

// InvoiceService handles invoice business logic
type InvoiceService struct {
	invoiceRepo invoice.InvoiceRepository
	logger      *logger.Logger
}

// NewInvoiceService creates a new invoice service
func NewInvoiceService(invoiceRepo invoice.InvoiceRepository) *InvoiceService {
	return &InvoiceService{
		invoiceRepo: invoiceRepo,
		logger:      logger.GetDefault(),
	}
}

// CreateInvoiceForPayment creates an invoice for a successful payment
// If invoice already exists, it verifies the invoice matches the payment details
// Returns the invoice (existing or newly created) and any verification errors (non-fatal)
func (s *InvoiceService) CreateInvoiceForPayment(ctx context.Context, paymentID string, gstinRecipient string, convenienceFee, convenienceFeeGST int64) (*invoice.Invoice, error) {
	s.logger.Info(fmt.Sprintf("[InvoiceService] CreateInvoiceForPayment called - PaymentID: %s", paymentID))

	// Check if invoice already exists for this payment
	existingInvoice, err := s.invoiceRepo.GetInvoiceByPaymentID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing invoice: %w", err)
	}

	// If invoice exists, verify it matches the payment details
	if existingInvoice != nil {
		s.logger.Info(fmt.Sprintf("[InvoiceService] Invoice already exists for PaymentID: %s, verifying details", paymentID))

		// Verify invoice matches current payment details
		if !s.VerifyInvoice(ctx, existingInvoice, gstinRecipient, convenienceFee, convenienceFeeGST) {
			s.logger.ErrorWith("Invoice verification failed - invoice exists but details don't match",
				"payment_id", paymentID,
				"invoice_id", existingInvoice.InvoiceID,
				"expected_gstin", gstinRecipient,
				"actual_gstin", existingInvoice.GSTINRecipient,
				"expected_taxable", float64(convenienceFee)/100.0,
				"actual_taxable", existingInvoice.TaxableValue,
				"expected_gst", float64(convenienceFeeGST)/100.0,
				"actual_gst", existingInvoice.GSTAmount)

			// Return error but don't fail the payment - this is non-fatal
			return existingInvoice, fmt.Errorf("invoice verification failed: existing invoice details don't match payment details")
		}

		s.logger.Info(fmt.Sprintf("[InvoiceService] Invoice verification passed for PaymentID: %s", paymentID))
		return existingInvoice, nil
	}

	// Calculate amounts (convert from paise to rupees)
	taxableValue := float64(convenienceFee) / 100.0
	gstAmount := float64(convenienceFeeGST) / 100.0
	totalAmount := taxableValue + gstAmount

	// Create invoice entity
	inv := &invoice.Invoice{
		PaymentID:      paymentID,
		InvoiceNumber:  "", // Will be set after getting invoice_id
		GSTINSupplier:  DefaultGSTINSupplier,
		GSTINRecipient: gstinRecipient,
		InvoiceDate:    time.Now().UTC(),
		IRNStatus:      invoice.IRNStatusPending,
		FilingStatus:   invoice.FilingStatusNotFiled,
		Description:    DefaultDescription,
		SACCode:        DefaultSACCode,
		TaxableValue:   taxableValue,
		GSTRate:        DefaultGSTRate,
		GSTAmount:      gstAmount,
		TotalAmount:    totalAmount,
	}

	// Generate IRN (will be in pending state until signed by GST portal)
	// IRN generation uses: GSTIN|DocType|DocNumber|FY
	// For now, we'll set it after creating the invoice to get the invoice_id

	// Create invoice in database
	err = s.invoiceRepo.CreateInvoice(ctx, inv)
	if err != nil {
		return nil, fmt.Errorf("failed to create invoice: %w", err)
	}

	// Now update the invoice number to INV-<invoice_id>
	inv.InvoiceNumber = fmt.Sprintf("INV-%d", inv.InvoiceID)

	// Generate IRN hash
	irn := s.generateIRN(inv.GSTINSupplier, inv.InvoiceNumber, inv.InvoiceDate)
	inv.IRN = &irn

	// Update IRN in database
	err = s.invoiceRepo.UpdateIRNDetails(ctx, inv.InvoiceID, irn, "", nil, invoice.IRNStatusPending)
	if err != nil {
		s.logger.Error(fmt.Sprintf("[InvoiceService] Failed to update IRN for invoice %d: %v", inv.InvoiceID, err))
		// Don't fail the invoice creation, just log the error
	}

	s.logger.Info(fmt.Sprintf("[InvoiceService] Invoice created successfully - InvoiceID: %d, InvoiceNumber: %s", inv.InvoiceID, inv.InvoiceNumber))
	return inv, nil
}

// generateIRN generates Invoice Reference Number (IRN) hash
// Format: SHA256(GSTIN|DocType|DocNumber|FY)
// Example: 27ABCDE1234F1Z5|INV|INV-002344|2025-26
func (s *InvoiceService) generateIRN(gstin, invoiceNumber string, invoiceDate time.Time) string {
	// Determine financial year (FY)
	// In India, FY starts on April 1
	year := invoiceDate.Year()
	month := invoiceDate.Month()

	var fy string
	if month >= time.April {
		// From April onwards, FY is current year to next year
		fy = fmt.Sprintf("%d-%02d", year, (year+1)%100)
	} else {
		// Before April, FY is previous year to current year
		fy = fmt.Sprintf("%d-%02d", year-1, year%100)
	}

	// Build IRN input string
	irnInput := fmt.Sprintf("%s|INV|%s|%s", gstin, invoiceNumber, fy)

	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(irnInput))
	irn := hex.EncodeToString(hash[:])

	s.logger.Info(fmt.Sprintf("[InvoiceService] Generated IRN - Input: %s, Hash: %s", irnInput, irn))
	return irn
}

// GetInvoiceByID retrieves an invoice by ID
func (s *InvoiceService) GetInvoiceByID(ctx context.Context, invoiceID int64) (*invoice.Invoice, error) {
	s.logger.Info(fmt.Sprintf("[InvoiceService] GetInvoiceByID called - InvoiceID: %d", invoiceID))
	return s.invoiceRepo.GetInvoiceByID(ctx, invoiceID)
}

// GetInvoiceByPaymentID retrieves an invoice by payment ID
func (s *InvoiceService) GetInvoiceByPaymentID(ctx context.Context, paymentID string) (*invoice.Invoice, error) {
	s.logger.Info(fmt.Sprintf("[InvoiceService] GetInvoiceByPaymentID called - PaymentID: %s", paymentID))
	return s.invoiceRepo.GetInvoiceByPaymentID(ctx, paymentID)
}

// ListInvoices retrieves invoices with filtering and pagination
func (s *InvoiceService) ListInvoices(ctx context.Context, filter *invoice.InvoiceListFilter) (*invoice.InvoiceListResponse, error) {
	s.logger.Info(fmt.Sprintf("[InvoiceService] ListInvoices called - Page: %d, PageSize: %d", filter.Page, filter.PageSize))

	// Set default pagination values if not provided
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	invoices, totalCount, err := s.invoiceRepo.ListInvoices(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list invoices: %w", err)
	}

	// Calculate total pages
	totalPages := int(totalCount) / filter.PageSize
	if int(totalCount)%filter.PageSize != 0 {
		totalPages++
	}

	return &invoice.InvoiceListResponse{
		Invoices:   invoices,
		TotalCount: totalCount,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	}, nil
}

// VerifyInvoice verifies that an existing invoice matches the expected payment details
// Returns true if invoice is valid, false if there's a mismatch
func (s *InvoiceService) VerifyInvoice(ctx context.Context, inv *invoice.Invoice, gstinRecipient string, convenienceFee, convenienceFeeGST int64) bool {
	s.logger.Info(fmt.Sprintf("[InvoiceService] VerifyInvoice called - InvoiceID: %d, PaymentID: %s", inv.InvoiceID, inv.PaymentID))

	// Calculate expected amounts (convert from paise to rupees)
	expectedTaxableValue := float64(convenienceFee) / 100.0
	expectedGSTAmount := float64(convenienceFeeGST) / 100.0
	expectedTotalAmount := expectedTaxableValue + expectedGSTAmount

	// Verify GSTIN recipient
	if inv.GSTINRecipient != gstinRecipient {
		s.logger.Error(fmt.Sprintf("[InvoiceService] GSTIN mismatch - Expected: %s, Got: %s", gstinRecipient, inv.GSTINRecipient))
		return false
	}

	// Verify taxable value (allow small floating point tolerance)
	const tolerance = 0.01
	if !floatEquals(inv.TaxableValue, expectedTaxableValue, tolerance) {
		s.logger.Error(fmt.Sprintf("[InvoiceService] Taxable value mismatch - Expected: %.2f, Got: %.2f", expectedTaxableValue, inv.TaxableValue))
		return false
	}

	// Verify GST amount
	if !floatEquals(inv.GSTAmount, expectedGSTAmount, tolerance) {
		s.logger.Error(fmt.Sprintf("[InvoiceService] GST amount mismatch - Expected: %.2f, Got: %.2f", expectedGSTAmount, inv.GSTAmount))
		return false
	}

	// Verify total amount
	if !floatEquals(inv.TotalAmount, expectedTotalAmount, tolerance) {
		s.logger.Error(fmt.Sprintf("[InvoiceService] Total amount mismatch - Expected: %.2f, Got: %.2f", expectedTotalAmount, inv.TotalAmount))
		return false
	}

	s.logger.Info(fmt.Sprintf("[InvoiceService] Invoice verification passed - InvoiceID: %d", inv.InvoiceID))
	return true
}

// DeleteInvoice deletes an invoice by ID
func (s *InvoiceService) DeleteInvoice(ctx context.Context, invoiceID int64) error {
	s.logger.Info(fmt.Sprintf("[InvoiceService] DeleteInvoice called - InvoiceID: %d", invoiceID))

	err := s.invoiceRepo.DeleteInvoice(ctx, invoiceID)
	if err != nil {
		return fmt.Errorf("failed to delete invoice: %w", err)
	}

	s.logger.Info(fmt.Sprintf("[InvoiceService] Invoice deleted successfully - InvoiceID: %d", invoiceID))
	return nil
}

// floatEquals checks if two floats are equal within a tolerance
func floatEquals(a, b, tolerance float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}
