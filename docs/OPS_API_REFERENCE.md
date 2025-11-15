# Ops Dashboard API Reference

This document provides a complete reference for all operations dashboard (ops) API endpoints for managing users, businesses, and notifications.

**Base Path:** `/ops`

---

## Business Operations

### 1. List Businesses (Paginated)

**Endpoint:** `GET /ops/business`

**Description:** Retrieves a paginated list of all businesses, with optional KYB status filter.

**Query Parameters:**
| Parameter | Type | Required | Validation | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `page` | integer | No | >= 1 | 1 | Page number |
| `limit` | integer | No | >= 1, <= 100 | 20 | Items per page |
| `kyb_status` | string | No | One of: `kyc_not_started`, `kyc_in_progress`, `kyc_completed`, `kyc_verified`, `kyc_needs_update`, `kyc_rejected` | - | Filter by KYB status |

**Response:** `200 OK`
```json
{
  "data": [
    {
      "bid": "string",
      "name": "string",
      "gstin": "string",
      "kybStatus": "kyc_not_started | kyc_in_progress | kyc_completed | kyc_verified | kyc_needs_update | kyc_rejected",
      "businessType": "sole_proprietorship | partnership | ...",
      "updatedAt": "ISO8601 timestamp"
    }
  ],
  "pagination": {
    "currentPage": 1,
    "pageSize": 20,
    "totalItems": 100,
    "totalPages": 5
  }
}
```

**Notes:**
- When `kyb_status` filter is provided, only businesses with that status are returned
- Results are sorted by KYB status priority (completed > in_progress > verified > others), then by updated date

---

### 2. Search Businesses

**Endpoint:** `GET /ops/business/search`

**Description:** Searches businesses by name or GSTIN, with optional KYB status filter.

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `q` | string | Yes | min: 2 | Search term (matches name or GSTIN) |
| `kyb_status` | string | No | One of: `kyc_not_started`, `kyc_in_progress`, `kyc_completed`, `kyc_verified`, `kyc_needs_update`, `kyc_rejected` | Filter by KYB status |

**Response:** `200 OK`
```json
[
  {
    "bid": "string",
    "name": "string",
    "gstin": "string",
    "hasCompletedOnboarding": false,
    "hasAddedPaymentMethod": false,
    "hasAddedVendor": false,
    "hasCreatedFirstPayment": false
  }
]
```

**Notes:**
- Returns empty array `[]` if no matches found
- Limited to 10 results
- Search is case-insensitive prefix match
- Includes onboarding tracking flags to show business progress

---

### 3. Get Business Details

**Endpoint:** `GET /ops/business/:bid`

**Description:** Retrieves complete business details including KYB data and associated users.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | Yes | Business ID |

**Response:** `200 OK`
```json
{
  "bid": "string",
  "name": "string",
  "gstin": "string",
  "gstData": "JSON string",
  "itid": "string (PAN)",
  "kybStatus": "kyc_not_started | kyc_in_progress | kyc_completed | kyc_verified | kyc_needs_update | kyc_rejected",
  "businessType": "sole_proprietorship | partnership | ...",
  "businessModel": "string",
  "businessTurnover": "string",
  "countryCode": "string",
  "currencyCode": "string",
  "updatedAt": "ISO8601 timestamp",
  "hasCompletedOnboarding": false,
  "hasAddedPaymentMethod": false,
  "hasAddedVendor": false,
  "hasCreatedFirstPayment": false,
  "itidData": "JSON string (PAN verification data)",
  "cid": "string (CIN)",
  "cidData": "JSON string (CIN verification data)",
  "docUrl": "string",
  "users": [
    {
      "userId": "string",
      "email": "string",
      "name": "string",
      "mobile": "string",
      "role": "owner | admin | manager | auditor",
      "roleClaim": "owner | admin | manager | auditor",
      "kycStatus": "kyc_not_started | kyc_in_progress | kyc_completed | kyc_verified | kyc_needs_update | kyc_rejected",
      "kycNeeded": false,
      "taxId": "string",
      "nationalId": "string",
      "emailVerified": false,
      "mobileVerified": false,
      "addedAt": "ISO8601 timestamp",
      "updatedAt": "ISO8601 timestamp"
    }
  ]
}
```

**Error Responses:**
- `404 Not Found` - Business not found

---

### 4. Reload Business Verification Data

**Endpoint:** `POST /ops/business/:bid/reload-data`

**Description:** Refreshes verification data from external verification APIs (GSTIN, PAN, or CIN).

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | Yes | Business ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `type` | string | Yes | One of: `gstin`, `income_tax_id`, `company_id` | Type of data to reload |

**Response:** `200 OK`
```json
{
  "message": "Business data reloaded successfully"
}
```

**Error Responses:**
- `404 Not Found` - Business not found
- `500 Internal Server Error` - Verification API failure

---

### 5. KYB Action (Reject/Verify)

**Endpoint:** `POST /ops/business/:bid/kyb-action`

**Description:** Performs KYB verification action (reject or verify business). Optionally triggers notifications with notes.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | Yes | Business ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `action` | string | Yes | One of: `reject`, `verify` | Action to perform |

**Request Body (Optional):**
```json
{
  "notes": "string (max 500 chars, optional)"
}
```

**Action Effects:**

**`reject`:**
- Sets KYB status to `kyc_rejected`
- Sets owner role to NULL
- Clears all KYB verification data
- **If notes provided:** Sends notification to user who initiated KYB (kyb_initiated=true)

**`verify`:**
- Sets KYB status to `kyc_verified`
- Sets owner's KYC status to `kyc_verified`
- **If notes provided:** Sends notification to business owner

**Response:** `200 OK`
```json
{
  "message": "KYB rejected successfully"
}
// OR
{
  "message": "KYB verified successfully"
}
```

**Error Responses:**
- `404 Not Found` - Business not found

**Notification Behavior:**
- Notifications are only sent if `notes` field is present and not empty
- For `reject`: Notifies user with `kyb_initiated = true`
- For `verify`: Notifies the business owner
- Notification type: Unicast (single user)
- Respects user's notification preferences

---

### 6. Update User Role in Business

**Endpoint:** `POST /ops/business/:bid/user-action`

**Description:** Updates a user's role in a business. Used by ops to promote/demote users or set specific roles.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | Yes | Business ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `user_id` | string | Yes | - | User ID to update |
| `action` | string | Yes | One of: `set_owner`, `set_admin`, `set_manager`, `set_auditor` | Role action to perform |

**Action Details:**

| Action | Sets Role To | Description |
|--------|-------------|-------------|
| `set_owner` | `owner` | Promotes user to owner (highest privilege) |
| `set_admin` | `admin` | Sets user as admin |
| `set_manager` | `manager` | Sets user as manager |
| `set_auditor` | `auditor` | Sets user as auditor (read-only) |

**Response:** `200 OK`
```json
{
  "message": "User role updated successfully"
}
```

**Error Responses:**
- `404 Not Found` - User not associated with business (no business-user relation exists)
- `500 Internal Server Error` - Failed to update role

**Important Notes:**
- User must already be associated with the business (existing business-user relation)
- If user-business relation doesn't exist, returns 404 error
- Ops team should verify user association before promoting
- Cache invalidation is automatically triggered for user roles after update
- No additional business logic checks - ops has full control

**Example:**
```bash
# Promote user to owner
POST /ops/business/biz-123/user-action?user_id=user-456&action=set_owner

# Set user as admin
POST /ops/business/biz-123/user-action?user_id=user-456&action=set_admin
```

---

## User Operations

### 1. Search Users

**Endpoint:** `GET /ops/user/search`

**Description:** Searches users by name or email within specified businesses.

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `business_id` | string[] | Yes | min: 1, max: 20 | Business IDs to search within (can be repeated) |
| `q` | string | Yes | min: 2, max: 100 | Search term (matches name or email) |

**Example:**
```
GET /ops/user/search?business_id=biz-123&business_id=biz-456&q=john
```

**Response:** `200 OK`
```json
[
  {
    "id": "string",
    "name": "string",
    "business_id": "string"
  }
]
```

**Notes:**
- Returns empty array `[]` if no matches found
- Can search across multiple businesses simultaneously

---

### 2. Get User Details

**Endpoint:** `GET /ops/user/:user_id`

**Description:** Retrieves complete user details including identity data and business role (if bid provided).

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `user_id` | string | Yes | User ID |

**Query Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | No | Business ID (if provided, includes role data) |

**Response:** `200 OK`
```json
{
  "id": "string",
  "email": "string",
  "mobile": "string",
  "name": "string",
  "dob": "YYYY-MM-DD",
  "taxId": "string (PAN)",
  "kycStatus": "kyc_not_started | kyc_in_progress | kyc_completed | kyc_verified | kyc_needs_update | kyc_rejected",
  "kycNeeded": false,
  "emailVerified": false,
  "mobileVerified": false,
  "createdAt": "ISO8601 timestamp",
  "updatedAt": "ISO8601 timestamp",
  "taxIdData": "JSON string (PAN verification data)",
  "nationalId": "string (Aadhaar)",
  "nationalIdData": "JSON string (Aadhaar verification data)",
  "nTxnId": "string (DigiLocker transaction ID)",
  "role": "owner | admin | manager | auditor (only if bid provided)",
  "roleClaim": "owner | admin | manager | auditor (only if bid provided)"
}
```

**Error Responses:**
- `404 Not Found` - User not found

---

### 3. Reload User PAN Data

**Endpoint:** `POST /ops/user/:user_id/reload-pan`

**Description:** Refreshes PAN verification data from external verification APIs.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `user_id` | string | Yes | User ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `type` | string | Yes | One of: `pan_com`, `pan_aadhaar_link` | Type of PAN data to reload |

**Data Types:**
- `pan_com` - Refresh PAN company verification data
- `pan_aadhaar_link` - Refresh PAN-Aadhaar linkage status

**Response:** `200 OK`
```json
{
  "message": "User PAN data reloaded successfully"
}
```

**Error Responses:**
- `404 Not Found` - User not found
- `500 Internal Server Error` - Verification API failure

---

### 4. KYC Action (Reject)

**Endpoint:** `POST /ops/user/:user_id/kyc-action`

**Description:** Performs KYC verification action on user. Optionally includes notes that will trigger a notification to the user.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `user_id` | string | Yes | User ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `action` | string | Yes | Currently only: `reject` | Action to perform |

**Request Body:**
```json
{
  "notes": "string (optional, max 500 characters)"
}
```

**Body Fields:**
| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `notes` | string | No | Max 500 characters | Reason or notes for the action |

**Action Effects:**

**`reject`:**
- Sets KYC status to `kyc_rejected`
- Resets all business relations for the user
- Clears identity verification data
- **If notes provided:** Sends unicast notification to the user with rejection reason

**Notification Behavior:**
- **Target:** User being rejected (unicast)
- **Event Type:** `kyc_rejected`
- **Title:** "KYC Rejected"
- **Message:** "Your KYC verification has been rejected. Notes: {notes}"
- **Triggered:** Asynchronously if notes are provided

**Response:** `200 OK`
```json
{
  "message": "KYC rejected successfully"
}
```

**Error Responses:**
- `404 Not Found` - User not found
- `400 Bad Request` - Notes exceed 500 characters

---

## Notification Operations

### Overview

The notification system supports three delivery channels:
- **Web:** In-app notifications (always sent)
- **Email:** Email notifications (based on user preferences)
- **Push:** Push notifications (based on user preferences and device availability)

Notifications respect user preferences for channels and event categories. The system handles fan-out, delivery, and retry logic automatically.

---

### 1. Trigger Bulk Notifications

**Endpoint:** `POST /ops/notification/trigger`

**Description:** Triggers notifications for multiple businesses with flexible targeting (unicast, multicast, broadcast). This endpoint creates notification events that are asynchronously processed and delivered through configured channels based on user preferences.

**Request Body:**
```json
{
  "targets": [
    {
      "business_id": "string (required)",
      "type": "unicast | multicast | broadcast (required)",
      "minimum_role": "owner | admin | finance | support (optional, for multicast/broadcast)",
      "ids": ["user_id1", "user_id2"] // (optional, for unicast/multicast)
    }
  ],
  "event_type": "string (required)",
  "message": {
    "title": "string (required, max 100 chars)",
    "body": "string (required, max 500 chars)"
  },
  "actions": [
    {
      "type": "button-positive | button-negative | button-neutral | link (required)",
      "text": "string (required, max 30 chars)",
      "href": "string (required, relative URL or deep link)"
    }
  ]
}
```

**Request Fields:**

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `targets` | array | Yes | Min 1 item | Array of target specifications for notification recipients |
| `event_type` | string | Yes | Non-empty | Event type identifier (see Event Types below) |
| `message` | object | Yes | Valid MessageContent | Notification message with title and body |
| `actions` | array | No | Valid ActionButtons | Optional action buttons for the notification |

**Target Object Fields:**

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `business_id` | string | Yes | Non-empty | Business ID for scoping the notification |
| `type` | string | Yes | `unicast`, `multicast`, or `broadcast` | Targeting type |
| `minimum_role` | string | Conditional | `owner`, `admin`, `finance`, `support` | Required for broadcast, optional for multicast with role-based targeting |
| `ids` | array | Conditional | Non-empty user IDs | Required for unicast (exactly 1), optional for multicast (specific users) |

**Message Object Fields:**

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `title` | string | Yes | Max 100 chars | Short notification title (used in push notifications and email subjects) |
| `body` | string | Yes | Max 500 chars | Main notification message content |

**Action Button Fields:**

| Field | Type | Required | Validation | Description |
|-------|------|----------|------------|-------------|
| `type` | string | Yes | `button-positive`, `button-negative`, `button-neutral`, `link` | Button style/appearance |
| `text` | string | Yes | Max 30 chars | Button label text |
| `href` | string | Yes | Non-empty | Relative URL (e.g., `/payments/123`) or deep link |

---

### Target Types Explained

#### Unicast
Sends notification to a single specific user.

**Requirements:**
- `ids` array must contain exactly 1 user ID
- User must be associated with the specified business

**Use Cases:**
- User-specific actions (KYC rejected, role changed)
- Direct messages to individual users

**Example:**
```json
{
  "business_id": "biz-123",
  "type": "unicast",
  "ids": ["user-456"]
}
```

#### Multicast
Sends notification to multiple specific users OR users with a minimum role.

**Requirements (choose one):**
- **Option 1:** `ids` array with multiple user IDs (specific users)
- **Option 2:** `minimum_role` for role-based targeting (e.g., notify all admins)
- **Cannot use both** `ids` and `minimum_role` together

**Use Cases:**
- Notify specific team members about an event
- Notify all users with a certain role (e.g., all finance managers)

**Example (Specific Users):**
```json
{
  "business_id": "biz-123",
  "type": "multicast",
  "ids": ["user-1", "user-2", "user-3"]
}
```

**Example (Role-Based):**
```json
{
  "business_id": "biz-123",
  "type": "multicast",
  "minimum_role": "admin"
}
```

#### Broadcast
Sends notification to all users in a business with a minimum role or higher.

**Requirements:**
- `minimum_role` is required
- Targets all users in the business meeting the role requirement

**Role Hierarchy (highest to lowest):**
1. `owner` - Business owner
2. `admin` - Administrators
3. `finance` - Finance managers
4. `support` - Support staff

**Use Cases:**
- Company-wide announcements
- System maintenance notifications
- Important updates affecting all users

**Example:**
```json
{
  "business_id": "biz-123",
  "type": "broadcast",
  "minimum_role": "support"
}
```
*This notifies everyone: owner, admins, finance, and support staff*

---

### Event Types

Common event types follow the format: `category.action.details`

**Categories map to user preference categories:**

| Category | Event Types | Description |
|----------|-------------|-------------|
| `payments` | `payments.completed`, `payments.failed`, `payments.refunded`, `payments.settlement.received`, `payments.payout.completed` | Payment-related events |
| `payees` | `payees.verified`, `payees.verification.failed` | Payee/vendor management |
| `approvals` | `approvals.voucher.requested`, `approvals.voucher.approved`, `approvals.voucher.rejected` | Approval workflow events |
| `security` | `security.user.invited`, `security.user.role.changed` | User access and security |
| `insights` | `insights.report.ready` | Reports and analytics |
| `announcements` | `announcements.maintenance`, `announcements.alert` | System announcements |

**User Preference Filtering:**
- Users can disable notifications for entire categories
- Event category is extracted from event type (first segment before dot)
- If user has disabled a category, they won't receive notifications for that category
- Web notifications are always sent regardless of preferences (for important alerts)

---

### Action Button Types

Action buttons provide interactive elements in notifications.

| Type | Visual Style | Use Cases |
|------|--------------|-----------|
| `button-positive` | Green/primary button | Approve, Accept, Confirm, Proceed |
| `button-negative` | Red/destructive button | Reject, Deny, Cancel, Delete |
| `button-neutral` | Gray/secondary button | View Details, Review, Maybe Later |
| `link` | Text link style | Learn More, View Report, Documentation |

**Example with Multiple Actions:**
```json
{
  "actions": [
    {
      "type": "button-positive",
      "text": "Approve",
      "href": "/approvals/123/approve"
    },
    {
      "type": "button-negative",
      "text": "Reject",
      "href": "/approvals/123/reject"
    },
    {
      "type": "button-neutral",
      "text": "View Details",
      "href": "/approvals/123"
    }
  ]
}
```

---

### Response Format

**Success Response:** `200 OK`
```json
{
  "success": true,
  "triggered_count": 42
}
```

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | Whether the notification events were created successfully |
| `triggered_count` | integer | Total number of notification events created |

**Error Responses:**

**`400 Bad Request` - Validation Errors:**
```json
{
  "error": "Validation failed",
  "errors": {
    "targets[0].type": "must be one of: unicast, multicast, broadcast",
    "message.title": "max length is 100 characters"
  }
}
```

**`400 Bad Request` - Target Type Errors:**
```json
{
  "error": "Unicast target must have exactly one user ID",
  "index": "0"
}
```

**`500 Internal Server Error` - System Errors:**
```json
{
  "error": "Failed to trigger notifications"
}
```

---

### Complete Examples

#### Example 1: Payment Approval Required (Unicast)
```json
{
  "targets": [
    {
      "business_id": "biz-789",
      "type": "unicast",
      "ids": ["user-owner-123"]
    }
  ],
  "event_type": "payments.approval.required",
  "message": {
    "title": "Payment Approval Required",
    "body": "A payment of ₹50,000 to ABC Corp requires your approval."
  },
  "actions": [
    {
      "type": "button-positive",
      "text": "Approve",
      "href": "/payments/pay-456/approve"
    },
    {
      "type": "button-negative",
      "text": "Reject",
      "href": "/payments/pay-456/reject"
    },
    {
      "type": "link",
      "text": "View Details",
      "href": "/payments/pay-456"
    }
  ]
}
```

#### Example 2: System Maintenance (Broadcast)
```json
{
  "targets": [
    {
      "business_id": "biz-123",
      "type": "broadcast",
      "minimum_role": "support"
    },
    {
      "business_id": "biz-456",
      "type": "broadcast",
      "minimum_role": "support"
    }
  ],
  "event_type": "announcements.maintenance",
  "message": {
    "title": "Scheduled Maintenance",
    "body": "Our platform will undergo maintenance on Jan 15, 2025 from 2 AM to 4 AM IST. Some features may be temporarily unavailable."
  },
  "actions": [
    {
      "type": "link",
      "text": "Learn More",
      "href": "/announcements/maintenance-jan-2025"
    }
  ]
}
```

#### Example 3: Vendor Approved (Multicast to Finance Team)
```json
{
  "targets": [
    {
      "business_id": "biz-123",
      "type": "multicast",
      "minimum_role": "finance"
    }
  ],
  "event_type": "payees.verified",
  "message": {
    "title": "Vendor Approved",
    "body": "ABC Corp has been verified and approved as a vendor. You can now create payments to this vendor."
  },
  "actions": [
    {
      "type": "button-neutral",
      "text": "View Vendor",
      "href": "/vendors/vnd-789"
    }
  ]
}
```

#### Example 4: Report Ready (Multicast to Specific Users)
```json
{
  "targets": [
    {
      "business_id": "biz-123",
      "type": "multicast",
      "ids": ["user-1", "user-2"]
    }
  ],
  "event_type": "insights.report.ready",
  "message": {
    "title": "Monthly Report Ready",
    "body": "Your monthly payment analytics report for December 2024 is now available."
  },
  "actions": [
    {
      "type": "button-positive",
      "text": "View Report",
      "href": "/reports/monthly-dec-2024"
    }
  ]
}
```

---

### Best Practices

1. **Event Type Naming:**
   - Use consistent `category.action.details` format
   - Map to existing category preferences when possible
   - Use descriptive names (e.g., `payments.settlement.received` not `pmt_sett`)

2. **Message Content:**
   - Keep titles short and actionable (max 100 chars)
   - Provide context in body (max 500 chars)
   - Include relevant amounts, names, or IDs
   - Use clear, professional language

3. **Action Buttons:**
   - Limit to 2-3 buttons maximum for better UX
   - Use positive/negative types for approval workflows
   - Always provide a neutral "View Details" option
   - Use relative URLs for deep linking

4. **Target Selection:**
   - Use unicast for user-specific notifications
   - Use multicast with role for departmental notifications
   - Use broadcast sparingly (system-wide announcements only)
   - Consider user preferences and notification fatigue

5. **Testing:**
   - Test with different user preference settings
   - Verify role-based targeting works correctly
   - Check notification appearance on all channels (web, email, push)
   - Validate deep links work as expected

---

## Common Data Types

### KYB Status Values
- `kyc_not_started` - Business onboarding not initiated
- `kyc_in_progress` - Business onboarding in progress
- `kyc_completed` - Business onboarding completed (pending ops verification)
- `kyc_verified` - Business verified by ops team
- `kyc_needs_update` - Business details need update
- `kyc_rejected` - Business rejected by ops team

### KYC Status Values
- `kyc_not_started` - User KYC not initiated
- `kyc_in_progress` - User KYC in progress
- `kyc_completed` - User KYC completed (pending verification)
- `kyc_verified` - User KYC verified
- `kyc_needs_update` - User identity needs update
- `kyc_rejected` - User KYC rejected

### Business Types
- `sole_proprietorship`
- `partnership`
- `limited_liability_partnership`
- `private_limited_company`
- `one_person_company`
- `section_8_non_profit`
- `trust`
- `ngo`
- `society`
- `cooperative_society`
- `other`

### User Roles
- `owner` - Business owner (highest privilege)
- `admin` - Administrator
- `manager` - Manager
- `auditor` - Auditor (read-only)

---

## Error Handling

All endpoints return standard HTTP status codes:

**Success:**
- `200 OK` - Request successful
- `201 Created` - Resource created (not used in ops endpoints currently)

**Client Errors:**
- `400 Bad Request` - Invalid request parameters or validation failed
- `404 Not Found` - Resource not found

**Server Errors:**
- `500 Internal Server Error` - Server error or external API failure

**Error Response Format:**
```json
{
  "error": "Error message",
  "errors": {
    "field_name": "Validation error message"
  }
}
```

---

## Notes for Frontend Implementation

1. **Authentication:** All ops endpoints require authentication (JWT token in Authorization header)

2. **Rate Limiting:** Standard rate limiting applies to all endpoints

3. **Pagination:** Use `page` and `limit` query parameters for list endpoints

4. **Search:** Search endpoints return empty arrays when no results found (not 404)

5. **Validation:** Client-side validation should match server-side rules to provide better UX

6. **Timestamps:** All timestamps are in ISO8601 format

7. **JSON Data:** Fields like `gstData`, `itidData`, `cidData` contain stringified JSON objects that may need parsing

8. **Multiple Business IDs:** In user search, the `business_id` parameter can be repeated multiple times in the query string

9. **Optional Filters:** The `kyb_status` filter in business search is optional - omit it to search all statuses

10. **Notification Targets:** Multiple targets can be specified in a single notification trigger request for batch operations
