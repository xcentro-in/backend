package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"

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
	// 1. Setup PDF in Portrait mode ("P") A4
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10) // Left, Top, Right
	pdf.AddPage()

	// --- LAYOUT CONSTANTS ---
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	printableWidth := pageWidth - leftMargin - rightMargin
	colWidth := printableWidth / 2
	lineHt := 5.0

	// --- 1. HEADER TITLE ---
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(printableWidth, 8, "TAX INVOICE", "", 1, "C", false, 0, "")
	pdf.Ln(2)

	// Save Y start position for the IRN/QR section
	yIRNStart := pdf.GetY()

	// --- 2. TOP SECTION: IRN and QR CODE ---

	// QR CODE (Top Right) - Calculate size first
	qrSize := 30.0 // Default QR size
	hasQRCode := inv.QRCode != nil && *inv.QRCode != ""
	if hasQRCode {
		qrData, err := base64.StdEncoding.DecodeString(*inv.QRCode)
		if err == nil {
			tmpReader := bytes.NewReader(qrData)
			imgName := fmt.Sprintf("qr_%d", inv.InvoiceID)
			_ = pdf.RegisterImageReader(imgName, "PNG", tmpReader)

			// Position QR code in the top-right corner
			pdf.Image(imgName, pageWidth-rightMargin-qrSize-2, yIRNStart+2, qrSize, qrSize, false, "", 0, "")
		}
	}

	// Left side: IRN, Ack. No., Ack. Date
	pdf.SetFont("Arial", "", 8)

	if inv.IRN != nil && *inv.IRN != "" {
		pdf.MultiCell(printableWidth-qrSize-5, 4, fmt.Sprintf("IRN: %s", *inv.IRN), "", "L", false)
	}

	// Placeholder for Ack. No. and Ack. Date (you'll need to add these fields to the invoice struct)
	pdf.Cell(printableWidth-qrSize-5, lineHt, "Ack. No: 112010004620119")
	pdf.Ln(lineHt)
	pdf.Cell(printableWidth-qrSize-5, lineHt, "Ack. Date: 17-09-2020 15:20:00")

	// Calculate the actual height needed
	yIRNEnd := pdf.GetY()
	irnContentHeight := yIRNEnd - yIRNStart

	// Box height should be at least as tall as QR code or content, whichever is taller
	minQRHeight := qrSize + 4            // QR size plus padding
	irnBoxHeight := irnContentHeight + 5 // Content height plus padding
	if minQRHeight > irnBoxHeight {
		irnBoxHeight = minQRHeight
	}

	// Draw border for IRN/QR section
	pdf.Rect(leftMargin, yIRNStart, printableWidth, irnBoxHeight, "D")

	// Move Y to below the IRN section
	pdf.SetY(yIRNStart + irnBoxHeight + 3)

	// --- 3. SUPPLIER AND INVOICE DETAILS SECTION ---
	yStart := pdf.GetY()

	// LEFT BOX: FROM (Supplier)
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(colWidth, lineHt, "From:")
	pdf.Ln(lineHt)

	pdf.SetFont("Arial", "B", 11)
	pdf.Cell(colWidth, lineHt, "Merchantbridge Fintech Solutions Pvt. Ltd.")
	pdf.Ln(lineHt)

	// Updated Address (Innov8)
	pdf.SetFont("Arial", "", 9)
	pdf.Cell(colWidth, lineHt, "Innov8 Solitaire Corporate Park,")
	pdf.Ln(lineHt)
	pdf.Cell(colWidth, lineHt, "Ghatkopar Link Road, Chakala,")
	pdf.Ln(lineHt)
	pdf.Cell(colWidth, lineHt, "Andheri East, Mumbai-400093")
	pdf.Ln(lineHt)

	pdf.SetFont("Arial", "B", 9)
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("GSTIN: %s", inv.GSTINSupplier))
	pdf.Ln(lineHt)

	// Extract state code from first 2 digits of GSTIN
	supplierStateCode := "XX"
	if len(inv.GSTINSupplier) >= 2 {
		supplierStateCode = inv.GSTINSupplier[:2]
	}
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("State Code: %s", supplierStateCode))

	// Save the Y position at the bottom of the left content
	yLeftBottom := pdf.GetY()

	// --- 4. RIGHT BOX: METADATA (Invoice No, Date, etc) ---
	// Move cursor to the top of the right column
	pdf.SetXY(leftMargin+colWidth, yStart)

	// Helper to reset X to the right column
	setRightCol := func() { pdf.SetX(leftMargin + colWidth) }

	setRightCol()
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("Invoice No: INV-%d", inv.InvoiceID))
	pdf.Ln(lineHt)

	setRightCol()
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("Date: %s", inv.InvoiceDate.Format("02-Jan-2006")))
	pdf.Ln(lineHt)

	// Calculate Section Height
	yRightBottom := pdf.GetY()
	sectionHeight := yLeftBottom - yStart
	if yRightBottom > yLeftBottom {
		sectionHeight = yRightBottom - yStart
	}
	sectionHeight += 5 // Add padding

	// Draw Borders for Supplier/Invoice Section
	pdf.Rect(leftMargin, yStart, colWidth, sectionHeight, "D")          // Left Box
	pdf.Rect(leftMargin+colWidth, yStart, colWidth, sectionHeight, "D") // Right Box

	// Move Y to below the supplier section
	pdf.SetY(yStart + sectionHeight)

	// --- 4. BUYER SECTION (Bill To Only) ---
	// Ship To logic removed completely

	yStartBox2 := pdf.GetY()
	box2Height := 30.0

	// BILL TO (Spanning full width or half width - kept half to match top style)
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(colWidth, lineHt, "Bill To:")
	pdf.Ln(lineHt)

	pdf.SetFont("Arial", "", 9)
	pdf.Cell(colWidth, lineHt, "Buyer Name Placeholder") // Replace with inv.CustomerName if available
	pdf.Ln(lineHt)
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("GSTIN: %s", inv.GSTINRecipient))
	pdf.Ln(lineHt)
	// Extract state code from first 2 digits of GSTIN
	recipientStateCode := "XX"
	if len(inv.GSTINRecipient) >= 2 {
		recipientStateCode = inv.GSTINRecipient[:2]
	}
	pdf.Cell(colWidth, lineHt, fmt.Sprintf("State Code: %s", recipientStateCode))

	// Draw Borders for Middle Section (Only one box now)
	// We draw a box across the full width or just left?
	// Let's keep it consistent with the top and just draw the left box,
	// or draw a full width box for the customer since Ship To is gone.
	// Here I draw a Full Width box for "Bill To" to look cleaner.
	pdf.Rect(leftMargin, yStartBox2, printableWidth, box2Height, "D")

	pdf.SetY(yStartBox2 + box2Height + 5) // Add space before table

	// --- 5. ITEM TABLE ---

	// Define Column Widths (Total ~190mm)
	wSno := 12.0
	wDesc := 75.0
	wSac := 20.0
	wVal := 25.0
	wRate := 15.0
	wTax := 20.0
	wTot := 23.0

	// Table Header
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(230, 230, 230) // Light Grey
	pdf.CellFormat(wSno, 8, "S.No", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wDesc, 8, "Description", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wSac, 8, "SAC", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wVal, 8, "Taxable", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wRate, 8, "Rate", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wTax, 8, "GST", "1", 0, "C", true, 0, "")
	pdf.CellFormat(wTot, 8, "Total", "1", 1, "C", true, 0, "")

	// Table Data
	pdf.SetFont("Arial", "", 9)

	// Prepare Description Text
	descText := fmt.Sprintf("Payment Processing Fee\nPayment ID: %s", inv.PaymentID)

	// Calculate Row Height based on text wrapping
	lines := pdf.SplitLines([]byte(descText), wDesc)
	rowHeight := float64(len(lines)) * 5.0
	if rowHeight < 10 {
		rowHeight = 10
	}

	// 1. S.No
	pdf.CellFormat(wSno, rowHeight, "1", "1", 0, "C", false, 0, "")

	// 2. Description (MultiCell Logic)
	xDesc := pdf.GetX()
	yDesc := pdf.GetY()
	pdf.MultiCell(wDesc, 5, descText, "1", "L", false)
	pdf.SetXY(xDesc+wDesc, yDesc) // Move cursor back to top-right of description cell

	// 3. Rest of the cells
	pdf.CellFormat(wSac, rowHeight, inv.SACCode, "1", 0, "C", false, 0, "")
	pdf.CellFormat(wVal, rowHeight, fmt.Sprintf("%.2f", inv.TaxableValue), "1", 0, "R", false, 0, "")
	pdf.CellFormat(wRate, rowHeight, fmt.Sprintf("%.2f%%", inv.GSTRate), "1", 0, "R", false, 0, "")
	pdf.CellFormat(wTax, rowHeight, fmt.Sprintf("%.2f", inv.GSTAmount), "1", 0, "R", false, 0, "")
	pdf.CellFormat(wTot, rowHeight, fmt.Sprintf("%.2f", inv.TotalAmount), "1", 1, "R", false, 0, "")

	// --- 6. FOOTER REMOVED (End of Doc) ---

	// Output
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}
