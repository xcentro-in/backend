# Auth Migration Guide

This document covers all breaking auth changes the frontend must adopt. Changes are cumulative — the system is in its final post-migration state and all items below apply.

---

## Breaking Changes Summary

| # | What changed | Action required |
|---|---|---|
| 1 | JWT carries `bid`, `role`, `kyc_status`, `kyc_needed` | Read these from `verify-session` response, not JWT |
| 2 | Login response body is now `{ "message": "Login successful" }` only | Call `GET /v1/auth/verify-session` after login to hydrate app state |
| 3 | New `GET /v1/auth/verify-session` endpoint | Call on every app load and after login |
| 4 | Business switch is server-side via `POST /v1/auth/switch-business` | Remove `X-Business-ID` header; call switch-business to change context |
| 5 | `X-Business-ID` header is ignored | Remove from all API requests |

---

## 1. JWT Shape

The JWT access token now embeds business context and KYC state. You should **not** decode the JWT in browser code — use `verify-session` instead. This table is provided for completeness (SSR or native apps only).

| Claim | Type | Description |
|---|---|---|
| `sub` | `string` | User UUID |
| `session_id` | `string` | Session UUID |
| `device_type` | `string` | `web` or `mobile` |
| `device_version` | `int` | Session revocation version |
| `bid` | `string` | Active business UUID. `""` if none. |
| `role` | `string` | Role in active business: `owner`, `admin`, `manager`, `auditor`, `none`, or `""`. |
| `kyc_status` | `string` | User KYC status at token issuance: `none`, `in_progress`, `completed`, or `verified`. |
| `kyc_needed` | `boolean` | Whether the user still needs to complete KYC for the active business. |
| `iat` / `exp` | `number` | Standard JWT timestamps |

`kyc_needed` and `kyc_status` are re-fetched from the DB on every token refresh and on every `switch-business`, so they stay current.

---

## 2. Login Response

The login endpoint no longer returns user or business data in the response body. Its only job is to establish the session via cookies.

**Request:**
```
POST /v1/auth/login
Content-Type: application/json
```
```json
{ "email": "user@example.com", "password": "..." }
```

**Response (200):**
```json
{ "message": "Login successful" }
```

**Cookies set on success:**

| Cookie | Description |
|---|---|
| `access_token` | Short-lived JWT (httpOnly, Secure) |
| `refresh_token` | Long-lived refresh token (httpOnly, Secure) |
| `csrf_token` | CSRF token — read this and send as `X-CSRF-Token` on mutating requests |

After receiving the 200, immediately call `GET /v1/auth/verify-session` to load user and business state.

---

## 3. verify-session (New — call this on every app load)

This endpoint returns the full session context. Call it:
- After login
- On every page load / app rehydration (e.g. in a root layout effect)
- After `switch-business`

**Request:**
```
GET /v1/auth/verify-session
Cookies: access_token (sent automatically with credentials: 'include')
Headers: X-CSRF-Token: <value of csrf_token cookie>
```

**Response (200):**
```json
{
  "message": "Session verified",
  "data": {
    "uid": "user-uuid",
    "kyc_status": "completed",
    "businesses": [
      {
        "bid": "business-uuid",
        "name": "Acme Exports Pvt Ltd",
        "role": "owner",
        "kyb_status": "completed",
        "kyc_needed": false
      }
    ],
    "active_bid": "business-uuid"
  }
}
```

**Response field reference:**

| Field | Type | Description |
|---|---|---|
| `uid` | `string` | User UUID |
| `kyc_status` | `string` | `none`, `in_progress`, `completed`, `verified` |
| `businesses` | `array` | All businesses the user belongs to |
| `businesses[].bid` | `string` | Business UUID |
| `businesses[].name` | `string` | Business display name |
| `businesses[].role` | `string` | `owner`, `admin`, `manager`, `auditor`, `none` |
| `businesses[].kyb_status` | `string` | `none`, `pending`, `completed`, `verified`. Non-owners who did not initiate KYB always see `"completed"` — use this to gate onboarding screens. |
| `businesses[].kyc_needed` | `boolean` | True if the user must complete personal KYC before accessing this business |
| `active_bid` | `string` | UUID of the currently active business (matches `bid` in the JWT). `""` if none. |

**Error responses:**

| Status | Meaning |
|---|---|
| 401 | No valid access token cookie — redirect to login |
| 500 | Server error fetching session data |

**TypeScript example:**
```typescript
interface Business {
  bid: string;
  name: string;
  role: string;
  kyb_status: string;
  kyc_needed: boolean;
}

interface SessionData {
  uid: string;
  kyc_status: string;
  businesses: Business[];
  active_bid: string;
}

async function verifySession(): Promise<SessionData> {
  const res = await fetch('/v1/auth/verify-session', {
    credentials: 'include',
    headers: {
      'X-CSRF-Token': getCsrfTokenFromCookie(),
    },
  });
  if (res.status === 401) {
    // Token expired or missing — send to login
    throw new Error('UNAUTHENTICATED');
  }
  if (!res.ok) throw new Error('Failed to verify session');
  const { data } = await res.json();
  return data;
}

// Call after login and on every app load:
const session = await verifySession();
// session.uid, session.kyc_status, session.businesses, session.active_bid
```

---

## 4. Switch Business

When a user belongs to multiple businesses, call this to switch the active context. The server issues a new `access_token` cookie with updated `bid`, `role`, `kyc_needed`. No re-login required.

**Request:**
```
POST /v1/auth/switch-business
Cookies: access_token
Headers: X-CSRF-Token: <value of csrf_token cookie>
Content-Type: application/json
```
```json
{ "bid": "target-business-uuid" }
```

**Response (200):**
```json
{
  "message": "Business switched successfully",
  "data": {
    "bid": "target-business-uuid",
    "role": "admin",
    "kyb_status": "completed",
    "kyc_needed": false
  }
}
```

A new `access_token` cookie is set automatically. The `refresh_token` and `csrf_token` are unchanged. After this call, call `verify-session` to refresh app state if needed, or use the `data` fields directly.

**Error responses:**

| Status | Meaning |
|---|---|
| 400 | `bid` missing or not a valid UUID |
| 401 | No valid access token cookie |
| 403 | User is not a member of that business |
| 500 | Server error |

---

## 5. Remove X-Business-ID Header

The `X-Business-ID` request header is no longer read by the server. Remove it from all API calls.

**Before:**
```typescript
headers: {
  'Content-Type': 'application/json',
  'X-Business-ID': activeBusinessId,  // remove this
}
```

**After:**
```typescript
headers: {
  'Content-Type': 'application/json',
  // Business context is in the access_token cookie — nothing to add here.
}
```

---

## Routing Logic

Use the `verify-session` response to decide where to send the user on app load:

```
verify-session →
  401                          → /login
  kyc_status !== "completed"
    && kyc_status !== "verified" → /onboarding/kyc
  active_bid === ""              → /onboarding/create-business
  active business kyb_status
    not "completed"/"verified"   → /onboarding/kyb   (only if role=owner or kyb_initiated)
  active business kyc_needed     → /onboarding/kyc
  role === "none"                → /pending-approval
  otherwise                      → /dashboard
```

Note: `kyb_status` is only the real value for the business owner or the user who initiated KYB. All other users see `"completed"` — so the KYB gate will correctly pass them through to the KYC or pending-approval check.

---

## Migration Checklist

```
[ ] After login, call GET /v1/auth/verify-session to load user + business state
[ ] On every app load, call GET /v1/auth/verify-session before rendering protected routes
[ ] No longer read user/business data from the login response body
[ ] X-Business-ID header removed from all API requests
[ ] Business switch calls POST /v1/auth/switch-business (not header swap)
[ ] Routing logic uses kyc_status, kyb_status, kyc_needed from verify-session
[ ] 401 from verify-session redirects to login
```
