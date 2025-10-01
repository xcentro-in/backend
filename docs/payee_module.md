# Payee Management System

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Database Schema](#database-schema)
4. [Core Entities](#core-entities)
5. [Onboarding Flow](#onboarding-flow)
6. [API Endpoints](#api-endpoints)
7. [External Service Integrations](#external-service-integrations)
8. [Security & Compliance](#security--compliance)
9. [Testing](#testing)
10. [Future Enhancements](#future-enhancements)

## Overview

The Payee Management System enables XCentro customers (businesses) to onboard external businesses (payees) for sending payments. The system performs comprehensive verification through:
- **GST Verification**: Validates GSTIN and fetches canonical business data from government APIs
- **Bank Verification**: Penny drop verification for bank account validation
- **Cross Verification**: Name matching and anti-fraud checks

### Key Principle

**A payee is uniquely identified by their GSTIN (GST Identification Number)**. Multiple customers can add the same payee, but each maintains their own display name, contacts, and metadata.

**Example Scenario**:
- Customer A adds Payee C (GSTIN: 29ABCDE1234F1Z5) with display name "ABC Corp"
- Customer B adds Payee C (same GSTIN) with display name "ABC Industries"
- Both reference the same legal entity, but maintain separate relationship metadata

## Architecture

### Three-Table Normalized Design

The system uses a normalized three-table design to efficiently handle shared payee data while maintaining customer-specific metadata:

```
┌─────────────────┐
│ payee_business  │ ← Global canonical payee (one per GSTIN)
└────────┬────────┘
         │ 1:N
         │
┌────────▼──────────────┐
│ payee_bank_accounts   │ ← Bank accounts (multiple per payee)
└────────┬──────────────┘
         │ N:M
         │
┌────────▼─────────────────────┐
│ business_payee_relation      │ ← Customer-payee-account relationships
└──────────────────────────────┘
```

### 1. Payee Business Table (`payee_business`)

**Purpose**: Stores global, canonical payee information verified through GST API.

**Characteristics**:
- One record per unique GSTIN
- Immutable GST data (legal name, address, etc.)
- Shared across all customers who onboard this payee
- Contains GST verification status and metadata

**Example**:
```
payee_id: 550e8400-e29b-41d4-a716-446655440000
gstin: 29ABCDE1234F1Z5
legal_name: ABC Industries Private Limited
business_type: Private Limited Company
gst_status: Active
```

### 2. Payee Bank Accounts Table (`payee_bank_accounts`)

**Purpose**: Stores verified bank accounts for each payee.

**Characteristics**:
- One-to-many relationship with payee_business
- A payee can have multiple accounts (e.g., Current Account 1, Current Account 2)
- Account hash prevents duplicates per payee
- Encrypted account numbers
- Bank verification status tracked

**Example**:
```
Payee C has:
- Account 1: HDFC Current (verified)
- Account 2: ICICI Current (verified)

Customer A uses Account 1
Customer B uses Account 2
```

### 3. Business Payee Relation Table (`business_payee_relation`)

**Purpose**: Maintains customer-specific metadata for each payee-account relationship.

**Characteristics**:
- Composite primary key: `(business_id, payee_id, account_id)`
- Customer-specific fields: display_name, contacts, notes, favorites, tags
- Soft delete via `is_active` flag
- Tracks usage statistics (`last_used_at`)

**Example**:
```
Business A → Payee C → Account 1:
  display_name: "ABC Corp"
  contact_email: "supplier@example.com"
  is_favorite: true

Business B → Payee C → Account 1:
  display_name: "ABC Industries"
  contact_email: "accounts@example.com"
  is_favorite: false
```

## Database Schema

### Complete Schema Definition

```sql
-- ============================================
-- Verification Status Enum
-- ============================================
CREATE TYPE xcentro.verification_status_enum AS ENUM (
    'pending',
    'in_progress',
    'success',
    'failed'
);

-- ============================================
-- Table 1: Payee Business (Global Canonical)
-- ============================================
CREATE TABLE xcentro.payee_business (
    -- Primary Key
    payee_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- GST Information
    gstin VARCHAR(15) UNIQUE NOT NULL,
    legal_name VARCHAR(500) NOT NULL,
    business_type VARCHAR(100),
    registration_date DATE,
    gst_status VARCHAR(50) NOT NULL,

    -- Primary Address
    building_name VARCHAR(255),
    street VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    pincode VARCHAR(10),

    -- Verification Status
    gst_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    gst_verified_at TIMESTAMPTZ,

    -- Additional Data
    gst_data JSONB,  -- Raw GST API response
    metadata JSONB,  -- Directors, state_code, etc.

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

-- Indexes
CREATE INDEX idx_payee_business_gstin ON xcentro.payee_business(gstin);
CREATE INDEX idx_payee_business_legal_name ON xcentro.payee_business(legal_name);
CREATE INDEX idx_payee_business_gst_status ON xcentro.payee_business(gst_status);

-- ============================================
-- Table 2: Payee Bank Accounts
-- ============================================
CREATE TABLE xcentro.payee_bank_accounts (
    -- Primary Key
    account_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Foreign Key
    payee_id UUID NOT NULL REFERENCES xcentro.payee_business(payee_id) ON DELETE CASCADE,

    -- Bank Account Details (Encrypted)
    account_number_encrypted TEXT NOT NULL,
    account_number_hash VARCHAR(64) NOT NULL,
    ifsc_code VARCHAR(11) NOT NULL,
    bank_name VARCHAR(255) NOT NULL,
    account_type xcentro.bank_account_type_enum NOT NULL,
    account_holder_name VARCHAR(255) NOT NULL,

    -- Verification
    is_verified BOOLEAN DEFAULT FALSE,
    bank_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    bank_verified_at TIMESTAMPTZ,
    bank_verification_data JSONB,

    -- Cross Verification
    cross_verification_status xcentro.verification_status_enum DEFAULT 'pending',
    cross_verified_at TIMESTAMPTZ,
    name_match_score DECIMAL(5,4),

    -- Status & Configuration
    is_active BOOLEAN DEFAULT TRUE,
    is_primary BOOLEAN DEFAULT FALSE,
    account_label VARCHAR(100),

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT unique_payee_account UNIQUE (payee_id, account_number_hash),
    CONSTRAINT check_name_match_score CHECK (name_match_score >= 0 AND name_match_score <= 1)
);

-- Indexes
CREATE INDEX idx_payee_bank_accounts_payee ON xcentro.payee_bank_accounts(payee_id);
CREATE INDEX idx_payee_bank_accounts_verified ON xcentro.payee_bank_accounts(payee_id, is_verified) WHERE is_verified = TRUE;
CREATE INDEX idx_payee_bank_accounts_primary ON xcentro.payee_bank_accounts(payee_id, is_primary) WHERE is_primary = TRUE;

-- ============================================
-- Table 3: Business Payee Relation
-- ============================================
CREATE TABLE xcentro.business_payee_relation (
    -- Composite Primary Key
    business_id UUID NOT NULL REFERENCES xcentro.business(bid) ON DELETE CASCADE,
    payee_id UUID NOT NULL REFERENCES xcentro.payee_business(payee_id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES xcentro.payee_bank_accounts(account_id) ON DELETE CASCADE,

    -- Customer-Specific Metadata
    display_name VARCHAR(255) NOT NULL,
    contact_email VARCHAR(255),
    contact_phone VARCHAR(20),
    notes TEXT,

    -- Organization
    is_active BOOLEAN DEFAULT TRUE,
    is_favorite BOOLEAN DEFAULT FALSE,
    tags TEXT[],

    -- Audit Trail
    added_at TIMESTAMPTZ DEFAULT NOW(),
    added_by UUID,
    last_used_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,

    -- Primary Key
    PRIMARY KEY (business_id, payee_id, account_id)
);

-- Indexes
CREATE INDEX idx_business_payee_relation_business ON xcentro.business_payee_relation(business_id);
CREATE INDEX idx_business_payee_relation_display_name ON xcentro.business_payee_relation(business_id, display_name);
CREATE INDEX idx_business_payee_relation_active ON xcentro.business_payee_relation(business_id, is_active) WHERE is_active = TRUE;
CREATE INDEX idx_business_payee_relation_favorite ON xcentro.business_payee_relation(business_id, is_favorite) WHERE is_favorite = TRUE;
```

## Core Entities

### PayeeBusiness Entity

```go
type PayeeBusiness struct {
    PayeeID              string
    GSTIN                string
    LegalName            string
    BusinessType         string
    RegistrationDate     *time.Time
    GSTStatus            string  // Active, Cancelled, Suspended
    PrimaryAddress       Address
    GSTVerificationStatus VerificationStatus
    GSTVerifiedAt        *time.Time
    GSTData              map[string]any  // Raw API response
    Metadata             map[string]any  // Directors, state_code
    CreatedAt            time.Time
    UpdatedAt            time.Time
}

type Address struct {
    BuildingName string
    Street       string
    City         string
    State        string
    Pincode      string
}
```

### PayeeBankAccount Entity

```go
type PayeeBankAccount struct {
    AccountID                 string
    PayeeID                   string
    AccountNumber             string  // Encrypted
    AccountNumberHash         string  // SHA-256
    IFSCCode                  string
    BankName                  string
    AccountType               payment.BankAccountType
    AccountHolderName         string
    IsVerified                bool
    BankVerificationStatus    VerificationStatus
    CrossVerificationStatus   VerificationStatus
    NameMatchScore            float64
    IsActive                  bool
    IsPrimary                 bool
    AccountLabel              string
    BankVerificationData      map[string]any
    CreatedAt                 time.Time
    UpdatedAt                 time.Time
}
```

### BusinessPayeeRelation Entity

```go
type BusinessPayeeRelation struct {
    BusinessID   string
    PayeeID      string
    AccountID    string
    DisplayName  string
    ContactEmail string
    ContactPhone string
    Notes        string
    IsActive     bool
    IsFavorite   bool
    Tags         []string
    AddedAt      time.Time
    AddedBy      string
    LastUsedAt   *time.Time
    UpdatedAt    time.Time
}
```

### Verification Status Enum

```go
type VerificationStatus string

const (
    VerificationStatusPending    VerificationStatus = "pending"
    VerificationStatusInProgress VerificationStatus = "in_progress"
    VerificationStatusSuccess    VerificationStatus = "success"
    VerificationStatusFailed     VerificationStatus = "failed"
)
```

## Onboarding Flow

### Complete 11-Step Process

```go
func (s *PayeeService) OnboardPayee(ctx context.Context, req OnboardPayeeRequest) (*OnboardPayeeResponse, error) {
    // Step 1: Validate input
    if err := s.validateOnboardRequest(req); err != nil {
        return nil, fmt.Errorf("validation failed: %w", err)
    }

    // Step 2: Check if payee exists by GSTIN
    existingPayee, err := s.payeeBusinessRepo.FindByGSTIN(ctx, req.GSTIN)

    // Step 3: If payee doesn't exist, verify GST and create
    var payeeBusiness *PayeeBusiness
    if existingPayee == nil {
        gstResp, err := s.gstVerificationSvc.VerifyGSTIN(ctx, GSTVerificationRequest{
            GSTIN: req.GSTIN,
        })
        if err != nil {
            return nil, fmt.Errorf("GST verification failed: %w", err)
        }

        payeeBusiness = &PayeeBusiness{
            GSTIN:        req.GSTIN,
            LegalName:    gstResp.LegalName,
            BusinessType: gstResp.BusinessType,
            GSTStatus:    gstResp.Status,
            // ... other fields
            Metadata: map[string]any{
                "directors":  gstResp.Directors,
                "state_code": gstResp.StateCode,
            },
        }

        err = s.payeeBusinessRepo.CreatePayeeBusiness(ctx, payeeBusiness)
    } else {
        payeeBusiness = existingPayee
    }

    payeeID := payeeBusiness.PayeeID

    // Step 4: Compute account hash for duplicate detection
    accountHash := s.computeAccountHash(req.AccountNumber)

    // Step 5: Check for duplicate bank account
    isDuplicate, err := s.payeeBankAccountRepo.CheckDuplicateAccount(ctx, payeeID, accountHash)
    if isDuplicate {
        return nil, fmt.Errorf("bank account already exists for this payee")
    }

    // Step 6: Verify bank account (penny drop)
    bankResp, err := s.bankVerificationSvc.VerifyBankAccount(ctx, BankVerificationRequest{
        AccountNumber:     req.AccountNumber,
        IFSCCode:          req.IFSCCode,
        AccountHolderName: req.AccountHolderName,
    })
    if err != nil {
        return nil, fmt.Errorf("bank verification failed: %w", err)
    }

    // Step 7: Cross-verify GST legal name vs Bank account holder name
    crossResp, err := s.crossVerificationSvc.VerifyGSTBankMatch(ctx, CrossVerificationRequest{
        GSTLegalName:      payeeBusiness.LegalName,
        BankAccountName:   bankResp.AccountHolderName,
    })

    // Step 8: Anti-money rotation check
    directors := extractDirectors(payeeBusiness.Metadata)
    moneyRotationResp, err := s.crossVerificationSvc.CheckMoneyRotation(ctx, MoneyRotationCheckRequest{
        CustomerBusinessID: req.BusinessID,
        PayeeAccountName:   bankResp.AccountHolderName,
        PayeeGSTDirectors:  directors,
    })

    if moneyRotationResp.IsMoneyRotation {
        return nil, fmt.Errorf("money rotation detected: %s", moneyRotationResp.Details)
    }

    // Step 9-10: Atomic transaction - Create account + relation
    var accountID string
    err = s.txManager.WithTx(ctx, func(ctx context.Context, tx client.Tx) error {
        // Create bank account
        account := &PayeeBankAccount{
            PayeeID:                   payeeID,
            AccountNumber:             req.AccountNumber,  // TODO: Encrypt
            AccountNumberHash:         accountHash,
            IFSCCode:                  req.IFSCCode,
            BankVerificationStatus:    VerificationStatusSuccess,
            CrossVerificationStatus:   crossResp.Status,
            NameMatchScore:            crossResp.MatchScore,
            // ... other fields
        }

        accountRepo := s.payeeBankAccountRepo.WithTx(tx)
        if err := accountRepo.CreateBankAccount(ctx, account); err != nil {
            return err
        }
        accountID = account.AccountID

        // Create business-payee relation
        relation := &BusinessPayeeRelation{
            BusinessID:  req.BusinessID,
            PayeeID:     payeeID,
            AccountID:   accountID,
            DisplayName: req.DisplayName,
            // ... other fields
        }

        relationRepo := s.businessPayeeRelationRepo.WithTx(tx)
        return relationRepo.CreateRelation(ctx, relation)
    })

    // Step 11: Return response
    return &OnboardPayeeResponse{
        PayeeID:                  payeeID,
        AccountID:                accountID,
        GSTVerificationStatus:    payeeBusiness.GSTVerificationStatus,
        BankVerificationStatus:   VerificationStatusSuccess,
        CrossVerificationStatus:  crossResp.Status,
        IsFullyVerified:          true,
        Message:                  "Payee fully verified and added",
    }, nil
}
```

### Flow Diagram

```
┌─────────────────────┐
│ 1. Validate Input   │
└──────────┬──────────┘
           │
┌──────────▼──────────────────────┐
│ 2. Check if GSTIN exists        │
└──────────┬──────────────────────┘
           │
       ┌───┴───┐
       │ Exists?│
       └───┬───┘
    No     │      Yes
    ┌──────▼──────┐
    │3. Verify GST│
    │   Create    │
    └──────┬──────┘
           │
┌──────────▼─────────────────────┐
│ 4. Compute Account Hash        │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 5. Check Duplicate Account     │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 6. Bank Verification (Penny)   │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 7. Cross Verify Names          │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 8. Money Rotation Check        │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 9-10. Atomic Transaction       │
│   - Create Bank Account        │
│   - Create Relation            │
└──────────┬─────────────────────┘
           │
┌──────────▼─────────────────────┐
│ 11. Return Response            │
└────────────────────────────────┘
```

## API Endpoints

### Base URL
All payee endpoints are under: `/v1/payees`

### 1. Onboard Payee

**Endpoint**: `POST /v1/payees`

**Description**: Onboard a new payee with complete verification (GST, bank, cross-verification).

**Request Headers**:
```
Authorization: Bearer <JWT_TOKEN>
Content-Type: application/json
```

**Request Body**:
```json
{
  "gstin": "29ABCDE1234F1Z5",
  "account_number": "1234567890123456",
  "ifsc_code": "HDFC0000123",
  "account_holder_name": "ABC Industries Pvt Ltd",
  "bank_name": "HDFC Bank",
  "account_type": "current",
  "account_label": "Primary Current Account",
  "display_name": "ABC Corp",
  "contact_email": "payments@abccorp.com",
  "contact_phone": "+919876543210",
  "notes": "Primary supplier for raw materials"
}
```

**Response** (201 Created):
```json
{
  "message": "Payee onboarded successfully",
  "status": "success",
  "data": {
    "payee_id": "550e8400-e29b-41d4-a716-446655440000",
    "account_id": "660e8400-e29b-41d4-a716-446655440001",
    "gst_verification_status": "success",
    "bank_verification_status": "success",
    "cross_verification_status": "success",
    "is_fully_verified": true,
    "message": "Payee fully verified and added"
  }
}
```

**Error Responses**:
- `400 Bad Request`: Invalid input
- `409 Conflict`: Duplicate payee-account relation
- `422 Unprocessable Entity`: Verification failed
- `500 Internal Server Error`: Server error

### 2. List Payees

**Endpoint**: `GET /v1/payees`

**Description**: List all payees for the authenticated business with optional filters.

**Query Parameters**:
- `display_name` (optional): Filter by display name (case-insensitive, partial match)
- `is_active` (optional): Filter by active status (`true`/`false`)
- `is_favorite` (optional): Filter by favorite status (`true`/`false`)
- `page` (optional, default: 1): Page number for pagination
- `page_size` (optional, default: 10, max: 100): Number of results per page

**Example**:
```bash
GET /v1/payees?display_name=ABC&is_active=true&is_favorite=true&page=1&page_size=20
```

**Response** (200 OK):
```json
{
  "message": "Payees retrieved successfully",
  "status": "success",
  "total_count": 15,
  "page": 1,
  "page_size": 20,
  "data": [
    {
      "business_id": "b12345",
      "payee_id": "p12345",
      "account_id": "a12345",
      "display_name": "ABC Corp",
      "contact_email": "payments@abc.com",
      "contact_phone": "+919876543210",
      "notes": "Primary supplier",
      "is_active": true,
      "is_favorite": true,
      "tags": ["supplier", "verified"],
      "added_at": "2024-01-15T10:30:00Z",
      "last_used_at": "2024-02-20T14:22:00Z"
    }
  ]
}
```

### 3. Get Payee Details

**Endpoint**: `GET /v1/payees/:payee_id/accounts/:account_id`

**Description**: Get detailed information about a specific payee-account relationship.

**Path Parameters**:
- `payee_id`: UUID of the payee
- `account_id`: UUID of the bank account

**Response** (200 OK):
```json
{
  "message": "Payee details retrieved successfully",
  "status": "success",
  "data": {
    "payee_id": "550e8400-e29b-41d4-a716-446655440000",
    "gstin": "29ABCDE1234F1Z5",
    "legal_name": "ABC Industries Private Limited",
    "business_type": "Private Limited Company",
    "gst_status": "Active",
    "account_id": "660e8400-e29b-41d4-a716-446655440001",
    "masked_account": "************3456",
    "bank_name": "HDFC Bank",
    "ifsc_code": "HDFC0000123",
    "account_type": "current",
    "account_label": "Primary Current Account",
    "is_verified": true,
    "display_name": "ABC Corp",
    "contact_email": "payments@abccorp.com",
    "contact_phone": "+919876543210",
    "notes": "Primary supplier",
    "is_favorite": true,
    "tags": ["supplier", "verified", "important"],
    "gst_verification_status": "success",
    "bank_verification_status": "success",
    "cross_verification_status": "success",
    "added_at": "2024-01-15T10:30:00Z",
    "last_used_at": "2024-02-20T14:22:00Z"
  }
}
```

**Error Responses**:
- `404 Not Found`: Payee or account not found

### 4. Update Payee Metadata

**Endpoint**: `PUT /v1/payees/:payee_id/accounts/:account_id`

**Description**: Update customer-specific metadata for a payee relationship (display name, contacts, favorites, tags).

**Request Body**:
```json
{
  "display_name": "ABC Industries",
  "contact_email": "newcontact@abc.com",
  "contact_phone": "+919999999999",
  "notes": "Updated supplier contact",
  "is_favorite": true,
  "tags": ["supplier", "verified", "critical"]
}
```

**Response** (200 OK):
```json
{
  "message": "Payee updated successfully",
  "status": "success"
}
```

### 5. Deactivate Payee

**Endpoint**: `DELETE /v1/payees/:payee_id/accounts/:account_id`

**Description**: Soft delete (deactivate) a payee relationship. Sets `is_active = false`.

**Response** (200 OK):
```json
{
  "message": "Payee deactivated successfully",
  "status": "success"
}
```

**Note**: This is a soft delete. The relation still exists in the database but won't appear in list queries.

## External Service Integrations

### GST Verification Service

**Interface**: `GSTVerificationService`

**Location**: `internal/core/business_rules/gst_verification_service.go`

**Purpose**: Verify GSTIN and fetch canonical business data from government GST APIs.

```go
type GSTVerificationService interface {
    VerifyGSTIN(ctx context.Context, request GSTVerificationRequest) (*GSTVerificationResponse, error)
    GetProviderType() GSTVerificationProvider
}

type GSTVerificationRequest struct {
    GSTIN string
}

type GSTVerificationResponse struct {
    RequestID        string
    GSTIN            string
    LegalName        string
    TradeName        string
    BusinessType     string
    RegistrationDate time.Time
    Status           string  // Active, Cancelled, Suspended
    StateCode        string
    Address          GSTAddress
    Directors        []GSTDirector
    Metadata         map[string]any
}
```

**Providers**:
- **AuthBridge** (production): Real GST API integration
- **Mock** (development/testing): Returns hardcoded success

**Mock Behavior**:
```go
// Returns after 500ms delay
{
    LegalName: "ABC Industries Private Limited",
    BusinessType: "Private Limited Company",
    Status: "Active",
    Directors: []GSTDirector{
        {Name: "Rajesh Kumar", DIN: "12345678"},
        {Name: "Priya Sharma", DIN: "87654321"},
    },
}
```

### Bank Verification Service

**Interface**: `BankVerificationService`

**Location**: `internal/core/business_rules/bank_verification_service.go`

**Purpose**: Penny drop verification for bank account validation.

**Reused from Payment Module**: Same service used for payment source verification.

**Test Patterns** (Mock):
- `****0000`: Account closed (ACCOUNT_CLOSED error)
- `****1111`: Invalid account (INVALID_ACCOUNT error)
- `****2222`: Name mismatch (NAME_MISMATCH error)
- `****9999`: API timeout (API_TIMEOUT error)
- Other: Success

### Cross Verification Service

**Interface**: `CrossVerificationService`

**Location**: `internal/core/business_rules/cross_verification_service.go`

**Purpose**: Name matching and anti-fraud checks.

```go
type CrossVerificationService interface {
    VerifyGSTBankMatch(ctx context.Context, request CrossVerificationRequest) (*CrossVerificationResponse, error)
    CheckMoneyRotation(ctx context.Context, request MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error)
}
```

**Method 1: GST-Bank Name Matching**
```go
type CrossVerificationRequest struct {
    GSTLegalName    string
    BankAccountName string
}

type CrossVerificationResponse struct {
    Status     VerificationStatus
    MatchScore float64  // 0.0 to 1.0
    Details    string
}
```

**Mock Behavior**: Returns match score of 0.95 (high confidence)

**Method 2: Money Rotation Detection**
```go
type MoneyRotationCheckRequest struct {
    CustomerBusinessID string
    PayeeAccountName   string
    PayeeGSTDirectors  []string
}

type MoneyRotationCheckResponse struct {
    IsMoneyRotation    bool
    MatchedDirector    string
    RecommendedAction  string  // "approve", "reject"
    Details            string
}
```

**Mock Behavior**: Checks if "Kumar" appears in both names (for testing)

**Production**: Will integrate with 3rd party fuzzy matching and director verification APIs.

## Security & Compliance

### Duplicate Prevention

**GSTIN Uniqueness**:
```sql
ALTER TABLE xcentro.payee_business
ADD CONSTRAINT unique_gstin UNIQUE (gstin);
```

**Account Hash Uniqueness (per payee)**:
```sql
ALTER TABLE xcentro.payee_bank_accounts
ADD CONSTRAINT unique_payee_account UNIQUE (payee_id, account_number_hash);
```

**Relation Composite Key**:
```sql
PRIMARY KEY (business_id, payee_id, account_id)
```

### Data Encryption

**Current**:
- Account numbers stored in plain text (TODO: Phase 5.5)
- SHA-256 hashing for duplicate detection

**Phase 5.5 Enhancements**:
- AES-256 encryption for account numbers
- Encrypted at application layer before storage
- Decryption only when displaying (last 4 digits masked)

### Anti-Fraud Measures

**1. GST Verification**:
- Validates GSTIN via government API
- Ensures payee is a legitimate registered business
- Checks GST status (Active/Cancelled/Suspended)

**2. Bank Verification**:
- Penny drop verification
- Confirms account number, IFSC, holder name match

**3. Name Matching**:
- Fuzzy matching between GST legal name and bank holder name
- Match score threshold (e.g., > 0.80)
- Flags mismatches for manual review

**4. Money Rotation Detection**:
- Checks if payee account holder is a director of customer business
- Prevents fraudulent circular payments
- Uses director information from GST API

### Audit Trail

**Relation Tracking**:
```go
added_at      TIMESTAMPTZ  // When relation was created
added_by      UUID         // User who added the payee
last_used_at  TIMESTAMPTZ  // Last payment to this payee
updated_at    TIMESTAMPTZ  // Last metadata update
```

**Bank Account Tracking**:
```go
bank_verified_at          TIMESTAMPTZ
cross_verified_at         TIMESTAMPTZ
bank_verification_data    JSONB  // Raw API response
```

**Compliance Requirements**:
- Never hard delete (use `is_active` flag)
- Maintain full verification history
- Store raw API responses in JSONB fields

### JWT Authentication

**Current Implementation**:
```go
// TODO: Extract from JWT token claims
businessID := "bid12345"  // Hardcoded
```

**Phase 5.7 Enhancement**:
```go
// Extract business_id from JWT
claims := c.Get("claims").(jwt.MapClaims)
businessID := claims["business_id"].(string)
```

**Required Middleware**:
- JWT validation middleware on all payee routes
- Business ID extraction from token
- Role-based access control (future)

## Testing

### Mock Services

**1. Mock GST Verification** (`mock_gst_verification.go`):
```go
type MockGSTVerificationService struct{}

func (m *MockGSTVerificationService) VerifyGSTIN(ctx context.Context, req GSTVerificationRequest) (*GSTVerificationResponse, error) {
    time.Sleep(500 * time.Millisecond)  // Simulate API delay

    return &GSTVerificationResponse{
        RequestID:    "mock-request-12345",
        GSTIN:        req.GSTIN,
        LegalName:    "ABC Industries Private Limited",
        BusinessType: "Private Limited Company",
        Status:       "Active",
        Directors: []GSTDirector{
            {Name: "Rajesh Kumar", DIN: "12345678"},
            {Name: "Priya Sharma", DIN: "87654321"},
        },
    }, nil
}
```

**2. Mock Cross Verification** (`mock_cross_verification.go`):
```go
type MockCrossVerificationService struct{}

func (m *MockCrossVerificationService) VerifyGSTBankMatch(ctx context.Context, req CrossVerificationRequest) (*CrossVerificationResponse, error) {
    time.Sleep(300 * time.Millisecond)

    return &CrossVerificationResponse{
        Status:     VerificationStatusSuccess,
        MatchScore: 0.95,
        Details:    "High confidence match",
    }, nil
}

func (m *MockCrossVerificationService) CheckMoneyRotation(ctx context.Context, req MoneyRotationCheckRequest) (*MoneyRotationCheckResponse, error) {
    // Simple test: Check if "Kumar" appears in both
    isRotation := strings.Contains(req.PayeeAccountName, "Kumar") &&
                  len(req.PayeeGSTDirectors) > 0 &&
                  strings.Contains(req.PayeeGSTDirectors[0], "Kumar")

    return &MoneyRotationCheckResponse{
        IsMoneyRotation:   isRotation,
        RecommendedAction: "approve",
    }, nil
}
```

### Integration Testing

**Test Scenarios**:

1. **New Payee Onboarding**:
```bash
curl -X POST http://localhost:8080/v1/payees \
  -H "Authorization: Bearer test-jwt" \
  -d '{
    "gstin": "29ABCDE1234F1Z5",
    "account_number": "1234567890123456",
    ...
  }'
```

2. **Duplicate GSTIN (Existing Payee)**:
```bash
# Customer A adds Payee C
POST /v1/payees { gstin: "29ABCDE1234F1Z5", display_name: "ABC Corp" }

# Customer B adds same Payee C
POST /v1/payees { gstin: "29ABCDE1234F1Z5", display_name: "ABC Industries" }

# Both should succeed with same payee_id but different relations
```

3. **Duplicate Bank Account**:
```bash
# Add Account 1 for Payee C
POST /v1/payees { payee_id: "p123", account_number: "1234567890" }

# Try to add same account again
POST /v1/payees { payee_id: "p123", account_number: "1234567890" }
# Should fail with 409 Conflict
```

4. **Bank Verification Failure**:
```bash
# Use test account ending in 0000
POST /v1/payees { account_number: "1234567890120000" }
# Should fail with ACCOUNT_CLOSED error
```

5. **Money Rotation Detection**:
```bash
# Add payee where account holder matches customer director
POST /v1/payees {
  payee_account_name: "Rajesh Kumar",
  # Customer business has director "Rajesh Kumar"
}
# Should fail with money rotation detected
```

### Unit Testing

**Repository Tests**:
```go
func TestPayeeBusinessRepository_CreatePayeeBusiness(t *testing.T) {
    // Test creating new payee
}

func TestPayeeBusinessRepository_FindByGSTIN(t *testing.T) {
    // Test finding existing payee
}

func TestPayeeBankAccountRepository_CheckDuplicateAccount(t *testing.T) {
    // Test duplicate detection
}
```

**Service Tests**:
```go
func TestPayeeService_OnboardPayee_Success(t *testing.T) {
    // Test successful onboarding
}

func TestPayeeService_OnboardPayee_DuplicateAccount(t *testing.T) {
    // Test duplicate account rejection
}

func TestPayeeService_OnboardPayee_MoneyRotation(t *testing.T) {
    // Test money rotation detection
}
```

## Future Enhancements

### Phase 5.5: Encryption & Security
- **AES-256 encryption** for bank account numbers
- **Secure key management** using environment variables or key vaults
- **Encrypted at rest** in database
- **Decrypted only** for display (masked)

### Phase 5.6: Testing & Validation
- **Integration tests** for full onboarding flow
- **Transaction rollback tests**
- **Load testing** for concurrent onboarding
- **Error scenario coverage**

### Phase 5.7: Production Readiness
- **AuthBridge GST API** integration
- **Real cross-verification** service integration
- **JWT extraction** for business_id
- **Authentication middleware** on all routes
- **Rate limiting** for external API calls
- **Comprehensive error handling**

### Phase 5.8: Advanced Features
- **Bulk payee import** via CSV/Excel
- **Payee validation** before onboarding (check blacklists)
- **Payee search** by GSTIN, legal name, bank account
- **Payment history** tracking per payee
- **Payee analytics** (most used, payment volumes, etc.)
- **Notification system** for verification failures

### Phase 5.9: Compliance & Reporting
- **Audit log export** for compliance
- **Payee verification reports**
- **Failed verification tracking**
- **Regulatory reporting** (AML/KYC compliance)
- **Data retention policies**

### Phase 5.10: Performance Optimization
- **Database indexing** review and optimization
- **Caching** frequently accessed payees
- **Batch verification** for multiple payees
- **Async verification** for non-blocking onboarding

---

## Quick Reference

### File Locations

**Core Domain**:
- Entities: `internal/core/payee/payee_entity.go`
- DTOs: `internal/core/payee/payee_dto.go`
- Repositories: `internal/core/payee/payee_repository.go`

**Service Layer**:
- Service: `internal/services/payee/payee_service.go`

**Adapters**:
- Repositories: `internal/adapters/database/postgres/payee/`
- GST Verification: `internal/core/business_rules/gst_verification_service.go`
- Cross Verification: `internal/core/business_rules/cross_verification_service.go`

**HTTP Layer**:
- Handler: `internal/handlers/http/payee/payee_handler.go`
- Routes: `internal/handlers/http/payee/routes.go`
- Wire-up: `cmd/api/routes.go`

**Database**:
- Migration: `internal/adapters/database/postgres/migrations/sql/000003_create_payee_tables.up.sql`
- Rollback: `internal/adapters/database/postgres/migrations/sql/000003_create_payee_tables.down.sql`

### Common Commands

```bash
# Run migrations
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up

# Build API
go build -o bin/api ./cmd/api

# Run API
go run ./cmd/api

# Test payee service
go test ./internal/services/payee/... -v

# Test repositories
go test ./internal/adapters/database/postgres/payee/... -v
```

### Import Paths

```go
import (
    "xcentro.in/internal/core/payee"
    "xcentro.in/internal/services/payee"
    "xcentro.in/internal/handlers/http/payee"
    "xcentro.in/internal/adapters/database/postgres/payee"
    "xcentro.in/internal/core/business_rules"
)
```
