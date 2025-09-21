package client

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxDB wraps pgxpool.Pool to implement the DB interface
type PgxDB struct {
	pool *pgxpool.Pool
}

// PgxTx wraps pgx.Tx to implement the Tx interface
type PgxTx struct {
	tx pgx.Tx
}

// DBConfig holds database configuration
type DBConfig struct {
	Host            string
	Port            int
	Database        string
	Username        string
	Password        string
	Schema          string
	MaxConnections  int32
	MinConnections  int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	SSLMode         string
}

// NewPgxDB creates a new PGX database connection pool
func NewPgxDB(ctx context.Context, config DBConfig) (*PgxDB, error) {
	// Build connection string
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s search_path=%s",
		config.Host,
		config.Port,
		config.Database,
		config.Username,
		config.Password,
		config.SSLMode,
		config.Schema,
	)

	// Parse config
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Set pool configuration
	poolConfig.MaxConns = config.MaxConnections
	poolConfig.MinConns = config.MinConnections
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PgxDB{pool: pool}, nil
}

// Query implements Querier interface
func (db *PgxDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// QueryRow implements Querier interface
func (db *PgxDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// Exec implements Querier interface
func (db *PgxDB) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// BeginTx starts a new transaction
func (db *PgxDB) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &PgxTx{tx: tx}, nil
}

// Close closes the database connection pool
func (db *PgxDB) Close() {
	db.pool.Close()
}

// Ping tests the database connection
func (db *PgxDB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// GetPool returns the underlying pgxpool.Pool
func (db *PgxDB) GetPool() *pgxpool.Pool {
	return db.pool
}

// Query implements Querier interface for transactions
func (tx *PgxTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return tx.tx.Query(ctx, sql, args...)
}

// QueryRow implements Querier interface for transactions
func (tx *PgxTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return tx.tx.QueryRow(ctx, sql, args...)
}

// Exec implements Querier interface for transactions
func (tx *PgxTx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return tx.tx.Exec(ctx, sql, args...)
}

// Commit commits the transaction
func (tx *PgxTx) Commit(ctx context.Context) error {
	return tx.tx.Commit(ctx)
}

// Rollback rolls back the transaction
func (tx *PgxTx) Rollback(ctx context.Context) error {
	return tx.tx.Rollback(ctx)
}

// DefaultDBConfig returns a default database configuration suitable for development
func DefaultDBConfig() DBConfig {
	return DBConfig{
		Host:            "localhost",
		Port:            5432,
		Database:        "xcentro",
		Username:        "postgres",
		Password:        "postgres",
		MaxConnections:  25,
		MinConnections:  5,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: time.Minute * 30,
		SSLMode:         "disable",
	}
}
