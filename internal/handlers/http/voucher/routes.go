package voucher

import (
	"github.com/labstack/echo/v4"
	redisClient "xcentro.in/internal/adapters/cache/redis/client"
	"xcentro.in/internal/core/auth"
	"xcentro.in/internal/core/business"
	"xcentro.in/internal/middleware"
	voucherSvc "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/httputil"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/ratelimiter"
)

// RegisterVoucherRoutes registers all voucher-related routes
func RegisterVoucherRoutes(group *echo.Group, voucherService voucherSvc.VoucherService, uploadHandler *DocumentUploadHandler, reviewHandler *DocumentReviewHandler, authService auth.AuthService, businessUserRelRepo business.BusinessUserRelationRepository, rclient *redisClient.RedisClient, cookieManager *httputil.CookieManager) {
	handler := NewVoucherHandler(voucherService, logger.GetDefault())

	group.Use(middleware.JWTMiddleware(authService))
	group.Use(middleware.BusinessAccessMiddleware(businessUserRelRepo, rclient))
	group.Use(middleware.CSRFMiddleware(cookieManager))
	group.Use(middleware.BusinessRolesMiddleware(businessUserRelRepo, rclient))
	group.Use(ratelimiter.MiddlewareDefault())

	// Voucher Document ingestion endpoint
	group.POST("/upload", uploadHandler.UploadDocument) // POST /vouchers/upload - Upload Invoice/PO document

	// Voucher management endpoints
	group.POST("/draft", handler.CreateDraftVoucher)                         // POST /vouchers/draft - Create draft voucher
	group.POST("/multi", handler.CreateMultiVouchers)                        // POST /vouchers/multi - Multi-bill payment
	group.POST("/batch-approve", handler.BatchApproveVouchers)               // POST /vouchers/batch-approve - Batch approve multiple vouchers
	group.GET("", handler.ListVouchers)                                      // GET /vouchers - List vouchers with filters
	group.GET("/list", handler.GetVoucherList)                               // GET /vouchers/list - Get bills with payment status (paid, open, overdue)
	group.GET("/unpaid", handler.GetUnpaidBills)                             // GET /vouchers/unpaid - Get unpaid bills (role-based: owner sees all, others see approved only)
	group.GET("/counts", handler.GetVoucherCounts)                           // GET /vouchers/counts - Get voucher counts by payment status (all, paid, open, overdue)
	group.GET("/calendar", handler.GetCalendarView)                          // GET /vouchers/calendar?year=2024&month=11 - Get bills for calendar view by month
	group.GET("/approval-inbox/counts", handler.GetApprovalInboxCounts)      // GET /vouchers/approval-inbox/counts - Get approval status counts
	group.GET("/approval-inbox", handler.GetApprovalInbox)                   // GET /vouchers/approval-inbox?status={status} - Get approval inbox
	group.GET("/reminders", handler.GetDraftReminders)                       // GET /vouchers/reminders - Get abandoned drafts
	group.GET("/payee/:relation_id", handler.GetVouchersByPayee)             // GET /vouchers/payee/:relation_id - Get vouchers by payee
	group.GET("/:id", handler.GetVoucherDetails)                             // GET /vouchers/:id - Get voucher details
	group.GET("/:id/activity", handler.GetVoucherActivity)                   // GET /vouchers/:id/activity - Get voucher activity history
	group.GET("/:id/payment", handler.GetVoucherPaymentDetails)              // GET /vouchers/:id/payment - Get payment details with breakdown
	group.PUT("/:id", handler.UpdateVoucher)                                 // PUT /vouchers/:id - Update draft voucher
	group.DELETE("/:id", handler.DeleteVoucher)                              // DELETE /vouchers/:id - Delete draft voucher
	group.PUT("/:id/finalize", handler.FinalizeVoucher)                      // PUT /vouchers/:id/finalize - Finalize voucher
	group.POST("/:voucher_id/request-approval", handler.RequestApproval)     // POST /vouchers/:voucher_id/request-approval - Create/update approval

	// Document review HITL endpoints
	group.GET("/documents", reviewHandler.ListDocuments)                   // GET /vouchers/documents - List voucher documents
	group.GET("/invoices", reviewHandler.ListInvoices)                     // GET /vouchers/invoices - List extracted invoices
	group.GET("/purchase-orders", reviewHandler.ListPurchaseOrders)         // GET /vouchers/purchase-orders - List purchase orders
	group.GET("/:document_id/draft", reviewHandler.GetDocumentDraft)       // GET /vouchers/:document_id/draft - Get extracted draft data
	group.GET("/:document_id/status", reviewHandler.GetDocumentStatus)     // GET /vouchers/:document_id/status - Get processing status
	group.POST("/:document_id/validate", reviewHandler.ValidateDocument)   // POST /vouchers/:document_id/validate - Validate with corrections
}
