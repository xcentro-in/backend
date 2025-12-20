# CSRF Token Implementation Guide for Frontend

## Overview

XCentro backend implements CSRF (Cross-Site Request Forgery) protection using an HMAC-based token system. This document explains how to read the CSRF cookie and add it to request headers when making authenticated API calls.

## What is CSRF Protection?

CSRF protection prevents malicious websites from making unauthorized requests to our API on behalf of authenticated users. Our implementation uses a token that must be included in request headers for all authenticated endpoints.

## Backend Implementation Summary

### Regular API (`/v1/*`)
- **Cookie Name**: `csrf_token`
- **Header Name**: `X-CSRF-Token` (case-sensitive)
- **Token Generation**: HMAC-SHA256 of session_id using JWT secret key
- **Cookie Attributes**:
  - `HttpOnly`: `false` (JavaScript can read this cookie)
  - `Secure`: Enabled in production
  - `SameSite`: Configured per environment
  - `Path`: `/` (available for all routes)
  - `MaxAge`: 30 days (matches refresh token expiry)

### Ops Dashboard (`/ops/v1/*`)
- **Cookie Name**: `ops_csrf_token` (different from regular API)
- **Header Name**: `X-CSRF-Token` (same header name)
- **Token Generation**: HMAC-SHA256 of ops session_id using JWT secret key
- **Cookie Attributes**: Same as regular API
- **Applied to**: POST, PUT, PATCH, DELETE operations only (GET requests skip CSRF)

## When is CSRF Required?

CSRF protection is enforced on **all authenticated endpoints** that modify data (POST, PUT, PATCH, DELETE). The backend applies `CSRFMiddleware` to protected routes.

## How to Implement (JavaScript/TypeScript)

### Step 1: Read the CSRF Cookie

Create a utility function to extract the CSRF token cookie. Use `csrf_token` for regular API or `ops_csrf_token` for ops dashboard:

```javascript
/**
 * Reads the CSRF token cookie from document.cookie
 * @param {string} cookieName - 'csrf_token' for regular API or 'ops_csrf_token' for ops dashboard
 * @returns {string|null} CSRF token value or null if not found
 */
function getCSRFToken(cookieName = 'csrf_token') {
  const cookies = document.cookie.split(';');

  for (let cookie of cookies) {
    const [name, value] = cookie.trim().split('=');
    if (name === cookieName) {
      return decodeURIComponent(value);
    }
  }

  return null;
}

// Usage:
const apiToken = getCSRFToken('csrf_token');        // For /v1/* routes
const opsToken = getCSRFToken('ops_csrf_token');    // For /ops/v1/* routes
```

**TypeScript version:**

```typescript
function getCSRFToken(cookieName: string = 'csrf_token'): string | null {
  const cookies = document.cookie.split(';');

  for (const cookie of cookies) {
    const [name, value] = cookie.trim().split('=');
    if (name === cookieName) {
      return decodeURIComponent(value);
    }
  }

  return null;
}

// Usage:
const apiToken = getCSRFToken('csrf_token');        // For /v1/* routes
const opsToken = getCSRFToken('ops_csrf_token');    // For /ops/v1/* routes
```

### Step 2: Add CSRF Token to Request Headers

Include the `X-CSRF-Token` header in all authenticated requests:

```javascript
// Using fetch API
async function makeAuthenticatedRequest(url, options = {}) {
  const csrfToken = getCSRFToken();

  if (!csrfToken) {
    throw new Error('CSRF token not found. Please log in again.');
  }

  const headers = {
    'Content-Type': 'application/json',
    'X-CSRF-Token': csrfToken,
    ...options.headers,
  };

  const response = await fetch(url, {
    ...options,
    headers,
    credentials: 'include', // Important: sends cookies with request
  });

  if (response.status === 401) {
    // Handle unauthorized - token may be invalid or expired
    throw new Error('Unauthorized. Please log in again.');
  }

  return response;
}

// Example usage
try {
  const response = await makeAuthenticatedRequest('/v1/business', {
    method: 'POST',
    body: JSON.stringify({ name: 'My Business' }),
  });

  const data = await response.json();
  console.log('Success:', data);
} catch (error) {
  console.error('Request failed:', error);
}
```

### Step 3: Axios Configuration (Alternative)

If using Axios, configure an interceptor to automatically add the CSRF token:

```javascript
import axios from 'axios';

// Create axios instance
const apiClient = axios.create({
  baseURL: 'https://api.xcentro.in',
  withCredentials: true, // Important: sends cookies with requests
});

// Add request interceptor to attach CSRF token
apiClient.interceptors.request.use(
  (config) => {
    const csrfToken = getCSRFToken();

    if (csrfToken) {
      config.headers['X-CSRF-Token'] = csrfToken;
    } else {
      console.warn('CSRF token not found');
    }

    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Add response interceptor for error handling
apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      // Handle unauthorized - redirect to login
      console.error('Unauthorized. Redirecting to login...');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

// Example usage
try {
  const response = await apiClient.post('/v1/business', {
    name: 'My Business',
  });
  console.log('Success:', response.data);
} catch (error) {
  console.error('Request failed:', error);
}
```

## Cookie Lifecycle

### When is the Cookie Set?

The `csrf_token` cookie is automatically set by the backend on:
- **Login**: After successful authentication (`POST /v1/auth/login`)
- **Token Refresh**: When access token is refreshed (`POST /v1/auth/refresh`)
- **Signup**: After email verification and initial login

### When is the Cookie Cleared?

The cookie is cleared on:
- **Logout**: When user explicitly logs out (`POST /v1/auth/logout`)
- **Expiry**: After 30 days of inactivity (automatic)
- **Session Invalidation**: When all user sessions are revoked

### Cookie Persistence

- **Expiry**: 30 days (720 hours)
- **Renewal**: Cookie is renewed on each token refresh
- **Path**: Available for all routes under `/`

## Error Handling

### 401 Unauthorized Response

When the CSRF token is missing, invalid, or expired, the backend returns:

```json
{
  "message": "Unauthorized"
}
```

**Handling Strategy:**

```javascript
async function handleAuthenticatedRequest(url, options) {
  try {
    const response = await makeAuthenticatedRequest(url, options);
    return await response.json();
  } catch (error) {
    if (error.message.includes('Unauthorized')) {
      // Clear local session data
      localStorage.clear();
      sessionStorage.clear();

      // Redirect to login
      window.location.href = '/login?reason=session_expired';
    }
    throw error;
  }
}
```

### Missing CSRF Cookie

If the cookie is not present, check:
1. User is logged in
2. `credentials: 'include'` is set in fetch requests
3. `withCredentials: true` is set for Axios
4. Browser is not blocking third-party cookies (if API is on different domain)

## Security Best Practices

### DO:
✅ Always include `credentials: 'include'` (fetch) or `withCredentials: true` (Axios)
✅ Store the CSRF token only in cookies (never in localStorage)
✅ Use HTTPS in production to ensure cookie security
✅ Check for cookie presence before making authenticated requests
✅ Handle 401 errors gracefully by redirecting to login

### DON'T:
❌ Never store CSRF token in localStorage or sessionStorage
❌ Don't expose the CSRF token in URL parameters
❌ Don't share cookies across different domains without proper CORS setup
❌ Don't disable `SameSite` cookie protection

## Testing CSRF Implementation

### Manual Testing with Browser DevTools

1. **Check Cookie Presence:**
   - Open DevTools → Application → Cookies
   - Look for `csrf_token` cookie
   - Verify it has a value (base64-encoded string)

2. **Inspect Request Headers:**
   - Open DevTools → Network
   - Make an authenticated request
   - Check request headers for `X-CSRF-Token`
   - Verify value matches cookie

3. **Test Invalid Token:**
   - Manually modify `X-CSRF-Token` header to invalid value
   - Expect 401 Unauthorized response

### Automated Testing

```javascript
describe('CSRF Token Implementation', () => {
  it('should include CSRF token in authenticated requests', async () => {
    // Login to get cookie
    await login('test@example.com', 'password123');

    // Get CSRF token
    const csrfToken = getCSRFToken();
    expect(csrfToken).toBeTruthy();

    // Make authenticated request
    const response = await fetch('/v1/business', {
      method: 'GET',
      headers: {
        'X-CSRF-Token': csrfToken,
      },
      credentials: 'include',
    });

    expect(response.status).toBe(200);
  });

  it('should return 401 for missing CSRF token', async () => {
    const response = await fetch('/v1/business', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'include',
      body: JSON.stringify({ name: 'Test' }),
    });

    expect(response.status).toBe(401);
  });
});
```

## Troubleshooting

### Issue: "CSRF token not found"

**Causes:**
- User is not logged in
- Cookie expired (30 days)
- Browser blocked cookies
- Using HTTP instead of HTTPS in production

**Solutions:**
1. Verify user is authenticated
2. Check cookie expiry in DevTools
3. Enable cookies in browser settings
4. Use HTTPS in production

### Issue: "Unauthorized" response even with token

**Causes:**
- Token doesn't match server-side calculation
- Session ID changed (token refresh without page reload)
- Token modified in transit

**Solutions:**
1. Refresh the page to get new token
2. Re-authenticate user
3. Check for browser extensions modifying requests

### Issue: Cookie not sent with requests

**Causes:**
- Missing `credentials: 'include'` (fetch)
- Missing `withCredentials: true` (Axios)
- CORS misconfiguration
- SameSite cookie restrictions

**Solutions:**
1. Add `credentials: 'include'` to all fetch requests
2. Configure Axios with `withCredentials: true`
3. Verify CORS headers allow credentials
4. Check SameSite cookie policy

## Backend Reference

For more details on the backend implementation, see:
- **Cookie Management**: `internal/shared/httputil/cookies.go`
- **CSRF Middleware**: `internal/middleware/csrf_middleware.go`
- **Token Generation**: HMAC-SHA256 based on session_id

## Summary Checklist

Before deploying frontend code, ensure:
- [ ] CSRF token is read from `csrf_token` cookie
- [ ] Token is added to `X-CSRF-Token` header (exact casing)
- [ ] `credentials: 'include'` is set for all authenticated requests
- [ ] 401 errors redirect to login page
- [ ] Cookie presence is checked before making requests
- [ ] HTTPS is used in production
- [ ] Error handling is implemented for missing/invalid tokens

## Questions?

If you encounter issues not covered in this guide, contact the backend team or refer to the source code in the backend repository.
