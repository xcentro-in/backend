# XCentro Backend

A Go-based payment processing platform built with Clean Architecture principles.

## Prerequisites

Before you begin, ensure you have the following installed:

- **Go 1.23+** - [Download](https://go.dev/dl/)
- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
- **Git** - [Install Git](https://git-scm.com/downloads)

Verify installations:
```bash
go version        # Should show go1.23 or higher
docker --version  # Should show Docker version
docker compose version
```

## Quick Start Guide

Complete step-by-step guide to set up XCentro backend for local development:

**Summary of Steps:**
1. Start Docker containers (PostgreSQL + Redis)
2. Setup database users (PostgreSQL & Redis)
3. Configure environment files (.env, ops.env, .env.local)
4. Run database migrations (creates schema)
5. Seed test data (admin user, business, etc.)
6. Install Air for hot reload
7. Start API and Ops Dashboard servers
8. Verify everything is working

**What You'll Get:**
- Main API server running on `http://localhost:8888`
- Ops Dashboard server running on `http://localhost:8889`
- Test admin user: `admin@acme.com` / `Admin@123`
- Sample payment data, cards, vouchers, and invoices
- Hot reload enabled for fast development

---

## Setup Instructions

Follow these steps to get the XCentro backend up and running locally:

### 1. Start Docker Services

Navigate to the Docker deployment directory and start PostgreSQL and Redis services:

```bash
cd deployments/docker
./start.sh
```

This will start the required PostgreSQL and Redis containers.

### 2. Setup Database and Cache Users

Run the setup scripts to configure PostgreSQL and Redis users:

```bash
# Setup PostgreSQL users and permissions
./setup-postgres-users.sh

# Setup Redis users and permissions
./setup-redis-users.sh
```

This creates:
- `xcentro_owner` - Database owner user (for migrations)
- `xcentro_rw` - Read-write user (for application)
- `xcentro_redis` - Redis user

### 3. Environment Configuration

Copy the environment templates and configure your settings:

```bash
cd ../..  # Back to project root
cp .env.example .env
cp ops.env.example ops.env
cp .env.local.example .env.local
```

**Important:** The `.env.local.example` file already contains default passwords that match the Docker setup. Simply copy it:

```bash
# The .env.local.example already has the correct default passwords:
# DB_PASSWORD=RwSecure101!
# DB_MIGRATION_PASSWORD=OwnerSecure456!
# REDIS_PASSWORD=XcentroRedis456!
```

**For production**, generate new secure keys and update `.env.local`:

```bash
# Generate secure keys (optional for local dev, required for production)
openssl rand -base64 32  # Use for JWT_SECRET_KEY
openssl rand -base64 32  # Use for BCRYPT_PEPPER
openssl rand -base64 32  # Use for BANK_ACCOUNT_ENCRYPTION_KEY
openssl rand -base64 32  # Use for OPS_TOKEN_ENCRYPTION_KEY
openssl rand -base64 32  # Use for RATE_LIMITER_HMAC_SECRET
```

The default values in `.env`, `ops.env`, and `.env.local.example` are already configured to work with the Docker setup.

### 4. Run Database Migrations

Execute the database migrations to set up the schema:

```bash
# From project root
go run ./cmd/migrate

# Or with explicit -action flag
go run ./cmd/migrate -action=up

# Or build and run
go build -o bin/migrate ./cmd/migrate
./bin/migrate -action=up
```

This will create all necessary tables and schema in your PostgreSQL database. You should see output like:
```
Migration completed successfully to version: 59
```

### 5. Seed Test Data

After migrations are complete, seed the database with test data:

```bash
cd deployments/docker
./seed-test-data.sh
```

**Test Data Created:**
- **Admin User**: `admin@acme.com` / `Admin@123`
- **Business**: Xcento's Team (KYC verified)
- **Ops User**: Dev User (for dev mode, UUID: `00000000-0000-0000-0000-000000000001`)
- **Vendor Payee**: Nexa Digital Solutions (with bank account)
- **Stored Cards**: 2 tokenized cards (1 failed, 1 active)
- **Payments**: 3 payment records with state changes
- **Voucher**: 1 voucher linked to payment
- **Invoices**: 2 GST invoices

### 6. Install Air (Hot Reload)

Install Air for automatic code reloading during development:

```bash
go install github.com/air-verse/air@latest

# Add Go bin to PATH (choose based on your shell)
# For Bash:
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# For Zsh:
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc

# Verify Air is installed
air -v
```

### 7. Start the Development Servers

Run the application with hot reload using Air:

```bash
# Terminal 1: Main API server (port 8888, uses .env + .env.local)
air

# Terminal 2: Ops Dashboard server (port 8889, uses ops.env + .env.local)
air -c .air.ops.toml
```

**What happens when you run Air:**
- Air watches all `.go` files for changes
- Automatically rebuilds and restarts the server on file changes
- Displays build errors and logs in real-time
- Main API server starts on `http://0.0.0.0:8888`
- Ops Dashboard server starts on `http://0.0.0.0:8889`

**Environment File Usage:**
- **Main API** (`air`): Loads `.env` + `.env.local` (secrets)
- **Ops Dashboard** (`air -c .air.ops.toml`): Loads `ops.env` + `.env.local` (secrets)
- Both servers share the same secrets from `.env.local`

### 8. Verify the Servers are Running

Check if both servers are healthy:

```bash
# Main API server health check
curl http://localhost:8888/health
# Expected: {"status":"ok"}

# Ops Dashboard server health check
curl http://localhost:8889/health
# Expected: {"status":"ok"}
```

You should see logs in your terminal indicating both servers started successfully.

**Test the seeded data:**

```bash
# Login with the seeded admin user
curl -X POST http://localhost:8888/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@acme.com","password":"Admin@123"}'

# Expected: Access and refresh tokens in response
```

---

## Development Workflow

### Hot Reload with Air

Air is configured to automatically:
- Watch all `.go` files for changes
- Rebuild the binary when changes are detected
- Restart the server with the new binary
- Display build errors and logs in real-time

Configuration files:
- [.air.toml](.air.toml) - API server (uses `.env`)
- [.air.ops.toml](.air.ops.toml) - Ops dashboard server (uses `ops.env`)
- [.air.worker.toml](.air.worker.toml) - Notification worker (uses `.env`)

```bash
# Main API server (port 8888)
air

# Ops Dashboard server (port 8889, separate environment)
air -c .air.ops.toml

# Notification worker
air -c .air.worker.toml
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run tests with verbose output
go test ./... -v

# Run tests for a specific package
go test ./internal/services/...
```

### Manual Build & Run (without Air)

If you prefer to build and run manually:

```bash
# Build the API server
go build -o bin/api ./cmd/api

# Run with default .env file
./bin/api

# Run with custom environment file
./bin/api -env=.env.staging
./bin/api -env=.env.production

# Or use go run directly
go run ./cmd/api
go run ./cmd/api -env=.env.staging

# Build and run ops server (uses ops.env by default with Air)
go build -o bin/ops ./cmd/ops
./bin/ops -env=ops.env
./bin/ops -env=.env.production  # Production deployment

# Build and run workers
go build -o bin/notification_worker ./cmd/workers/notification_worker
./bin/notification_worker
./bin/notification_worker -env=.env.production

go build -o bin/payout_worker ./cmd/workers/payout_worker
./bin/payout_worker
./bin/payout_worker -env=.env.staging
```

### Database Migrations

```bash
# Build migration tool
go build -o bin/migrate ./cmd/migrate

# Run migrations with default .env
./bin/migrate -action=up

# Run migrations with custom environment file
./bin/migrate -action=up -env=.env.staging
./bin/migrate -action=up -env=.env.production

# Rollback migrations
./bin/migrate -action=down -env=.env.staging

# Check migration status
./bin/migrate -action=version -env=.env.production

# Or use go run directly
go run ./cmd/migrate -action=up -env=.env.staging
```

### Applying Seed Data (Testing)

Load test data for development:

```bash
cd internal/adapters/database/postgres/migrations/seeds/

# List available seeds
./apply-seeds.sh list

# Apply all seeds
./apply-seeds.sh apply-all

# Apply specific seed (e.g., approval workflow)
./apply-seeds.sh apply 005

# Rollback specific seed
./apply-seeds.sh rollback 005
```

---

## Troubleshooting

### Issue: "Cannot connect to PostgreSQL"

**Solution:**
```bash
# Check if PostgreSQL container is running
docker ps | grep postgres

# View PostgreSQL logs
cd deployments/docker
docker compose logs postgres

# Restart PostgreSQL
docker compose restart postgres
```

### Issue: "Cannot connect to Redis"

**Solution:**
```bash
# Check if Redis container is running
docker ps | grep redis

# View Redis logs
docker compose logs redis

# Restart Redis
docker compose restart redis
```

### Issue: "Air command not found"

**Solution:**
```bash
# Reinstall Air
go install github.com/air-verse/air@latest

# Add GOPATH/bin to your PATH (Linux/Mac)
export PATH=$PATH:$(go env GOPATH)/bin

# For permanent fix, add to ~/.bashrc or ~/.zshrc:
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### Issue: "Migration failed"

**Solution:**
```bash
# Check current migration version
./bin/migrate -action=version

# Force migration version (use with caution)
./bin/migrate -action=force -version=X

# Rollback and retry
./bin/migrate -action=down
./bin/migrate -action=up
```

### Issue: Port 8888 already in use

**Solution:**
```bash
# Find process using port 8888
lsof -i :8888

# Kill the process (replace PID with actual process ID)
kill -9 <PID>

# Or change the port in .env file
echo "PORT=8889" >> .env
```

---

## Development Notes

- **Architecture**: Clean Architecture with layers (handlers → services → core → adapters)
- **Module Path**: `xcentro.in`
- **Database**: PostgreSQL 18 with pgx/v5 driver
- **Cache**: Redis v9 for session management and caching
- **Framework**: Echo v4 for HTTP routing
- **Environment**: Variables loaded from `.env` file automatically
- **Air Config**: [.air.toml](.air.toml) watches all Go files and rebuilds on changes
- **Logs**: Air displays build errors and application logs in real-time

### Project Structure

```
backend/
├── cmd/                    # Entry points
│   ├── api/               # API server
│   └── migrate/           # Database migrations
├── internal/
│   ├── core/              # Domain entities & interfaces
│   ├── services/          # Business logic orchestration
│   ├── handlers/          # HTTP handlers
│   ├── middleware/        # Reusable middleware
│   ├── adapters/          # External integrations
│   └── shared/            # Common utilities
├── migrations/sql/        # Database migrations
├── deployments/docker/    # Docker Compose setup
└── .air.toml             # Hot reload configuration
```

For detailed documentation, see:
- [CLAUDE.md](CLAUDE.md) - Project overview and conventions
- [API_REFERENCE.md](API_REFERENCE.md) - Complete API documentation
- [docs/](docs/) - Detailed module documentation

---

## Stopping Services

To stop the Docker services when done:

```bash
cd deployments/docker
docker compose down

# To also remove volumes (⚠️ deletes all data)
docker compose down -v
```