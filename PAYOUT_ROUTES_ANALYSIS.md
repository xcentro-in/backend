# Payout Routes Analysis & Cleanup Recommendations

## Executive Summary

After analyzing all payout routes from route → handler → service → repository, here's what needs to be done:

**Routes to REMOVE (3)**: Truly deprecated endpoints that return HTTP 410 Gone
**Routes to KEEP (10)**: Active endpoints with working implementations
**Routes with ISSUES (2)**: Routes with conflicts or concerns

---

## Current Routes in `/ops/v1/payouts`

### Active Routes (10) ✅

| # | Method | Path | Handler | Service | Repository | Purpose |
|---|--------|------|---------|---------|------------|---------|
| 1 | GET | `/payouts` | ListPayouts | OpsPayoutService | OpsPayoutRepository | List payouts with pagination |
| 2 | GET | `/payouts/summary/:id` | GetPayoutSummary | OpsPayoutService | OpsPayoutRepository | Get payout details |
| 3 | POST | `/payouts/initiate_payout` | InitiatePayoutManually | PayoutService | Multiple repos | **PRIMARY ACTION** - Manual payout initiation |
| 7 | GET | `/payouts/:transfer_id` | GetTransferDetails | PayoutService | TransferRepository | Get transfer details |
| 8 | GET | `/payouts/:transfer_id/webhooks` | GetWebhookHistory | PayoutService | TransferRepository | View webhook history |
| 9 | GET | `/payouts/timeouts` | GetTimeoutTransfers | Direct repo call | TransferRepository | List timeout transfers |
| 10 | POST | `/payouts/:transfer_id/mark-completed` | MarkCompletedManually | PayoutService | TransferRepository | Manual completion for timeouts |
| 11 | GET | `/payouts/config` | GetConfig | Direct repo call | ConfigRepository | Get payout config |
| 12 | PUT | `/payouts/config` | UpdateConfig | Direct repo call | ConfigRepository | Update payout config |
| 13 | POST | `/payouts/config` | CreateConfig | Direct repo call | ConfigRepository | Create business config |

### Deprecated Routes - Return HTTP 410 Gone (3) ⚠️

| # | Method | Path | Handler | Status | Action |
|---|--------|------|---------|--------|--------|
| 4 | GET | `/payouts/pending-approvals` | GetPendingApprovals | Returns 410 | **REMOVE** |
| 5 | POST | `/payouts/:transfer_id/approve` | ApproveTransfer | Returns 410 | **REMOVE** |
| 6 | POST | `/payouts/:transfer_id/cancel` | CancelTransfer | Returns 410 | **REMOVE** |

**Reason for deprecation**: Approval workflow was removed in payout redesign. All payouts are now auto-initiated after settlement.

---

## Detailed Route Analysis

### Route #1: GET /payouts ✅ KEEP
```
Handler:     OpsPayoutHandler.ListPayouts
Service:     OpsPayoutService.ListPayouts
Repository:  OpsPayoutRepository.ListPayoutsPaginated
```
**Status**: Active, core functionality
**Implementation**: Full chain implemented
**Purpose**: Ops dashboard listing with pagination and status filters

---

### Route #2: GET /payouts/summary/:id ✅ KEEP
```
Handler:     OpsPayoutHandler.GetPayoutSummary
Service:     OpsPayoutService.GetPayoutSummary
Repository:  OpsPayoutRepository.GetPayoutSummary
```
**Status**: Active, detail view
**Implementation**: Full chain implemented
**Purpose**: Detailed summary of a single payout transfer

---

### Route #3: POST /payouts/initiate_payout ✅ KEEP
```
Handler:     PayoutHandler.InitiatePayoutManually
Service:     PayoutService.InitiateManualPayout
Repository:  Multiple (Transfer, Payment, Voucher, BankAccount, Config)
```
**Status**: **PRIMARY ACTION ENDPOINT**
**Implementation**: Complete orchestration with:
- Settlement status validation
- Voucher processing
- Transfer request creation
- Async Castler API calls
- Activity tracking

**Purpose**: Manual payout initiation by ops team for settled payments

---

### Route #4: GET /payouts/pending-approvals ❌ REMOVE
```go
func (h *PayoutHandler) GetPendingApprovals(c echo.Context) error {
    h.logger.WarnWith("GetPendingApprovals API called - DEPRECATED")
    return c.JSON(http.StatusGone, map[string]interface{}{
        "error":   "feature_removed",
        "message": "Approval workflow has been removed...",
    })
}
```
**Status**: TRULY DEPRECATED - Returns HTTP 410 Gone
**Service**: None - hardcoded response
**Recommendation**: **DELETE** - No longer used, approval workflow removed

---

### Route #5: POST /payouts/:transfer_id/approve ❌ REMOVE
```go
func (h *PayoutHandler) ApproveTransfer(c echo.Context) error {
    h.logger.WarnWith("ApproveTransfer API called - DEPRECATED")
    return c.JSON(http.StatusGone, map[string]interface{}{
        "error":   "feature_removed",
        "message": "Approval workflow has been removed...",
    })
}
```
**Status**: TRULY DEPRECATED - Returns HTTP 410 Gone
**Service**: None - hardcoded response
**Recommendation**: **DELETE** - No longer used

---

### Route #6: POST /payouts/:transfer_id/cancel ⚠️ DEPRECATED BUT UNCLEAR
```go
func (h *PayoutHandler) CancelTransfer(c echo.Context) error {
    h.logger.WarnWith("CancelTransfer API called - DEPRECATED")
    return c.JSON(http.StatusGone, map[string]interface{}{
        "error":   "feature_removed",
        "message": "Cancellation workflow has been removed...",
    })
}
```
**Status**: Returns HTTP 410 Gone
**Service**: None - hardcoded response
**Concern**: Cancel might still be needed for operational reasons?
**Recommendation**: **DELETE** - but confirm with team if manual cancellation is never needed

---

### Route #7: GET /payouts/:transfer_id ⚠️ POTENTIAL CONFLICT
```
Handler:     PayoutHandler.GetTransferDetails
Service:     PayoutService.GetTransferDetails
Repository:  TransferRepository.GetByID
```
**Status**: Active
**ISSUE**: Conflicts with route #2 (GET /payouts/summary/:id)
- Route #2 uses `/payouts/summary/:id` → OpsPayoutRepository (detailed summary)
- Route #7 uses `/payouts/:transfer_id` → TransferRepository (basic details)

**Recommendation**:
- **KEEP Route #2** (summary/:id) - More detailed, ops-specific
- **CONSIDER REMOVING Route #7** - Redundant with route #2
- Or rename to `/payouts/transfer/:id` to avoid path conflicts

---

### Route #8: GET /payouts/:transfer_id/webhooks ✅ KEEP
```
Handler:     PayoutHandler.GetWebhookHistory
Service:     PayoutService.GetWebhookHistory
Repository:  TransferRepository.GetByID (reads webhook_history field)
```
**Status**: Active
**Implementation**: Returns webhook_history array from transfer record
**Purpose**: Debugging and audit trail for webhook events

---

### Route #9: GET /payouts/timeouts ✅ KEEP
```
Handler:     PayoutHandler.GetTimeoutTransfers
Service:     None (direct repo call)
Repository:  TransferRepository.GetWebhookTimeouts
```
**Status**: Active
**Implementation**: Direct repository call
**Purpose**: Find transfers that exceeded webhook timeout threshold

---

### Route #10: POST /payouts/:transfer_id/mark-completed ✅ KEEP
```
Handler:     PayoutHandler.MarkCompletedManually
Service:     PayoutService.MarkCompletedManually
Repository:  TransferRepository.MarkCompleted
```
**Status**: Active
**Implementation**: Full chain - validates timeout status, marks completed
**Purpose**: Manual intervention when webhook fails but transfer actually completed
**Validation**: Only allows timeout transfers to be manually completed

---

### Route #11: GET /payouts/config ✅ KEEP
```
Handler:     PayoutHandler.GetConfig
Service:     None (direct repo call)
Repository:  ConfigRepository.GetConfig
```
**Status**: Active
**Implementation**: Direct repository call
**Purpose**: View payout configuration (global or business-specific)

---

### Route #12: PUT /payouts/config ✅ KEEP
```
Handler:     PayoutHandler.UpdateConfig
Service:     None (direct repo call)
Repository:  ConfigRepository.UpdateConfig
```
**Status**: Active
**Implementation**: Direct repository call
**Purpose**: Update payout configuration

---

### Route #13: POST /payouts/config ✅ KEEP
```
Handler:     PayoutHandler.CreateConfig
Service:     None (direct repo call)
Repository:  ConfigRepository.Create
```
**Status**: Active
**Implementation**: Direct repository call
**Purpose**: Create business-specific payout configuration

---

## Cleanup Recommendations

### 1. REMOVE Deprecated Routes (Priority: HIGH)

**Files to modify**:
- `internal/handlers/http/payout/payout_handler.go`

**Routes to DELETE**:
```go
// Lines 55-64: GetPendingApprovals - DELETE
// Lines 66-75: ApproveTransfer - DELETE
// Lines 77-86: CancelTransfer - DELETE
```

**Route registration to REMOVE** (in RegisterOpsDashboardPayoutRoutes):
```go
payoutGroup.GET("/pending-approvals", handler.GetPendingApprovals)       // DELETE
payoutGroup.POST("/:transfer_id/approve", handler.ApproveTransfer)       // DELETE
payoutGroup.POST("/:transfer_id/cancel", handler.CancelTransfer)         // DELETE
```

---

### 2. RESOLVE Route Conflict (Priority: MEDIUM)

**Issue**: Two endpoints for getting transfer details:
- `GET /payouts/summary/:id` (OpsPayoutRepository - detailed)
- `GET /payouts/:transfer_id` (TransferRepository - basic)

**Options**:

**Option A** (Recommended): Remove redundant route #7
```go
// DELETE this from RegisterOpsDashboardPayoutRoutes:
payoutGroup.GET("/:transfer_id", handler.GetTransferDetails)  // REMOVE - redundant with /summary/:id
```

**Option B**: Rename to avoid path collision
```go
// Change to:
payoutGroup.GET("/transfer/:transfer_id", handler.GetTransferDetails)  // More specific path
```

**Recommendation**: Choose Option A - route #2 provides more comprehensive data

---

### 3. REMOVE Deprecated Service Method (Priority: LOW)

**File**: `internal/services/payout/payout_service.go`

**Method to DELETE** (lines 216-288):
```go
// InitiatePayoutForVoucher - DEPRECATED: Use InitiateManualPayout instead
func (s *PayoutService) InitiatePayoutForVoucher(...) {...}
```
**Reason**: Replaced by InitiateManualPayout which works with payment_id

---

### 4. CHECK Repository Method Usage (Priority: LOW)

**Methods in repository but not exposed via routes**:
- `TransferRepository.GetByStatus` - Used by workers?
- `TransferRepository.UpdateStatus` - Used internally/webhooks
- `TransferRepository.GetByProviderTransferID` - Used by webhooks
- `TransferRepository.AppendWebhookHistory` - Used by webhooks

**Action**: Verify these are used by webhook handler and workers (not ops routes)

---

## Path Collision Issues

### Current Issue:
```
GET /payouts/summary/:id          → OpsPayoutHandler.GetPayoutSummary
GET /payouts/timeouts             → PayoutHandler.GetTimeoutTransfers
GET /payouts/config               → PayoutHandler.GetConfig
GET /payouts/:transfer_id         → PayoutHandler.GetTransferDetails  ⚠️  CATCHES ALL!
```

The route `GET /payouts/:transfer_id` will match ANY GET request to `/payouts/[anything]` because `:transfer_id` is a wildcard parameter.

**Echo router evaluation order**:
1. Static routes first (exact matches)
2. Parameterized routes last

**Result**: Routes like `/summary/:id`, `/timeouts`, `/config` should be registered BEFORE `/:transfer_id`

**Current registration order in code**:
```go
// ops_payout_handler.go (RegisterOpsPayoutRoutes)
payoutGroup.GET("", handler.ListPayouts)              // OK: exact match /payouts
payoutGroup.GET("/summary/:id", handler.GetPayoutSummary)  // OK: static prefix

// payout_handler.go (RegisterOpsDashboardPayoutRoutes)
payoutGroup.POST("/initiate_payout", ...)             // OK: static
payoutGroup.GET("/pending-approvals", ...)            // OK: static
payoutGroup.POST("/:transfer_id/approve", ...)        // OK: has subpath
payoutGroup.POST("/:transfer_id/cancel", ...)         // OK: has subpath
payoutGroup.GET("/:transfer_id", ...)                 // ⚠️  PROBLEM - catches everything!
payoutGroup.GET("/:transfer_id/webhooks", ...)        // OK: has subpath
payoutGroup.GET("/timeouts", ...)                     // ⚠️  MAY BE SHADOWED
payoutGroup.POST("/:transfer_id/mark-completed", ...) // OK: has subpath
payoutGroup.GET("/config", ...)                       // ⚠️  MAY BE SHADOWED
payoutGroup.PUT("/config", ...)                       // OK: different method
payoutGroup.POST("/config", ...)                      // OK: different method
```

**FIX**: Remove `GET /:transfer_id` entirely (use `/summary/:id` instead)

---

## Summary of Actions

### Immediate Actions (Can be done now):

1. **DELETE 3 deprecated route handlers** (payout_handler.go:55-86)
   - GetPendingApprovals
   - ApproveTransfer
   - CancelTransfer

2. **DELETE 3 route registrations** (payout_handler.go:680-682)
   ```go
   payoutGroup.GET("/pending-approvals", handler.GetPendingApprovals)
   payoutGroup.POST("/:transfer_id/approve", handler.ApproveTransfer)
   payoutGroup.POST("/:transfer_id/cancel", handler.CancelTransfer)
   ```

3. **DELETE redundant route** (payout_handler.go:683)
   ```go
   payoutGroup.GET("/:transfer_id", handler.GetTransferDetails)  // Redundant with /summary/:id
   ```

4. **DELETE deprecated service method** (payout_service.go:216-288)
   ```go
   func (s *PayoutService) InitiatePayoutForVoucher(...)  // Use InitiateManualPayout instead
   ```

### Verification Actions:

5. **Verify webhook handler** uses TransferRepository methods:
   - GetByProviderTransferID
   - UpdateStatus
   - AppendWebhookHistory
   - MarkCompleted/MarkFailed

6. **Verify payout worker** (if exists) uses:
   - GetByStatus
   - UpdateStatus

---

## Final Route List (After Cleanup)

### `/ops/v1/payouts` - **10 routes remaining**

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/payouts` | ListPayouts | List all payouts |
| GET | `/payouts/summary/:id` | GetPayoutSummary | Get payout details |
| POST | `/payouts/initiate_payout` | InitiatePayoutManually | **PRIMARY** - Initiate payout |
| GET | `/payouts/:transfer_id/webhooks` | GetWebhookHistory | Webhook history |
| GET | `/payouts/timeouts` | GetTimeoutTransfers | List timeouts |
| POST | `/payouts/:transfer_id/mark-completed` | MarkCompletedManually | Manual completion |
| GET | `/payouts/config` | GetConfig | Get config |
| PUT | `/payouts/config` | UpdateConfig | Update config |
| POST | `/payouts/config` | CreateConfig | Create config |

**Removed**: 4 routes (3 deprecated + 1 redundant)
**Kept**: 10 active routes

---

## Code Changes Required

### File 1: internal/handlers/http/payout/payout_handler.go

**Delete lines 55-86** (3 deprecated handler methods):
- GetPendingApprovals
- ApproveTransfer
- CancelTransfer

**Modify RegisterOpsDashboardPayoutRoutes** (lines 662-690):
```go
func RegisterOpsDashboardPayoutRoutes(...) {
    handler := NewPayoutHandler(...)

    payoutGroup := e.Group("/payouts")
    payoutGroup.POST("/initiate_payout", handler.InitiatePayoutManually)
    // DELETE: payoutGroup.GET("/pending-approvals", handler.GetPendingApprovals)
    // DELETE: payoutGroup.POST("/:transfer_id/approve", handler.ApproveTransfer)
    // DELETE: payoutGroup.POST("/:transfer_id/cancel", handler.CancelTransfer)
    // DELETE: payoutGroup.GET("/:transfer_id", handler.GetTransferDetails)  // Redundant
    payoutGroup.GET("/:transfer_id/webhooks", handler.GetWebhookHistory)
    payoutGroup.GET("/timeouts", handler.GetTimeoutTransfers)
    payoutGroup.POST("/:transfer_id/mark-completed", handler.MarkCompletedManually)
    payoutGroup.GET("/config", handler.GetConfig)
    payoutGroup.PUT("/config", handler.UpdateConfig)
    payoutGroup.POST("/config", handler.CreateConfig)
}
```

### File 2: internal/services/payout/payout_service.go

**Delete lines 216-288**:
```go
// DELETE entire function
func (s *PayoutService) InitiatePayoutForVoucher(...) {...}
```

### File 3: internal/services/payout/payout_service.go

**Delete lines 308-316** (unused RetryTransfer placeholder):
```go
// DELETE - not implemented, no route uses it
func (s *PayoutService) RetryTransfer(...) {...}
```

---

## Dependencies Check

### These repository methods are still used:

**TransferRepository** (internal/adapters/database/postgres/payout/transfer_repository.go):
- ✅ Create - Used by InitiateManualPayout
- ✅ GetByID - Used by GetTransferDetails, GetWebhookHistory, MarkCompletedManually
- ✅ GetByVoucherID - Used by InitiateManualPayout (idempotency check)
- ✅ GetByProviderTransferID - Used by webhook handler
- ✅ UpdateStatus - Used by webhook handler, async processing
- ✅ UpdateProviderTransferID - Used by async processing
- ✅ MarkCompleted - Used by MarkCompletedManually, webhook handler
- ✅ MarkFailed - Used by async processing error handling
- ✅ SetWebhookTimeout - Used by async processing
- ✅ AppendWebhookHistory - Used by webhook handler
- ✅ GetWebhookTimeouts - Used by GetTimeoutTransfers
- ⚠️ GetByStatus - Check if payout worker uses this

**ConfigRepository** (internal/adapters/database/postgres/payout/configuration_repository.go):
- ✅ GetConfig - Used by GetConfig handler, InitiateManualPayout
- ✅ UpdateConfig - Used by UpdateConfig handler
- ✅ Create - Used by CreateConfig handler

**OpsPayoutRepository** (internal/adapters/database/postgres/payout/ops_payout_repository.go):
- ✅ ListPayoutsPaginated - Used by ListPayouts
- ✅ GetPayoutSummary - Used by GetPayoutSummary

All repository methods are actively used. No dead code in repositories.

---

## Testing Checklist

After cleanup, test:

1. ✅ POST /ops/v1/payouts/initiate_payout - Primary action
2. ✅ GET /ops/v1/payouts - Listing works
3. ✅ GET /ops/v1/payouts/summary/:id - Details work
4. ✅ GET /ops/v1/payouts/timeouts - Not shadowed by removed route
5. ✅ GET /ops/v1/payouts/config - Not shadowed by removed route
6. ✅ GET /ops/v1/payouts/:transfer_id/webhooks - Subpath still works
7. ✅ POST /ops/v1/payouts/:transfer_id/mark-completed - Subpath still works
8. ❌ GET /ops/v1/payouts/pending-approvals - Should return 404 (removed)
9. ❌ POST /ops/v1/payouts/:id/approve - Should return 404 (removed)
10. ❌ POST /ops/v1/payouts/:id/cancel - Should return 404 (removed)

---

## Migration Notes

No frontend changes required if:
- Frontend never calls pending-approvals, approve, cancel endpoints
- Frontend uses /summary/:id instead of /:transfer_id

**Check frontend code for**:
```javascript
// These should NOT exist:
GET /payout/dashboard/pending-approvals  // Old deprecated endpoint
POST /payout/dashboard/:id/approve       // Old deprecated endpoint
POST /payout/dashboard/:id/cancel        // Old deprecated endpoint
GET /payout/dashboard/:id                // Old endpoint (use /summary/:id)

// Should use:
GET /ops/v1/payouts/summary/:id          // New summary endpoint
```
