package payee

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/middleware"
	payeeSvc "xcentro.in/internal/services/payee"
	"xcentro.in/internal/shared/logger"
)

// RegisterPayeeRoutes registers all payee-related routes
func RegisterPayeeRoutes(group *echo.Group, payeeService *payeeSvc.PayeeService, authService auth.AuthService) {
	handler := NewPayeeHandler(payeeService, logger.GetDefault())

	// Apply JWT middleware to all payee routes
	
	// Payee management endpoints
	group.POST("", handler.OnboardPayee, middleware.TurnstileMiddleware())                       // POST /payees
	group.GET("", handler.GetPayees)                           // GET /payees
	group.GET("/details/:relation_id", handler.GetPayeeDetails) // GET /payees/details/:relation_id
	group.GET("/:relation_id/acc_details", handler.GetPayeeAccountDetails) // GET /payees/:relation_id/acc_details - Lightweight account details
	group.PATCH("/:relation_id/status", handler.UpdatePayeeStatus, middleware.TurnstileMiddleware()) // PATCH /payees/:relation_id/status - Toggle active status
	// TODO: wrongly implemented, path is suppsoed to update the payee details
	// TODO: Delete is supposed to deactivate
	// TODO: frontend is calling PATCH to deactivate, need to FIX IT !
	group.DELETE("/:payee_id/accounts/:account_id", handler.DeactivatePayee, middleware.TurnstileMiddleware()) // DELETE /payees/:id/accounts/:id (kept for now)

	// Employee remuneration document upload endpoints
	group.GET("/remuneration/upload-url", handler.GetRemunerationDocumentUploadURL)         // GET /payees/remuneration/upload-url?relation_id=xxx
	group.GET("/remuneration/confirm-upload", handler.ConfirmRemunerationDocumentUpload)   // GET /payees/remuneration/confirm-upload?relation_id=xxx
	group.GET("/remuneration/download-url", handler.GetRemunerationDocumentDownloadURL)    // GET /payees/remuneration/download-url?relation_id=xxx

	// Vendor dropdown endpoint (optimized for voucher creation UI)
	group.GET("/vendors", handler.GetVendorsForDropdown)
	// Employee list endpoint (for tabulated display)
	group.GET("/employees", handler.GetEmployeesForList)
}
