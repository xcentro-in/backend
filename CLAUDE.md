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
- **Payment**: Payment processing with gateway orchestration (`/api/v1/process_payment`)
- **Payee**: Payee onboarding with verification (`/v1/payees`)
- **Business Rules**: Fee calculation, bank/GST verification

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

# Payment Gateways - JioPay
JIOPAY_BASE_URL=https://uat.jiopay.co.in
JIOPAY_TOKENIZATION_BASE_URL=https://uat.jiopay.co.in/qatsp/api/vault
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key                    # For HMAC-SHA256 hash generation
JIOPAY_APP_ID=your_app_id                      # For x-app-id header (Vault APIs)
JIOPAY_ENCRYPTION_KEY=your_16byte_key!!        # Must be exactly 16 bytes for AES-128
JIOPAY_WEBHOOK_SECRET=your_webhook_secret

# Payment Gateways - Easebuzz
EASEBUZZ_BASE_URL=https://pay.easebuzz.in
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt_key
EASEBUZZ_API_KEY=your_api_key                  # For EBZ-KEY header (TMS APIs)
EASEBUZZ_ENCRYPTION_KEY=your_encryption_key    # Optional
EASEBUZZ_ENCRYPTION_IV=your_encryption_iv      # Optional
EASEBUZZ_WEBHOOK_SECRET=your_webhook_secret

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

### Payment Gateway Implementation

**JioPay Client** (`internal/adapters/external/payment/jiopay/`):
- ✅ **Tokenization**: POST to Vault API `/fetch-card` with 5 mandatory headers
- ✅ **Cryptogram**: POST to Vault API `/getCryptogram` with 5 mandatory headers
- ✅ **Payment**: POST to `/pg/api/v2/initiateSale` with HMAC-SHA256 hash
- ⏳ **Webhook**: Signature verification (TODO)

**Easebuzz Client** (`internal/adapters/external/payment/easebuzz/`):
- ✅ **Tokenization**: POST to TMS API `/v1/create_token` with full payload encryption
- ✅ **Cryptogram**: POST to TMS API `/v1/get_cryptogram` with full payload encryption
- ✅ **Payment**: POST to `/initiate_seamless_payment/` with field-level encryption
- ⏳ **Webhook**: Signature verification (TODO)

### JioPay Integration Details

**Vault API Mandatory Headers** (for tokenization & cryptogram):
1. `Content-Type: application/json`
2. `x-app-id`: App identifier from config
3. `x-timestamp-id`: Format yyyyMMddHHmmss (e.g., "20250610125921")
4. `x-trace-id`: Uppercase UUID without hyphens
5. `x-app-token`: AES-128-ECB encrypted token (Base64)

**x-app-token Generation**:
```
1. Concatenate: x-app-id + "$" + x-trace-id + "$" + x-timestamp-id
2. Encrypt using AES-128-ECB with 16-byte shared key
3. Base64 encode the encrypted bytes
```

**Payment API HMAC-SHA256 Hash**:
```
1. Sort all request fields alphabetically by key
2. Concatenate all VALUES in sorted order (no separators)
3. HMAC-SHA256(apiKey, concatenated_string)
4. Lowercase hex string (64 characters)
```

### Easebuzz Integration Details

**TMS API Headers** (for tokenization & cryptogram):
- `Content-Type: application/json`
- `EBZ-KEY`: API key from config

**Encryption Methods**:
1. **Field-Level** (for payments): Encrypt specific fields (card_token, cryptogram, cvv) with AES-256-CBC
2. **Full Payload** (for tokenization/cryptogram): Encrypt entire JSON payload with AES-256-CBC

**AES-256 Key Derivation**:
- Key: SHA-256(merchantKey) → 32 bytes
- IV: SHA-256(salt) → first 16 bytes

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

## API Reference (All Endpoints)

### Authentication APIs (`/v1/auth`)
- `POST /v1/auth/signup` - User registration with email verification
- `POST /v1/auth/login` - User login with JWT token generation
- `POST /v1/auth/logout` - User logout (invalidate JWT)

### User APIs (`/v1/user`)
- `GET /v1/user/profile` - Get user profile
- `PUT /v1/user/profile` - Update user profile
- `GET /v1/user/verify-email?token=xxx` - Email verification
- `GET /v1/user/forgot-password?email=xxx` - Request password reset
- `POST /v1/user/reset-password` - Reset password with token
- `GET /v1/user/roles` - Get user's business roles (returns userId, businessId, role)

### User Onboarding (KYC) APIs (`/v1/user/onboard`)
- `POST /v1/user/onboard/initiate` - Start KYC process (PAN + Aadhaar verification)
- `POST /v1/user/onboard/complete` - Complete KYC with DigiLocker data
- `GET /v1/user/onboard/status` - Check KYC status

### Business APIs (`/v1/business`)
- `POST /v1/business` - Create new business entity
- `GET /v1/business?bid=xxx` - Get business details
- `POST /v1/business/kyb` - Update KYB information
- `GET /v1/business/kyb/upload?bid=xxx&docType=xxx` - Get S3 presigned upload URL
- `POST /v1/business/kyb/confirm-upload` - Confirm document upload
- `POST /v1/business/invite-user` - Invite user to business
- `GET /v1/business/users?bid=xxx` - List business users
- `POST /v1/business/users/update` - Update user role
- `DELETE /v1/business/users` - Remove user from business

### Wallet Management APIs (`/v1/wallet`)
- `POST /v1/wallet/fetch` - Get business wallet with stored cards/banks
  - Request body: `{"business_id": "uuid-of-business"}`
  - Returns: Stored cards, bank accounts, UPI details, credit lines
- `POST /v1/wallet/add_source` - Add bank account or UPI to wallet
  - Request body: `{"business_id": "uuid-of-business", "source_type": "bank_account", "bank_account_details": {...}}`
  - Performs verification (penny drop for bank accounts)

### Payment Processing APIs (`/api/v1/process_payment`)
**Payment Source Selection**:
- `POST /api/v1/process_payment/get_payment_sources_with_fees` - Get payment sources + fee calculation
  - Request body: `{"business_id": "uuid", "payment_context": "vendor_payment", "base_amount": 50000}`
  - Returns: Filtered payment sources with fee breakdown based on context

**Card Tokenization**:
- `POST /api/v1/process_payment/tokenize` - Tokenize card (routes to JioPay/Easebuzz)
  - Response includes: `token`, `card_id`, `last_four_digits`, `gateway_used`
  - Card automatically stored in database

**Payment Processing**:
- `POST /api/v1/process_payment/make_payment` - Process payment with auto-routing
  - Supports: card, token_card, netbanking, upi, wallet
  - Returns: `payment_id`, `redirect_url`, `status`

**Fee Calculation**:
- `POST /api/v1/process_payment/get_fee_rules` - Get fee rules for payment context

**Webhooks (Payment Status)**:
- `POST /api/v1/process_payment/webhooks/payment_status?type=jiopay` - JioPay payment status webhook
- `POST /api/v1/process_payment/webhooks/payment_status?type=easebuzz` - Easebuzz payment status webhook
  - **Purpose**: Real-time payment authorization status (after 3DS/OTP completion)
  - **Updates**: Payment status (success/failed/pending), initializes settlement_status='pending'
  - **Metadata**: Payment-specific fields (txn_id, response_code, etc.)
  - **Timing**: Immediate (within seconds of payment completion)

**Webhooks (Settlement Status)**:
- `POST /api/v1/process_payment/webhooks/settlement_status?type=jiopay` - JioPay settlement webhook ✅ **IMPLEMENTED**
  - **Data Format**: JSON
  - **Processing**: Single transaction per webhook
  - **Purpose**: Settlement notification (actual funds transfer to merchant account)
  - **Timing**: T+1 to T+3 days after successful payment

- `POST /api/v1/process_payment/webhooks/settlement_status?type=easebuzz` - Easebuzz settlement webhook ✅ **IMPLEMENTED**
  - **Data Format**: Form-urlencoded (parsed to JSON)
  - **Processing**: Batch (multiple transactions in `settled_transactions[]` array)
  - **Purpose**: Batch settlement notifications (actual funds transfer to merchant account)
  - **Timing**: T+1 to T+3 days after successful payment

  **Common Characteristics**:
  - **Updates**: Settlement status 'pending' → 'settled', settlement financial data
  - **Financial Fields**:
    - `settlement_amount`: Original payment amount (txnAmount from webhook)
    - `mdr_amount`: Gateway fees = service_charge + service_tax (MDR + GST on MDR)
    - `net_settlement_amount`: Final amount received = settlement_amount (transaction_amount - mdr)
    - `settled_at`: Actual settlement date
    - `settlement_reference_id`: Settlement batch ID (payout_id for Easebuzz)
  - **Key Characteristic**: Arrival = 100% success (no status field in webhook)
  - **Idempotency**: Duplicate webhooks skipped (check settlement_status = 'settled')
  - **Security**: SHA-512 signature validation (Easebuzz), HMAC-SHA256 (JioPay)

### Payee APIs (`/v1/payees`)
- `POST /v1/payees` - Onboard new payee (GSTIN + bank verification)
- `GET /v1/payees?bid=xxx` - List payees for business
- `GET /v1/payees/:payee_id/accounts/:account_id` - Get payee account details
- `PUT /v1/payees/:payee_id/accounts/:account_id` - Update payee metadata
- `DELETE /v1/payees/:payee_id/accounts/:account_id` - Soft delete payee account

### Voucher APIs (`/v1/vouchers`)
- `POST /v1/vouchers` - Create voucher
- `GET /v1/vouchers` - List vouchers
- `GET /v1/vouchers/:id` - Get voucher details
- `PUT /v1/vouchers/:id` - Update voucher
- `DELETE /v1/vouchers/:id` - Delete voucher
- `POST /v1/vouchers/:id/submit` - Submit voucher for approval
- `POST /v1/vouchers/:id/approve` - Approve voucher
- `POST /v1/vouchers/:id/reject` - Reject voucher

### Payout APIs (`/v1/payout`)
**Webhooks**:
- `POST /v1/payout/webhooks/castler` - Castler payout webhook (HMAC verified)

**Operations Dashboard**:
- `GET /v1/payout/pending-approvals` - Get pending payout approvals
- `POST /v1/payout/:transfer_id/approve` - Approve payout transfer
- `POST /v1/payout/:transfer_id/cancel` - Cancel payout transfer
- `GET /v1/payout/:transfer_id` - Get transfer details
- `GET /v1/payout/:transfer_id/webhooks` - Get webhook history
- `GET /v1/payout/timeouts` - Get timed-out transfers
- `POST /v1/payout/:transfer_id/mark-completed` - Manually mark as completed
- `GET /v1/payout/config` - Get payout configuration
- `PUT /v1/payout/config` - Update payout configuration
- `POST /v1/payout/config` - Create payout configuration

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

### Payment Gateway Routing Logic
Orchestrator automatically selects gateway based on payment method and card type:

**JioPay Routes**:
- All Net Banking transactions
- All UPI transactions
- All Wallet transactions
- Credit Cards: Visa, MasterCard, RuPay
- All Corporate Credit Cards

**Easebuzz Routes**:
- Credit Cards: Amex, Diners
- Debit Cards: Visa, MasterCard (< ₹2000)
- Debit Cards: Amex, Diners (all amounts)

**Special Rules**:
- Visa/MasterCard Debit ≥ ₹2000 → JioPay
- RuPay Debit (all amounts) → JioPay

### Payment Gateway Technical Details

**JioPay Implementation Status**:
- ✅ Tokenization with Vault API headers (x-app-id, x-timestamp-id, x-trace-id, x-app-token)
- ✅ Cryptogram generation with Vault API headers
- ✅ Payment initiation with HMAC-SHA256 secure hash
- ✅ Card storage in database after tokenization
- ✅ Payment webhook validation with HMAC-SHA256
- ✅ Settlement webhook validation with HMAC-SHA256
- ✅ Settlement financial tracking (settlement_amount, mdr_amount, net_settlement_amount)

**Easebuzz Implementation Status**:
- ✅ Tokenization with TMS API (full payload AES-256 encryption)
- ✅ Cryptogram generation with TMS API (full payload encryption)
- ✅ Payment with field-level AES-256 encryption (token, cryptogram, cvv)
- ✅ Card storage in database after tokenization
- ✅ Payment webhook validation with SHA-512
- ✅ Settlement webhook (batch processing with form-urlencoded parsing)
- ✅ Settlement financial tracking (settlement_amount, mdr_amount, net_settlement_amount)

### Tokenization Flow (Auto-routing)
1. **Client Request**: `/tokenize` with card details
2. **Gateway Selection**: Orchestrator selects JioPay or Easebuzz based on card type
3. **Tokenization**: Gateway-specific API call with encryption/headers
4. **Database Storage**: Card automatically saved via `StoredCardRepository`
5. **Response**: Token + CardID returned to client

### Token-Based Payment Flow
1. **Cryptogram Generation**: Gateway generates fresh cryptogram for stored card
2. **Payment Initiation**: Token + Cryptogram sent to gateway
3. **3DS/OTP**: User redirected for authentication
4. **Webhook**: Gateway sends status update
5. **Status Update**: Payment status updated in database

### Tokenization vs Wallet
- `/tokenize`: PCI DSS compliant card storage (returns gateway token)
- `/add_payment_source`: Bank accounts, UPI (requires verification)
- Cards auto-added to wallet after tokenization
- Cryptogram generated fresh for each payment

### Context & Timeouts
- Payment: 30s (standard transactions)
- Tokenization: 60s (JioPay Vault API, Easebuzz TMS API)
- Cryptogram: 30s (generated on-demand)
- Bank verification: 30s (penny drop)

### Amount Handling
- All amounts in smallest unit (paise for INR)
- Example: ₹500.00 = 50000 paise
- JioPay expects: "500.00" (rupees with decimals)
- Easebuzz expects: "500.00" (rupees with decimals)
- Conversion done automatically in gateway clients

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

### Example: Token-Based Card Payment Flow (Complete)
```bash
# Step 1: Tokenize Card (Auto-routes to JioPay or Easebuzz)
POST /api/v1/process_payment/tokenize -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business",
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit",
    "last_four_digits": "1111"
  }
}

# Response:
{
  "data": {
    "token": "jio_tok_visa_abc123",           # Gateway-provided token
    "card_id": "card_uuid_001",               # Our card identifier
    "last_four_digits": "1111",
    "card_type": "visa",
    "card_category": "credit",
    "expiry_month": "12",
    "expiry_year": "25",
    "gateway_used": "jiopay",                 # JioPay or Easebuzz
    "status": "success"
  }
}

# Step 2: Make Payment with Tokenized Card
POST /api/v1/process_payment/make_payment -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business",
  "amount": 50000,                            # ₹500.00 in paise
  "currency": "INR",
  "payment_method": "token_card",             # Use saved card
  "token_card_details": {
    "card_id": "card_uuid_001"                # Card ID from Step 1
  },
  "description": "Payment for invoice #12345",
  "callback_url": "https://yourapp.com/payment/callback"
}

# Response:
{
  "data": {
    "payment_id": "pay_uuid_001",
    "status": "pending",                      # Pending 3DS/OTP
    "amount": 50000,
    "fee_amount": 1250,                       # 2.5% fee
    "total_amount": 51250,
    "currency": "INR",
    "payment_method": "token_card",
    "gateway_used": "jiopay",
    "redirect_url": "https://uat.jiopay.co.in/tsp/pg/api/v2/authRedirect",
    "message": "Payment initiated successfully"
  }
}

# Step 3: User completes 3DS/OTP on redirect_url

# Step 4: Webhook callback (automatic)
POST https://yourapp.com/api/v1/process_payment/webhooks/payment_status?type=jiopay
{
  "payment_id": "pay_uuid_001",
  "status": "success",
  "gateway_payment_id": "jio_pay_789",
  "amount": "51250",
  ...
}

# Your system updates payment status to "success"
```

### Example: Direct Card Payment Flow (No Tokenization)
```bash
# Make payment with card details (one-time payment)
POST /api/v1/process_payment/make_payment -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "card",
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit"
  },
  "description": "One-time payment",
  "callback_url": "https://yourapp.com/payment/callback"
}

# Gateway auto-routing:
# - Visa Credit → JioPay
# - If it was Amex → Easebuzz
```

### Example: Get Payment Sources with Fee Calculation
```bash
# Get all payment sources with calculated fees for specific context
# Note: Returns only real database data (no mock data)
POST /api/v1/process_payment/get_payment_sources_with_fees -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business",         # Required in request body
  "payment_context": "vendor_payment",       # or "payroll", "gst_payment"
  "base_amount": 100000                      # ₹1000.00 in paise
}

# Response:
{
  "data": {
    "cards": [
      {
        "card_id": "card_uuid_001",
        "last_four_digits": "1111",
        "card_type": "visa",
        "card_category": "credit",
        "gateway_used": "jiopay",
        "fee_amount": 2500,                   # 2.5% of 100000
        "total_amount": 102500
      }
    ],
    "bank_accounts": [
      {
        "account_id": "bank_uuid_001",
        "last_five_digits": "67890",
        "bank_name": "HDFC Bank",
        "fee_amount": 0,                      # No fee for bank transfer
        "total_amount": 100000
      }
    ]
  }
}
```

### Example: Wallet Management
```bash
# Fetch wallet with all stored payment sources
POST /v1/wallet/fetch -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business"
}

# Response:
{
  "data": {
    "business_id": "uuid-of-business",
    "stored_cards": [
      {
        "card_id": "card_uuid_001",
        "last_four_digits": "1111",
        "card_type": "visa",
        "card_category": "credit",
        "expiry_month": "12",
        "expiry_year": "25",
        "is_active": true
      }
    ],
    "bank_accounts": [
      {
        "account_id": "bank_uuid_001",
        "last_five_digits": "67890",
        "bank_name": "HDFC Bank",
        "ifsc_code": "HDFC0000123",
        "account_type": "current",
        "is_verified": true,
        "is_active": true
      }
    ],
    "upi_details": [],
    "credit_lines": []
  }
}

# Add bank account to wallet
POST /v1/wallet/add_source -H "Authorization: Bearer <TOKEN>"
{
  "business_id": "uuid-of-business",
  "source_type": "bank_account",
  "bank_account_details": {
    "account_number": "1234567890123456",
    "ifsc_code": "HDFC0000123",
    "account_holder_name": "XCentro Pvt Ltd",
    "bank_name": "HDFC Bank",
    "branch_name": "MG Road",
    "account_type": "current"
  }
}

# Response (after penny drop verification):
{
  "status": "success",
  "data": {
    "success": true,
    "message": "Bank account verified and added successfully",
    "source_id": "bank_uuid_002",
    "source_type": "bank_account",
    "verification_status": "verified"
  }
}
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

### Example: Settlement Webhook Flow (JioPay)
```bash
# Timeline: Payment → Settlement (T+1 to T+3 days)

# Day 0: Payment completed (payment_status webhook)
# - payment.status = 'success'
# - payment.settlement_status = 'pending'
# - payment.total_amount = 51250 (₹512.50 with fees)

# Day 2: Settlement completed by JioPay (settlement webhook arrives)
POST https://yourapp.com/api/v1/process_payment/webhooks/settlement_status?type=jiopay
{
  "aggregatorID": "A001",
  "merchantId": "123243",
  "merchantTxnNo": "pay_uuid_001",           # Our payment_id
  "invoiceNos": "INV12345",
  "paymentMode": "CARD",
  "paymentSubInstType": "CC",
  "txnChannel": "WEB",
  "txnAmount": "512.50",                     # Original amount in rupees
  "txnCharges": "12.81",                     # MDR base (2.5% of 512.50)
  "serviceTax": "2.31",                      # GST on MDR (18% of 12.81)
  "settlementID": "A00121-20251013-010",     # Settlement batch ID
  "settlementDate": "20251013",              # YYYYMMDD format
  "settlementAccount": "9876543210987654321",
  "settlementAccountIFSC": "HDFC0001",
  "settledAmount": "497.38",                 # Net: 512.50 - 12.81 - 2.31 = 497.38
  "cardNetwork": "VISA",
  "addlParam1": "SettlementInfo",
  "secureHash": "39867b8ef38a2853aacb84bbf5f927a72b27bdb301365245564f012d20dbc2f2"
}

# System Processing:
# 1. Validate HMAC-SHA256 signature
# 2. Find payment by merchantTxnNo
# 3. Verify payment.status = 'success' and settlement_status = 'pending'
# 4. Parse and convert amounts:
#    - settlement_amount = 51250 paise (₹512.50 * 100)
#    - mdr_amount = 1512 paise [(₹12.81 + ₹2.31) * 100]
#    - net_settlement_amount = 49738 paise (₹497.38 * 100)
# 5. Update database:
#    - settlement_status = 'settled'
#    - settlement_amount = 51250
#    - mdr_amount = 1512
#    - net_settlement_amount = 49738
#    - settled_at = 2025-10-13
#    - settlement_reference_id = "A00121-20251013-010"
#    - settlement_metadata = {full webhook payload as JSONB}

# Financial Summary:
# Customer Paid:    ₹512.50 (including payment processing fee)
# Gateway Settled:  ₹512.50 (same as payment amount)
# Gateway MDR:      ₹15.12  (2.5% + 18% GST)
# Merchant Received: ₹497.38 (final amount in bank account)
```

### Example: Settlement Webhook Flow (Easebuzz - Batch Processing)
```bash
# Timeline: Multiple Payments → Batch Settlement (T+1 to T+3 days)

# Day 0: Multiple payments completed (payment_status webhooks)
# Payment 1:
#   - payment.id = "pay_uuid_001", txnid = "ORDER123"
#   - payment.status = 'success'
#   - payment.settlement_status = 'pending'
#   - payment.total_amount = 100000 (₹1000.00)
# Payment 2:
#   - payment.id = "pay_uuid_002", txnid = "ORDER124"
#   - payment.status = 'success'
#   - payment.settlement_status = 'pending'
#   - payment.total_amount = 50000 (₹500.00)

# Day 2: Batch settlement completed by Easebuzz (single webhook for multiple transactions)
POST https://yourapp.com/api/v1/process_payment/webhooks/settlement_status?type=easebuzz
Content-Type: application/x-www-form-urlencoded

data={
  "status": 1,                                 # 1 = success, 0 = failure
  "data": {
    "hash": "abc123...def789",                 # SHA-512 signature
    "paid_out": 1,                             # 1 = funds disbursed
    "payout_id": "PT1LCHKBAB",                 # Settlement batch ID
    "payout_date": "2025-10-13 14:30:00.000000",
    "bank_transaction_id": "UTR202510131430001",
    "bank_name": "HDFC Bank",
    "account_number": "50200012345678",
    "ifsc_code": "HDFC0001234",
    "name_on_bank": "XCentro Private Limited",

    # Batch-level totals (all amounts as numbers, not strings)
    "total_amount": 1500.00,                   # Total of all transactions
    "payout_amount": 1455.30,                  # Net amount paid out
    "refund_amount": 0.00,
    "service_charge_amount": 37.50,            # Total MDR
    "service_tax_amount": 7.20,                # Total GST on MDR

    # Transaction array - KEY: Process each one
    "settled_transactions": [
      {
        "txnid": "ORDER123",                   # Our payment_id for lookup
        "easepayid": "EZBPAY789012",          # Easebuzz reference
        "transaction_amount": 1000.00,         # Original amount in rupees
        "service_charge": 25.00,               # MDR base (2.5%)
        "service_tax": 4.50,                   # GST on MDR (18% of 25.00)
        "settlement_amount": 970.50,           # Net: 1000 - 25 - 4.50
        "transaction_type": "CARD",
        "submerchant_id": null,
        "split_transactions": []
      },
      {
        "txnid": "ORDER124",
        "easepayid": "EZBPAY789013",
        "transaction_amount": 500.00,
        "service_charge": 12.50,               # MDR base (2.5%)
        "service_tax": 2.25,                   # GST on MDR (18% of 12.50)
        "settlement_amount": 485.25,           # Net: 500 - 12.50 - 2.25
        "transaction_type": "UPI",
        "submerchant_id": null,
        "split_transactions": []
      }
    ],
    "refund_transactions": [],                 # Not processed (future feature)
    "split_payouts": [],
    "submerchant_payouts": []
  }
}

# System Processing (Batch Loop):
# For each transaction in settled_transactions[]:
#
# Transaction 1 (ORDER123):
# 1. Parse form-urlencoded data to JSON
# 2. Validate webhook.status = 1
# 3. Hash validation (SHA-512) - TODO: Add proper validation
# 4. Find payment by txnid: "ORDER123" → payment_id "pay_uuid_001"
# 5. Verify payment.status = 'success' and settlement_status = 'pending'
# 6. Convert amounts from float64 to paise:
#    - settlement_amount = 100000 paise (1000.00 * 100)
#    - mdr_amount = 2950 paise [(25.00 + 4.50) * 100]
#    - net_settlement_amount = 97050 paise (970.50 * 100)
# 7. Update database:
#    - settlement_status = 'settled'
#    - settlement_amount = 100000
#    - mdr_amount = 2950
#    - net_settlement_amount = 97050
#    - settled_at = 2025-10-13 14:30:00
#    - settlement_reference_id = "PT1LCHKBAB"
#    - settlement_metadata = {
#        "settlement_batch_id": "PT1LCHKBAB",
#        "settlement_bank_transaction_id": "UTR202510131430001",
#        "easepayid": "EZBPAY789012",
#        "transaction_type": "CARD",
#        "batch_total_amount": 1500.00,
#        ...full webhook data...
#      }
# 8. Increment success_count
#
# Transaction 2 (ORDER124):
# [Same processing steps as Transaction 1]
#
# Response (after batch processing):
{
  "status": "success",
  "message": "Batch settlement processed",
  "summary": {
    "total_transactions": 2,
    "processed": 2,                            # Successfully updated
    "skipped": 0,                              # Already settled or invalid
    "errors": 0                                # Failed to process
  }
}

# Financial Summary (Transaction 1):
# Customer Paid:     ₹1000.00 (original payment)
# Gateway Settled:   ₹1000.00 (confirmed by webhook)
# Gateway MDR:       ₹29.50   (2.5% MDR + 18% GST)
# Merchant Received: ₹970.50  (in bank account)

# Financial Summary (Transaction 2):
# Customer Paid:     ₹500.00
# Gateway Settled:   ₹500.00
# Gateway MDR:       ₹14.75  (2.5% MDR + 18% GST)
# Merchant Received: ₹485.25

# Batch Total:
# Total Settled:     ₹1500.00
# Total MDR:         ₹44.25
# Total Received:    ₹1455.75 (actual bank credit)
```

### Easebuzz Settlement Webhook: Key Differences
```bash
# JioPay vs Easebuzz Settlement Webhooks

JioPay:
- Data Format: JSON (direct)
- Processing Model: Single transaction per webhook
- Amount Format: Decimal strings ("1000.00")
- Date Format: YYYYMMDD ("20251013")
- Hash Algorithm: HMAC-SHA256
- Webhook Frequency: One per payment

Easebuzz:
- Data Format: Form-urlencoded (requires parsing to JSON)
- Processing Model: Batch (multiple transactions in array)
- Amount Format: Numbers (1000.00 as float64)
- Date Format: "YYYY-MM-DD HH:MM:SS.SSSSSS"
- Hash Algorithm: SHA-512
- Webhook Frequency: One per batch (multiple payments)

# Error Handling Strategy:
# - Continue processing batch even if individual transactions fail
# - Return 200 OK always (prevent gateway retry storms)
# - Track success/skip/error counts separately
# - Idempotency: Skip already-settled transactions
# - Log errors but don't halt batch processing
```
