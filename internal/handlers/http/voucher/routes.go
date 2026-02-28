package voucher

import (
	"github.com/labstack/echo/v4"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/middleware"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/ratelimiter"
	"xcentro.in/internal/shared/logger"
)

// RegisterVoucherRoutes registers all voucher-related routes
func RegisterVoucherRoutes(group *echo.Group, voucherService voucherSvc.VoucherService, authService auth.AuthService, cookieManager *httputil.CookieManager) {
	handler := NewVoucherHandler(voucherService, logger.GetDefault())

	group.Use(middleware.JWTMiddleware(authService))
	group.Use(middleware.CSRFMiddleware(cookieManager))
	group.Use(ratelimiter.MiddlewareDefault())

	// Voucher management endpoints
	group.POST("/draft", handler.CreateDraftVoucher)                         // POST /vouchers/draft - Create draft voucher
	group.POST("/multi", handler.CreateMultiVouchers)                        // POST /vouchers/multi - Multi-bill payment
	group.POST("/batch-approve", handler.BatchApproveVouchers)               // POST /vouchers/batch-approve - Batch approve multiple vouchers
	// group.GET("", handler.ListVouchers)                                   // GET /vouchers - disabled until business motive is finalized
	group.GET("/list", handler.GetVoucherList)                               // GET /vouchers/list - Get bills with payment status (paid, open, overdue)
	group.GET("/unpaid", handler.GetUnpaidBills)                             // GET /vouchers/unpaid - Get unpaid bills (role-based: owner sees all, others see approved only)
	group.GET("/counts", handler.GetVoucherCounts)                           // GET /vouchers/counts - Get voucher counts by payment status (all, paid, open, overdue)
	group.GET("/calendar", handler.GetCalendarView)                          // GET /vouchers/calendar?year=2024&month=11 - Get bills for calendar view by month
	group.GET("/approval-inbox/counts", handler.GetApprovalInboxCounts)      // GET /vouchers/approval-inbox/counts - Get approval status counts
	group.GET("/approval-inbox", handler.GetApprovalInbox)                   // GET /vouchers/approval-inbox?status={status} - Get approval inbox
	// group.GET("/reminders", handler.GetDraftReminders)                    // GET /vouchers/reminders - disabled until business motive is finalized
	group.GET("/payee/:relation_id", handler.GetVouchersByPayee)             // GET /vouchers/payee/:relation_id - Get vouchers by payee
	group.GET("/:id", handler.GetVoucherDetails)                             // GET /vouchers/:id - Get voucher details
	group.GET("/:id/activity", handler.GetVoucherActivity)                   // GET /vouchers/:id/activity - Get voucher activity history
	group.GET("/:id/payment", handler.GetVoucherPaymentDetails)              // GET /vouchers/:id/payment - Get payment details with breakdown
	group.PUT("/:id", handler.UpdateVoucher)                                 // PUT /vouchers/:id - Update draft voucher
	group.DELETE("/:id", handler.DeleteVoucher)                              // DELETE /vouchers/:id - Delete draft voucher
	group.PUT("/:id/finalize", handler.FinalizeVoucher)                      // PUT /vouchers/:id/finalize - Finalize voucher
	group.POST("/:voucher_id/request-approval", handler.RequestApproval)     // POST /vouchers/:voucher_id/request-approval - Create/update approval
}
