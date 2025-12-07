package invoice

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/invoice"
	invoiceService "xcentro.in/internal/services/invoice"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/utils"
)

type OpsInvoiceHandler struct {
	invoiceService *invoiceService.InvoiceService
	pdfGenerator   *utils.PDFGenerator
	logger         *logger.Logger
}

func NewOpsInvoiceHandler(invoiceService *invoiceService.InvoiceService, pdfGenerator *utils.PDFGenerator) *OpsInvoiceHandler {
	return &OpsInvoiceHandler{
		invoiceService: invoiceService,
		pdfGenerator:   pdfGenerator,
		logger:         logger.GetDefault(),
	}
}

// ListInvoices lists invoices with pagination and filtering
func (h *OpsInvoiceHandler) ListInvoices(c echo.Context) error {
	// Parse pagination parameters
	pageStr := c.QueryParam("page")
	pageSizeStr := c.QueryParam("page_size")

	page := 1
	pageSize := 20

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Build filter
	filter := &invoice.InvoiceListFilter{
		Page:     page,
		PageSize: pageSize,
	}

	// Parse filters
	if invoiceIDStr := c.QueryParam("invoice_id"); invoiceIDStr != "" {
		if invoiceID, err := strconv.ParseInt(invoiceIDStr, 10, 64); err == nil {
			filter.InvoiceID = &invoiceID
		}
	}

	if paymentID := c.QueryParam("payment_id"); paymentID != "" {
		filter.PaymentID = &paymentID
	}

	if irn := c.QueryParam("irn"); irn != "" {
		filter.IRN = &irn
	}

	if gstinRecipient := c.QueryParam("gstin_recipient"); gstinRecipient != "" {
		filter.GSTINRecipient = &gstinRecipient
	}

	if irnStatus := c.QueryParam("irn_status"); irnStatus != "" {
		status := invoice.IRNStatus(irnStatus)
		if status.IsValid() {
			filter.IRNStatus = &status
		}
	}

	if filingStatus := c.QueryParam("filing_status"); filingStatus != "" {
		status := invoice.FilingStatus(filingStatus)
		if status.IsValid() {
			filter.FilingStatus = &status
		}
	}

	// Parse date range
	if dateFromStr := c.QueryParam("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse("2006-01-02", dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := c.QueryParam("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse("2006-01-02", dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	// Fetch invoices
	response, err := h.invoiceService.ListInvoices(c.Request().Context(), filter)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Error listing invoices: %v", err))
		return c.JSON(500, map[string]any{"message": "Failed to list invoices", "data": nil})
	}

	return c.JSON(200, map[string]any{
		"message": "Invoices retrieved successfully",
		"data": map[string]any{
			"invoices":    response.Invoices,
			"total_count": response.TotalCount,
			"page":        response.Page,
			"page_size":   response.PageSize,
			"total_pages": response.TotalPages,
		},
	})
}

// ExportInvoices exports invoices to PDF or JSON
func (h *OpsInvoiceHandler) ExportInvoices(c echo.Context) error {
	// Get format
	format := c.QueryParam("format")
	if format == "" {
		format = "pdf"
	}

	exportFormat := invoice.ExportFormat(format)
	if !exportFormat.IsValid() {
		return c.JSON(400, map[string]any{"message": "Invalid format. Must be 'pdf' or 'json'", "data": nil})
	}

	// Parse filters (same as ListInvoices)
	filter := &invoice.InvoiceListFilter{
		Page:     1,
		PageSize: 1000, // Export up to 1000 invoices
	}

	// Parse filters (same as ListInvoices but without pagination)
	if invoiceIDStr := c.QueryParam("invoice_id"); invoiceIDStr != "" {
		if invoiceID, err := strconv.ParseInt(invoiceIDStr, 10, 64); err == nil {
			filter.InvoiceID = &invoiceID
		}
	}

	if paymentID := c.QueryParam("payment_id"); paymentID != "" {
		filter.PaymentID = &paymentID
	}

	if irn := c.QueryParam("irn"); irn != "" {
		filter.IRN = &irn
	}

	if gstinRecipient := c.QueryParam("gstin_recipient"); gstinRecipient != "" {
		filter.GSTINRecipient = &gstinRecipient
	}

	if irnStatus := c.QueryParam("irn_status"); irnStatus != "" {
		status := invoice.IRNStatus(irnStatus)
		if status.IsValid() {
			filter.IRNStatus = &status
		}
	}

	if filingStatus := c.QueryParam("filing_status"); filingStatus != "" {
		status := invoice.FilingStatus(filingStatus)
		if status.IsValid() {
			filter.FilingStatus = &status
		}
	}

	// Parse date range
	if dateFromStr := c.QueryParam("date_from"); dateFromStr != "" {
		if dateFrom, err := time.Parse("2006-01-02", dateFromStr); err == nil {
			filter.DateFrom = &dateFrom
		}
	}

	if dateToStr := c.QueryParam("date_to"); dateToStr != "" {
		if dateTo, err := time.Parse("2006-01-02", dateToStr); err == nil {
			filter.DateTo = &dateTo
		}
	}

	// Fetch invoices
	response, err := h.invoiceService.ListInvoices(c.Request().Context(), filter)
	if err != nil {
		h.logger.Error(fmt.Sprintf("Error fetching invoices for export: %v", err))
		return c.JSON(500, map[string]any{"message": "Failed to fetch invoices", "data": nil})
	}

	if len(response.Invoices) == 0 {
		return c.JSON(404, map[string]any{"message": "No invoices found for export", "data": nil})
	}

	// Export based on format
	timestamp := time.Now().Format("20060102-150405")

	switch exportFormat {
	case invoice.ExportFormatJSON:
		// Export all invoices as JSON array
		c.Response().Header().Set("Content-Type", "application/json")
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoices-%s.json", timestamp))

		jsonData, err := json.MarshalIndent(response.Invoices, "", "  ")
		if err != nil {
			h.logger.Error(fmt.Sprintf("Error marshaling JSON: %v", err))
			return c.JSON(500, map[string]any{"message": "Failed to generate JSON", "data": nil})
		}

		return c.Blob(200, "application/json", jsonData)

	case invoice.ExportFormatPDF:
		// For PDF, export only the first invoice (or implement multi-invoice PDF)
		if len(response.Invoices) == 1 {
			pdfData, err := h.pdfGenerator.GenerateInvoicePDF(&response.Invoices[0])
			if err != nil {
				h.logger.Error(fmt.Sprintf("Error generating PDF: %v", err))
				return c.JSON(500, map[string]any{"message": "Failed to generate PDF", "data": nil})
			}

			c.Response().Header().Set("Content-Type", "application/pdf")
			c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.pdf", response.Invoices[0].InvoiceNumber))
			return c.Blob(200, "application/pdf", pdfData)
		} else {
			// For multiple invoices, suggest using JSON or provide single invoice_id
			return c.JSON(400, map[string]any{
				"message": "PDF export supports single invoice only. Please provide invoice_id or use JSON format for multiple invoices",
				"data": map[string]any{
					"invoice_count": len(response.Invoices),
					"suggestion":    "Use format=json for multiple invoices or add invoice_id parameter for single invoice PDF",
				},
			})
		}

	default:
		return c.JSON(400, map[string]any{"message": "Invalid format", "data": nil})
	}
}
