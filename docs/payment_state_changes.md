# Payment State Changes - Audit Trail System

## Overview

The `payment_state_changes` table provides a complete audit trail for the payment and payout lifecycle. This tracks **who initiated payments** and **when funds settled to payee accounts**, maintaining separation of concerns from the main voucher/payment tables.

---

## Database Schema

### Table: `payment_state_changes`

```sql
CREATE TABLE xcentro.payment_state_changes (
    state_change_id      UUID PRIMARY KEY,
    voucher_id           UUID NOT NULL,           -- FK to payment_vouchers
    payment_id           UUID,                    -- FK to payments (nullable)
    transfer_request_id  UUID,                    -- FK to payout_transfer_requests (nullable)
    action               payment_action_enum,     -- Type of state change
    action_timestamp     TIMESTAMPTZ,             -- When action occurred
    actor_type           payment_actor_enum,      -- user, system, webhook, api
    actor_user_id        UUID,                    -- User who triggered (if applicable)
    actor_name           VARCHAR(100),            -- System actor name (e.g., 'payout_worker')
    previous_state       VARCHAR(50),             -- Previous status
    new_state            VARCHAR(50),             -- New status
    amount               BIGINT,                  -- Amount in paise
    currency             VARCHAR(3),              -- INR
    utr                  VARCHAR(100),            -- Bank UTR
    provider             VARCHAR(50),             -- Gateway/provider name
    metadata             JSONB,                   -- Additional context
    remarks              TEXT,                    -- Human-readable description
    created_at           TIMESTAMPTZ              -- Record creation time
);
```

### Enums

**payment_action_enum:**
- `payment_initiated` - User clicked pay
- `payment_processing` - Gateway processing
- `payment_success` - Payment succeeded
- `payment_failed` - Payment failed
- `payment_refunded` - Payment refunded
- `settlement_pending` - Awaiting settlement
- `settlement_received` - Funds in escrow (T+2)
- `settlement_failed` - Settlement failed
- `payout_initiated` - Payout transfer started
- `payout_processing` - Provider processing payout
- `payout_completed` - **Funds settled to payee** ✅
- `payout_failed` - Payout failed
- `payout_timeout` - No webhook received
- `payout_cancelled` - Payout cancelled

**payment_actor_enum:**
- `user` - Human user (customer, admin, ops)
- `system` - System/worker (payout_worker)
- `webhook` - External webhook (gateway, payout provider)
- `api` - External API call

---

## Integration Points

### 1. Payment Handler (When User Initiates Payment)

**File:** `internal/handlers/http/payment/payment_handler.go`

```go
func (h *PaymentHandler) InitiatePayment(c echo.Context) error {
    // ... existing payment initiation code ...

    // Log state change: Payment initiated
    userID, _ := c.Get("userId").(string)
    userUUID, _ := uuid.Parse(userID)

    err = h.stateLogger.LogPaymentInitiated(
        ctx,
        voucherID,
        userUUID,
        req.Amount,
        "jiopay", // or "easebuzz" based on orchestrator decision
    )
    if err != nil {
        h.logger.WarnWith("Failed to log payment initiated", "error", err)
        // Non-blocking - continue with payment
    }

    // ... continue with gateway call ...
}
```

### 2. Payment Webhook Handler (Payment Success)

**File:** `internal/handlers/http/payment/webhook_handler.go`

```go
func (h *WebhookHandler) HandlePaymentSuccess(c echo.Context) error {
    // ... existing webhook processing ...

    // Log state change: Payment success
    metadata := map[string]any{
        "gateway_payment_id": webhookPayload.PaymentID,
        "gateway_order_id":   webhookPayload.OrderID,
        "payment_method":     webhookPayload.PaymentMethod,
    }

    err = h.stateLogger.LogPaymentSuccess(
        ctx,
        voucherID,
        paymentID,
        payment.Amount,
        "jiopay",
        webhookPayload.PaymentID,
        metadata,
    )
    if err != nil {
        h.logger.WarnWith("Failed to log payment success", "error", err)
    }

    // ... finalize voucher, update status ...
}
```

### 3. Settlement Webhook Handler (Settlement Received)

**File:** `internal/handlers/http/payment/settlement_webhook_handler.go`

```go
func (h *SettlementWebhookHandler) HandleSettlement(c echo.Context) error {
    // ... existing settlement processing ...

    // Log state change: Settlement received
    metadata := map[string]any{
        "settlement_batch_id": webhookPayload.BatchID,
        "settlement_date":     webhookPayload.SettlementDate,
        "net_amount":          payment.NetSettlementAmount,
    }

    err = h.stateLogger.LogSettlementReceived(
        ctx,
        voucherID,
        paymentID,
        payment.SettlementAmount,
        webhookPayload.UTR,
        metadata,
    )
    if err != nil {
        h.logger.WarnWith("Failed to log settlement received", "error", err)
    }

    // ... update payment settlement status ...
}
```

### 4. Payout Worker (Automatic Payout Initiation)

**File:** `internal/workers/payout_worker.go`

```go
func (w *PayoutWorker) initiateTransfer(ctx context.Context, transfer *TransferRequest) error {
    // ... existing payout initiation code ...

    // Log state change: Payout initiated (automatic)
    err = w.stateLogger.LogPayoutInitiated(
        ctx,
        transfer.VoucherID,
        *transfer.PaymentID,
        transfer.TransferRequestID,
        nil, // No user - system initiated
        "payout_worker",
        transfer.Amount,
        transfer.Provider,
        false, // isManual = false
    )
    if err != nil {
        w.logger.WarnWith("Failed to log payout initiated", "error", err)
    }

    // ... call Castler API ...
}
```

### 5. Payout Handler (Manual Payout Initiation)

**File:** `internal/handlers/http/payout/payout_handler.go`

```go
func (h *PayoutHandler) InitiatePayoutManually(c echo.Context) error {
    // ... existing manual payout code ...

    // Log state change: Payout initiated (manual)
    userID, _ := c.Get("userId").(string)
    userUUID, _ := uuid.Parse(userID)

    err = h.stateLogger.LogPayoutInitiated(
        ctx,
        voucherID,
        paymentID,
        transferID,
        &userUUID, // User who clicked initiate
        req.InitiatedBy,
        transfer.Amount,
        "castler",
        true, // isManual = true
    )
    if err != nil {
        h.logger.WarnWith("Failed to log payout initiated", "error", err)
    }

    // ... call payout service ...
}
```

### 6. Payout Webhook Handler (Payout Completed - FINAL STATE)

**File:** `internal/handlers/http/payout/webhook_handler.go`

```go
func (h *PayoutWebhookHandler) HandleCastlerWebhook(c echo.Context) error {
    // ... existing webhook processing ...

    switch webhookPayload.EventType {
    case "transfer.completed":
        // Log state change: Payout completed (funds settled to payee)
        metadata := map[string]any{
            "provider_transfer_id": webhookPayload.TransferID,
            "bank_reference":       webhookPayload.BankReference,
            "completion_time":      webhookPayload.CompletedAt,
        }

        err = h.stateLogger.LogPayoutCompleted(
            ctx,
            transfer.VoucherID,
            *transfer.PaymentID,
            transfer.TransferRequestID,
            transfer.Amount,
            "castler",
            webhookPayload.UTR,
            metadata,
        )
        if err != nil {
            h.logger.WarnWith("Failed to log payout completed", "error", err)
        }

        // ... update transfer status, update voucher payout_status ...

    case "transfer.failed":
        // Log state change: Payout failed
        metadata := map[string]any{
            "failure_code":    webhookPayload.FailureCode,
            "failure_message": webhookPayload.FailureMessage,
        }

        err = h.stateLogger.LogPayoutFailed(
            ctx,
            transfer.VoucherID,
            *transfer.PaymentID,
            transfer.TransferRequestID,
            "castler",
            webhookPayload.FailureMessage,
            metadata,
        )
        if err != nil {
            h.logger.WarnWith("Failed to log payout failed", "error", err)
        }
    }

    // ... continue webhook processing ...
}
```

---

## Querying State Changes

### Get Complete Payment Timeline for Voucher

```go
stateChanges, err := stateChangeRepo.GetByVoucherID(ctx, voucherID)

// Returns chronologically:
// 1. payment_initiated (by user X at 10:00)
// 2. payment_success (by jiopay_webhook at 10:02)
// 3. settlement_received (by settlement_webhook at 10:00 T+2)
// 4. payout_initiated (by payout_worker at 10:05 T+2)
// 5. payout_completed (by castler_webhook at 12:00 T+2) <- Funds in payee account
```

### Get Unified Voucher Activity (User-Facing)

**Endpoint:** `GET /v1/vouchers/:id/activity`

This endpoint returns a **unified activity feed** combining:
- Voucher creation
- Approval workflow (pending, approved, rejected, edited)
- **Payment lifecycle** (with user-friendly labels)

**User-Friendly Action Mapping:**

Internal payment actions are automatically mapped to simplified user-facing strings:

| Internal Action | User-Facing Label | Description |
|-----------------|-------------------|-------------|
| `payment_initiated`, `payment_processing`, `payment_success` | **Payment Initiated** | User clicked pay, payment succeeded |
| `payment_failed` | **Payment Failed** | Payment failed at gateway |
| `payment_refunded` | **Payment Refunded** | Payment was refunded |
| `settlement_pending`, `settlement_received`, `settlement_failed` | _(Hidden)_ | **Not shown in activity feed** - Internal escrow step |
| `payout_initiated`, `payout_processing` | **Payment Processing** | Payout transfer in progress |
| `payout_completed` | **Payment Completed to Payee** | ✅ Final state: Funds in payee account |
| `payout_failed` | **Payout Failed** | Payout transfer failed |
| `payout_timeout` | **Payout Under Investigation** | No webhook received, needs investigation |
| `payout_cancelled` | **Payout Cancelled** | Payout was cancelled |

**Why Simplified?**
- Users don't need to see internal state transitions (payment → settlement → payout)
- **Settlement actions are filtered out** to avoid confusion - users don't care when funds hit escrow
- They only care about: "Payment started", "Processing" (payout), "Completed to payee"
- Technical failures are shown for transparency

**Example Response:**
```json
{
  "voucher_id": "123e4567-e89b-12d3-a456-426614174000",
  "voucher_number": "VOUCH-2024-001",
  "activities": [
    {
      "activity_id": "...",
      "activity_type": "voucher_created",
      "user_id": "user-abc",
      "user_name": "John Doe",
      "action": "created",
      "timestamp": "2024-01-15T10:00:00Z"
    },
    {
      "activity_id": "...",
      "activity_type": "payment_activity",
      "user_id": "user-abc",
      "user_name": "John Doe",
      "action": "Payment Initiated",
      "timestamp": "2024-01-15T10:02:00Z"
    },
    {
      "activity_id": "...",
      "activity_type": "payment_activity",
      "user_id": "",
      "user_name": "payout_worker",
      "action": "Payment Processing",
      "timestamp": "2024-01-17T14:00:00Z",
      "metadata": "Provider: castler"
    },
    {
      "activity_id": "...",
      "activity_type": "payment_activity",
      "user_id": "",
      "user_name": "castler_webhook",
      "action": "Payment Completed to Payee",
      "timestamp": "2024-01-17T14:30:00Z",
      "metadata": "UTR: ICIC2401171234"
    }
  ],
  "total_count": 4
}
```

**Note:** Settlement actions (`settlement_received` at 2024-01-17T10:00:00Z) are **not shown** to avoid duplicate "Payment Processing" entries.

### Get Latest State for Voucher

```go
latestState, err := stateChangeRepo.GetLatestByVoucherID(ctx, voucherID)

// Returns: payout_completed with UTR and timestamp
```

### Get All Failed Payouts

```go
failedPayouts, err := stateChangeRepo.GetByAction(
    ctx,
    payment.PayoutActionFailed,
    100, // limit
    0,   // offset
)

// Returns all payout failures for investigation
```

---

## Service Initialization

### Update Service Constructors

**Payment Service:**
```go
type PaymentServiceImpl struct {
    paymentRepo     payment.PaymentRepository
    stateChangeRepo payment.PaymentStateChangeRepository
    stateLogger     *PaymentStateLogger
    // ... other dependencies
}

func NewPaymentService(
    paymentRepo payment.PaymentRepository,
    stateChangeRepo payment.PaymentStateChangeRepository,
    // ... other params
) *PaymentServiceImpl {
    return &PaymentServiceImpl{
        paymentRepo:     paymentRepo,
        stateChangeRepo: stateChangeRepo,
        stateLogger:     NewPaymentStateLogger(stateChangeRepo),
        // ... other fields
    }
}
```

**Payout Service:**
```go
type PayoutServiceImpl struct {
    transferRepo    payout.TransferRepository
    stateChangeRepo payment.PaymentStateChangeRepository
    stateLogger     *PaymentStateLogger
    // ... other dependencies
}
```

### Update Dependency Injection (routes.go)

```go
// Create state change repository
stateChangeRepo := paymentDB.NewPostgresPaymentStateChangeRepository(db)

// Pass to payment service
paymentService := paymentSvc.NewPaymentService(
    paymentRepo,
    stateChangeRepo,
    // ... other deps
)

// Pass to payout service
payoutService := payoutSvc.NewPayoutService(
    transferRepo,
    stateChangeRepo,
    // ... other deps
)

// Pass to handlers
paymentHandler := paymentHTTP.NewPaymentHandler(paymentService, stateChangeRepo, logger)
payoutHandler := payoutHTTP.NewPayoutHandler(payoutService, stateChangeRepo, logger)
```

---

## Benefits

1. **Complete Audit Trail**: Every payment and payout state transition is logged
2. **Accountability**: Know exactly who initiated payments and payouts
3. **Debugging**: Full timeline for troubleshooting payment issues
4. **Compliance**: Proof of payment settlement to payee accounts
5. **Separation of Concerns**: Audit trail separate from operational tables
6. **Non-Blocking**: State logging failures don't break payment flow
7. **Queryable**: Easy to generate reports on payment lifecycle

---

## Migration Guide

### Step 1: Run Migration
```bash
./bin/migrate -action=up
```

### Step 2: Update Service Constructors
Add `stateChangeRepo` parameter and initialize `stateLogger`

### Step 3: Add Logging Calls
Insert state logging calls at key points (see integration examples above)

### Step 4: Test Non-Blocking Behavior
Verify that state logging failures don't break payment flow

### Step 5: Create Dashboard Queries
Build ops dashboard views using state change queries

---

## Important Notes

- **Non-Blocking**: Always log state changes in a non-blocking manner (log errors but don't fail operations)
- **Idempotency**: Multiple logs of the same action are acceptable (duplicate webhook deliveries)
- **Timestamps**: Use `action_timestamp` for when the action occurred, `created_at` for when it was logged
- **Metadata**: Store rich context in metadata JSONB for debugging
- **Actor Names**: Use descriptive names for system actors: `payout_worker`, `razorpay_webhook`, etc.

---

## Example Timeline

### Complete Payment-to-Payout Flow (Internal Audit Trail)

```
2024-01-15 10:00:00 | payment_initiated      | user        | user_abc     | Amount: ₹50,000
2024-01-15 10:02:15 | payment_success        | webhook     | jiopay_whk   | Gateway Payment ID: xyz
2024-01-17 10:00:00 | settlement_received    | webhook     | settlement   | UTR: HDFC2401171234 (Hidden from users)
2024-01-17 10:05:00 | payout_initiated       | system      | payout_wkr   | Provider: castler
2024-01-17 12:30:45 | payout_completed       | webhook     | castler_whk  | UTR: ICIC2401171234 ✅
```

The final `payout_completed` event with UTR proves funds settled to payee's bank account.

### User-Facing Activity Feed (Simplified)

```
2024-01-15 10:00:00 | Voucher Created             | John Doe
2024-01-15 10:02:15 | Payment Initiated           | John Doe
2024-01-17 10:05:00 | Payment Processing          | payout_worker
2024-01-17 12:30:45 | Payment Completed to Payee  | castler_webhook (UTR: ICIC2401171234) ✅
```

**Note:** Settlement action at 10:00:00 is filtered out to avoid confusion with duplicate "Payment Processing" labels.

---

**Document Version**: 1.0
**Last Updated**: 2025-01-15
**Migration**: 000031
