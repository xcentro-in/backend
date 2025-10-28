# Approval Workflow Implementation Summary

## Overview

Complete implementation of voucher approval workflow with two API endpoints for frontend integration.

## API Endpoints Implemented

### 1. Get Approval Inbox Counts
**Route**: `GET /api/v1/vouchers/approval-inbox/counts`

**Purpose**: Quick overview for tab badges/counts in frontend

**Response**:
```json
{
  "pending": 4,
  "approved": 3,
  "rejected": 1,
  "request_changes": 2
}
```

**Query Strategy**:
- Single optimized SQL query using CTEs and COUNT FILTER
- Applies 7-day filter for approved status automatically
- Returns all counts in one database round-trip

**Performance**: O(1) database call, very efficient for UI tab badges

---

### 2. Get Approval Inbox (Filtered)
**Route**: `GET /api/v1/vouchers/approval-inbox?status={status}`

**Purpose**: Fetch detailed voucher list for selected tab

**Parameters**:
- `status` (required): `pending` | `approved` | `rejected` | `request_changes`

**Response**:
```json
{
  "vouchers": [
    {
      "voucher_id": "...",
      "voucher_number": "PV-2024-0003",
      "invoice_number": "INV-BUILD-003",
      "invoice_due_date": "2024-11-15T00:00:00Z",
      "payee_name": "BuildRight - Construction Contractor",
      "total_amount": 100000.00,
      "voucher_status": "finalized",
      "current_approval_status": "pending",
      "approval_history": [
        {
          "approval_id": "...",
          "requester_user_id": "...",
          "requester_name": "Shubhankar Bhadra",
          "approver_user_id": "...",
          "approver_name": "Darahas Koppurappu",
          "action": "pending",
          "remarks": "Please review and approve this construction payment",
          "created_at": "2024-10-26T00:00:00Z"
        }
      ]
    }
  ],
  "total_count": 4
}
```

**Features**:
- Returns complete approval history for each voucher (audit trail)
- Resolves user names (requester + approver) from user_table
- Chronologically sorted history (oldest to newest)
- 7-day filter for approved status
- No pagination (returns all matching vouchers)

---

### 3. Create/Update Approval
**Route**: `POST /api/v1/vouchers/:voucher_id/request-approval`

**Purpose**: Submit approval action for a voucher

**Request**:
```json
{
  "approver_user_id": "uuid",
  "action": "pending|approved|rejected|request_changes",
  "remarks": "Optional comment"
}
```

**Response**:
```json
{
  "success": true,
  "message": "Approval request has been submitted",
  "approval_id": "...",
  "voucher_id": "...",
  "action": "pending",
  "created_at": "2024-10-28T00:00:00Z"
}
```

**Behavior**:
- Always creates NEW row (INSERT-only for audit trail)
- Never updates existing approval records
- Each action creates separate entry for complete history

---

## Database Design

### Table: `voucher_approvals`

**Columns**:
- `approval_id` (UUID, PK) - Unique identifier
- `voucher_id` (UUID, FK) - Links to payment_vouchers
- `requester_user_id` (UUID, FK) - Who initiated the action
- `approver_user_id` (UUID, FK) - Who should approve
- `action` (ENUM) - pending, approved, rejected, request_changes
- `remarks` (TEXT) - Optional comments
- `created_at` (TIMESTAMPTZ) - When action was taken

**Key Design Decisions**:
1. **Audit Trail Pattern**: Every action = new row (INSERT-only)
2. **No Updates**: Historical records never modified
3. **Latest Status**: Determined by `ORDER BY created_at DESC LIMIT 1`
4. **No Unique Constraint**: Allows multiple entries per voucher

---

## SQL Query Optimization

### Counts Query (Single Pass)
```sql
WITH latest_approvals AS (
    SELECT DISTINCT ON (voucher_id)
        voucher_id, action, created_at
    FROM voucher_approvals
    ORDER BY voucher_id, created_at DESC
),
voucher_approvals_with_status AS (
    SELECT v.voucher_id, la.action, la.created_at
    FROM payment_vouchers v
    INNER JOIN latest_approvals la ON v.voucher_id = la.voucher_id
    WHERE v.business_id = $1
)
SELECT
    COUNT(*) FILTER (WHERE action = 'pending') as pending,
    COUNT(*) FILTER (WHERE action = 'approved'
        AND created_at >= CURRENT_DATE - INTERVAL '6 days') as approved,
    COUNT(*) FILTER (WHERE action = 'rejected') as rejected,
    COUNT(*) FILTER (WHERE action = 'request_changes') as request_changes
FROM voucher_approvals_with_status
```

**Efficiency**: All counts in single query using PostgreSQL's COUNT FILTER

### Inbox Query (With History)
```sql
WITH latest_approvals AS (
    -- Get latest status per voucher
),
filtered_vouchers AS (
    -- Filter by status + 7-day rule for approved
)
SELECT v.*, va.*, requester.name, approver.name
FROM filtered_vouchers fv
JOIN payment_vouchers v ON fv.voucher_id = v.voucher_id
JOIN voucher_approvals va ON v.voucher_id = va.voucher_id
LEFT JOIN user_table requester ON va.requester_user_id = requester.id
LEFT JOIN user_table approver ON va.approver_user_id = approver.id
ORDER BY v.created_at DESC, va.created_at ASC
```

**Features**:
- Fetches ALL approval actions (not just latest)
- Joins user_table for name resolution
- Groups by voucher in Go code

---

## Testing

### Seed Data
**File**: `005_seed_approval_workflow.sql`

**Contains**:
- 14 test vouchers
- 21 approval records
- Various scenarios (pending, approved within/beyond 7 days, rejected, request_changes)
- 1 voucher with complete lifecycle history

**Apply**:
```bash
cd internal/adapters/database/postgres/migrations/seeds/
./apply-seeds.sh apply 005
```

**Rollback**:
```bash
./apply-seeds.sh rollback 005
```

### Expected Results

| Status | Count | Voucher Numbers |
|--------|-------|-----------------|
| pending | 4 | PV-2024-0003, 0004, 0005, 0014 |
| approved | 3 | PV-2024-0009, 0010, 0011 |
| rejected | 1 | PV-2024-0006 |
| request_changes | 2 | PV-2024-0007, 0008 |

**Edge Cases Tested**:
- Approved exactly 6 days ago ✓ APPEARS
- Approved 8 days ago ✗ DOES NOT APPEAR
- Multiple history entries (voucher 0014)
- No approval entries (vouchers 0001, 0002)

---

## Files Modified/Created

### Core Domain
- `internal/core/voucher/voucher_approval_dto.go` - Added `VoucherApprovalInboxCounts`
- `internal/core/voucher/voucher_repository.go` - Added `GetApprovalInboxCounts()` interface

### Repository
- `internal/adapters/database/postgres/voucher/voucher_repository.go`
  - `GetApprovalInboxVouchers()` - Fetch filtered inbox
  - `GetApprovalInboxCounts()` - Get counts for all statuses

### Service
- `internal/services/voucher/voucher_service.go`
  - `GetApprovalInbox()` - Business logic for inbox
  - `GetApprovalInboxCounts()` - Business logic for counts

### Handlers
- `internal/handlers/http/voucher/voucher_handler.go`
  - `GetApprovalInbox()` - HTTP handler for inbox
  - `GetApprovalInboxCounts()` - HTTP handler for counts

### Routes
- `internal/handlers/http/voucher/routes.go`
  - `GET /approval-inbox/counts`
  - `GET /approval-inbox?status={status}`

### Database
- `migrations/sql/000025_fix_voucher_approvals_table.up.sql` - Fixed table structure
- `migrations/sql/000026_remove_action_timestamp_from_approvals.up.sql` - Simplified design

### Test Data
- `migrations/seeds/005_seed_approval_workflow.sql` - Test vouchers
- `migrations/seeds/005_seed_approval_workflow_rollback.sql` - Cleanup
- `migrations/seeds/005_TESTING_GUIDE.md` - Testing instructions

---

## Frontend Integration

### Typical Flow

1. **Load Tab Counts** (on page load):
   ```javascript
   GET /api/v1/vouchers/approval-inbox/counts
   // Response: { pending: 4, approved: 3, rejected: 1, request_changes: 2 }
   // Use for tab badges: Pending (4), Approved (3), etc.
   ```

2. **Switch Tab** (user clicks tab):
   ```javascript
   GET /api/v1/vouchers/approval-inbox?status=pending
   // Response: { vouchers: [...], total_count: 4 }
   // Display voucher list for selected tab
   ```

3. **Expand Voucher** (view history):
   ```javascript
   // Use approval_history array from response
   // Display timeline of all approval actions
   ```

4. **Take Action** (approve/reject/request changes):
   ```javascript
   POST /api/v1/vouchers/:voucher_id/request-approval
   // Then refresh counts and list
   ```

### Performance Considerations

- **Counts endpoint**: Very fast (single query), call frequently
- **Inbox endpoint**: Heavier (joins + history), call on demand
- **No polling needed**: Counts can be refreshed on user action
- **Caching**: Consider client-side cache invalidation after actions

---

## Business Rules

1. **7-Day Approved Filter**: Approved vouchers only visible for 7 days
2. **Audit Trail**: All approval actions permanently recorded
3. **Latest Status**: Most recent action determines current state
4. **User Resolution**: Names fetched from user_table for display
5. **No Deletions**: Approval records never deleted (compliance)

---

## Next Steps (Optional Enhancements)

1. **Pagination**: Add to inbox endpoint for large datasets
2. **Sorting**: Allow custom sort (by date, amount, etc.)
3. **Filtering**: Add filters (date range, amount range, payee)
4. **Notifications**: Push notifications on approval actions
5. **Bulk Actions**: Approve/reject multiple vouchers
6. **Role-based**: Filter by approver (show only my approvals)
7. **Search**: Full-text search on invoice numbers, payee names
8. **Export**: Download approval history as CSV/PDF

---

## Documentation Updated

- `CLAUDE.md` - Added seed data section with location and usage
