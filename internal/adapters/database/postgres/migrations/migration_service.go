package migrations

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"xcentro.in/internal/shared/config"
)

// MigrationService handles database migrations
type MigrationService struct {
	migrationsPath string
	databaseURL    string
}

// NewMigrationService creates a new migration service
func NewMigrationService(migrationsPath string, dbConfig config.MigrationDatabaseConfig) *MigrationService {
	databaseURL := fmt.Sprintf("pgx5://%s:%s@%s:%d/%s?sslmode=%s",
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Database,
		dbConfig.SSLMode,
	)
	return &MigrationService{
		migrationsPath: migrationsPath,
		databaseURL:    databaseURL,
	}
}

// Up runs all pending up migrations
func (m *MigrationService) Up(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run up migrations: %w", err)
	}

	return nil
}

// Down runs one down migration
func (m *MigrationService) Down(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Steps(-1); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run down migration: %w", err)
	}

	return nil
}

// Version returns the current migration version
func (m *MigrationService) Version(ctx context.Context) (uint, bool, error) {
	migrator, err := m.createMigrator()
	if err != nil {
		return 0, false, fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	version, dirty, err := migrator.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return 0, false, fmt.Errorf("failed to get migration version: %w", err)
	}

	return version, dirty, nil
}

// MigrateToVersion migrates to a specific version
func (m *MigrationService) MigrateToVersion(ctx context.Context, version uint) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Migrate(version); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to migrate to version %d: %w", version, err)
	}

	return nil
}

// createMigrator creates a new migrate instance
func (m *MigrationService) createMigrator() (*migrate.Migrate, error) {
	// Convert relative path to absolute
	absPath, err := filepath.Abs(m.migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	sourceURL := fmt.Sprintf("file://%s", absPath)

	migrator, err := migrate.New(sourceURL, m.databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return migrator, nil
}
