# Payout Workflow Documentation

## Overview

This document explains the automated settlement-to-payout system that transfers funds from escrow account to payee bank accounts. The system orchestrates the complete flow from payment gateway settlement to final payout completion.

**Key Components:**
- Payment settlement tracking (Phase 7)
- Payee provider onboarding (Phase 8)
- Payout transfer orchestration (Phase 9)

---

## Architecture Overview

### End-to-End Flow

```
┌─────────────────┐
│  Customer Pays  │
│  (via Gateway)  │
└────────┬────────┘
         │
         ▼
┌─────────────────────────┐
│  Payment Accepted       │
│  status = 'success'     │
└────────┬────────────────┘
         │
         ▼ (T+2 delay)
┌─────────────────────────┐
│  Funds Settled          │
│  settlement_status =    │
│  'settled'              │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Voucher Ready          │
│  payment_status = 'paid'│
│  payout_status =        │
│  'pending'              │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Approval Check         │
│  Amount > ₹2,00,000?    │
└────────┬────────────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
  YES        NO
    │         │
    │         └──> Auto-approve
    │
    ▼
┌──────────────────┐
│ Manual Approval  │
│ (Ops Dashboard)  │
└────────┬─────────┘
         │
         ▼
┌─────────────────────────┐
│  Initiate Transfer      │
│  (Castler API)          │
└────────┬────────────────┘
         │
         ▼
┌─────────────────────────┐
│  Wait for Webhook       │
│  (60min timeout)        │
└────────┬────────────────┘
         │
    ┌────┴─────┬──────────┬─────────┐
    │          │          │         │
    ▼          ▼          ▼         ▼
processing  completed  failed   timeout
    │          │          │         │
    │          ▼          │         │
    │      ✅ SUCCESS     │         │
    │                     │         │
    └─────────────────────┴─────────┴──> Manual Intervention
                                          (Retry/Investigation)
```

---

## Phase 7: Payment Settlement Tracking

### Purpose
Track when gateway funds actually settle into escrow (T+2 delay from acceptance).

### Database Schema

**New Columns in `payments` table:**
```sql
settlement_status       settlement_status_enum DEFAULT 'pending'
settled_at              TIMESTAMPTZ
settlement_reference_id VARCHAR(255)
settlement_utr          VARCHAR(100)
settlement_metadata     JSONB
```

**settlement_status_enum values:**
- `pending` - Payment accepted, awaiting settlement
- `settled` - Funds in escrow, ready for payout
- `failed` - Settlement failed
- `refunded` - Settlement refunded

### Key Insight
**Payment success ≠ Settlement**
- `payment.status = 'success'` → Gateway accepted payment
- `payment.settlement_status = 'settled'` → Funds in our escrow (T+2 later)

Only after `settlement_status = 'settled'` can we initiate payout!

---

## Phase 8: Payee Provider Onboarding

### Purpose
Register payees as beneficiaries with payout provider (Castler/Razorpay) during payee creation.

### Database Schema

**Table: `payee_provider_mappings`**
```sql
mapping_id              UUID PRIMARY KEY
payee_id                UUID → payee_master
account_id              UUID → payee_bank_accounts
provider                payout_provider_enum (castler, razorpay, bank_api)
provider_beneficiary_id VARCHAR(255)  -- Castler's beneficiary ID
provider_account_id     VARCHAR(255)  -- Castler's account ID
onboarding_status       payee_onboarding_status_enum
onboarded_at            TIMESTAMPTZ
provider_metadata       JSONB
```

### Onboarding Flow

1. **Payee Creation** → Vendor/Employee onboarding API
2. **Bank Verification** → Penny drop via existing service
3. **GST/PAN Verification** → Cross-verification
4. **Provider Onboarding** → Call `OnboardBeneficiary()` API
   - Castler: Create beneficiary (two-step: create + approve)
   - Razorpay: Direct beneficiary registration
5. **Store Mapping** → Save provider_beneficiary_id for future payouts

### Key Design
**Onboard during creation, not at payout time**
- ✅ Immediate validation (know if payee can receive payments)
- ✅ Faster payouts (no onboarding delay)
- ✅ Better UX (payee status shows "ready to receive")
- ✅ Support provider migration via ETL

---

## Phase 9: Payout Transfer Orchestration

### Purpose
Automated worker that initiates payouts after settlement, with approval workflow and webhook-based status tracking.

### Database Schema

**Table: `payout_transfer_requests`**
```sql
transfer_request_id     UUID PRIMARY KEY
voucher_id              UUID UNIQUE → payment_vouchers
provider                VARCHAR(50)
provider_beneficiary_id VARCHAR(255)
provider_transfer_id    VARCHAR(255)  -- Castler's transfer ID

-- Transfer details
amount                  BIGINT
currency                VARCHAR(3) DEFAULT 'INR'
purpose                 VARCHAR(255)
reference               VARCHAR(255)  -- voucher_number
narration               TEXT
transfer_mode           VARCHAR(50)   -- NEFT, RTGS, IMPS

-- Approval workflow
requires_approval       BOOLEAN DEFAULT false
approval_mode           VARCHAR(20)   -- 'auto', 'manual'
approved_by             UUID → user_table
approved_at             TIMESTAMPTZ

-- Status tracking (webhook-driven)
status                  payout_transfer_status_enum
initiated_at            TIMESTAMPTZ
processing_at           TIMESTAMPTZ
completed_at            TIMESTAMPTZ
failed_at               TIMESTAMPTZ
failure_reason          TEXT
utr                     VARCHAR(100)

-- Webhook tracking
last_webhook_at         TIMESTAMPTZ
webhook_timeout_at      TIMESTAMPTZ  -- initiated_at + 60min
webhook_history         JSONB[]      -- Array of all webhook payloads

-- Retry tracking (placeholder for future)
retry_count             INT DEFAULT 0
max_retries             INT DEFAULT 3
next_retry_at           TIMESTAMPTZ

created_at              TIMESTAMPTZ DEFAULT NOW()
updated_at              TIMESTAMPTZ DEFAULT NOW()
```

**payout_transfer_status_enum values:**
- `pending_approval` - Awaiting manual approval (amount > ₹2L)
- `approved` - Approved, ready to initiate
- `initiated` - Sent to Castler, awaiting webhook
- `processing` - Castler confirmed processing (from webhook)
- `completed` - Successfully completed (from webhook)
- `failed` - Failed (from webhook) - needs manual retry
- `cancelled` - Cancelled by ops before initiation
- `timeout` - No webhook received after 60min (needs investigation)

**Table: `payout_configuration`**
```sql
config_id                   UUID PRIMARY KEY
business_id                 UUID → business (NULL = global)

-- Auto-approval settings
auto_approval_enabled       BOOLEAN DEFAULT true
auto_approval_limit         BIGINT DEFAULT 20000000  -- ₹2,00,000 in paise

-- Transfer preferences
preferred_transfer_mode     VARCHAR(50) DEFAULT 'NEFT'

-- Webhook settings
webhook_timeout_minutes     INT DEFAULT 60

-- Retry settings (placeholder)
max_retry_attempts          INT DEFAULT 3
retry_delay_minutes         INT DEFAULT 15

-- Worker settings
batch_size                  INT DEFAULT 100
worker_enabled              BOOLEAN DEFAULT true

created_at                  TIMESTAMPTZ DEFAULT NOW()
updated_at                  TIMESTAMPTZ DEFAULT NOW()
```

---

## Castler Integration Details

### API Workflow

**1. Beneficiary Onboarding (Phase 8)**
```http
POST /v1/beneficiaries
{
  "account_number": "1234567890",
  "ifsc_code": "HDFC0000123",
  "account_holder_name": "ABC Corp",
  "bank_name": "HDFC Bank",
  "account_type": "current",
  "gstin": "29ABCDE1234F1Z5",
  "email": "payments@abc.com",
  "phone": "+919876543210"
}

Response:
{
  "beneficiary_id": "ben_xxxx",
  "account_id": "acc_yyyy",
  "status": "active",
  "verification_status": "verified"
}
```

**2. Transfer Creation (Two-Step)**
```http
POST /v1/transfers
{
  "beneficiary_id": "ben_xxxx",
  "amount": 50000,  // paise
  "currency": "INR",
  "purpose": "vendor_payment",
  "reference": "VOUCH-2024-001",
  "narration": "Payment for invoice INV-123",
  "transfer_mode": "NEFT"
}

Response:
{
  "transfer_id": "txn_zzzz",
  "status": "pending_approval",
  "requires_approval": true
}
```

**3. Transfer Approval**
```http
POST /v1/transfers/{transfer_id}/approve
{
  "approved_by": "admin@business.com"
}

Response:
{
  "transfer_id": "txn_zzzz",
  "status": "processing"
}
```

### Webhook Events

**Castler sends these webhooks:**

**Event 1: Transfer Processing**
```json
{
  "event_type": "transfer.processing",
  "transfer_id": "txn_zzzz",
  "status": "processing",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Event 2: Transfer Completed**
```json
{
  "event_type": "transfer.completed",
  "transfer_id": "txn_zzzz",
  "status": "success",
  "utr": "HDFC24011512345678",
  "timestamp": "2024-01-15T10:45:00Z"
}
```

**Event 3: Transfer Failed**
```json
{
  "event_type": "transfer.failed",
  "transfer_id": "txn_zzzz",
  "status": "failed",
  "failure_code": "INSUFFICIENT_BALANCE",
  "failure_message": "Insufficient balance in virtual account",
  "timestamp": "2024-01-15T10:32:00Z"
}
```

### Webhook Security

**Signature Verification:**
```go
// Castler sends HMAC-SHA256 signature in header
signature := request.Header.Get("X-Castler-Signature")
expectedSignature := hmac.SHA256(webhookSecret, requestBody)

if signature != expectedSignature {
    return 401 Unauthorized
}
```

### Key Constraints

- **Fire once**: Castler initiates transfer once, sends webhook updates
- **No status polling**: No GET /transfers/{id}/status API
- **Webhook-driven**: All status updates come via webhooks
- **NEFT/RTGS**: Typical settlement 30min-4 hours (bank dependent)
- **Two-step approval**: Always required (admin-level transfers)

---

## Approval Workflow

### Auto-Approval Logic

```go
const AutoApprovalLimit = 20000000 // ₹2,00,000 in paise

if transfer.Amount <= config.AutoApprovalLimit {
    // Auto-approve
    transfer.RequiresApproval = false
    transfer.ApprovalMode = "auto"
    transfer.ApprovedBy = "SYSTEM"
    transfer.ApprovedAt = time.Now()
    transfer.Status = "approved"

    // Worker will initiate immediately
} else {
    // Needs manual approval
    transfer.RequiresApproval = true
    transfer.ApprovalMode = "manual"
    transfer.Status = "pending_approval"

    // Wait for ops team via dashboard
}
```

### Manual Approval Flow

1. **Ops Dashboard** → View pending approvals
2. **Review Details** → Voucher, payee, amount, purpose
3. **Approve** → POST `/operations/payouts/:id/approve`
4. **Status Change** → `pending_approval` → `approved`
5. **Worker Picks Up** → Initiates transfer with Castler

---

## Worker Implementation

### Worker Tasks (Runs Every 5 Minutes)

**Task 1: Process Ready Vouchers**
```sql
SELECT v.*
FROM payment_vouchers v
JOIN payments p ON v.payment_id = p.payment_id
WHERE p.settlement_status = 'settled'
  AND v.payment_status = 'paid'
  AND v.payout_status = 'pending'
  AND v.cancellation_status = 'not_cancelled'
  AND NOT EXISTS (
      SELECT 1 FROM payout_transfer_requests ptr
      WHERE ptr.voucher_id = v.voucher_id
  )
LIMIT 100;
```

For each voucher:
1. Get active provider mapping (payee + account → Castler)
2. Get payout configuration
3. Determine if needs approval (amount > ₹2L)
4. Create transfer_request record

**Task 2: Initiate Approved Transfers**
```sql
SELECT *
FROM payout_transfer_requests
WHERE status = 'approved'
LIMIT 100;
```

For each transfer:
1. Get provider (Castler/Razorpay)
2. Call `CreateTransferRequest()` API
3. If two-step: Call `ApproveTransferRequest()` API
4. Update status to `initiated`
5. Set `webhook_timeout_at = NOW() + 60min`
6. Update voucher `payout_status = 'initiated'`

**Task 3: Flag Timeout Transfers**
```sql
SELECT *
FROM payout_transfer_requests
WHERE status = 'initiated'
  AND webhook_timeout_at < NOW()
LIMIT 100;
```

For each timeout:
1. Update status to `timeout`
2. Update voucher `payout_status = 'needs_attention'`
3. Send alert to ops team

**Task 4: Retry Failed Transfers (PLACEHOLDER)**
```go
func (w *PayoutWorker) retryPendingTransfers(ctx context.Context) {
    fmt.Println("TODO: Implement retry strategy with exponential backoff")
    fmt.Println("TODO: Consider unified retry strategy across payments/payouts")
    // Skip for MVP - ops team handles failed transfers manually
}
```

---

## Webhook Handler Flow

### Endpoint: `POST /api/v1/webhooks/payout/castler`

```go
func (h *WebhookHandler) HandleCastlerWebhook(c echo.Context) error {
    // 1. Verify signature
    signature := c.Request().Header.Get("X-Castler-Signature")
    if !verifyCastlerSignature(signature, body, secretKey) {
        return c.JSON(401, "invalid signature")
    }

    // 2. Parse payload
    var payload CastlerWebhookPayload
    c.Bind(&payload)

    // 3. Find transfer by provider_transfer_id
    transfer := transferRepo.GetByProviderTransferID(ctx, payload.TransferID)

    // 4. Handle event
    switch payload.EventType {
    case "transfer.processing":
        transfer.Status = "processing"
        transfer.ProcessingAt = payload.Timestamp

    case "transfer.completed":
        transfer.Status = "completed"
        transfer.CompletedAt = payload.Timestamp
        transfer.UTR = payload.UTR
        voucher.PayoutStatus = "completed"

    case "transfer.failed":
        transfer.Status = "failed"
        transfer.FailedAt = payload.Timestamp
        transfer.FailureReason = payload.FailureCode + ": " + payload.FailureMsg

        // TODO: Implement retry logic
        fmt.Println("TODO: Retry failed transfer:", payload.TransferID)
        // For MVP: Mark failed, ops handles manually
    }

    // 5. Store webhook in history
    transfer.WebhookHistory = append(transfer.WebhookHistory, payload)
    transfer.LastWebhookAt = time.Now()

    // 6. Save updates
    transferRepo.Update(ctx, transfer)

    return c.JSON(200, "ok")
}
```

---

## Operations Dashboard

### Endpoints

**Pending Approvals**
```http
GET /api/v1/operations/payouts/pending-approvals?page=1&limit=50
```

**Approve Transfer**
```http
POST /api/v1/operations/payouts/{transfer_id}/approve
{
  "approved_by": "user_uuid",
  "remarks": "Approved for Q1 vendor payment"
}
```

**Cancel Transfer**
```http
POST /api/v1/operations/payouts/{transfer_id}/cancel
{
  "cancelled_by": "user_uuid",
  "reason": "Duplicate request"
}
```

**Transfer Details**
```http
GET /api/v1/operations/payouts/{transfer_id}

Response:
{
  "transfer_id": "...",
  "voucher": { ... },
  "payee": { ... },
  "amount": 50000,
  "status": "processing",
  "timeline": [
    { "event": "created", "at": "..." },
    { "event": "approved", "at": "...", "by": "..." },
    { "event": "initiated", "at": "..." },
    { "event": "processing", "at": "..." }
  ],
  "webhooks": [ ... ]
}
```

**Webhook History**
```http
GET /api/v1/operations/payouts/{transfer_id}/webhooks

Response:
{
  "webhooks": [
    {
      "event_type": "transfer.processing",
      "timestamp": "2024-01-15T10:30:00Z",
      "payload": { ... }
    },
    {
      "event_type": "transfer.completed",
      "timestamp": "2024-01-15T10:45:00Z",
      "payload": { ... }
    }
  ]
}
```

**Timeout Transfers**
```http
GET /api/v1/operations/payouts/timeout

Response:
{
  "transfers": [
    {
      "transfer_id": "...",
      "voucher_number": "VOUCH-2024-001",
      "amount": 50000,
      "initiated_at": "2024-01-15T10:00:00Z",
      "timeout_at": "2024-01-15T11:00:00Z",
      "status": "timeout"
    }
  ]
}
```

**Configuration Management**
```http
GET /api/v1/operations/payouts/config
PUT /api/v1/operations/payouts/config
{
  "auto_approval_limit": 20000000,
  "preferred_transfer_mode": "NEFT",
  "webhook_timeout_minutes": 60
}
```

---

## Error Handling & Edge Cases

### Failure Scenarios

**1. Provider API Failure (During Initiation)**
- Log error with full context
- Keep status as `approved` (not `initiated`)
- Worker will retry on next run (idempotent)

**2. Webhook Not Received (Timeout)**
- Worker flags as `timeout` after 60min
- Ops team investigates:
  - Check Castler dashboard for status
  - Verify with bank using provider_transfer_id
  - Manually mark completed if verified

**3. Webhook Failed Event**
- Mark transfer as `failed`
- Store failure_reason from webhook
- **MVP**: Ops team handles manually
- **Future**: Implement retry with exponential backoff

**4. Duplicate Webhook**
- Check `last_webhook_at` and event type
- Idempotent: Same event → update timestamp, append to history
- Different event → process as state transition

**5. Missing Provider Mapping**
- Skip voucher (log warning)
- Alert: "Payee {id} not onboarded with any provider"
- Ops must onboard payee first

### Monitoring & Alerts

**Alert Conditions:**
1. Transfer stuck in `initiated` > 60min (timeout)
2. Transfer `failed` with specific error codes
3. Webhook signature verification failures
4. Provider API errors (>5 consecutive failures)
5. Voucher stuck in `pending` payout > 24 hours

---

## Retry Strategy (Future Enhancement)

### Current State (MVP)
```go
// Placeholder in webhook handler
case "transfer.failed":
    transfer.Status = "failed"
    fmt.Println("TODO: Implement retry strategy for transfer:", transfer.TransferRequestID)
    // Ops team handles manually via dashboard
```

### Future Design Considerations

**Unified Retry Strategy Needed For:**
- Payment gateway failures
- Payout provider failures
- Bank verification timeouts
- GST verification failures
- Webhook delivery failures

**Proposed Approach:**
```go
type RetryPolicy struct {
    MaxAttempts       int
    InitialDelay      time.Duration
    BackoffMultiplier float64
    MaxDelay          time.Duration
    RetryableErrors   []string
}

// Exponential backoff: 15min, 30min, 60min, 120min
func CalculateNextRetry(attempt int, policy RetryPolicy) time.Time {
    delay := policy.InitialDelay * time.Duration(
        math.Pow(policy.BackoffMultiplier, float64(attempt))
    )

    if delay > policy.MaxDelay {
        delay = policy.MaxDelay
    }

    return time.Now().Add(delay)
}
```

**Implementation Tasks (Post-MVP):**
- [ ] Design unified retry configuration
- [ ] Implement exponential backoff
- [ ] Add circuit breaker pattern
- [ ] Create dead letter queue for max failures
- [ ] Implement retry worker (separate from payout worker)

---

## Testing Strategy

### Unit Tests

**Approval Logic:**
```go
func TestAutoApprovalLogic(t *testing.T) {
    // Amount ≤ ₹2L → auto-approve
    // Amount > ₹2L → pending_approval
}
```

**Webhook Processing:**
```go
func TestWebhookHandler(t *testing.T) {
    // Valid signature → process
    // Invalid signature → reject
    // Duplicate webhook → idempotent
}
```

**Timeout Detection:**
```go
func TestTimeoutFlagging(t *testing.T) {
    // initiated_at + 60min < NOW() → timeout
}
```

### Integration Tests

**Worker End-to-End:**
```go
func TestPayoutWorkerFlow(t *testing.T) {
    // 1. Create settled voucher
    // 2. Worker creates transfer_request
    // 3. Worker initiates transfer
    // 4. Simulate webhook
    // 5. Verify voucher.payout_status = 'completed'
}
```

**Provider API Mocking:**
```go
// Mock Castler responses
mockCastler := &MockCastlerClient{
    OnboardResponse: {...},
    TransferResponse: {...},
}
```

---

## Troubleshooting Guide

### Common Issues

**1. Voucher Stuck in 'pending' Payout**

**Debug Query:**
```sql
SELECT
    v.voucher_id,
    v.payout_status,
    p.settlement_status,
    v.payment_status,
    v.cancellation_status,
    ptr.status as transfer_status
FROM payment_vouchers v
JOIN payments p ON v.payment_id = p.payment_id
LEFT JOIN payout_transfer_requests ptr ON v.voucher_id = ptr.voucher_id
WHERE v.payout_status = 'pending';
```

**Possible Causes:**
- Payment not settled: `settlement_status != 'settled'`
- Voucher cancelled: `cancellation_status != 'not_cancelled'`
- Worker disabled: Check `payout_configuration.worker_enabled`
- No provider mapping: Check `payee_provider_mappings`

**Fix:**
```sql
-- If settled and ready, manually create transfer request
INSERT INTO payout_transfer_requests (...) VALUES (...);
```

**2. Transfer Not Auto-Completing**

**Debug Query:**
```sql
SELECT *
FROM payout_transfer_requests
WHERE status = 'initiated'
  AND initiated_at < NOW() - INTERVAL '2 hours';
```

**Possible Causes:**
- Webhook not received (check Castler dashboard)
- Webhook signature verification failed (check logs)
- Transfer actually failed (check Castler)

**Fix:**
- View webhook history: `GET /operations/payouts/{id}/webhooks`
- Check Castler dashboard for actual status
- Manually complete if verified: `POST /operations/payouts/{id}/mark-completed`

**3. Approval Not Triggering Initiation**

**Debug Query:**
```sql
SELECT *
FROM payout_transfer_requests
WHERE status = 'approved'
  AND approved_at < NOW() - INTERVAL '10 minutes';
```

**Possible Causes:**
- Worker not running (check service status)
- Provider API down (check error logs)
- Provider mapping missing

**Fix:**
- Restart worker service
- Check provider API health
- Manually initiate: Force worker run

---

## Development Notes

### Design Decisions

**1. Why Webhook-Driven?**
- Castler doesn't provide status polling API
- Webhooks are real-time and efficient
- Reduces API calls and polling overhead

**2. Why 60min Timeout?**
- NEFT/RTGS typically completes in 30min-2 hours
- 60min is reasonable for flagging issues
- Allows time for bank processing

**3. Why ₹2L Auto-Approval Limit?**
- Business requirement for risk management
- Balances automation with control
- Configurable per business

**4. Why Placeholder Retry?**
- Retry strategy needs holistic design
- Affects payments, payouts, verifications
- MVP: Manual intervention is acceptable
- Future: Unified retry with exponential backoff

**5. Why Idempotent Operations?**
- Worker runs every 5min
- Prevents duplicate transfers
- `UNIQUE(voucher_id)` constraint ensures one transfer per voucher

### Future Enhancements

**Phase 10+ (Post-MVP):**
- [ ] Implement unified retry strategy
- [ ] Add Razorpay integration (has status polling)
- [ ] Webhook replay mechanism
- [ ] Real-time dashboard updates (websockets)
- [ ] Payout scheduling (pay on specific date)
- [ ] Bulk payout API (CSV upload)
- [ ] Reconciliation dashboard
- [ ] Provider failover (Castler → Razorpay)

---

## Quick Reference

### Key Constants

```go
AutoApprovalLimit         = 20000000    // ₹2,00,000 in paise
WebhookTimeoutMinutes     = 60          // 1 hour
WorkerIntervalMinutes     = 5           // Run every 5 minutes
BatchSize                 = 100         // Process 100 at a time
MaxRetries                = 3           // (Future) Max retry attempts
```

### Status Transitions

```
Payout Transfer Status Flow:
pending_approval → approved → initiated → processing → completed ✅
                                      ↓
                                    failed → (manual retry)
                                      ↓
                                   timeout → (investigation)
```

### Critical Queries

```sql
-- Vouchers ready for payout
SELECT v.* FROM payment_vouchers v
JOIN payments p ON v.payment_id = p.payment_id
WHERE p.settlement_status = 'settled'
  AND v.payment_status = 'paid'
  AND v.payout_status = 'pending'
  AND v.cancellation_status = 'not_cancelled';

-- Pending approvals
SELECT * FROM payout_transfer_requests
WHERE status = 'pending_approval'
ORDER BY amount DESC;

-- Timeout transfers
SELECT * FROM payout_transfer_requests
WHERE status = 'initiated'
  AND webhook_timeout_at < NOW();
```

---

## Support & Escalation

### When to Escalate

1. **Timeout > 2 hours**: Immediate escalation
2. **Failed with unknown error**: Check Castler docs, escalate if needed
3. **Webhook signature failures**: Security concern, escalate immediately
4. **Provider API down > 30min**: Escalate to Castler support

### Contacts

- **Castler Support**: support@castler.in
- **Internal Ops Team**: ops@xcentro.in
- **On-Call Engineer**: Defined in PagerDuty

---

**Document Version**: 1.0
**Last Updated**: 2024-01-15
**Author**: Development Team
**Review Cycle**: Quarterly
