# Integration Examples

Complete code examples and integration flows for XCentro backend.

---

## User Signup → KYC → Business Creation

Complete flow from user registration to business setup:

```bash
# Step 1: User Signup
POST /v1/auth/signup
{
  "email": "user@example.com",
  "password": "Pass123!"
}

# Step 2: Verify Email (click link in email)
GET /v1/user/verify-email?token=<TOKEN_FROM_EMAIL>

# Step 3: Login
POST /v1/auth/login
{
  "email": "user@example.com",
  "password": "Pass123!"
}
# Response:
{
  "data": {
    "token": "eyJhbGc..."
  }
}

# Step 4: Initiate User Onboarding (Identity Verification)
POST /v1/user/onboard/initiate
Authorization: Bearer <TOKEN>
{
  "name": "John Doe",
  "dob": "1990-01-15",
  "taxId": "ABCDE1234F",
  "nationalId": "123456789012"
}
# Response:
{
  "status": "kyc_in_progress",
  "aadhaarLink": "https://digilocker.gov.in/...",
  "txnId": "..."
}

# Step 5: User completes DigiLocker verification (external action)
# User clicks aadhaarLink and completes verification

# Step 6: Complete Onboarding & Claim Role in Business
POST /v1/user/onboard/complete
Authorization: Bearer <TOKEN>
{
  "businessId": "uuid-of-business",
  "claimedRole": "owner"
}

# Step 7: Create Business (if needed)
POST /v1/business
Authorization: Bearer <TOKEN>
{
  "name": "Acme Corp",
  "gstin": "29ABCDE1234F1Z5"
}
```

---

## Token-Based Card Payment Flow

Complete flow for saving a card and making payments:

### Step 1: Tokenize Card (Auto-routes to Gateway)

```bash
POST /v1/process_payment/tokenize
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit",
    "last_four_digits": "1111"
  }
}
```

**Response:**
```json
{
  "data": {
    "token": "jio_tok_visa_abc123",
    "card_id": "card_uuid_001",
    "last_four_digits": "1111",
    "card_type": "visa",
    "card_category": "credit",
    "expiry_month": "12",
    "expiry_year": "25",
    "gateway_used": "jiopay",
    "status": "success"
  }
}
```

**Gateway Auto-routing:**
- Visa credit → JioPay
- Amex → Easebuzz
- Card automatically stored in database

### Step 2: Make Payment with Tokenized Card

```bash
POST /v1/process_payment/make_payment
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "token_card",
  "calling_device": "web",
  "token_card_details": {
    "card_id": "card_uuid_001"
  },
  "description": "Payment for invoice #12345"
}
```

**Response:**
```json
{
  "message": "Payment initiated successfully",
  "endpoint": "POST /v1/process_payment/make_payment",
  "status": "success",
  "data": {
    "payment_id": "pay_uuid_001",
    "status": "pending",
    "amount": 50000,
    "redirect_url": "https://uat.jiopay.co.in/tsp/pg/api/v2/authRedirect?txnId=pay_uuid_001",
    "message": "Redirect user to complete 3DS/OTP verification"
  }
}
```

### Step 3: User Completes 3DS/OTP

User is redirected to `redirect_url` and completes authentication on gateway's page.

### Step 4: Payment Webhook (Automatic)

Backend receives webhook from gateway:

```bash
POST https://yourapp.com/v1/process_payment/webhooks/payment_status?type=jiopay
{
  "payment_id": "pay_uuid_001",
  "status": "success",
  "gateway_payment_id": "jio_pay_789",
  "amount": "51250"
}
```

Backend automatically updates payment status to "success".

---

## Direct Card Payment Flow (No Tokenization)

One-time payment without saving card:

```bash
POST /v1/process_payment/make_payment
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "card",
  "calling_device": "web",
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "25",
    "cvv": "123",
    "holder_name": "John Doe",
    "card_type": "visa",
    "card_category": "credit"
  },
  "description": "One-time payment"
}
```

**Gateway Auto-routing:**
- Visa Credit → JioPay
- Amex → Easebuzz
- Debit cards → Based on amount and type

---

## Net Banking Payment Flow

```bash
POST /v1/process_payment/make_payment
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "amount": 50000,
  "currency": "INR",
  "payment_method": "netbanking",
  "calling_device": "web",
  "net_banking": {
    "bank_code": "HDFC",
    "bank_name": "HDFC Bank"
  },
  "description": "Net banking payment"
}
```

**Response:**
```json
{
  "payment_id": "uuid",
  "redirect_url": "https://uat.jiopay.co.in/pg/netbanking?txnId=...",
  "status": "pending"
}
```

User completes authentication on bank's net banking portal.

---

## Get Payment Sources with Fee Calculation

Retrieve all payment sources with calculated fees for specific context:

```bash
POST /v1/process_payment/get_payment_sources_with_fees
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "payment_context": "vendor_payment",
  "base_amount": 100000
}
```

**Response:**
```json
{
  "data": {
    "cards": [
      {
        "card_id": "card_uuid_001",
        "last_four_digits": "1111",
        "card_type": "visa",
        "card_category": "credit",
        "gateway_used": "jiopay",
        "fee_amount": 2500,
        "total_amount": 102500
      }
    ],
    "bank_accounts": [
      {
        "account_id": "bank_uuid_001",
        "last_five_digits": "67890",
        "bank_name": "HDFC Bank",
        "fee_amount": 0,
        "total_amount": 100000
      }
    ]
  }
}
```

**Note:** Returns only real database data (no mock data).

---

## Wallet Management

### Fetch Wallet

```bash
POST /v1/wallet/fetch
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business"
}
```

**Response:**
```json
{
  "data": {
    "business_id": "uuid-of-business",
    "stored_cards": [
      {
        "card_id": "card_uuid_001",
        "last_four_digits": "1111",
        "card_type": "visa",
        "card_category": "credit",
        "expiry_month": "12",
        "expiry_year": "25",
        "is_active": true
      }
    ],
    "bank_accounts": [
      {
        "account_id": "bank_uuid_001",
        "last_five_digits": "67890",
        "bank_name": "HDFC Bank",
        "ifsc_code": "HDFC0000123",
        "account_type": "current",
        "is_verified": true,
        "is_active": true
      }
    ],
    "upi_details": [],
    "credit_lines": []
  }
}
```

### Add Bank Account to Wallet

```bash
POST /v1/wallet/add_source
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
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

**Response (after penny drop verification):**
```json
{
  "status": "success",
  "data": {
    "success": true,
    "message": "Bank account verified and added successfully",
    "source_id": "bank_uuid_002",
    "source_type": "bank_account",
    "verification_status": "verified"
  }
}
```

---

## Payee Onboarding

```bash
POST /v1/payees
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid-of-business",
  "gstin": "29ABCDE1234F1Z5",
  "account_number": "1234567890123456",
  "ifsc_code": "HDFC0000123",
  "account_holder_name": "ABC Industries Pvt Ltd",
  "display_name": "ABC Corp"
}
```

**11-Step Verification Process:**
1. Validate input (GSTIN, account, IFSC)
2. Check if payee exists by GSTIN
3. If new: Verify GSTIN via AuthBridge GST API
4. Compute account hash (SHA-256)
5. Check duplicate account for payee
6. Penny drop verification
7. Cross-verify: GST name vs bank account holder
8. Anti-money rotation: Check payee holder ≠ customer director
9. Atomic transaction: Create account + relation
10. Return verification statuses
11. Audit trail logging

**Response:**
```json
{
  "status": "success",
  "data": {
    "payee_id": "uuid",
    "account_id": "uuid",
    "verification_status": {
      "gstin_verified": true,
      "bank_verified": true,
      "name_match": true,
      "anti_money_rotation_passed": true
    }
  }
}
```

---

## Voucher-Based Payment Flow (Multi-Bill Payment)

Complete flow for paying multiple vendors with a single payment transaction.

### Scenario: Pay 2 Vendors with Single Payment

#### Step 1: Create Draft Vouchers for Each Payee

**Voucher 1: ABC Industries**
```bash
POST /v1/vouchers/draft
Authorization: Bearer <TOKEN>
{
  "business_id": "business-uuid-123",
  "payee_id": "payee-abc-uuid",
  "account_id": "account-abc-uuid",
  "gross_amount": 50000,
  "net_amount": 47500,
  "tds_amount": 500,
  "tds_rate": 1.0,
  "tds_section": "194C",
  "gst_amount": 9000,
  "invoice_number": "INV-ABC-001",
  "invoice_date": "2025-01-10",
  "description": "Web development services - January 2025"
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Draft voucher created successfully",
  "data": {
    "voucher_id": "voucher-abc-uuid-001",
    "voucher_number": "PV-2025-00001",
    "status": "draft",
    "payee_id": "payee-abc-uuid",
    "gross_amount": 50000,
    "net_amount": 47500
  }
}
```

**Voucher 2: XYZ Corp**
```bash
POST /v1/vouchers/draft
Authorization: Bearer <TOKEN>
{
  "business_id": "business-uuid-123",
  "payee_id": "payee-xyz-uuid",
  "account_id": "account-xyz-uuid",
  "gross_amount": 50000,
  "net_amount": 47500,
  "tds_amount": 500,
  "tds_rate": 1.0,
  "tds_section": "194C",
  "gst_amount": 9000,
  "invoice_number": "INV-XYZ-002",
  "invoice_date": "2025-01-12",
  "description": "Consulting services - January 2025"
}
```

**Response:**
```json
{
  "status": "success",
  "data": {
    "voucher_id": "voucher-xyz-uuid-002",
    "voucher_number": "PV-2025-00002",
    "status": "draft"
  }
}
```

#### Step 2: Initiate Single Payment for Both Vouchers

```bash
POST /v1/process_payment/make_payment
Authorization: Bearer <TOKEN>
{
  "amount": 95000,
  "currency": "INR",
  "payment_method": "netbanking",
  "calling_device": "web",
  "net_banking": {
    "bank_code": "HDFC",
    "bank_name": "HDFC Bank"
  },
  "voucher_ids": [
    "voucher-abc-uuid-001",
    "voucher-xyz-uuid-002"
  ],
  "description": "Payment for INV-ABC-001 and INV-XYZ-002"
}
```

**Response:**
```json
{
  "message": "Payment initiated successfully",
  "status": "success",
  "data": {
    "payment_id": "payment-uuid-789",
    "status": "pending",
    "amount": 95000,
    "redirect_url": "https://uat.jiopay.co.in/pg/netbanking?txnId=payment-uuid-789",
    "message": "Redirect user to complete net banking authentication"
  }
}
```

#### Step 3: User Completes Net Banking Authentication

User is redirected to bank's net banking portal → Authenticates → Approves payment

#### Step 4: Payment Success Webhook (Automatic)

```bash
POST https://yourapp.com/v1/process_payment/webhooks/payment_status?type=jiopay
{
  "payment_id": "payment-uuid-789",
  "status": "success"
}
```

**Backend automatically:**
1. Updates `payment.status = "success"`
2. Finalizes both vouchers:
   - `voucher-abc-uuid-001.status = "finalized"`
   - `voucher-xyz-uuid-002.status = "finalized"`
3. Links vouchers to payment:
   - `voucher.payment_id = "payment-uuid-789"`

#### Step 5: Settlement (T+2 days later) - Automatic

```bash
POST https://yourapp.com/v1/process_payment/webhooks/settlement_status?type=jiopay
{
  "merchantTxnNo": "payment-uuid-789",
  "txnAmount": "950.00",
  "settlementID": "SETTLE-12345"
}
```

**Backend updates:**
- `payment.settlement_status = "settled"`
- `payment.settled_at = "2025-01-20"`

#### Step 6: Payout Worker Triggers (Automatic)

Payout worker runs every 1 minute:
1. Finds finalized vouchers with settled payments
2. Creates 2 payout transfer requests:
   - Transfer 1: ₹475 to ABC Industries
   - Transfer 2: ₹475 to XYZ Corp
3. Initiates transfers via Castler API
4. Tracks status via payout webhooks

#### Step 7: Payouts Complete (T+3 to T+5)

**Payout Webhook 1:**
```json
{
  "transfer_id": "transfer-abc-001",
  "status": "completed",
  "utr": "HDFC202501201234567"
}
```

**Payout Webhook 2:**
```json
{
  "transfer_id": "transfer-xyz-002",
  "status": "completed",
  "utr": "HDFC202501201234568"
}
```

**Final State:**
- Voucher PV-2025-00001: `payout_status = "completed"`, UTR = HDFC202501201234567
- Voucher PV-2025-00002: `payout_status = "completed"`, UTR = HDFC202501201234568
- ABC Industries received ₹475
- XYZ Corp received ₹475

### Complete Timeline

```
Day 0:  Create vouchers (draft)
Day 0:  Initiate payment with voucher_ids → Redirect to bank
Day 0:  User completes net banking → Payment success webhook → Vouchers finalized
Day 2:  Settlement webhook → payment.settlement_status = "settled"
Day 2:  Payout worker → Creates 2 transfer requests
Day 2:  Castler API → Initiates 2 payouts
Day 3-5: Payout webhooks → Payees receive funds
```

### Key Benefits

- ✅ Single payment transaction → Multiple payees
- ✅ Automatic voucher finalization (no manual step)
- ✅ Automatic payout triggering (worker-based)
- ✅ Full audit trail (payment → vouchers → payouts)
- ✅ Individual UTR tracking per payee

---

## Settlement Webhook Flow (JioPay)

Timeline: Payment → Settlement (T+1 to T+3 days)

### Day 0: Payment Completed

```
payment.status = 'success'
payment.settlement_status = 'pending'
payment.total_amount = 51250 (₹512.50 with fees)
```

### Day 2: Settlement Webhook Arrives

```bash
POST https://yourapp.com/v1/process_payment/webhooks/settlement_status?type=jiopay
{
  "aggregatorID": "A001",
  "merchantId": "123243",
  "merchantTxnNo": "pay_uuid_001",
  "invoiceNos": "INV12345",
  "paymentMode": "CARD",
  "paymentSubInstType": "CC",
  "txnChannel": "WEB",
  "txnAmount": "512.50",
  "txnCharges": "12.81",
  "serviceTax": "2.31",
  "settlementID": "A00121-20251013-010",
  "settlementDate": "20251013",
  "settlementAccount": "9876543210987654321",
  "settlementAccountIFSC": "HDFC0001",
  "settledAmount": "497.38",
  "cardNetwork": "VISA",
  "addlParam1": "SettlementInfo",
  "secureHash": "39867b8ef38a2853aacb84bbf5f927a72b27bdb301365245564f012d20dbc2f2"
}
```

### System Processing

1. Validate HMAC-SHA256 signature
2. Find payment by `merchantTxnNo`
3. Verify `payment.status = 'success'` and `settlement_status = 'pending'`
4. Parse and convert amounts:
   - `settlement_amount = 51250` paise (₹512.50 × 100)
   - `mdr_amount = 1512` paise [(₹12.81 + ₹2.31) × 100]
   - `net_settlement_amount = 49738` paise (₹497.38 × 100)
5. Update database:
   - `settlement_status = 'settled'`
   - `settlement_amount = 51250`
   - `mdr_amount = 1512`
   - `net_settlement_amount = 49738`
   - `settled_at = 2025-10-13`
   - `settlement_reference_id = "A00121-20251013-010"`
   - `settlement_metadata = {full webhook payload as JSONB}`

### Financial Summary

```
Customer Paid:     ₹512.50 (including payment processing fee)
Gateway Settled:   ₹512.50 (same as payment amount)
Gateway MDR:       ₹15.12  (2.5% + 18% GST)
Merchant Received: ₹497.38 (final amount in bank account)
```

---

## Settlement Webhook Flow (Easebuzz - Batch Processing)

Timeline: Multiple Payments → Batch Settlement (T+1 to T+3 days)

### Day 0: Multiple Payments Completed

**Payment 1:**
```
payment.id = "pay_uuid_001", txnid = "ORDER123"
payment.status = 'success'
payment.settlement_status = 'pending'
payment.total_amount = 100000 (₹1000.00)
```

**Payment 2:**
```
payment.id = "pay_uuid_002", txnid = "ORDER124"
payment.status = 'success'
payment.settlement_status = 'pending'
payment.total_amount = 50000 (₹500.00)
```

### Day 2: Batch Settlement Webhook Arrives

```bash
POST https://yourapp.com/v1/process_payment/webhooks/settlement_status?type=easebuzz
Content-Type: application/x-www-form-urlencoded

data={
  "status": 1,
  "data": {
    "hash": "abc123...def789",
    "paid_out": 1,
    "payout_id": "PT1LCHKBAB",
    "payout_date": "2025-10-13 14:30:00.000000",
    "bank_transaction_id": "UTR202510131430001",
    "bank_name": "HDFC Bank",
    "account_number": "50200012345678",
    "ifsc_code": "HDFC0001234",
    "name_on_bank": "XCentro Private Limited",
    "total_amount": 1500.00,
    "payout_amount": 1455.30,
    "refund_amount": 0.00,
    "service_charge_amount": 37.50,
    "service_tax_amount": 7.20,
    "settled_transactions": [
      {
        "txnid": "ORDER123",
        "easepayid": "EZBPAY789012",
        "transaction_amount": 1000.00,
        "service_charge": 25.00,
        "service_tax": 4.50,
        "settlement_amount": 970.50,
        "transaction_type": "CARD",
        "submerchant_id": null,
        "split_transactions": []
      },
      {
        "txnid": "ORDER124",
        "easepayid": "EZBPAY789013",
        "transaction_amount": 500.00,
        "service_charge": 12.50,
        "service_tax": 2.25,
        "settlement_amount": 485.25,
        "transaction_type": "UPI",
        "submerchant_id": null,
        "split_transactions": []
      }
    ],
    "refund_transactions": [],
    "split_payouts": [],
    "submerchant_payouts": []
  }
}
```

### System Processing (Batch Loop)

For each transaction in `settled_transactions[]`:

**Transaction 1 (ORDER123):**
1. Parse form-urlencoded data to JSON
2. Validate `webhook.status = 1`
3. Hash validation (SHA-512)
4. Find payment by `txnid: "ORDER123"` → `payment_id "pay_uuid_001"`
5. Verify `payment.status = 'success'` and `settlement_status = 'pending'`
6. Convert amounts from float64 to paise:
   - `settlement_amount = 100000` paise (1000.00 × 100)
   - `mdr_amount = 2950` paise [(25.00 + 4.50) × 100]
   - `net_settlement_amount = 97050` paise (970.50 × 100)
7. Update database
8. Increment `success_count`

**Transaction 2 (ORDER124):**
Same processing steps

**Response (after batch processing):**
```json
{
  "status": "success",
  "message": "Batch settlement processed",
  "summary": {
    "total_transactions": 2,
    "processed": 2,
    "skipped": 0,
    "errors": 0
  }
}
```

### Financial Summary

**Transaction 1:**
```
Customer Paid:     ₹1000.00
Gateway Settled:   ₹1000.00
Gateway MDR:       ₹29.50
Merchant Received: ₹970.50
```

**Transaction 2:**
```
Customer Paid:     ₹500.00
Gateway Settled:   ₹500.00
Gateway MDR:       ₹14.75
Merchant Received: ₹485.25
```

**Batch Total:**
```
Total Settled:     ₹1500.00
Total MDR:         ₹44.25
Total Received:    ₹1455.75
```

---

## Password Reset Flow

### Request Password Reset

```bash
GET /v1/user/forgot-password?email=user@example.com
```

**Response:** Always returns success for security (even if email doesn't exist)
**Email sent:** Contains reset token (1-hour expiry)

### Reset Password with Token

```bash
POST /v1/user/reset-password
{
  "token": "reset-token-from-email",
  "newPassword": "NewPass123!"
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Password reset successfully"
}
```

**Token expires:** 1 hour after generation

---

## Business User Management

### Invite User to Business

```bash
POST /v1/business/invite-user
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid",
  "email": "newuser@example.com",
  "name": "Jane Doe",
  "role": "finance",
  "kyc_needed": true
}
```

**Backend automatically:**
1. Creates user account
2. Generates random password
3. Sends password via email
4. Adds user to `business_user_relation` with specified role

### Update User Role

```bash
POST /v1/business/users/update
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid",
  "user_id": "uuid",
  "new_role": "admin"
}
```

**Authorization Rules:**
- Admin cannot change/remove owner
- Admin cannot promote to owner
- Only owner can perform these actions

### Remove User from Business

```bash
DELETE /v1/business/users
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid",
  "user_id": "uuid"
}
```

---

## Document Upload Flow (KYB)

### Step 1: Get Presigned Upload URL

```bash
GET /v1/business/kyb/upload?bid=<BUSINESS_ID>&docType=business_deed
Authorization: Bearer <TOKEN>
```

**Response:**
```json
{
  "upload_url": "https://s3.amazonaws.com/xcentro-pii-documents/PII/business-uuid/business_deed_123.pdf?X-Amz-...",
  "expiry": "15 minutes",
  "document_type": "business_deed"
}
```

### Step 2: Upload File to S3

```bash
PUT <upload_url>
Content-Type: application/pdf

<binary file data>
```

**Upload from client-side (JavaScript example):**
```javascript
const response = await fetch(uploadUrl, {
  method: 'PUT',
  body: file,
  headers: {
    'Content-Type': file.type
  }
});
```

### Step 3: Confirm Upload

```bash
POST /v1/business/kyb/confirm-upload
Authorization: Bearer <TOKEN>
{
  "business_id": "uuid",
  "document_type": "business_deed"
}
```

**Response:**
```json
{
  "status": "success",
  "download_url": "https://s3.amazonaws.com/...",
  "document_verified": true
}
```

---

## Mock Bank Verification (Penny Drop)

Mock service for testing with pattern-based responses:

**Last 4 digits → Result:**
- `****0000` → ACCOUNT_CLOSED
- `****1111` → INVALID_ACCOUNT
- `****2222` → NAME_MISMATCH
- `****9999` → API_TIMEOUT
- **Other** → Success

**Example:**
```bash
Account: 1234567890120000 → ACCOUNT_CLOSED
Account: 1234567890121111 → INVALID_ACCOUNT
Account: 1234567890125678 → Success
```
