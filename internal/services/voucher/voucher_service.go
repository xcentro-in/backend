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

	// UpdateVoucher updates a draft voucher (only draft vouchers can be updated)
	UpdateVoucher(ctx context.Context, req *UpdateVoucherRequest) (*UpdateVoucherResponse, error)

	// ListVouchersForBusiness lists vouchers for a business with filters and pagination
	ListVouchersForBusiness(ctx context.Context, businessID uuid.UUID, filters voucher.VoucherFilters) (*VoucherListResponse, error)

	// GetDraftReminders retrieves abandoned draft vouchers for reminder notifications
	GetDraftReminders(ctx context.Context, req *GetDraftRemindersRequest) (*GetDraftRemindersResponse, error)

	// EnqueueForERPSync adds a voucher to ERP sync queue
	EnqueueForERPSync(ctx context.Context, voucherID uuid.UUID) error

	// FinalizeVoucherByID is a convenience method that accepts string IDs (for payment integration)
	FinalizeVoucherByID(ctx context.Context, voucherID string, paymentID string) error

	// RequestCancellation requests cancellation of a voucher (Phase 7)
	RequestCancellation(ctx context.Context, req *RequestCancellationRequest) (*RequestCancellationResponse, error)

	// ApproveCancellation approves or rejects a cancellation request (Phase 7)
	ApproveCancellation(ctx context.Context, req *ApproveCancellationRequest) (*ApproveCancellationResponse, error)

	// GetVouchersByPayee retrieves all vouchers for a specific payee (by relation_id) with business validation
	GetVouchersByPayee(ctx context.Context, req *GetVouchersByPayeeRequest) (*GetVouchersByPayeeResponse, error)
}

// VoucherServiceImpl implements the VoucherService interface
type VoucherServiceImpl struct {
	voucherRepo       voucher.VoucherRepository
	approvalRepo      voucher.ApprovalRepository
	queueRepo         voucher.ERPSyncQueueRepository
	voucherCore       voucher.VoucherService // Domain service for business logic
	businessPayeeRepo interface{}            // payee.BusinessPayeeRelationRepository, using interface{} to avoid circular import
}

// NewVoucherService creates a new instance of VoucherService
func NewVoucherService(
	voucherRepo voucher.VoucherRepository,
	approvalRepo voucher.ApprovalRepository,
	queueRepo voucher.ERPSyncQueueRepository,
	voucherCore voucher.VoucherService,
	businessPayeeRepo interface{}, // Repository to resolve relation_id
) VoucherService {
	return &VoucherServiceImpl{
		voucherRepo:       voucherRepo,
		approvalRepo:      approvalRepo,
		queueRepo:         queueRepo,
		voucherCore:       voucherCore,
		businessPayeeRepo: businessPayeeRepo,
	}
}

// CreateDraftVoucher creates a draft voucher
func (s *VoucherServiceImpl) CreateDraftVoucher(ctx context.Context, req *CreateDraftVoucherRequest) (*CreateDraftVoucherResponse, error) {
	// 1. Resolve relation_id to payee_id and account_id (validates business ownership)
	payeeID, accountID, err := s.resolveRelationID(ctx, req.BusinessID, req.RelationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve relation_id: %w", err)
	}

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
		VoucherID:        uuid.Must(uuid.NewV7()),
		VoucherNumber:    voucherNumber,
		VoucherDate:      now,
		FiscalYear:       fiscalYear,
		AccountingPeriod: accountingPeriod,
		VoucherType:      voucher.VoucherTypePayment,
		BusinessID:       req.BusinessID,
		PayeeID:          payeeID,
		AccountID:        accountID,
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
			RelationID:       bill.RelationID, // Use relation_id instead of payee_id + account_id
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

// UpdateVoucher updates a draft voucher (only draft vouchers can be updated)
func (s *VoucherServiceImpl) UpdateVoucher(ctx context.Context, req *UpdateVoucherRequest) (*UpdateVoucherResponse, error) {
	// 1. Fetch existing voucher
	v, err := s.voucherRepo.GetByID(ctx, req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Verify voucher is in draft status (only drafts can be updated)
	if v.Status != voucher.StatusDraft {
		return nil, fmt.Errorf("can only update draft vouchers (current status: %s)", v.Status)
	}

	// 3. Track updated fields
	var updatedFields []string

	// 4. If relation_id is provided, resolve it and update payee/account
	if req.RelationID != nil {
		// Validate and resolve relation_id (validates business ownership)
		payeeID, accountID, err := s.resolveRelationID(ctx, v.BusinessID, *req.RelationID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve relation_id: %w", err)
		}

		v.PayeeID = payeeID
		v.AccountID = accountID
		updatedFields = append(updatedFields, "payee_id", "account_id")
	}

	// 5. Apply partial updates for invoice details
	if req.InvoiceNumber != nil {
		v.InvoiceNumber = *req.InvoiceNumber
		updatedFields = append(updatedFields, "invoice_number")
	}
	if req.InvoiceDate != nil {
		v.InvoiceDate = req.InvoiceDate
		updatedFields = append(updatedFields, "invoice_date")
	}
	if req.InvoiceDueDate != nil {
		v.InvoiceDueDate = req.InvoiceDueDate
		updatedFields = append(updatedFields, "invoice_due_date")
	}

	// 6. Apply partial updates for amounts
	if req.GrossAmount != nil {
		v.GrossAmount = *req.GrossAmount
		updatedFields = append(updatedFields, "gross_amount")
	}
	if req.NetAmount != nil {
		v.NetAmount = *req.NetAmount
		updatedFields = append(updatedFields, "net_amount")
	}
	if req.Currency != nil {
		v.Currency = *req.Currency
		updatedFields = append(updatedFields, "currency")
	}
	if req.GSTAmount != nil {
		v.GSTAmount = *req.GSTAmount
		updatedFields = append(updatedFields, "gst_amount")
	}
	if req.GSTRate != nil {
		v.GSTRate = *req.GSTRate
		updatedFields = append(updatedFields, "gst_rate")
	}
	if req.CGSTAmount != nil {
		v.CGSTAmount = *req.CGSTAmount
		updatedFields = append(updatedFields, "cgst_amount")
	}
	if req.SGSTAmount != nil {
		v.SGSTAmount = *req.SGSTAmount
		updatedFields = append(updatedFields, "sgst_amount")
	}
	if req.IGSTAmount != nil {
		v.IGSTAmount = *req.IGSTAmount
		updatedFields = append(updatedFields, "igst_amount")
	}
	if req.TDSAmount != nil {
		v.TDSAmount = *req.TDSAmount
		updatedFields = append(updatedFields, "tds_amount")
	}
	if req.TDSRate != nil {
		v.TDSRate = *req.TDSRate
		updatedFields = append(updatedFields, "tds_rate")
	}
	if req.TDSSection != nil {
		v.TDSSection = *req.TDSSection
		updatedFields = append(updatedFields, "tds_section")
	}
	if req.FeeAmount != nil {
		v.FeeAmount = *req.FeeAmount
		updatedFields = append(updatedFields, "fee_amount")
	}
	if req.DiscountAmount != nil {
		v.DiscountAmount = *req.DiscountAmount
		updatedFields = append(updatedFields, "discount_amount")
	}

	// 7. Apply partial updates for additional details
	if req.FinancialDetails != nil {
		v.FinancialDetails = *req.FinancialDetails
		updatedFields = append(updatedFields, "financial_details")
	}
	if req.Attachments != nil {
		v.Attachments = *req.Attachments
		updatedFields = append(updatedFields, "attachments")
	}
	if req.ExpenseCategory != nil {
		v.ExpenseCategory = *req.ExpenseCategory
		updatedFields = append(updatedFields, "expense_category")
	}
	if req.Description != nil {
		v.Description = *req.Description
		updatedFields = append(updatedFields, "description")
	}

	// 8. Apply partial updates for GL codes
	if req.GLExpenseAccount != nil {
		v.GLExpenseAccount = *req.GLExpenseAccount
		updatedFields = append(updatedFields, "gl_expense_account")
	}
	if req.GLBankAccount != nil {
		v.GLBankAccount = *req.GLBankAccount
		updatedFields = append(updatedFields, "gl_bank_account")
	}
	if req.CostCenter != nil {
		v.CostCenter = *req.CostCenter
		updatedFields = append(updatedFields, "cost_center")
	}

	// 9. Apply partial updates for metadata
	if req.Metadata != nil {
		v.Metadata = *req.Metadata
		updatedFields = append(updatedFields, "metadata")
	}

	// 10. Update timestamp
	now := time.Now()
	v.UpdatedAt = &now

	// 11. Validate updated voucher data
	if err := s.voucherCore.ValidateVoucherData(ctx, v); err != nil {
		return nil, fmt.Errorf("voucher validation failed: %w", err)
	}

	// 12. Save updated voucher to database
	if err := s.voucherRepo.Update(ctx, v); err != nil {
		return nil, fmt.Errorf("failed to update voucher: %w", err)
	}

	// 13. Return response with updated fields
	return &UpdateVoucherResponse{
		VoucherID:     v.VoucherID,
		VoucherNumber: v.VoucherNumber,
		Status:        v.Status,
		Message:       fmt.Sprintf("Voucher %s updated successfully", v.VoucherNumber),
		UpdatedFields: updatedFields,
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
		QueueID:       uuid.Must(uuid.NewV7()),
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

// FinalizeVoucherByID finalizes a voucher using string IDs (Phase 6: Payment integration)
func (s *VoucherServiceImpl) FinalizeVoucherByID(ctx context.Context, voucherIDStr string, paymentIDStr string) error {
	voucherID, err := uuid.Parse(voucherIDStr)
	if err != nil {
		return fmt.Errorf("invalid voucher_id: %w", err)
	}

	paymentID, err := uuid.Parse(paymentIDStr)
	if err != nil {
		return fmt.Errorf("invalid payment_id: %w", err)
	}

	req := &FinalizeVoucherRequest{
		VoucherID: voucherID,
		PaymentID: paymentID,
	}

	_, err = s.FinalizeVoucher(ctx, req)
	return err
}

// RequestCancellation requests cancellation of a voucher (Phase 7)
func (s *VoucherServiceImpl) RequestCancellation(ctx context.Context, req *RequestCancellationRequest) (*RequestCancellationResponse, error) {
	// 1. Get voucher
	v, err := s.voucherRepo.GetByID(ctx, req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Validate voucher can be cancelled
	// Can only cancel finalized vouchers that haven't been paid out yet
	if v.Status != voucher.StatusFinalized {
		return nil, fmt.Errorf("can only cancel finalized vouchers (current status: %s)", v.Status)
	}

	// Check if already cancelled or cancellation requested
	if v.CancellationStatus != voucher.CancellationStatusNotCancelled {
		return nil, fmt.Errorf("voucher already has cancellation status: %s", v.CancellationStatus)
	}

	// Check if payout already initiated
	if v.PayoutStatus == "completed" {
		return nil, fmt.Errorf("cannot cancel voucher - payout already completed")
	}

	// 3. Update voucher cancellation status
	now := time.Now()
	if err := s.voucherRepo.RequestCancellation(ctx, req.VoucherID, req.UserID, req.Reason, now); err != nil {
		return nil, fmt.Errorf("failed to request cancellation: %w", err)
	}

	// 4. Return response
	return &RequestCancellationResponse{
		VoucherID:          req.VoucherID,
		VoucherNumber:      v.VoucherNumber,
		CancellationStatus: string(voucher.CancellationStatusCancellationRequested),
		Message:            fmt.Sprintf("Cancellation request submitted for voucher %s", v.VoucherNumber),
	}, nil
}

// ApproveCancellation approves or rejects a cancellation request (Phase 7)
func (s *VoucherServiceImpl) ApproveCancellation(ctx context.Context, req *ApproveCancellationRequest) (*ApproveCancellationResponse, error) {
	// 1. Get voucher
	v, err := s.voucherRepo.GetByID(ctx, req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Validate voucher is in cancellation_requested status
	if v.CancellationStatus != voucher.CancellationStatusCancellationRequested {
		return nil, fmt.Errorf("voucher not in cancellation_requested status (current: %s)", v.CancellationStatus)
	}

	// 3. Process approval or rejection
	now := time.Now()
	var newCancellationStatus voucher.CancellationStatus
	var refundStatus voucher.RefundStatus
	var message string

	if req.Action == "approve" {
		// Approve cancellation
		newCancellationStatus = voucher.CancellationStatusCancelled
		refundStatus = voucher.RefundStatusPending // Mark refund as pending

		// Update voucher
		if err := s.voucherRepo.ApproveCancellation(ctx, req.VoucherID, req.UserID, now, refundStatus); err != nil {
			return nil, fmt.Errorf("failed to approve cancellation: %w", err)
		}

		// TODO: Trigger refund worker to process refund to customer
		// This will be implemented in Phase 7.5 or 8

		message = fmt.Sprintf("Cancellation approved for voucher %s. Refund will be initiated.", v.VoucherNumber)
	} else if req.Action == "reject" {
		// Reject cancellation - revert to not_cancelled
		newCancellationStatus = voucher.CancellationStatusNotCancelled
		refundStatus = voucher.RefundStatusNotApplicable

		// Update voucher
		if err := s.voucherRepo.RejectCancellation(ctx, req.VoucherID); err != nil {
			return nil, fmt.Errorf("failed to reject cancellation: %w", err)
		}

		message = fmt.Sprintf("Cancellation rejected for voucher %s. Payout will proceed.", v.VoucherNumber)
	} else {
		return nil, fmt.Errorf("invalid action: %s (must be 'approve' or 'reject')", req.Action)
	}

	// 4. Return response
	return &ApproveCancellationResponse{
		VoucherID:          req.VoucherID,
		VoucherNumber:      v.VoucherNumber,
		CancellationStatus: string(newCancellationStatus),
		RefundStatus:       string(refundStatus),
		Message:            message,
	}, nil
}

// Helper functions

// resolveRelationID validates relation_id belongs to business and returns payee/account IDs
func (s *VoucherServiceImpl) resolveRelationID(ctx context.Context, businessID uuid.UUID, relationID uuid.UUID) (payeeID, accountID uuid.UUID, err error) {
	// Type assert to access ValidateRelationForBusiness method
	repo, ok := s.businessPayeeRepo.(interface {
		ValidateRelationForBusiness(ctx context.Context, businessID, relationID string) (payeeID, accountID string, err error)
	})
	if !ok {
		return uuid.Nil, uuid.Nil, fmt.Errorf("businessPayeeRepo does not implement ValidateRelationForBusiness")
	}

	// Simple SQL query validates and returns IDs
	payeeIDStr, accountIDStr, err := repo.ValidateRelationForBusiness(ctx, businessID.String(), relationID.String())
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("failed to resolve relation_id: %w", err)
	}

	// Parse UUIDs
	payeeID, err = uuid.Parse(payeeIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid payee_id: %w", err)
	}

	accountID, err = uuid.Parse(accountIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid account_id: %w", err)
	}

	return payeeID, accountID, nil
}

// maskAccountNumber masks account number for security (show last 4 digits only)
func maskAccountNumber(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return "****"
	}
	return "****" + accountNumber[len(accountNumber)-4:]
}

// GetVouchersByPayee retrieves all vouchers for a specific payee with business validation
func (s *VoucherServiceImpl) GetVouchersByPayee(ctx context.Context, req *GetVouchersByPayeeRequest) (*GetVouchersByPayeeResponse, error) {
	// 1. Validate inputs
	if req.BusinessID == uuid.Nil {
		return nil, fmt.Errorf("business_id is required")
	}
	if req.RelationID == uuid.Nil {
		return nil, fmt.Errorf("relation_id is required")
	}

	// Set default pagination if not provided
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 10
	}

	offset := (req.Page - 1) * req.PageSize

	// 2. Call repository (validates business ownership via INNER JOIN)
	vouchersWithPayment, totalCount, err := s.voucherRepo.GetVouchersByRelation(
		ctx,
		req.BusinessID.String(),
		req.RelationID.String(),
		req.PageSize,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get vouchers by payee: %w", err)
	}

	// 3. Transform to response DTOs
	var voucherItems []VoucherByPayeeItem
	var totalAmount int64
	var draftCount, completedCount int64

	for _, v := range vouchersWithPayment {
		// Build payment method string
		status := "Draft"
		paymentMethod := ""

		if v.PaymentID != nil {
			// Not a draft - has payment
			status = v.PaymentStatus
			if v.PaymentSourceType != "" {
				switch v.PaymentSourceType {
				case "card":
					paymentMethod = fmt.Sprintf("Card ****%s", v.PaymentSourceDigits)
				case "bank_account":
					paymentMethod = fmt.Sprintf("Bank *****%s", v.PaymentSourceDigits)
				case "upi":
					paymentMethod = "UPI"
				case "wallet":
					paymentMethod = "Wallet"
				default:
					paymentMethod = v.PaymentSourceType
				}
			}

			if status == "success" {
				completedCount++
			}
		} else {
			// Draft voucher
			draftCount++
			status = "Draft"
			paymentMethod = "N/A"
		}

		voucherItems = append(voucherItems, VoucherByPayeeItem{
			VoucherID:     v.VoucherID,
			InvoiceNumber: v.InvoiceNumber,
			NetAmount:     v.NetAmount,
			CreatedAt:     v.CreatedAt,
			Status:        status,
			PaymentMethod: paymentMethod,
			Description:   v.Description,
		})

		totalAmount += v.NetAmount
	}

	// 4. Build pagination
	totalPages := int(totalCount) / req.PageSize
	if int(totalCount)%req.PageSize != 0 {
		totalPages++
	}

	pagination := &voucher.PaginationResult{
		Page:       req.Page,
		Limit:      req.PageSize,
		Total:      totalCount,
		TotalPages: totalPages,
	}

	// 5. Build summary
	summary := &VouchersByPayeeSummary{
		TotalVouchers:  totalCount,
		TotalAmount:    totalAmount,
		DraftCount:     draftCount,
		CompletedCount: completedCount,
	}

	return &GetVouchersByPayeeResponse{
		Vouchers:   voucherItems,
		Pagination: pagination,
		Summary:    summary,
	}, nil
}
