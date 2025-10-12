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
	// Use x-migrations-table-quoted parameter for schema-qualified table name
	// This ensures schema_migrations is created in xcentro schema, not public
	// When quoted=true, we must use double quotes around schema.table
	databaseURL := fmt.Sprintf(`pgx5://%s:%s@%s:%d/%s?sslmode=%s&x-migrations-table-quoted=true&x-migrations-table="xcentro"."schema_migrations"`,
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

// Force sets the migration version without running migrations (for fixing dirty state)
func (m *MigrationService) Force(ctx context.Context, version int) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	if err := migrator.Force(version); err != nil {
		return fmt.Errorf("failed to force version %d: %w", version, err)
	}

	return nil
}

// Drop runs all down migrations to completely wipe the database
func (m *MigrationService) Drop(ctx context.Context) error {
	migrator, err := m.createMigrator()
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer migrator.Close()

	// Get current version to determine how many down migrations to run
	version, dirty, err := migrator.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// If database is dirty, force clean it first
	if dirty {
		if err := migrator.Force(int(version)); err != nil {
			return fmt.Errorf("failed to force clean dirty state: %w", err)
		}
	}

	// If we're already at version 0 or no migrations, nothing to do
	if version == 0 || err == migrate.ErrNilVersion {
		return nil
	}

	// Run down migrations one by one to properly clean up all database objects
	// We can't use Steps(-int(version)) because of potential gaps in migration numbers
	// So we repeatedly call Down() until we reach version 0
	for {
		currentVersion, _, err := migrator.Version()
		if err == migrate.ErrNilVersion || currentVersion == 0 {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get version during drop: %w", err)
		}

		// Run one down migration
		if err := migrator.Steps(-1); err != nil {
			if err == migrate.ErrNoChange {
				break
			}
			return fmt.Errorf("failed to run down migration from version %d: %w", currentVersion, err)
		}
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
