# API Reference

Complete reference for all XCentro backend API endpoints.

## Base URL

```
Development: http://localhost:8080
Production: https://api.xcentro.in
```

All API endpoints require `Authorization: Bearer <JWT_TOKEN>` header unless specified otherwise.

---

## Authentication APIs (`/v1/auth`)

### User Registration
```
POST /v1/auth/signup
```
**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "Pass123!"
}
```
**Response:** Sends email verification link via AWS SES

### User Login
```
POST /v1/auth/login
```
**Request Body:**
```json
{
  "email": "user@example.com",
  "password": "Pass123!"
}
```
**Response:**
```json
{
  "data": {
    "token": "eyJhbGc...",
    "user_id": "uuid"
  }
}
```

### User Logout
```
POST /v1/auth/logout
```
**Headers:** `Authorization: Bearer <TOKEN>`
**Response:** Invalidates JWT in Redis

---

## User APIs (`/v1/user`)

### Get User Profile
```
GET /v1/user/profile
```
**Response:**
```json
{
  "user_id": "uuid",
  "email": "user@example.com",
  "name": "John Doe",
  "kyc_status": "kyc_completed"
}
```

### Update User Profile
```
PUT /v1/user/profile
```
**Request Body:**
```json
{
  "name": "John Doe",
  "phone": "+919876543210"
}
```

### Email Verification
```
GET /v1/user/verify-email?token=<TOKEN>
```
**Description:** Verifies email with SHA-256 hashed token (24-hour expiry, one-time use)

### Request Password Reset
```
GET /v1/user/forgot-password?email=user@example.com
```
**Description:** Sends reset token via email (always returns success for security)

### Reset Password
```
POST /v1/user/reset-password
```
**Request Body:**
```json
{
  "token": "reset-token",
  "newPassword": "NewPass123!"
}
```
**Description:** 1-hour token expiry

### Get User Roles
```
GET /v1/user/roles
```
**Response:**
```json
{
  "roles": [
    {
      "user_id": "uuid",
      "business_id": "uuid",
      "role": "owner"
    }
  ]
}
```

---

## User Onboarding (KYC) APIs (`/v1/user/onboard`)

### Initiate KYC
```
POST /v1/user/onboard/initiate
```
**Request Body:**
```json
{
  "name": "John Doe",
  "dob": "1990-01-15",
  "taxId": "ABCDE1234F",
  "nationalId": "123456789012"
}
```
**Response:**
```json
{
  "status": "kyc_in_progress",
  "aadhaarLink": "https://digilocker.gov.in/...",
  "txnId": "transaction-id"
}
```
**Description:** Verifies PAN, PAN-Aadhaar link, generates DigiLocker URL

### Complete KYC
```
POST /v1/user/onboard/complete
```
**Request Body:**
```json
{
  "businessId": "uuid-of-business",
  "claimedRole": "owner"
}
```
**Description:** Fetches Aadhaar data, stores verification, claims role in business

### Check KYC Status
```
GET /v1/user/onboard/status
```
**Response:**
```json
{
  "status": "kyc_completed",
  "verification_data": {...}
}
```

**KYC Status Flow:**
```
kyc_not_started → kyc_in_progress → kyc_completed → kyc_verified
```

---

## Business APIs (`/v1/business`)

### Create Business
```
POST /v1/business
```
**Request Body:**
```json
{
  "name": "Acme Corp",
  "gstin": "29ABCDE1234F1Z5"
}
```
**Response:** Creates business entity with support role for creator

### Get Business Details
```
GET /v1/business?bid=<BUSINESS_ID>
```

### Update KYB Information
```
POST /v1/business/kyb
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "business_type": "private_limited_company",
  "income_tax_id": "ITID123456",
  "company_id": "CIN12345"
}
```
**Business Types:** `sole_proprietorship`, `partnership`, `private_limited_company`, `trust`, `ngo`, `society`, etc.

### Get S3 Presigned Upload URL
```
GET /v1/business/kyb/upload?bid=<BUSINESS_ID>&docType=business_deed
```
**Response:**
```json
{
  "upload_url": "https://s3.amazonaws.com/...",
  "expiry": "15 minutes"
}
```

### Confirm Document Upload
```
POST /v1/business/kyb/confirm-upload
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "document_type": "business_deed"
}
```
**Response:** Returns download URL

### Invite User to Business
```
POST /v1/business/invite-user
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "email": "newuser@example.com",
  "name": "Jane Doe",
  "role": "finance",
  "kyc_needed": true
}
```
**Description:** Creates user, sends password via email, adds to business_user_relation

### List Business Users
```
GET /v1/business/users?bid=<BUSINESS_ID>
```

### Update User Role
```
POST /v1/business/users/update
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "user_id": "uuid",
  "new_role": "admin"
}
```
**Rules:** Admin cannot change/remove owner or promote to owner

### Remove User from Business
```
DELETE /v1/business/users
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "user_id": "uuid"
}
```

---

## Wallet Management APIs (`/v1/wallet`)

### Fetch Wallet
```
POST /v1/wallet/fetch
```
**Request Body:**
```json
{
  "business_id": "uuid"
}
```
**Response:**
```json
{
  "business_id": "uuid",
  "stored_cards": [{...}],
  "bank_accounts": [{...}],
  "upi_details": [],
  "credit_lines": []
}
```

### Add Payment Source
```
POST /v1/wallet/add_source
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "source_type": "bank_account",
  "bank_account_details": {
    "account_number": "1234567890123456",
    "ifsc_code": "HDFC0000123",
    "account_holder_name": "XCentro Pvt Ltd",
    "bank_name": "HDFC Bank",
    "branch_name": "MG Road",
    "account_type": "current"
  }
}
```
**Description:** Performs penny drop verification for bank accounts

---

## Payment Processing APIs (`/v1/process_payment`)

### Get Payment Sources with Fees
```
POST /v1/process_payment/get_payment_sources_with_fees
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "payment_context": "vendor_payment",
  "base_amount": 100000
}
```
**Payment Contexts:** `vendor_payment`, `payroll`, `gst_payment`

**Response:**
```json
{
  "cards": [{
    "card_id": "uuid",
    "fee_amount": 2500,
    "total_amount": 102500
  }],
  "bank_accounts": [{
    "account_id": "uuid",
    "fee_amount": 0,
    "total_amount": 100000
  }]
}
```

### Tokenize Card
```
POST /v1/process_payment/tokenize
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit"
  }
}
```
**Response:**
```json
{
  "token": "jio_tok_visa_abc123",
  "card_id": "uuid",
  "gateway_used": "jiopay"
}
```
**Description:** Auto-routes to JioPay or Easebuzz, stores card in database

### Make Payment
```
POST /v1/process_payment/make_payment
```
**Request Body (Card):**
```json
{
  "business_id": "uuid",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "card",
  "calling_device": "web",
  "card_details": {...},
  "voucher_ids": ["voucher-uuid-1", "voucher-uuid-2"],
  "description": "Payment for invoice #12345"
}
```

**Request Body (Token Card):**
```json
{
  "business_id": "uuid",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "token_card",
  "calling_device": "web",
  "token_card_details": {
    "card_id": "uuid"
  }
}
```

**Request Body (Net Banking):**
```json
{
  "business_id": "uuid",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "netbanking",
  "calling_device": "web",
  "net_banking": {
    "bank_code": "HDFC",
    "bank_name": "HDFC Bank"
  }
}
```

**Payment Methods:** `card`, `token_card`, `netbanking`, `upi`, `wallet`

**Calling Device:** `web` (default), `mobile`, `pos` - Backend generates device-specific callback URLs

**Response:**
```json
{
  "payment_id": "uuid",
  "status": "pending",
  "redirect_url": "https://gateway.com/auth?txnId=...",
  "message": "Redirect user to complete 3DS/OTP verification"
}
```

### Get Fee Rules
```
POST /v1/process_payment/get_fee_rules
```
**Request Body:**
```json
{
  "payment_context": "vendor_payment"
}
```

### Payment Status Webhook (JioPay)
```
POST /v1/process_payment/webhooks/payment_status?type=jiopay
```
**Description:** Real-time payment authorization status (after 3DS/OTP completion)
**Updates:** Payment status (success/failed/pending), initializes settlement_status='pending'

### Payment Status Webhook (Easebuzz)
```
POST /v1/process_payment/webhooks/payment_status?type=easebuzz
```
**Description:** Real-time payment authorization status
**Validation:** SHA-512 signature

### Settlement Status Webhook (JioPay)
```
POST /v1/process_payment/webhooks/settlement_status?type=jiopay
```
**Data Format:** JSON
**Processing:** Single transaction per webhook
**Timing:** T+1 to T+3 days after successful payment
**Validation:** HMAC-SHA256

**Financial Fields Updated:**
- `settlement_amount`: Original payment amount
- `mdr_amount`: Gateway fees (MDR + GST)
- `net_settlement_amount`: Final amount received
- `settled_at`: Settlement date
- `settlement_reference_id`: Settlement batch ID

### Settlement Status Webhook (Easebuzz)
```
POST /v1/process_payment/webhooks/settlement_status?type=easebuzz
```
**Data Format:** Form-urlencoded (parsed to JSON)
**Processing:** Batch (multiple transactions in array)
**Timing:** T+1 to T+3 days after successful payment
**Validation:** SHA-512

**Batch Processing:**
- Processes `settled_transactions[]` array
- Updates each payment's settlement data
- Returns summary: `{total_transactions, processed, skipped, errors}`

---

## Payee APIs (`/v1/payees`)

### Onboard Payee
```
POST /v1/payees
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "gstin": "29ABCDE1234F1Z5",
  "account_number": "1234567890123456",
  "ifsc_code": "HDFC0000123",
  "account_holder_name": "ABC Industries Pvt Ltd",
  "display_name": "ABC Corp"
}
```
**11-Step Flow:**
1. Validate input
2. Check payee exists by GSTIN
3. Verify GSTIN via AuthBridge
4. Compute account hash (SHA-256)
5. Check duplicate account
6. Penny drop verification
7. Cross-verify GST vs bank name
8. Anti-money rotation check
9. Atomic transaction
10. Return verification statuses
11. Audit trail

### List Payees
```
GET /v1/payees?bid=<BUSINESS_ID>
```

### Get Payee Account Details
```
GET /v1/payees/:payee_id/accounts/:account_id
```

### Update Payee Metadata
```
PUT /v1/payees/:payee_id/accounts/:account_id
```

### Soft Delete Payee Account
```
DELETE /v1/payees/:payee_id/accounts/:account_id
```

---

## Voucher APIs (`/v1/vouchers`)

### Create Draft Voucher
```
POST /v1/vouchers/draft
```
**Request Body:**
```json
{
  "business_id": "uuid",
  "payee_id": "uuid",
  "account_id": "uuid",
  "gross_amount": 50000,
  "net_amount": 47500,
  "tds_amount": 500,
  "tds_rate": 1.0,
  "tds_section": "194C",
  "gst_amount": 9000,
  "invoice_number": "INV-001",
  "invoice_date": "2025-01-10",
  "description": "Web development services"
}
```
**Response:**
```json
{
  "voucher_id": "uuid",
  "voucher_number": "PV-2025-00001",
  "status": "draft"
}
```

### Create Multiple Vouchers
```
POST /v1/vouchers/multi
```
**Request Body:** Array of voucher objects

### List Vouchers
```
GET /v1/vouchers?business_id=uuid&status=draft&page=1&limit=20
```
**Query Params:**
- `business_id` (required)
- `status` (optional): draft, finalized, posted, synced
- `payee_id` (optional)
- `from_date`, `to_date` (optional)
- `page`, `limit` (optional)

### Get Abandoned Draft Vouchers
```
GET /v1/vouchers/reminders
```
**Description:** Returns vouchers created but not paid within 7 days

### Get Voucher Details
```
GET /v1/vouchers/:id
```

### Manually Finalize Voucher
```
PUT /v1/vouchers/:id/finalize
```
**Description:** Rare use case - vouchers auto-finalize on payment success

**Voucher Status Flow:**
```
draft → finalized (auto on payment) → posted → synced
```

---

## Payout APIs (`/v1/payout`)

### Castler Payout Webhook
```
POST /v1/payout/webhooks/castler
```
**Description:** HMAC verified webhook from Castler

### Get Pending Approvals
```
GET /v1/payout/pending-approvals
```

### Approve Payout Transfer
```
POST /v1/payout/:transfer_id/approve
```

### Cancel Payout Transfer
```
POST /v1/payout/:transfer_id/cancel
```

### Get Transfer Details
```
GET /v1/payout/:transfer_id
```

### Get Webhook History
```
GET /v1/payout/:transfer_id/webhooks
```

### Get Timed-out Transfers
```
GET /v1/payout/timeouts
```

### Manually Mark as Completed
```
POST /v1/payout/:transfer_id/mark-completed
```

### Get Payout Configuration
```
GET /v1/payout/config
```

### Update Payout Configuration
```
PUT /v1/payout/config
```

### Create Payout Configuration
```
POST /v1/payout/config
```

---

## Third-Party Integrations

### AuthBridge (Document Verification)
**Supported Documents:** PAN, PAN_COM, PAN_GST, GSTIN, CIN, AADHAAR

**Aadhaar Flow:**
1. LINK action: Generate DigiLocker URL
2. User completes verification
3. STATUS action: Fetch verification data

**Encryption:** AES-256 encrypt/decrypt

### AWS SES (Email Service)
- Single and bulk emails
- HTML/text support
- Send quota monitoring
- Email verification templates

### AWS S3 (File Storage)
- Presigned URLs (15-min expiry)
- Server-side encryption (AES-256)
- Multipart upload for large files
- Bucket structure: `PII/{business_id}/`, `PII/{user_id}/`

---

## Error Responses

All API endpoints return errors in this format:

```json
{
  "status": "error",
  "message": "Error description",
  "error_code": "ERROR_CODE"
}
```

**Common HTTP Status Codes:**
- `200` - Success
- `400` - Bad Request (validation error)
- `401` - Unauthorized (invalid/missing JWT)
- `403` - Forbidden (insufficient permissions)
- `404` - Not Found
- `500` - Internal Server Error
