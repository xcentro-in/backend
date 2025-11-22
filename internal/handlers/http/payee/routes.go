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
	group.Use(middleware.JWTMiddleware(authService))

	// Payee management endpoints
	group.POST("", handler.OnboardPayee)                       // POST /payees
	group.GET("", handler.GetPayees)                           // GET /payees
	group.GET("/details/:relation_id", handler.GetPayeeDetails) // GET /payees/details/:relation_id
	group.GET("/:relation_id/acc_details", handler.GetPayeeAccountDetails) // GET /payees/:relation_id/acc_details - Lightweight account details
	group.DELETE("/:payee_id/accounts/:account_id", handler.DeactivatePayee) // DELETE /payees/:id/accounts/:id (kept for now)

	// Employee remuneration document upload endpoints
	group.GET("/remuneration/upload-url", handler.GetRemunerationDocumentUploadURL)         // GET /payees/remuneration/upload-url?relation_id=xxx
	group.GET("/remuneration/confirm-upload", handler.ConfirmRemunerationDocumentUpload)   // GET /payees/remuneration/confirm-upload?relation_id=xxx
	group.GET("/remuneration/download-url", handler.GetRemunerationDocumentDownloadURL)    // GET /payees/remuneration/download-url?relation_id=xxx
}

// RegisterVendorDropdownRoute registers the lightweight vendor dropdown endpoint
// This is registered separately at /v1/vendors (not under /v1/payees)
func RegisterVendorDropdownRoute(group *echo.Group, payeeService *payeeSvc.PayeeService, authService auth.AuthService) {
	handler := NewPayeeHandler(payeeService, logger.GetDefault())

	// Apply JWT middleware
	vendorGroup := group.Group("/vendors")
	vendorGroup.Use(middleware.JWTMiddleware(authService))

	// Vendor dropdown endpoint (optimized for voucher creation UI)
	vendorGroup.GET("", handler.GetVendorsForDropdown) // GET /v1/vendors
}

// RegisterEmployeeListRoute registers the lightweight employee list endpoint
// This is registered separately at /v1/employees (not under /v1/payees)
func RegisterEmployeeListRoute(group *echo.Group, payeeService *payeeSvc.PayeeService, authService auth.AuthService) {
	handler := NewPayeeHandler(payeeService, logger.GetDefault())

	// Apply JWT middleware
	employeeGroup := group.Group("/employees")
	employeeGroup.Use(middleware.JWTMiddleware(authService))

	// Employee list endpoint (for tabulated display)
	employeeGroup.GET("", handler.GetEmployeesForList) // GET /v1/employees
}
