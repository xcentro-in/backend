package batch_payment

import (
	"time"

	"github.com/google/uuid"
)

// CSVRow represents a single row from uploaded CSV
type CSVRow struct {
	PayeeName       string  `csv:"payee_name"`
	AccountLast4    string  `csv:"account_last_4"`
	IFSCCode        string  `csv:"ifsc_code"`
	Amount          float64 `csv:"amount"` // In rupees
	PaymentContext  string  `csv:"payment_context"`
	Description     string  `csv:"description"`
	ExpenseCategory string  `csv:"expense_category"`
	GSTAmount       float64 `csv:"gst_amount"`
	TDSAmount       float64 `csv:"tds_amount"`
	InvoiceNumber   string  `csv:"invoice_number"`
}

// MatchedItem represents an auto-matched batch item
type MatchedItem struct {
	RowNumber           int       `json:"row_number"`
	CSVPayeeName        string    `json:"csv_payee_name"`
	CSVAccountLast4     string    `json:"csv_account_last_4"`
	Amount              int64     `json:"amount"` // In paise
	PaymentContext      string    `json:"payment_context"`
	Description         string    `json:"description"`
	InvoiceNumber       string    `json:"invoice_number"`
	MatchedRelationID   uuid.UUID `json:"matched_relation_id"`
	MatchedPayeeName    string    `json:"matched_payee_name"`
	MatchedAccount      string    `json:"matched_account"`
	MatchedBankName     string    `json:"matched_bank_name"`
	MatchConfidence     float64   `json:"match_confidence"`
}

// AmbiguousItem represents an item with multiple possible matches
type AmbiguousItem struct {
	RowNumber        int                `json:"row_number"`
	CSVPayeeName     string             `json:"csv_payee_name"`
	CSVAccountLast4  string             `json:"csv_account_last_4"`
	Amount           int64              `json:"amount"` // In paise
	PaymentContext   string             `json:"payment_context"`
	Description      string             `json:"description"`
	InvoiceNumber    string             `json:"invoice_number"`
	PossibleMatches  []PossibleMatch    `json:"possible_matches"`
}

// PossibleMatch represents a possible payee match
type PossibleMatch struct {
	RelationID      uuid.UUID `json:"relation_id"`
	PayeeName       string    `json:"payee_name"`
	AccountLast4    string    `json:"account_last_4"`
	BankName        string    `json:"bank_name"`
	MatchConfidence float64   `json:"match_confidence"`
}

// NotFoundItem represents an item with no matches
type NotFoundItem struct {
	RowNumber       int    `json:"row_number"`
	CSVPayeeName    string `json:"csv_payee_name"`
	CSVAccountLast4 string `json:"csv_account_last_4"`
	Amount          int64  `json:"amount"` // In paise
	PaymentContext  string `json:"payment_context"`
	Description     string `json:"description"`
	InvoiceNumber   string `json:"invoice_number"`
	Reason          string `json:"reason"`
}

// PreviewResponse represents the response after CSV upload with matching results
type PreviewResponse struct {
	BatchID           uuid.UUID         `json:"batch_id"`
	TotalRows         int               `json:"total_rows"`
	MatchedCount      int               `json:"matched_count"`
	AmbiguousCount    int               `json:"ambiguous_count"`
	NotFoundCount     int               `json:"not_found_count"`
	Matched           []MatchedItem     `json:"matched"`
	Ambiguous         []AmbiguousItem   `json:"ambiguous"`
	NotFound          []NotFoundItem    `json:"not_found"`
	TotalAmount       int64             `json:"total_amount"`        // Sum of all amounts in paise
	FileName          string            `json:"file_name,omitempty"`
	UploadedAt        time.Time         `json:"uploaded_at"`
}

// ProcessRequest represents the request to process a batch
type ProcessRequest struct {
	BatchID             uuid.UUID            `json:"batch_id"`
	AmbiguousMappings   map[int]uuid.UUID    `json:"ambiguous_mappings"` // Row number → relation_id
	SkipRows            []int                `json:"skip_rows"`          // Rows to skip
}

// ProcessResponse represents the response after starting batch processing
type ProcessResponse struct {
	BatchID        uuid.UUID `json:"batch_id"`
	Status         string    `json:"status"`
	TotalItems     int       `json:"total_items"`
	ProcessedItems int       `json:"processed_items"`
	Message        string    `json:"message"`
}

// BatchStatusResponse represents the current status of a batch
type BatchStatusResponse struct {
	BatchID          uuid.UUID         `json:"batch_id"`
	Status           string            `json:"status"`
	TotalRows        int               `json:"total_rows"`
	ProcessedRows    int               `json:"processed_rows"`
	SuccessRows      int              `json:"success_rows"`
	FailedRows       int               `json:"failed_rows"`
	TotalAmount      int64             `json:"total_amount"`
	Items            []BatchItemStatus `json:"items"`
	UploadedAt       time.Time         `json:"uploaded_at"`
	CompletedAt      *time.Time        `json:"completed_at,omitempty"`
}

// BatchItemStatus represents the status of a single batch item
type BatchItemStatus struct {
	RowNumber      int        `json:"row_number"`
	PayeeName      string     `json:"payee_name"`
	Amount         int64      `json:"amount"`
	Status         string     `json:"status"`
	VoucherID      *uuid.UUID `json:"voucher_id,omitempty"`
	VoucherNumber  string     `json:"voucher_number,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
}

// BatchListResponse represents paginated list of batches
type BatchListResponse struct {
	Batches    []BatchSummary `json:"batches"`
	TotalCount int            `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
}

// BatchSummary represents a summary of a batch for listing
type BatchSummary struct {
	BatchID       uuid.UUID  `json:"batch_id"`
	FileName      string     `json:"file_name"`
	Status        string     `json:"status"`
	TotalRows     int        `json:"total_rows"`
	SuccessRows   int        `json:"success_rows"`
	FailedRows    int        `json:"failed_rows"`
	TotalAmount   int64      `json:"total_amount"`
	UploadedAt    time.Time  `json:"uploaded_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}
