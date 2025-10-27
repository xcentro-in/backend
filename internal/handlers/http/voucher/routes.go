package voucher

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/middleware"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/logger"
)

// RegisterVoucherRoutes registers all voucher-related routes
func RegisterVoucherRoutes(group *echo.Group, voucherService voucherSvc.VoucherService, authService auth.AuthService) {
	handler := NewVoucherHandler(voucherService, logger.GetDefault())

	group.Use(middleware.JWTMiddleware(authService))
	// Voucher management endpoints
	group.POST("/draft", handler.CreateDraftVoucher)                         // POST /vouchers/draft - Create draft voucher
	group.POST("/multi", handler.CreateMultiVouchers)                        // POST /vouchers/multi - Multi-bill payment
	group.GET("", handler.ListVouchers)                                      // GET /vouchers - List vouchers with filters
	group.GET("/list", handler.GetVoucherList)                               // GET /vouchers/list - Get bills with payment status (paid, open, overdue)
	group.GET("/reminders", handler.GetDraftReminders)                       // GET /vouchers/reminders - Get abandoned drafts
	group.GET("/payee/:relation_id", handler.GetVouchersByPayee)             // GET /vouchers/payee/:relation_id - Get vouchers by payee
	group.GET("/:id", handler.GetVoucherDetails)                             // GET /vouchers/:id - Get voucher details
	group.PUT("/:id", handler.UpdateVoucher)                                 // PUT /vouchers/:id - Update draft voucher
	group.PUT("/:id/finalize", handler.FinalizeVoucher)                      // PUT /vouchers/:id/finalize - Finalize voucher
	group.POST("/:voucher_id/request-approval", handler.RequestApproval)     // POST /vouchers/:voucher_id/request-approval - Create/update approval
}
