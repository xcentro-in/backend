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

**Description:** Performs KYB verification action (reject or verify business).

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `bid` | string | Yes | Business ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `action` | string | Yes | One of: `reject`, `verify` | Action to perform |

**Action Effects:**

**`reject`:**
- Sets KYB status to `kyc_rejected`
- Sets owner role to NULL
- Clears all KYB verification data

**`verify`:**
- Sets KYB status to `kyc_verified`
- Sets owner's KYC status to `kyc_verified`

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

**Description:** Performs KYC verification action on user.

**Path Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `user_id` | string | Yes | User ID |

**Query Parameters:**
| Parameter | Type | Required | Validation | Description |
|-----------|------|----------|------------|-------------|
| `action` | string | Yes | Currently only: `reject` | Action to perform |

**Action Effects:**

**`reject`:**
- Sets KYC status to `kyc_rejected`
- Resets all business relations for the user
- Clears identity verification data

**Response:** `200 OK`
```json
{
  "message": "KYC rejected successfully"
}
```

**Error Responses:**
- `404 Not Found` - User not found

---

## Notification Operations

### 1. Trigger Bulk Notifications

**Endpoint:** `POST /ops/notification/trigger`

**Description:** Triggers notifications for multiple businesses with flexible targeting (unicast, multicast, broadcast).

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

**Target Types:**

**Unicast:**
- Must have exactly one user ID in `ids` array
- Direct notification to a specific user

**Multicast:**
- Either `ids` array OR `minimum_role` (not both)
- Multiple users in a business

**Broadcast:**
- Must have `minimum_role`
- All users with minimum role or higher in the business

**Validation Rules:**
- `targets` array must have at least 1 item
- Unicast: exactly 1 user ID required
- Multicast: either IDs or MinimumRole, not both
- Broadcast: MinimumRole is required

**Response:** `200 OK`
```json
{
  "success": true,
  "triggered_count": 42
}
```

**Error Responses:**
- `400 Bad Request` - Validation failed (includes field-specific errors)

**Example - Notify All Admins:**
```json
{
  "targets": [
    {
      "business_id": "biz-123",
      "type": "multicast",
      "minimum_role": "admin"
    }
  ],
  "event_type": "payment_processed",
  "message": {
    "title": "Payment Processed",
    "body": "Your payment of ₹10,000 has been processed successfully."
  },
  "actions": [
    {
      "type": "button-positive",
      "text": "View Details",
      "href": "/payments/pay-123"
    }
  ]
}
```

**Example - Notify Specific Users:**
```json
{
  "targets": [
    {
      "business_id": "biz-123",
      "type": "multicast",
      "ids": ["user-1", "user-2", "user-3"]
    }
  ],
  "event_type": "vendor_approved",
  "message": {
    "title": "Vendor Approved",
    "body": "ABC Corp has been approved as a vendor."
  }
}
```

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
