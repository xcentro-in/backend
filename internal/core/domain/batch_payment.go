package domain

import (
	"time"

	"github.com/google/uuid"
)

// BatchPaymentStatus represents the status of a batch payment
type BatchPaymentStatus string

const (
	BatchStatusPending    BatchPaymentStatus = "pending"     // Uploaded, waiting for user confirmation
	BatchStatusProcessing BatchPaymentStatus = "processing" // Payment processing in progress
	BatchStatusCompleted  BatchPaymentStatus = "completed"  // All payments processed
	BatchStatusFailed     BatchPaymentStatus = "failed"     // Batch failed to process
	BatchStatusCancelled  BatchPaymentStatus = "cancelled"  // User cancelled the batch
)

// BatchPaymentItemStatus represents the status of an individual item in batch
type BatchPaymentItemStatus string

const (
	ItemStatusMatched   BatchPaymentItemStatus = "matched"    // Auto-matched to payee
	ItemStatusAmbiguous BatchPaymentItemStatus = "ambiguous"  // Multiple matches found
	ItemStatusNotFound  BatchPaymentItemStatus = "not_found"  // No match found
	ItemStatusPending   BatchPaymentItemStatus = "pending"    // Waiting for payment
	ItemStatusProcessing BatchPaymentItemStatus = "processing" // Payment in progress
	ItemStatusSuccess   BatchPaymentItemStatus = "success"   // Payment successful
	ItemStatusFailed    BatchPaymentItemStatus = "failed"    // Payment failed
	ItemStatusSkipped   BatchPaymentItemStatus = "skipped"   // User chose to skip
)

// BatchPayment represents a batch CSV upload job for tracking
type BatchPayment struct {
	BatchID    uuid.UUID          `json:"batch_id"`
	BusinessID uuid.UUID          `json:"business_id"`
	UploadedBy uuid.UUID          `json:"uploaded_by"` // User who uploaded
	FileName   string             `json:"file_name,omitempty"`
	Status     BatchPaymentStatus `json:"status"`

	// Row tracking
	TotalRows     int   `json:"total_rows"`
	TotalAmount   int64 `json:"total_amount"`   // Total CSV amount in paise (informational)
	ProcessedRows int   `json:"processed_rows"`
	SuccessRows   int   `json:"success_rows"`
	FailedRows    int   `json:"failed_rows"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// BatchPaymentItem represents a single row in the batch
type BatchPaymentItem struct {
	ItemID      uuid.UUID              `json:"item_id"`
	BatchID     uuid.UUID              `json:"batch_id"`
	RowNumber   int                    `json:"row_number"`
	Status      BatchPaymentItemStatus `json:"status"`

	// CSV data (as provided by user)
	CSVPayeeName    string `json:"csv_payee_name"`
	CSVAccountLast4 string `json:"csv_account_last_4"`
	CSVAmount       int64  `json:"csv_amount"` // In paise
	PaymentContext  string `json:"payment_context"`
	Description     string `json:"description"`
	ExpenseCategory string `json:"expense_category,omitempty"`
	GSTAmount       int64  `json:"gst_amount,omitempty"`
	TDSAmount       int64  `json:"tds_amount,omitempty"`
	InvoiceNumber   string `json:"invoice_number,omitempty"`

	// Matching result
	MatchedRelationID *uuid.UUID `json:"matched_relation_id,omitempty"`
	MatchConfidence   float64    `json:"match_confidence,omitempty"` // 0.0 to 1.0

	// Voucher creation result
	VoucherID    *uuid.UUID `json:"voucher_id,omitempty"`    // Created draft voucher
	ErrorMessage string     `json:"error_message,omitempty"` // Error if voucher creation failed

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

// BatchPaymentItemMatch represents a possible match for ambiguous items
type BatchPaymentItemMatch struct {
	ItemID          uuid.UUID `json:"item_id"`
	RelationID      uuid.UUID `json:"relation_id"`
	PayeeName       string    `json:"payee_name"`
	AccountLast4    string    `json:"account_last_4"`
	BankName        string    `json:"bank_name"`
	MatchConfidence float64   `json:"match_confidence"`
}
