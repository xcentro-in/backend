package voucher

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/voucher"
	"xcentro.in/internal/core/notification"
	"xcentro.in/internal/shared/logger"
	"xcentro.in/internal/shared/types"
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

	// DeleteVoucher deletes a voucher (only draft vouchers can be deleted)
	DeleteVoucher(ctx context.Context, voucherID uuid.UUID, businessID uuid.UUID) error

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

	// GetVoucherList retrieves paginated voucher/bill list with payment status (paid, open, overdue)
	GetVoucherList(ctx context.Context, businessID string, status *voucher.BillPaymentStatus, limit, offset int) (*voucher.VoucherListResponse, error)

	// RequestApproval creates or updates an approval request for a voucher
	RequestApproval(ctx context.Context, req *voucher.VoucherApprovalRequest) (*voucher.VoucherApprovalResponse, error)

	// GetApprovalInbox retrieves vouchers for approval inbox filtered by approval status
	GetApprovalInbox(ctx context.Context, req *voucher.VoucherApprovalInboxRequest) (*voucher.VoucherApprovalInboxResponse, error)

	// GetApprovalInboxCounts retrieves counts for each approval status
	GetApprovalInboxCounts(ctx context.Context, businessID string) (*voucher.VoucherApprovalInboxCounts, error)

	// BatchApproveVouchers approves multiple vouchers in a single operation
	BatchApproveVouchers(ctx context.Context, req *voucher.BatchApprovalRequest) (*voucher.BatchApprovalResponse, error)

	// GetUnpaidBills retrieves unpaid bills with role-based filtering
	// Owner: sees all unpaid bills, Other roles: sees only approved unpaid bills
	GetUnpaidBills(ctx context.Context, req *voucher.UnpaidBillsRequest, isOwner bool) (*voucher.UnpaidBillsResponse, error)

	// GetVoucherCounts retrieves counts of vouchers by payment status
	GetVoucherCounts(ctx context.Context, businessID string) (*voucher.VoucherCountsResponse, error)

	// GetVoucherActivity retrieves complete activity history for a voucher
	GetVoucherActivity(ctx context.Context, voucherID string) (*voucher.VoucherActivityResponse, error)

	// GetVoucherPaymentDetails retrieves complete payment details for a voucher
	GetVoucherPaymentDetails(ctx context.Context, voucherID string) (*VoucherPaymentDetailsResponse, error)

	// GetCalendarView retrieves bills for a specific month and year for calendar view
	GetCalendarView(ctx context.Context, req *voucher.CalendarViewRequest) (*voucher.CalendarViewResponse, error)
}

// ensureEmptyObject ensures that a map is never nil - returns empty map if nil
// This prevents frontend errors when accessing nested properties
func ensureEmptyObject(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

// VoucherServiceImpl implements the VoucherService interface
type VoucherServiceImpl struct {
	voucherRepo       voucher.VoucherRepository
	queueRepo         voucher.ERPSyncQueueRepository
	voucherCore       voucher.VoucherService // Domain service for business logic
	businessPayeeRepo interface{}            // payee.BusinessPayeeRelationRepository, using interface{} to avoid circular import
	notificationEventRepo notification.NotificationEventRepository
	logger                *logger.Logger
}

// NewVoucherService creates a new instance of VoucherService
func NewVoucherService(
	voucherRepo voucher.VoucherRepository,
	queueRepo voucher.ERPSyncQueueRepository,
	voucherCore voucher.VoucherService,
	businessPayeeRepo interface{}, // Repository to resolve relation_id
	notificationEventRepo notification.NotificationEventRepository,
	logger *logger.Logger,
) VoucherService {
	return &VoucherServiceImpl{
		voucherRepo:       voucherRepo,
		queueRepo:         queueRepo,
		voucherCore:       voucherCore,
		businessPayeeRepo: businessPayeeRepo,
		notificationEventRepo: notificationEventRepo,
		logger:                logger,
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
		VoucherID:             uuid.Must(uuid.NewV7()),
		VoucherNumber:         voucherNumber,
		VoucherDate:           now,
		FiscalYear:            fiscalYear,
		AccountingPeriod:      accountingPeriod,
		VoucherType:           voucher.VoucherTypePayment,
		BusinessID:            req.BusinessID,
		PayeeID:               payeeID,
		AccountID:             accountID,
		InvoiceNumber:         req.InvoiceNumber,
		InvoiceDate:           convertDateToTimePtr(req.InvoiceDate),
		InvoiceDueDate:        convertDateToTimePtr(req.InvoiceDueDate),
		GrossAmount:           req.GrossAmount,
		NetAmount:             req.NetAmount,
		Currency:              currency,
		GSTAmount:             req.GSTAmount,
		GSTRate:               req.GSTRate,
		CGSTAmount:            req.CGSTAmount,
		SGSTAmount:            req.SGSTAmount,
		IGSTAmount:            req.IGSTAmount,
		TDSAmount:             req.TDSAmount,
		TDSRate:               req.TDSRate,
		TDSSection:            req.TDSSection,
		FeeAmount:             req.FeeAmount,
		DiscountAmount:        req.DiscountAmount,
		FinancialDetails:      req.FinancialDetails,
		Attachments:           req.Attachments,
		ExpenseCategory:       req.ExpenseCategory,
		Description:           req.Description,
		Status:                voucher.StatusDraft,
		PaymentID:             nil, // NULL when draft
		ERPSyncStatus:         voucher.ERPNotSynced,
		ERPSyncAttempts:       0,
		GLExpenseAccount:      req.GLExpenseAccount,
		GLBankAccount:         req.GLBankAccount,
		CostCenter:            req.CostCenter,
		PaymentTerms:          req.PaymentTerms,
		CreatedAt:             now,
		CreatedByID:           &req.UserID,
		Metadata:              req.Metadata,
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
		VoucherID:     v.VoucherID,
		InvoiceNumber: v.InvoiceNumber,
		Status:        v.Status,
		Message:       "Draft voucher created successfully",
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

	// 2. Check if voucher has been paid - paid vouchers cannot be edited
	var paymentStatus *string
	if repo, ok := s.voucherRepo.(interface {
		GetPaymentStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
	}); ok {
		paymentStatus, err = repo.GetPaymentStatus(ctx, v.VoucherID)
		if err != nil {
			return nil, fmt.Errorf("failed to check payment status: %w", err)
		}
	}

	// If payment exists and is successful, voucher cannot be edited
	if paymentStatus != nil && *paymentStatus == "success" {
		return nil, fmt.Errorf("cannot edit voucher: payment has been completed successfully")
	}

	// 3. If voucher is finalized/posted/synced but not paid, shift it back to draft for editing
	statusChanged := false
	if v.Status != voucher.StatusDraft {
		v.Status = voucher.StatusDraft
		statusChanged = true
		// Note: Voucher status automatically shifted to draft for editing
	}

	// 4. Track updated fields
	var updatedFields []string
	if statusChanged {
		updatedFields = append(updatedFields, "status")
	}

	// 5. If relation_id is provided, resolve it and update payee/account
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
		v.InvoiceDate = req.InvoiceDate.ToTimePtr()
		updatedFields = append(updatedFields, "invoice_date")
	}
	if req.InvoiceDueDate != nil {
		v.InvoiceDueDate = req.InvoiceDueDate.ToTimePtr()
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

	// 13. Log edit action in approval history (audit trail)
	if req.EditorUserID != "" && len(updatedFields) > 0 {
		remarks := fmt.Sprintf("Voucher edited. Updated fields: %v", updatedFields)
		approvalReq := &voucher.VoucherApprovalRequest{
			VoucherID:       v.VoucherID.String(),
			BusinessID:      v.BusinessID.String(),
			RequesterUserID: req.EditorUserID,
			ApproverUserID:  req.EditorUserID, // Same user for edits
			Action:          voucher.ApprovalActionEdited,
			Remarks:         remarks,
		}

		// Create approval record (non-blocking - log error but don't fail the update)
		if _, err := s.voucherRepo.CreateOrUpdateApproval(ctx, approvalReq); err != nil {
			// Log the error but don't fail the voucher update
			fmt.Printf("Warning: failed to log edit action for voucher %s: %v\n", v.VoucherNumber, err)
		}
	}

	// 14. Return response with updated fields
	return &UpdateVoucherResponse{
		VoucherID:     v.VoucherID,
		VoucherNumber: v.VoucherNumber,
		Status:        v.Status,
		Message:       fmt.Sprintf("Voucher %s updated successfully", v.VoucherNumber),
		UpdatedFields: updatedFields,
	}, nil
}

// DeleteVoucher deletes a voucher (only draft vouchers can be deleted)
func (s *VoucherServiceImpl) DeleteVoucher(ctx context.Context, voucherID uuid.UUID, businessID uuid.UUID) error {
	// 1. Fetch existing voucher
	v, err := s.voucherRepo.GetByID(ctx, voucherID)
	if err != nil {
		return fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Verify voucher belongs to the business
	if v.BusinessID != businessID {
		return fmt.Errorf("voucher does not belong to the specified business")
	}

	// 3. Check if voucher has been paid - paid vouchers cannot be deleted
	var paymentStatus *string
	if repo, ok := s.voucherRepo.(interface {
		GetPaymentStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
	}); ok {
		paymentStatus, err = repo.GetPaymentStatus(ctx, voucherID)
		if err != nil {
			return fmt.Errorf("failed to check payment status: %w", err)
		}
	}

	// If payment exists and is successful, voucher cannot be deleted
	if paymentStatus != nil && *paymentStatus == "success" {
		return fmt.Errorf("cannot delete voucher: payment has been completed successfully")
	}

	// 4. Delete voucher (repository handles cascade and payout transfer checks)
	if err := s.voucherRepo.Delete(ctx, voucherID); err != nil {
		return fmt.Errorf("failed to delete voucher: %w", err)
	}

	return nil
}

// GetVoucherDetails retrieves enriched voucher details
func (s *VoucherServiceImpl) GetVoucherDetails(ctx context.Context, voucherID uuid.UUID) (*VoucherDetailsResponse, error) {
	// 1. Get voucher
	v, err := s.voucherRepo.GetByID(ctx, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher: %w", err)
	}

	// 2. Get payee details from payee_id + account_id
	var payeeDetails *PayeeDetails
	if repo, ok := s.voucherRepo.(interface {
		GetPayeeDetailsByIDs(ctx context.Context, payeeID, accountID uuid.UUID) (map[string]interface{}, error)
	}); ok {
		payeeData, err := repo.GetPayeeDetailsByIDs(ctx, v.PayeeID, v.AccountID)
		if err != nil {
			// Log error but continue - payee details are optional
			fmt.Printf("Warning: failed to get payee details: %v\n", err)
		} else {
			// Convert map to PayeeDetails struct
			payeeDetails = &PayeeDetails{
				RelationID:        payeeData["relation_id"].(uuid.UUID),
				PayeeID:           payeeData["payee_id"].(uuid.UUID),
				Name:              payeeData["name"].(string),
				GSTIN:             payeeData["gstin"].(string),
				Type:              payeeData["type"].(string),
				BankAccountMasked: payeeData["bank_account_masked"].(string),
				AccountID:         payeeData["account_id"].(uuid.UUID),
				IFSCCode:          payeeData["ifsc_code"].(string),
				BankName:          payeeData["bank_name"].(string),
				AccountType:       payeeData["account_type"].(string),
			}
		}
	}

	// 4. Get latest approval status
	var approvalStatus *string
	if repo, ok := s.voucherRepo.(interface {
		GetLatestApprovalStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
	}); ok {
		approvalStatus, err = repo.GetLatestApprovalStatus(ctx, v.VoucherID)
		if err != nil {
			// Log error but continue - approval status is optional
			fmt.Printf("Warning: failed to get approval status: %v\n", err)
		}
	}

	// 5. Get payment status from payments table
	var paymentStatus *string
	if repo, ok := s.voucherRepo.(interface {
		GetPaymentStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
	}); ok {
		paymentStatus, err = repo.GetPaymentStatus(ctx, v.VoucherID)
		if err != nil {
			// Log error but continue - payment status is optional
			fmt.Printf("Warning: failed to get payment status: %v\n", err)
		}
	}

	// 6. Build response with payee enrichment
	response := &VoucherDetailsResponse{
		VoucherID:        v.VoucherID,
		VoucherNumber:    v.VoucherNumber,
		VoucherDate:      v.VoucherDate,
		FiscalYear:       v.FiscalYear,
		AccountingPeriod: v.AccountingPeriod,
		VoucherType:      v.VoucherType,
		Status:           v.Status,
		Payee:            payeeDetails, // Nested payee object
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
		FinancialDetails: ensureEmptyObject(v.FinancialDetails),
		Attachments:      ensureEmptyObject(v.Attachments),
		ExpenseCategory:  v.ExpenseCategory,
		Description:      v.Description,
		ApprovalStatus:   approvalStatus,
		PaymentStatus:    paymentStatus,
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

	// 2. Convert to list items with enriched status information
	var items []VoucherListItem
	for _, v := range vouchers {
		// TODO: Enrich with payee name
		// payee, _ := s.payeeRepo.GetByID(ctx, v.PayeeID)
		payeeName := "Unknown" // Placeholder

		// Get latest approval status for this voucher
		var approvalStatus *string
		if repo, ok := s.voucherRepo.(interface {
			GetLatestApprovalStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
		}); ok {
			approvalStatus, _ = repo.GetLatestApprovalStatus(ctx, v.VoucherID)
			// Ignoring error - approval status is optional
		}

		// Get payment status from payments table
		var paymentStatus *string
		if repo, ok := s.voucherRepo.(interface {
			GetPaymentStatus(ctx context.Context, voucherID uuid.UUID) (*string, error)
		}); ok {
			paymentStatus, _ = repo.GetPaymentStatus(ctx, v.VoucherID)
			// Ignoring error - payment status is optional
		}

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
			ApprovalStatus:  approvalStatus,
			PaymentStatus:   paymentStatus,
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
// NOTE: Cancellation functionality removed in payout redesign
// Voucher cancellation/refund workflow has been deprecated
func (s *VoucherServiceImpl) RequestCancellation(ctx context.Context, req *RequestCancellationRequest) (*RequestCancellationResponse, error) {
	return nil, fmt.Errorf("voucher cancellation functionality has been removed - cancellation/refund fields deleted from schema")
}

// ApproveCancellation approves or rejects a cancellation request (Phase 7)
// NOTE: Cancellation functionality removed in payout redesign
// Voucher cancellation/refund workflow has been deprecated
func (s *VoucherServiceImpl) ApproveCancellation(ctx context.Context, req *ApproveCancellationRequest) (*ApproveCancellationResponse, error) {
	return nil, fmt.Errorf("voucher cancellation functionality has been removed - cancellation/refund fields deleted from schema")
}

// Helper functions

// convertDateToTimePtr converts custom Date type to *time.Time
func convertDateToTimePtr(d *types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return d.ToTimePtr()
}

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

// GetVoucherList retrieves paginated voucher/bill list for a business
func (s *VoucherServiceImpl) GetVoucherList(ctx context.Context, businessID string, status *voucher.BillPaymentStatus, limit, offset int) (*voucher.VoucherListResponse, error) {
	// Validate business ID
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Set default pagination if not provided
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100 // Max 100 items per page
	}
	if offset < 0 {
		offset = 0
	}

	// Fetch voucher list from repository
	items, totalCount, err := s.voucherRepo.GetVoucherListForBusiness(ctx, businessID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher list: %w", err)
	}

	return &voucher.VoucherListResponse{
		Vouchers:   items,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

// RequestApproval creates or updates an approval request for a voucher
func (s *VoucherServiceImpl) RequestApproval(ctx context.Context, req *voucher.VoucherApprovalRequest) (*voucher.VoucherApprovalResponse, error) {
	// Validate voucher exists
	voucherUUID, err := uuid.Parse(req.VoucherID)
	if err != nil {
		return nil, fmt.Errorf("invalid voucher_id: %w", err)
	}

	// Use optimized validation instead of fetching full voucher
	status, err := s.voucherRepo.ValidateVoucherForApproval(ctx, voucherUUID, req.BusinessID)
	if err != nil {
		return nil, fmt.Errorf("voucher not found: %w", err)
	}

	// Only draft or finalized vouchers can have approval requests
	if status != voucher.StatusDraft && status != voucher.StatusFinalized {
		return nil, fmt.Errorf("cannot request approval for voucher in status: %s", status)
	}

	// Call repository to create/update approval
	response, err := s.voucherRepo.CreateOrUpdateApproval(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to process approval request: %w", err)
	}

	// Trigger notification based on action
	switch req.Action {
	case voucher.ApprovalActionPending:
		// When requesting approval, notify the owner
		if err := s.triggerApprovalRequestNotification(ctx, req); err != nil {
			// Log error but don't fail the approval request
			s.logger.WarnWith("Failed to trigger approval request notification",
				"voucher_id", req.VoucherID,
				"business_id", req.BusinessID,
				"error", err.Error(),
			)
		}
	case voucher.ApprovalActionApproved, voucher.ApprovalActionRejected, voucher.ApprovalActionRequestChanges:
		// When approved/rejected/request_changes, notify the requester
		// Use requester_user_id from response (avoids extra DB call)
		if err := s.triggerApprovalResponseNotification(ctx, req, response.PreviousApproverID); err != nil {
			// Log error but don't fail the approval action
			s.logger.WarnWith("Failed to trigger approval response notification",
				"voucher_id", req.VoucherID,
				"business_id", req.BusinessID,
				"action", string(req.Action),
				"error", err.Error(),
			)
		}
	}

	return response, nil
}

// GetApprovalInbox retrieves vouchers for approval inbox filtered by approval status
func (s *VoucherServiceImpl) GetApprovalInbox(ctx context.Context, req *voucher.VoucherApprovalInboxRequest) (*voucher.VoucherApprovalInboxResponse, error) {
	// Validate business ID
	if req.BusinessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Validate status
	if req.Status == "" {
		return nil, fmt.Errorf("status query parameter is required")
	}

	// Validate status is a valid approval action
	validStatuses := map[voucher.ApprovalAction]bool{
		voucher.ApprovalActionPending:        true,
		voucher.ApprovalActionApproved:       true,
		voucher.ApprovalActionRejected:       true,
		voucher.ApprovalActionRequestChanges: true,
	}
	if !validStatuses[req.Status] {
		return nil, fmt.Errorf("invalid status: must be one of pending, approved, rejected, request_changes")
	}

	// Call repository to get filtered vouchers with complete approval history
	vouchers, err := s.voucherRepo.GetApprovalInboxVouchers(ctx, req.BusinessID, req.Status)
	if err != nil {
		return nil, fmt.Errorf("failed to get approval inbox: %w", err)
	}

	return &voucher.VoucherApprovalInboxResponse{
		Vouchers:   vouchers,
		TotalCount: len(vouchers),
	}, nil
}

// GetApprovalInboxCounts retrieves counts for each approval status
func (s *VoucherServiceImpl) GetApprovalInboxCounts(ctx context.Context, businessID string) (*voucher.VoucherApprovalInboxCounts, error) {
	// Validate business ID
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Call repository to get counts
	counts, err := s.voucherRepo.GetApprovalInboxCounts(ctx, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get approval inbox counts: %w", err)
	}

	return counts, nil
}

// BatchApproveVouchers approves multiple vouchers in a single operation
func (s *VoucherServiceImpl) BatchApproveVouchers(ctx context.Context, req *voucher.BatchApprovalRequest) (*voucher.BatchApprovalResponse, error) {
	// Validate business ID
	if req.BusinessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Validate approved_by_id
	if req.ApprovedByID == "" {
		return nil, fmt.Errorf("approved_by_id is required")
	}

	// Validate voucher IDs
	if len(req.VoucherIDs) == 0 {
		return nil, fmt.Errorf("at least one voucher_id is required")
	}

	// Call repository to perform batch approval
	response, err := s.voucherRepo.BatchApproveVouchers(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to batch approve vouchers: %w", err)
	}

	return response, nil
}

// GetUnpaidBills retrieves unpaid bills with role-based filtering
func (s *VoucherServiceImpl) GetUnpaidBills(ctx context.Context, req *voucher.UnpaidBillsRequest, isOwner bool) (*voucher.UnpaidBillsResponse, error) {
	// Validate inputs
	if req.BusinessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Get unpaid bills with role-based filtering
	bills, totalCount, err := s.voucherRepo.GetUnpaidBills(ctx, req, isOwner)
	if err != nil {
		return nil, fmt.Errorf("failed to get unpaid bills: %w", err)
	}

	return &voucher.UnpaidBillsResponse{
		Bills:      bills,
		TotalCount: totalCount,
		Limit:      req.Limit,
		Offset:     req.Offset,
	}, nil
}

// GetVoucherCounts retrieves counts of vouchers by payment status
func (s *VoucherServiceImpl) GetVoucherCounts(ctx context.Context, businessID string) (*voucher.VoucherCountsResponse, error) {
	// Validate business ID
	if businessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}

	// Get counts from repository
	counts, err := s.voucherRepo.GetVoucherCounts(ctx, businessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher counts: %w", err)
	}

	return counts, nil
}

// GetVoucherActivity retrieves complete activity history for a voucher
func (s *VoucherServiceImpl) GetVoucherActivity(ctx context.Context, voucherID string) (*voucher.VoucherActivityResponse, error) {
	// Validate voucher ID
	if voucherID == "" {
		return nil, fmt.Errorf("voucher_id is required")
	}

	// Validate UUID format
	if _, err := uuid.Parse(voucherID); err != nil {
		return nil, fmt.Errorf("invalid voucher_id format: %w", err)
	}

	// Get activity from repository
	activity, err := s.voucherRepo.GetVoucherActivity(ctx, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get voucher activity: %w", err)
	}

	return activity, nil
}

// GetVoucherPaymentDetails retrieves complete payment details for a voucher
func (s *VoucherServiceImpl) GetVoucherPaymentDetails(ctx context.Context, voucherID string) (*VoucherPaymentDetailsResponse, error) {
	// Validate voucher ID
	if voucherID == "" {
		return nil, fmt.Errorf("voucher_id is required")
	}

	// Validate UUID format
	if _, err := uuid.Parse(voucherID); err != nil {
		return nil, fmt.Errorf("invalid voucher_id format: %w", err)
	}

	// Get payment details from repository
	details, err := s.voucherRepo.GetPaymentDetailsByVoucher(ctx, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment details: %w", err)
	}

	// Use stored fee breakdown (no reverse calculation needed)
	convenienceFee := details.ConvenienceFee
	gstOnFee := details.ConvenienceFeeGST

	// Format convenience fee rate
	convenienceFeeRate := ""
	if details.PaymentMethod == "card" {
		// For cards, it's usually percentage-based
		if details.Amount > 0 {
			rate := (float64(convenienceFee) / float64(details.Amount)) * 100
			convenienceFeeRate = fmt.Sprintf("%.2f%%", rate)
		}
	} else {
		// For net banking/UPI, it's usually fixed
		convenienceFeeRate = fmt.Sprintf("₹%.2f (fixed)", float64(convenienceFee)/100.0)
	}

	response := &VoucherPaymentDetailsResponse{
		PaymentID:     details.PaymentID,
		VoucherID:     details.VoucherID,
		VoucherNumber: details.VoucherNumber,
		PaymentStatus: details.PaymentStatus,
		PaymentDate:   details.PaymentDate,

		PaymentMethod:  details.PaymentMethod,
		CardType:       details.CardType,
		CardCategory:   details.CardCategory,
		LastFourDigits: details.LastFourDigits,
		BankName:       details.BankName,

		VoucherAmount:      details.Amount,
		ConvenienceFee:     convenienceFee,
		ConvenienceFeeRate: convenienceFeeRate,
		GSTOnFee:           gstOnFee,
		GSTRate:            "18.00%",
		TotalFeeAmount:     details.FeeAmount,
		TotalAmountPaid:    details.TotalAmount,

		PayoutUTR:         details.PayoutUTR,
		PayoutCompletedAt: details.PayoutCompletedAt,

		Description: details.Description,
	}

	return response, nil
}

// GetCalendarView retrieves bills for a specific month and year based on invoice_due_date
// Used for calendar view to show bills month by month with high-level details
func (s *VoucherServiceImpl) GetCalendarView(ctx context.Context, req *voucher.CalendarViewRequest) (*voucher.CalendarViewResponse, error) {
	// Validate input
	if req.BusinessID == "" {
		return nil, fmt.Errorf("business_id is required")
	}
	if req.Year < 2000 || req.Year > 2100 {
		return nil, fmt.Errorf("invalid year: must be between 2000 and 2100")
	}
	if req.Month < 1 || req.Month > 12 {
		return nil, fmt.Errorf("invalid month: must be between 1 and 12")
	}

	// Fetch bills from repository
	bills, err := s.voucherRepo.GetCalendarViewBills(ctx, req.BusinessID, req.Year, req.Month)
	if err != nil {
		return nil, fmt.Errorf("failed to get calendar view bills: %w", err)
	}

	// Build response
	response := &voucher.CalendarViewResponse{
		Year:  req.Year,
		Month: req.Month,
		Bills: bills,
		Count: len(bills),
	}

	return response, nil
}

// triggerApprovalRequestNotification sends notification to business owner when approval is requested
func (s *VoucherServiceImpl) triggerApprovalRequestNotification(ctx context.Context, req *voucher.VoucherApprovalRequest) error {
	// Get voucher details to build meaningful notification message
	voucherUUID, err := uuid.Parse(req.VoucherID)
	if err != nil {
		return fmt.Errorf("invalid voucher_id: %w", err)
	}

	v, err := s.voucherRepo.GetByID(ctx, voucherUUID)
	if err != nil {
		return fmt.Errorf("failed to get voucher details: %w", err)
	}

	// Format amount for display (convert paise to rupees)
	amountInRupees := float64(v.NetAmount) / 100.0

	// Use multicast targeting with MinimumRole=Owner (more efficient than DB lookup)
	ownerRole := notification.RoleOwner

	// Build notification payload
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: notification.EventVoucherApprovalRequested,
		Target: notification.TargetInfo{
			BusinessID:  req.BusinessID,
			Type:        notification.TargetMulticast,
			MinimumRole: &ownerRole, // Multicast to all users with Owner role
		},
		Entities: notification.EntityReferences{
			BusinessID: &req.BusinessID,
			VoucherID:  &req.VoucherID,
		},
		Message: notification.MessageContent{
			Title: "Voucher Approval Request",
			Body:  fmt.Sprintf("Voucher %s for ₹%.2f requires your approval. Invoice: %s", v.VoucherNumber, amountInRupees, v.InvoiceNumber),
		},
		Actions: []notification.ActionButton{
			{
				Type: notification.ButtonTypePositive,
				Text: "Go to Approvals",
				Href: "/approvals",
			},
		},
	}

	// Create notification event
	event := &notification.NotificationEvent{
		EventType: notification.EventVoucherApprovalRequested,
		Payload:   payload,
	}

	// Create event (non-blocking - just inserts into notification_events table)
	if err := s.notificationEventRepo.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to create notification event: %w", err)
	}

	s.logger.InfoWith("Voucher approval request notification sent",
		"voucher_id", req.VoucherID,
		"voucher_number", v.VoucherNumber,
		"business_id", req.BusinessID,
		"target_role", "owner",
		"amount", amountInRupees,
	)

	return nil
}

// triggerApprovalResponseNotification sends notification to requester when approval is responded to
func (s *VoucherServiceImpl) triggerApprovalResponseNotification(ctx context.Context, req *voucher.VoucherApprovalRequest, requesterUserID string) error {
	// Get voucher details to build meaningful notification message
	voucherUUID, err := uuid.Parse(req.VoucherID)
	if err != nil {
		return fmt.Errorf("invalid voucher_id: %w", err)
	}

	v, err := s.voucherRepo.GetByID(ctx, voucherUUID)
	if err != nil {
		return fmt.Errorf("failed to get voucher details: %w", err)
	}

	// Format amount for display (convert paise to rupees)
	amountInRupees := float64(v.NetAmount) / 100.0

	// Determine event type and message based on action
	var eventType string
	var title string
	var body string

	switch req.Action {
	case voucher.ApprovalActionApproved:
		eventType = notification.EventVoucherApprovalApproved
		title = "Voucher Approved"
		body = fmt.Sprintf("Your voucher %s for ₹%.2f has been approved. Invoice: %s", v.VoucherNumber, amountInRupees, v.InvoiceNumber)
	case voucher.ApprovalActionRejected:
		eventType = notification.EventVoucherApprovalRejected
		title = "Voucher Rejected"
		body = fmt.Sprintf("Your voucher %s for ₹%.2f has been rejected. Invoice: %s", v.VoucherNumber, amountInRupees, v.InvoiceNumber)
		if req.Remarks != "" {
			body += fmt.Sprintf(". Reason: %s", req.Remarks)
		}
	case voucher.ApprovalActionRequestChanges:
		eventType = notification.EventVoucherApprovalRejected // Using rejected event for request_changes
		title = "Changes Requested for Voucher"
		body = fmt.Sprintf("Changes requested for your voucher %s for ₹%.2f. Invoice: %s", v.VoucherNumber, amountInRupees, v.InvoiceNumber)
		if req.Remarks != "" {
			body += fmt.Sprintf(". Details: %s", req.Remarks)
		}
	default:
		return fmt.Errorf("unsupported action: %s", req.Action)
	}

	// Build notification payload
	payload := notification.NotificationEventPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		EventType: eventType,
		Target: notification.TargetInfo{
			BusinessID: req.BusinessID,
			Type:       notification.TargetUnicast,
			IDs:        []string{requesterUserID}, // Send to the original requester
		},
		Entities: notification.EntityReferences{
			BusinessID: &req.BusinessID,
			VoucherID:  &req.VoucherID,
		},
		Message: notification.MessageContent{
			Title: title,
			Body:  body,
		},
		Actions: []notification.ActionButton{
			{
				Type: notification.ButtonTypeNeutral,
				Text: "View Details",
				Href: fmt.Sprintf("/bills/%s", req.VoucherID),
			},
		},
	}

	// Create notification event
	event := &notification.NotificationEvent{
		EventType: eventType,
		Payload:   payload,
	}

	// Create event (non-blocking - just inserts into notification_events table)
	if err := s.notificationEventRepo.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to create notification event: %w", err)
	}

	s.logger.InfoWith("Voucher approval response notification sent",
		"voucher_id", req.VoucherID,
		"voucher_number", v.VoucherNumber,
		"business_id", req.BusinessID,
		"requester_user_id", requesterUserID,
		"action", string(req.Action),
		"amount", amountInRupees,
	)

	return nil
}
