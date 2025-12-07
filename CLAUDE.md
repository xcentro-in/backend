# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

XCentro backend is a Go-based payment processing platform using Clean Architecture principles. Built as a single binary with multiple entry points for independent scaling.

**Module Path**: `xcentro.in`

## Quick Commands

```bash
# Build & Run
go build -o bin/api ./cmd/api && go run ./cmd/api
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up

# Database Setup
cd deployments/docker && ./start.sh && ./setup-postgres-users.sh && ./setup-redis-users.sh

# Testing
go test ./... -cover
```

## Architecture

### Clean Architecture Layers
- **`internal/core/`**: Domain entities, business logic, repository interfaces
- **`internal/adapters/`**: Database, cache, external APIs, payment gateways
- **`internal/handlers/`**: HTTP handlers and middleware
- **`internal/middleware/`**: Reusable middleware (JWT, business roles with Redis caching)
- **`internal/services/`**: Application orchestration layer
- **`internal/shared/`**: Common utilities and configuration
- **`cmd/`**: Entry points (api, migrate)

### Data Flow
HTTP → handlers → services → core → repository interface → adapters

### Key Domains
- **User**: Authentication, KYC (`/v1/auth`, `/v1/user`)
- **Business**: Business entity management, KYB (`/v1/business`)
- **Wallet**: Payment source management (`/v1/wallet`)
- **Payment**: Payment processing with gateway orchestration (`/v1/process_payment`)
- **Invoice**: GST-compliant invoice generation (`/v1/invoices`, `/ops/v1/invoices`)
- **Payee**: Payee onboarding with verification (`/v1/payees`)
- **Voucher**: Multi-bill payments (`/v1/vouchers`)
- **Payout**: Vendor payouts (`/v1/payout`)

## Technology Stack

- **Framework**: Echo v4
- **Database**: PostgreSQL 18 (pgx/v5), Redis v9
- **Migrations**: golang-migrate/migrate
- **Auth**: JWT (golang-jwt/jwt/v5), bcrypt
- **Cloud**: AWS SES, AWS S3
- **Payment Gateways**: JioPay, Easebuzz
- **UUID**: PostgreSQL UUIDv7 function

## Environment Variables

See `.env.example` for full list. Key variables:
- `DB_PASSWORD`, `DB_MIGRATION_PASSWORD`, `JWT_SECRET_KEY`
- `JIOPAY_*` - JioPay gateway credentials
- `EASEBUZZ_*` - Easebuzz gateway credentials
- `AWS_*` - AWS SES and S3 credentials
- `AUTHBRIDGE_*` - Document verification credentials

## Core Concepts

### User Management
- **Tables**: `user_table`, `user_identity`, `user_verification_codes`
- **Auth Flow**: Signup → Email Verification → Login (JWT) → Password Reset
- **KYC**: PAN + Aadhaar verification via DigiLocker
- **Roles**: Owner, Admin, Finance, Support

### Business Management
- **Tables**: `business`, `kyb_data`, `business_user_relation`
- **Flow**: Create Business → KYB → User Invites → Role Management
- **Authorization**: Admin cannot change/remove owner

### Payment System
- **Gateway Orchestrator**: Auto-routing based on payment method and card type
  - **JioPay**: Net banking, Visa/MC/RuPay credit, corporate cards
  - **Easebuzz**: Amex/Diners, Visa/MC debit (< ₹2000)
- **Tokenization**: PCI DSS compliant card storage with gateway tokens
- **Settlement**: T+1 to T+3 days, tracks MDR and net settlement
- **Webhooks**: Payment status (immediate) + Settlement status (batch/single)

### Payee Management
- **Three-table design**: `payee_business` (global) + `payee_bank_accounts` + `business_payee_relation`
- **Verification**: GSTIN check, penny drop, cross-verification
- **Anti-fraud**: Name matching, anti-money rotation checks

### Voucher & Payout System
- **Voucher Flow**: Draft → Finalized (on payment success) → Posted → Synced
- **Multi-bill**: Single payment can link to multiple vouchers
- **Payout**: Auto-triggered after settlement, tracks individual UTRs per payee

### Invoice System
- **Table**: `payment_invoices` with GST compliance fields
- **Auto-generation**: Created automatically on payment success (bidirectional)
- **Endpoints**: Both callback (`/v1/process_payment/callback`) and webhook (`/v1/process_payment/webhooks/payment_status`) can create invoices
- **Flow**: Whichever endpoint processes payment first creates invoice, second one verifies
- **IRN**: SHA256-based Invoice Reference Number for GST e-invoicing
- **PDF Generation**: GST-compliant invoice PDF with QR code support
- **User Routes**: `/v1/invoices/download` (PDF/JSON by invoice_id or payment_id)
- **Ops Routes**: `/ops/v1/invoices` (paginated list with filters), `/ops/v1/invoices/export`
- **Verification**: Automatic validation of GSTIN, amounts (taxable, GST, total) with 0.01 tolerance
- **Error Handling**: Non-fatal - invoice errors don't affect payment success

## Development Workflow

### Adding Features
1. Start with domain entities in `internal/core/domain/`
2. Add repository interfaces in `internal/core/repositories/`
3. Implement adapters in `internal/adapters/`
4. Create service orchestration in `internal/services/`
5. Add HTTP handlers in `internal/handlers/`

### Database Migrations
1. Create sequential files in `migrations/sql/` (000001, 000002)
2. Always create `.up.sql` and `.down.sql`
3. Test with `./bin/migrate -action=up` and `-action=down`

### Database Seed Data (Testing)
**Location**: `internal/adapters/database/postgres/migrations/seeds/`

Seed files provide test data for development and testing. Follow this workflow:

1. **Create Seed File**: `NNN_seed_description.sql` (e.g., `005_seed_approval_workflow.sql`)
2. **Create Rollback**: `NNN_seed_description_rollback.sql`
3. **Apply Seeds**: Use the helper script
   ```bash
   cd internal/adapters/database/postgres/migrations/seeds/
   ./apply-seeds.sh apply 005        # Apply specific seed
   ./apply-seeds.sh apply-all         # Apply all seeds
   ./apply-seeds.sh rollback 005      # Rollback specific seed
   ./apply-seeds.sh list              # List available seeds
   ```

**Available Seeds**:
- `001` - Wallet test data (stored cards, bank accounts)
- `002` - Vendor payees (3 vendors with bank accounts)
- `003` - Employee/biller payees
- `004` - Voucher with card payment
- `005` - Approval workflow (14 vouchers with various approval states)

**Naming Convention**: Sequential numbering with descriptive names. Always provide rollback files for cleanup.

### Adding New Payment Gateway
1. Create client in `internal/adapters/external/payment/newgateway/`
2. Implement `PaymentGateway` interface
3. Update `factory/gateway_factory.go`
4. Update routing in `payment_orchestrator.go`
5. Add environment variables

## Important Notes

### Amount Handling
- All amounts in smallest unit (paise for INR)
- Example: ₹500.00 = 50000 paise
- Gateways expect decimal strings: "500.00"

### PostgreSQL Timestamp Handling
- Use `::text` to cast timestamps to string when scanning into Go string fields
- Use `COALESCE(column::text, '')` for nullable timestamp columns
- Example: `SELECT added_at::text, COALESCE(updated_at::text, '') FROM table`

### Security
- **PCI DSS**: Cards never in plaintext, CVV transient only
- **Bank Accounts**: Encrypted at rest, masked in responses
- **JWT**: Required for all protected endpoints
- **Presigned URLs**: Short expiry (15 min)
- **Webhook**: Signature verification (HMAC-SHA256, SHA-512)

### Calling Device & Callback URLs
- Backend controls callback URLs based on `calling_device` field (web/mobile/pos)
- Prevents redirect vulnerabilities
- Default: `web`

## Documentation References

For detailed information, see:
- **`API_REFERENCE.md`** - Complete API endpoint documentation
- **`EXAMPLES.md`** - Code examples and integration flows
- **`GATEWAY_INTEGRATION.md`** - Payment gateway technical details
- **`docs/payment_module.md`** - Payment system architecture
- **`docs/frontend_invoice_integration.md`** - GST invoice system integration guide for frontend
- **`docs/business_module.md`** - Business entity management
- **`docs/payee_module.md`** - Payee management
- **`docs/migrations.md`** - Migration guide
- **`docs/FOLDER_STRUCTURE.md`** - Project structure
