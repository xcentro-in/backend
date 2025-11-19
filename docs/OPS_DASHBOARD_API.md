# Ops Dashboard API Documentation

This document describes all API endpoints available for the ops dashboard with OAuth2/OIDC authentication.

## Base URL

```
http://localhost:8080/ops/v1
```

## Authentication

All endpoints except login/callback/refresh require JWT authentication via cookies.

### Cookies Used
- `access_token`: JWT access token (1 hour expiry)
- `refresh_token`: JWT refresh token (30 days expiry)
- `oauth_state`: Temporary state for CSRF protection during OAuth flow

### Dev Mode
Set `OPS_OAUTH_SKIP_VALIDATION=true` in `.env` to bypass authentication (dev/testing only).

---

## 1. Authentication Endpoints

### 1.1 Initiate Login

**Endpoint:** `GET /ops/v1/auth/login`
**Authentication:** None (Public)
**Description:** Initiates OAuth2 login flow with Microsoft Entra ID

**Response:**
```json
{
  "message": "Redirect to Office365 login",
  "data": {
    "authUrl": "https://login.microsoftonline.com/..."
  }
}
```

**Cookies Set:**
- `oauth_state` - CSRF protection token (10 min expiry)

**Frontend Action:**
Redirect user to the `authUrl` to complete authentication with Microsoft.

---

### 1.2 OAuth Callback

**Endpoint:** `GET /ops/v1/auth/callback?code=xxx&state=xxx`
**Authentication:** None (Public)
**Description:** Handles OAuth2 callback from Microsoft Entra ID

**Query Parameters:**
- `code` (string, required) - Authorization code from OAuth provider
- `state` (string, required) - CSRF protection state

**Success Response (200):**
```json
{
  "message": "Login successful",
  "data": {
    "accessToken": "eyJhbGci...",
    "refreshToken": "eyJhbGci..."
  }
}
```

**Cookies Set:**
- `access_token` - JWT access token
- `refresh_token` - JWT refresh token

**Error Responses:**
- `400` - Missing/invalid code or state
- `500` - Authentication failed

**Frontend Action:**
After redirect back from Microsoft, the callback will automatically set auth cookies. Redirect user to dashboard.

---

### 1.3 Refresh Access Token

**Endpoint:** `POST /ops/v1/auth/refresh`
**Authentication:** Requires `refresh_token` cookie
**Description:** Refreshes expired access token and fetches updated roles from Microsoft

**Success Response (200):**
```json
{
  "message": "Token refreshed successfully",
  "data": {
    "accessToken": "eyJhbGci..."
  }
}
```

**Cookies Updated:**
- `access_token` - New JWT access token with updated roles

**Error Responses:**
- `401` - Missing or invalid refresh token (requires re-login)

**Frontend Action:**
Call this endpoint when you receive a 401 with `error: "TOKEN_EXPIRED"`. If refresh fails, redirect to login.

---

### 1.4 Logout

**Endpoint:** `POST /ops/v1/auth/logout`
**Authentication:** Optional (works with or without JWT)
**Description:** Revokes session and clears authentication cookies

**Success Response (200):**
```json
{
  "message": "Logged out successfully",
  "data": null
}
```

**Cookies Cleared:**
- `access_token`
- `refresh_token`

---

### 1.5 Get User Profile

**Endpoint:** `GET /ops/v1/user/profile`
**Authentication:** Required (JWT)
**Description:** Retrieves authenticated ops user's profile

**Success Response (200):**
```json
{
  "message": "Profile retrieved successfully",
  "data": {
    "id": "ops-user-uuid",
    "name": "John Doe",
    "email": "john.doe@company.com",
    "roles": ["Admin", "Approver"],
    "status": "active"
  }
}
```

**Error Responses:**
- `401` - Unauthorized (missing/invalid token)
- `500` - Failed to retrieve profile

---

## 2. Business Management Endpoints

All business endpoints require JWT authentication.

### 2.1 List Businesses (Paginated)

**Endpoint:** `GET /ops/v1/business?page=1&limit=20&kyb_status=pending`
**Authentication:** Required (JWT)

**Query Parameters:**
- `page` (integer, optional, default: 1) - Page number
- `limit` (integer, optional, default: 20) - Items per page
- `kyb_status` (string, optional) - Filter by status: `pending`, `verified`, `rejected`

**Success Response (200):**
```json
{
  "data": [
    {
      "business_id": "biz-uuid",
      "business_name": "Acme Corp",
      "gstin": "29ABCDE1234F1Z5",
      "kyb_status": "pending",
      "created_at": "2025-01-15T10:30:00Z",
      "owner_name": "Jane Smith",
      "owner_email": "jane@acme.com"
    }
  ],
  "pagination": {
    "currentPage": 1,
    "pageSize": 20,
    "totalItems": 150,
    "totalPages": 8
  }
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `500` - Internal server error

---

### 2.2 Search Businesses

**Endpoint:** `GET /ops/v1/business/search?q=acme&kyb_status=verified`
**Authentication:** Required (JWT)

**Query Parameters:**
- `q` (string, required, min: 2) - Search term (name, GSTIN, email)
- `kyb_status` (string, optional) - Filter by status

**Success Response (200):**
```json
[
  {
    "business_id": "biz-uuid",
    "business_name": "Acme Corp",
    "gstin": "29ABCDE1234F1Z5",
    "kyb_status": "verified",
    "owner_email": "jane@acme.com"
  }
]
```

**Error Responses:**
- `400` - Invalid/missing search term
- `401` - Unauthorized
- `500` - Search failed

---

### 2.3 Get Business Details

**Endpoint:** `GET /ops/v1/business/:bid`
**Authentication:** Required (JWT)

**Path Parameters:**
- `bid` (string, required) - Business ID (UUID)

**Success Response (200):**
```json
{
  "business_id": "biz-uuid",
  "business_name": "Acme Corp",
  "legal_name": "Acme Corporation Pvt Ltd",
  "gstin": "29ABCDE1234F1Z5",
  "pan": "ABCDE1234F",
  "kyb_status": "pending",
  "kyb_data": {
    "gstin_verified": true,
    "gstin_data": {
      "trade_name": "Acme Corp",
      "legal_name": "Acme Corporation Pvt Ltd",
      "address": "123 Main St, Bangalore",
      "status": "Active"
    },
    "income_tax_verified": false,
    "company_id_verified": false
  },
  "created_at": "2025-01-15T10:30:00Z",
  "users": [
    {
      "user_id": "user-uuid",
      "name": "Jane Smith",
      "email": "jane@acme.com",
      "role": "owner",
      "kyc_status": "verified"
    }
  ]
}
```

**Error Responses:**
- `400` - Invalid business ID
- `401` - Unauthorized
- `404` - Business not found
- `500` - Failed to fetch details

---

### 2.4 Reload Business Data

**Endpoint:** `POST /ops/v1/business/:bid/reload-data?type=gstin`
**Authentication:** Required (JWT)
**Description:** Re-fetches business verification data from external APIs

**Path Parameters:**
- `bid` (string, required) - Business ID

**Query Parameters:**
- `type` (string, required) - Data type to reload:
  - `gstin` - GSTIN verification data
  - `income_tax_id` - Income tax verification
  - `company_id` - Company registration data

**Success Response (200):**
```json
{
  "message": "Business data reloaded successfully"
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `404` - Business not found
- `500` - Reload failed

---

### 2.5 KYB Action (Reject/Verify)

**Endpoint:** `POST /ops/v1/business/:bid/kyb-action?action=reject`
**Authentication:** Required (JWT)

**Path Parameters:**
- `bid` (string, required) - Business ID

**Query Parameters:**
- `action` (string, required) - `reject` or `verify`

**Request Body:**
```json
{
  "notes": "Missing company incorporation certificate"
}
```

**Success Response (200):**
```json
{
  "message": "KYB rejected successfully"
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `404` - Business not found
- `500` - Action failed

---

### 2.6 Update User Role

**Endpoint:** `POST /ops/v1/business/:bid/user-action?user_id=xxx&action=set_admin`
**Authentication:** Required (JWT)

**Path Parameters:**
- `bid` (string, required) - Business ID

**Query Parameters:**
- `user_id` (string, required) - User ID (UUID)
- `action` (string, required) - Role action:
  - `set_owner`
  - `set_admin`
  - `set_manager`
  - `set_auditor`

**Success Response (200):**
```json
{
  "message": "User role updated successfully"
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `404` - User not associated with business
- `500` - Update failed

---

## 3. User Management Endpoints

All user endpoints require JWT authentication.

### 3.1 Search Users

**Endpoint:** `GET /ops/v1/user/search?business_id=biz1&business_id=biz2&q=john`
**Authentication:** Required (JWT)

**Query Parameters:**
- `business_id` (array of strings, optional) - Filter by business IDs
- `q` (string, optional, min: 2) - Search term (name, email, phone)

**Success Response (200):**
```json
[
  {
    "user_id": "user-uuid",
    "name": "John Doe",
    "email": "john@example.com",
    "phone": "+919876543210",
    "kyc_status": "verified",
    "businesses": [
      {
        "business_id": "biz-uuid",
        "business_name": "Acme Corp",
        "role": "admin"
      }
    ]
  }
]
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `500` - Search failed

---

### 3.2 Get User Details

**Endpoint:** `GET /ops/v1/user/:user_id?bid=biz-123`
**Authentication:** Required (JWT)

**Path Parameters:**
- `user_id` (string, required) - User ID (UUID)

**Query Parameters:**
- `bid` (string, optional) - Business ID for business-specific info

**Success Response (200):**
```json
{
  "user_id": "user-uuid",
  "name": "John Doe",
  "email": "john@example.com",
  "phone": "+919876543210",
  "kyc_status": "verified",
  "kyc_data": {
    "pan_number": "ABCDE1234F",
    "pan_name": "JOHN DOE",
    "pan_verified": true,
    "aadhaar_last_4": "1234",
    "aadhaar_verified": true
  },
  "created_at": "2024-06-10T15:20:00Z",
  "businesses": [
    {
      "business_id": "biz-uuid",
      "business_name": "Acme Corp",
      "role": "admin"
    }
  ]
}
```

**Error Responses:**
- `400` - Invalid user ID
- `401` - Unauthorized
- `404` - User not found
- `500` - Failed to fetch details

---

### 3.3 Reload User PAN Data

**Endpoint:** `POST /ops/v1/user/:user_id/reload-pan?type=pan_com`
**Authentication:** Required (JWT)

**Path Parameters:**
- `user_id` (string, required) - User ID

**Query Parameters:**
- `type` (string, required) - Data type:
  - `pan_com` - PAN comprehensive data
  - `pan_aadhaar_link` - PAN-Aadhaar linkage status

**Success Response (200):**
```json
{
  "message": "User PAN data reloaded successfully"
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `404` - User not found
- `500` - Reload failed

---

### 3.4 KYC Action (Reject)

**Endpoint:** `POST /ops/v1/user/:user_id/kyc-action?action=reject`
**Authentication:** Required (JWT)

**Path Parameters:**
- `user_id` (string, required) - User ID

**Query Parameters:**
- `action` (string, required) - Currently only `reject` supported

**Request Body:**
```json
{
  "notes": "PAN name mismatch"
}
```

**Success Response (200):**
```json
{
  "message": "KYC rejected successfully"
}
```

**Error Responses:**
- `400` - Invalid parameters
- `401` - Unauthorized
- `404` - User not found
- `500` - Action failed

---

## 4. Notification Endpoints

### 4.1 Trigger Bulk Notifications

**Endpoint:** `POST /ops/v1/notification/trigger`
**Authentication:** Required (JWT)
**Description:** Triggers notifications to multiple users/businesses

**Request Body:**
```json
{
  "event_type": "payment_received",
  "targets": [
    {
      "business_id": "biz-uuid-1",
      "type": "unicast",
      "ids": ["user-uuid-1"],
      "channels": ["email", "push"]
    },
    {
      "business_id": "biz-uuid-2",
      "type": "multicast",
      "ids": ["user-uuid-2", "user-uuid-3"],
      "channels": ["email"]
    },
    {
      "business_id": "biz-uuid-3",
      "type": "broadcast",
      "minimum_role": "admin",
      "channels": ["email", "push"]
    }
  ],
  "template_data": {
    "amount": "50000",
    "transaction_id": "TXN123456"
  }
}
```

**Request Fields:**
- `event_type` (string, required) - Notification event type
- `targets` (array, required) - Target configurations:
  - `business_id` (string, required) - Business context
  - `type` (string, required) - `unicast`, `multicast`, or `broadcast`
  - `ids` (array, conditional) - User IDs (required for unicast/multicast with IDs)
  - `minimum_role` (string, optional) - Role filter for broadcast/multicast
  - `channels` (array, required) - `email`, `push`, `sms`
- `template_data` (object, optional) - Dynamic template variables

**Target Type Rules:**
- **Unicast**: Must have exactly 1 user ID
- **Multicast**: Must have either IDs or MinimumRole (not both)
- **Broadcast**: Optionally has MinimumRole (nil = all users in business)

**Success Response (200):**
```json
{
  "total_targets": 3,
  "successful": 3,
  "failed": 0,
  "results": [
    {
      "business_id": "biz-uuid-1",
      "success": true,
      "notifications_created": 1
    },
    {
      "business_id": "biz-uuid-2",
      "success": true,
      "notifications_created": 2
    },
    {
      "business_id": "biz-uuid-3",
      "success": true,
      "notifications_created": 5
    }
  ]
}
```

**Error Responses:**
- `400` - Invalid request body or target configuration
- `401` - Unauthorized
- `500` - Failed to trigger notifications

---

## 5. Error Handling

### Standard Error Format

All error responses follow this format:

```json
{
  "error": "Error message",
  "message": "Detailed error description"
}
```

### Validation Errors

Validation errors include field-specific details:

```json
{
  "error": "Validation failed",
  "errors": {
    "email": "Invalid email format",
    "page": "Must be greater than 0"
  }
}
```

### Authentication Errors

When access token expires:

```json
{
  "error": "TOKEN_EXPIRED",
  "message": "Access token has expired, please refresh."
}
```

**Frontend Action:** Call `/ops/v1/auth/refresh` to get a new access token.

When refresh token is invalid:

```json
{
  "error": "INVALID_TOKEN",
  "message": "Invalid token"
}
```

**Frontend Action:** Redirect to login page.

---

## 6. Frontend Integration Guide

### 6.1 Initial Setup

1. **Check if user is authenticated:**
   - Check for `access_token` cookie
   - If exists, try calling `/ops/v1/user/profile`
   - If fails with 401, attempt refresh

2. **Login Flow:**
   ```javascript
   // Step 1: Get auth URL
   const response = await fetch('/ops/v1/auth/login');
   const { data } = await response.json();

   // Step 2: Redirect user to Microsoft
   window.location.href = data.authUrl;

   // Step 3: After redirect back to /callback
   // Cookies are automatically set, redirect to dashboard
   window.location.href = '/dashboard';
   ```

### 6.2 Token Refresh Strategy

Use an Axios/Fetch interceptor:

```javascript
// Response interceptor
axios.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      const originalRequest = error.config;

      // Try refresh
      try {
        await axios.post('/ops/v1/auth/refresh');
        // Retry original request
        return axios(originalRequest);
      } catch (refreshError) {
        // Refresh failed, redirect to login
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);
```

### 6.3 Logout

```javascript
async function logout() {
  await fetch('/ops/v1/auth/logout', { method: 'POST' });
  window.location.href = '/login';
}
```

### 6.4 Cookie Configuration

All cookies are:
- **HttpOnly**: true (cannot be accessed via JavaScript)
- **Secure**: true (HTTPS only in production)
- **SameSite**: Lax (CSRF protection)

You don't need to manually manage cookies - they're automatically sent with requests.

---

## 7. Development Mode

For local development without OAuth setup:

**`.env` Configuration:**
```bash
# Bypass OAuth validation
OPS_OAUTH_SKIP_VALIDATION=true

# Disable ops access control
OPS_REJECT_ACCESS=false
```

**Default Dev User:**
- User ID: `dev-user`
- Email: `dev@xcentro.in`
- Roles: `[]` (empty)

**⚠️ NEVER enable `OPS_OAUTH_SKIP_VALIDATION` in production!**

---

## 8. Rate Limiting & CORS

### Rate Limiting
- Configured via `RATE_LIMITER_*` environment variables
- Default: 120 requests/minute per device

### CORS
- Configured via `CORS_ALLOW_ORIGINS`
- Credentials: Enabled (for cookies)
- Dev default: `http://localhost:8080`

---

## 9. Health Check

**Endpoint:** `GET /ops/v1/health`
**Authentication:** None
**Description:** Health check endpoint

**Response:**
```json
{
  "status": "ok",
  "service": "ops-dashboard"
}
```

---

## 10. Common Use Cases

### Use Case 1: Verify a Business

```javascript
// 1. Get business details
const business = await fetch(`/ops/v1/business/${businessId}`).then(r => r.json());

// 2. If data is stale, reload it
if (needsReload) {
  await fetch(`/ops/v1/business/${businessId}/reload-data?type=gstin`, {
    method: 'POST'
  });

  // Fetch again to get updated data
  business = await fetch(`/ops/v1/business/${businessId}`).then(r => r.json());
}

// 3. Verify KYB
await fetch(`/ops/v1/business/${businessId}/kyb-action?action=verify`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ notes: 'All documents verified' })
});
```

### Use Case 2: Reject User KYC

```javascript
// 1. Get user details
const user = await fetch(`/ops/v1/user/${userId}`).then(r => r.json());

// 2. Reject KYC
await fetch(`/ops/v1/user/${userId}/kyc-action?action=reject`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    notes: 'PAN name does not match Aadhaar name'
  })
});

// 3. Send notification
await fetch('/ops/v1/notification/trigger', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    event_type: 'kyc_rejected',
    targets: [{
      business_id: user.businesses[0].business_id,
      type: 'unicast',
      ids: [userId],
      channels: ['email']
    }],
    template_data: {
      reason: 'PAN name mismatch'
    }
  })
});
```

### Use Case 3: Search and Filter

```javascript
// Search businesses by name/GSTIN
const results = await fetch(
  '/ops/v1/business/search?q=acme&kyb_status=pending'
).then(r => r.json());

// Get paginated list with filters
const businesses = await fetch(
  '/ops/v1/business?page=1&limit=50&kyb_status=verified'
).then(r => r.json());
```

---

## Appendix A: Status Values

### KYB Status
- `pending` - Awaiting verification
- `verified` - Verified by ops
- `rejected` - Rejected by ops

### KYC Status
- `pending` - Awaiting verification
- `verified` - Verified by ops/system
- `rejected` - Rejected by ops

### User Roles (in Business)
- `owner` - Business owner
- `admin` - Administrator
- `manager` - Manager
- `auditor` - Auditor

### Notification Channels
- `email` - Email notification
- `push` - Push notification (mobile/web)
- `sms` - SMS notification (future)

---

## Appendix B: Environment Variables Reference

```bash
# JWT Configuration
JWT_SECRET_KEY=your_secret_key
JWT_ACCESS_TOKEN_EXPIRY=1h
JWT_REFRESH_TOKEN_EXPIRY=720h  # 30 days

# OAuth2/OIDC Configuration
OPS_OAUTH_CLIENT_ID=your_office365_client_id
OPS_OAUTH_CLIENT_SECRET=your_office365_client_secret
OPS_OAUTH_REDIRECT_URL=http://localhost:8080/ops/v1/auth/callback
OPS_OAUTH_TENANT_ID=organizations
OPS_OAUTH_SKIP_VALIDATION=false  # Dev only!

# Token Encryption
OPS_TOKEN_ENCRYPTION_KEY=your_32_byte_encryption_key

# Access Control
OPS_REJECT_ACCESS=false  # Allow ops dashboard access
```

---

**Last Updated:** 2025-01-19
**API Version:** v1
