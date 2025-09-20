package client

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is a common interface for both pgx.Conn and pgx.Tx
// This allows repositories to work with either a regular connection or a transaction
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// DB represents the database connection interface
type DB interface {
	Querier
	// BeginTx starts a new transaction
	BeginTx(ctx context.Context) (Tx, error)
	// Close closes the database connection
	Close()
	// Ping tests the database connection
	Ping(ctx context.Context) error
}

// Tx represents a database transaction interface
type Tx interface {
	Querier
	// Commit commits the transaction
	Commit(ctx context.Context) error
	// Rollback rolls back the transaction
	Rollback(ctx context.Context) error
}

// TxManager provides transaction management functionality
type TxManager interface {
	// WithTx executes a function within a database transaction
	// If the function returns an error, the transaction is rolled back
	// If the function returns nil, the transaction is committed
	WithTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}
