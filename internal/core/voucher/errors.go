package voucher

import "errors"

// Domain errors for voucher operations
var (
	// Validation errors
	ErrInvalidBusinessID    = errors.New("invalid business ID")
	ErrInvalidPayeeID       = errors.New("invalid payee ID")
	ErrInvalidAccountID     = errors.New("invalid account ID")
	ErrInvalidGrossAmount   = errors.New("gross amount must be greater than zero")
	ErrInvalidNetAmount     = errors.New("net amount must be greater than zero")
	ErrInvalidCurrency      = errors.New("currency is required")
	ErrInvalidVoucherDate   = errors.New("invalid voucher date")
	ErrInvalidFiscalYear    = errors.New("invalid fiscal year")
	ErrInvalidInvoiceNumber = errors.New("invoice number is required")

	// State errors
	ErrVoucherNotFound         = errors.New("voucher not found")
	ErrVoucherAlreadyExists    = errors.New("voucher with this number already exists")
	ErrVoucherNotDraft         = errors.New("voucher is not in draft status")
	ErrVoucherNotFinalized     = errors.New("voucher is not finalized")
	ErrVoucherAlreadyFinalized = errors.New("voucher is already finalized")
	ErrVoucherAlreadySynced    = errors.New("voucher is already synced to ERP")
	ErrVoucherLocked           = errors.New("voucher is locked (payment already in progress)")

	// Business rule errors
	ErrAmountMismatch       = errors.New("amount calculation mismatch")
	ErrPayeeNotFound        = errors.New("payee not found for business")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentNotSuccessful = errors.New("payment is not successful")

	// ERP errors
	ErrERPSyncFailed         = errors.New("ERP sync failed")
	ErrERPNotConfigured      = errors.New("ERP is not configured for this business")
	ErrERPMaxRetriesExceeded = errors.New("ERP sync max retries exceeded")
)
