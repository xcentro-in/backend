package payment

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/payment"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/templates"
)

// PaymentCallbackHandler handles payment redirect callbacks (user-facing HTML pages)
type PaymentCallbackHandler struct {
	paymentRepo payment.PaymentRepository
	logger      *logger.Logger
}

// NewPaymentCallbackHandler creates a new payment callback handler
func NewPaymentCallbackHandler(paymentRepo payment.PaymentRepository) *PaymentCallbackHandler {
	return &PaymentCallbackHandler{
		paymentRepo: paymentRepo,
		logger:      logger.GetDefault(),
	}
}

// HandlePaymentCallback handles the payment callback redirect (GET)
// This is where the user lands after completing/canceling payment at gateway
// Returns HTML response for user feedback
func (h *PaymentCallbackHandler) HandlePaymentCallback(c echo.Context) error {
	ctx := c.Request().Context()

	// Get transaction ID from query params
	transactionID := c.QueryParam("transaction_id")
	if transactionID == "" {
		h.logger.Error("Missing transaction_id in payment callback")
		return h.renderErrorPage(c, "Invalid Request", "Payment transaction ID is missing")
	}

	// Get status from query params (optional - some gateways include this)
	statusParam := c.QueryParam("status")

	h.logger.InfoWith("Payment callback received",
		"transaction_id", transactionID,
		"status_param", statusParam)

	// Fetch payment from database
	pmt, err := h.paymentRepo.GetPaymentByID(ctx, transactionID)
	if err != nil {
		h.logger.ErrorWith("Failed to fetch payment in callback", "error", err, "transaction_id", transactionID)
		return h.renderErrorPage(c, "Payment Not Found", "Unable to locate your payment transaction")
	}

	if pmt == nil {
		h.logger.ErrorWith("Payment not found in callback", "transaction_id", transactionID)
		return h.renderErrorPage(c, "Payment Not Found", "Unable to locate your payment transaction")
	}

	// Render appropriate page based on payment status
	switch pmt.Status {
	case payment.PaymentStatusSuccess:
		return h.renderSuccessPage(c, transactionID, pmt)

	case payment.PaymentStatusFailed:
		return h.renderFailedPage(c, transactionID, pmt)

	case payment.PaymentStatusCancelled:
		return h.renderFailedPage(c, transactionID, pmt)

	case payment.PaymentStatusPending:
		return h.renderPendingPage(c, transactionID, pmt)

	default:
		h.logger.WarnWith("Unknown payment status in callback", "status", pmt.Status, "transaction_id", transactionID)
		return h.renderPendingPage(c, transactionID, pmt)
	}
}

// renderSuccessPage renders the payment success page
func (h *PaymentCallbackHandler) renderSuccessPage(c echo.Context, transactionID string, pmt *payment.Payment) error {
	// Format amount for display (convert paise to rupees)
	amountINR := float64(pmt.Amount) / 100.0

	html := templates.GenerateResponsePage(templates.ResponsePageConfig{
		PageType:     templates.PageTypeSuccess,
		Title:        "Payment Successful!",
		Message:      "Thank you for your payment.",
		SubMessage:   fmt.Sprintf("✓ Payment of ₹%.2f has been processed successfully", amountINR),
		AutoCloseSec: 3,
		ShowCloseHint: true,
		AdditionalHTML: fmt.Sprintf(`
        <div style="background: #f5f5f5; padding: 16px; border-radius: 8px; margin-top: 16px;">
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Transaction ID:</strong> %s</p>
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Amount:</strong> ₹%.2f</p>
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Gateway:</strong> %s</p>
        </div>`, transactionID, amountINR, pmt.GatewayUsed),
	})

	return c.HTML(http.StatusOK, html)
}

// renderFailedPage renders the payment failed page
func (h *PaymentCallbackHandler) renderFailedPage(c echo.Context, transactionID string, pmt *payment.Payment) error {
	// Format amount for display (convert paise to rupees)
	amountINR := float64(pmt.Amount) / 100.0

	// Get failure reason from metadata if available
	failureReason := "Payment could not be completed"
	if pmt.Metadata != nil {
		if reason, ok := pmt.Metadata["failure_reason"].(string); ok && reason != "" {
			failureReason = reason
		}
	}

	html := templates.GenerateResponsePage(templates.ResponsePageConfig{
		PageType:     templates.PageTypeError,
		Title:        "Payment Failed",
		Message:      "We were unable to process your payment.",
		SubMessage:   fmt.Sprintf("✗ %s", failureReason),
		AutoCloseSec: 5,
		ShowCloseHint: true,
		AdditionalHTML: fmt.Sprintf(`
        <div style="background: #f5f5f5; padding: 16px; border-radius: 8px; margin-top: 16px;">
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Transaction ID:</strong> %s</p>
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Amount:</strong> ₹%.2f</p>
        </div>
        <p style="margin-top: 16px; font-size: 14px;">Please try again or contact support if the issue persists.</p>`,
		transactionID, amountINR),
	})

	return c.HTML(http.StatusOK, html)
}

// renderPendingPage renders the payment pending page
func (h *PaymentCallbackHandler) renderPendingPage(c echo.Context, transactionID string, pmt *payment.Payment) error {
	// Format amount for display (convert paise to rupees)
	amountINR := float64(pmt.Amount) / 100.0

	html := templates.GenerateResponsePage(templates.ResponsePageConfig{
		PageType:     templates.PageTypePending,
		Title:        "Payment Processing",
		Message:      "Your payment is being processed.",
		SubMessage:   "⏳ Please wait while we confirm your payment",
		AutoCloseSec: 5,
		ShowCloseHint: true,
		AdditionalHTML: fmt.Sprintf(`
        <div style="background: #f5f5f5; padding: 16px; border-radius: 8px; margin-top: 16px;">
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Transaction ID:</strong> %s</p>
            <p style="font-size: 14px; color: #5f6368; margin: 4px 0;"><strong>Amount:</strong> ₹%.2f</p>
        </div>
        <p style="margin-top: 16px; font-size: 14px;">You will receive a confirmation shortly. This may take a few minutes.</p>`,
		transactionID, amountINR),
	})

	return c.HTML(http.StatusOK, html)
}

// renderErrorPage renders a generic error page
func (h *PaymentCallbackHandler) renderErrorPage(c echo.Context, title, message string) error {
	html := templates.GenerateResponsePage(templates.ResponsePageConfig{
		PageType:      templates.PageTypeError,
		Title:         title,
		Message:       message,
		SubMessage:    "Please contact support if you need assistance",
		AutoCloseSec:  5,
		ShowCloseHint: true,
	})

	return c.HTML(http.StatusBadRequest, html)
}
