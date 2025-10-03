package voucher

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// VoucherService defines the interface for voucher business logic
type VoucherService interface {
	// GenerateVoucherNumber generates a unique voucher number for a business
	// Format: PV-YYYY-NNNNN (e.g., PV-2024-00001)
	GenerateVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (string, error)

	// CalculateFiscalYear calculates the fiscal year for a given date
	// Indian fiscal year: April 1 - March 31
	// Example: 2024-09-15 → FY2024-25
	CalculateFiscalYear(date time.Time) string

	// CalculateAccountingPeriod calculates the accounting period for a given date
	// Format: YYYY-MM (e.g., 2024-09)
	CalculateAccountingPeriod(date time.Time) string

	// ValidateVoucherData validates voucher data before creation
	ValidateVoucherData(ctx context.Context, voucher *Voucher) error

	// CalculateNetAmount calculates net amount based on gross, TDS, and discount
	// Formula: NetAmount = GrossAmount - TDS - Discount
	CalculateNetAmount(grossAmount, tdsAmount, discountAmount int64) int64

	// CalculateTDSAmount calculates TDS amount based on gross amount and TDS rate
	CalculateTDSAmount(grossAmount int64, tdsRate float64) int64

	// CalculateGSTComponents splits GST into CGST/SGST or IGST based on state codes
	CalculateGSTComponents(gstAmount int64, isInterState bool) (cgst, sgst, igst int64)

	// ValidateInvoiceNumber validates invoice number format and uniqueness
	ValidateInvoiceNumber(ctx context.Context, invoiceNumber string) error
}

// VoucherServiceImpl implements the VoucherService interface
type VoucherServiceImpl struct {
	voucherRepo VoucherRepository
}

// NewVoucherService creates a new instance of VoucherService
func NewVoucherService(voucherRepo VoucherRepository) VoucherService {
	return &VoucherServiceImpl{
		voucherRepo: voucherRepo,
	}
}

// GenerateVoucherNumber generates a unique voucher number
func (s *VoucherServiceImpl) GenerateVoucherNumber(ctx context.Context, businessID uuid.UUID, fiscalYear string) (string, error) {
	// Get the next sequence number for this business and fiscal year
	sequenceNumber, err := s.voucherRepo.GetNextVoucherNumber(ctx, businessID, fiscalYear)
	if err != nil {
		return "", fmt.Errorf("failed to get next voucher number: %w", err)
	}

	// Extract year from fiscal year (e.g., FY2024-25 → 2024)
	year := extractYearFromFiscalYear(fiscalYear)

	// Format: PV-YYYY-NNNNN (e.g., PV-2024-00001)
	voucherNumber := fmt.Sprintf("PV-%s-%05d", year, sequenceNumber)

	return voucherNumber, nil
}

// CalculateFiscalYear calculates the fiscal year for a given date
// Indian fiscal year: April 1 - March 31
func (s *VoucherServiceImpl) CalculateFiscalYear(date time.Time) string {
	year := date.Year()
	month := date.Month()

	// If month is Jan-Mar, fiscal year started in previous calendar year
	if month >= time.January && month <= time.March {
		return fmt.Sprintf("FY%d-%02d", year-1, year%100)
	}

	// If month is Apr-Dec, fiscal year starts in current calendar year
	return fmt.Sprintf("FY%d-%02d", year, (year+1)%100)
}

// CalculateAccountingPeriod calculates the accounting period
func (s *VoucherServiceImpl) CalculateAccountingPeriod(date time.Time) string {
	return date.Format("2006-01")
}

// ValidateVoucherData validates voucher data
func (s *VoucherServiceImpl) ValidateVoucherData(ctx context.Context, voucher *Voucher) error {
	// Basic entity validation
	if err := voucher.IsValid(); err != nil {
		return err
	}

	// Validate voucher date (not in future)
	if voucher.VoucherDate.After(time.Now()) {
		return ErrInvalidVoucherDate
	}

	// Validate amount calculation
	if !voucher.ValidateAmountCalculation() {
		return ErrAmountMismatch
	}

	// Validate GST components
	totalGST := voucher.CalculateTotalTax()
	if totalGST != voucher.GSTAmount {
		return fmt.Errorf("GST component mismatch: CGST+SGST+IGST (%d) != GSTAmount (%d)", totalGST, voucher.GSTAmount)
	}

	// Validate invoice number is provided
	if voucher.InvoiceNumber == "" {
		return ErrInvalidInvoiceNumber
	}

	return nil
}

// CalculateNetAmount calculates net amount
func (s *VoucherServiceImpl) CalculateNetAmount(grossAmount, tdsAmount, discountAmount int64) int64 {
	return grossAmount - tdsAmount - discountAmount
}

// CalculateTDSAmount calculates TDS amount
func (s *VoucherServiceImpl) CalculateTDSAmount(grossAmount int64, tdsRate float64) int64 {
	return int64(float64(grossAmount) * tdsRate / 100.0)
}

// CalculateGSTComponents splits GST into CGST/SGST or IGST
func (s *VoucherServiceImpl) CalculateGSTComponents(gstAmount int64, isInterState bool) (cgst, sgst, igst int64) {
	if isInterState {
		// Inter-state transaction: Full amount goes to IGST
		return 0, 0, gstAmount
	}

	// Intra-state transaction: Split equally between CGST and SGST
	cgst = gstAmount / 2
	sgst = gstAmount - cgst // Handle odd amounts

	return cgst, sgst, 0
}

// ValidateInvoiceNumber validates invoice number format
func (s *VoucherServiceImpl) ValidateInvoiceNumber(ctx context.Context, invoiceNumber string) error {
	if invoiceNumber == "" {
		return ErrInvalidInvoiceNumber
	}

	// Additional validation rules can be added here
	// For now, just check it's not empty

	return nil
}

// Helper function to extract year from fiscal year string
func extractYearFromFiscalYear(fiscalYear string) string {
	// FY2024-25 → 2024
	if len(fiscalYear) >= 6 {
		return fiscalYear[2:6]
	}
	return fmt.Sprintf("%d", time.Now().Year())
}
