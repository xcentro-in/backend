# Change Password with OTP Verification API

This document describes the updated change password flow with mobile OTP verification.

## Overview

The change password flow now requires **two-step verification**:
1. **Step 1:** Request OTP to be sent to registered mobile number
2. **Step 2:** Change password using old password + new password + OTP

---

## API Endpoints

### 1. Generate Mobile OTP

**Endpoint:** `POST /auth/generate-mobile-otp`

**Authentication:** Required (JWT Bearer Token)

**Description:** Generates and sends a 6-digit OTP to the user's registered mobile number. The OTP is valid for 10 minutes (configurable).

#### Request

**Headers:**
```
Authorization: Bearer <access_token>
Content-Type: application/json
```

**Body:** None (userID is extracted from JWT token)

#### Response

**Success Response (200 OK):**
```json
{
  "message": "OTP sent successfully to your registered mobile number",
  "data": null
}
```

**Error Responses:**

**401 Unauthorized** - Invalid or missing JWT token
```json
{
  "message": "Unauthorized",
  "data": null
}
```

**404 Not Found** - User doesn't have a mobile number registered
```json
{
  "message": "Mobile number not registered. Please update your profile.",
  "data": null
}
```

**500 Internal Server Error** - Failed to send OTP
```json
{
  "message": "Failed to send OTP",
  "data": null
}
```

---

### 2. Change Password with OTP

**Endpoint:** `POST /auth/change-password`

**Authentication:** Required (JWT Bearer Token)

**Description:** Changes the user's password after verifying the old password and mobile OTP. All existing sessions are invalidated after successful password change.

#### Request

**Headers:**
```
Authorization: Bearer <access_token>
Content-Type: application/json
```

**Body:**
```json
{
  "oldPassword": "OldP@ssw0rd123",
  "newPassword": "NewP@ssw0rd456",
  "otp": "123456"
}
```

**Field Validations:**
- `oldPassword` (string, required): User's current password
- `newPassword` (string, required): Must meet password strength requirements:
  - At least 8 characters long
  - Contains at least one uppercase letter
  - Contains at least one lowercase letter
  - Contains at least one number
  - Contains at least one special character
- `otp` (string, required): 6-digit numeric OTP received via SMS
  - Must be exactly 6 digits
  - Must be numeric

#### Response

**Success Response (200 OK):**
```json
{
  "message": "Password changed successfully. All existing sessions have been invalidated.",
  "data": null
}
```

**Error Responses:**

**400 Bad Request** - Validation failed
```json
{
  "message": "Validation failed",
  "errors": {
    "otp": "otp must be 6 characters in length",
    "newPassword": "newPassword must meet strong_password requirements"
  },
  "data": null
}
```

**400 Bad Request** - Invalid or expired OTP
```json
{
  "message": "invalid or expired OTP",
  "data": null
}
```

**400 Bad Request** - OTP already used
```json
{
  "message": "OTP already used",
  "data": null
}
```

**400 Bad Request** - OTP has expired
```json
{
  "message": "OTP has expired",
  "data": null
}
```

**400 Bad Request** - New password same as old
```json
{
  "message": "new password cannot be the same as old password",
  "data": null
}
```

**400 Bad Request** - Password strength validation failed
```json
{
  "message": "password must be at least 8 characters long",
  "data": null
}
```

or

```json
{
  "message": "password must contain at least one uppercase letter, one lowercase letter, one number, and one special character",
  "data": null
}
```

**401 Unauthorized** - Invalid JWT token
```json
{
  "message": "Unauthorized",
  "data": null
}
```

**401 Unauthorized** - Wrong old password
```json
{
  "message": "Current password is incorrect",
  "data": null
}
```

**500 Internal Server Error** - Server error
```json
{
  "message": "Failed to change password",
  "data": null
}
```

---

## Integration Flow

### Frontend Implementation Steps

1. **User initiates password change:**
   - User navigates to "Change Password" page
   - Display form with fields: Old Password, New Password, OTP

2. **Request OTP:**
   - Add a "Send OTP" button
   - On click, call `POST /auth/generate-mobile-otp` with JWT token
   - Show success message: "OTP sent to your registered mobile number"
   - Start a countdown timer (10 minutes)
   - Optionally, add "Resend OTP" functionality (can call the same endpoint again)

3. **User enters OTP and submits:**
   - User enters old password, new password, and OTP
   - Validate form fields on frontend:
     - Old password: required
     - New password: required, min 8 chars, strong password pattern
     - OTP: required, exactly 6 digits, numeric
   - Call `POST /auth/change-password` with all three fields
   - On success:
     - Show success message
     - Clear JWT tokens from storage
     - Redirect to login page (all sessions invalidated)

4. **Error handling:**
   - Display appropriate error messages based on response
   - For OTP errors (invalid/expired/used), allow user to request new OTP
   - For wrong old password, highlight the old password field
   - For password strength errors, show validation hints

---

## Example Usage

### Complete Flow Example (cURL)

**Step 1: Generate OTP**
```bash
curl -X POST https://api.xcentro.in/v1/auth/generate-mobile-otp \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json"
```

**Step 2: Change Password**
```bash
curl -X POST https://api.xcentro.in/v1/auth/change-password \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{
    "oldPassword": "OldP@ssw0rd123",
    "newPassword": "NewP@ssw0rd456",
    "otp": "123456"
  }'
```

---

## Complete Flow Example (JavaScript/Fetch)

```javascript
// Step 1: Generate and send OTP
async function requestOTP() {
  try {
    const response = await fetch('/v1/auth/generate-mobile-otp', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${getAccessToken()}`,
        'Content-Type': 'application/json'
      }
    });

    const data = await response.json();

    if (response.ok) {
      showSuccessMessage('OTP sent to your registered mobile number');
      startOTPTimer(600); // 10 minutes in seconds
    } else {
      showErrorMessage(data.message);
    }
  } catch (error) {
    showErrorMessage('Failed to send OTP');
  }
}

// Step 2: Change password with OTP
async function changePassword(oldPassword, newPassword, otp) {
  try {
    const response = await fetch('/v1/auth/change-password', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${getAccessToken()}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        oldPassword,
        newPassword,
        otp
      })
    });

    const data = await response.json();

    if (response.ok) {
      showSuccessMessage('Password changed successfully');
      // Clear tokens and redirect to login
      clearAuthTokens();
      window.location.href = '/login';
    } else {
      if (response.status === 401) {
        // Wrong old password or unauthorized
        showErrorMessage(data.message, 'oldPassword');
      } else if (data.message.includes('OTP')) {
        // OTP related error
        showErrorMessage(data.message, 'otp');
      } else {
        // Other errors
        showErrorMessage(data.message);
      }
    }
  } catch (error) {
    showErrorMessage('Failed to change password');
  }
}
```

---

## Important Notes

1. **OTP Expiry:** OTPs are valid for 10 minutes by default (configurable on backend)

2. **OTP Reuse:** Each OTP can only be used once. If verification fails, user must request a new OTP

3. **Session Invalidation:** After successful password change, ALL user sessions are invalidated. Frontend must:
   - Clear stored JWT tokens (access and refresh tokens)
   - Redirect user to login page

4. **Mobile Number Requirement:** User must have a mobile number registered in their profile to use this flow. If not registered, the endpoint returns 404

5. **Rate Limiting:** Both endpoints are rate-limited. Avoid excessive requests

6. **Security:**
   - Always use HTTPS for these endpoints
   - Never log or store OTP in frontend code
   - Clear OTP input field after submission

7. **Backward Compatibility:** This is a breaking change. The old change password endpoint (without OTP) will no longer work

---

## Testing Checklist

- [ ] Successfully request OTP with valid JWT
- [ ] Receive SMS with 6-digit OTP
- [ ] Successfully change password with correct old password + valid OTP
- [ ] Handle 401 error for wrong old password
- [ ] Handle 400 error for invalid OTP format
- [ ] Handle 400 error for expired OTP (test after 10 minutes)
- [ ] Handle 400 error for already used OTP (submit same OTP twice)
- [ ] Handle 404 error when user has no mobile number
- [ ] Verify all sessions are invalidated after password change
- [ ] Test OTP resend functionality
- [ ] Test with various password validation errors
- [ ] Test rate limiting behavior

---

## Migration Notes for Frontend Team

### Changes Required:

1. **Update Change Password Component:**
   - Add "Send OTP" button
   - Add OTP input field (6-digit numeric)
   - Add countdown timer for OTP expiry
   - Update form submission to include OTP

2. **Update API Service:**
   - Add `generateMobileOTP()` method
   - Update `changePassword()` to include OTP parameter

3. **Update Error Handling:**
   - Add handlers for new OTP-related errors
   - Update logout flow after successful password change

4. **Update User Flow:**
   - Add informational message about OTP requirement
   - Add guidance for users without registered mobile numbers

---

## Support

For questions or issues, contact the backend team or refer to the main API documentation.
