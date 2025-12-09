# Mobile Verification & OTP Flows API Documentation

This document describes all mobile verification and OTP-based authentication flows in the XCentro backend.

---

## Table of Contents

1. [Regular Signup Flow](#1-regular-signup-flow)
2. [Business User Invite Flow](#2-business-user-invite-flow)
3. [Change Password Flow](#3-change-password-flow)
4. [API Endpoints Reference](#4-api-endpoints-reference)

---

## 1. Regular Signup Flow

### Overview
When a user signs up, they verify their email and then add their mobile number for OTP verification.

### Flow Diagram
```
User Signs Up → Email Sent → User Clicks Email Link →
Frontend Captures Mobile → Backend Verifies Email + Sends OTP →
User Enters OTP → Backend Verifies Mobile
```

### Step-by-Step Integration

#### Step 1: User Signs Up
**Endpoint:** `POST /v1/auth/signup`
```json
Request:
{
  "email": "user@example.com",
  "password": "SecureP@ss123"
}

Response (200):
{
  "message": "User registered successfully. Please verify your email.",
  "data": {
    "userId": "01234567-89ab-cdef-0123-456789abcdef"
  }
}
```

#### Step 2: User Clicks Email Verification Link
Email contains link: `https://app.xcentro.in/verify-email?uid=<userId>&token=<token>&sig=<signature>`

#### Step 3: Verify Email + Capture Mobile + Send OTP
**Endpoint:** `POST /v1/auth/verify-email-with-mobile`
```json
Request:
{
  "uid": "01234567-89ab-cdef-0123-456789abcdef",
  "token": "abc123...",
  "sig": "def456...",
  "mobileNumber": "9876543210"
}

Response (200):
{
  "message": "Email verified and OTP sent to mobile number",
  "data": null
}
```

#### Step 4: Verify Mobile OTP
**Endpoint:** `POST /v1/auth/verify-mobile-otp`
```json
Request:
{
  "uid": "01234567-89ab-cdef-0123-456789abcdef",
  "otp": "123456"
}

Response (200):
{
  "message": "Mobile number verified successfully",
  "data": null
}
```

---

## 2. Business User Invite Flow

### Overview
When a business owner invites a user, the user receives an email with a password reset link. The user then clicks "Generate OTP", enters password + OTP to complete setup.

### Flow Diagram
```
Owner Invites User (with mobile) → Email Sent →
User Clicks Link → User Clicks "Generate OTP" → OTP Sent →
User Enters Password + OTP → Backend Verifies Both → Account Activated
```

### Step-by-Step Integration

#### Step 1: Business Owner Invites User
**Endpoint:** `POST /v1/business/:bid/invite`
**Authentication:** Required (JWT)

```json
Request:
{
  "email": "newuser@example.com",
  "name": "New User",
  "mobile": "9876543210",
  "role": "FINANCE"
}

Response (201):
{
  "message": "User invited successfully. Invitation email sent.",
  "data": {
    "userId": "01234567-89ab-cdef-0123-456789abcdef",
    "email": "newuser@example.com",
    "role": "FINANCE"
  }
}
```

#### Step 2: User Clicks Email Link
Email contains link: `https://app.xcentro.in/invite-reset?uid=<userId>&token=<token>&sig=<signature>`

Frontend displays form with:
- Password field
- OTP field
- "Generate OTP" button

#### Step 3: User Clicks "Generate OTP"
**Endpoint:** `POST /v1/auth/send-invite-otp` (PUBLIC)

```json
Request:
{
  "uid": "01234567-89ab-cdef-0123-456789abcdef",
  "token": "abc123...",
  "sig": "def456..."
}

Response (200):
{
  "message": "OTP sent successfully to your registered mobile number",
  "data": null
}
```

#### Step 4: User Enters Password + OTP and Submits
**Endpoint:** `POST /v1/auth/reset-password-with-otp` (PUBLIC)

```json
Request:
{
  "uid": "01234567-89ab-cdef-0123-456789abcdef",
  "token": "abc123...",
  "sig": "def456...",
  "otp": "123456",
  "newPassword": "NewSecureP@ss456"
}

Response (200):
{
  "message": "Password reset and mobile verified successfully. All existing sessions have been invalidated.",
  "data": null
}
```

---

## 3. Change Password Flow

### Overview
When a logged-in user wants to change their password, they must verify with OTP for security.

### Flow Diagram
```
User Navigates to Change Password → User Clicks "Send OTP" → OTP Sent →
User Enters Old Password + New Password + OTP → Backend Verifies All →
Password Changed + Sessions Invalidated
```

### Step-by-Step Integration

#### Step 1: User Clicks "Send OTP"
**Endpoint:** `POST /v1/auth/generate-mobile-otp` (PROTECTED)
**Authentication:** Required (JWT)

```json
Request: (No body, userID from JWT)

Response (200):
{
  "message": "OTP sent successfully to your registered mobile number",
  "data": null
}
```

#### Step 2: User Enters Old Password + New Password + OTP
**Endpoint:** `POST /v1/auth/change-password` (PROTECTED)
**Authentication:** Required (JWT)

```json
Request:
{
  "oldPassword": "OldSecureP@ss123",
  "newPassword": "NewSecureP@ss456",
  "otp": "123456"
}

Response (200):
{
  "message": "Password changed successfully. All existing sessions have been invalidated.",
  "data": null
}
```

**Important:** After successful password change:
- Clear all JWT tokens from frontend storage
- Redirect user to login page

---

## 4. API Endpoints Reference

### POST /v1/auth/signup
**Authentication:** None
**Description:** Register a new user account

**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "SecureP@ss123"
}
```

**Responses:**
- **200 OK:** User created, verification email sent
- **400 Bad Request:** Validation error or weak password
- **409 Conflict:** User already exists

---

### POST /v1/auth/verify-email-with-mobile
**Authentication:** None (validates signature)
**Description:** Verify email, capture mobile number, and send OTP

**Request Body:**
```json
{
  "uid": "user-id",
  "token": "verification-token",
  "sig": "hmac-signature",
  "mobileNumber": "9876543210"
}
```

**Validations:**
- `mobileNumber`: 10-digit Indian mobile number

**Responses:**
- **200 OK:** Email verified, OTP sent to mobile
- **400 Bad Request:** Invalid token, expired token, or validation error
- **403 Forbidden:** Invalid signature (tampered link)

---

### POST /v1/auth/verify-mobile-otp
**Authentication:** None
**Description:** Verify mobile OTP

**Request Body:**
```json
{
  "uid": "user-id",
  "otp": "123456"
}
```

**Validations:**
- `otp`: Exactly 6 numeric digits

**Responses:**
- **200 OK:** Mobile verified successfully
- **400 Bad Request:** Invalid OTP, expired OTP, or already used
- **404 Not Found:** OTP not found for user

---

### POST /v1/auth/send-invite-otp
**Authentication:** None (validates signature)
**Description:** Send OTP to invited user's registered mobile number

**Request Body:**
```json
{
  "uid": "user-id",
  "token": "reset-token",
  "sig": "hmac-signature"
}
```

**Responses:**
- **200 OK:** OTP sent successfully
- **403 Forbidden:** Invalid signature (tampered link)
- **404 Not Found:** User has no mobile number registered
- **500 Internal Server Error:** Failed to send SMS

---

### POST /v1/auth/reset-password-with-otp
**Authentication:** None (validates signature)
**Description:** Reset password and verify mobile using OTP (for invited users)

**Request Body:**
```json
{
  "uid": "user-id",
  "token": "reset-token",
  "sig": "hmac-signature",
  "otp": "123456",
  "newPassword": "NewSecureP@ss456"
}
```

**Validations:**
- `otp`: Exactly 6 numeric digits
- `newPassword`: Strong password (min 8 chars, uppercase, lowercase, number, special char)

**Responses:**
- **200 OK:** Password reset and mobile verified, all sessions invalidated
- **400 Bad Request:** Invalid token, expired token, invalid OTP, or validation error
- **403 Forbidden:** Invalid signature (tampered link)

---

### POST /v1/auth/generate-mobile-otp
**Authentication:** Required (JWT)
**Description:** Generate and send OTP to logged-in user's mobile number

**Request:** No body (userID extracted from JWT)

**Responses:**
- **200 OK:** OTP sent successfully
- **401 Unauthorized:** Invalid or missing JWT token
- **404 Not Found:** User has no mobile number registered
- **500 Internal Server Error:** Failed to send SMS

---

### POST /v1/auth/change-password
**Authentication:** Required (JWT)
**Description:** Change password with OTP verification

**Request Body:**
```json
{
  "oldPassword": "OldSecureP@ss123",
  "newPassword": "NewSecureP@ss456",
  "otp": "123456"
}
```

**Validations:**
- `oldPassword`: Required
- `newPassword`: Strong password (min 8 chars, uppercase, lowercase, number, special char)
- `otp`: Exactly 6 numeric digits

**Responses:**
- **200 OK:** Password changed, all sessions invalidated
- **400 Bad Request:** Invalid OTP, expired OTP, validation error, or new password same as old
- **401 Unauthorized:** Wrong old password or invalid JWT
- **500 Internal Server Error:** Server error

---

## Error Codes Summary

| Status Code | Scenario |
|-------------|----------|
| **200** | Success |
| **400** | Validation error, invalid OTP, expired token |
| **401** | Authentication failed (wrong password or invalid JWT) |
| **403** | Invalid HMAC signature (tampered link) |
| **404** | User not found or mobile number not registered |
| **409** | Duplicate user |
| **500** | Server error or SMS sending failed |

---

## OTP Specifications

- **Length:** 6 digits (numeric only)
- **Validity:** 10 minutes (configurable)
- **Storage:** Hashed using SHA-256
- **Reuse:** Each OTP can only be used once
- **Rate Limiting:** Applied on all OTP generation endpoints

---

## SMS Templates (Fast2SMS)

### Template 1: Signup Verification (204688)
```
Dear User, {OTP} is your verification code to sign up on Xcentro.
Do not share this code with anyone. Valid for 5 minutes.
```

### Template 2: Password Reset (204686)
```
Your OTP to reset your Xcentro account password is {OTP}.
Valid for 5 minutes. If you did not request this, please ignore this message.
```

### Template 3: Payment Success (204687) - Future Use
```
Dear Customer, payment of INR {Amount} has been transferred to {PayeeName} successfully.
If not done by you, contact Xcentro support. Thank you for using Xcentro.
```

---

## Security Considerations

### HMAC Signature Validation
All public endpoints with `uid`, `token`, and `sig` parameters validate the HMAC-SHA256 signature:
- **Data:** `userid:token`
- **Secret:** JWT secret key
- **Algorithm:** HMAC-SHA256
- **Encoding:** Hexadecimal

This prevents:
- URL tampering
- Replay attacks
- Unauthorized OTP requests

### Session Invalidation
After password change or reset:
- All refresh tokens are revoked from Redis
- User must log in again
- Frontend must clear stored JWT tokens and redirect to login

### Mobile Number Format
- Accepted formats: `9876543210` or `+919876543210`
- Cleaned before sending to Fast2SMS (removes `+91` prefix)
- E.164 format internally

---

## Testing Checklist

### Regular Signup Flow
- [ ] Sign up with email/password
- [ ] Receive email with verification link
- [ ] Click link and enter mobile number
- [ ] Receive OTP via SMS
- [ ] Enter OTP and verify mobile
- [ ] Handle expired OTP (wait 10+ minutes)
- [ ] Handle invalid OTP
- [ ] Handle already used OTP

### Business Invite Flow
- [ ] Business owner invites user with mobile
- [ ] Invited user receives email (no SMS yet)
- [ ] User clicks email link
- [ ] User clicks "Generate OTP" button
- [ ] User receives OTP via SMS
- [ ] User enters password + OTP and submits
- [ ] Password set and mobile verified
- [ ] Handle invalid signature
- [ ] Handle missing mobile number

### Change Password Flow
- [ ] Logged-in user navigates to change password
- [ ] User clicks "Send OTP"
- [ ] User receives OTP via SMS
- [ ] User enters old password + new password + OTP
- [ ] Password changed successfully
- [ ] User logged out (all sessions invalidated)
- [ ] Handle wrong old password
- [ ] Handle invalid OTP
- [ ] Handle expired OTP

---

## Troubleshooting

### OTP Not Received
1. Check SMS service logs for errors
2. Verify mobile number format (10 digits, no +91)
3. Check Fast2SMS API key and sender ID configuration
4. Verify template IDs are correct

### Invalid Signature Errors
1. Verify JWT secret key matches across all services
2. Check if URL parameters are being modified
3. Ensure `userid:token` format is correct

### Session Issues After Password Change
1. Ensure frontend clears all JWT tokens
2. Verify Redis connection for token revocation
3. Check if refresh token is being sent in subsequent requests

---

## Configuration

### Environment Variables
```bash
# Fast2SMS Configuration
FAST2SMS_API_KEY=your-api-key-here
FAST2SMS_SENDER_ID=XCNTRO
FAST2SMS_ENDPOINT=https://www.fast2sms.com/dev/bulkV2

# OTP Configuration
SMS_OTP_EXPIRY_MINUTES=10
SMS_OTP_LENGTH=6
SMS_OTP_EXPIRY=10m

# JWT Configuration (used for HMAC signatures)
JWT_SECRET_KEY=your-secret-key-here
```

---

## Support

For integration questions or issues, contact the backend team or refer to:
- Main API documentation
- CHANGE_PASSWORD_OTP_API.md
- API_REFERENCE.md
