package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"xcentro.in/internal/adapters/database/postgres/client"
	"xcentro.in/internal/core/payment"
)

// PostgresPaymentStateChangeRepository implements payment state change persistence
type PostgresPaymentStateChangeRepository struct {
	querier client.Querier
}

// NewPostgresPaymentStateChangeRepository creates a new PostgreSQL state change repository
func NewPostgresPaymentStateChangeRepository(db client.DB) payment.PaymentStateChangeRepository {
	return &PostgresPaymentStateChangeRepository{
		querier: db,
	}
}

// WithTx creates a new repository instance that uses a transaction
func (r *PostgresPaymentStateChangeRepository) WithTx(tx client.Tx) *PostgresPaymentStateChangeRepository {
	return &PostgresPaymentStateChangeRepository{
		querier: tx,
	}
}

// Create records a new payment state change
func (r *PostgresPaymentStateChangeRepository) Create(ctx context.Context, req *payment.CreatePaymentStateChangeRequest) (*payment.PaymentStateChange, error) {
	stateChangeID := uuid.Must(uuid.NewV7())

	// Auto-populate previous_state from last state change
	// Repository owns this responsibility - not the caller
	lastState, lastStateErr := r.GetLatestByPaymentID(ctx, req.PaymentID)
	if lastStateErr == nil && lastState != nil {
		// Last state's "new_state" becomes current state's "previous_state"
		req.PreviousState = lastState.NewState
	}
	// If no previous state exists (first state change), previous_state remains empty

	// Marshal metadata to JSON
	var metadataJSON []byte
	var err error
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO xcentro.payment_state_changes (
			state_change_id, payment_id, transfer_request_id,
			action, action_timestamp, actor_type, actor_user_id, actor_name,
			previous_state, new_state, amount, currency, utr, provider,
			metadata, remarks, created_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14,
			$15, $16, NOW()
		)
		RETURNING created_at
	`

	var createdAt sql.NullTime
	err = r.querier.QueryRow(ctx, query,
		stateChangeID,
		req.PaymentID,
		req.TransferID,
		req.Action,
		req.ActionTimestamp,
		req.ActorType,
		req.ActorUserID,
		nullString(req.ActorName),
		nullString(req.PreviousState),
		nullString(req.NewState),
		nullInt64(req.Amount),
		nullString(req.Currency),
		nullString(req.UTR),
		nullString(req.Provider),
		metadataJSON,
		nullString(req.Remarks),
	).Scan(&createdAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create payment state change: %w", err)
	}

	// Build response
	stateChange := &payment.PaymentStateChange{
		StateChangeID:   stateChangeID,
		PaymentID:       req.PaymentID,
		TransferID:      req.TransferID,
		Action:          req.Action,
		ActionTimestamp: req.ActionTimestamp,
		ActorType:       req.ActorType,
		ActorUserID:     req.ActorUserID,
		ActorName:       req.ActorName,
		PreviousState:   req.PreviousState,
		NewState:        req.NewState,
		Amount:          req.Amount,
		Currency:        req.Currency,
		UTR:             req.UTR,
		Provider:        req.Provider,
		Metadata:        req.Metadata,
		Remarks:         req.Remarks,
		CreatedAt:       createdAt.Time,
	}

	return stateChange, nil
}

// GetByPaymentID retrieves all state changes for a payment (chronologically)
func (r *PostgresPaymentStateChangeRepository) GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]*payment.PaymentStateChange, error) {
	query := `
		SELECT
			state_change_id,
			payment_id::text,
			COALESCE(transfer_request_id::text, '') as transfer_request_id,
			action, action_timestamp,
			actor_type,
			COALESCE(actor_user_id::text, '') as actor_user_id,
			COALESCE(actor_name, '') as actor_name,
			COALESCE(previous_state, '') as previous_state,
			COALESCE(new_state, '') as new_state,
			COALESCE(amount, 0) as amount,
			COALESCE(currency, '') as currency,
			COALESCE(utr, '') as utr,
			COALESCE(provider, '') as provider,
			COALESCE(metadata, '{}'::jsonb) as metadata,
			COALESCE(remarks, '') as remarks,
			created_at
		FROM xcentro.payment_state_changes
		WHERE payment_id = $1
		ORDER BY action_timestamp ASC, created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, paymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query state changes: %w", err)
	}
	defer rows.Close()

	var stateChanges []*payment.PaymentStateChange
	for rows.Next() {
		sc, err := r.scanStateChange(rows)
		if err != nil {
			return nil, err
		}
		stateChanges = append(stateChanges, sc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state changes: %w", err)
	}

	return stateChanges, nil
}

// GetByTransferID retrieves all state changes for a payout transfer
func (r *PostgresPaymentStateChangeRepository) GetByTransferID(ctx context.Context, transferID uuid.UUID) ([]*payment.PaymentStateChange, error) {
	query := `
		SELECT
			state_change_id,
			payment_id::text,
			COALESCE(transfer_request_id::text, '') as transfer_request_id,
			action, action_timestamp,
			actor_type,
			COALESCE(actor_user_id::text, '') as actor_user_id,
			COALESCE(actor_name, '') as actor_name,
			COALESCE(previous_state, '') as previous_state,
			COALESCE(new_state, '') as new_state,
			COALESCE(amount, 0) as amount,
			COALESCE(currency, '') as currency,
			COALESCE(utr, '') as utr,
			COALESCE(provider, '') as provider,
			COALESCE(metadata, '{}'::jsonb) as metadata,
			COALESCE(remarks, '') as remarks,
			created_at
		FROM xcentro.payment_state_changes
		WHERE transfer_request_id = $1
		ORDER BY action_timestamp ASC, created_at ASC
	`

	rows, err := r.querier.Query(ctx, query, transferID)
	if err != nil {
		return nil, fmt.Errorf("failed to query state changes: %w", err)
	}
	defer rows.Close()

	var stateChanges []*payment.PaymentStateChange
	for rows.Next() {
		sc, err := r.scanStateChange(rows)
		if err != nil {
			return nil, err
		}
		stateChanges = append(stateChanges, sc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state changes: %w", err)
	}

	return stateChanges, nil
}

// GetLatestByPaymentID retrieves the most recent state change for a payment
func (r *PostgresPaymentStateChangeRepository) GetLatestByPaymentID(ctx context.Context, paymentID uuid.UUID) (*payment.PaymentStateChange, error) {
	query := `
		SELECT
			state_change_id,
			payment_id::text,
			COALESCE(transfer_request_id::text, '') as transfer_request_id,
			action, action_timestamp,
			actor_type,
			COALESCE(actor_user_id::text, '') as actor_user_id,
			COALESCE(actor_name, '') as actor_name,
			COALESCE(previous_state, '') as previous_state,
			COALESCE(new_state, '') as new_state,
			COALESCE(amount, 0) as amount,
			COALESCE(currency, '') as currency,
			COALESCE(utr, '') as utr,
			COALESCE(provider, '') as provider,
			COALESCE(metadata, '{}'::jsonb) as metadata,
			COALESCE(remarks, '') as remarks,
			created_at
		FROM xcentro.payment_state_changes
		WHERE payment_id = $1
		ORDER BY action_timestamp DESC, created_at DESC
		LIMIT 1
	`

	row := r.querier.QueryRow(ctx, query, paymentID)
	return r.scanStateChange(row)
}

// GetByAction retrieves state changes by action type (for monitoring/reporting)
func (r *PostgresPaymentStateChangeRepository) GetByAction(ctx context.Context, action payment.PaymentAction, limit int, offset int) ([]*payment.PaymentStateChange, error) {
	query := `
		SELECT
			state_change_id,
			payment_id::text,
			COALESCE(transfer_request_id::text, '') as transfer_request_id,
			action, action_timestamp,
			actor_type,
			COALESCE(actor_user_id::text, '') as actor_user_id,
			COALESCE(actor_name, '') as actor_name,
			COALESCE(previous_state, '') as previous_state,
			COALESCE(new_state, '') as new_state,
			COALESCE(amount, 0) as amount,
			COALESCE(currency, '') as currency,
			COALESCE(utr, '') as utr,
			COALESCE(provider, '') as provider,
			COALESCE(metadata, '{}'::jsonb) as metadata,
			COALESCE(remarks, '') as remarks,
			created_at
		FROM xcentro.payment_state_changes
		WHERE action = $1
		ORDER BY action_timestamp DESC, created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.querier.Query(ctx, query, action, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query state changes: %w", err)
	}
	defer rows.Close()

	var stateChanges []*payment.PaymentStateChange
	for rows.Next() {
		sc, err := r.scanStateChange(rows)
		if err != nil {
			return nil, err
		}
		stateChanges = append(stateChanges, sc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state changes: %w", err)
	}

	return stateChanges, nil
}

// scanStateChange scans a row into a PaymentStateChange struct
func (r *PostgresPaymentStateChangeRepository) scanStateChange(row interface {
	Scan(dest ...interface{}) error
}) (*payment.PaymentStateChange, error) {
	var sc payment.PaymentStateChange
	var paymentIDStr, transferIDStr, actorUserIDStr string
	var previousState, newState, actorName string
	var amount int64
	var currency, utr, provider, remarks string
	var metadataJSON []byte

	err := row.Scan(
		&sc.StateChangeID,
		&paymentIDStr,
		&transferIDStr,
		&sc.Action,
		&sc.ActionTimestamp,
		&sc.ActorType,
		&actorUserIDStr,
		&actorName,
		&previousState,
		&newState,
		&amount,
		&currency,
		&utr,
		&provider,
		&metadataJSON,
		&remarks,
		&sc.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan state change: %w", err)
	}

	// Parse payment_id (required field)
	if paymentIDStr != "" {
		paymentID, err := uuid.Parse(paymentIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse payment_id: %w", err)
		}
		sc.PaymentID = paymentID
	}

	// Parse transfer_id (optional)
	if transferIDStr != "" {
		transferID, _ := uuid.Parse(transferIDStr)
		sc.TransferID = &transferID
	}

	// Parse actor_user_id (optional)
	if actorUserIDStr != "" {
		actorUserID, _ := uuid.Parse(actorUserIDStr)
		sc.ActorUserID = &actorUserID
	}

	// Set optional fields
	sc.ActorName = actorName
	sc.PreviousState = previousState
	sc.NewState = newState
	sc.Amount = amount
	sc.Currency = currency
	sc.UTR = utr
	sc.Provider = provider
	sc.Remarks = remarks

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &sc.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &sc, nil
}

// Helper functions for nullable fields
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}
