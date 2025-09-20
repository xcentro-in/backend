package client

import (
	"context"
	"fmt"
	"log"
	"time"
)

// DefaultTxManager implements TxManager interface
type DefaultTxManager struct {
	db DB
}

// NewTxManager creates a new transaction manager
func NewTxManager(db DB) TxManager {
	return &DefaultTxManager{
		db: db,
	}
}

// WithTx executes a function within a database transaction
// It handles the transaction lifecycle automatically:
// - Begins a transaction
// - Executes the provided function
// - Commits if the function succeeds
// - Rolls back if the function returns an error or panics
func (tm *DefaultTxManager) WithTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	// Begin transaction
	tx, err := tm.db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure transaction is properly closed
	defer func() {
		if p := recover(); p != nil {
			// Create a timeout context for rollback to avoid hanging
			rbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if rbErr := tx.Rollback(rbCtx); rbErr != nil {
				// Report rollback failure but keep original panic so we preserve its stack trace.
				// Use logging so library users can decide how to capture/report this.
				log.Printf("rollback failed after panic: %v; rollback error: %v", p, rbErr)
			}
			panic(p)
		}
	}()

	// Execute the function
	if err := fn(ctx, tx); err != nil {
		// Function returned error, rollback
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("transaction failed: %w, rollback failed: %v", err, rbErr)
		}
		return err
	}

	// Function succeeded, commit
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
