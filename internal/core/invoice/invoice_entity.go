package invoice

import "time"

// Invoice represents a GST invoice for a successful payment
type Invoice struct {
	InvoiceID       int64        `json:"invoice_id"`
	PaymentID       string       `json:"payment_id"`
	InvoiceNumber   string       `json:"invoice_number"`
	GSTINSupplier   string       `json:"gstin_supplier"`
	GSTINRecipient  string       `json:"gstin_recipient"`
	InvoiceDate     time.Time    `json:"invoice_date"`
	IRN             *string      `json:"irn,omitempty"`
	IRNAckNo        *string      `json:"irn_ack_no,omitempty"`
	IRNAckDate      *time.Time   `json:"irn_ack_date,omitempty"`
	IRNStatus       IRNStatus    `json:"irn_status"`
	QRCode          *string      `json:"qr_code,omitempty"`
	FilingStatus    FilingStatus `json:"filing_status"`
	Description     string       `json:"description"`
	SACCode         string       `json:"sac_code"`
	TaxableValue    float64      `json:"taxable_value"`
	GSTRate         float64      `json:"gst_rate"`
	GSTAmount       float64      `json:"gst_amount"`
	TotalAmount     float64      `json:"total_amount"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// CreateInvoiceRequest represents a request to create an invoice
type CreateInvoiceRequest struct {
	PaymentID      string    `json:"payment_id" validate:"required,uuid"`
	GSTINRecipient string    `json:"gstin_recipient" validate:"required,len=15"`
	InvoiceDate    time.Time `json:"invoice_date" validate:"required"`
	TaxableValue   float64   `json:"taxable_value" validate:"required,gt=0"`
	GSTRate        float64   `json:"gst_rate" validate:"required,gte=0,lte=100"`
	Description    string    `json:"description,omitempty"`
	SACCode        string    `json:"sac_code,omitempty"`
}

// InvoiceListFilter represents filters for listing invoices
type InvoiceListFilter struct {
	InvoiceID      *int64        `json:"invoice_id,omitempty"`
	PaymentID      *string       `json:"payment_id,omitempty"`
	IRN            *string       `json:"irn,omitempty"`
	GSTINRecipient *string       `json:"gstin_recipient,omitempty"`
	IRNStatus      *IRNStatus    `json:"irn_status,omitempty"`
	FilingStatus   *FilingStatus `json:"filing_status,omitempty"`
	DateFrom       *time.Time    `json:"date_from,omitempty"`
	DateTo         *time.Time    `json:"date_to,omitempty"`
	Page           int           `json:"page"`
	PageSize       int           `json:"page_size"`
}

// InvoiceListResponse represents a paginated list of invoices
type InvoiceListResponse struct {
	Invoices   []Invoice `json:"invoices"`
	TotalCount int64     `json:"total_count"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	TotalPages int       `json:"total_pages"`
}

// InvoiceDownloadRequest represents a request to download an invoice
type InvoiceDownloadRequest struct {
	InvoiceID *int64       `json:"invoice_id,omitempty"`
	PaymentID *string      `json:"payment_id,omitempty"`
	Format    ExportFormat `json:"format" validate:"required,oneof=pdf json"`
}
