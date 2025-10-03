package voucher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xcentro.in/internal/core/voucher"
)

// PostgresERPSyncQueueRepository implements the ERPSyncQueueRepository interface using PostgreSQL
type PostgresERPSyncQueueRepository struct {
	db *pgxpool.Pool
}

// NewPostgresERPSyncQueueRepository creates a new PostgreSQL ERP sync queue repository
func NewPostgresERPSyncQueueRepository(db *pgxpool.Pool) voucher.ERPSyncQueueRepository {
	return &PostgresERPSyncQueueRepository{
		db: db,
	}
}

// Enqueue adds a voucher to the ERP sync queue
func (r *PostgresERPSyncQueueRepository) Enqueue(ctx context.Context, queueEntry *voucher.ERPSyncQueue) error {
	query := `
		INSERT INTO xcentro.erp_sync_queue (
			queue_id, voucher_id, sync_direction, queue_status,
			retry_count, max_retries, request_payload, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`

	// Marshal request payload
	requestPayloadJSON, err := json.Marshal(queueEntry.RequestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal request_payload: %w", err)
	}

	_, err = r.db.Exec(ctx, query,
		queueEntry.QueueID,
		queueEntry.VoucherID,
		queueEntry.SyncDirection,
		queueEntry.QueueStatus,
		queueEntry.RetryCount,
		queueEntry.MaxRetries,
		requestPayloadJSON,
		queueEntry.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to enqueue voucher for ERP sync: %w", err)
	}

	return nil
}

// Dequeue retrieves pending queue entries for processing
func (r *PostgresERPSyncQueueRepository) Dequeue(ctx context.Context, limit int) ([]*voucher.ERPSyncQueue, error) {
	query := `
		SELECT queue_id, voucher_id, sync_direction, queue_status,
		       retry_count, max_retries, next_retry_at,
		       request_payload, response_payload,
		       error_code, error_message,
		       created_at, started_at, completed_at
		FROM xcentro.erp_sync_queue
		WHERE queue_status = 'pending'
		   OR (queue_status = 'failed' AND next_retry_at <= NOW())
		ORDER BY created_at
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue ERP sync entries: %w", err)
	}
	defer rows.Close()

	var entries []*voucher.ERPSyncQueue
	for rows.Next() {
		entry, err := r.scanERPSyncQueueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue entry row: %w", err)
		}
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queue entry rows: %w", err)
	}

	return entries, nil
}

// UpdateQueueStatus updates the status of a queue entry
func (r *PostgresERPSyncQueueRepository) UpdateQueueStatus(ctx context.Context, queueID uuid.UUID, status string, response map[string]interface{}, errorCode string, errorMessage string) error {
	query := `
		UPDATE xcentro.erp_sync_queue
		SET queue_status = $2,
		    response_payload = $3,
		    error_code = $4,
		    error_message = $5,
		    completed_at = CASE WHEN $2 = 'completed' THEN NOW() ELSE NULL END,
		    started_at = CASE WHEN started_at IS NULL THEN NOW() ELSE started_at END
		WHERE queue_id = $1
	`

	// Marshal response payload
	responsePayloadJSON, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response_payload: %w", err)
	}

	result, err := r.db.Exec(ctx, query, queueID, status, responsePayloadJSON, errorCode, errorMessage)
	if err != nil {
		return fmt.Errorf("failed to update queue status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("queue entry not found")
	}

	return nil
}

// IncrementRetryCount increments retry count and schedules next retry
func (r *PostgresERPSyncQueueRepository) IncrementRetryCount(ctx context.Context, queueID uuid.UUID, nextRetryAt time.Time) error {
	query := `
		UPDATE xcentro.erp_sync_queue
		SET retry_count = retry_count + 1,
		    next_retry_at = $2,
		    queue_status = CASE
		      WHEN retry_count + 1 < max_retries THEN 'pending'
		      ELSE 'failed'
		    END
		WHERE queue_id = $1
	`

	result, err := r.db.Exec(ctx, query, queueID, nextRetryAt)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("queue entry not found")
	}

	return nil
}

// GetByVoucher retrieves queue entries for a voucher
func (r *PostgresERPSyncQueueRepository) GetByVoucher(ctx context.Context, voucherID uuid.UUID) ([]*voucher.ERPSyncQueue, error) {
	query := `
		SELECT queue_id, voucher_id, sync_direction, queue_status,
		       retry_count, max_retries, next_retry_at,
		       request_payload, response_payload,
		       error_code, error_message,
		       created_at, started_at, completed_at
		FROM xcentro.erp_sync_queue
		WHERE voucher_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, voucherID)
	if err != nil {
		return nil, fmt.Errorf("failed to query queue entries by voucher: %w", err)
	}
	defer rows.Close()

	var entries []*voucher.ERPSyncQueue
	for rows.Next() {
		entry, err := r.scanERPSyncQueueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue entry row: %w", err)
		}
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queue entry rows: %w", err)
	}

	return entries, nil
}

// GetFailedEntries retrieves failed queue entries for monitoring
func (r *PostgresERPSyncQueueRepository) GetFailedEntries(ctx context.Context, limit int) ([]*voucher.ERPSyncQueue, error) {
	query := `
		SELECT queue_id, voucher_id, sync_direction, queue_status,
		       retry_count, max_retries, next_retry_at,
		       request_payload, response_payload,
		       error_code, error_message,
		       created_at, started_at, completed_at
		FROM xcentro.erp_sync_queue
		WHERE queue_status = 'failed'
		  AND retry_count >= max_retries
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query failed queue entries: %w", err)
	}
	defer rows.Close()

	var entries []*voucher.ERPSyncQueue
	for rows.Next() {
		entry, err := r.scanERPSyncQueueRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan queue entry row: %w", err)
		}
		entries = append(entries, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queue entry rows: %w", err)
	}

	return entries, nil
}

// Helper methods

// scanERPSyncQueue scans a single row into an ERPSyncQueue struct
func (r *PostgresERPSyncQueueRepository) scanERPSyncQueue(row pgx.Row) (*voucher.ERPSyncQueue, error) {
	var entry voucher.ERPSyncQueue
	var requestPayloadJSON, responsePayloadJSON []byte

	err := row.Scan(
		&entry.QueueID,
		&entry.VoucherID,
		&entry.SyncDirection,
		&entry.QueueStatus,
		&entry.RetryCount,
		&entry.MaxRetries,
		&entry.NextRetryAt,
		&requestPayloadJSON,
		&responsePayloadJSON,
		&entry.ErrorCode,
		&entry.ErrorMessage,
		&entry.CreatedAt,
		&entry.StartedAt,
		&entry.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSONB fields
	if len(requestPayloadJSON) > 0 {
		if err := json.Unmarshal(requestPayloadJSON, &entry.RequestPayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request_payload: %w", err)
		}
	}

	if len(responsePayloadJSON) > 0 {
		if err := json.Unmarshal(responsePayloadJSON, &entry.ResponsePayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response_payload: %w", err)
		}
	}

	return &entry, nil
}

// scanERPSyncQueueRow scans rows (for Query) into an ERPSyncQueue struct
func (r *PostgresERPSyncQueueRepository) scanERPSyncQueueRow(rows pgx.Rows) (*voucher.ERPSyncQueue, error) {
	var entry voucher.ERPSyncQueue
	var requestPayloadJSON, responsePayloadJSON []byte

	err := rows.Scan(
		&entry.QueueID,
		&entry.VoucherID,
		&entry.SyncDirection,
		&entry.QueueStatus,
		&entry.RetryCount,
		&entry.MaxRetries,
		&entry.NextRetryAt,
		&requestPayloadJSON,
		&responsePayloadJSON,
		&entry.ErrorCode,
		&entry.ErrorMessage,
		&entry.CreatedAt,
		&entry.StartedAt,
		&entry.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSONB fields
	if len(requestPayloadJSON) > 0 {
		if err := json.Unmarshal(requestPayloadJSON, &entry.RequestPayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal request_payload: %w", err)
		}
	}

	if len(responsePayloadJSON) > 0 {
		if err := json.Unmarshal(responsePayloadJSON, &entry.ResponsePayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response_payload: %w", err)
		}
	}

	return &entry, nil
}

// CalculateNextRetryTime calculates next retry time with exponential backoff
// Retry schedule:
// - Retry 1: After 1 minute
// - Retry 2: After 5 minutes
// - Retry 3: After 15 minutes
func CalculateNextRetryTime(retryCount int) time.Time {
	now := time.Now()

	switch retryCount {
	case 0:
		// First retry after 1 minute
		return now.Add(1 * time.Minute)
	case 1:
		// Second retry after 5 minutes
		return now.Add(5 * time.Minute)
	case 2:
		// Third retry after 15 minutes
		return now.Add(15 * time.Minute)
	default:
		// Beyond max retries, set far in future (should not happen due to check constraint)
		return now.Add(24 * time.Hour)
	}
}
