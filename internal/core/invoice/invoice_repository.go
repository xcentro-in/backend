package invoice

import "context"

// InvoiceRepository defines database operations for invoices
type InvoiceRepository interface {
	// Core CRUD operations
	CreateInvoice(ctx context.Context, invoice *Invoice) error
	GetInvoiceByID(ctx context.Context, invoiceID int64) (*Invoice, error)
	GetInvoiceByPaymentID(ctx context.Context, paymentID string) (*Invoice, error)

	// List operations with filtering and pagination
	ListInvoices(ctx context.Context, filter *InvoiceListFilter) ([]Invoice, int64, error)

	// Update operations
	UpdateIRNDetails(ctx context.Context, invoiceID int64, irn, ackNo string, ackDate *string, status IRNStatus) error
	UpdateFilingStatus(ctx context.Context, invoiceID int64, status FilingStatus) error
	UpdateQRCode(ctx context.Context, invoiceID int64, qrCode string) error

	// Delete operations
	DeleteInvoice(ctx context.Context, invoiceID int64) error

	// Transaction support
	WithTx(tx interface{}) InvoiceRepository
}
