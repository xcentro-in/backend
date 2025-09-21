# Database Migrations

This project uses [golang-migrate](https://github.com/golang-migrate/migrate) for database schema management with PostgreSQL and pgxv5.

## Migration Files

Migration files are located in `internal/adapters/database/postgres/migrations/` and follow the naming convention:
- `{version}_{description}.up.sql` - Contains SQL statements to apply the migration
- `{version}_{description}.down.sql` - Contains SQL statements to rollback the migration

Example:
- `000001_create_users_tables.up.sql`
- `000001_create_users_tables.down.sql`

## Running Migrations

### CLI Command

A dedicated CLI tool is available at `cmd/migrate/main.go`:

```bash
# Build the migrate tool
go build -o bin/migrate ./cmd/migrate

# Run all pending migrations
./bin/migrate -action=up

# Rollback one migration
./bin/migrate -action=down

# Check current migration version
./bin/migrate -action=version

# Migrate to specific version
./bin/migrate -action=goto -version=1
```

### Programmatic Usage

```go
package main

import (
    "context"
    "xcentro.in/internal/shared/container"
    "xcentro.in/internal/shared/config"
    "xcentro.in/internal/adapters/database/postgres/migrations"
)

func main() {
    cfg, _ := config.Load("")
    container, _ := container.NewContainer(cfg)
    defer container.Close()
    
    ctx := context.Background()
    
    // Option 1: Using container
    err := migrations.RunMigrationsFromContainer(ctx, container)
    if err != nil {
        // handle error
    }
    
    // Option 2: Direct usage
    service := container.GetMigrationService()
    err = service.Up(ctx)
    if err != nil {
        // handle error
    }
}
```

### On Application Startup

To automatically run migrations when your application starts, you can use the container's migration service:

```go
// In your main application
container, err := container.NewContainer(cfg)
if err != nil {
    log.Fatal(err)
}

// Run migrations on startup (optional)
ctx := context.Background()
if err := migrations.RunMigrationsFromContainer(ctx, container); err != nil {
    log.Fatalf("Migration failed: %v", err)
}
```

## Creating New Migrations

1. Create the migration files with sequential version numbers:
   ```bash
   # Create new migration files
   touch internal/adapters/database/postgres/migrations/000002_add_business_tables.up.sql
   touch internal/adapters/database/postgres/migrations/000002_add_business_tables.down.sql
   ```

2. Add your SQL statements:
   - **Up file**: Contains CREATE, ALTER, INSERT statements
   - **Down file**: Contains DROP, DELETE statements to reverse the up migration

3. Test your migration:
   ```bash
   # Apply migration
   ./bin/migrate -action=up
   
   # Test rollback
   ./bin/migrate -action=down
   ```

## Configuration

The migration service automatically uses your existing database configuration from the container. It integrates seamlessly with your pgxv5 connection pool.

## Best Practices

1. **Always create both up and down migrations** - This allows you to rollback changes if needed
2. **Test migrations in development** before applying to production
3. **Make migrations idempotent** - Use `IF NOT EXISTS` and similar clauses
4. **Keep migrations small and focused** - One logical change per migration
5. **Never edit existing migrations** - Create new migrations to fix issues
6. **Backup your database** before running migrations in production

## Environment Variables

The migration service uses the same database configuration as your application:
- `DB_HOST` - Database host (default: localhost)
- `DB_PORT` - Database port (default: 5432)
- `DB_NAME` - Database name (default: xcentro)
- `DB_USER` - Database username (default: postgres)
- `DB_PASSWORD` - Database password (required)
- `DB_SSL_MODE` - SSL mode (default: disable)

## Troubleshooting

### Dirty State
If migrations are in a "dirty" state:
1. Check what went wrong in the last migration
2. Manually fix the database state
3. Update the schema_migrations table to mark as clean
4. Re-run migrations

### Version Conflicts
- Ensure version numbers are sequential and unique
- Don't skip version numbers
- Use leading zeros for consistent sorting (000001, 000002, etc.)