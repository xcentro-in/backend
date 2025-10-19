package payee

import (
	"github.com/labstack/echo/v4"
	payeeSvc "xcentro.in/internal/services/payee"
)

// RegisterPayeeRoutes registers all payee-related routes
func RegisterPayeeRoutes(group *echo.Group, payeeService *payeeSvc.PayeeService) {
	handler := NewPayeeHandler(payeeService)

	// Payee management endpoints
	group.POST("", handler.OnboardPayee)                       // POST /payees
	group.GET("", handler.GetPayees)                           // GET /payees
	group.GET("/details/:relation_id", handler.GetPayeeDetails) // GET /payees/details/:relation_id
	group.DELETE("/:payee_id/accounts/:account_id", handler.DeactivatePayee) // DELETE /payees/:id/accounts/:id (kept for now)
}

// RegisterVendorDropdownRoute registers the lightweight vendor dropdown endpoint
// This is registered separately at /v1/vendors (not under /v1/payees)
func RegisterVendorDropdownRoute(group *echo.Group, payeeService *payeeSvc.PayeeService) {
	handler := NewPayeeHandler(payeeService)

	// Vendor dropdown endpoint (optimized for voucher creation UI)
	group.GET("/vendors", handler.GetVendorsForDropdown) // GET /v1/vendors?bid=xxx
}
