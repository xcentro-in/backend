package payee

import (
	"github.com/labstack/echo/v4"
	payeeSvc "xcentro.in/internal/services/payee"
)

// RegisterPayeeRoutes registers all payee-related routes
func RegisterPayeeRoutes(group *echo.Group, payeeService *payeeSvc.PayeeService) {
	handler := NewPayeeHandler(payeeService)

	// Payee management endpoints
	group.POST("", handler.OnboardPayee)                                    // POST /payees
	group.GET("", handler.GetPayees)                                        // GET /payees
	group.GET("/:payee_id/accounts/:account_id", handler.GetPayeeDetails)   // GET /payees/:id/accounts/:id
	group.PUT("/:payee_id/accounts/:account_id", handler.UpdatePayee)       // PUT /payees/:id/accounts/:id
	group.DELETE("/:payee_id/accounts/:account_id", handler.DeactivatePayee) // DELETE /payees/:id/accounts/:id
}
