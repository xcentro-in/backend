# Batch Approval API Documentation

## Endpoint

**POST** `/api/v1/vouchers/batch-approve`

## Purpose

Approve multiple vouchers in a single operation. All approvals are processed in a single database transaction for atomicity.

## Authentication

- **Required**: JWT token in `Authorization` header
- **Required**: `X-Business-ID` header

## Request

### Headers
```
Authorization: Bearer {jwt_token}
X-Business-ID: {business_uuid}
Content-Type: application/json
```

### Body
```json
{
  "voucher_ids": [
    "01bbbbbb-0003-7000-0000-000000000003",
    "01bbbbbb-0004-7000-0000-000000000004",
    "01bbbbbb-0005-7000-0000-000000000005"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a",
  "remarks": "Batch approval for pending construction invoices"
}
```

### Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `voucher_ids` | array of UUID strings | Yes | List of voucher IDs to approve (min 1) |
| `approved_by_id` | UUID string | Yes | User ID who is approving (approver) |
| `remarks` | string | No | Optional comments for all approvals |

**Notes**:
- `requester_id` is automatically extracted from JWT token
- `business_id` is automatically extracted from header

## Response

### Success Response (All Approved)

**Status Code**: `200 OK`

```json
{
  "success": true,
  "message": "Successfully approved all 3 vouchers",
  "total_requested": 3,
  "total_approved": 3,
  "total_failed": 0,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000001"
    },
    {
      "voucher_id": "01bbbbbb-0004-7000-0000-000000000004",
      "voucher_number": "PV-2024-0004",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000002"
    },
    {
      "voucher_id": "01bbbbbb-0005-7000-0000-000000000005",
      "voucher_number": "PV-2024-0005",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000003"
    }
  ]
}
```

### Partial Success Response (Some Failed)

**Status Code**: `200 OK`

```json
{
  "success": true,
  "message": "Approved 2 out of 3 vouchers, 1 failed",
  "total_requested": 3,
  "total_approved": 2,
  "total_failed": 1,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000001"
    },
    {
      "voucher_id": "01bbbbbb-0004-7000-0000-000000000004",
      "voucher_number": "PV-2024-0004",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000002"
    },
    {
      "voucher_id": "01ffffff-0013-7000-0000-000000000013",
      "voucher_number": "PV-2024-0013",
      "success": false,
      "message": "Cannot approve voucher in status: posted"
    }
  ]
}
```

### Error Response (All Failed)

**Status Code**: `200 OK`

```json
{
  "success": false,
  "message": "Failed to approve all 2 vouchers",
  "total_requested": 2,
  "total_approved": 0,
  "total_failed": 2,
  "results": [
    {
      "voucher_id": "invalid-uuid",
      "voucher_number": "",
      "success": false,
      "message": "Invalid voucher ID: invalid UUID length: 12"
    },
    {
      "voucher_id": "01ffffff-9999-7000-0000-999999999999",
      "voucher_number": "",
      "success": false,
      "message": "Voucher not found: no rows in result set"
    }
  ]
}
```

### Validation Error Responses

**Status Code**: `400 Bad Request`

```json
{
  "error": "MISSING_BUSINESS_ID",
  "message": "X-Business-ID header is required"
}
```

```json
{
  "error": "MISSING_VOUCHER_IDS",
  "message": "At least one voucher_id is required"
}
```

```json
{
  "error": "INVALID_REQUEST",
  "message": "Invalid request body"
}
```

**Status Code**: `401 Unauthorized`

```json
{
  "error": "MISSING_USER_ID",
  "message": "User ID not found in token"
}
```

## Business Rules

1. **Transaction Atomicity**: All approvals processed in a single transaction
2. **Partial Success**: Transaction commits even if some vouchers fail (individual validations)
3. **Voucher Status**: Only vouchers in `draft` or `finalized` status can be approved
4. **Business Validation**: Each voucher must belong to the specified business
5. **Audit Trail**: Each approval creates a new row in `voucher_approvals` table
6. **Approval Action**: Always creates action = `approved`

## Failure Scenarios

Individual vouchers can fail for these reasons:

| Reason | Message Example |
|--------|-----------------|
| Invalid UUID format | `Invalid voucher ID: invalid UUID length: 12` |
| Voucher not found | `Voucher not found: no rows in result set` |
| Wrong business | `Voucher does not belong to this business` |
| Invalid status | `Cannot approve voucher in status: posted` |
| Database error | `Failed to create approval: {error}` |

## Sample cURL Command

```bash
curl -X POST "http://localhost:8888/api/v1/vouchers/batch-approve" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b" \
  -H "Content-Type: application/json" \
  -d '{
    "voucher_ids": [
      "01bbbbbb-0003-7000-0000-000000000003",
      "01bbbbbb-0004-7000-0000-000000000004",
      "01bbbbbb-0005-7000-0000-000000000005"
    ],
    "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a",
    "remarks": "Batch approval for construction payments"
  }'
```

## Testing with Seed Data

Using the test vouchers from `005_seed_approval_workflow.sql`:

### Scenario 1: Approve All Pending Vouchers

```bash
curl -X POST "http://localhost:8888/api/v1/vouchers/batch-approve" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b" \
  -H "Content-Type: application/json" \
  -d '{
    "voucher_ids": [
      "01bbbbbb-0003-7000-0000-000000000003",
      "01bbbbbb-0004-7000-0000-000000000004",
      "01bbbbbb-0005-7000-0000-000000000005"
    ],
    "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a"
  }'
```

**Expected**: All 3 vouchers approved successfully

### Scenario 2: Mixed - Some Valid, Some Invalid

```bash
curl -X POST "http://localhost:8888/api/v1/vouchers/batch-approve" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b" \
  -H "Content-Type: application/json" \
  -d '{
    "voucher_ids": [
      "01bbbbbb-0003-7000-0000-000000000003",
      "01ffffff-0013-7000-0000-000000000013",
      "invalid-uuid"
    ],
    "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a"
  }'
```

**Expected**:
- Voucher 0003: Success (pending → approved)
- Voucher 0013: Failed (status = posted, cannot approve)
- Invalid UUID: Failed (invalid format)

## Performance Considerations

- **Transaction**: All approvals in single transaction for consistency
- **Batch Size**: Recommended max 50 vouchers per request
- **Validation**: Each voucher validated individually before approval
- **Rollback**: No rollback on individual failures (partial success supported)

## Frontend Integration

```javascript
// Example: Approve selected vouchers
async function batchApproveVouchers(voucherIds, approverUserId, remarks = '') {
  const response = await fetch('/api/v1/vouchers/batch-approve', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${getJwtToken()}`,
      'X-Business-ID': getCurrentBusinessId(),
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      voucher_ids: voucherIds,
      approved_by_id: approverUserId,
      remarks: remarks
    })
  });

  const result = await response.json();

  if (result.success) {
    console.log(`Approved ${result.total_approved}/${result.total_requested} vouchers`);

    // Handle partial failures
    if (result.total_failed > 0) {
      const failed = result.results.filter(r => !r.success);
      console.error('Failed vouchers:', failed);
      // Show error UI for failed vouchers
    }
  }

  return result;
}
```

## Database Impact

Each successful approval creates:
- 1 new row in `voucher_approvals` table
- Approval ID: Auto-generated UUIDv7
- Action: `approved`
- Timestamp: Current time
- Audit trail: Complete history preserved

---

## Complete Examples

### Example 1: Complete Success (All 3 Vouchers Approved)

**Request**:
```bash
POST /api/v1/vouchers/batch-approve
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
Content-Type: application/json

{
  "voucher_ids": [
    "01bbbbbb-0003-7000-0000-000000000003",
    "01bbbbbb-0004-7000-0000-000000000004",
    "01bbbbbb-0005-7000-0000-000000000005"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a",
  "remarks": "Approved after verification"
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Successfully approved all 3 vouchers",
  "total_requested": 3,
  "total_approved": 3,
  "total_failed": 0,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000001"
    },
    {
      "voucher_id": "01bbbbbb-0004-7000-0000-000000000004",
      "voucher_number": "PV-2024-0004",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000002"
    },
    {
      "voucher_id": "01bbbbbb-0005-7000-0000-000000000005",
      "voucher_number": "PV-2024-0005",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000003"
    }
  ]
}
```

### Example 2: Partial Success (2 Approved, 1 Failed - Wrong Status)

**Request**:
```bash
POST /api/v1/vouchers/batch-approve
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
Content-Type: application/json

{
  "voucher_ids": [
    "01bbbbbb-0003-7000-0000-000000000003",
    "01bbbbbb-0004-7000-0000-000000000004",
    "01ffffff-0013-7000-0000-000000000013"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a",
  "remarks": "Q4 approvals"
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Approved 2 out of 3 vouchers, 1 failed",
  "total_requested": 3,
  "total_approved": 2,
  "total_failed": 1,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000001"
    },
    {
      "voucher_id": "01bbbbbb-0004-7000-0000-000000000004",
      "voucher_number": "PV-2024-0004",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000002"
    },
    {
      "voucher_id": "01ffffff-0013-7000-0000-000000000013",
      "voucher_number": "PV-2024-0013",
      "success": false,
      "message": "Cannot approve voucher in status: posted",
      "approval_id": ""
    }
  ]
}
```

**Note**: The 2 successful approvals are committed to the database. The failed one is simply skipped with an error message.

### Example 3: Partial Success (1 Approved, 2 Failed - Different Reasons)

**Request**:
```bash
POST /api/v1/vouchers/batch-approve
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
Content-Type: application/json

{
  "voucher_ids": [
    "01bbbbbb-0003-7000-0000-000000000003",
    "01ffffff-9999-7000-0000-999999999999",
    "invalid-uuid-format"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a"
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Approved 1 out of 3 vouchers, 2 failed",
  "total_requested": 3,
  "total_approved": 1,
  "total_failed": 2,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": true,
      "message": "Approved successfully",
      "approval_id": "01ffffff-aaaa-7000-0000-000000000001"
    },
    {
      "voucher_id": "01ffffff-9999-7000-0000-999999999999",
      "voucher_number": "",
      "success": false,
      "message": "Voucher not found: no rows in result set",
      "approval_id": ""
    },
    {
      "voucher_id": "invalid-uuid-format",
      "voucher_number": "",
      "success": false,
      "message": "Invalid voucher ID: invalid UUID length: 18",
      "approval_id": ""
    }
  ]
}
```

### Example 4: Complete Failure (All 3 Failed)

**Request**:
```bash
POST /api/v1/vouchers/batch-approve
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Business-ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
Content-Type: application/json

{
  "voucher_ids": [
    "01ffffff-9999-7000-0000-999999999999",
    "01ffffff-8888-7000-0000-888888888888",
    "invalid-uuid"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a"
}
```

**Response** (200 OK):
```json
{
  "success": false,
  "message": "Failed to approve all 3 vouchers",
  "total_requested": 3,
  "total_approved": 0,
  "total_failed": 3,
  "results": [
    {
      "voucher_id": "01ffffff-9999-7000-0000-999999999999",
      "voucher_number": "",
      "success": false,
      "message": "Voucher not found: no rows in result set",
      "approval_id": ""
    },
    {
      "voucher_id": "01ffffff-8888-7000-0000-888888888888",
      "voucher_number": "",
      "success": false,
      "message": "Voucher not found: no rows in result set",
      "approval_id": ""
    },
    {
      "voucher_id": "invalid-uuid",
      "voucher_number": "",
      "success": false,
      "message": "Invalid voucher ID: invalid UUID length: 12",
      "approval_id": ""
    }
  ]
}
```

### Example 5: Wrong Business ID (Security Check)

**Request**:
```bash
POST /api/v1/vouchers/batch-approve
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
X-Business-ID: 0199aaaa-1111-7777-b611-111111111111
Content-Type: application/json

{
  "voucher_ids": [
    "01bbbbbb-0003-7000-0000-000000000003"
  ],
  "approved_by_id": "0199f7d1-7514-7dc7-bdfa-075975b8ac2a"
}
```

**Response** (200 OK):
```json
{
  "success": false,
  "message": "Failed to approve all 1 vouchers",
  "total_requested": 1,
  "total_approved": 0,
  "total_failed": 1,
  "results": [
    {
      "voucher_id": "01bbbbbb-0003-7000-0000-000000000003",
      "voucher_number": "PV-2024-0003",
      "success": false,
      "message": "Voucher does not belong to this business",
      "approval_id": ""
    }
  ]
}
```

**Security Note**: Even though the voucher exists, it's rejected because it belongs to a different business.

---

## Response Field Reference

### Top-level Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | `true` if at least 1 voucher approved, `false` if all failed |
| `message` | string | Human-readable summary of the operation |
| `total_requested` | integer | Total number of vouchers in the request |
| `total_approved` | integer | Number of vouchers successfully approved |
| `total_failed` | integer | Number of vouchers that failed |
| `results` | array | Detailed results for each voucher (same order as request) |

### Result Item Fields

| Field | Type | Description |
|-------|------|-------------|
| `voucher_id` | string (UUID) | The voucher ID from request |
| `voucher_number` | string | Voucher number (e.g., "PV-2024-0003"), empty if not found |
| `success` | boolean | `true` if this voucher was approved, `false` if failed |
| `message` | string | Success message or error reason |
| `approval_id` | string (UUID) | Generated approval ID if successful, empty if failed |
