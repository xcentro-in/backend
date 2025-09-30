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
- **Business Rules**: Cross-cutting business logic (fee calculation, wallet management, bank verification) (`internal/core/business_rules/`)

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
- Business rules: `internal/core/business_rules/`
  - Fee calculator: `fee_calculator.go`
  - Wallet service: `wallet_service.go`
  - Bank verification: `bank_verification_service.go`, `mock_bank_verification.go`

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
All endpoints are under `/api/v1/process_payment/` prefix:

```bash
# Core payment operations
POST /api/v1/process_payment/fetch_wallet        # Get business wallet with all payment sources
POST /api/v1/process_payment/get_fee_rules       # Get fee calculation rules
POST /api/v1/process_payment/make_payment        # Process payment (automatic gateway routing)
POST /api/v1/process_payment/tokenize            # Tokenize card (PCI DSS compliant)
POST /api/v1/process_payment/add_payment_source  # Add payment source (bank account, UPI)

# Webhook endpoints
POST /api/v1/process_payment/webhooks/jiopay     # JioPay webhook handler
POST /api/v1/process_payment/webhooks/easebuzz   # Easebuzz webhook handler
```

### Payment Methods Supported
1. **Card Payments**:
   - Direct card payment (one-time with card details)
   - Token-based payment (using stored card token)
   - Card types: Visa, MasterCard, RuPay, Amex, Diners
   - Card categories: Credit, Debit, Corporate

2. **Net Banking**:
   - Multiple bank support via bank codes
   - Automatically routed to JioPay gateway

3. **UPI**:
   - VPA-based payments
   - Real-time processing

4. **Wallet**:
   - Digital wallet integration
   - Multiple wallet providers

### Wallet Management System
The wallet system manages all payment sources for a business:

**Payment Sources Supported**:
- **Tokenized Cards**: Securely stored card tokens from gateways
- **Bank Accounts**: Verified via penny drop with detailed metadata
- **UPI Details**: UPI VPA addresses (future implementation)
- **Credit Lines**: Credit facilities (future implementation)

**Wallet Service Features**:
- Interface-based design (`WalletService` in `business_rules/wallet_service.go`)
- Mock implementation for development and testing
- In-memory storage (to be replaced with database)
- Automatic duplicate detection
- Payment source validation
- Metadata tracking for audit compliance

### Bank Account Verification System
Comprehensive penny drop verification for bank accounts:

**Verification Flow**:
1. Validate bank account details (account number, IFSC, holder name)
2. Check for duplicate accounts per business
3. Perform penny drop verification via 3rd party API (simulated)
4. Store verification metadata (status, date, reference, amount)
5. Add verified account to business wallet

**Mock Verification Service** (`mock_bank_verification.go`):
- Simulates realistic 2-second API delay
- Pattern-based testing for different scenarios
- Test patterns (based on last 4 digits of account number):
  - `****0000`: Account closed (ACCOUNT_CLOSED error)
  - `****1111`: Invalid account (INVALID_ACCOUNT error)
  - `****2222`: Name mismatch (NAME_MISMATCH error)
  - `****9999`: API timeout (API_TIMEOUT error)
  - Other numbers: Successful verification

**Bank Verification Interface** (`bank_verification_service.go`):
- `BankVerificationService` interface for production implementation
- `BankVerificationRequest` and `BankVerificationResponse` structures
- Error types: `BankVerificationError` with specific error codes
- Provider support: Multiple provider types (Razorpay, Cashfree, etc.)

### Fee Calculator System
Flexible fee calculation for different payment methods:

**Fee Rules**:
- Percentage-based fees (e.g., 2% for cards)
- Fixed fees (e.g., ₹10 per transaction)
- Min/max fee enforcement
- Card type and category-based differentiation
- Multiple currency support

**Fee Calculator Interface** (`fee_calculator.go`):
- `FeeCalculator` interface for custom implementations
- `MockFeeCalculator` with realistic fee structures
- Method-specific rules (card, net banking, UPI, wallet)
- GST handling for Indian market

### Adding New Payment Gateways
1. Create gateway client in `internal/adapters/external/payment/newgateway/`
2. Implement `PaymentGateway` interface with required methods:
   - `GetGatewayType()` - Return gateway identifier
   - `ProcessPayment()` - Process payment transaction
   - `TokenizeCard()` - Tokenize card for future use
   - `GetPaymentStatus()` - Query payment status
   - `RefundPayment()` - Process refunds
3. Add gateway configuration to `factory/gateway_factory.go`
4. Update routing rules in `payment_orchestrator.go` (in `services/payments/`)
5. Add gateway constants to `payment_entity.go`
6. Add gateway environment variables to configuration

### Payment Error Handling
The system includes comprehensive error handling with:
- **Retry logic**: Exponential backoff for transient failures
- **Gateway failover**: Automatic switching to backup gateway
- **Error categorization**: Specific error codes for different failure types
  - Payment errors: INVALID_CARD, EXPIRED_CARD, INSUFFICIENT_FUNDS, GATEWAY_ERROR
  - Bank verification errors: ACCOUNT_CLOSED, INVALID_ACCOUNT, NAME_MISMATCH, API_TIMEOUT
  - Validation errors: validation_failed, invalid_request, unsupported_source_type
- **Audit trails**: Payment attempt tracking for debugging and compliance

### Security Best Practices
**PCI DSS Compliance**:
- Cards never stored in plain text
- Card details only used in tokenization flow
- CVV never stored (transient only)
- Tokenization uses gateway-provided secure tokens
- `/tokenize` endpoint is separate from general payment source addition

**Bank Account Security**:
- Account numbers masked (show last 4 digits only)
- Verification metadata encrypted at rest
- Penny drop amounts randomized
- Audit trail for all verification attempts

**API Security**:
- JWT authentication required for all endpoints
- Business ID validation from JWT token
- Request rate limiting
- Input sanitization and validation
- Webhook signature verification

## Testing Strategy

### Payment System Testing
**Mock Implementations Available**:
- `MockPaymentOrchestrator` - For testing payment flows
- `MockFeeCalculator` - For testing fee calculations
- `MockWalletService` - For testing wallet operations
- `MockBankVerificationService` - For testing bank account verification

**Bank Verification Testing Patterns**:
Use specific account number endings to test different scenarios:
```go
// Success case
accountNumber := "1234567890123456"  // Any number not matching failure patterns

// Failure cases (based on last 4 digits)
accountNumber := "1234567890120000"  // Account closed (ACCOUNT_CLOSED)
accountNumber := "1234567890121111"  // Invalid account (INVALID_ACCOUNT)
accountNumber := "1234567890122222"  // Name mismatch (NAME_MISMATCH)
accountNumber := "1234567890129999"  // API timeout (API_TIMEOUT)
```

### Integration Testing
```bash
# Run payment integration tests
go test ./internal/services/payments/... -v

# Run gateway adapter tests
go test ./internal/adapters/external/payment/jiopay/... -v
go test ./internal/adapters/external/payment/easebuzz/... -v

# Run bank verification tests
go test ./internal/core/business_rules/... -v -run TestBankVerification
```

## Documentation

For detailed information, see:
- **Payment Module**: `docs/payment_module.md` - Complete payment system documentation
- **Business Module**: `docs/business_module.md` - Business entity management
- **Migrations**: `docs/migrations.md` - Database migration guide
- **Folder Structure**: `docs/FOLDER_STRUCTURE.md` - Project architecture guide

## Module Path
All imports use the module path: `xcentro.in`

Example imports:
```go
import (
    "xcentro.in/internal/core/user"
    "xcentro.in/internal/core/payment"
    "xcentro.in/internal/core/business_rules"
    "xcentro.in/internal/shared/config"
    "xcentro.in/internal/adapters/database/postgres/client"
    "xcentro.in/internal/adapters/external/payment/jiopay"
    "xcentro.in/internal/adapters/external/payment/easebuzz"
    "xcentro.in/internal/services/payments"
)
```

## Quick Reference: Common Tasks

### Testing Payment Flow Locally
1. Start the API server: `go run ./cmd/api`
2. Use Postman or curl to test endpoints
3. Check console logs for detailed payment flow traces

### Example: Add Bank Account via API
```bash
curl -X POST http://localhost:8080/api/v1/process_payment/add_payment_source \
  -H "Content-Type: application/json" \
  -d '{
    "business_id": "test-business-123",
    "source_type": "bank_account",
    "bank_account_details": {
      "account_number": "1234567890123456",
      "ifsc_code": "HDFC0000123",
      "account_holder_name": "John Doe",
      "bank_name": "HDFC Bank",
      "account_type": "savings"
    }
  }'
```

### Example: Tokenize Card via API
```bash
curl -X POST http://localhost:8080/api/v1/process_payment/tokenize \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "business-uuid-123",
    "card_details": {
      "card_number": "4111111111111111",
      "expiry_month": "12",
      "expiry_year": "2025",
      "cvv": "123",
      "holder_name": "John Doe",
      "card_type": "credit",
      "card_category": "visa"
    }
  }'
```

### Example: Make Payment via API
```bash
curl -X POST http://localhost:8080/api/v1/process_payment/make_payment \
  -H "Content-Type: application/json" \
  -d '{
    "amount": 50000,
    "currency": "INR",
    "payment_method": "card",
    "description": "Test payment",
    "card_details": {
      "card_number": "5555555555554444",
      "expiry_month": "12",
      "expiry_year": "2025",
      "cvv": "123",
      "holder_name": "Jane Smith",
      "card_type": "credit",
      "card_category": "mastercard"
    },
    "callback_url": "https://example.com/callback"
  }'
```

## Important Notes for Development

### Payment Gateway Routing
- Net Banking → Always JioPay
- Amex/Diners → Always Easebuzz
- Master/Visa/RuPay Credit → JioPay
- Master/Visa Debit → Easebuzz
- Routing is automatic based on payment method and card details

### Wallet vs Tokenization
- **Tokenization** (`/tokenize`): PCI DSS compliant card storage, returns gateway token
- **Add Payment Source** (`/add_payment_source`): General endpoint for non-card payment sources
- Cards are added to wallet automatically after tokenization
- Bank accounts require penny drop verification before being added to wallet
- UPI accounts will require verification in future implementation

### Context and Timeouts
- Payment operations: 30 second timeout
- Tokenization operations: 60 second timeout (JioPay 2-step process)
- Bank verification: 30 second timeout (includes 2-second API simulation)
- Always propagate context from HTTP handler to services

### Amount Handling
- All amounts in smallest currency unit (paise for INR)
- Example: ₹500.00 = 50000 paise
- Fee calculations return amounts in same unit
- Gateway APIs may have different conventions (check adapter implementations)