# Approval Workflow - Testing Guide

## Seed Data Applied ✓

Total vouchers created: **14 vouchers**

## Test Scenarios

### Expected Results for GET `/api/v1/vouchers/approval-inbox?status=X`

| Status Query | Expected Count | Voucher Numbers | Notes |
|--------------|----------------|-----------------|-------|
| `status=pending` | 4 | PV-2024-0003, 0004, 0005, 0014 | Voucher 0014 has full history |
| `status=rejected` | 1 | PV-2024-0006 | Has history: pending → rejected |
| `status=request_changes` | 2 | PV-2024-0007, 0008 | One old (6 days), one recent (5 hrs) |
| `status=approved` | 3 | PV-2024-0009, 0010, 0011 | Only within 7 days |

### Vouchers NOT in Any Inbox

| Voucher Number | Reason |
|----------------|--------|
| PV-2024-0001 | No approval entry |
| PV-2024-0002 | No approval entry |
| PV-2024-0012 | Approved 8 days ago (exceeds 7-day limit) |
| PV-2024-0013 | Approved 15 days ago (exceeds 7-day limit) |

## Edge Cases to Verify

### 1. 7-Day Boundary Test (Approved Status)
- **PV-2024-0009**: Approved TODAY (30 mins ago) ✓ SHOULD APPEAR
- **PV-2024-0010**: Approved 3 days ago ✓ SHOULD APPEAR
- **PV-2024-0011**: Approved 6 days ago ✓ SHOULD APPEAR (edge case - exactly within 7 days)
- **PV-2024-0012**: Approved 8 days ago ✗ SHOULD NOT APPEAR
- **PV-2024-0013**: Approved 15 days ago ✗ SHOULD NOT APPEAR

### 2. Approval History Test (Multiple Entries)
**PV-2024-0014** has complete approval lifecycle:
```
1. pending (7 days ago) - Initial submission
2. request_changes (5 days ago) - Approver requested GST update
3. pending (2 days ago) - Resubmitted with updates
```

Current status: **pending** (most recent)

**Verify**:
- Response shows `current_approval_status: "pending"`
- Response includes ALL 3 history entries in chronological order
- Each entry has requester and approver names resolved

### 3. Paid Voucher Test
**PV-2024-0009**:
- Has successful payment (`01eeeeee-0909-7000-0000-000000000009`)
- Status: `finalized`
- Approval: `approved` (today)

**Verify**: Paid vouchers still appear in approved inbox if within 7 days

### 4. Rejected with History
**PV-2024-0006**:
- First: `pending` (4 days ago)
- Latest: `rejected` (3 days ago) with reason: "Invoice amount does not match PO"

**Verify**: Shows as rejected with full history

## API Endpoints

### 1. Get Approval Inbox Counts

**Endpoint**: `GET /api/v1/vouchers/approval-inbox/counts`

**Expected Response**:
```json
{
  "pending": 4,
  "approved": 3,
  "rejected": 1,
  "request_changes": 2
}
```

**Note**: The `pending` count includes voucher 0014 which has a complex history but latest status is pending.

### 2. Get Approval Inbox (Filtered)

**Endpoint**: `GET /api/v1/vouchers/approval-inbox?status={status}`

See table above for expected counts per status.

## Sample cURL Commands

```bash
# Business ID (from seed data)
BUSINESS_ID="0199f7c6-39ab-77a1-b611-385a4cb8f24b"

# Get JWT token first (replace with your auth endpoint)
TOKEN="your_jwt_token"

# Test 0: Get counts (overview)
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox/counts" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 1: Pending approvals
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox?status=pending" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 2: Approved (7-day filter)
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox?status=approved" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 3: Request changes
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox?status=request_changes" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 4: Rejected
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox?status=rejected" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 5: Invalid status (should return error)
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox?status=invalid" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"

# Test 6: Missing status (should return error)
curl -X GET "http://localhost:8888/api/v1/vouchers/approval-inbox" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Business-ID: $BUSINESS_ID"
```

## Cleanup

To remove all test data:

```bash
cd internal/adapters/database/postgres/migrations/seeds/
./apply-seeds.sh rollback 005
```

## User Details (For Reference)

- **Requester**: Shubhankar Bhadra (`0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6`)
- **Approver**: Darahas Koppurappu (`0199f7d1-7514-7dc7-bdfa-075975b8ac2a`)

All approval records use these two users, so names should be properly resolved in the response.
