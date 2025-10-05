# Phase 9: Payout Transfer Orchestration - Implementation Status

## Overview

Phase 9 implements automated payout processing with webhook-based status tracking. This system transfers funds from escrow to payee bank accounts after payment settlement.

**Status**: ⚠️ Partially Complete (MVP Structure Ready)

---

## ✅ Completed Components

### 1. Phase 9.0: Documentation
**File**: `docs/payout_workflow.md`

Comprehensive 400+ line documentation covering:
- Complete architecture with webhook flow
- Castler integration details (two-step approval, webhook events)
- Approval workflow (₹2L auto-approval limit)
- Database schema explanation
- Worker tasks breakdown
- API endpoints documentation
- Troubleshooting guide

### 2. Phase 9.1: Database Schema (Migration 000009)
**Files**:
- `internal/adapters/database/postgres/migrations/sql/000009_add_payout_transfer_requests.up.sql`
- `internal/adapters/database/postgres/migrations/sql/000009_add_payout_transfer_requests.down.sql`

**Created**:
```sql
-- Enums
CREATE TYPE payout_transfer_status_enum AS ENUM (
    'pending_approval', 'approved', 'initiated',
    'processing', 'completed', 'failed',
    'cancelled', 'timeout'
);

-- Tables
CREATE TABLE payout_transfer_requests (
    transfer_request_id UUID PRIMARY KEY,
    voucher_id UUID UNIQUE NOT NULL,
    provider VARCHAR(50),
    provider_beneficiary_id VARCHAR(255),
    provider_transfer_id VARCHAR(255),
    amount BIGINT NOT NULL,
    status payout_transfer_status_enum,
    webhook_history JSONB[],  -- Array of webhook payloads
    webhook_timeout_at TIMESTAMPTZ,
    retry_count INT DEFAULT 0,
    ...
);

CREATE TABLE payout_configuration (
    config_id UUID PRIMARY KEY,
    business_id UUID,  -- NULL = global default
    auto_approval_limit BIGINT DEFAULT 20000000,  -- ₹2,00,000
    preferred_transfer_mode VARCHAR(50) DEFAULT 'NEFT',
    webhook_timeout_minutes INT DEFAULT 60,
    ...
);
```

**Migration Status**: ✅ Applied successfully

### 3. Phase 9.3: Payout Service Layer
**File**: `internal/services/payout/payout_service.go`

**Implemented Methods**:
```go
type PayoutService struct {
    voucherRepo     voucher.VoucherRepository
    transferRepo    payout.PayoutTransferRepository
    mappingRepo     payout.PayoutProviderMappingRepository
    configRepo      payout.PayoutConfigurationRepository
    providerFactory PayoutProviderFactory
}

// Core methods
InitiatePayoutForVoucher(voucherID string) (*PayoutTransferRequest, error)
ApproveTransfer(transferRequestID, approvedBy, remarks string) error
CancelTransfer(transferRequestID, cancelledBy, reason string) error
GetTransferDetails(transferRequestID string) (*PayoutTransferRequest, error)
GetWebhookHistory(transferRequestID string) ([]map[string]interface{}, error)

// Placeholder methods (TODO for future)
RetryTransfer(transferRequestID, triggeredBy string) error  // PLACEHOLDER
MarkCompletedManually(transferRequestID, utr, completedBy, remarks string) error
```

**Auto-Approval Logic**:
- Amount ≤ ₹2,00,000 → Auto-approved (status='approved', approvedBy='SYSTEM')
- Amount > ₹2,00,000 → Pending approval (status='pending_approval')

### 4. Phase 9.4: Payout Worker Orchestrator
**Files**:
- `internal/workers/payout_worker.go` - Worker logic
- `cmd/payout_worker/main.go` - Entry point

**Worker Tasks** (runs every 5 minutes):
```go
type PayoutWorker struct {
    voucherRepo     voucher.VoucherRepository
    transferRepo    payout.PayoutTransferRepository
    mappingRepo     payout.PayoutProviderMappingRepository
    configRepo      payout.PayoutConfigurationRepository
    payoutService   *PayoutService
    providerFactory PayoutProviderFactory
}

// Implemented tasks
1. processReadyVouchers()        // PLACEHOLDER - manual API initiation for MVP
2. initiateApprovedTransfers()   // ✅ Calls Castler API, handles two-step approval
3. flagTimeoutTransfers()        // ✅ Marks as timeout after 60min
4. retryFailedTransfers()        // PLACEHOLDER - needs retry strategy design
```

**Binary**: ✅ Compiles successfully (`bin/payout_worker`)

### 5. Updated Core Entities

**Added `PayoutStatus` enum** (`internal/core/voucher/voucher_entity.go`):
```go
type PayoutStatus string

const (
    PayoutStatusPending      PayoutStatus = "pending"
    PayoutStatusInitiated    PayoutStatus = "initiated"
    PayoutStatusProcessing   PayoutStatus = "processing"
    PayoutStatusCompleted    PayoutStatus = "completed"
    PayoutStatusFailed       PayoutStatus = "failed"
    PayoutStatusNeedsAttention PayoutStatus = "needs_attention"
)
```

**Updated `PayoutTransferRequest`** (`internal/core/payout/payout_entity.go`):
- Added webhook tracking fields (WebhookHistory, LastWebhookAt, WebhookTimeoutAt)
- Added retry tracking fields (RetryCount, MaxRetries, NextRetryAt)
- Added status timestamps (InitiatedAt, ProcessingAt, CompletedAt, FailedAt)

**Added `PayoutConfiguration`** (`internal/core/payout/payout_entity.go`):
```go
type PayoutConfiguration struct {
    ConfigID                string
    BusinessID              *string  // NULL = global default
    AutoApprovalEnabled     bool
    AutoApprovalLimit       int64    // 20000000 paise = ₹2,00,000
    PreferredTransferMode   string   // NEFT, RTGS, IMPS
    WebhookTimeoutMinutes   int      // 60 minutes
    MaxRetryAttempts        int      // 3 (placeholder)
    ...
}
```

**Updated `TransferStatusType` constants**:
```go
const (
    TransferStatusPendingApproval
    TransferStatusApproved
    TransferStatusInitiated
    TransferStatusProcessing
    TransferStatusCompleted
    TransferStatusFailed
    TransferStatusCancelled
    TransferStatusTimeout
)
```

---

## ⏳ Pending Components

### Phase 9.2: Webhook Handler for Castler (NOT STARTED)
**Needs to be created**: `internal/handlers/http/payout/webhook_handler.go`

**Required Implementation**:
```go
type CastlerWebhookPayload struct {
    EventType    string    // "transfer.processing", "transfer.completed", "transfer.failed"
    TransferID   string    // Castler's transfer ID
    Status       string    // "processing", "success", "failed"
    UTR          string    // Bank UTR (on success)
    FailureCode  string    // Error code (on failure)
    FailureMsg   string
    Timestamp    time.Time
    Metadata     map[string]interface{}
}

// Handler methods
HandleCastlerWebhook(c echo.Context) error
  → Verify signature (HMAC-SHA256)
  → Parse payload
  → Find transfer by provider_transfer_id
  → Handle event type:
      - "transfer.processing" → update status
      - "transfer.completed" → mark completed with UTR
      - "transfer.failed" → mark failed (+ retry placeholder)
  → Store webhook in history array
  → Update voucher payout status

// Webhook endpoint
POST /api/v1/webhooks/payout/castler
```

**Webhook Signature Verification**:
```go
signature := request.Header.Get("X-Castler-Signature")
expectedSignature := hmac.SHA256(castlerWebhookSecret, requestBody)

if signature != expectedSignature {
    return 401 Unauthorized
}
```

### Phase 9.5: Operations Dashboard Endpoints (NOT STARTED)
**Needs to be created**: `internal/handlers/http/payout/payout_handler.go`

**Required Endpoints**:
```
GET  /api/v1/operations/payouts/pending-approvals
  → List transfers with status='pending_approval'
  → Show voucher, payee, amount details
  → Pagination support

POST /api/v1/operations/payouts/:transfer_id/approve
  → Approve pending transfer
  → Body: { "approved_by": "user_uuid", "remarks": "..." }

POST /api/v1/operations/payouts/:transfer_id/cancel
  → Cancel pending transfer
  → Only if status = 'pending_approval'

GET  /api/v1/operations/payouts/:transfer_id
  → Get transfer details with timeline
  → Show webhook history

GET  /api/v1/operations/payouts/:transfer_id/webhooks
  → View all webhook payloads for debugging

GET  /api/v1/operations/payouts/timeout
  → List transfers flagged as timeout
  → Needs manual investigation

POST /api/v1/operations/payouts/:transfer_id/mark-completed
  → Manual completion for timeout transfers
  → After external verification

GET  /api/v1/operations/payouts/config
PUT  /api/v1/operations/payouts/config
  → Get/update payout configuration
  → Auto-approval limit, transfer mode, etc.
```

---

## 🚧 Missing Repository Implementations

### 1. PayoutTransferRepository (HIGH PRIORITY)
**Needs to be created**: `internal/adapters/database/postgres/payout/transfer_repository.go`

**Required Methods**:
```go
type PostgresTransferRepository struct {
    querier client.Querier
}

func NewPostgresTransferRepository(db client.DB) payout.PayoutTransferRepository {
    return &PostgresTransferRepository{querier: db}
}

// Core CRUD
Create(ctx, transfer *PayoutTransferRequest) error
GetByID(ctx, transferRequestID string) (*PayoutTransferRequest, error)
GetByVoucherID(ctx, voucherID string) (*PayoutTransferRequest, error)
GetByProviderTransferID(ctx, providerTransferID string) (*PayoutTransferRequest, error)

// Worker queries
GetByStatus(ctx, status TransferStatusType, limit int) ([]*PayoutTransferRequest, error)
GetWebhookTimeouts(ctx, limit int) ([]*PayoutTransferRequest, error)

// Status updates
UpdateStatus(ctx, transferRequestID string, status TransferStatusType, timestamp time.Time) error
UpdateProviderTransferID(ctx, transferRequestID, providerTransferID string) error
SetWebhookTimeout(ctx, transferRequestID string, timeoutAt time.Time) error

// Approval
ApproveTransfer(ctx, transferRequestID, approvedBy string, approvalMode ApprovalMode, timestamp time.Time) error

// Completion/Failure
MarkCompleted(ctx, transferRequestID, utr string, completedAt time.Time) error
MarkFailed(ctx, transferRequestID, failureReason string, failedAt time.Time) error

// Webhook
AppendWebhookHistory(ctx, transferRequestID string, payload map[string]interface{}) error

// Operations dashboard
GetPendingApprovals(ctx, limit int) ([]*PayoutTransferRequest, error)
```

**Key Query Examples**:
```sql
-- Get by status (for worker)
SELECT * FROM payout_transfer_requests
WHERE status = $1
ORDER BY created_at ASC
LIMIT $2;

-- Get webhook timeouts (for worker)
SELECT * FROM payout_transfer_requests
WHERE status = 'initiated'
  AND webhook_timeout_at < NOW()
LIMIT $1;

-- Append webhook to history array
UPDATE payout_transfer_requests
SET webhook_history = array_append(webhook_history, $2::JSONB),
    last_webhook_at = NOW()
WHERE transfer_request_id = $1;
```

### 2. PayoutConfigurationRepository
**Needs to be created**: `internal/adapters/database/postgres/payout/configuration_repository.go`

**Required Methods**:
```go
type PostgresConfigurationRepository struct {
    querier client.Querier
}

func NewPostgresConfigurationRepository(db client.DB) payout.PayoutConfigurationRepository {
    return &PostgresConfigurationRepository{querier: db}
}

GetConfig(ctx, businessID *string) (*PayoutConfiguration, error)
  → If businessID != nil, try business-specific config
  → Fallback to global config (where business_id IS NULL)

UpdateConfig(ctx, config *PayoutConfiguration) error
Create(ctx, config *PayoutConfiguration) error
```

**Query Logic**:
```sql
-- Get config with fallback
SELECT * FROM payout_configuration
WHERE business_id = $1
UNION ALL
SELECT * FROM payout_configuration
WHERE business_id IS NULL
ORDER BY business_id NULLS LAST  -- Business-specific first, then global
LIMIT 1;
```

### 3. VoucherRepository Extension
**File to update**: `internal/adapters/database/postgres/voucher/voucher_repository.go`

**Missing Method** (already added to interface, needs implementation):
```go
// GetVouchersReadyForPayout retrieves vouchers ready for payout (for worker)
GetVouchersReadyForPayout(ctx context.Context, limit int) ([]*Voucher, error)
```

**Query**:
```sql
SELECT v.*
FROM payment_vouchers v
JOIN payments p ON v.payment_id = p.payment_id
WHERE p.settlement_status = 'settled'
  AND v.payment_id IS NOT NULL
  AND v.payout_status = 'pending'
  AND v.cancellation_status = 'not_cancelled'
  AND NOT EXISTS (
      SELECT 1 FROM payout_transfer_requests ptr
      WHERE ptr.voucher_id = v.voucher_id::TEXT
  )
ORDER BY v.voucher_date ASC
LIMIT $1;
```

---

## 📝 Configuration Updates Needed

### 1. Add Castler Config to `config.Config`
**File**: `internal/shared/config/config.go`

**Add**:
```go
type Config struct {
    // ... existing fields

    // Payout configuration (Phase 9)
    Castler CastlerConfig `json:"castler"`
}

type CastlerConfig struct {
    BaseURL      string `json:"base_url" env:"CASTLER_BASE_URL" env-required:"true"`
    APIKey       string `json:"api_key" env:"CASTLER_API_KEY" env-required:"true"`
    SecretKey    string `json:"secret_key" env:"CASTLER_SECRET_KEY" env-required:"true"`
    WebhookSecret string `json:"webhook_secret" env:"CASTLER_WEBHOOK_SECRET" env-required:"true"`
    Environment  string `json:"environment" env:"CASTLER_ENVIRONMENT" env-default:"sandbox"`
}
```

### 2. Environment Variables
**Add to `.env`**:
```bash
# Castler Payout Configuration
CASTLER_BASE_URL=https://sandbox.castler.in
CASTLER_API_KEY=your_api_key
CASTLER_SECRET_KEY=your_secret_key
CASTLER_WEBHOOK_SECRET=your_webhook_secret
CASTLER_ENVIRONMENT=sandbox
```

---

## 🔄 Retry Strategy (Deferred to Post-MVP)

**Current Status**: Placeholder implementations with TODO messages

**Places with Retry Placeholders**:
1. `PayoutService.RetryTransfer()` - prints TODO message
2. `PayoutWorker.retryFailedTransfers()` - prints TODO message
3. Webhook handler on `transfer.failed` - needs retry logic

**Design Considerations for Future**:
```go
type RetryPolicy struct {
    MaxAttempts       int           // 3
    InitialDelay      time.Duration // 15 minutes
    BackoffMultiplier float64       // 2.0 (exponential: 15, 30, 60 min)
    MaxDelay          time.Duration // 2 hours
    RetryableErrors   []string      // Specific error codes
}

// Unified retry strategy needed across:
// - Payment gateway failures
// - Payout provider failures
// - Bank verification timeouts
// - GST verification failures
// - Webhook delivery failures
```

**Future Tasks**:
- [ ] Design unified retry configuration
- [ ] Implement exponential backoff
- [ ] Add circuit breaker pattern
- [ ] Create dead letter queue for max failures
- [ ] Implement retry worker (separate from payout worker)

---

## 🧪 Testing Instructions (Once Complete)

### 1. Database Setup
```bash
# Apply migration
./bin/migrate -action=up

# Verify tables created
psql -h localhost -U xcentro_rw -d xcentro \
  -c "SELECT * FROM payout_configuration;"
```

### 2. Worker Testing
```bash
# Build worker
go build -o bin/payout_worker ./cmd/payout_worker

# Run worker (once repositories are implemented)
./bin/payout_worker

# Should see:
# 🚀 Starting Payout Worker...
# ✅ Database connection established
# 🔄 Running initial worker cycle...
# 📋 Task 1: Processing ready vouchers...
# 🚀 Task 2: Initiating approved transfers...
# ⏰ Task 3: Flagging timeout transfers...
# 🔄 Task 4: Retrying failed transfers... (TODO)
```

### 3. Service Testing (Manual API)
```bash
# Test InitiatePayoutForVoucher
curl -X POST http://localhost:8080/api/v1/payouts/initiate \
  -H "Content-Type: application/json" \
  -d '{"voucher_id": "uuid-here"}'

# Expected flow:
# 1. Validates voucher is settled and paid
# 2. Checks auto-approval limit (₹2L)
# 3. Creates transfer request
# 4. Returns: {"transfer_id": "...", "status": "approved|pending_approval"}
```

### 4. Webhook Testing (Once Handler Created)
```bash
# Simulate Castler webhook
curl -X POST http://localhost:8080/api/v1/webhooks/payout/castler \
  -H "X-Castler-Signature: <hmac-signature>" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type": "transfer.completed",
    "transfer_id": "txn_xyz123",
    "status": "success",
    "utr": "HDFC24011512345678",
    "timestamp": "2024-01-15T10:45:00Z"
  }'

# Should update:
# - transfer status to 'completed'
# - voucher payout_status to 'completed'
# - append webhook to history array
```

### 5. Timeout Testing
```bash
# Manually set webhook_timeout_at in past
psql -c "UPDATE payout_transfer_requests
         SET webhook_timeout_at = NOW() - INTERVAL '1 hour'
         WHERE transfer_request_id = 'uuid-here';"

# Run worker - should flag as timeout
./bin/payout_worker

# Verify status changed to 'timeout'
psql -c "SELECT transfer_request_id, status FROM payout_transfer_requests;"
```

---

## 📊 Current System Flow

```
Payment Settlement (Phase 7)
    ↓
Voucher Ready (payment_id NOT NULL, payout_status='pending')
    ↓
[Manual API Call] InitiatePayoutForVoucher  ← TODO: Automate via worker
    ↓
Transfer Request Created
    ├─ Amount ≤ ₹2L → status='approved'
    └─ Amount > ₹2L → status='pending_approval' → [Manual] Approve via API
    ↓
Worker Task 2: Initiate Transfer
    ↓
    ├─ Call Castler API (create transfer)
    ├─ Two-step: Approve transfer
    ├─ Update status='initiated'
    ├─ Set webhook_timeout_at = NOW() + 60min
    └─ Update voucher payout_status='initiated'
    ↓
Wait for Webhook (Phase 9.2 - NOT IMPLEMENTED)
    ├─ "transfer.processing" → status='processing'
    ├─ "transfer.completed" → status='completed' ✅
    └─ "transfer.failed" → status='failed' (TODO: retry)
    ↓
Worker Task 3: Flag Timeouts
    └─ If no webhook after 60min → status='timeout' 🚨
```

---

## 🚀 Next Steps to Complete Phase 9

### Priority 1: Repository Implementations (CRITICAL)
1. ✅ Already done: `PostgresProviderMappingRepository`
2. ⏳ **TODO**: `PostgresTransferRepository` (~300 lines)
3. ⏳ **TODO**: `PostgresConfigurationRepository` (~100 lines)
4. ⏳ **TODO**: `VoucherRepository.GetVouchersReadyForPayout()` (~30 lines)

### Priority 2: Webhook Handler (CRITICAL)
1. ⏳ **TODO**: Create `webhook_handler.go` (~200 lines)
2. ⏳ **TODO**: Implement signature verification (HMAC-SHA256)
3. ⏳ **TODO**: Handle 3 webhook events (processing, completed, failed)
4. ⏳ **TODO**: Update transfer + voucher status
5. ⏳ **TODO**: Store webhook in history array

### Priority 3: Operations Dashboard (HIGH)
1. ⏳ **TODO**: Create `payout_handler.go` (~400 lines)
2. ⏳ **TODO**: Implement 8 API endpoints
3. ⏳ **TODO**: Add to routes in `cmd/api/routes.go`
4. ⏳ **TODO**: Add JWT authentication middleware

### Priority 4: Configuration (HIGH)
1. ⏳ **TODO**: Add `CastlerConfig` to `config.Config`
2. ⏳ **TODO**: Update `.env` with Castler credentials
3. ⏳ **TODO**: Update worker `main.go` to use config

### Priority 5: Worker Completion (MEDIUM)
1. ⏳ **TODO**: Uncomment worker initialization in `main.go`
2. ⏳ **TODO**: Implement scheduler with graceful shutdown
3. ⏳ **TODO**: Add production logging (replace fmt.Println)
4. ⏳ **TODO**: Add monitoring/alerting hooks

### Priority 6: Testing & Deployment (LOW)
1. ⏳ **TODO**: Write unit tests for service layer
2. ⏳ **TODO**: Write integration tests for worker
3. ⏳ **TODO**: Create systemd service for worker
4. ⏳ **TODO**: Add deployment documentation

---

## 📈 Progress Summary

**Completed**: 4/6 phases (67%)
- ✅ Phase 9.0: Documentation
- ✅ Phase 9.1: Database migration
- ✅ Phase 9.3: Service layer
- ✅ Phase 9.4: Worker structure
- ⏳ Phase 9.2: Webhook handler (0%)
- ⏳ Phase 9.5: Operations dashboard (0%)

**Code Statistics**:
- Lines written: ~1200
- Files created: 8
- Files modified: 15
- Database tables: 2
- Build status: ✅ All compiles

**Estimated Remaining Work**:
- Repository implementations: 4-6 hours
- Webhook handler: 3-4 hours
- Operations dashboard: 4-6 hours
- Testing & integration: 4-6 hours
- **Total**: ~15-22 hours

---

## 🔗 Related Documentation

- **Architecture**: `docs/payout_workflow.md` (complete workflow)
- **Payment Module**: `docs/payment_module.md` (payment settlement)
- **Payee Module**: `docs/payee_module.md` (beneficiary onboarding)
- **Migrations**: `docs/migrations.md` (database changes)

---

**Last Updated**: 2024-10-05
**Author**: Development Team
**Status**: In Progress - 67% Complete
