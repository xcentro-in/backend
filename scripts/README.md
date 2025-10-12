# Scripts

This directory contains utility scripts for development and operations tasks.

## Available Scripts

### migrate.sh

Database migration helper script that ensures proper workflow to avoid cached plan errors.

**Features:**
- Checks if API server is running before migrations
- Warns about potential cached plan errors
- Provides clear post-migration instructions
- Supports all migration actions (up, down, version, goto, force, drop)

**Usage:**

```bash
# Run all pending migrations
./scripts/migrate.sh -a up

# Rollback one migration
./scripts/migrate.sh -a down

# Check current migration version
./scripts/migrate.sh -a version

# Migrate to specific version
./scripts/migrate.sh -a goto -v 5

# Force migration version (fix dirty state)
./scripts/migrate.sh -a force -v 3

# Skip confirmation prompt
./scripts/migrate.sh -a up -y

# Show help
./scripts/migrate.sh -h
```

**Best Practice Workflow:**

```bash
# 1. Stop your API server (if running)
pkill -f "go run cmd/api/http_server.go" || pkill -f "http_server"

# 2. Run migrations
./scripts/migrate.sh -a up

# 3. Start your API server
go run cmd/api/http_server.go
```

## Adding New Scripts

When adding new scripts to this directory:

1. Make scripts executable: `chmod +x scripts/your_script.sh`
2. Add proper documentation and help messages
3. Use colored output for better UX (see migrate.sh for example)
4. Handle errors gracefully with proper exit codes
5. Update this README with usage instructions

