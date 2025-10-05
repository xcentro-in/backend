# Phase 9: Next Steps - Quick Reference

## 🎯 What's Done (67% Complete)

✅ **Documentation** - Complete workflow guide
✅ **Database Schema** - Migration 000009 applied
✅ **Service Layer** - Auto-approval logic, transfer management
✅ **Worker Structure** - Binary compiles, logic implemented

## ⚠️ What's Missing (33% Remaining)

### Critical Path (Must Complete for MVP)

#### 1. Repository Implementations (~6 hours)
**Create these files**:

```
internal/adapters/database/postgres/payout/
├── transfer_repository.go          (300 lines)
├── configuration_repository.go     (100 lines)
```

**Update**:
```
internal/adapters/database/postgres/voucher/voucher_repository.go
└── Add: GetVouchersReadyForPayout() method
```

**Template for transfer_repository.go**:
```go
package payout

import (
    "context"
    "encoding/json"
    "github.com/jackc/pgx/v5"
    "xcentro.in/internal/adapters/database/postgres/client"
    "xcentro.in/internal/core/payout"
)

type PostgresTransferRepository struct {
    querier client.Querier
}

func NewPostgresTransferRepository(db client.DB) payout.PayoutTransferRepository {
    return &PostgresTransferRepository{querier: db}
}

// Implement 15 methods from interface...
```

#### 2. Webhook Handler (~4 hours)
**Create**:
```
internal/handlers/http/payout/webhook_handler.go
```

**Implementation Checklist**:
- [ ] Parse webhook payload
- [ ] Verify HMAC signature
- [ ] Find transfer by `provider_transfer_id`
- [ ] Handle 3 event types:
  - `transfer.processing` → update status
  - `transfer.completed` → mark completed with UTR
  - `transfer.failed` → mark failed
- [ ] Store webhook in `webhook_history[]` array
- [ ] Update voucher `payout_status`

**Endpoint**: `POST /api/v1/webhooks/payout/castler`

#### 3. Operations Dashboard (~5 hours)
**Create**:
```
internal/handlers/http/payout/payout_handler.go
```

**8 Endpoints to Implement**:
```
GET  /operations/payouts/pending-approvals   - List pending
POST /operations/payouts/:id/approve         - Approve transfer
POST /operations/payouts/:id/cancel          - Cancel transfer
GET  /operations/payouts/:id                 - Get details
GET  /operations/payouts/:id/webhooks        - Webhook history
GET  /operations/payouts/timeout             - Timeout list
POST /operations/payouts/:id/mark-completed  - Manual complete
GET  /operations/payouts/config              - Get config
PUT  /operations/payouts/config              - Update config
```

#### 4. Configuration (~1 hour)
**Update**:
```go
// internal/shared/config/config.go
type Config struct {
    // ... existing
    Castler CastlerConfig `json:"castler"`
}

type CastlerConfig struct {
    BaseURL       string `env:"CASTLER_BASE_URL" env-required:"true"`
    APIKey        string `env:"CASTLER_API_KEY" env-required:"true"`
    SecretKey     string `env:"CASTLER_SECRET_KEY" env-required:"true"`
    WebhookSecret string `env:"CASTLER_WEBHOOK_SECRET" env-required:"true"`
}
```

**Add to `.env`**:
```bash
CASTLER_BASE_URL=https://sandbox.castler.in
CASTLER_API_KEY=your_key
CASTLER_SECRET_KEY=your_secret
CASTLER_WEBHOOK_SECRET=your_webhook_secret
```

#### 5. Worker Finalization (~1 hour)
**Update**: `cmd/payout_worker/main.go`
- Uncomment worker initialization
- Use config for Castler credentials
- Add scheduler with graceful shutdown

---

## 📋 Step-by-Step Implementation Guide

### Day 1: Repositories (Critical Foundation)

**Morning (3 hours)**:
1. Create `transfer_repository.go`
2. Implement CRUD methods:
   - `Create()`, `GetByID()`, `GetByVoucherID()`
   - `GetByProviderTransferID()` (for webhooks!)
   - `GetByStatus()` (for worker)
3. Implement status updates:
   - `UpdateStatus()`, `UpdateProviderTransferID()`
   - `SetWebhookTimeout()`, `AppendWebhookHistory()`

**Afternoon (3 hours)**:
1. Implement approval/completion:
   - `ApproveTransfer()`, `MarkCompleted()`, `MarkFailed()`
2. Implement dashboard queries:
   - `GetPendingApprovals()`, `GetWebhookTimeouts()`
3. Create `configuration_repository.go`:
   - `GetConfig()` with business fallback logic
   - `UpdateConfig()`, `Create()`
4. Update `VoucherRepository.GetVouchersReadyForPayout()`

### Day 2: Webhook Handler (Critical for Status Updates)

**Morning (2 hours)**:
1. Create webhook handler structure
2. Implement signature verification
3. Parse Castler webhook payload

**Afternoon (2 hours)**:
1. Implement event handlers:
   - `handleProcessing()` → status='processing'
   - `handleCompleted()` → status='completed', update UTR
   - `handleFailed()` → status='failed', log reason
2. Store webhook in history array
3. Update voucher payout status
4. Add route: `POST /api/v1/webhooks/payout/castler`

### Day 3: Operations Dashboard

**Morning (3 hours)**:
1. Create payout handler
2. Implement 4 core endpoints:
   - Pending approvals list
   - Approve transfer
   - Cancel transfer
   - Get transfer details

**Afternoon (2 hours)**:
1. Implement 4 monitoring endpoints:
   - Webhook history view
   - Timeout transfers list
   - Manual completion
   - Config get/update
2. Add routes to `cmd/api/routes.go`
3. Add JWT middleware

### Day 4: Integration & Testing

**Morning (2 hours)**:
1. Add Castler config to `config.Config`
2. Update worker `main.go` with config
3. Uncomment worker initialization
4. Add scheduler logic

**Afternoon (3 hours)**:
1. End-to-end testing:
   - Create settled voucher
   - Initiate payout (API)
   - Worker initiates with Castler
   - Simulate webhook
   - Verify completion
2. Test timeout scenario
3. Test approval workflow (> ₹2L)

---

## 🔧 Quick Commands

### Build Everything
```bash
go build ./cmd/api
go build -o bin/payout_worker ./cmd/payout_worker
```

### Run Migration
```bash
./bin/migrate -action=up
```

### Start Worker (once complete)
```bash
./bin/payout_worker
```

### Test Webhook (once handler created)
```bash
curl -X POST http://localhost:8080/api/v1/webhooks/payout/castler \
  -H "X-Castler-Signature: <hmac>" \
  -H "Content-Type: application/json" \
  -d @webhook_payload.json
```

---

## 🚨 Critical Dependencies

**Must Complete in Order**:
1. ✅ Database schema (done)
2. ✅ Service layer (done)
3. **→ Repository implementations** ← START HERE
4. → Webhook handler
5. → Operations dashboard
6. → Configuration updates

**Blocking Issues**:
- ❌ Worker can't run without transfer repository
- ❌ Webhooks can't work without handler
- ❌ Dashboard can't work without endpoints

---

## 📊 Estimated Completion Time

| Component | Hours | Status |
|-----------|-------|--------|
| Repositories | 6h | ⏳ Not Started |
| Webhook Handler | 4h | ⏳ Not Started |
| Operations Dashboard | 5h | ⏳ Not Started |
| Configuration | 1h | ⏳ Not Started |
| Testing & Integration | 4h | ⏳ Not Started |
| **TOTAL** | **~20h** | **33% Remaining** |

**Best Case**: 2.5 days (focused work)
**Realistic**: 4-5 days (with other tasks)

---

## ✅ Definition of Done

Phase 9 is complete when:

- [ ] All repository methods implemented
- [ ] Webhook handler processes all 3 event types
- [ ] Ops dashboard has all 8 endpoints functional
- [ ] Worker runs without errors
- [ ] End-to-end test passes:
  1. Voucher settled → Transfer created
  2. Worker initiates → Castler API called
  3. Webhook received → Status updated
  4. Voucher marked completed
- [ ] Timeout scenario tested
- [ ] Manual approval tested (> ₹2L)
- [ ] Documentation updated with actual implementation notes

---

## 📞 Help & References

- **Architecture**: `docs/payout_workflow.md`
- **Implementation Status**: `docs/phase9_implementation_status.md`
- **Migration**: `migrations/sql/000009_*.sql`
- **Service**: `internal/services/payout/payout_service.go`
- **Worker**: `internal/workers/payout_worker.go`

---

**Quick Start Command** (once repos are implemented):
```bash
# Terminal 1: Run API
go run ./cmd/api

# Terminal 2: Run Worker
./bin/payout_worker

# Terminal 3: Test
curl -X POST http://localhost:8080/api/v1/payouts/initiate \
  -H "Content-Type: application/json" \
  -d '{"voucher_id": "uuid-here"}'
```

---

**Last Updated**: 2024-10-05
**Status**: 67% Complete - Repository implementation is next critical path
