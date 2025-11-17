package invoice

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
	"xcentro.in/internal/core/invoice"
)

// PDFGenerator generates PDF invoices
type PDFGenerator struct{}

// NewPDFGenerator creates a new PDF generator
func NewPDFGenerator() *PDFGenerator {
	return &PDFGenerator{}
}

// GenerateInvoicePDF generates a PDF invoice
func (g *PDFGenerator) GenerateInvoicePDF(inv *invoice.Invoice) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	// Set font
	pdf.SetFont("Arial", "B", 16)

	// Header
	pdf.Cell(0, 10, "GST TAX INVOICE")
	pdf.Ln(12)

	// Invoice details
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 8, fmt.Sprintf("Invoice Number: %s", inv.InvoiceNumber))
	pdf.Ln(6)

	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Invoice Date: %s", inv.InvoiceDate.Format("02-Jan-2006")))
	pdf.Ln(6)

	if inv.IRN != nil && *inv.IRN != "" {
		pdf.Cell(0, 6, fmt.Sprintf("IRN: %s", *inv.IRN))
		pdf.Ln(6)
	}

	if inv.IRNAckNo != nil && *inv.IRNAckNo != "" {
		pdf.Cell(0, 6, fmt.Sprintf("IRN Ack No: %s", *inv.IRNAckNo))
		pdf.Ln(6)
	}

	pdf.Ln(4)

	// Supplier and Recipient details
	pdf.SetFont("Arial", "B", 11)
	pdf.Cell(95, 6, "Supplier Details")
	pdf.Cell(95, 6, "Recipient Details")
	pdf.Ln(6)

	pdf.SetFont("Arial", "", 10)
	pdf.Cell(95, 6, fmt.Sprintf("GSTIN: %s", inv.GSTINSupplier))
	pdf.Cell(95, 6, fmt.Sprintf("GSTIN: %s", inv.GSTINRecipient))
	pdf.Ln(10)

	// Line items table
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(200, 200, 200)

	// Table header
	pdf.CellFormat(15, 8, "S.No", "1", 0, "C", true, 0, "")
	pdf.CellFormat(60, 8, "Description", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, "SAC Code", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "Taxable Value", "1", 0, "C", true, 0, "")
	pdf.CellFormat(20, 8, "GST Rate", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "GST Amount", "1", 0, "C", true, 0, "")
	pdf.CellFormat(30, 8, "Total Amount", "1", 1, "C", true, 0, "")

	// Table data
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(15, 8, "1", "1", 0, "C", false, 0, "")
	pdf.CellFormat(60, 8, inv.Description, "1", 0, "L", false, 0, "")
	pdf.CellFormat(25, 8, inv.SACCode, "1", 0, "C", false, 0, "")
	pdf.CellFormat(30, 8, fmt.Sprintf("%.2f", inv.TaxableValue), "1", 0, "R", false, 0, "")
	pdf.CellFormat(20, 8, fmt.Sprintf("%.2f%%", inv.GSTRate), "1", 0, "R", false, 0, "")
	pdf.CellFormat(30, 8, fmt.Sprintf("%.2f", inv.GSTAmount), "1", 0, "R", false, 0, "")
	pdf.CellFormat(30, 8, fmt.Sprintf("%.2f", inv.TotalAmount), "1", 1, "R", false, 0, "")

	pdf.Ln(4)

	// Total
	pdf.SetFont("Arial", "B", 11)
	pdf.Cell(180, 8, "Total Amount (INR)")
	pdf.Cell(30, 8, fmt.Sprintf("%.2f", inv.TotalAmount))
	pdf.Ln(10)

	// Status information
	pdf.SetFont("Arial", "", 9)
	pdf.Cell(0, 6, fmt.Sprintf("IRN Status: %s", inv.IRNStatus))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Filing Status: %s", inv.FilingStatus))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Payment ID: %s", inv.PaymentID))
	pdf.Ln(10)

	// QR Code (if available)
	if inv.QRCode != nil && *inv.QRCode != "" {
		// Decode base64 QR code
		qrData, err := base64.StdEncoding.DecodeString(*inv.QRCode)
		if err == nil {
			// Create a temporary file for QR code
			tmpReader := bytes.NewReader(qrData)

			// Register the image
			_ = pdf.RegisterImageReader(fmt.Sprintf("qr_%d", inv.InvoiceID), "PNG", tmpReader)
			if pdf.Ok() {
				pdf.SetFont("Arial", "B", 10)
				pdf.Cell(0, 6, "QR Code:")
				pdf.Ln(6)
				pdf.Image(fmt.Sprintf("qr_%d", inv.InvoiceID), 10, pdf.GetY(), 40, 40, false, "", 0, "")
				pdf.Ln(42)
			}
		}
	}

	// Footer
	pdf.SetY(-20)
	pdf.SetFont("Arial", "I", 8)
	pdf.Cell(0, 10, fmt.Sprintf("Generated on: %s", time.Now().Format("02-Jan-2006 15:04:05")))

	// Output PDF to buffer
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}
