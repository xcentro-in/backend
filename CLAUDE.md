# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

XCentro backend is a Go-based application using Clean Architecture principles. It's built as a single binary with multiple entry points, allowing different services to share the same business logic while scaling independently.

## Development Commands

### Building and Running
```bash
# Build specific services
go build -o bin/api ./cmd/api
go build -o bin/migrate ./cmd/migrate

# Run the API server
go run ./cmd/api

# Run database migrations
go build -o bin/migrate ./cmd/migrate
./bin/migrate -action=up
```

### Database Management
```bash
# Start database services (PostgreSQL + Redis)
cd deployments/docker && ./start.sh

# Set up database users
cd deployments/docker && ./setup-postgres-users.sh
cd deployments/docker && ./setup-redis-users.sh

# Connect to databases
cd deployments/docker && ./connect-postgres.sh rw    # For app development
cd deployments/docker && ./connect-postgres.sh admin # For migrations
cd deployments/docker && ./connect-redis.sh app
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run integration tests specifically
go test ./test/integration/...
```

### Code Quality
```bash
# Format code
go fmt ./...

# Run static analysis
go vet ./...

# Tidy dependencies
go mod tidy
```

## Architecture Overview

### Clean Architecture Structure
- **`internal/core/`**: Business domain logic (entities, services, repository interfaces)
- **`internal/adapters/`**: External system integrations (database, cache, queues, APIs)
- **`internal/handlers/`**: HTTP request handlers and middleware
- **`internal/services/`**: Application services that orchestrate core domains
- **`internal/workers/`**: Background job processors
- **`internal/shared/`**: Common utilities and configuration
- **`cmd/`**: Entry points for different executables

### Key Domains
- **User**: Authentication, profiles, permissions (`internal/core/user/`)
- **Auth**: Authentication services (`internal/core/auth/`)
- **Business**: Business entity management (`internal/core/business/`)
- **Payment**: Payment processing with gateway orchestration (`internal/core/payment/`)

### Data Flow Pattern
1. HTTP Request → `internal/handlers/http/`
2. Handler → `internal/services/` (orchestration)
3. Service → `internal/core/` (business logic)
4. Core → Repository interface
5. Repository → `internal/adapters/database/postgres/` (implementation)

### Multiple Entry Points
- **`cmd/api/`**: REST API server (primary application)
- **`cmd/migrate/`**: Database migration tool

## Database Configuration

### Environment Variables
Required environment variables:
```bash
# Database (required)
DB_PASSWORD=your_db_password
DB_MIGRATION_PASSWORD=your_migration_password

# JWT (required)
JWT_SECRET_KEY=your_jwt_secret

# Optional with defaults
DB_HOST=localhost
DB_PORT=5432
DB_NAME=xcentro
DB_USER=xcentro_rw
DB_MIGRATION_USER=xcentro_admin
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USERNAME=xcentro_redis
REDIS_PASSWORD=your_redis_password
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
```

### Database Users
- **`xcentro_admin`**: For migrations (DDL operations)
- **`xcentro_rw`**: For application runtime (DML operations)
- **`xcentro_owner`**: Database owner (administrative tasks)

## Key Technologies

- **Framework**: Echo v4 (HTTP server)
- **Database**: PostgreSQL 18 with pgx/v5 driver
- **Cache**: Redis v9
- **Migrations**: golang-migrate/migrate
- **Config**: cleanenv (supports .env files)
- **JWT**: golang-jwt/jwt/v5
- **UUID**: Native PostgreSQL UUIDv7 function

## File Locations

### Configuration
- Application config: `internal/shared/config/config.go`
- Environment variables: `.env` file or system environment

### Database
- Migrations: `internal/adapters/database/postgres/migrations/sql/`
- Migration tool: `cmd/migrate/main.go`
- Database clients: `internal/adapters/database/postgres/client/`

### Core Business Logic
- User domain: `internal/core/user/`
- Auth domain: `internal/core/auth/`
- Business domain: `internal/core/business/`
- Payment domain: `internal/core/payment/`

### API Layer
- HTTP handlers: `internal/handlers/http/`
- Routes: `cmd/api/routes.go`
- Server setup: `cmd/api/http_server.go`

### Services
- User service: `internal/services/user/`
- Auth service: `internal/services/auth/`
- Payment service: `internal/services/payments/`

### Payment Gateway Adapters
- JioPay gateway: `internal/adapters/external/payment/jiopay/`
- Easebuzz gateway: `internal/adapters/external/payment/easebuzz/`
- Gateway factory: `internal/adapters/external/payment/factory/`

## Development Workflow

1. **Local Development Setup**:
   ```bash
   # Start databases
   cd deployments/docker && ./start.sh && ./setup-postgres-users.sh && ./setup-redis-users.sh

   # Set up environment
   cp .env.example .env  # Edit with your values

   # Run migrations
   go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up

   # Start API server
   go run ./cmd/api
   ```

2. **Adding New Features**:
   - Start with business logic in `internal/core/domain/`
   - Add repository interfaces in core
   - Implement adapters in `internal/adapters/`
   - Create service orchestration in `internal/services/`
   - Add HTTP handlers in `internal/handlers/http/`

3. **Database Changes**:
   - Create new migration files in `internal/adapters/database/postgres/migrations/sql/`
   - Use sequential numbering (000001, 000002, etc.)
   - Always create both `.up.sql` and `.down.sql` files
   - Test migrations with `./bin/migrate -action=up` and `./bin/migrate -action=down`

## Payment System Architecture

### Gateway Orchestrator Pattern
The payment system uses a sophisticated orchestrator pattern that automatically routes payments to appropriate gateways based on payment method and card details:

**JioPay Gateway Routes:**
- Net Banking (all banks)
- Master/Visa Credit Cards
- RuPay Credit Cards
- Corporate Credit Cards (all types)

**Easebuzz Gateway Routes:**
- Amex Cards (all categories)
- Diners Cards (all categories)
- Master/Visa Debit Cards

### Payment Environment Variables
```bash
# JioPay Configuration (required)
JIOPAY_BASE_URL=https://api.jiopay.com
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key
JIOPAY_SECRET_KEY=your_secret_key
JIOPAY_ENVIRONMENT=sandbox  # or production

# Easebuzz Configuration (required)
EASEBUZZ_BASE_URL=https://testpay.easebuzz.in
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt_key
EASEBUZZ_ENVIRONMENT=test  # or prod
```

### Payment Database Schema
The payment system requires these database tables:
- `payments` - Main payment records with gateway routing info
- `payment_attempts` - Retry attempt tracking for debugging
- `tokenized_cards` - Secure card token storage for future payments

### Payment API Endpoints
```bash
# Core payment operations
POST /api/payments           # Create new payment
GET  /api/payments/:id       # Get payment status
POST /api/payments/:id/refund # Process refund

# Card tokenization
POST /api/payments/tokenize  # Tokenize card for future use
GET  /api/payments/cards     # Get user's tokenized cards
DELETE /api/payments/cards/:id # Deactivate tokenized card

# Analytics and management
GET  /api/payments           # List user payments (paginated)
GET  /api/payments/stats     # Payment statistics
GET  /api/payments/methods   # Supported payment methods
POST /api/webhooks/:gateway  # Gateway webhook handling
```

### Adding New Payment Gateways
1. Create gateway client in `internal/adapters/external/payment/newgateway/`
2. Implement `PaymentGateway` interface with required methods
3. Add gateway configuration to `factory/gateway_factory.go`
4. Update routing rules in `payment_orchestrator.go`
5. Add gateway constants to `payment_entity.go`

### Payment Error Handling
The system includes comprehensive error handling with:
- Retry logic with exponential backoff
- Gateway failover capabilities
- Detailed error categorization
- Payment attempt audit trails

## Module Path
All imports use the module path: `xcentro.in`

Example imports:
```go
import (
    "xcentro.in/internal/core/user"
    "xcentro.in/internal/core/payment"
    "xcentro.in/internal/shared/config"
    "xcentro.in/internal/adapters/database/postgres/client"
    "xcentro.in/internal/adapters/external/payment/jiopay"
    "xcentro.in/internal/adapters/external/payment/easebuzz"
)
```