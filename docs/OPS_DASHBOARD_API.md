# Ops Dashboard API Documentation

This document describes all API endpoints available for the ops dashboard with OAuth2/OIDC authentication.

## Base URL

```
http://localhost:8080/ops/v1
```

## Authentication

All endpoints except login/callback/refresh/health require JWT authentication via **HttpOnly cookies**.

### Cookies Used
- `access_token`: JWT access token (1 hour expiry, HttpOnly, Secure in production)
- `refresh_token`: JWT refresh token (30 days expiry, HttpOnly, Secure in production)
- `oauth_state`: Temporary state for CSRF protection during OAuth flow

### Authentication Middleware

All protected ops routes are protected by `OpsJWTMiddleware` which:
1. Reads `access_token` from HttpOnly cookie (JavaScript **cannot** access this cookie)
2. Validates the JWT token
3. Returns structured error responses for different scenarios
4. Sets `opsUserId`, `opsUserEmail`, `opsUserRoles` in request context

**Protected Routes:**
- `/ops/v1/user/profile` - User profile
- `/ops/v1/business/*` - Business management
- `/ops/v1/user/*` - User management
- `/ops/v1/notification/*` - Notifications
- `/ops/v1/payout/*` - Payouts
- `/ops/v1/collection/*` - Collections/Payments
- `/ops/v1/fee-rules/*` - Fee rules management

**Public Routes (no JWT required):**
- `/ops/v1/auth/login` - Initiate OAuth login
- `/ops/v1/auth/callback` - OAuth callback
- `/ops/v1/auth/refresh` - Refresh access token
- `/ops/v1/health` - Health check

### Authentication Error Responses

#### Protected Endpoint Errors (From Middleware)

When calling any protected endpoint (e.g., `/ops/v1/user/profile`, `/ops/v1/business`, etc.):

**1. MISSING_TOKEN (401)** - No access token cookie present
```json
{
  "error": "MISSING_TOKEN",
  "message": "Access token cookie required"
}
```
**Causes:**
- User never logged in
- Cookies were cleared
- Cookie expired and browser deleted it
- Wrong domain/path for cookie

**Frontend Action:** → Redirect to `/login` page

---

**2. TOKEN_EXPIRED (401)** - Access token is expired (> 1 hour old)
```json
{
  "error": "TOKEN_EXPIRED",
  "message": "Access token has expired, please refresh."
}
```
**Causes:**
- Access token is older than 1 hour (configured via `JWT_ACCESS_TOKEN_EXPIRY`)
- This is **normal behavior** - access tokens expire frequently for security

**Frontend Action:** → Call `/ops/v1/auth/refresh` to get new access token, then retry original request

---

**3. INVALID_TOKEN (401)** - Access token is malformed or invalid
```json
{
  "error": "INVALID_TOKEN",
  "message": "Invalid token"
}
```
**Causes:**
- Token signature verification failed
- Token was tampered with
- Token was issued by different server (JWT secret changed)
- Token structure is corrupted

**Frontend Action:** → Redirect to `/login` page (cannot recover)

---

#### Refresh Endpoint Errors

When calling `/ops/v1/auth/refresh`:

**4. MISSING_REFRESH_TOKEN (401)** - No refresh token cookie present
```json
{
  "error": "MISSING_REFRESH_TOKEN",
  "message": "Refresh token cookie required"
}
```
**Causes:**
- User never logged in
- Refresh token cookie was cleared
- Cookie expired (> 30 days old) and browser deleted it

**Frontend Action:** → Redirect to `/login` page

---

**5. REFRESH_TOKEN_EXPIRED (401)** - Refresh token is expired or invalid
```json
{
  "error": "REFRESH_TOKEN_EXPIRED",
  "message": "Refresh token has expired or is invalid. Please login again."
}
```
**Causes:**
- Refresh token is older than 30 days (configured via `JWT_REFRESH_TOKEN_EXPIRY`)
- Refresh token was revoked (user logged out)
- Session was deleted from database
- OIDC refresh token expired with Microsoft Entra ID
- User was deactivated in database

**Frontend Action:** → Redirect to `/login` page (session completely expired, requires re-authentication)

---

#### Complete Error Flow Decision Tree

```
User makes API request to protected endpoint
    ↓
[Middleware validates access_token cookie]
    ↓
├─ No cookie → MISSING_TOKEN → Redirect to /login
├─ Empty cookie → MISSING_TOKEN → Redirect to /login
├─ Expired token → TOKEN_EXPIRED → Call /ops/v1/auth/refresh
│                                        ↓
│                                   [Validate refresh_token]
│                                        ↓
│                                   ├─ No cookie → MISSING_REFRESH_TOKEN → Redirect to /login
│                                   ├─ Expired/Invalid → REFRESH_TOKEN_EXPIRED → Redirect to /login
│                                   └─ Valid → New access_token set → Retry original request
│
└─ Invalid token → INVALID_TOKEN → Redirect to /login
```

### Dev Mode
Set `OPS_OAUTH_SKIP_VALIDATION=true` in `.env` to bypass authentication (dev/testing only).
**⚠️ NEVER enable in production!**

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
**Description:** Handles OAuth2 callback from Microsoft Entra ID, sets auth cookies, and redirects to ops dashboard

**Query Parameters:**
- `code` (string, required) - Authorization code from OAuth provider
- `state` (string, required) - CSRF protection state

**Success Response (302):**
- **Redirect Location:** `FRONTEND_URL` (configured via environment variable)
- Default development: `http://localhost:3000` (set to `http://localhost:3001` for ops dashboard)
- Production example: `https://ops.yourdomain.com`

**Cookies Set:**
- `access_token` - JWT access token (1 hour expiry)
- `refresh_token` - JWT refresh token (30 days expiry)

**Error Responses:**
- `400` - Missing/invalid code or state (JSON response)
- `500` - Authentication failed (JSON response)

**Flow:**
1. User completes authentication with Microsoft Entra ID
2. Microsoft redirects to this callback endpoint with `code` and `state`
3. Backend validates state for CSRF protection
4. Backend exchanges authorization code for OAuth tokens
5. Backend creates/updates ops user in database
6. Backend generates internal JWT access and refresh tokens
7. Backend sets authentication cookies (`access_token`, `refresh_token`)
8. **Backend redirects user to ops dashboard root page** (configured via `FRONTEND_URL`)
9. Frontend receives authenticated user with cookies already set

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

### 6.1 Initial Setup & Authentication Flow

**Important:** Since cookies are HttpOnly, JavaScript **cannot** read them. Instead, rely on middleware error responses.

1. **Login Flow:**
   ```javascript
   // Step 1: Get auth URL from backend
   const response = await fetch('/ops/v1/auth/login', {
     credentials: 'include' // Important: include cookies in request
   });
   const { data } = await response.json();

   // Step 2: Redirect user to Microsoft for authentication
   window.location.href = data.authUrl;

   // Step 3: After user authenticates with Microsoft
   // Microsoft redirects to backend: /ops/v1/auth/callback?code=xxx&state=xxx

   // Step 4: Backend automatically:
   // - Validates the OAuth callback
   // - Creates/updates ops user
   // - Sets HttpOnly authentication cookies (access_token, refresh_token)
   // - Redirects user BACK to ops dashboard root (/)

   // Step 5: Your ops dashboard root page (/) loads
   ```

2. **Ops Dashboard Root Page (`/`) Handler:**
   ```javascript
   // In your root page component (e.g., App.js, index.js)
   useEffect(() => {
     // Try to fetch user profile - middleware will validate token
     fetch('/ops/v1/user/profile', {
       credentials: 'include' // Important: include cookies
     })
       .then(async (res) => {
         if (res.ok) {
           // Valid token, user is authenticated
           const profile = await res.json();
           setUser(profile.data);
           setUserAuthenticated(true);
         } else {
           // Check error type
           const error = await res.json();

           if (error.error === 'TOKEN_EXPIRED') {
             // Token expired, try refresh
             return handleTokenRefresh();
           } else {
             // MISSING_TOKEN or INVALID_TOKEN - redirect to login
             router.push('/login');
           }
         }
       })
       .catch(() => {
         // Network error or other issue - redirect to login
         router.push('/login');
       });
   }, []);

   async function handleTokenRefresh() {
     try {
       const res = await fetch('/ops/v1/auth/refresh', {
         method: 'POST',
         credentials: 'include'
       });

       if (res.ok) {
         // New access token set, retry fetching profile
         window.location.reload(); // Or re-fetch profile
       } else {
         // Check refresh error type
         const error = await res.json();

         // All refresh errors require re-login:
         // - MISSING_REFRESH_TOKEN: User needs to login
         // - REFRESH_TOKEN_EXPIRED: Session expired, need fresh login
         console.log('Refresh failed:', error.error);
         router.push('/login');
       }
     } catch (err) {
       router.push('/login');
     }
   }
   ```

3. **Key Points:**
   - Always use `credentials: 'include'` in fetch requests to include cookies
   - Don't try to read cookies via JavaScript (they're HttpOnly)
   - Handle `TOKEN_EXPIRED` by calling refresh endpoint
   - Handle `MISSING_TOKEN` or `INVALID_TOKEN` by redirecting to login
   - Middleware validates every protected request automatically

### 6.2 Token Refresh Strategy

Use an Axios/Fetch interceptor to automatically handle token expiration:

**Axios Setup:**
```javascript
// Configure Axios to include credentials
axios.defaults.withCredentials = true;

// Response interceptor
axios.interceptors.response.use(
  (response) => response,
  async (error) => {
    if (error.response?.status === 401) {
      const errorCode = error.response?.data?.error;
      const originalRequest = error.config;

      // Only retry on TOKEN_EXPIRED, not MISSING_TOKEN or INVALID_TOKEN
      if (errorCode === 'TOKEN_EXPIRED' && !originalRequest._retry) {
        originalRequest._retry = true; // Prevent infinite retry loop

        try {
          // Try to refresh the access token
          await axios.post('/ops/v1/auth/refresh');

          // Retry the original request with new token
          return axios(originalRequest);
        } catch (refreshError) {
          // Refresh failed, redirect to login
          window.location.href = '/login';
          return Promise.reject(refreshError);
        }
      }

      // MISSING_TOKEN or INVALID_TOKEN - redirect to login immediately
      // Also handles refresh errors: MISSING_REFRESH_TOKEN, REFRESH_TOKEN_EXPIRED
      window.location.href = '/login';
      return Promise.reject(error);
    }

    return Promise.reject(error);
  }
);

// Example: All possible error codes the interceptor handles
// From protected endpoints:
//   - MISSING_TOKEN → redirect to login
//   - TOKEN_EXPIRED → try refresh, then retry request
//   - INVALID_TOKEN → redirect to login
// From refresh endpoint:
//   - MISSING_REFRESH_TOKEN → redirect to login
//   - REFRESH_TOKEN_EXPIRED → redirect to login
```

**Fetch Setup:**
```javascript
// Create a custom fetch wrapper
async function authenticatedFetch(url, options = {}) {
  const defaultOptions = {
    credentials: 'include', // Always include cookies
    ...options
  };

  let response = await fetch(url, defaultOptions);

  // Handle 401 errors
  if (response.status === 401) {
    const error = await response.json();

    // Only retry on TOKEN_EXPIRED
    if (error.error === 'TOKEN_EXPIRED') {
      // Try refresh
      const refreshResponse = await fetch('/ops/v1/auth/refresh', {
        method: 'POST',
        credentials: 'include'
      });

      if (refreshResponse.ok) {
        // Retry original request with new token
        response = await fetch(url, defaultOptions);
      } else {
        // Refresh failed, redirect to login
        window.location.href = '/login';
        throw new Error('Authentication failed');
      }
    } else {
      // MISSING_TOKEN or INVALID_TOKEN - redirect to login
      window.location.href = '/login';
      throw new Error('Authentication required');
    }
  }

  return response;
}

// Usage
const profile = await authenticatedFetch('/ops/v1/user/profile')
  .then(res => res.json());
```

### 6.3 Logout

```javascript
async function logout() {
  await fetch('/ops/v1/auth/logout', {
    method: 'POST',
    credentials: 'include' // Include cookies to identify session
  });
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

# Frontend Configuration
# For ops dashboard, set this to your ops frontend URL
FRONTEND_URL=http://localhost:3001  # Dev: http://localhost:3001, Prod: https://ops.yourdomain.com

# Access Control
OPS_REJECT_ACCESS=false  # Allow ops dashboard access
```

---

**Last Updated:** 2025-12-05
**API Version:** v1
