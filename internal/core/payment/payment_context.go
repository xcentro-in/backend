package payment

// PaymentContext represents the business context for a payment
type PaymentContext string

const (
	PaymentContextVendorPayment PaymentContext = "vendor_payment"
	PaymentContextGSTPayment    PaymentContext = "gst_payment"
	PaymentContextBillPayment   PaymentContext = "bill_payment"
	PaymentContextPayroll       PaymentContext = "payroll"
)

// IsValid checks if the payment context is valid
func (pc PaymentContext) IsValid() bool {
	switch pc {
	case PaymentContextVendorPayment,
		PaymentContextGSTPayment,
		PaymentContextBillPayment,
		PaymentContextPayroll:
		return true
	default:
		return false
	}
}
