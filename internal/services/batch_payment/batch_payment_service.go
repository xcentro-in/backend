package batch_payment

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"xcentro.in/internal/core/domain"
	"xcentro.in/internal/core/payee"
	"xcentro.in/internal/core/repositories"
	voucherService "xcentro.in/internal/services/voucher"
	"xcentro.in/internal/shared/utils"
)

const (
	MinMatchConfidence = 0.75 // 75% similarity threshold for auto-match
	MaxBatchSize      = 500   // Maximum rows per batch
)

// BatchPaymentService handles batch payment operations
type BatchPaymentService struct {
	batchRepo                 repositories.BatchPaymentRepository
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository
	voucherService            voucherService.VoucherService
}

// NewBatchPaymentService creates a new batch payment service
func NewBatchPaymentService(
	batchRepo repositories.BatchPaymentRepository,
	businessPayeeRelationRepo payee.BusinessPayeeRelationRepository,
	voucherService voucherService.VoucherService,
) *BatchPaymentService {
	return &BatchPaymentService{
		batchRepo:                 batchRepo,
		businessPayeeRelationRepo: businessPayeeRelationRepo,
		voucherService:            voucherService,
	}
}

// ParseAndPreviewCSV parses CSV and generates preview with matching results
func (s *BatchPaymentService) ParseAndPreviewCSV(
	ctx context.Context,
	businessID uuid.UUID,
	userID uuid.UUID,
	fileName string,
	csvReader io.Reader,
) (*PreviewResponse, error) {
	// Parse CSV
	rows, err := s.parseCSV(csvReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	if len(rows) > MaxBatchSize {
		return nil, fmt.Errorf("batch size exceeds maximum of %d rows", MaxBatchSize)
	}

	// Get all business payee relations for matching
	relations, err := s.businessPayeeRelationRepo.FindByBusiness(ctx, businessID.String(), payee.RelationSearchFilters{})
	if err != nil {
		return nil, fmt.Errorf("failed to get payee relations: %w", err)
	}

	// Create batch record
	batchID := uuid.New()
	batch := &domain.BatchPayment{
		BatchID:       batchID,
		BusinessID:    businessID,
		UploadedBy:    userID,
		FileName:      fileName,
		Status:        domain.BatchStatusPending,
		TotalRows:     len(rows),
		TotalAmount:   0,
		ProcessedRows: 0,
		SuccessRows:   0,
		FailedRows:    0,
		CreatedAt:     time.Now(),
	}

	// Match rows with payees
	var matched []MatchedItem
	var ambiguous []AmbiguousItem
	var notFound []NotFoundItem
	var batchItems []*domain.BatchPaymentItem
	var totalAmount int64

	for i, row := range rows {
		rowNumber := i + 1
		amountPaise := int64(row.Amount * 100) // Convert rupees to paise
		totalAmount += amountPaise

		// Perform fuzzy matching
		matches := s.fuzzyMatchPayee(ctx, businessID, row, relations)

		item := &domain.BatchPaymentItem{
			ItemID:          uuid.New(),
			BatchID:         batchID,
			RowNumber:       rowNumber,
			CSVPayeeName:    row.PayeeName,
			CSVAccountLast4: row.AccountLast4,
			CSVAmount:       amountPaise,
			PaymentContext:  row.PaymentContext,
			Description:     row.Description,
			ExpenseCategory: row.ExpenseCategory,
			GSTAmount:       int64(row.GSTAmount * 100),
			TDSAmount:       int64(row.TDSAmount * 100),
			InvoiceNumber:   row.InvoiceNumber,
			CreatedAt:       time.Now(),
		}

		if len(matches) == 0 {
			// No match found
			item.Status = domain.ItemStatusNotFound
			notFound = append(notFound, NotFoundItem{
				RowNumber:       rowNumber,
				CSVPayeeName:    row.PayeeName,
				CSVAccountLast4: row.AccountLast4,
				Amount:          amountPaise,
				PaymentContext:  row.PaymentContext,
				Description:     row.Description,
				InvoiceNumber:   row.InvoiceNumber,
				Reason:          "No matching payee found in system",
			})
		} else if len(matches) == 1 && matches[0].MatchConfidence >= MinMatchConfidence {
			// Single high-confidence match
			item.Status = domain.ItemStatusMatched
			item.MatchedRelationID = &matches[0].RelationID
			item.MatchConfidence = matches[0].MatchConfidence

			matched = append(matched, MatchedItem{
				RowNumber:         rowNumber,
				CSVPayeeName:      row.PayeeName,
				CSVAccountLast4:   row.AccountLast4,
				Amount:            amountPaise,
				PaymentContext:    row.PaymentContext,
				Description:       row.Description,
				InvoiceNumber:     row.InvoiceNumber,
				MatchedRelationID: matches[0].RelationID,
				MatchedPayeeName:  matches[0].PayeeName,
				MatchedAccount:    matches[0].AccountLast4,
				MatchedBankName:   matches[0].BankName,
				MatchConfidence:   matches[0].MatchConfidence,
			})
		} else {
			// Multiple matches or low confidence - needs user review
			item.Status = domain.ItemStatusAmbiguous

			possibleMatches := make([]PossibleMatch, 0, len(matches))
			for _, match := range matches {
				possibleMatches = append(possibleMatches, PossibleMatch{
					RelationID:      match.RelationID,
					PayeeName:       match.PayeeName,
					AccountLast4:    match.AccountLast4,
					BankName:        match.BankName,
					MatchConfidence: match.MatchConfidence,
				})
			}

			ambiguous = append(ambiguous, AmbiguousItem{
				RowNumber:       rowNumber,
				CSVPayeeName:    row.PayeeName,
				CSVAccountLast4: row.AccountLast4,
				Amount:          amountPaise,
				PaymentContext:  row.PaymentContext,
				Description:     row.Description,
				InvoiceNumber:   row.InvoiceNumber,
				PossibleMatches: possibleMatches,
			})
		}

		batchItems = append(batchItems, item)
	}

	// Update batch total amount
	batch.TotalAmount = totalAmount

	// Save batch and items to database
	if err := s.batchRepo.CreateBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to create batch: %w", err)
	}

	if err := s.batchRepo.CreateBatchItems(ctx, batchItems); err != nil {
		return nil, fmt.Errorf("failed to create batch items: %w", err)
	}

	// Prepare response
	response := &PreviewResponse{
		BatchID:        batchID,
		TotalRows:      len(rows),
		MatchedCount:   len(matched),
		AmbiguousCount: len(ambiguous),
		NotFoundCount:  len(notFound),
		Matched:        matched,
		Ambiguous:      ambiguous,
		NotFound:       notFound,
		TotalAmount:    totalAmount,
		FileName:       fileName,
		UploadedAt:     batch.CreatedAt,
	}

	return response, nil
}

// parseCSV parses CSV content and returns rows
func (s *BatchPaymentService) parseCSV(reader io.Reader) ([]CSVRow, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Validate and map headers
	headerMap := make(map[string]int)
	for i, h := range header {
		headerMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Required fields
	requiredFields := []string{"payee_name", "amount", "payment_context", "description"}
	for _, field := range requiredFields {
		if _, exists := headerMap[field]; !exists {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	// Read data rows
	var rows []CSVRow
	lineNum := 1 // Start at 1 (header is line 0)

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV line %d: %w", lineNum+1, err)
		}

		lineNum++

		// Skip empty rows or rows with all empty fields
		isEmpty := true
		for _, field := range record {
			if strings.TrimSpace(field) != "" {
				isEmpty = false
				break
			}
		}
		if isEmpty {
			continue
		}

		// Validate row length and UTF-8 encoding
		if err := utils.SanitizeAndValidateRow(record, lineNum); err != nil {
			return nil, err
		}

		// Parse row
		row := CSVRow{}

		// Required fields - sanitize and validate
		payeeName, err := utils.ValidatePayeeName(record[headerMap["payee_name"]], lineNum)
		if err != nil {
			return nil, err
		}
		row.PayeeName = payeeName

		paymentContext, err := utils.ValidatePaymentContext(record[headerMap["payment_context"]], lineNum)
		if err != nil {
			return nil, err
		}
		row.PaymentContext = paymentContext

		description, err := utils.ValidateDescription(record[headerMap["description"]], lineNum)
		if err != nil {
			return nil, err
		}
		row.Description = description

		// Parse amount - validate first
		amountStr := strings.TrimSpace(record[headerMap["amount"]])
		if err := utils.ValidateAmount(amountStr, lineNum); err != nil {
			return nil, err
		}
		// Remove commas before parsing (e.g., "1,000.00" -> "1000.00")
		amountStr = strings.ReplaceAll(amountStr, ",", "")
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid amount on line %d: %s", lineNum, amountStr)
		}
		row.Amount = amount

		// Optional fields - sanitize and validate
		if idx, exists := headerMap["account_last_4"]; exists && idx < len(record) {
			accountLast4, err := utils.ValidateAccountLast4(record[idx], lineNum)
			if err != nil {
				return nil, err
			}
			row.AccountLast4 = accountLast4
		}
		if idx, exists := headerMap["ifsc_code"]; exists && idx < len(record) {
			ifscCode, err := utils.ValidateIFSCCode(record[idx], lineNum)
			if err != nil {
				return nil, err
			}
			row.IFSCCode = ifscCode
		}
		if idx, exists := headerMap["expense_category"]; exists && idx < len(record) {
			expenseCategory, err := utils.ValidateExpenseCategory(record[idx], lineNum)
			if err != nil {
				return nil, err
			}
			row.ExpenseCategory = expenseCategory
		}
		if idx, exists := headerMap["gst_amount"]; exists && idx < len(record) {
			gstStr := strings.TrimSpace(record[idx])
			if gstStr != "" {
				// Validate GST amount format
				if err := utils.ValidateAmount(gstStr, lineNum); err != nil {
					return nil, fmt.Errorf("invalid GST amount on line %d: %s", lineNum, gstStr)
				}
				gstStr = strings.ReplaceAll(gstStr, ",", "")
				gst, _ := strconv.ParseFloat(gstStr, 64)
				row.GSTAmount = gst
			}
		}
		if idx, exists := headerMap["tds_amount"]; exists && idx < len(record) {
			tdsStr := strings.TrimSpace(record[idx])
			if tdsStr != "" {
				// Validate TDS amount format
				if err := utils.ValidateAmount(tdsStr, lineNum); err != nil {
					return nil, fmt.Errorf("invalid TDS amount on line %d: %s", lineNum, tdsStr)
				}
				tdsStr = strings.ReplaceAll(tdsStr, ",", "")
				tds, _ := strconv.ParseFloat(tdsStr, 64)
				row.TDSAmount = tds
			}
		}
		if idx, exists := headerMap["invoice_number"]; exists && idx < len(record) {
			invoiceNumber, err := utils.ValidateInvoiceNumber(record[idx], lineNum)
			if err != nil {
				return nil, err
			}
			row.InvoiceNumber = invoiceNumber
		}

		// Additional business logic validation
		if row.Amount <= 0 {
			return nil, fmt.Errorf("amount must be positive on line %d", lineNum)
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// fuzzyMatchPayee performs fuzzy matching on payee name, account number, and IFSC
// CSV provides: payee_name, account_last_4, ifsc_code
// DB has: display_name, full account details with IFSC
// Returns matches sorted by confidence (highest first)
func (s *BatchPaymentService) fuzzyMatchPayee(ctx context.Context, businessID uuid.UUID, row CSVRow, relations []*payee.BusinessPayeeRelation) []PossibleMatch {
	var matches []PossibleMatch

	for _, relation := range relations {
		// Calculate name similarity (primary match factor)
		nameSimilarity := utils.FuzzyMatch(row.PayeeName, relation.DisplayName)

		// Skip if name similarity is too low (< 50%)
		if nameSimilarity < 0.5 {
			continue
		}

		// Parse relation_id to UUID
		relationUUID, err := uuid.Parse(relation.RelationID)
		if err != nil {
			continue // Skip invalid UUIDs
		}

		// Get full payee details including bank account
		fullDetails, err := s.businessPayeeRelationRepo.GetPayeeFullDetailsByRelation(ctx, businessID.String(), relation.RelationID)
		if err != nil || fullDetails == nil || fullDetails.BankDetails == nil {
			// No bank details available, use name match only
			matches = append(matches, PossibleMatch{
				RelationID:      relationUUID,
				PayeeName:       relation.DisplayName,
				AccountLast4:    "N/A",
				BankName:        "N/A",
				MatchConfidence: nameSimilarity,
			})
			continue
		}

		// Extract bank details from DB
		accountNumber := fullDetails.BankDetails.AccountNumber
		accountLast4 := ""
		if len(accountNumber) >= 4 {
			accountLast4 = accountNumber[len(accountNumber)-4:]
		}
		bankName := fullDetails.BankDetails.BankName
		ifscCode := fullDetails.BankDetails.IFSCCode

		// Calculate composite match confidence using CSV inputs vs DB data
		finalConfidence := s.calculateMatchConfidence(row, nameSimilarity, accountLast4, ifscCode)

		matches = append(matches, PossibleMatch{
			RelationID:      relationUUID,
			PayeeName:       relation.DisplayName,
			AccountLast4:    accountLast4,
			BankName:        bankName,
			MatchConfidence: finalConfidence,
		})
	}

	// Sort by confidence (highest first)
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].MatchConfidence > matches[i].MatchConfidence {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Return top 5 matches
	if len(matches) > 5 {
		matches = matches[:5]
	}

	return matches
}

// calculateMatchConfidence calculates weighted confidence score
// Compares CSV inputs against DB data:
// - Name: 60% weight (fuzzy match)
// - Account Last 4: 30% weight (exact match)
// - IFSC Code: 10% weight (exact match)
func (s *BatchPaymentService) calculateMatchConfidence(csvRow CSVRow, nameSimilarity float64, dbAccountLast4, dbIFSCCode string) float64 {
	confidence := nameSimilarity * 0.6 // Name contributes 60%

	// Account last 4 match (30% weight)
	// Compare CSV account_last_4 vs DB account last 4 digits
	if csvRow.AccountLast4 != "" && dbAccountLast4 != "" {
		if csvRow.AccountLast4 == dbAccountLast4 {
			confidence += 0.3 // Exact match adds 30%
		}
	}

	// IFSC code match (10% weight)
	// Compare CSV ifsc_code vs DB IFSC
	if csvRow.IFSCCode != "" && dbIFSCCode != "" {
		if strings.EqualFold(csvRow.IFSCCode, dbIFSCCode) { // Case-insensitive
			confidence += 0.1 // Exact match adds 10%
		}
	}

	return confidence
}

// ProcessBatch processes a batch by creating draft vouchers for matched items
func (s *BatchPaymentService) ProcessBatch(
	ctx context.Context,
	businessID uuid.UUID,
	userID uuid.UUID,
	req *ProcessRequest,
) (*ProcessResponse, error) {
	// Get batch
	batch, err := s.batchRepo.GetBatchByID(ctx, req.BatchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	if batch.Status != domain.BatchStatusPending {
		return nil, fmt.Errorf("batch is not in pending status")
	}

	// Get all items first
	allItems, err := s.batchRepo.GetBatchItems(ctx, req.BatchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch items: %w", err)
	}

	// Update ambiguous mappings
	for rowNum, relationID := range req.AmbiguousMappings {
		for _, item := range allItems {
			if item.RowNumber == rowNum && item.Status == domain.ItemStatusAmbiguous {
				if err := s.batchRepo.UpdateItemMatching(ctx, item.ItemID, relationID); err != nil {
					return nil, fmt.Errorf("failed to update item matching: %w", err)
				}
				// Update the item status in memory for processing below
				item.Status = domain.ItemStatusMatched
				item.MatchedRelationID = &relationID
			}
		}
	}

	// Mark skipped rows
	skipRowMap := make(map[int]bool)
	for _, rowNum := range req.SkipRows {
		skipRowMap[rowNum] = true
	}

	for _, item := range allItems {
		if skipRowMap[item.RowNumber] {
			item.Status = domain.ItemStatusSkipped
			if err := s.batchRepo.UpdateBatchItem(ctx, item); err != nil {
				return nil, fmt.Errorf("failed to skip item: %w", err)
			}
		}
	}

	// Update batch status to processing
	if err := s.batchRepo.UpdateBatchStatus(ctx, req.BatchID, domain.BatchStatusProcessing); err != nil {
		return nil, fmt.Errorf("failed to update batch status: %w", err)
	}

	// Prepare vouchers for matched items
	var voucherRequests []voucherService.BillVoucherRequest
	var processableItems []*domain.BatchPaymentItem

	for _, item := range allItems {
		if item.Status == domain.ItemStatusMatched && item.MatchedRelationID != nil {
			// Calculate net amount (gross - TDS)
			netAmount := item.CSVAmount - item.TDSAmount

			voucherRequests = append(voucherRequests, voucherService.BillVoucherRequest{
				RelationID:      *item.MatchedRelationID,
				InvoiceNumber:   item.InvoiceNumber,
				GrossAmount:     item.CSVAmount,
				NetAmount:       netAmount,
				GSTAmount:       item.GSTAmount,
				TDSAmount:       item.TDSAmount,
				ExpenseCategory: item.ExpenseCategory,
				Description:     item.Description,
			})

			processableItems = append(processableItems, item)
		}
	}

	if len(voucherRequests) == 0 {
		return nil, fmt.Errorf("no matched items to process")
	}

	// Create draft vouchers using voucher service
	voucherResp, err := s.voucherService.CreateMultiVouchers(ctx, &voucherService.CreateMultiVouchersRequest{
		BusinessID: businessID,
		UserID:     userID,
		Vouchers:   voucherRequests,
	})

	if err != nil {
		// Mark batch as failed
		_ = s.batchRepo.UpdateBatchStatus(ctx, req.BatchID, domain.BatchStatusFailed)
		return nil, fmt.Errorf("failed to create vouchers: %w", err)
	}

	// Update batch items with created voucher IDs
	successCount := 0
	failedCount := 0
	now := time.Now()

	for i, voucherResult := range voucherResp.Vouchers {
		if i >= len(processableItems) {
			break
		}

		item := processableItems[i]
		item.ProcessedAt = &now

		if voucherResult.VoucherID != uuid.Nil {
			// Success
			item.Status = domain.ItemStatusSuccess
			item.VoucherID = &voucherResult.VoucherID
			successCount++
		} else {
			// Failed
			item.Status = domain.ItemStatusFailed
			item.ErrorMessage = "Failed to create voucher"
			failedCount++
		}

		if err := s.batchRepo.UpdateBatchItem(ctx, item); err != nil {
			return nil, fmt.Errorf("failed to update batch item %d: %w", item.RowNumber, err)
		}
	}

	// Update batch status to completed
	if err := s.batchRepo.UpdateBatchStatus(ctx, req.BatchID, domain.BatchStatusCompleted); err != nil {
		return nil, fmt.Errorf("failed to update batch status: %w", err)
	}

	return &ProcessResponse{
		BatchID:        req.BatchID,
		Status:         string(domain.BatchStatusCompleted),
		TotalItems:     len(processableItems),
		ProcessedItems: successCount + failedCount,
		Message:        fmt.Sprintf("Successfully created %d draft vouchers, %d failed", successCount, failedCount),
	}, nil
}

// GetBatchStatus retrieves the current status of a batch
func (s *BatchPaymentService) GetBatchStatus(ctx context.Context, batchID uuid.UUID) (*BatchStatusResponse, error) {
	// Get batch
	batch, err := s.batchRepo.GetBatchByID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	// Get all items
	items, err := s.batchRepo.GetBatchItems(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch items: %w", err)
	}

	// Build item statuses
	itemStatuses := make([]BatchItemStatus, 0, len(items))
	for _, item := range items {
		itemStatus := BatchItemStatus{
			RowNumber:    item.RowNumber,
			PayeeName:    item.CSVPayeeName,
			Amount:       item.CSVAmount,
			Status:       string(item.Status),
			VoucherID:    item.VoucherID,
			ErrorMessage: item.ErrorMessage,
		}

		// TODO: Fetch voucher number if voucher_id exists

		itemStatuses = append(itemStatuses, itemStatus)
	}

	response := &BatchStatusResponse{
		BatchID:       batch.BatchID,
		Status:        string(batch.Status),
		TotalRows:     batch.TotalRows,
		ProcessedRows: batch.ProcessedRows,
		SuccessRows:   batch.SuccessRows,
		FailedRows:    batch.FailedRows,
		TotalAmount:   batch.TotalAmount,
		Items:         itemStatuses,
		UploadedAt:    batch.CreatedAt,
		CompletedAt:   batch.CompletedAt,
	}

	return response, nil
}

// GetBusinessBatches retrieves all batches for a business
func (s *BatchPaymentService) GetBusinessBatches(
	ctx context.Context,
	businessID uuid.UUID,
	page, pageSize int,
) (*BatchListResponse, error) {
	offset := (page - 1) * pageSize

	batches, totalCount, err := s.batchRepo.GetBusinessBatches(ctx, businessID, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get business batches: %w", err)
	}

	summaries := make([]BatchSummary, 0, len(batches))
	for _, batch := range batches {
		summaries = append(summaries, BatchSummary{
			BatchID:     batch.BatchID,
			FileName:    batch.FileName,
			Status:      string(batch.Status),
			TotalRows:   batch.TotalRows,
			SuccessRows: batch.SuccessRows,
			FailedRows:  batch.FailedRows,
			TotalAmount: batch.TotalAmount,
			UploadedAt:  batch.CreatedAt,
			CompletedAt: batch.CompletedAt,
		})
	}

	return &BatchListResponse{
		Batches:    summaries,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
	}, nil
}
