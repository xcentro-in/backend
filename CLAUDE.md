# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

XCentro backend is a Go-based payment processing platform using Clean Architecture principles. Built as a single binary with multiple entry points for independent scaling.

**Module Path**: `xcentro.in`

## Development Commands

```bash
# Build & Run
go build -o bin/api ./cmd/api && go run ./cmd/api
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up

# Database Setup
cd deployments/docker && ./start.sh && ./setup-postgres-users.sh && ./setup-redis-users.sh

# Testing
go test ./... -cover
go test ./test/integration/...

# Code Quality
go fmt ./... && go vet ./... && go mod tidy
```

## Architecture

### Clean Architecture Layers
- **`internal/core/`**: Domain entities, business logic, repository interfaces
- **`internal/adapters/`**: Database, cache, external APIs, payment gateways
- **`internal/handlers/`**: HTTP handlers and middleware
- **`internal/services/`**: Application orchestration layer
- **`internal/shared/`**: Common utilities and configuration
- **`cmd/`**: Entry points (api, migrate)

### Data Flow
HTTP → handlers → services → core → repository interface → adapters

### Key Domains
- **User**: Authentication, KYC (`/v1/auth`, `/v1/user`)
- **Business**: Business entity management, KYB (`/v1/business`)
- **Payment**: Payment processing with gateway orchestration (`/api/v1/process_payment`)
- **Payee**: Payee onboarding with verification (`/v1/payees`)
- **Business Rules**: Fee calculation, wallet, bank/GST verification

## Technology Stack

- **Framework**: Echo v4
- **Database**: PostgreSQL 18 (pgx/v5), Redis v9
- **Migrations**: golang-migrate/migrate
- **Auth**: JWT (golang-jwt/jwt/v5), bcrypt
- **Cloud**: AWS SES, AWS S3
- **UUID**: PostgreSQL UUIDv7 function

## Environment Variables

```bash
# Database (required)
DB_PASSWORD=your_db_password
DB_MIGRATION_PASSWORD=your_migration_password
JWT_SECRET_KEY=your_jwt_secret

# Database users
DB_USER=xcentro_rw              # Application runtime (DML)
DB_MIGRATION_USER=xcentro_admin # Migrations (DDL)

# Payment Gateways
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt_key

# AWS Services
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
S3_BUCKET_NAME=xcentro-pii-documents
SES_DEFAULT_FROM_EMAIL=noreply@xcentro.com

# Third-Party Integrations
AUTHBRIDGE_USERNAME=your_username
AUTHBRIDGE_PASSWORD=your_password
```

## User Management

### Database Schema
- **`user_table`**: Main user accounts (email, password_hash, name, dob, verification status, KYC status, kyc_needed)
- **`user_identity`**: Externalized tax_id/national_id data (supports family members with same identity)
- **`user_verification_codes`**: Verification codes (email_verification, mobile_verification, password_reset) with expiry

### Authentication Flow
1. **Signup** (`POST /v1/auth/signup`): bcrypt hashing, email verification link via AWS SES
2. **Email Verification**: SHA-256 hashed tokens, 24-hour expiry, one-time use
3. **Login** (`POST /v1/auth/login`): Email verification gate, JWT with Redis storage
4. **Password Reset**:
   - `GET /v1/user/forgot-password?email=xyz`: Sends reset token via email (always returns success for security)
   - `POST /v1/user/reset-password`: Body `{token, newPassword}`, 1-hour token expiry

### User Onboarding (KYC)
**Independent of business** - User identity verification separate from business role claiming

1. **Initiate Onboarding** (`POST /v1/user/onboard/initiate`):
   - Provide name, dob, taxId (PAN), nationalId (Aadhaar)
   - Verifies PAN via PAN_COM
   - Verifies PAN-Aadhaar link
   - Generates Aadhaar DigiLocker link (returned immediately)
   - Sets status to `kyc_in_progress`

2. **Complete Onboarding** (`POST /v1/user/onboard/complete`):
   - User completes DigiLocker verification
   - Fetch Aadhaar STATUS using stored transaction ID
   - Store verification data in `national_id_data`
   - Set status to `kyc_completed`
   - Claim role in specific business (businessId, claimedRole)

3. **Check Status** (`GET /v1/user/onboard/status`): Resume capability

**KYC Status**: kyc_not_started → kyc_in_progress → kyc_completed → kyc_verified

### User Roles
- **Owner**: Full business control
- **Admin**: Administrative access
- **Finance**: Financial operations only
- **Support**: Read-only/support access

## Business Management

### Database Schema
- **`business`**: Main business entities (GSTIN, KYB status, business type)
- **`kyb_data`**: Income Tax ID, Company ID, business deed document URL
- **`business_user_relation`**: Many-to-many user-business mapping with roles (added_at, updated_at timestamps)
- **`subscription_status`**: Subscription tiers (free, basic, premium, enterprise)

### Business Creation & KYB
1. **Create Business** (`POST /v1/business`): GSTIN + name, creates relation with support role
2. **Update KYB** (`POST /v1/business/kyb`): Business type, ITID, CIN
3. **Upload Documents** (S3 presigned URLs):
   - `GET /v1/business/kyb/upload` - Get presigned PUT URL
   - `POST /v1/business/kyb/confirm-upload` - Verify and get download URL

**Business Types**: sole_proprietorship, partnership, private_limited_company, trust, ngo, society, etc.

### User Invitation & Management
**Invite User** (`POST /v1/business/invite-user`): Admin/owner invites user with email, name, role, kycNeeded. Creates user, sends password via email, adds to business_user_relation.

**User Management Routes** (admin/owner only):
- `GET /v1/business/users?bid=xxx` - List business users
- `POST /v1/business/users/update` - Update role (body: `{bid, userId, newRole}`)
- `DELETE /v1/business/users` - Remove user (body: `{bid, userId}`)

**Authorization Rules**:
- Admin cannot change/remove owner
- Admin cannot promote to owner

## Payment System

### Gateway Orchestrator Pattern
Automatic routing based on payment method and card details:
- **JioPay**: Net banking, Master/Visa/RuPay credit, corporate cards
- **Easebuzz**: Amex/Diners, Master/Visa debit

### Payment API Endpoints
```bash
POST /api/v1/process_payment/fetch_wallet        # Get business wallet
POST /api/v1/process_payment/get_fee_rules       # Fee calculation rules
POST /api/v1/process_payment/make_payment        # Process payment (auto-routing)
POST /api/v1/process_payment/tokenize            # Tokenize card (PCI DSS)
POST /api/v1/process_payment/add_payment_source  # Add bank/UPI source
POST /api/v1/process_payment/webhooks/jiopay     # JioPay webhook
POST /api/v1/process_payment/webhooks/easebuzz   # Easebuzz webhook
```

### Payment Methods
- **Card**: Visa, MasterCard, RuPay, Amex, Diners (credit/debit/corporate)
- **Net Banking**: Multiple banks via bank codes
- **UPI**: VPA-based payments
- **Wallet**: Digital wallet integration

### Wallet Management
- **Tokenized Cards**: Gateway-provided secure tokens
- **Bank Accounts**: Verified via penny drop
- **UPI Details**: Future implementation
- In-memory storage (to be replaced with database)

### Bank Verification (Penny Drop)
Mock service with pattern-based testing (last 4 digits):
- `****0000`: ACCOUNT_CLOSED
- `****1111`: INVALID_ACCOUNT
- `****2222`: NAME_MISMATCH
- `****9999`: API_TIMEOUT
- Other: Success

### Fee Calculator
- Percentage-based + fixed fees
- Min/max enforcement
- Card type/category differentiation
- GST handling for Indian market

### Adding New Gateway
1. Create client in `internal/adapters/external/payment/newgateway/`
2. Implement `PaymentGateway` interface
3. Update `factory/gateway_factory.go`
4. Update routing in `payment_orchestrator.go`
5. Add environment variables

## Payee Management

### Architecture
Three-table normalized design:
1. **`payee_business`**: Global canonical payee (one per GSTIN)
2. **`payee_bank_accounts`**: Bank accounts per payee (one-to-many)
3. **`business_payee_relation`**: Customer-specific metadata

**Key Principle**: Payee identified by GSTIN. Multiple customers can add same payee with different display names.

### Onboarding Flow (11 Steps)
1. Validate input (GSTIN, account, IFSC)
2. Check payee exists by GSTIN
3. If new: Verify GSTIN via AuthBridge GST API
4. Compute account hash (SHA-256)
5. Check duplicate account for payee
6. Penny drop verification
7. Cross-verify: GST name vs bank account holder
8. Anti-money rotation: Check payee holder ≠ customer director
9. Atomic transaction: Create account + relation
10. Return verification statuses
11. Audit trail logging

### Payee API Endpoints
```bash
POST   /v1/payees                                      # Onboard payee
GET    /v1/payees                                      # List payees
GET    /v1/payees/:payee_id/accounts/:account_id      # Get details
PUT    /v1/payees/:payee_id/accounts/:account_id      # Update metadata
DELETE /v1/payees/:payee_id/accounts/:account_id      # Soft delete
```

## Third-Party Integrations

### AuthBridge (Document Verification)
**Supported Documents**: PAN, PAN_COM, PAN_GST, GSTIN, CIN, AADHAAR
**Process**: Request → JSON → AES-256 encrypt → API → Decrypt → Parse

**Aadhaar Flow**:
1. LINK action: Generate DigiLocker URL
2. User completes verification
3. STATUS action: Fetch verification data

### AWS SES (Email Service)
- Single and bulk emails
- HTML/text support
- Send quota monitoring
- Email verification templates

### AWS S3 (File Storage)
- Presigned URLs (15-min expiry)
- Server-side encryption (AES-256)
- Multipart upload for large files
- Bucket structure: `PII/{business_id}/`, `PII/{user_id}/`

## Development Workflow

### Local Setup
```bash
cd deployments/docker && ./start.sh && ./setup-postgres-users.sh && ./setup-redis-users.sh
cp .env.example .env  # Edit values
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up
go run ./cmd/api
```

### Adding Features
1. Start with domain entities in `internal/core/domain/`
2. Add repository interfaces
3. Implement adapters
4. Create service orchestration
5. Add HTTP handlers

### Database Migrations
1. Create sequential files in `migrations/sql/` (000001, 000002)
2. Always create `.up.sql` and `.down.sql`
3. Test with `./bin/migrate -action=up` and `-action=down`

## Testing

### Mock Implementations
- `MockPaymentOrchestrator`, `MockFeeCalculator`, `MockWalletService`
- `MockBankVerificationService`, `MockGSTVerificationService`, `MockCrossVerificationService`

### Integration Testing
```bash
go test ./internal/services/payments/... -v
go test ./internal/adapters/external/payment/jiopay/... -v
go test ./internal/core/business_rules/... -v -run TestBankVerification
```

## Important Notes

### Payment Gateway Routing
- Net Banking → JioPay
- Amex/Diners → Easebuzz
- Master/Visa/RuPay Credit → JioPay
- Master/Visa Debit → Easebuzz

### Tokenization vs Wallet
- `/tokenize`: PCI DSS compliant card storage (returns gateway token)
- `/add_payment_source`: Bank accounts, UPI (requires verification)
- Cards auto-added to wallet after tokenization

### Context & Timeouts
- Payment: 30s
- Tokenization: 60s (JioPay 2-step)
- Bank verification: 30s

### Amount Handling
- All amounts in smallest unit (paise for INR)
- Example: ₹500.00 = 50000 paise

### PostgreSQL Timestamp Handling
- Use `::text` to cast timestamps to string when scanning into Go string fields
- Use `COALESCE(column::text, '')` for nullable timestamp columns
- Example: `SELECT added_at::text, COALESCE(updated_at::text, '') FROM table`

### Security
- **PCI DSS**: Cards never in plaintext, CVV transient only
- **Bank Accounts**: Encrypted at rest, masked in responses
- **JWT**: Required for all protected endpoints
- **Presigned URLs**: Short expiry (15 min)
- **Webhook**: Signature verification

## Documentation

Detailed docs in `docs/`:
- `payment_module.md` - Payment system
- `business_module.md` - Business entity management
- `payee_module.md` - Payee management
- `migrations.md` - Migration guide
- `FOLDER_STRUCTURE.md` - Project architecture

## Quick Reference

### Example: User Signup → KYC → Business Creation
```bash
# 1. Signup
POST /v1/auth/signup {"email": "user@example.com", "password": "Pass123!"}

# 2. Verify email (click link in email)
GET /v1/user/verify-email?token=<TOKEN>

# 3. Login
POST /v1/auth/login {"email": "user@example.com", "password": "Pass123!"}
# Returns: {"data": {"token": "eyJhbGc..."}}

# 4. Initiate User Onboarding (identity verification)
POST /v1/user/onboard/initiate -H "Authorization: Bearer <TOKEN>"
  {"name": "John Doe", "dob": "1990-01-15", "taxId": "ABCDE1234F", "nationalId": "123456789012"}
# Returns: {"data": {"status": "kyc_in_progress", "aadhaarLink": "https://...", "txnId": "..."}}

# 5. Complete DigiLocker verification (user action on aadhaarLink)

# 6. Complete Onboarding & Claim Role in Business
POST /v1/user/onboard/complete -H "Authorization: Bearer <TOKEN>"
  {"businessId": "uuid-of-business", "claimedRole": "owner"}

# 7. Create Business (if needed)
POST /v1/business -H "Authorization: Bearer <TOKEN>"
  {"name": "Acme Corp", "gstin": "29ABCDE1234F1Z5"}
```

### Example: Payment Flow
```bash
# 1. Tokenize card
POST /api/v1/process_payment/tokenize
  {"user_id": "uuid", "card_details": {...}}

# 2. Make payment (auto-routing)
POST /api/v1/process_payment/make_payment
  {"amount": 50000, "currency": "INR", "payment_method": "card", ...}
```

### Example: Payee Onboarding
```bash
POST /v1/payees -H "Authorization: Bearer <TOKEN>"
  {
    "gstin": "29ABCDE1234F1Z5",
    "account_number": "1234567890123456",
    "ifsc_code": "HDFC0000123",
    "account_holder_name": "ABC Industries Pvt Ltd",
    "display_name": "ABC Corp"
  }
```
