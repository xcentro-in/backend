# Payout Worker Architecture

## Overview

The payout system uses a **two-layer architecture** that separates business logic from orchestration/scheduling. This document explains why we have both `PayoutService` and `PayoutWorker`, and how they work together.

---

## Architecture Layers

### Layer 1: PayoutService (Business Logic)
**"HOW to do something"**

**Location:** `internal/services/payout/payout_service.go`

**Purpose:** Encapsulates all business rules and validation logic for payout operations.

**Responsibilities:**
- ✅ Validate voucher is ready for payout
- ✅ Check auto-approval threshold (₹2,00,000)
- ✅ Get provider mapping (which payout provider to use)
- ✅ Create transfer request in database
- ✅ Apply business rules and validations

**Use Cases:**
1. **Manual initiation** - Ops team clicks button in dashboard
2. **API-triggered** - External system calls API endpoint
3. **Single voucher processing** - One voucher at a time

**Example Usage:**
```go
// API Handler
func (h *PayoutHandler) InitiatePayout(c echo.Context) error {
    var req InitiatePayoutRequest
    c.Bind(&req)

    // Call service directly
    transfer, err := h.payoutService.InitiatePayoutForVoucher(ctx, req.VoucherID)
    if err != nil {
        return c.JSON(500, err)
    }

    return c.JSON(200, transfer)
}
```

---

### Layer 2: PayoutWorker (Orchestration/Scheduling)
**"WHEN to do something + batch processing"**

**Location:** `internal/workers/payout_worker.go`

**Purpose:** Automates payout processing by running periodic batch jobs without manual intervention.

**Responsibilities:**
- ✅ **Scheduling** - Run tasks every N minutes (configurable)
- ✅ **Batch processing** - Handle 100 items at a time (config-driven)
- ✅ **Error isolation** - If one transfer fails, continue with others
- ✅ **Orchestration** - Coordinate multiple tasks in sequence

**Use Cases:**
1. **Automated processing** - No human intervention needed
2. **Bulk operations** - Process many vouchers at once
3. **Background jobs** - Runs independently from API server

**Worker Tasks (runs every 1 minute):**
1. **Process Ready Vouchers** - Find settled vouchers, create transfer requests
2. **Initiate Approved Transfers** - Send approved transfers to Castler API
3. **Flag Timeout Transfers** - Mark transfers without webhook after 60min
4. **Retry Failed Transfers** - Placeholder for future retry strategy

---

## Why Two Layers?

### The Problem Without Separation

**What if we put everything in Worker?**

```go
// BAD: All logic in worker
func (w *PayoutWorker) processReadyVouchers() {
    vouchers := w.voucherRepo.GetReady()
    for _, v := range vouchers {
        // DUPLICATE all validation logic here
        if v.PaymentID == nil { return }
        if v.Status != "pending" { return }
        config := w.configRepo.GetConfig()
        if v.Amount > config.AutoApprovalLimit {
            // Needs approval...
        }
        // Create transfer...
        // 50 lines of business logic...
    }
}
```

**Problems:**
- ❌ Can't manually initiate payout via API (logic is locked in worker)
- ❌ Duplicate code if we add API endpoint later
- ❌ Can't test business logic without running entire worker
- ❌ Tight coupling - changing worker affects business rules

---

### The Solution: Separation of Concerns

```
┌─────────────────────────────────────────┐
│         API Handler (Manual)            │
│  POST /api/v1/payouts/initiate          │
└──────────────┬──────────────────────────┘
               │
               ↓
┌──────────────────────────────────────────┐
│       PayoutService (Business Logic)     │
│  - Validate voucher                      │
│  - Check auto-approval                   │
│  - Create transfer request               │
└──────────────┬───────────────────────────┘
               ↑
               │ (reuses same logic)
               │
┌──────────────┴───────────────────────────┐
│     PayoutWorker (Automation)            │
│  - Timer: every 1 minute                 │
│  - Find 100 ready vouchers               │
│  - Call PayoutService.InitiatePayoutForVoucher()  │
│  - Initiate approved transfers           │
│  - Flag timeouts                         │
└──────────────────────────────────────────┘
```

**Benefits:**
- ✅ Service layer = reusable business logic
- ✅ Worker layer = automation + orchestration
- ✅ Can use service without worker (API-only mode)
- ✅ Can use worker without changing service
- ✅ Clean separation of concerns
- ✅ Easy to test each layer independently

---

## How They Work Together

### Example 1: Manual Initiation (API)

```
User clicks button
  → API Handler
  → PayoutService.InitiatePayoutForVoucher(voucherID)
  → Validates voucher
  → Creates transfer request
  → Returns transfer object
```

**No worker involved** - Direct service call.

---

### Example 2: Automated Initiation (Worker)

```
Timer (every 1 minute)
  → PayoutWorker.Run()
  → PayoutWorker.processReadyVouchers()
  → Finds 100 vouchers with status='pending'
  → For each voucher:
      → PayoutService.InitiatePayoutForVoucher(voucherID)
      → (Same business logic as manual!)
  → PayoutWorker.initiateApprovedTransfers()
  → Finds 100 transfers with status='approved'
  → For each transfer:
      → Calls Castler API
      → Updates status to 'initiated'
```

**Worker orchestrates** - But reuses service logic for validation!

---

## Worker Implementation Details

### Scheduler: Simple Ticker

**Location:** `cmd/payout_worker/main.go`

**Strategy:** Time-based ticker (runs every 1 minute)

```go
// Setup
ticker := time.NewTicker(1 * time.Minute)
defer ticker.Stop()

// Main loop
for {
    select {
    case <-ticker.C:
        // Execute all worker tasks
        if err := worker.Run(ctx); err != nil {
            log.Printf("Worker run failed: %v", err)
        }

    case <-sigChan:
        // Graceful shutdown on Ctrl+C
        cancel()
        return
    }
}
```

**Why ticker instead of cron?**
- ✅ No external dependencies
- ✅ Simple and predictable
- ✅ Easy to adjust interval
- ✅ Graceful shutdown support
- ✅ Suitable for MVP

---

### Worker Tasks Breakdown

#### Task 1: Process Ready Vouchers
```go
func (w *PayoutWorker) processReadyVouchers(ctx context.Context) error {
    config := w.configRepo.GetConfig(ctx, nil)

    // Find vouchers ready for payout (batch_size from config)
    // WHERE payment.settlement_status = 'settled'
    //   AND voucher.payment_id IS NOT NULL
    //   AND voucher.payout_status = 'pending'
    //   AND NOT EXISTS (SELECT 1 FROM payout_transfer_requests...)
    // LIMIT config.BatchSize

    // For MVP: Placeholder - manual initiation via API
}
```

**Status:** ⏸️ Placeholder (manual for MVP)

---

#### Task 2: Initiate Approved Transfers
```go
func (w *PayoutWorker) initiateApprovedTransfers(ctx context.Context) error {
    // Get transfers with status='approved'
    transfers := w.transferRepo.GetByStatus(ctx, "approved", batchSize)

    for _, transfer := range transfers {
        // Get provider (Castler)
        provider := w.providerFactory.GetProvider(transfer.Provider)

        // Call Castler API
        resp := provider.CreateTransferRequest(ctx, transfer)

        // If two-step approval (Castler), approve immediately
        if provider.SupportsTwoStepApproval() {
            provider.ApproveTransferRequest(ctx, resp.TransferID)
        }

        // Update status to 'initiated'
        w.transferRepo.UpdateStatus(ctx, transfer.ID, "initiated", time.Now())

        // Set webhook timeout (initiated_at + 60 minutes)
        w.transferRepo.SetWebhookTimeout(ctx, transfer.ID, timeoutAt)
    }
}
```

**Status:** ✅ Fully implemented

---

#### Task 3: Flag Timeout Transfers
```go
func (w *PayoutWorker) flagTimeoutTransfers(ctx context.Context) error {
    // Get transfers where webhook_timeout_at < NOW()
    transfers := w.transferRepo.GetWebhookTimeouts(ctx, batchSize)

    for _, transfer := range transfers {
        // Update status to 'timeout'
        w.transferRepo.UpdateStatus(ctx, transfer.ID, "timeout", time.Now())

        // Update voucher to 'needs_attention'
        w.voucherRepo.UpdatePayoutStatus(ctx, transfer.VoucherID,
            "needs_attention", "Webhook timeout")

        // TODO: Send alert to ops team
    }
}
```

**Status:** ✅ Fully implemented

---

#### Task 4: Retry Failed Transfers
```go
func (w *PayoutWorker) retryFailedTransfers(ctx context.Context) error {
    // TODO: Implement retry strategy with exponential backoff
    // For MVP: Ops team handles failed transfers manually
}
```

**Status:** ⏸️ Placeholder (manual for MVP)

---

## Configuration

### Database-Driven Config

**Table:** `payout_configuration`

**Configurable Parameters:**
```go
type PayoutConfiguration struct {
    // Auto-approval settings
    AutoApprovalEnabled bool
    AutoApprovalLimit   int64  // ₹2,00,000 in paise

    // Transfer preferences
    PreferredTransferMode string  // NEFT, RTGS, IMPS

    // Retry settings (future)
    MaxRetryAttempts      int
    RetryDelayMinutes     int
    RetryBackoffMultiplier float64

    // Webhook timeout
    WebhookTimeoutMinutes int  // 60 minutes default

    // Worker settings
    BatchSize     int   // 100 default
    WorkerEnabled bool  // true default
}
```

**Per-business override:** Set `business_id` to specific UUID for business-specific config

**Global default:** `business_id = NULL` applies to all businesses

---

## Deployment

### Running the Worker

```bash
# Build
go build -o bin/payout_worker ./cmd/payout_worker

# Run
./bin/payout_worker
```

**Expected Output:**
```
🚀 Starting Payout Worker...
📝 Loaded configuration
✅ Database connection established
✅ Repositories initialized
✅ Payout provider factory initialized (using config: Castler)
✅ Payout service initialized
✅ Payout worker initialized

📦 Worker structure:
  ✅ Worker logic: internal/workers/payout_worker.go
  ✅ Entry point: cmd/payout_worker/main.go
  ✅ Service layer: internal/services/payout/payout_service.go
  ✅ Repositories: transfer, configuration, mapping, voucher
  ✅ Provider factory: Castler client initialized

💡 Worker tasks:
  1. Process ready vouchers (create transfer requests)
  2. Initiate approved transfers (call Castler API)
  3. Flag timeout transfers (no webhook after 60min)
  4. Handle failed transfers (placeholder for retry strategy)

🔄 Worker scheduler started (runs every 1 minute)
⏸️  Press Ctrl+C to stop gracefully

🏃 Running initial worker cycle...
=== PAYOUT WORKER: Starting run ===
...
```

---

### Graceful Shutdown

**Ctrl+C handling:**
```
🛑 Received signal: interrupt
🔄 Shutting down worker gracefully...
✅ Worker stopped
```

---

## Testing Strategy

### Unit Testing

**Test PayoutService independently:**
```go
func TestInitiatePayoutForVoucher(t *testing.T) {
    mockVoucherRepo := &MockVoucherRepository{}
    mockTransferRepo := &MockTransferRepository{}

    service := NewPayoutService(
        mockVoucherRepo,
        mockTransferRepo,
        ...
    )

    transfer, err := service.InitiatePayoutForVoucher(ctx, "voucher-id")
    assert.NoError(t, err)
    assert.Equal(t, "approved", transfer.Status)
}
```

**Test PayoutWorker orchestration:**
```go
func TestInitiateApprovedTransfers(t *testing.T) {
    mockTransferRepo := &MockTransferRepository{
        transfers: []*Transfer{
            {ID: "1", Status: "approved", Amount: 100000},
        },
    }

    worker := NewPayoutWorker(...)
    err := worker.initiateApprovedTransfers(ctx)

    assert.NoError(t, err)
    assert.Equal(t, "initiated", mockTransferRepo.transfers[0].Status)
}
```

---

### Integration Testing

**Manual Testing:**
1. Start worker: `./bin/payout_worker`
2. Create transfer via API (sets status='approved')
3. Wait 1 minute → worker picks it up
4. Verify transfer moves to 'initiated'
5. Verify Castler API was called
6. Test timeout by waiting 60 min without webhook

**Automated Testing:**
- Mock Castler API responses
- Verify database state changes
- Test graceful shutdown
- Test error handling (continue on failure)

---

## Monitoring & Observability

### Logs

**Worker outputs:**
- `🏃 Running initial worker cycle...` - Startup
- `📋 Task 1: Processing ready vouchers...` - Task execution
- `✅ No approved transfers to initiate` - No work to do
- `📦 Found 5 approved transfers to initiate` - Work found
- `⚠️ Found 2 transfers that exceeded webhook timeout` - Timeouts detected

**Error logs:**
```go
log.Printf("⚠️  Worker run failed: %v", err)
log.Printf("❌ Failed to initiate transfer %s: %v", transferID, err)
```

### Metrics (Future)

- Total vouchers processed per run
- Total transfers initiated per run
- Total timeouts flagged per run
- Average execution time per task
- Success/failure rates

---

## Future Enhancements

### 1. Smart Retry Strategy
```go
func (w *PayoutWorker) retryFailedTransfers(ctx context.Context) error {
    config := w.configRepo.GetConfig(ctx, nil)

    // Get failed transfers eligible for retry
    transfers := w.transferRepo.GetByStatus(ctx, "failed", config.BatchSize)

    for _, transfer := range transfers {
        if transfer.RetryCount >= config.MaxRetryAttempts {
            continue  // Max retries exceeded
        }

        // Exponential backoff
        delay := config.RetryDelayMinutes * math.Pow(config.RetryBackoffMultiplier, transfer.RetryCount)
        nextRetry := transfer.FailedAt.Add(time.Duration(delay) * time.Minute)

        if time.Now().Before(nextRetry) {
            continue  // Too early to retry
        }

        // Retry transfer
        w.initiateTransfer(ctx, transfer, config)
    }
}
```

### 2. Event-Driven Architecture
Instead of polling every minute, listen to database events:
```go
// Listen to PostgreSQL NOTIFY
LISTEN voucher_ready_for_payout;
```

### 3. Distributed Worker (Future)
For high volume, run multiple worker instances:
- Add leader election (Redis/etcd)
- Partition work by business_id
- Prevent duplicate processing

### 4. Observability Integration
- Prometheus metrics export
- Distributed tracing (OpenTelemetry)
- Structured logging (JSON format)

---

## Analogy: Restaurant Kitchen

**PayoutService** = **Chef** (knows how to cook a dish)
- Validates ingredients
- Follows recipe
- Prepares one dish at a time
- Can be called by waiter (API) or kitchen manager (worker)

**PayoutWorker** = **Kitchen Manager** (coordinates multiple orders)
- Every 1 minute, checks pending orders
- Tells chef to cook 10 dishes in batch
- Monitors cooking time (timeouts)
- Handles failed dishes (retries)

---

## Summary

| Component | Purpose | Called By | Runs |
|-----------|---------|-----------|------|
| **PayoutService** | Business logic (HOW) | API handlers, Worker | On-demand |
| **PayoutWorker** | Automation (WHEN + batch) | Background scheduler | Every 1 min |

**Key Principles:**
- ✅ **Separation of Concerns** - Service handles business rules, Worker handles orchestration
- ✅ **Reusability** - Service can be used by API and Worker
- ✅ **Testability** - Each layer can be tested independently
- ✅ **Scalability** - Can add more workers or adjust batch sizes
- ✅ **Maintainability** - Changes to business logic don't affect scheduling

**Both paths (manual + automated) use the same business logic from PayoutService!**

---

## Related Documentation

- [Payout Workflow](./payout_workflow.md) - Overall payout flow (settlement → payout)
- [Payment Voucher Integration](./payment_voucher_integration.md) - Payment → Voucher flow
- [Folder Structure](./FOLDER_STRUCTURE.md) - Project organization
