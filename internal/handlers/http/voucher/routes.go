package voucher

import (
	"github.com/labstack/echo/v4"
	voucherSvc "xcentro.in/internal/services/voucher"
)

// RegisterVoucherRoutes registers all voucher-related routes
func RegisterVoucherRoutes(group *echo.Group, voucherService voucherSvc.VoucherService) {
	handler := NewVoucherHandler(voucherService)

	// Voucher management endpoints
	group.POST("/draft", handler.CreateDraftVoucher)               // POST /vouchers/draft - Create draft voucher
	group.POST("/multi", handler.CreateMultiVouchers)              // POST /vouchers/multi - Multi-bill payment
	group.GET("", handler.ListVouchers)                            // GET /vouchers - List vouchers with filters
	group.GET("/reminders", handler.GetDraftReminders)             // GET /vouchers/reminders - Get abandoned drafts
	group.GET("/payee/:relation_id", handler.GetVouchersByPayee)  // GET /vouchers/payee/:relation_id - Get vouchers by payee
	group.GET("/:id", handler.GetVoucherDetails)                   // GET /vouchers/:id - Get voucher details
	group.PUT("/:id", handler.UpdateVoucher)                       // PUT /vouchers/:id - Update draft voucher
	group.PUT("/:id/finalize", handler.FinalizeVoucher)            // PUT /vouchers/:id/finalize - Finalize voucher
}
