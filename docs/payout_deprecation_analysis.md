# Payout System Deprecation Analysis (Split Settlement Migration)

## Context

The existing payout flow routes collected funds through a Castler escrow account, requiring
a separate manual/automated outbound transfer step ("payout") to move money from the escrow
to each payee's bank account.

The new approach is **split settlement**: at payment initiation time, split instructions are
provided to the payment gateway (JioPay / Easebuzz). On T+1 the gateway itself bifurcates
the funds directly into each payee's account, and the settlement webhook carries the
individual UTRs per payee. The escrow leg and the entire outbound Castler payout machinery
are therefore no longer needed.

---

## What to Delete Entirely

### 1. Castler Payout Adapter (`internal/adapters/external/payout/`)

| File | Reason |
|---|---|
| `castler/castler_client.go` | Implements `PayoutProvider` (outbound transfers) and `OnboardBeneficiary` (Castler beneficiary registration). Both are obsolete. |
| `castler/castler_types.go` | Types for `CastlerBeneficiaryRequest/Response`, payout transfer requests — all Castler payout types. |
| `castler/mock_castler_client.go` | Mock implementation used in tests. Goes away with the real client. |
| `factory/provider_factory.go` | `PayoutProviderFactory` that creates Castler clients for outbound payouts. |

> **Note on `castler/castler_bank_verification.go`**: This file implements the
> `BankVerificationService` interface (penny-drop / account verification via Castler). It
> is used during payee onboarding, not for outbound payouts. **Keep it** if Castler
> continues to be used for penny drop; remove it only if a different provider replaces it.

---

### 2. Payout Domain (`internal/core/payout/`)

The entire package becomes dead code:

| File | Content |
|---|---|
| `payout_entity.go` | `PayoutTransferRequest`, `PayoutConfiguration`, `PayoutProviderMapping` |
| `payout_provider.go` | `PayoutProvider` interface, all request/response types for beneficiary onboarding and transfer |
| `payout_repository.go` | `PayoutProviderMappingRepository`, `PayoutTransferRepository`, `PayoutConfigurationRepository`, `OpsPayoutRepository` |
| `payout_dto.go` | DTOs for payout operations |
| `payout_errors.go` | Error definitions |
| `ops_payout_types.go` | Ops dashboard listing types (`OpsPayoutListItem`, `OpsPayoutSummary`, etc.) |

---

### 3. Payout Services (`internal/services/payout/`)

| File | Content |
|---|---|
| `payout_service.go` | `InitiateManualPayout`, `ProcessTransferToCastler`, `InitiateAndProcessPayout`, `MarkCompletedManually`, `RetryTransfer`, etc. — the entire manual/auto payout orchestration. |
| `ops_payout_service.go` | `OpsPayoutService` — listing payout transfers for the ops dashboard. |

The validation helper `ValidatePaymentAmountsForPayout` (used by the worker) also goes away.

---

### 4. Payout Handlers (`internal/handlers/http/payout/`)

| File | Content |
|---|---|
| `payout_handler.go` | `InitiatePayoutManually`, config CRUD endpoints, deprecated approval/cancel endpoints. |
| `webhook_handler.go` | `HandleCastlerWebhook` — receives Castler outbound-transfer completion webhooks. |
| `ops_payout_handler.go` | `ListPayouts`, `GetPayoutSummary`, `GetPayoutDetail`, `InitiatePayoutManually` for the ops dashboard. |

---

### 5. Payout Worker

| File | Content |
|---|---|
| `internal/workers/payout_worker.go` | Periodic worker that auto-retries initiated transfers, flags timeouts, auto-initiates payouts for settled payments. |
| `cmd/workers/payout_worker/main.go` | Entry point for the worker binary. |

The worker binary is a separate deployable process; it should be taken down after the
switch to split settlement.

---

### 6. Payout Database Adapters (`internal/adapters/database/postgres/payout/`)

| File | Content |
|---|---|
| `transfer_repository.go` | `PostgresPayoutTransferRepository` — CRUD for `payout_transfer_requests` table. |
| `ops_payout_repository.go` | `PostgresOpsPayoutRepository` — complex queries for the ops dashboard listing. |
| `configuration_repository.go` | `PostgresPayoutConfigurationRepository` — config table CRUD (already dropped by migration 000050). |

---

### 7. Database Tables (New Migration Needed)

A new migration must be created to drop the payout-specific tables:

```sql
-- New migration: NNN_drop_payout_tables.up.sql
DROP TABLE IF EXISTS xcentro.payout_transfer_requests;
DROP TYPE IF EXISTS xcentro.payout_transfer_status_enum;
-- payout_configuration already dropped by migration 000050
-- payout_provider_mappings dropped by migration 000020
```

The reverse migration should recreate the table (for rollback safety).

---

## Coupling Points That Must Be Mended

These files are **not** part of the payout package but have direct dependencies on payout
logic that must be removed or replaced.

---

### 8. Payee Service — Castler Beneficiary Onboarding

**File:** `internal/services/payee/payee_service.go`

The payee onboarding flow (specifically `OnboardVendorPayee`) calls
`s.castlerClient.OnboardPayeeToCastler(...)` (lines ~300–313) immediately after bank
verification passes, and stores the returned `castlerPayeeResp.Result` as
`CastlerBeneficiaryID` in the bank account entity (line ~329).

A parallel pattern exists for employee onboarding around line ~972.

**Mend:**
- Remove the `OnboardPayeeToCastler` step from both `OnboardVendorPayee` and the employee
  path.
- Remove the `payoutProvider payout.PayoutProvider` field from `PayeeService`.
- Remove the `castlerClient CastlerPayeeOnboarder` field from `PayeeService`.
- Remove the `CastlerPayeeOnboarder` interface definition.
- Update the `NewPayeeService` constructor signature accordingly.
- The log lines referencing `castler_beneficiary_id` in the "account already exists" path
  (lines ~230, ~249, ~848, ~867) should be cleaned up too.

---

### 9. Payee Service Factory — Castler Client Wiring

**File:** `internal/services/payee/factory.go`

The factory currently:
1. Creates a `PayoutProviderFactory` and gets the Castler `PayoutProvider`.
2. Asserts it as `CastlerPayeeOnboarder`.
3. If mock mode is disabled, further asserts it as `*castler.CastlerClient` to build the
   bank verification service.

**Mend:**
- Remove steps 1 and 2 entirely (no more payout provider creation here).
- For bank verification, instantiate the Castler client directly (or via a lighter
  constructor) without going through the payout provider factory — or replace with whatever
  provider will be used for penny drop going forward.
- Remove imports: `payoutFactory`, `payout` core package.

---

### 10. Payee Bank Account Entity — `CastlerBeneficiaryID` Field

**File:** `internal/core/payee/payee_entity.go`

```go
// Line ~224
CastlerBeneficiaryID string `json:"castler_beneficiary_id,omitempty"`
```

This field exists solely to store a reference that the payout service uses to initiate
outbound transfers.

**Mend:** Remove the field from `PayeeBankAccount`.

---

### 11. Payee Bank Account Repository — `GetCastlerBeneficiaryID`

**File:** `internal/core/payee/payee_repository.go`

```go
// Line ~157
GetCastlerBeneficiaryID(ctx context.Context, accountID string) (string, error)
```

Only consumed by `payout_service.go` (`bankAccountRepo.GetCastlerBeneficiaryID`).

**Mend:** Remove from the `PayeeBankAccountRepository` interface.

**File:** `internal/adapters/database/postgres/payee/payee_bank_account_repository.go`

- Remove the `GetCastlerBeneficiaryID` method (lines ~322–359).
- Remove `castler_beneficiary_id` column from all `SELECT`, `INSERT`, and `UPDATE` SQL
  queries in this file (lines ~61, ~102, ~131, ~193, ~230, ~293, ~359, ~394).
- Remove scanning into `account.CastlerBeneficiaryID` (lines ~193, ~293).

A database migration must also remove the column:
```sql
ALTER TABLE xcentro.payee_bank_accounts DROP COLUMN IF EXISTS castler_beneficiary_id;
```

---

### 12. Payment State Change Actions — Payout Action Types

**File:** `internal/core/payment/payment_state_change.go`

Lines 27–32 define payout-specific action types:

```go
PayoutActionInitiated  PaymentAction = "payout_initiated"
PayoutActionProcessing PaymentAction = "payout_processing"
PayoutActionCompleted  PaymentAction = "payout_completed"
PayoutActionFailed     PaymentAction = "payout_failed"
PayoutActionTimeout    PaymentAction = "payout_timeout"
PayoutActionCancelled  PaymentAction = "payout_cancelled"
```

The `MapPaymentActionToUserFriendly` function maps these (lines ~59–73).
The `TransferID *uuid.UUID` field on `PaymentStateChange` and
`CreatePaymentStateChangeRequest` exists only for payout tracking.

**Mend:**
- Remove all `PayoutAction*` constants.
- Remove the corresponding `case` arms in `MapPaymentActionToUserFriendly`.
- Remove the `TransferID` field from the state change structs (or keep if needed for
  settlement webhook tracking in the new model).
- The `payment_state_changes` table rows with payout actions are historical data — no
  migration needed for existing rows, but new rows will no longer be written with these
  action types.

---

### 13. State Change Helper — Payout Logging

**File:** `internal/services/payment/state_change_helper.go`

Contains helper functions that log `PayoutActionInitiated`, `PayoutActionCompleted`,
`PayoutActionFailed` to `payment_state_changes` (lines ~129, ~154, ~180).

**Mend:** Remove the payout-specific helper functions.

---

### 14. Payment Repository — `GetSettledPaymentsReadyForPayout`

**File:** `internal/core/payment/payment_repository.go` (line ~30)

```go
GetSettledPaymentsReadyForPayout(ctx context.Context, limit int) ([]*Payment, error)
```

**File:** `internal/adapters/database/postgres/payment/payment_repository.go` (line ~654)

Implementation queries `WHERE settlement_status = 'settled'` to find payments ready for
the auto-payout worker.

**Mend:** Remove from both the interface and the implementation. This method is only called
by the payout worker.

---

### 15. Voucher Payment Details — Payout UTR and Completed At

**File:** `internal/adapters/database/postgres/voucher/voucher_repository.go` (lines ~2071–2082)

The `GetPaymentDetails` query JOINs `payout_transfer_requests` to retrieve:
```sql
COALESCE(ptr.utr, '') as payout_utr,
ptr.completed_at as payout_completed_at,
...
LEFT JOIN xcentro.payout_transfer_requests ptr ON v.voucher_id = ptr.voucher_id
```

**File:** `internal/core/voucher/voucher_approval_dto.go` (lines ~158–159)
```go
PayoutUTR         string     `json:"payout_utr"`
PayoutCompletedAt *time.Time `json:"payout_completed_at"`
```

**File:** `internal/services/voucher/models.go` (lines ~386–387) — same fields on
`VoucherPaymentDetails`.

**File:** `internal/services/voucher/voucher_service.go` (lines ~1272–1273) — maps the
fields.

**Mend:** With split settlement, the UTR for a payee's settlement arrives in the payment
gateway's settlement webhook. The coupling must be replaced by one of:
- **Option A**: Store per-voucher settlement UTRs from the settlement webhook (gateway-side
  split), then JOIN from that new column/table.
- **Option B**: Derive payout completion status from the payment's `settlement_status` +
  `settlement_utr` field (simpler if one UTR per payment suffices).

Until the replacement is implemented, remove the `LEFT JOIN` to
`payout_transfer_requests` and return empty/null for `payout_utr` /
`payout_completed_at`, so the API still works after the table is dropped.

Also remove the field from `VoucherPaymentDetails` (or rename to `settlement_utr` /
`settled_at`) once the new data source is wired in.

The voucher `Delete` function also has a guard that blocks deletion if
`payout_transfer_requests` rows exist (`voucher_repository.go` ~line 2148). Remove this
guard after the table is dropped.

---

### 16. Payment Activity Feed — Payout Action Filtering

**File:** `internal/handlers/http/payment/payment_handler.go` (lines ~1664–1669)

```go
sc.Action == payment.PayoutActionInitiated ||
sc.Action == payment.PayoutActionProcessing ||
sc.Action == payment.PayoutActionCompleted ||
sc.Action == payment.PayoutActionFailed ||
sc.Action == payment.PayoutActionTimeout ||
sc.Action == payment.PayoutActionCancelled
```

These guards in the payment activity feed handler select which state-change records to
show/hide.

**Mend:** Remove these filter conditions once the payout action types are deleted.

---

### 17. Voucher Repository — Payout Action Strings in SQL

**File:** `internal/adapters/database/postgres/voucher/voucher_repository.go` (lines ~1956–1975)

SQL CASE expression inside the voucher activity timeline query maps payout action strings
to display text:
```sql
WHEN psc.action IN ('payout_initiated', 'payout_processing') THEN 'Payment Processing'
WHEN psc.action = 'payout_completed' THEN 'Payment Completed to Payee'
WHEN psc.action = 'payout_failed' THEN 'Payout Failed'
WHEN psc.action = 'payout_timeout' THEN 'Payout Under Investigation'
```

**Mend:** Remove these WHEN arms. Historical rows in `payment_state_changes` will no
longer appear in new queries once the filter is gone, or display gracefully via the
`default` branch.

---

### 18. Routes — Castler Webhook and Ops Payout Endpoints

**File:** `cmd/api/routes.go` (lines ~292–307)

`registerV1PayoutRoutes` sets up:
- `POST /v1/payout/webhooks/castler` → `HandleCastlerWebhook`

**Mend:** Remove the entire `registerV1PayoutRoutes` function and its call in
`registerV1Routes`. Remove the `payoutHandler` and `payoutRepo` imports.

**File:** `cmd/ops/routes.go` (lines ~182–209)

`registerOpsPayoutRoutes` sets up the ops payout dashboard: listing, detail, and
`initiate_payout` endpoints.

**Mend:** Remove the function and its call in `registerOpsV1Routes`. Remove the
`payoutSvc`, `payoutHandler`, `payoutRepo`, and `factory` imports from ops routes.

---

### 19. Actor Redaction — Castler Webhook Actor

**File:** `internal/handlers/http/common/actor_redaction.go` (line ~27)

```go
"castler_webhook": true,
```

This entry hides Castler actor names from user-visible activity feeds.

**Mend:** Remove the `castler_webhook` entry.

---

### 20. Configuration — Castler and Payout Settings

**File:** `internal/shared/config/config.go`

| Item | Lines | Action |
|---|---|---|
| `CastlerConfig` struct | ~121–131 | Remove if Castler is no longer used for anything (penny drop included). If penny drop stays, keep but narrow the struct to only the fields needed for bank verification (remove `EscrowID`, `WebhookSecret`). |
| `cfg.Castler` field on main config | ~29 | Keep or remove per above. |
| `PayoutConfig` struct and `NewPayoutConfig()` | ~134–151 | Remove entirely. |
| `AutoPayoutWorkerConfig` struct | ~281–... | Remove entirely. |
| `cfg.Workers.AutoPayout` | ~254 | Remove the `AutoPayout` field. |
| Secrets loading for `castler_*` keys | ~493–510 | Remove if Castler config is fully removed, or narrow to the keys still needed. |
| Redaction of Castler secrets | ~609–613 | Remove. |

**File:** `.env.example` / `ops.env.example`

Remove all `CASTLER_*` environment variables and the `AUTO_PAYOUT_*` variables.

---

## What Changes in the Settlement Webhook (New Work)

After deprecating the payout system, the settlement webhook handler
(`internal/handlers/http/payment/webhook_handler.go`, `HandleSettlementStatusWebhook`)
needs to be enhanced for split settlement:

1. **Parse per-payee UTRs** from the gateway settlement payload. JioPay and Easebuzz will
   deliver individual settlement records, each with a UTR and an amount corresponding to a
   specific payee.

2. **Store per-voucher settlement data**. Options:
   - Add a `settlement_utr` column to `payment_vouchers` (or a new
     `voucher_settlement_details` table) and populate it from the webhook.
   - Reuse the existing `settlement_utr` on `payments` if one UTR covers the whole payment
     (only viable when there is a single payee per payment).

3. **Mark the "payee paid" state** via `payment_state_changes` using a new action type
   (e.g., `settlement_split_completed`) rather than the old Castler payout action types.

4. **Update the voucher payment details query** (`GetPaymentDetails`) to read payout
   completion status from the new column/table instead of `payout_transfer_requests`.

---

## Summary Table

| Layer | Action |
|---|---|
| `internal/adapters/external/payout/castler/castler_client.go` | **Delete** |
| `internal/adapters/external/payout/castler/castler_types.go` | **Delete** |
| `internal/adapters/external/payout/castler/mock_castler_client.go` | **Delete** |
| `internal/adapters/external/payout/castler/castler_bank_verification.go` | Keep (penny drop) |
| `internal/adapters/external/payout/factory/provider_factory.go` | **Delete** |
| `internal/core/payout/` (all files) | **Delete** |
| `internal/services/payout/payout_service.go` | **Delete** |
| `internal/services/payout/ops_payout_service.go` | **Delete** |
| `internal/handlers/http/payout/payout_handler.go` | **Delete** |
| `internal/handlers/http/payout/webhook_handler.go` | **Delete** |
| `internal/handlers/http/payout/ops_payout_handler.go` | **Delete** |
| `internal/workers/payout_worker.go` | **Delete** |
| `cmd/workers/payout_worker/main.go` | **Delete** |
| `internal/adapters/database/postgres/payout/` (all files) | **Delete** |
| `internal/services/payee/payee_service.go` | **Mend** — remove Castler onboarding calls |
| `internal/services/payee/factory.go` | **Mend** — remove payout provider wiring |
| `internal/core/payee/payee_entity.go` | **Mend** — remove `CastlerBeneficiaryID` |
| `internal/core/payee/payee_repository.go` | **Mend** — remove `GetCastlerBeneficiaryID` |
| `internal/adapters/database/postgres/payee/payee_bank_account_repository.go` | **Mend** — remove `castler_beneficiary_id` from all queries |
| `internal/core/payment/payment_state_change.go` | **Mend** — remove `PayoutAction*` constants |
| `internal/services/payment/state_change_helper.go` | **Mend** — remove payout log helpers |
| `internal/core/payment/payment_repository.go` | **Mend** — remove `GetSettledPaymentsReadyForPayout` |
| `internal/adapters/database/postgres/payment/payment_repository.go` | **Mend** — remove implementation |
| `internal/adapters/database/postgres/voucher/voucher_repository.go` | **Mend** — remove payout JOIN and SQL CASE arms |
| `internal/core/voucher/voucher_approval_dto.go` | **Mend** — remove/rename payout fields |
| `internal/services/voucher/models.go` | **Mend** — remove/rename payout fields |
| `internal/services/voucher/voucher_service.go` | **Mend** — remove payout field mapping |
| `internal/handlers/http/payment/payment_handler.go` | **Mend** — remove payout action filter |
| `internal/handlers/http/common/actor_redaction.go` | **Mend** — remove `castler_webhook` entry |
| `cmd/api/routes.go` | **Mend** — remove `registerV1PayoutRoutes` |
| `cmd/ops/routes.go` | **Mend** — remove `registerOpsPayoutRoutes` |
| `internal/shared/config/config.go` | **Mend** — remove `PayoutConfig`, `AutoPayoutWorkerConfig`, narrow `CastlerConfig` |
| `.env.example` / `ops.env.example` | **Mend** — remove Castler payout + auto-payout vars |
| DB migrations | **New** — drop `payout_transfer_requests`, `castler_beneficiary_id` column |
| Settlement webhook handler | **Enhance** — parse per-payee UTRs, store against vouchers |
