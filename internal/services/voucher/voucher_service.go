package voucher

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
)

// VoucherService defines the interface for voucher orchestration operations
type VoucherService interface {
	// CreateDraftVoucher creates a draft voucher (called when user clicks "Next" on payment details page)
	CreateDraftVoucher(ctx context.Context, req *CreateDraftVoucherRequest) (*CreateDraftVoucherResponse, error)

	// FinalizeVoucher finalizes a draft voucher after payment success
	FinalizeVoucher(ctx context.Context, req *FinalizeVoucherRequest) (*FinalizeVoucherResponse, error)

	// CreateMultiVouchers creates multiple vouchers for multi-bill payment
	CreateMultiVouchers(ctx context.Context, req *CreateMultiVouchersRequest) (*CreateMultiVouchersResponse, error)

	// GetVoucherDetails retrieves enriched voucher details with payee information
	GetVoucherDetails(ctx context.Context, voucherID uuid.UUID) (*VoucherDetailsResponse, error)

	// ListVouchersForBusiness lists vouchers for a business with filters and pagination
	ListVouchersForBusiness(ctx context.Context, businessID uuid.UUID, filters voucher.VoucherFilters) (*VoucherListResponse, error)

	// GetDraftReminders retrieves abandoned draft vouchers for reminder notifications
	GetDraftReminders(ctx context.Context, req *GetDraftRemindersRequest) (*GetDraftRemindersResponse, error)

	// EnqueueForERPSync adds a voucher to ERP sync queue
	EnqueueForERPSync(ctx context.Context, voucherID uuid.UUID) error
}

// VoucherServiceImpl implements the VoucherService interface
type VoucherServiceImpl struct {
	voucherRepo  voucher.VoucherRepository
	approvalRepo voucher.ApprovalRepository
	queueRepo    voucher.ERPSyncQueueRepository
	voucherCore  voucher.VoucherService // Domain service for business logic
	// TODO: Add payeeRepo when needed for enrichment
	// payeeRepo    payee.PayeeRepository
}

// NewVoucherService creates a new instance of VoucherService
func NewVoucherService(
	voucherRepo voucher.VoucherRepository,
	approvalRepo voucher.ApprovalRepository,
	queueRepo voucher.ERPSyncQueueRepository,
	voucherCore voucher.VoucherService,
) VoucherService {
	return &VoucherServiceImpl{
		voucherRepo:  voucherRepo,
		approvalRepo: approvalRepo,
		queueRepo:    queueRepo,
		voucherCore:  voucherCore,
	}
}

// CreateDraftVoucher creates a draft voucher
func (s *VoucherServiceImpl) CreateDraftVoucher(ctx context.Context, req *CreateDraftVoucherRequest) (*CreateDraftVoucherResponse, error) {
	// 1. Validate payee-account relationship exists
	// TODO: Uncomment when payee repository is integrated
	// relation, err := s.payeeRepo.GetRelation(ctx, req.BusinessID, req.PayeeID, req.AccountID)
	// if err != nil {
	//     return nil, voucher.ErrPayeeNotFound
	// }

	// 2. Generate voucher number
	now := time.Now()
	fiscalYear := s.voucherCore.CalculateFiscalYear(now)
	accountingPeriod := s.voucherCore.CalculateAccountingPeriod(now)

	voucherNumber, err := s.voucherCore.GenerateVoucherNumber(ctx, req.BusinessID, fiscalYear)
	if err != nil {
		return nil, fmt.Errorf("failed to generate voucher number: %w", err)
	}

	// 3. Set default currency if not provided
	currency := req.Currency
	if currency == "" {
		currency = "INR"
	}

	// 4. Create voucher entity
	v := &voucher.Voucher{
		VoucherID:        uuid.New(),
		VoucherNumber:    voucherNumber,
		VoucherDate:      now,
		FiscalYear:       fiscalYear,
		AccountingPeriod: accountingPeriod,
		VoucherType:      voucher.VoucherTypePayment,
		BusinessID:       req.BusinessID,
		PayeeID:          req.PayeeID,
		AccountID:        req.AccountID,
		InvoiceNumber:    req.InvoiceNumber,
		InvoiceDate:      req.InvoiceDate,
		InvoiceDueDate:   req.InvoiceDueDate,
		GrossAmount:      req.GrossAmount,
		NetAmount:        req.NetAmount,
		Currency:         currency,
		GSTAmount:        req.GSTAmount,
		GSTRate:          req.GSTRate,
		CGSTAmount:       req.CGSTAmount,
		SGSTAmount:       req.SGSTAmount,
		IGSTAmount:       req.IGSTAmount,
		TDSAmount:        req.TDSAmount,
		TDSRate:          req.TDSRate,
		TDSSection:       req.TDSSection,
		FeeAmount:        req.FeeAmount,
		DiscountAmount:   req.DiscountAmount,
		FinancialDetails: req.FinancialDetails,
		Attachments:      req.Attachments,
		ExpenseCategory:  req.ExpenseCategory,
		Description:      req.Description,
		Status:           voucher.StatusDraft,
		PaymentID:        nil, // NULL when draft
		ERPSyncStatus:    voucher.ERPNotSynced,
		ERPSyncAttempts:  0,
		GLExpenseAccount: req.GLExpenseAccount,
		GLBankAccount:    req.GLBankAccount,
		CostCenter:       req.CostCenter,
		CreatedAt:        now,
		CreatedByID:      &req.UserID,
		Metadata:         req.Metadata,
	}

	// 5. Validate voucher data
	if err := s.voucherCore.ValidateVoucherData(ctx, v); err != nil {
		return nil, fmt.Errorf("voucher validation failed: %w", err)
	}

	// 6. Save draft voucher
	if err := s.voucherRepo.CreateDraft(ctx, v); err != nil {
		return nil, fmt.Errorf("failed to create draft voucher: %w", err)
	}

	// 7. Return response
	return &CreateDraftVoucherResponse{
		VoucherID:        v.VoucherID,
		VoucherNumber:    v.VoucherNumber,
		VoucherDate:      v.VoucherDate,
		FiscalYear:       v.FiscalYear,
		AccountingPeriod: v.AccountingPeriod,
		Status:           v.Status,
		ERPSyncStatus:    v.ERPSyncStatus,
		Message:          fmt.Sprintf("Draft voucher %s created successfully", v.VoucherNumber),
	}, nil
}

// FinalizeVoucher finalizes a draft voucher after payment success
func (s *VoucherServiceImpl) FinalizeVoucher(ctx context.Context, req *FinalizeVoucherRequest) (*FinalizeVoucherResponse, error) {
	// 1. Verify voucher exists and is in draft status
	v, err := s.voucherRepo.GetByID(ctx, req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	if !v.CanBeFinalized() {
		return nil, voucher.ErrVoucherNotDraft
	}

	// 2. TODO: Verify payment exists and is successful
	// payment, err := s.paymentRepo.GetByID(ctx, req.PaymentID)
	// if err != nil {
	//     return nil, voucher.ErrPaymentNotFound
	// }
	// if payment.Status != "success" {
	//     return nil, voucher.ErrPaymentNotSuccessful
	// }

	// 3. Finalize voucher
	if err := s.voucherRepo.FinalizeVoucher(ctx, req.VoucherID, req.PaymentID); err != nil {
		return nil, fmt.Errorf("failed to finalize voucher: %w", err)
	}

	// 4. Enqueue for ERP sync (if configured)
	// TODO: Make this configurable per business
	if err := s.EnqueueForERPSync(ctx, req.VoucherID); err != nil {
		// Log error but don't fail finalization
		// logger.Errorf("Failed to enqueue voucher for ERP sync: %v", err)
	}

	// 5. Return response
	return &FinalizeVoucherResponse{
		VoucherID:     req.VoucherID,
		VoucherNumber: v.VoucherNumber,
		Status:        voucher.StatusFinalized,
		PaymentID:     req.PaymentID,
		Message:       fmt.Sprintf("Voucher %s finalized successfully", v.VoucherNumber),
	}, nil
}

// CreateMultiVouchers creates multiple vouchers for multi-bill payment
func (s *VoucherServiceImpl) CreateMultiVouchers(ctx context.Context, req *CreateMultiVouchersRequest) (*CreateMultiVouchersResponse, error) {
	var responses []CreateDraftVoucherResponse

	// Create a draft voucher for each bill
	for _, bill := range req.Vouchers {
		draftReq := &CreateDraftVoucherRequest{
			BusinessID:       req.BusinessID,
			UserID:           req.UserID,
			PayeeID:          bill.PayeeID,
			AccountID:        bill.AccountID,
			InvoiceNumber:    bill.InvoiceNumber,
			InvoiceDate:      bill.InvoiceDate,
			GrossAmount:      bill.GrossAmount,
			NetAmount:        bill.NetAmount,
			Currency:         "INR",
			GSTAmount:        bill.GSTAmount,
			TDSAmount:        bill.TDSAmount,
			ExpenseCategory:  bill.ExpenseCategory,
			Description:      bill.Description,
			FinancialDetails: bill.FinancialDetails,
		}

		resp, err := s.CreateDraftVoucher(ctx, draftReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create voucher for invoice %s: %w", bill.InvoiceNumber, err)
		}

		responses = append(responses, *resp)
	}

	return &CreateMultiVouchersResponse{
		Vouchers: responses,
		Message:  fmt.Sprintf("Successfully created %d draft vouchers", len(responses)),
	}, nil
}

// GetVoucherDetails retrieves enriched voucher details
func (s *VoucherServiceImpl) GetVoucherDetails(ctx context.Context, voucherID uuid.UUID) (*VoucherDetailsResponse, error) {
	// 1. Get voucher
	v, err := s.voucherRepo.GetByID(ctx, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. TODO: Enrich with payee information
	// payee, err := s.payeeRepo.GetByID(ctx, v.PayeeID)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to get payee: %w", err)
	// }

	// 3. TODO: Enrich with bank account information
	// account, err := s.payeeRepo.GetAccountByID(ctx, v.AccountID)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to get account: %w", err)
	// }

	// 4. Build response (currently without enrichment)
	response := &VoucherDetailsResponse{
		VoucherID:        v.VoucherID,
		VoucherNumber:    v.VoucherNumber,
		VoucherDate:      v.VoucherDate,
		FiscalYear:       v.FiscalYear,
		AccountingPeriod: v.AccountingPeriod,
		VoucherType:      v.VoucherType,
		Status:           v.Status,
		// TODO: Add payee enrichment
		// PayeeName:    payee.DisplayName,
		// PayeeGSTIN:   payee.GSTIN,
		// PayeeType:    payee.Type,
		// BankAccountMasked: maskAccountNumber(account.AccountNumber),
		// IFSCCode:     account.IFSCCode,
		// BankName:     account.BankName,
		InvoiceNumber:    v.InvoiceNumber,
		InvoiceDate:      v.InvoiceDate,
		InvoiceDueDate:   v.InvoiceDueDate,
		GrossAmount:      v.GrossAmount,
		NetAmount:        v.NetAmount,
		Currency:         v.Currency,
		GSTAmount:        v.GSTAmount,
		GSTRate:          v.GSTRate,
		CGSTAmount:       v.CGSTAmount,
		SGSTAmount:       v.SGSTAmount,
		IGSTAmount:       v.IGSTAmount,
		TDSAmount:        v.TDSAmount,
		TDSRate:          v.TDSRate,
		TDSSection:       v.TDSSection,
		FeeAmount:        v.FeeAmount,
		DiscountAmount:   v.DiscountAmount,
		FinancialDetails: v.FinancialDetails,
		Attachments:      v.Attachments,
		ExpenseCategory:  v.ExpenseCategory,
		Description:      v.Description,
		PaymentID:        v.PaymentID,
		ERPSyncStatus:    v.ERPSyncStatus,
		ERPReferenceID:   v.ERPReferenceID,
		ERPPostingDate:   v.ERPPostingDate,
		ERPLastSyncAt:    v.ERPLastSyncAt,
		ERPSyncAttempts:  v.ERPSyncAttempts,
		GLExpenseAccount: v.GLExpenseAccount,
		GLBankAccount:    v.GLBankAccount,
		CostCenter:       v.CostCenter,
		CreatedAt:        v.CreatedAt,
		UpdatedAt:        v.UpdatedAt,
	}

	return response, nil
}

// ListVouchersForBusiness lists vouchers for a business
func (s *VoucherServiceImpl) ListVouchersForBusiness(ctx context.Context, businessID uuid.UUID, filters voucher.VoucherFilters) (*VoucherListResponse, error) {
	// 1. Get vouchers with pagination
	vouchers, pagination, err := s.voucherRepo.ListByBusiness(ctx, businessID, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list vouchers: %w", err)
	}

	// 2. Convert to list items
	var items []VoucherListItem
	for _, v := range vouchers {
		// TODO: Enrich with payee name
		// payee, _ := s.payeeRepo.GetByID(ctx, v.PayeeID)
		payeeName := "Unknown" // Placeholder

		items = append(items, VoucherListItem{
			VoucherID:       v.VoucherID,
			VoucherNumber:   v.VoucherNumber,
			VoucherDate:     v.VoucherDate,
			FiscalYear:      v.FiscalYear,
			PayeeName:       payeeName,
			InvoiceNumber:   v.InvoiceNumber,
			NetAmount:       v.NetAmount,
			Currency:        v.Currency,
			Status:          v.Status,
			ERPSyncStatus:   v.ERPSyncStatus,
			ExpenseCategory: v.ExpenseCategory,
			CreatedAt:       v.CreatedAt,
		})
	}

	// 3. Calculate summary (optional)
	summary := s.calculateSummary(vouchers)

	return &VoucherListResponse{
		Vouchers:   items,
		Pagination: pagination,
		Summary:    summary,
	}, nil
}

// GetDraftReminders retrieves abandoned draft vouchers
func (s *VoucherServiceImpl) GetDraftReminders(ctx context.Context, req *GetDraftRemindersRequest) (*GetDraftRemindersResponse, error) {
	// 1. Get draft vouchers older than threshold
	drafts, err := s.voucherRepo.GetDraftVouchers(ctx, req.BusinessID, req.OlderThan)
	if err != nil {
		return nil, fmt.Errorf("failed to get draft vouchers: %w", err)
	}

	// 2. Convert to reminder items
	var items []DraftReminderItem
	for _, v := range drafts {
		// TODO: Enrich with payee name
		// payee, _ := s.payeeRepo.GetByID(ctx, v.PayeeID)
		payeeName := "Unknown" // Placeholder

		ageHours := int(time.Since(v.CreatedAt).Hours())

		items = append(items, DraftReminderItem{
			VoucherID:     v.VoucherID,
			VoucherNumber: v.VoucherNumber,
			PayeeName:     payeeName,
			NetAmount:     v.NetAmount,
			CreatedAt:     v.CreatedAt,
			AgeHours:      ageHours,
		})
	}

	return &GetDraftRemindersResponse{
		Drafts:  items,
		Count:   len(items),
		Message: fmt.Sprintf("Found %d abandoned draft vouchers", len(items)),
	}, nil
}

// EnqueueForERPSync adds a voucher to ERP sync queue
func (s *VoucherServiceImpl) EnqueueForERPSync(ctx context.Context, voucherID uuid.UUID) error {
	// 1. Get voucher
	v, err := s.voucherRepo.GetByID(ctx, voucherID)
	if err != nil {
		return fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Check if already synced or in queue
	if v.ERPSyncStatus == voucher.ERPSynced {
		return nil // Already synced, skip
	}

	// 3. Create queue entry
	queueEntry := &voucher.ERPSyncQueue{
		QueueID:       uuid.New(),
		VoucherID:     voucherID,
		SyncDirection: "push", // Push to ERP
		QueueStatus:   "pending",
		RetryCount:    0,
		MaxRetries:    3,
		RequestPayload: map[string]interface{}{
			"voucher_id":     voucherID.String(),
			"voucher_number": v.VoucherNumber,
			"business_id":    v.BusinessID.String(),
		},
		CreatedAt: time.Now(),
	}

	// 4. Enqueue
	if err := s.queueRepo.Enqueue(ctx, queueEntry); err != nil {
		return fmt.Errorf("failed to enqueue for ERP sync: %w", err)
	}

	// 5. Update voucher ERP status to pending
	if err := s.voucherRepo.UpdateERPStatus(ctx, voucherID, voucher.ERPPending, "", nil); err != nil {
		// Log error but don't fail
		// logger.Errorf("Failed to update ERP status: %v", err)
	}

	return nil
}

// Helper methods

// calculateSummary calculates aggregate statistics for voucher list
func (s *VoucherServiceImpl) calculateSummary(vouchers []*voucher.Voucher) *VoucherListSummary {
	summary := &VoucherListSummary{
		TotalVouchers: int64(len(vouchers)),
	}

	for _, v := range vouchers {
		summary.TotalAmount += v.NetAmount

		switch v.Status {
		case voucher.StatusDraft:
			summary.DraftCount++
		case voucher.StatusFinalized, voucher.StatusPosted:
			summary.FinalizedCount++
		case voucher.StatusSynced:
			summary.SyncedCount++
		}

		if v.ERPSyncStatus == voucher.ERPNotSynced || v.ERPSyncStatus == voucher.ERPFailed {
			summary.PendingSyncCount++
		}
	}

	return summary
}

// maskAccountNumber masks account number for security (show last 4 digits only)
func maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
}
