# Email and OTP Verification API Usage Guide

This document explains when and how each email verification and OTP API endpoint is used across different user flows.

**Last Updated**: 2026-02-16

---

## ⚠️ Important Changes

**Removed Endpoints** (mobile verification always required):
- ❌ `POST /v1/auth/verify-email` - No longer exists
  - **Why removed**: Mobile verification is always required, so email-only verification is not needed
  - **Use instead**: `POST /v1/auth/verify-email-with-mobile` (verifies email + captures mobile in one step)

---

## Table of Contents

1. [Signup Flow](#signup-flow)
2. [Login Flow](#login-flow)
3. [Forgot Password Flow](#forgot-password-flow)
4. [Change Password Flow](#change-password-flow)
5. [User Invite Flow](#user-invite-flow)
6. [API Endpoint Reference](#api-endpoint-reference)
7. [Rate Limiting](#rate-limiting)

---

## Signup Flow

**Purpose**: New user registration with email and mobile verification

### Steps

1. **User submits signup form**
   - `POST /v1/auth/signup`
   - **Body**: `{ email, password }`
   - **Returns**: Always 201 (even for existing users - prevents enumeration)
   - **What happens**:
     - **New user**: Creates account + sends email verification
     - **Existing unverified user**: Resends email verification
     - **Existing verified user**: ⚠️ **Returns success WITHOUT sending email** (prevents enumeration attack)

2. **User clicks email link and provides mobile number**
   - `POST /v1/auth/verify-email-with-mobile`
   - **Body**: `{ userID, token, signature, mobileNumber }`
   - **Returns**: OTP sent confirmation
   - **What happens**: Verifies email + sends OTP to mobile number

3. **User enters OTP**
   - `POST /v1/auth/verify-mobile-otp`
   - **Body**: `{ uid, otp }`
   - **Returns**: Mobile verification status
   - **What happens**: Verifies OTP and marks mobile as verified

### Resend Options

#### Resend Email Verification (Before Clicking Email Link)

**When**: User hasn't received the email verification link yet

**Endpoints**:
- `POST /v1/auth/signup` (same endpoint, idempotent) - **Recommended**
- OR `POST /v1/auth/resend-verification`

**Body**:
```json
{ "email": "user@example.com" }
```

**Rate Limit**: 1 per 60s + 10 per day

**⚠️ Important Behavior**:
- If email is **not verified**: Sends verification email
- If email is **already verified**: Returns 201 success WITHOUT sending email (security feature)

**Frontend Context**:
- User just signed up or is on signup page
- Frontend has: `email` from signup form (stored in localStorage)

---

#### Resend Mobile OTP (After Clicking Email Link)

**When**: User clicked email verification link, entered mobile, but didn't receive OTP

**Endpoint**:
- `POST /v1/auth/resend-mobile-otp`

**Body**:
```json
{ "email": "user@example.com" }
```

**Rate Limit**: 1 per 60s + 5 per day (stricter due to SMS cost)

**Frontend Context**:
- User is on mobile verification page (after clicking email link)
- URL has: `uid`, `token`, `signature` from email link
- Frontend has: `email` from localStorage (from signup)
- **Note**: Endpoint uses `email` (not `uid`) to lookup user - simpler for frontend

---

## Login Flow

**Purpose**: Existing user authentication

### Steps

1. **User submits login credentials**
   - `POST /v1/auth/login`
   - **Body**: `{ email, password }`
   - **Returns**: Access token + refresh token + onboarding flags
   - **Validation**:
     - Email must be verified
     - Mobile must be verified
     - If either is unverified, login fails with 403

2. **Token refresh**
   - `POST /v1/auth/refresh`
   - **Body**: `{ refreshToken }`
   - **Returns**: New access token

### No Resend APIs

Login **does not** support resend. If verification is incomplete:
- User must restart signup flow
- Use `/v1/auth/signup` (which handles resends automatically)

---

## Forgot Password Flow

**Purpose**: Password reset for users who forgot their password

### Steps

1. **User requests password reset**
   - `GET /v1/auth/forgot-password?email={email}`
   - **Returns**: Always 200 (prevents enumeration)
   - **What happens**:
     - If email exists: Sends reset link
     - If email doesn't exist: No email sent (silent)

2. **User clicks reset link in email**
   - `POST /v1/auth/reset-password`
   - **Body**: `{ userID, token, signature, newPassword }`
   - **Returns**: Password reset confirmation
   - **What happens**: Updates password, invalidates all sessions

### Resend Option

- **Resend Reset Link**:
  - `GET /v1/auth/forgot-password?email={email}` (same endpoint)
  - **Rate Limit**: 1 per 60s + 10 per day

### Notes

- **No mobile OTP required** for password reset (email verification only)
- This is intentional - users may have lost access to their mobile

---

## Change Password Flow

**Purpose**: Authenticated users changing their password (requires mobile OTP)

### Steps

1. **User requests OTP** (authenticated)
   - `POST /v1/auth/generate-mobile-otp`
   - **Headers**: `Authorization: Bearer {accessToken}`
   - **Returns**: OTP sent confirmation
   - **What happens**: Sends OTP to user's registered mobile

2. **User submits password change**
   - `POST /v1/auth/change-password`
   - **Headers**: `Authorization: Bearer {accessToken}`
   - **Body**: `{ oldPassword, newPassword, otp }`
   - **Returns**: Password change confirmation
   - **What happens**: Updates password, invalidates all sessions

### Resend Option

- **Resend OTP**:
  - `POST /v1/auth/generate-mobile-otp` (same endpoint)
  - **Rate Limit**: 1 per 60s + 5 per day

### Notes

- **Requires authentication** (must be logged in)
- **Requires mobile OTP** for security (prevents unauthorized password change)

---

## User Invite Flow

**Purpose**: Admin invites new user who sets password via OTP verification

### Steps

#### Admin Side (Authenticated)

1. **Admin creates invite**
   - **Endpoint**: `POST /v1/business/invite-user`
   - **Auth**: ✅ Required (JWT with admin/owner role)
   - **Headers**: `Authorization: Bearer {token}`, `X-Business-ID: {businessId}`
   - **Body**: `{ email, name, mobile, role }`
   - **Returns**: User created + invite email sent (background)
   - **What happens**: Creates user in business + sends invite email with reset link
   - **Link format**: `/set-password?uid={uid}&token={token}&sig={sig}`

2. **If email fails, admin resends** (**NEW FEATURE**)
   - **Endpoint**: `POST /v1/business/resend-invite`
   - **Auth**: ✅ Required (JWT with admin/owner role)
   - **Headers**: `Authorization: Bearer {token}`, `X-Business-ID: {businessId}`
   - **Body**: `{ email }`
   - **Returns**: Invite email resent
   - **What happens**:
     - Validates user belongs to business
     - Checks user hasn't activated (password not set)
     - Invalidates old invite tokens (transaction)
     - Generates new token + sends email

#### User Side (Public)

3. **User clicks invite link, requests OTP**
   - **Endpoint**: `POST /v1/auth/send-invite-otp`
   - **Auth**: ❌ Public (validates HMAC signature instead)
   - **Body**: `{ userID, token, signature }`
   - **Returns**: OTP sent confirmation
   - **What happens**: Validates invite link + sends OTP to pre-registered mobile

4. **User sets password with OTP**
   - **Endpoint**: `POST /v1/auth/reset-password-with-otp`
   - **Auth**: ❌ Public (validates HMAC signature + OTP)
   - **Body**: `{ userID, token, signature, otp, newPassword }`
   - **Returns**: Password set confirmation
   - **What happens**: Verifies OTP + sets password + marks mobile as verified

### Resend Options

#### Admin Resend Invite Email
- **When**: Email sending failed or user didn't receive invite link
- **Endpoint**: `POST /v1/business/resend-invite`
- **Auth**: ✅ Required (admin/owner)
- **Body**: `{ email }`
- **Rate Limit**: Inherited from default rate limiter (10 per hour)

#### User Resend Invite OTP
- **When**: User clicked invite link but didn't receive OTP
- **Endpoint**: `POST /v1/auth/resend-invite-otp`
- **Auth**: ❌ Public (HMAC signature)
- **Body**: `{ userID, token, signature }`
- **Rate Limit**: 1 per 60s + 5 per day

### Notes

- **Admin endpoints**: Authenticated, use business context (X-Business-ID header)
- **User endpoints**: Public, use HMAC signature validation for security
- **Mobile number**: Must be pre-registered by admin during invite creation
- **Email verification**: Not required (admin pre-verified the user)
- **Transaction safety**: Old invite tokens are atomically invalidated when resending

---

## API Endpoint Reference

### Email Verification Endpoints

| Endpoint | Type | Auth | Purpose |
|----------|------|------|---------|
| `POST /v1/auth/signup` | Public | No | Create account + send email verification |
| `POST /v1/auth/resend-verification` | Public | No | Resend email verification link |
| `GET /v1/auth/forgot-password` | Public | No | Request password reset email |
| `POST /v1/auth/reset-password` | Public | No | Reset password with email token |

### Mobile OTP Endpoints

| Endpoint | Type | Auth | Purpose |
|----------|------|------|---------|
| `POST /v1/auth/verify-email-with-mobile` | Public | No | Capture mobile + send OTP (signup) |
| `POST /v1/auth/resend-mobile-otp` | Public | No | Resend OTP during signup |
| `POST /v1/auth/verify-mobile-otp` | Public | No | Verify OTP |
| `POST /v1/auth/generate-mobile-otp` | Protected | Yes | Generate OTP (change password) |
| `POST /v1/auth/send-invite-otp` | Public | No | Send OTP for invited user |
| `POST /v1/auth/resend-invite-otp` | Public | No | Resend OTP for invited user |
| `POST /v1/auth/change-password` | Protected | Yes | Change password with OTP |
| `POST /v1/auth/reset-password-with-otp` | Public | No | Set password with OTP (invite) |

### Admin User Management Endpoints

| Endpoint | Type | Auth | Purpose |
|----------|------|------|---------|
| `POST /v1/business/invite-user` | Protected | Yes | Invite new user to business |
| `POST /v1/business/resend-invite` | Protected | Yes | Resend invite email to user |

---

## Rate Limiting

### Email Verification

- **Policy**: `resend_email_verification`
- **Limits**: 1 per 60 seconds + 10 per day
- **Scope**: Per-email
- **Fail Mode**: Fail-open (UX priority)
- **Applies to**:
  - `/v1/auth/signup`
  - `/v1/auth/resend-verification`
  - `/v1/auth/forgot-password`

### Mobile OTP (Public)

- **Policy**: `resend_mobile_otp`
- **Limits**: 1 per 60 seconds + 5 per day
- **Scope**: Per-email (signup) or per-user (invite)
- **Fail Mode**: Fail-closed (SMS cost control)
- **Applies to**:
  - `/v1/auth/resend-mobile-otp`
  - `/v1/auth/send-invite-otp`
  - `/v1/auth/resend-invite-otp`

### Mobile OTP (Authenticated)

- **Policy**: `resend_mobile_otp` (same as public)
- **Limits**: 1 per 60 seconds + 5 per day
- **Scope**: Per-user
- **Fail Mode**: Fail-closed (SMS cost control)
- **Applies to**:
  - `/v1/auth/generate-mobile-otp`

### OTP Verification

- **Policy**: `verify_mobile_otp`
- **Limits**: 5 attempts total, refills ~1 per hour
- **Scope**: Per-user
- **Fail Mode**: Fail-closed (security priority)
- **Applies to**:
  - `/v1/auth/verify-mobile-otp`

---

## Error Handling

### HTTP Status Codes

| Status | Meaning | User Action |
|--------|---------|-------------|
| **200** | Success | Proceed to next step |
| **201** | Created (signup) | Always returned for signup (prevents enumeration) |
| **400** | Invalid input / expired token | Request new verification |
| **401** | Unauthorized | Login again |
| **403** | Invalid/tampered link | Contact support |
| **404** | User/mobile not found | Check email or contact support |
| **429** | Rate limited | Wait {retry_after} seconds |
| **503** | Service unavailable | Infrastructure issue, try again later |

### Rate Limit Errors

**429 Response**:
```json
{
  "message": "too many resend requests. Please wait 60 seconds before trying again",
  "data": null
}
```

**UI Action**: Show countdown timer, disable resend button until cooldown expires

**503 Response**:
```json
{
  "message": "Service temporarily unavailable. Please try again in a few minutes.",
  "data": null
}
```

**UI Action**: Show error message, provide retry button

---

## API Response Codes by Endpoint

### User Invite Flow - Admin Endpoints

#### `POST /v1/business/invite-user` (Create Invite)

**Success Responses**:
```json
// 201 - User invited successfully
{
  "message": "User invited successfully. Invitation email sent.",
  "data": {
    "userId": "uuid-here",
    "email": "user@example.com",
    "role": "admin"
  }
}
```

**Error Responses**:
| Status | Error Code | Message | Frontend Action |
|--------|-----------|---------|-----------------|
| 400 | `MISSING_BUSINESS_ID` | X-Business-ID header is required | Show error, check headers |
| 400 | `INVALID_BUSINESS_ID` | X-Business-ID must be a valid UUID | Show error, validate business ID |
| 400 | `INVALID_INPUT` | Invalid request payload | Show validation errors |
| 400 | `VALIDATION_FAILED` | Validation failed | Show field-specific errors |
| 403 | `FORBIDDEN` | Only admin/owner can invite users | Show "Insufficient permissions" |
| 500 | `INTERNAL_ERROR` | Service error message | Show "Failed to invite user. Try again" |

---

#### `POST /v1/business/resend-invite` (Resend Invite Email)

**Success Responses**:
```json
// 200 - Invite email resent successfully
{
  "message": "Invitation email resent successfully",
  "data": {
    "email": "user@example.com"
  }
}
```

**Error Responses**:
| Status | Error Code | Message | Frontend Action |
|--------|-----------|---------|-----------------|
| 400 | `MISSING_BUSINESS_ID` | X-Business-ID header is required | Show error, check headers |
| 400 | `INVALID_BUSINESS_ID` | X-Business-ID must be a valid UUID | Show error, validate business ID |
| 400 | `INVALID_INPUT` | Invalid request payload | Show validation errors |
| 400 | `VALIDATION_FAILED` | Validation failed | Show field-specific errors (email required) |
| 400 | `ALREADY_ACTIVATED` | User has already activated their account | Show "User already activated. No resend needed." |
| 403 | `FORBIDDEN` | Only business owner or admin can resend invitations | Show "Insufficient permissions" |
| 404 | `USER_NOT_FOUND` | User not found in this business | Show "User not found. Check email or invite new user." |
| 500 | `INTERNAL_ERROR` | Failed to validate user | Show "Service error. Try again later." |
| 500 | `EMAIL_SEND_FAILED` | Failed to send invitation email. Please try again. | Show "Email sending failed. Try again." + retry button |

**Frontend Implementation Example**:
```typescript
async function resendInvite(businessId: string, email: string, accessToken: string) {
  const response = await fetch('/v1/business/resend-invite', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${accessToken}`,
      'X-Business-ID': businessId,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ email })
  });

  const data = await response.json();

  switch (response.status) {
    case 200:
      showSuccess("Invitation email resent successfully");
      break;

    case 400:
      if (data.error === 'ALREADY_ACTIVATED') {
        showInfo("User has already activated their account");
      } else {
        showError(data.message || "Invalid request");
      }
      break;

    case 403:
      showError("You don't have permission to resend invites");
      break;

    case 404:
      showError("User not found in this business");
      break;

    case 500:
      if (data.error === 'EMAIL_SEND_FAILED') {
        showError("Email sending failed. Please try again.", {
          showRetry: true,
          onRetry: () => resendInvite(businessId, email, accessToken)
        });
      } else {
        showError("Service error. Please try again later.");
      }
      break;

    default:
      showError("Unexpected error occurred");
  }
}
```

---

### User Invite Flow - User Endpoints

#### `POST /v1/auth/send-invite-otp` (Request OTP)

**Success Responses**:
```json
// 200 - OTP sent successfully
{
  "message": "OTP sent successfully to registered mobile number",
  "data": null
}
```

**Error Responses**:
| Status | Message | Frontend Action |
|--------|---------|-----------------|
| 400 | Invalid request payload | Show validation errors |
| 403 | Invalid invitation link | Show "Invalid link. Contact admin for new invite." |
| 404 | Mobile number not registered | Show "Contact admin to update mobile number" |
| 429 | Too many resend requests. Wait X seconds | Show countdown timer, disable resend button |
| 500 | Failed to send OTP | Show retry button |

---

#### `POST /v1/auth/resend-invite-otp` (Resend OTP)

**Success Responses**:
```json
// 200 - OTP resent successfully
{
  "message": "OTP resent successfully to registered mobile number",
  "data": null
}
```

**Error Responses**: Same as `send-invite-otp`

---

#### `POST /v1/auth/reset-password-with-otp` (Set Password)

**Success Responses**:
```json
// 200 - Password set successfully
{
  "message": "Password set successfully. Please login.",
  "data": null
}
```

**Error Responses**:
| Status | Message | Frontend Action |
|--------|---------|-----------------|
| 400 | Invalid OTP | Show "Invalid OTP. Try again or resend." |
| 400 | OTP expired | Show "OTP expired. Request new OTP." |
| 400 | Password validation error | Show password requirements |
| 403 | Invalid invitation link | Show "Invalid link. Contact admin." |
| 429 | Too many verification attempts | Show "Too many attempts. Try again later." |
| 500 | Server error | Show retry button |

---

## Security Features

### Email Enumeration Prevention

- **Signup** always returns 201 with same message (even for existing users)
  - ⚠️ **Critical**: If user is already verified, **NO email is sent** (only returns success message)
  - This prevents attackers from discovering valid emails
- **Forgot password** always returns 200 (no indication if email exists)
- **Resend endpoints** return same message regardless of user existence

### OTP Brute Force Protection

- Maximum 5 verification attempts per user
- Rate limit middleware enforced at handler level (defense-in-depth)
- Old OTP codes invalidated atomically when new ones are generated

### HMAC Signature Validation

- All verification links include HMAC signature
- Prevents tampering with uid/token parameters
- Uses constant-time comparison (timing attack protection)

### Code Invalidation

- Only latest verification code is valid (old codes marked as used)
- Atomically handled in database transaction (prevents race conditions)

---

## Frontend Data Flow & Parameters

### What Data Frontend Has at Each Step

| Step | Frontend Has | What to Send | Endpoint |
|------|-------------|--------------|----------|
| **1. Signup** | email, password (from form) | `{ email, password }` | POST /signup |
| **2. Email Link Clicked** | uid, token, sig (from URL params) + email (localStorage) | User enters mobile → `{ userID: uid, token, signature: sig, mobileNumber }` | POST /verify-email-with-mobile |
| **3. Resend Email** | email (localStorage) | `{ email }` | POST /resend-verification |
| **4. Resend OTP** | email (localStorage) | `{ email }` | POST /resend-mobile-otp |
| **5. Verify OTP** | uid (from URL or state) + otp (from user input) | `{ uid, otp }` | POST /verify-mobile-otp |

### Key Points for Frontend

1. **Store email in localStorage** during signup
   ```typescript
   localStorage.setItem('signup_email', email);
   ```

2. **Extract URL parameters** when user clicks email link
   ```typescript
   const params = new URLSearchParams(window.location.search);
   const uid = params.get('uid');
   const token = params.get('token');
   const sig = params.get('sig');
   ```

3. **Resend endpoints use email** (not uid)
   - Even though frontend has `uid` from URL, resend endpoints expect `email`
   - This is intentional for simplicity - email is always available from localStorage

4. **OTP verification uses uid** (not email)
   - Frontend should store `uid` from URL or previous API response
   - OTP verification is user-scoped (by uid)

---

## Frontend Integration Examples

### Signup with Resend

```typescript
// Initial signup
async function signup(email: string, password: string) {
  const response = await fetch('/v1/auth/signup', {
    method: 'POST',
    body: JSON.stringify({ email, password })
  });

  if (response.ok) {
    // Always show same message (security - prevents enumeration)
    // Note: If user is already verified, NO email is sent (but same message is shown)
    showMessage("Account created! Check your email for verification link.");
  } else if (response.status === 429) {
    showMessage("Too many requests. Try again in 60 seconds.");
  }
}

// Resend (Option 1: Use signup endpoint - idempotent)
async function resendEmailVerification(email: string) {
  return signup(email, "dummy-password-not-used");
  // Backend handles existing users automatically:
  // - Unverified user: Sends email ✓
  // - Verified user: Returns success WITHOUT sending email (security)
}

// Resend (Option 2: Use dedicated endpoint)
async function resendEmailVerification(email: string) {
  const response = await fetch('/v1/auth/resend-verification', {
    method: 'POST',
    body: JSON.stringify({ email })
  });

  if (response.status === 429) {
    const data = await response.json();
    showCooldown(60); // Start 60s countdown
  }
}
```

### OTP Input with Resend

```typescript
// Show OTP input with countdown
function OTPInput() {
  const [canResend, setCanResend] = useState(false);
  const [countdown, setCountdown] = useState(60);

  useEffect(() => {
    if (countdown > 0) {
      const timer = setTimeout(() => setCountdown(countdown - 1), 1000);
      return () => clearTimeout(timer);
    } else {
      setCanResend(true);
    }
  }, [countdown]);

  async function resendOTP() {
    const response = await fetch('/v1/auth/resend-mobile-otp', {
      method: 'POST',
      body: JSON.stringify({ email })
    });

    if (response.ok) {
      setCountdown(60);
      setCanResend(false);
    }
  }

  return (
    <>
      <input type="text" placeholder="Enter OTP" />
      <button disabled={!canResend} onClick={resendOTP}>
        {canResend ? "Resend OTP" : `Resend in ${countdown}s`}
      </button>
    </>
  );
}
```

---

## Troubleshooting

### User not receiving emails

**For Signup/Password Reset**:
1. Check spam folder
2. Use resend endpoint (respects rate limits)
3. Verify email service status (AWS SES)

**For User Invites**:
1. Admin can resend invite using `POST /v1/business/resend-invite`
2. Check spam folder
3. Verify email service status (AWS SES)
4. If user already activated, admin cannot resend (user should use forgot-password instead)

### User not receiving OTP

1. Check mobile number format (must be valid Indian number)
2. Use resend OTP endpoint (respects rate limits)
3. Verify SMS service status (Fast2SMS)
4. Check rate limit hasn't been exceeded (5 per day limit)

### Rate limit exceeded

- Wait for cooldown period (60 seconds)
- Check daily limit not exceeded (10/day for email, 5/day for SMS)
- Frontend should show countdown timer

### OTP verification failing

- OTP expires after 10 minutes
- Maximum 5 attempts allowed
- Old OTPs are invalidated when new ones are generated
- Request new OTP if expired

---

## Summary

| Flow | Email API | OTP API | Resend Email | Resend OTP |
|------|-----------|---------|--------------|------------|
| **Signup** | `/signup` | `/verify-email-with-mobile` | `/signup` (idempotent) | `/resend-mobile-otp` |
| **Login** | N/A | N/A | N/A | N/A |
| **Forgot Password** | `/forgot-password` | N/A | `/forgot-password` | N/A |
| **Change Password** | N/A | `/generate-mobile-otp` | N/A | `/generate-mobile-otp` |
| **User Invite (Admin)** | `/business/invite-user` | N/A | `/business/resend-invite` ✨ | N/A |
| **User Invite (User)** | (Auto-sent) | `/send-invite-otp` | N/A | `/resend-invite-otp` |

---

## Quick Reference: All Active Endpoints

### Signup & Email Verification
```
POST /v1/auth/signup                    → Create account + send email
POST /v1/auth/resend-verification       → Resend email verification (uses email)
POST /v1/auth/verify-email-with-mobile  → Verify email + capture mobile + send OTP
POST /v1/auth/resend-mobile-otp         → Resend OTP during signup (uses email)
POST /v1/auth/verify-mobile-otp         → Verify OTP (uses uid)
```

### Login
```
POST /v1/auth/login                     → Login with email + password
POST /v1/auth/refresh                   → Refresh access token
POST /v1/auth/logout                    → Logout
```

### Password Management
```
GET  /v1/auth/forgot-password           → Request reset link (uses email)
POST /v1/auth/reset-password            → Reset password with token
POST /v1/auth/change-password           → Change password (authenticated)
POST /v1/auth/generate-mobile-otp       → Get OTP for password change (uses uid)
```

### User Invite (Admin)
```
POST /v1/business/invite-user           → Create invite (authenticated, admin/owner)
POST /v1/business/resend-invite         → Resend invite email (authenticated, admin/owner)
```

### User Invite (User)
```
POST /v1/auth/send-invite-otp           → Send OTP to invited user
POST /v1/auth/resend-invite-otp         → Resend invite OTP (uses uid + token + sig)
POST /v1/auth/reset-password-with-otp   → Set password with OTP
```

### Parameter Reference
- **Uses `email`**: signup, resend-verification, resend-mobile-otp, forgot-password, **resend-invite (admin)**
- **Uses `uid`**: verify-mobile-otp, generate-mobile-otp (authenticated)
- **Uses `uid + token + sig`**: verify-email-with-mobile, resend-invite-otp, send-invite-otp
- **Requires `X-Business-ID` header**: invite-user, resend-invite (admin endpoints)

---

## What Happens When User is Already Verified?

### Email Already Verified

| Endpoint | Behavior | HTTP Status | Notes |
|----------|----------|-------------|-------|
| `/signup` | ⚠️ Returns success **WITHOUT sending email** | 201 | Security: Prevents enumeration |
| `/resend-verification` | Returns error | 400 | "Email already verified" |
| `/verify-email-with-mobile` | Still works (if mobile not verified) | 200 | Sends OTP to mobile |

### Mobile Already Verified

| Endpoint | Behavior | HTTP Status | Notes |
|----------|----------|-------------|-------|
| `/resend-mobile-otp` | Returns error | 400 | "Mobile already verified" |
| `/verify-mobile-otp` | Returns error | 400 | "Mobile already verified" |
| `/generate-mobile-otp` | Still works (for password change) | 200 | Authenticated endpoint |

### Both Email and Mobile Verified

**Result**: User can login successfully. All verification endpoints return errors except:
- ✅ `/signup` - Returns 201 **WITHOUT sending email** (prevents enumeration)
- ✅ `/generate-mobile-otp` - Works for authenticated password change

---

## Related Documentation

- [Security Fixes Summary](./SECURITY-FIXES-SUMMARY.md)
- [Security Audit and Implementation Plan](./security-audit-and-implementation-plan.md)
- [Rate Limiting Configuration](../internal/shared/ratelimiter/README.md)
