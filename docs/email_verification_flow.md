# Email Verification Flow

This document describes the email verification implementation for user registration.

## Flow Overview

1. **User Signup**: User provides email and password
2. **Account Creation**: User account is created with `email_verified: false`
3. **Email Sent**: Verification email with secure token is sent automatically
4. **Email Verification**: User clicks link in email to verify their account
5. **Login**: Only verified users can successfully log in and get JWT tokens

## API Endpoints

### 1. Sign Up
```
POST /v1/auth/signup
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Response (201):**
```json
{
  "message": "User created successfully. Please check your email for verification link.",
  "data": {
    "userId": "01234567-89ab-cdef-0123-456789abcdef"
  }
}
```

### 2. Verify Email
```
GET /v1/user/verify-email?token=<verification_token>
```

**Response (200):**
```json
{
  "message": "Email verified successfully. You can now log in.",
  "data": null
}
```

### 3. Resend Verification Email
```
POST /v1/user/resend-verification
Content-Type: application/json

{
  "email": "user@example.com"
}
```

**Response (200):**
```json
{
  "message": "Verification email sent successfully. Please check your inbox.",
  "data": null
}
```

### 4. Login (After Email Verification)
```
POST /v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "securepassword123"
}
```

**Response (200):**
```json
{
  "message": "Login successful",
  "data": {
    "token": "jwt-token-here"
  }
}
```

**Response (401 - Before Email Verification):**
```json
{
  "message": "Email not verified. Please verify your email before logging in.",
  "data": null
}
```

## Security Features

1. **Secure Token Generation**: Uses crypto/rand for 256-bit tokens
2. **Token Expiration**: Verification tokens expire after 24 hours
3. **Single Use**: Each token can only be used once
4. **Email Template**: Professional HTML/text email templates
5. **Rate Limiting**: Prevents abuse of verification email sending

## Database Schema

The implementation uses the existing tables:
- `xcentro.user_table`: Stores user data with `email_verified` boolean
- `xcentro.user_verification_codes`: Stores verification tokens with expiration

## Email Template

The verification email includes:
- Professional branding
- Clear call-to-action button
- Plain text fallback
- Expiration notice
- Security disclaimer

## Environment Variables Required

```bash
# SES Configuration
SES_DEFAULT_FROM_EMAIL=noreply@xcentro.com
SES_DEFAULT_FROM_NAME=Xcentro Team
SES_REGION=us-east-1

# Optional
SES_DEFAULT_CONFIG_SET=your-config-set
SES_ENDPOINT=https://email.amazonaws.com
```

## Testing the Flow

1. Set up AWS SES credentials or use localstack for testing
2. Start the server: `go run cmd/api/*.go`
3. Sign up with a test email
4. Check server logs for verification URL (if SES not configured)
5. Visit the verification URL
6. Try logging in with the verified account

## Error Handling

- **Invalid token**: 404 with "Invalid verification token"
- **Expired token**: 400 with "Verification token has expired"
- **Used token**: 400 with "Verification token has already been used"
- **User not found**: 404 with "User not found"
- **Already verified**: 400 with "Email is already verified"