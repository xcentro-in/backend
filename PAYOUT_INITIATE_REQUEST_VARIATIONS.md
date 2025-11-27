# POST /ops/v1/payouts/initiate_payout - Request Body Variations

## Endpoint
```
POST /ops/v1/payouts/initiate_payout
Content-Type: application/json
```

## Request Structure

```go
{
  "payment_id": string,      // REQUIRED - UUID of the payment
  "voucher_ids": []string,   // OPTIONAL - Array of voucher UUIDs (default: [])
  "initiated_by": string,    // REQUIRED - UUID of the user initiating payout
  "bypass_checks": boolean,  // OPTIONAL - Skip settlement check (default: false)
  "remarks": string          // OPTIONAL - Additional notes (default: "")
}
```

---

## Field Behaviors

### 1. `payment_id` (REQUIRED)
- **Type**: `string` (UUID format)
- **Validation**: Must be valid UUID
- **Behavior**: System fetches all vouchers linked to this payment
- **Example**: `"01936d89-1234-7abc-9def-0123456789ab"`

### 2. `voucher_ids` (OPTIONAL)
- **Type**: `[]string` (array of UUID strings)
- **Default**: Empty array `[]`
- **Behavior**:
  - **If empty/omitted**: Processes ALL unpaid vouchers for the payment
  - **If provided**: Processes ONLY the specified voucher IDs
- **Idempotency**:
  - When empty: Skips vouchers that already have transfers
  - When specified: Returns error if any voucher already has a transfer

### 3. `initiated_by` (REQUIRED)
- **Type**: `string` (UUID format)
- **Validation**: Must be valid UUID
- **Purpose**: Audit trail - tracks who initiated the payout
- **Example**: `"01936d89-5678-7abc-9def-fedcba987654"`

### 4. `bypass_checks` (OPTIONAL)
- **Type**: `boolean`
- **Default**: `false`
- **Behavior**:
  - **`false` (default)**: Requires `payment.settlement_status = 'settled'`
  - **`true`**: Bypasses settlement check (emergency override)
- **Use Case**: Set to `true` only for emergency payouts when settlement hasn't completed

### 5. `remarks` (OPTIONAL)
- **Type**: `string`
- **Default**: `""` (empty string)
- **Purpose**: Additional notes for audit/tracking
- **Note**: Currently logged but not stored in database

---

## All Request Body Variations

### Variation 1: Minimal Request (Most Common)
**Use Case**: Process all unpaid vouchers for a settled payment

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Behavior**:
- ✅ Requires settlement_status = 'settled'
- ✅ Processes ALL unpaid vouchers for the payment
- ✅ Skips vouchers that already have transfers (idempotent)
- ✅ No remarks

---

### Variation 2: With Empty Arrays (Same as Variation 1)
**Use Case**: Explicitly specify empty voucher_ids

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Behavior**: Identical to Variation 1

---

### Variation 3: Specific Vouchers Only
**Use Case**: Pay out only selected vouchers (partial payout)

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [
    "01936d8a-1111-7abc-9def-111111111111",
    "01936d8a-2222-7abc-9def-222222222222"
  ],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Behavior**:
- ✅ Requires settlement_status = 'settled'
- ✅ Processes ONLY the 2 specified vouchers
- ❌ Returns error if any voucher already has a transfer
- ✅ Returns error if voucher IDs don't belong to the payment

---

### Variation 4: Single Voucher
**Use Case**: Pay out just one specific voucher

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [
    "01936d8a-1111-7abc-9def-111111111111"
  ],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Behavior**: Same as Variation 3, but for single voucher

---

### Variation 5: Bypass Settlement Check (Emergency)
**Use Case**: Emergency payout before settlement completes

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "bypass_checks": true
}
```

**Behavior**:
- ⚠️ **BYPASSES settlement_status check** (can pay out unsettled payments)
- ✅ Processes all unpaid vouchers
- 🚨 Use with caution - only for emergency scenarios

---

### Variation 6: Bypass with Specific Vouchers
**Use Case**: Emergency payout for specific vouchers only

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [
    "01936d8a-1111-7abc-9def-111111111111"
  ],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "bypass_checks": true
}
```

**Behavior**:
- ⚠️ Bypasses settlement check
- ✅ Processes only the specified voucher
- 🚨 Emergency use only

---

### Variation 7: With Remarks (Audit Trail)
**Use Case**: Document reason for payout

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "remarks": "Regular monthly vendor payment batch"
}
```

**Behavior**: Same as Variation 1, but with audit remarks

---

### Variation 8: Bypass with Remarks (Emergency Documentation)
**Use Case**: Emergency payout with reason documented

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "bypass_checks": true,
  "remarks": "EMERGENCY: Vendor critical payment - settlement delayed due to gateway issue"
}
```

**Behavior**:
- ⚠️ Bypasses settlement check
- ✅ Documents emergency reason
- 🚨 Emergency use only

---

### Variation 9: Complete Request (All Fields)
**Use Case**: Maximum control with full documentation

```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [
    "01936d8a-1111-7abc-9def-111111111111",
    "01936d8a-2222-7abc-9def-222222222222",
    "01936d8a-3333-7abc-9def-333333333333"
  ],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "bypass_checks": false,
  "remarks": "Q4 2024 vendor payments - batch 1 of 3"
}
```

**Behavior**:
- ✅ Requires settlement (bypass_checks = false explicitly)
- ✅ Processes 3 specific vouchers
- ✅ Full audit trail with remarks

---

## Default Value Behavior Matrix

| Field | Omitted | Empty Value | Explicit Value |
|-------|---------|-------------|----------------|
| `payment_id` | ❌ Error | ❌ Error | ✅ Used |
| `voucher_ids` | ✅ All unpaid vouchers | ✅ All unpaid vouchers | ✅ Only specified |
| `initiated_by` | ❌ Error | ❌ Error | ✅ Used |
| `bypass_checks` | ✅ `false` (check required) | ✅ `false` | ✅ Used |
| `remarks` | ✅ `""` (empty) | ✅ `""` | ✅ Logged |

---

## JSON Field Handling in Go

### How Go `json` binding handles omitted fields:

```go
var req struct {
    PaymentID    string   `json:"payment_id" validate:"required"`
    VoucherIDs   []string `json:"voucher_ids"`
    InitiatedBy  string   `json:"initiated_by" validate:"required"`
    BypassChecks bool     `json:"bypass_checks"`
    Remarks      string   `json:"remarks"`
}
```

**Omitted vs Explicit Values**:

1. **`voucher_ids` omitted**:
   ```json
   { "payment_id": "...", "initiated_by": "..." }
   ```
   - Go: `req.VoucherIDs = nil` (nil slice)
   - Check: `len(req.VoucherIDs) == 0` → **true**

2. **`voucher_ids` as empty array**:
   ```json
   { "payment_id": "...", "voucher_ids": [], "initiated_by": "..." }
   ```
   - Go: `req.VoucherIDs = []string{}` (empty slice)
   - Check: `len(req.VoucherIDs) == 0` → **true**

3. **`bypass_checks` omitted**:
   ```json
   { "payment_id": "...", "initiated_by": "..." }
   ```
   - Go: `req.BypassChecks = false` (zero value for bool)

4. **`bypass_checks` as false**:
   ```json
   { "payment_id": "...", "bypass_checks": false, "initiated_by": "..." }
   ```
   - Go: `req.BypassChecks = false`

**Result**: Both `nil` and `[]` for `voucher_ids` behave identically. Same for omitting `bypass_checks` vs setting to `false`.

---

## Service Layer Logic

### When `voucher_ids` is empty:

```go
if len(voucherIDs) == 0 {
    // Process ALL unpaid vouchers
    for _, v := range allVouchers {
        existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
        if err == nil && existing != nil {
            // Skip - already has transfer
            continue
        }
        targetVouchers = append(targetVouchers, v)
    }
}
```

**Behavior**: Idempotent - skips vouchers that already have transfers

---

### When `voucher_ids` has values:

```go
} else {
    // Process ONLY specified vouchers
    for _, v := range allVouchers {
        if voucherIDSet[v.VoucherID.String()] {
            existing, err := s.transferRepo.GetByVoucherID(ctx, v.VoucherID.String())
            if err == nil && existing != nil {
                return nil, fmt.Errorf("voucher %s already has transfer...", ...)
            }
            targetVouchers = append(targetVouchers, v)
        }
    }
}
```

**Behavior**: Returns error if ANY specified voucher already has a transfer

---

### When `bypass_checks` is false:

```go
if !bypassSettlementCheck && pmt.SettlementStatus != "settled" {
    return nil, fmt.Errorf(
        "payment not settled (current status: %s). Settlement must complete...",
        pmt.SettlementStatus,
    )
}
```

**Behavior**: Rejects payout if `payment.settlement_status != 'settled'`

---

### When `bypass_checks` is true:

```go
if bypassSettlementCheck {
    s.logger.WarnWith("⚠️ SETTLEMENT CHECK BYPASSED - Manual override by ops",
        "payment_id", paymentID,
        "settlement_status", pmt.SettlementStatus)
}
```

**Behavior**: Allows payout regardless of settlement status, logs warning

---

## Response Format

### Success Response (HTTP 201 Created):

```json
{
  "message": "Payout initiated - processing in background",
  "status": "success",
  "data": {
    "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
    "transfer_count": 3,
    "transfer_ids": [
      "01936d8b-aaaa-7abc-9def-transfer111111",
      "01936d8b-bbbb-7abc-9def-transfer222222",
      "01936d8b-cccc-7abc-9def-transfer333333"
    ],
    "total_amount": 150000,
    "currency": "INR",
    "status": "initiated",
    "next_action": "Castler API calls in progress - check status via transfer_ids"
  }
}
```

**Notes**:
- `transfer_count`: Number of transfer requests created
- `transfer_ids`: Array of UUIDs for created transfer requests
- `total_amount`: Sum of all transfer amounts (in paise)
- Status immediately returns "initiated" - actual Castler API calls happen in background

---

### Error Response - Settlement Not Complete (HTTP 500):

```json
{
  "error": "failed_to_initiate_payout",
  "message": "payment not settled (current status: pending). Settlement must complete before payout can be initiated. Use bypass_checks=true in request if you need to override this (ops discretion only)"
}
```

**When**: `bypass_checks = false` and `settlement_status != 'settled'`

---

### Error Response - Voucher Already Paid (HTTP 500):

```json
{
  "error": "failed_to_initiate_payout",
  "message": "voucher 01936d8a-1111-7abc-9def-111111111111 already has transfer (transfer_id: 01936d8b-xxxx-7abc-9def-transferxxxx, status: completed)"
}
```

**When**: Specific `voucher_ids` provided and one already has a transfer

---

### Error Response - No Vouchers to Process (HTTP 500):

```json
{
  "error": "failed_to_initiate_payout",
  "message": "no vouchers to process (all already have transfers)"
}
```

**When**: All vouchers for the payment already have transfer requests

---

### Error Response - Invalid Payment ID (HTTP 500):

```json
{
  "error": "failed_to_initiate_payout",
  "message": "invalid payment_id format: invalid UUID length: 10"
}
```

**When**: `payment_id` is not a valid UUID

---

### Error Response - Payment Not Found (HTTP 500):

```json
{
  "error": "failed_to_initiate_payout",
  "message": "payment not found: 01936d89-1234-7abc-9def-0123456789ab"
}
```

**When**: `payment_id` doesn't exist in database

---

## Common Scenarios

### Scenario 1: Regular Payout (Happy Path)
**Situation**: Payment is settled, all vouchers need payout

**Request**:
```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Process**:
1. ✅ Validates payment exists
2. ✅ Checks settlement_status = 'settled'
3. ✅ Fetches all vouchers for payment
4. ✅ Filters out vouchers with existing transfers
5. ✅ Creates transfer requests for remaining vouchers
6. ✅ Triggers async Castler API calls
7. ✅ Returns immediately with transfer IDs

---

### Scenario 2: Partial Payout
**Situation**: Want to pay only 2 out of 5 vouchers

**Request**:
```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": [
    "01936d8a-1111-7abc-9def-111111111111",
    "01936d8a-2222-7abc-9def-222222222222"
  ],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Process**:
1. ✅ Validates payment exists
2. ✅ Checks settlement_status = 'settled'
3. ✅ Fetches all vouchers for payment
4. ✅ Filters to only the 2 specified vouchers
5. ❌ Returns error if either voucher already has transfer
6. ✅ Creates transfer requests for the 2 vouchers
7. ✅ Returns with 2 transfer IDs

---

### Scenario 3: Emergency Payout (Settlement Delayed)
**Situation**: Settlement stuck but vendor needs urgent payment

**Request**:
```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654",
  "bypass_checks": true,
  "remarks": "EMERGENCY: Critical vendor - settlement delayed 48hrs"
}
```

**Process**:
1. ✅ Validates payment exists
2. ⚠️ **SKIPS** settlement_status check (bypass enabled)
3. ⚠️ Logs warning about bypass
4. ✅ Processes all unpaid vouchers
5. ✅ Creates transfer requests
6. ✅ Returns with transfer IDs

---

### Scenario 4: Retry Failed Idempotent Request
**Situation**: Request failed, retrying with same parameters

**Request** (same as before):
```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Process** (if some vouchers already processed):
1. ✅ Validates payment exists
2. ✅ Checks settlement_status = 'settled'
3. ✅ Fetches all vouchers for payment
4. ✅ Skips vouchers with existing transfers (idempotent)
5. ✅ Creates transfers only for remaining vouchers
6. ✅ Returns with new transfer IDs (or empty if all done)

**Note**: Idempotency only works when `voucher_ids` is empty/omitted

---

### Scenario 5: Retry with Specific Vouchers (Non-Idempotent)
**Situation**: Retrying with specific voucher_ids that already have transfers

**Request**:
```json
{
  "payment_id": "01936d89-1234-7abc-9def-0123456789ab",
  "voucher_ids": ["01936d8a-1111-7abc-9def-111111111111"],
  "initiated_by": "01936d89-5678-7abc-9def-fedcba987654"
}
```

**Process** (if voucher already has transfer):
1. ✅ Validates payment exists
2. ✅ Checks settlement_status = 'settled'
3. ✅ Fetches all vouchers for payment
4. ✅ Checks specified voucher
5. ❌ **RETURNS ERROR** - voucher already has transfer

**Note**: NOT idempotent when specific vouchers are provided

---

## Validation Summary

### Required Fields (Missing = HTTP 400 Bad Request):
- ✅ `payment_id` - Must be valid UUID
- ✅ `initiated_by` - Must be valid UUID

### Optional Fields (Can be omitted):
- ✅ `voucher_ids` - Defaults to `[]` (all unpaid vouchers)
- ✅ `bypass_checks` - Defaults to `false`
- ✅ `remarks` - Defaults to `""`

### Business Validation (Failure = HTTP 500):
- Payment must exist
- Payment must be settled (unless `bypass_checks = true`)
- At least one voucher must be available to process
- Specified voucher IDs must belong to the payment
- Specified voucher IDs must not already have transfers

---

## Quick Reference

### Most Common Variations:

| Use Case | Request Body |
|----------|--------------|
| **Regular payout (all vouchers)** | `{"payment_id": "...", "initiated_by": "..."}` |
| **Specific vouchers only** | `{"payment_id": "...", "voucher_ids": ["..."], "initiated_by": "..."}` |
| **Emergency (bypass settlement)** | `{"payment_id": "...", "initiated_by": "...", "bypass_checks": true}` |
| **With audit trail** | `{"payment_id": "...", "initiated_by": "...", "remarks": "..."}` |

### Field Defaults:

| Field | Default | Description |
|-------|---------|-------------|
| `voucher_ids` | `[]` | Empty = all unpaid vouchers |
| `bypass_checks` | `false` | Requires settlement check |
| `remarks` | `""` | No remarks |
