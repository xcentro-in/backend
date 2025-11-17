package invoice

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/invoice"
	invoiceService "xcentro.in/internal/services/invoice"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/utils"
)

type InvoiceHandler struct {
	invoiceService *invoiceService.InvoiceService
	pdfGenerator   *utils.PDFGenerator
	logger         *logger.Logger
}

func NewInvoiceHandler(invoiceService *invoiceService.InvoiceService, pdfGenerator *utils.PDFGenerator) *InvoiceHandler {
	return &InvoiceHandler{
		invoiceService: invoiceService,
		pdfGenerator:   pdfGenerator,
		logger:         logger.GetDefault(),
	}
}

// DownloadInvoice downloads an invoice by invoice_id or payment_id in PDF or JSON format
func (h *InvoiceHandler) DownloadInvoice(c echo.Context) error {
	// Get query parameters
	invoiceIDStr := c.QueryParam("invoice_id")
	paymentID := c.QueryParam("payment_id")
	format := c.QueryParam("format")

	// Validate that at least one ID is provided
	if invoiceIDStr == "" && paymentID == "" {
		return c.JSON(400, map[string]any{"message": "Either invoice_id or payment_id must be provided", "data": nil})
	}

	// Validate format
	if format == "" {
		format = "pdf" // Default to PDF
	}

	exportFormat := invoice.ExportFormat(format)
	if !exportFormat.IsValid() {
		return c.JSON(400, map[string]any{"message": "Invalid format. Must be 'pdf' or 'json'", "data": nil})
	}

	// Fetch invoice
	var inv *invoice.Invoice
	var err error

	if invoiceIDStr != "" {
		invoiceID, parseErr := strconv.ParseInt(invoiceIDStr, 10, 64)
		if parseErr != nil {
			return c.JSON(400, map[string]any{"message": "Invalid invoice_id", "data": nil})
		}
		inv, err = h.invoiceService.GetInvoiceByID(c.Request().Context(), invoiceID)
	} else {
		inv, err = h.invoiceService.GetInvoiceByPaymentID(c.Request().Context(), paymentID)
	}

	if err != nil {
		h.logger.Error(fmt.Sprintf("Error fetching invoice: %v", err))
		return c.JSON(500, map[string]any{"message": "Failed to fetch invoice", "data": nil})
	}

	if inv == nil {
		return c.JSON(404, map[string]any{"message": "Invoice not found", "data": nil})
	}

	// Generate response based on format
	switch exportFormat {
	case invoice.ExportFormatPDF:
		pdfData, err := h.pdfGenerator.GenerateInvoicePDF(inv)
		if err != nil {
			h.logger.Error(fmt.Sprintf("Error generating PDF: %v", err))
			return c.JSON(500, map[string]any{"message": "Failed to generate PDF", "data": nil})
		}

		// Set headers for PDF download
		c.Response().Header().Set("Content-Type", "application/pdf")
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.pdf", inv.InvoiceNumber))
		return c.Blob(200, "application/pdf", pdfData)

	case invoice.ExportFormatJSON:
		// Set headers for JSON download
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.json", inv.InvoiceNumber))

		jsonData, err := json.MarshalIndent(inv, "", "  ")
		if err != nil {
			h.logger.Error(fmt.Sprintf("Error marshaling JSON: %v", err))
			return c.JSON(500, map[string]any{"message": "Failed to generate JSON", "data": nil})
		}

		return c.Blob(200, "application/json", jsonData)

	default:
		return c.JSON(400, map[string]any{"message": "Invalid format", "data": nil})
	}
}

// GetInvoiceByID retrieves an invoice by ID (for viewing, not download)
func (h *InvoiceHandler) GetInvoiceByID(c echo.Context) error {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.ParseInt(invoiceIDStr, 10, 64)
	if err != nil {
		return c.JSON(400, map[string]any{"message": "Invalid invoice_id", "data": nil})
	}

	inv, err := h.invoiceService.GetInvoiceByID(c.Request().Context(), invoiceID)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Error fetching invoice: %v", err))
		return c.JSON(500, map[string]any{"message": "Failed to fetch invoice", "data": nil})
	}

	if inv == nil {
		return c.JSON(404, map[string]any{"message": "Invoice not found", "data": nil})
	}

	return c.JSON(200, map[string]any{"message": "Invoice retrieved successfully", "data": inv})
}

// GetInvoiceByPaymentID retrieves an invoice by payment ID (for viewing, not download)
func (h *InvoiceHandler) GetInvoiceByPaymentID(c echo.Context) error {
	paymentID := c.Param("payment_id")

	inv, err := h.invoiceService.GetInvoiceByPaymentID(c.Request().Context(), paymentID)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Error fetching invoice: %v", err))
		return c.JSON(500, map[string]any{"message": "Failed to fetch invoice", "data": nil})
	}

	if inv == nil {
		return c.JSON(404, map[string]any{"message": "Invoice not found", "data": nil})
	}

	return c.JSON(200, map[string]any{"message": "Invoice retrieved successfully", "data": inv})
}
