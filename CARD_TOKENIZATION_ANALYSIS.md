# Card Tokenization API - Comprehensive Analysis

**Generated:** 2025-11-15
**Branch:** claude/card-tokenization-api-01LAW5zvW6f8EYFLDc6YBP7K

---

## Table of Contents
1. [Executive Summary](#executive-summary)
2. [API Endpoint](#api-endpoint)
3. [Architecture & Flow](#architecture--flow)
4. [Actors Involved](#actors-involved)
5. [Gateway Routing](#gateway-routing)
6. [Penny Transaction Flow](#penny-transaction-flow)
7. [Data Storage](#data-storage)
8. [Complete Request-Response Flow](#complete-request-response-flow)
9. [Security Considerations](#security-considerations)
10. [Key Files](#key-files)

---

## Executive Summary

The card tokenization feature allows businesses to securely store customer payment cards for future recurring payments. The system:

- **Accepts** raw card details (PAN, CVV, expiry)
- **Routes** to appropriate payment gateway (JioPay or Easebuzz) based on card type
- **Performs** a ₹1.00 penny transaction to validate and tokenize the card
- **Stores** gateway-provided token (never the actual card number)
- **Returns** card_id for future payments

**Key Security:** PCI DSS compliant - actual card numbers are never stored in the database.

---

## API Endpoint

### POST `/v1/process_payment/tokenize`

**Authentication:** Required (JWT + X-Business-ID header)

**Request:**
```json
{
  "card_details": {
    "card_number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit"
  }
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Card tokenized and saved successfully",
  "card_info": {
    "card_id": "0193e8f4-7b2c-7890-a1b2-c3d4e5f6a7b8",
    "last_four_digits": "1111",
    "card_type": "visa",
    "card_category": "credit",
    "expiry_month": "12",
    "expiry_year": "25",
    "gateway_used": "jiopay"
  }
}
```

**File:** `internal/handlers/http/payment/payment_handler.go:291-425`

---

## Architecture & Flow

### Layer Architecture (Clean Architecture)

```
┌─────────────────────────────────────────────────────────────┐
│  HTTP Handler Layer                                          │
│  internal/handlers/http/payment/payment_handler.go           │
│  - Authentication (JWT + X-Business-ID)                      │
│  - Request validation                                        │
│  - Response formatting                                       │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Service Layer                                               │
│  internal/services/payments/payment_service.go               │
│  - Creates ₹1 payment record (payment_type: tokenization)   │
│  - Calls orchestrator                                        │
│  - Updates payment status (success/failed)                   │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator Layer (Core Business Logic)                   │
│  internal/core/payment/payment_orchestrator.go               │
│  - Gateway selection based on card type                     │
│  - Routes to JioPay or Easebuzz                             │
└────────────────────┬────────────────────────────────────────┘
                     │
            ┌────────┴────────┐
            │                 │
            ▼                 ▼
┌──────────────────┐  ┌──────────────────┐
│  JioPay Gateway  │  │ Easebuzz Gateway │
│  (Vault API)     │  │  (TMS API)       │
│                  │  │                  │
│  Tokenization:   │  │  Tokenization:   │
│  No penny txn    │  │  ₹1.00 txn       │
│  (direct token)  │  │  (implicit)      │
└──────────────────┘  └──────────────────┘
```

### Control Flow

```
User Request
    │
    ▼
[1] Handler: Tokenize()
    │   - Extract userID from JWT
    │   - Extract businessID from X-Business-ID header
    │   - Validate card details
    │   - Create 60s timeout context
    │
    ▼
[2] Service: TokenizeCard()
    │   - Generate payment UUID (UUIDv7)
    │   - Create payment record (amount: 100 paise, type: tokenization)
    │   - Set transaction_id for orchestrator
    │
    ▼
[3] Orchestrator: TokenizeCard()
    │   - Select gateway based on card type/category
    │   - Prepare gateway tokenization request
    │
    ▼
[4] Gateway: TokenizeCard()
    │   ┌─ JioPay: Vault API /fetch-card
    │   │   - 5 mandatory headers (x-app-id, x-timestamp-id, x-trace-id, x-app-token)
    │   │   - Direct tokenization (no penny transaction)
    │   │
    │   └─ Easebuzz: TMS API /v1/create_token
    │       - EBZ-KEY header
    │       - Full payload encryption (AES-256-CBC)
    │       - ₹1.00 penny transaction (implicit in API)
    │
    ▼
[5] Service: Update payment status to success
    │
    ▼
[6] Handler: Store card in wallet
    │   - Create StoredCard entity
    │   - Save to stored_cards table
    │   - Never store actual card number (only token)
    │
    ▼
[7] Return card_id to client
```

---

## Actors Involved

### 1. **Client/Frontend**
- **Role:** Initiates tokenization request
- **Input:** Raw card details (PAN, CVV, expiry)
- **Output:** Receives card_id for future use
- **Security:** Must use HTTPS, JWT token, X-Business-ID header

### 2. **Payment Handler** (`PaymentHandler`)
- **File:** `internal/handlers/http/payment/payment_handler.go`
- **Responsibilities:**
  - Authenticate user (JWT validation)
  - Validate business authorization (X-Business-ID)
  - Validate card details format
  - Create 60-second timeout context (tokenization can take time)
  - Store tokenized card in wallet after success

### 3. **Payment Service** (`PaymentService`)
- **File:** `internal/services/payments/payment_service.go`
- **Responsibilities:**
  - Generate payment UUID (UUIDv7)
  - Create payment record with `payment_type: tokenization`
  - Set amount to 100 paise (₹1.00)
  - Update payment status based on gateway response
  - Coordinate with orchestrator

### 4. **Payment Orchestrator** (`paymentOrchestratorImpl`)
- **File:** `internal/core/payment/payment_orchestrator.go`
- **Responsibilities:**
  - **Gateway Selection Logic:**
    - Visa/MC/RuPay Credit → JioPay
    - Amex/Diners Credit → Easebuzz
    - Visa/MC Debit < ₹2000 → Easebuzz
    - Visa/MC Debit ≥ ₹2000 → JioPay
    - RuPay Debit → JioPay
    - Corporate cards → JioPay
  - Prepare gateway-specific requests
  - Return tokenization response

### 5. **JioPay Gateway Client** (`JioPayClient`)
- **File:** `internal/adapters/external/payment/jiopay/client.go`
- **Responsibilities:**
  - Call Vault API `/fetch-card`
  - Generate 5 mandatory headers:
    - `x-app-id`
    - `x-timestamp-id` (format: yyyyMMddHHmmss)
    - `x-trace-id` (uppercase UUID without hyphens)
    - `x-app-token` (AES-128-ECB encrypted)
    - `Content-Type: application/json`
  - **No penny transaction** - direct tokenization
  - Return token + card_id

### 6. **Easebuzz Gateway Client** (`EasebuzzClient`)
- **File:** `internal/adapters/external/payment/easebuzz/client.go`
- **Responsibilities:**
  - Call TMS API `/v1/create_token`
  - Generate EBZ-KEY header
  - Encrypt full payload (AES-256-CBC)
  - **Implicit ₹1.00 penny transaction** in tokenization API
  - Return token + card_id

### 7. **Wallet Service** (`WalletServiceImpl`)
- **File:** `internal/core/business_rules/wallet_service.go`
- **Responsibilities:**
  - Store tokenized card in database
  - Manage payment sources (cards, bank accounts)
  - Update onboarding flags

### 8. **Stored Card Repository** (`StoredCardRepository`)
- **File:** `internal/adapters/database/postgres/payment/stored_card_repository.go`
- **Responsibilities:**
  - Database CRUD operations for stored_cards table
  - Transaction support
  - Deactivate cards, update last_used_at

### 9. **Payment Repository** (`PaymentRepository`)
- **File:** `internal/adapters/database/postgres/payment/payment_repository.go`
- **Responsibilities:**
  - Create payment record for ₹1 tokenization transaction
  - Update payment status (pending → success/failed)
  - Store gateway payment ID

---

## Gateway Routing

### Routing Decision Tree

```
Card Input
    │
    ├─ Corporate Credit Card ──────────────────────► JioPay
    │
    ├─ Credit Card
    │   ├─ Visa/MasterCard/RuPay ──────────────────► JioPay
    │   └─ Amex/Diners ────────────────────────────► Easebuzz
    │
    └─ Debit Card
        ├─ Visa/MasterCard
        │   ├─ Amount < ₹2000 ─────────────────────► Easebuzz
        │   └─ Amount ≥ ₹2000 ─────────────────────► JioPay
        │
        ├─ RuPay ──────────────────────────────────► JioPay
        └─ Amex/Diners ────────────────────────────► Easebuzz
```

**Implementation:** `internal/core/payment/payment_orchestrator.go:537-659`

### Routing Examples

| Card Type | Category | Amount | Gateway | Reason |
|-----------|----------|--------|---------|--------|
| Visa | Credit | Any | JioPay | All Visa credit → JioPay |
| Amex | Credit | Any | Easebuzz | Amex credit → Easebuzz |
| Visa | Debit | ₹1,500 | Easebuzz | Debit < ₹2000 → Easebuzz |
| Visa | Debit | ₹5,000 | JioPay | Debit ≥ ₹2000 → JioPay |
| RuPay | Debit | Any | JioPay | All RuPay → JioPay |
| Visa | Corporate | Any | JioPay | All corporate → JioPay |

---

## Penny Transaction Flow

### What is a Penny Transaction?

A penny transaction is a small-value transaction (₹1.00 in this case) used to:
- **Validate** card is active and valid
- **Verify** customer can complete 3DS authentication
- **Obtain** gateway token for the card

### Penny Transaction Amount

**Amount:** ₹1.00 = 100 paise
**Configured at:** `internal/services/payments/payment_service.go:300`

```go
p := &payment.Payment{
    Amount:      100, // ₹1.00 in paise (tokenization charge)
    PaymentType: payment.PaymentTypeTokenization,
    Description: "Card tokenization (₹1 verification charge)",
}
```

### Gateway-Specific Behavior

#### JioPay (Vault API)
- **API:** POST `/qatsp/api/vault/fetch-card`
- **Penny Transaction:** **NOT PERFORMED**
- **Process:**
  1. Send card details (number, expiry, name)
  2. JioPay validates card format
  3. Returns token immediately
  4. No actual charge to customer
- **File:** `internal/adapters/external/payment/jiopay/client.go`

#### Easebuzz (TMS API)
- **API:** POST `https://tms.easebuzz.in/v1/create_token`
- **Penny Transaction:** **₹1.00 IMPLICIT**
- **Process:**
  1. Build tokenization payload with `amount: "1.00"`
  2. Encrypt full payload (AES-256-CBC)
  3. POST encrypted data to TMS
  4. Easebuzz performs ₹1 transaction (may require 3DS)
  5. Returns token after successful transaction
- **File:** `internal/adapters/external/payment/easebuzz/client.go:64-213`
- **Amount field:** Line 125

```go
tokenReq := TokenizationRequest{
    Amount:     "1.00", // Tokenization amount
    // ... other fields
}
```

### Payment Record Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│  Payment Record (payment_type: tokenization)                │
└─────────────────────────────────────────────────────────────┘

Created by Service:
├─ payment_id: UUIDv7 (generated)
├─ user_id: From JWT
├─ business_id: From X-Business-ID header
├─ amount: 100 (paise)
├─ currency: INR
├─ payment_method: card
├─ payment_type: tokenization ◄── Distinguishes from regular payment
├─ status: pending
├─ description: "Card tokenization (₹1 verification charge)"
├─ fee_amount: 0
├─ total_amount: 100
└─ metadata: { card_type, card_category, last_four }

After Tokenization Success:
├─ status: success ◄── Updated
├─ metadata: { card_id, token, gateway_used } ◄── Added
└─ updated_at: timestamp

After Tokenization Failure:
├─ status: failed ◄── Updated
└─ metadata: { error: err.Error() } ◄── Added
```

**Created:** `internal/services/payments/payment_service.go:296-319`
**Updated:** `internal/services/payments/payment_service.go:332-344`

---

## Data Storage

### Database Schema

#### Table: `xcentro.stored_cards`

**Purpose:** Store tokenized card information (PCI DSS compliant)

**Schema:**
```sql
CREATE TABLE xcentro.stored_cards (
    card_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID NOT NULL,

    -- Gateway token (NOT actual card number)
    token TEXT NOT NULL,

    -- Display information only
    last_four_digits VARCHAR(4) NOT NULL,
    card_type xcentro.card_type_enum NOT NULL,
    card_category xcentro.card_category_enum NOT NULL,
    expiry_month VARCHAR(2) NOT NULL,
    expiry_year VARCHAR(4) NOT NULL,
    issuer_bank TEXT,
    card_holder_name TEXT,

    -- Status
    is_active BOOLEAN DEFAULT TRUE,
    gateway_used xcentro.gateway_type_enum NOT NULL,
    added_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,

    -- Metadata
    metadata JSONB
);
```

**Unique Constraint:**
```sql
CREATE UNIQUE INDEX idx_stored_cards_token_unique
ON xcentro.stored_cards(business_id, token);
```

**File:** `internal/adapters/database/postgres/migrations/sql/000002_create_payment_tables.up.sql:35-73`

### What Gets Stored

#### ✅ Stored (Safe)
- **Gateway Token** (e.g., `jio_tok_visa_abc123`) - NOT the actual card number
- **Last 4 Digits** (e.g., `1111`) - for display purposes
- **Card Type** (visa, mastercard, rupay, amex, diners)
- **Card Category** (credit, debit, corporate)
- **Expiry Month/Year** (e.g., `12/25`)
- **Issuer Bank** (e.g., HDFC Bank)
- **Card Holder Name** (e.g., John Doe)
- **Gateway Used** (jiopay, easebuzz)
- **Metadata** (tokenization_date, gateway_used, tokenization_id)

#### ❌ NEVER Stored (PCI DSS)
- **Full Card Number** (PAN) - sent to gateway, discarded immediately
- **CVV** - transient only, never persisted
- **Raw card data** - encrypted in transit, never in database

### Stored Card Entity

**File:** `internal/core/payment/business_wallet_entity.go`

```go
type StoredCard struct {
    CardID         string
    BusinessID     string
    Token          string  // Gateway token - NOT card number
    LastFourDigits string
    CardType       CardType
    CardCategory   CardCategory
    ExpiryMonth    string
    ExpiryYear     string
    IssuerBank     string
    CardHolderName string
    IsActive       bool
    GatewayUsed    GatewayType
    AddedAt        time.Time
    LastUsedAt     *time.Time
    Metadata       map[string]any
}
```

---

## Complete Request-Response Flow

### Step-by-Step Execution

```
┌────────────────────────────────────────────────────────────────┐
│ STEP 1: Client Request                                         │
└────────────────────────────────────────────────────────────────┘

POST /v1/process_payment/tokenize
Headers:
  Authorization: Bearer eyJhbGc...
  X-Business-ID: 0193e8f4-1234-7890-abcd-ef1234567890
  Content-Type: application/json

Body:
{
  "card_details": {
    "card_number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit"
  }
}

┌────────────────────────────────────────────────────────────────┐
│ STEP 2: Handler - Authentication & Validation                 │
│ File: payment_handler.go:292-338                               │
└────────────────────────────────────────────────────────────────┘

→ Extract userID from JWT context
→ Extract businessID from X-Business-ID header
→ Bind and validate request body
→ Validate card details (number, expiry, CVV, holder name)
→ Create 60-second timeout context

┌────────────────────────────────────────────────────────────────┐
│ STEP 3: Service - Create Payment Record                       │
│ File: payment_service.go:287-320                               │
└────────────────────────────────────────────────────────────────┘

→ Generate payment UUID: payment_id = uuid.NewV7()
→ Create payment record:
    {
      payment_id: "0193e8f4-7b2c-7890-a1b2-c3d4e5f6a7b8",
      amount: 100,  // ₹1.00 in paise
      payment_type: "tokenization",
      status: "pending"
    }
→ Save to database (payments table)
→ Set req.TransactionID = payment_id

┌────────────────────────────────────────────────────────────────┐
│ STEP 4: Orchestrator - Gateway Selection                      │
│ File: payment_orchestrator.go:175-257                          │
└────────────────────────────────────────────────────────────────┘

→ Analyze card: Visa + Credit
→ Apply routing rules: Visa Credit → JioPay
→ Selected gateway: jiopay

┌────────────────────────────────────────────────────────────────┐
│ STEP 5: JioPay Gateway - Tokenization                         │
│ File: jiopay/client.go                                         │
└────────────────────────────────────────────────────────────────┘

→ Generate headers:
    x-app-id: APP123
    x-timestamp-id: 20251115143022
    x-trace-id: ABCD1234EFGH5678IJKL9012MNOP3456
    x-app-token: <AES-128-ECB encrypted>

→ POST /qatsp/api/vault/fetch-card
    {
      "cardNumber": "4111111111111111",
      "cardExpiryMonth": "12",
      "cardExpiryYear": "25",
      "nameOnCard": "John Doe"
    }

→ Receive response:
    {
      "status": "SUCCESS",
      "token": "jio_tok_visa_abc123xyz",
      "cardId": "card_jio_12345",
      "lastFourDigits": "1111"
    }

┌────────────────────────────────────────────────────────────────┐
│ STEP 6: Service - Update Payment Status                       │
│ File: payment_service.go:338-349                               │
└────────────────────────────────────────────────────────────────┘

→ Update payment status to "success"
→ Add metadata: { card_id, token, gateway_used: "jiopay" }

┌────────────────────────────────────────────────────────────────┐
│ STEP 7: Handler - Store Card in Wallet                        │
│ File: payment_handler.go:372-400                               │
└────────────────────────────────────────────────────────────────┘

→ Create StoredCard entity:
    {
      card_id: "card_jio_12345",
      business_id: "0193e8f4-1234-7890-abcd-ef1234567890",
      token: "jio_tok_visa_abc123xyz",  // Gateway token, NOT card number
      last_four_digits: "1111",
      card_type: "visa",
      card_category: "credit",
      expiry_month: "12",
      expiry_year: "25",
      gateway_used: "jiopay",
      is_active: true,
      metadata: {
        tokenization_date: "2025-11-15",
        gateway_used: "jiopay",
        tokenization_id: "card_jio_12345"
      }
    }

→ Call walletService.AddPaymentSource()
→ Save to stored_cards table

┌────────────────────────────────────────────────────────────────┐
│ STEP 8: Handler - Return Response                             │
│ File: payment_handler.go:402-424                               │
└────────────────────────────────────────────────────────────────┘

Response (201 Created):
{
  "status": "success",
  "message": "Card tokenized and saved successfully",
  "card_info": {
    "card_id": "card_jio_12345",
    "last_four_digits": "1111",
    "card_type": "visa",
    "card_category": "credit",
    "expiry_month": "12",
    "expiry_year": "25",
    "gateway_used": "jiopay"
  }
}

NOTE: Token is NOT returned to client (backend-only)
```

### Alternative Flow: Easebuzz

If card was Amex Credit (routes to Easebuzz):

```
┌────────────────────────────────────────────────────────────────┐
│ STEP 5 (Alternative): Easebuzz Gateway - Tokenization         │
│ File: easebuzz/client.go:64-213                                │
└────────────────────────────────────────────────────────────────┘

→ Derive encryption keys:
    sKey = SHA256(merchantKey) [32 bytes]
    sIV = SHA256(salt)[:16] [first 16 bytes]

→ Build tokenization payload:
    {
      "card_num": "4111111111111111",
      "expiry_month": "12",
      "expiry_year": "25",
      "cvv": "123",
      "name_on_card": "John Doe",
      "amount": "1.00",  ◄── Penny transaction
      "merchant_key": "ABC123",
      "salt": "XYZ789"
    }

→ Encrypt full payload (AES-256-CBC)
    encData = AES256_CBC_Encrypt(jsonPayload, sKey, sIV)

→ POST https://tms.easebuzz.in/v1/create_token
    Headers:
      EBZ-KEY: <API_KEY>
      Content-Type: application/json
    Body:
      {
        "enc_data": "<base64_encrypted_payload>"
      }

→ Receive encrypted response
→ Decrypt response
    {
      "status": 1,
      "data": {
        "token": "easebuzz_tok_amex_def456",
        "card_id": "card_ebz_67890",
        "last_four": "1111",
        "message": "Token created successfully"
      }
    }

NOTE: Easebuzz performs ₹1 transaction internally
      This may trigger 3DS authentication for customer
```

---

## Security Considerations

### PCI DSS Compliance

✅ **Compliant:**
- Actual card numbers NEVER stored in database
- Only gateway tokens stored
- CVV is transient (in-memory only, never persisted)
- Card data encrypted in transit (HTTPS/TLS)
- Gateway tokens are useless without gateway credentials

❌ **Non-Compliant Alternatives (NOT Used):**
- Storing full PAN in database
- Storing CVV
- Storing card data in logs
- Returning tokens to client

### Data Flow Security

```
Client ──(HTTPS)──► Backend ──(HTTPS)──► Gateway
  │                    │                     │
  │                    │                     │
  └─ Raw Card      ┌───┴────┐           ┌───┴────┐
     (encrypted    │ Memory │           │ Gateway│
      in transit)  │ Only   │           │ Stores │
                   │        │           │ Secure │
                   │ NEVER  │           │ Token  │
                   │ Stored │           └────────┘
                   └────┬───┘
                        │
                        ▼
                   ┌─────────────┐
                   │  Database   │
                   │             │
                   │ Stores:     │
                   │ - Token     │◄── Gateway token, NOT card
                   │ - Last 4    │◄── Display only
                   │ - Card type │
                   └─────────────┘
```

### Authentication & Authorization

**Required Headers:**
1. `Authorization: Bearer <JWT>` - User authentication
2. `X-Business-ID: <UUID>` - Business context

**Validation:**
- JWT verified via middleware
- User must have access to specified business
- Business role authorization (owner, admin, finance)

**Implementation:** `internal/handlers/http/payment/payment_handler.go:295-315`

### Timeouts

| Operation | Timeout | Reason |
|-----------|---------|--------|
| Tokenization | 60s | Gateway API may be slow, 3DS auth may be required |
| Regular Payment | 30s | Faster checkout experience |
| Cryptogram | 30s | Generated on-demand, should be quick |

**Implementation:** `internal/handlers/http/payment/payment_handler.go:341`

```go
ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
defer cancel()
```

---

## Key Files

### Core Implementation

| File | Lines | Purpose |
|------|-------|---------|
| `internal/handlers/http/payment/payment_handler.go` | 291-425 | HTTP handler for tokenization endpoint |
| `internal/services/payments/payment_service.go` | 286-350 | Service layer - creates ₹1 payment record |
| `internal/core/payment/payment_orchestrator.go` | 174-257 | Gateway selection and routing logic |
| `internal/adapters/external/payment/jiopay/client.go` | - | JioPay Vault API integration |
| `internal/adapters/external/payment/easebuzz/client.go` | 64-213 | Easebuzz TMS API integration (₹1 txn) |
| `internal/core/business_rules/wallet_service.go` | 104-264 | Wallet management - stores cards |

### Database

| File | Lines | Purpose |
|------|-------|---------|
| `migrations/sql/000002_create_payment_tables.up.sql` | 35-73 | stored_cards table schema |
| `migrations/sql/000013_add_payment_type.up.sql` | 1-15 | payment_type enum (tokenization) |
| `internal/adapters/database/postgres/payment/stored_card_repository.go` | - | Card CRUD operations |
| `internal/adapters/database/postgres/payment/payment_repository.go` | - | Payment CRUD operations |

### Configuration

| File | Purpose |
|------|---------|
| `.env.example` | Gateway credentials (JIOPAY_*, EASEBUZZ_*) |
| `internal/shared/config/config.go` | Configuration loading |

### Documentation

| File | Purpose |
|------|---------|
| `API_REFERENCE.md` | API endpoint documentation |
| `GATEWAY_INTEGRATION.md` | Gateway technical specs |
| `CLAUDE.md` | Project architecture overview |

---

## Summary

### Key Takeaways

1. **Two Gateways, Different Approaches:**
   - **JioPay:** No penny transaction, direct tokenization
   - **Easebuzz:** ₹1.00 penny transaction implicit in TMS API

2. **Payment Record is Always Created:**
   - Every tokenization creates a `payment_type: tokenization` record
   - Amount is always 100 paise (₹1.00)
   - Status tracks: pending → success/failed

3. **PCI DSS Compliance:**
   - Actual card numbers never stored
   - Only gateway tokens stored
   - CVV is transient (in-memory only)

4. **Gateway Selection is Automatic:**
   - Based on card type and category
   - Handles all major card brands
   - Optimizes for lowest MDR fees

5. **Clean Architecture:**
   - Clear separation of concerns
   - Handler → Service → Orchestrator → Gateway
   - Repository pattern for database access

### Future Enhancements (TODO)

- [ ] Add support for card nickname (currently in metadata)
- [ ] Implement card expiry notifications
- [ ] Add retry logic for failed tokenization
- [ ] Implement card verification status tracking
- [ ] Add audit logging for tokenization attempts

---

**End of Analysis**
