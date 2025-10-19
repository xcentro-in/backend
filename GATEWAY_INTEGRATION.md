# Payment Gateway Integration Guide

Technical details for integrating with JioPay and Easebuzz payment gateways.

---

## Gateway Routing Logic

The payment orchestrator automatically selects the appropriate gateway based on payment method and card type.

### JioPay Routes
- All Net Banking transactions
- All UPI transactions
- All Wallet transactions
- Credit Cards: Visa, MasterCard, RuPay
- All Corporate Credit Cards
- Debit Cards: Visa, MasterCard, RuPay (≥ ₹2000)

### Easebuzz Routes
- Credit Cards: Amex, Diners
- Debit Cards: Visa, MasterCard (< ₹2000)
- Debit Cards: Amex, Diners (all amounts)

### Special Rules
- Visa/MasterCard Debit ≥ ₹2000 → JioPay
- RuPay Debit (all amounts) → JioPay

---

## JioPay Integration

**Base URL:** `https://uat.jiopay.co.in`
**Tokenization URL:** `https://uat.jiopay.co.in/qatsp/api/vault`

### Implementation Status
- ✅ **Tokenization**: POST to Vault API `/fetch-card` with 5 mandatory headers
- ✅ **Cryptogram**: POST to Vault API `/getCryptogram` with 5 mandatory headers
- ✅ **Payment**: POST to `/pg/api/v2/initiateSale` with HMAC-SHA256 hash
- ✅ **Payment Webhook**: HMAC-SHA256 signature validation
- ✅ **Settlement Webhook**: HMAC-SHA256 signature validation
- ✅ **Settlement Financial Tracking**: settlement_amount, mdr_amount, net_settlement_amount

### Vault API Headers (Tokenization & Cryptogram)

**5 Mandatory Headers:**
1. `Content-Type: application/json`
2. `x-app-id`: App identifier from config
3. `x-timestamp-id`: Format `yyyyMMddHHmmss` (e.g., "20250610125921")
4. `x-trace-id`: Uppercase UUID without hyphens
5. `x-app-token`: AES-128-ECB encrypted token (Base64)

### x-app-token Generation

```
Step 1: Concatenate
    x-app-id + "$" + x-trace-id + "$" + x-timestamp-id

Step 2: Encrypt
    AES-128-ECB with 16-byte shared key (JIOPAY_ENCRYPTION_KEY)

Step 3: Encode
    Base64 encode the encrypted bytes
```

**Example:**
```go
plaintext := "APP123$ABCD1234EFGH5678$20250610125921"
encrypted := AES128ECB_Encrypt(plaintext, encryptionKey)
xAppToken := Base64Encode(encrypted)
```

**IMPORTANT:** Encryption key must be exactly 16 bytes for AES-128.

### Payment API HMAC-SHA256 Hash

**Hash Generation:**
```
Step 1: Sort all request fields alphabetically by key
Step 2: Concatenate all VALUES in sorted order (no separators)
Step 3: HMAC-SHA256(apiKey, concatenated_string)
Step 4: Convert to lowercase hex string (64 characters)
```

**Example:**
```go
request := map[string]string{
    "merchantId": "123243",
    "amount": "500.00",
    "txnId": "ORDER12345",
    "currency": "INR",
}

// Sort keys alphabetically
sortedKeys := []string{"amount", "currency", "merchantId", "txnId"}

// Concatenate values
concatenated := "500.00INR123243ORDER12345"

// Generate HMAC-SHA256
hash := HMAC_SHA256(apiKey, concatenated)
secureHash := strings.ToLower(hex.EncodeToString(hash))
```

### Tokenization Flow (JioPay)

**Endpoint:** `POST /qatsp/api/vault/fetch-card`

**Headers:**
```
Content-Type: application/json
x-app-id: <APP_ID>
x-timestamp-id: 20250610125921
x-trace-id: ABCD1234EFGH5678IJKL9012MNOP3456
x-app-token: <BASE64_ENCRYPTED_TOKEN>
```

**Request Body:**
```json
{
  "cardNumber": "4111111111111111",
  "cardExpiryMonth": "12",
  "cardExpiryYear": "25",
  "nameOnCard": "John Doe"
}
```

**Response:**
```json
{
  "status": "SUCCESS",
  "token": "jio_tok_visa_abc123",
  "cardId": "card_12345",
  "lastFourDigits": "1111"
}
```

### Cryptogram Generation (JioPay)

**Endpoint:** `POST /qatsp/api/vault/getCryptogram`

**Headers:** Same 5 mandatory headers as tokenization

**Request Body:**
```json
{
  "token": "jio_tok_visa_abc123",
  "cardId": "card_12345"
}
```

**Response:**
```json
{
  "status": "SUCCESS",
  "cryptogram": "AJK3MN45PQR67STU89VWX"
}
```

### Payment Initiation (JioPay)

**Endpoint:** `POST /pg/api/v2/initiateSale`

**Request Body:**
```json
{
  "merchantId": "123243",
  "txnId": "ORDER12345",
  "amount": "500.00",
  "currency": "INR",
  "paymentMode": "CARD",
  "cardToken": "jio_tok_visa_abc123",
  "cryptogram": "AJK3MN45PQR67STU89VWX",
  "returnUrl": "https://api.xcentro.in/v1/process_payment/callback?payment_id=uuid&source=web",
  "secureHash": "abc123...def789"
}
```

**secureHash:** HMAC-SHA256 of all fields (see hash generation above)

### Payment Webhook (JioPay)

**Endpoint:** `POST /v1/process_payment/webhooks/payment_status?type=jiopay`

**Webhook Payload:**
```json
{
  "merchantId": "123243",
  "merchantTxnNo": "ORDER12345",
  "jiopayTxnId": "JIO789012",
  "status": "SUCCESS",
  "amount": "500.00",
  "currency": "INR",
  "paymentMode": "CARD",
  "responseCode": "0000",
  "responseMessage": "Transaction Successful",
  "secureHash": "xyz789...abc123"
}
```

**Signature Validation:**
1. Extract all fields except `secureHash`
2. Sort fields alphabetically by key
3. Concatenate values
4. Compute HMAC-SHA256(webhookSecret, concatenated)
5. Compare with `secureHash`

### Settlement Webhook (JioPay)

**Endpoint:** `POST /v1/process_payment/webhooks/settlement_status?type=jiopay`

**Data Format:** JSON (single transaction per webhook)

**Webhook Payload:**
```json
{
  "aggregatorID": "A001",
  "merchantId": "123243",
  "merchantTxnNo": "ORDER12345",
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

**Financial Fields:**
- `txnAmount`: Original payment amount (₹512.50)
- `txnCharges`: MDR base fee (₹12.81 = 2.5% of 512.50)
- `serviceTax`: GST on MDR (₹2.31 = 18% of 12.81)
- `settledAmount`: Net amount (₹497.38 = 512.50 - 12.81 - 2.31)

**Amount Conversion (Rupees → Paise):**
```go
settlementAmount := int64(txnAmount * 100)           // 51250 paise
mdrAmount := int64((txnCharges + serviceTax) * 100)  // 1512 paise
netSettlement := int64(settledAmount * 100)          // 49738 paise
```

**Signature Validation:**
Same HMAC-SHA256 process as payment webhook

---

## Easebuzz Integration

**Base URL:** `https://pay.easebuzz.in`

### Implementation Status
- ✅ **Tokenization**: POST to TMS API `/v1/create_token` with full payload encryption
- ✅ **Cryptogram**: POST to TMS API `/v1/get_cryptogram` with full payload encryption
- ✅ **Payment**: POST to `/initiate_seamless_payment/` with field-level encryption
- ✅ **Payment Webhook**: SHA-512 signature validation
- ✅ **Settlement Webhook**: Batch processing with form-urlencoded parsing
- ✅ **Settlement Financial Tracking**: settlement_amount, mdr_amount, net_settlement_amount

### TMS API Headers (Tokenization & Cryptogram)

**Required Headers:**
- `Content-Type: application/json`
- `EBZ-KEY`: API key from config (EASEBUZZ_API_KEY)

### Encryption Methods

Easebuzz uses **two different encryption methods**:

#### 1. Full Payload Encryption (Tokenization & Cryptogram)

Encrypt the entire JSON payload with AES-256-CBC:

```go
payload := `{"card_number":"4111111111111111","card_exp_month":"12","card_exp_year":"25"}`
encryptedPayload := AES256_CBC_Encrypt(payload, key, iv)
base64Encrypted := Base64Encode(encryptedPayload)
```

**Request:**
```json
{
  "data": "<BASE64_ENCRYPTED_PAYLOAD>"
}
```

#### 2. Field-Level Encryption (Payment Initiation)

Encrypt specific fields individually:

```go
encryptedToken := AES256_CBC_Encrypt(cardToken, key, iv)
encryptedCryptogram := AES256_CBC_Encrypt(cryptogram, key, iv)
encryptedCVV := AES256_CBC_Encrypt(cvv, key, iv)

request := map[string]string{
    "card_token": Base64Encode(encryptedToken),
    "cryptogram": Base64Encode(encryptedCryptogram),
    "cvv": Base64Encode(encryptedCVV),
    // ... other unencrypted fields
}
```

### AES-256 Key Derivation

```go
// Key: SHA-256 hash of merchant key → 32 bytes
key := SHA256(merchantKey)

// IV: First 16 bytes of SHA-256 hash of salt
ivFull := SHA256(salt)
iv := ivFull[:16]
```

**Example:**
```go
merchantKey := "ABC123XYZ789"
salt := "SALT456DEF012"

keyHash := SHA256(merchantKey)      // 32 bytes
key := keyHash                      // Use all 32 bytes

ivHash := SHA256(salt)              // 32 bytes
iv := ivHash[:16]                   // Use first 16 bytes
```

### Tokenization Flow (Easebuzz)

**Endpoint:** `POST /v1/create_token`

**Headers:**
```
Content-Type: application/json
EBZ-KEY: <EASEBUZZ_API_KEY>
```

**Request Body (plaintext):**
```json
{
  "card_number": "4111111111111111",
  "card_exp_month": "12",
  "card_exp_year": "25",
  "name_on_card": "John Doe",
  "merchant_key": "ABC123XYZ789"
}
```

**Encrypt entire payload:**
```go
plaintextJSON := `{"card_number":"4111111111111111",...}`
encrypted := AES256_CBC_Encrypt(plaintextJSON, key, iv)
base64Encrypted := Base64Encode(encrypted)
```

**Actual Request:**
```json
{
  "data": "<BASE64_ENCRYPTED_JSON>"
}
```

**Response (encrypted):**
```json
{
  "data": "<BASE64_ENCRYPTED_RESPONSE>"
}
```

**Decrypt Response:**
```go
decrypted := AES256_CBC_Decrypt(Base64Decode(response.Data), key, iv)
```

**Decrypted Response:**
```json
{
  "status": 1,
  "data": {
    "token": "ebz_tok_visa_xyz789",
    "card_id": "card_67890",
    "last_four_digits": "1111"
  }
}
```

### Cryptogram Generation (Easebuzz)

**Endpoint:** `POST /v1/get_cryptogram`

**Headers:** Same as tokenization

**Request Body (plaintext):**
```json
{
  "token": "ebz_tok_visa_xyz789",
  "card_id": "card_67890",
  "merchant_key": "ABC123XYZ789"
}
```

**Encryption:** Full payload encryption (same as tokenization)

**Response (decrypted):**
```json
{
  "status": 1,
  "data": {
    "cryptogram": "MNO8PQR23STU45VWX"
  }
}
```

### Payment Initiation (Easebuzz)

**Endpoint:** `POST /initiate_seamless_payment/`

**Request Body (field-level encryption):**
```json
{
  "merchant_key": "ABC123XYZ789",
  "txnid": "ORDER12345",
  "amount": "500.00",
  "productinfo": "Payment",
  "firstname": "John",
  "email": "john@example.com",
  "phone": "9876543210",
  "surl": "https://api.xcentro.in/v1/process_payment/callback?payment_id=uuid&source=web",
  "furl": "https://api.xcentro.in/v1/process_payment/callback?payment_id=uuid&source=web&status=failed",
  "hash": "<SHA512_HASH>",
  "card_token": "<BASE64_ENCRYPTED_TOKEN>",
  "cryptogram": "<BASE64_ENCRYPTED_CRYPTOGRAM>",
  "cvv": "<BASE64_ENCRYPTED_CVV>"
}
```

**Field-Level Encryption:**
Only `card_token`, `cryptogram`, and `cvv` are encrypted individually.

**Hash Generation:**
```go
hashString := merchantKey + "|" + txnid + "|" + amount + "|" + productinfo + "|" +
              firstname + "|" + email + "|||||||||||" + salt
hash := SHA512(hashString)
hashHex := hex.EncodeToString(hash)
```

### Payment Webhook (Easebuzz)

**Endpoint:** `POST /v1/process_payment/webhooks/payment_status?type=easebuzz`

**Webhook Payload (form-urlencoded):**
```
txnid=ORDER12345&
amount=500.00&
status=success&
firstname=John&
email=john@example.com&
phone=9876543210&
productinfo=Payment&
hash=<SHA512_HASH>&
easepayid=EZBPAY123456
```

**Hash Validation:**
```go
// Reverse order for webhook hash
hashString := salt + "|" + status + "|||||||||||" + email + "|" + firstname + "|" +
              productinfo + "|" + amount + "|" + txnid + "|" + merchantKey
hash := SHA512(hashString)
```

### Settlement Webhook (Easebuzz)

**Endpoint:** `POST /v1/process_payment/webhooks/settlement_status?type=easebuzz`

**Data Format:** Form-urlencoded (requires parsing to JSON)

**Processing Model:** Batch (multiple transactions in `settled_transactions[]` array)

**Webhook Payload:**
```
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
        "transaction_type": "CARD"
      },
      {
        "txnid": "ORDER124",
        "easepayid": "EZBPAY789013",
        "transaction_amount": 500.00,
        "service_charge": 12.50,
        "service_tax": 2.25,
        "settlement_amount": 485.25,
        "transaction_type": "UPI"
      }
    ]
  }
}
```

**Processing Logic:**
```go
for _, txn := range settledTransactions {
    // 1. Find payment by txnid
    payment := findPaymentByTxnID(txn.TxnID)

    // 2. Verify not already settled
    if payment.SettlementStatus == "settled" {
        skip++
        continue
    }

    // 3. Convert amounts to paise
    settlementAmount := int64(txn.TransactionAmount * 100)
    mdrAmount := int64((txn.ServiceCharge + txn.ServiceTax) * 100)
    netSettlement := int64(txn.SettlementAmount * 100)

    // 4. Update database
    updatePayment(payment.ID, settlementAmount, mdrAmount, netSettlement)

    processed++
}
```

**Response:**
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

---

## Key Differences: JioPay vs Easebuzz

| Feature | JioPay | Easebuzz |
|---------|--------|----------|
| **Tokenization Encryption** | AES-128-ECB (headers only) | AES-256-CBC (full payload) |
| **Payment Hash** | HMAC-SHA256 | SHA-512 |
| **Webhook Signature** | HMAC-SHA256 | SHA-512 |
| **Settlement Format** | JSON | Form-urlencoded |
| **Settlement Processing** | Single transaction | Batch (array) |
| **Amount Format** | Decimal strings ("500.00") | Numbers (500.00 as float64) |
| **Date Format** | YYYYMMDD | YYYY-MM-DD HH:MM:SS |
| **Headers (Tokenization)** | 5 mandatory custom headers | EBZ-KEY only |

---

## Environment Variables

### JioPay Configuration

```bash
JIOPAY_BASE_URL=https://uat.jiopay.co.in
JIOPAY_TOKENIZATION_BASE_URL=https://uat.jiopay.co.in/qatsp/api/vault
JIOPAY_MERCHANT_ID=your_merchant_id
JIOPAY_API_KEY=your_api_key                    # For HMAC-SHA256 hash generation
JIOPAY_APP_ID=your_app_id                      # For x-app-id header (Vault APIs)
JIOPAY_ENCRYPTION_KEY=your_16byte_key!!        # Must be exactly 16 bytes for AES-128
JIOPAY_WEBHOOK_SECRET=your_webhook_secret
```

**IMPORTANT:** `JIOPAY_ENCRYPTION_KEY` must be exactly 16 bytes (16 characters for ASCII).

### Easebuzz Configuration

```bash
EASEBUZZ_BASE_URL=https://pay.easebuzz.in
EASEBUZZ_MERCHANT_KEY=your_merchant_key
EASEBUZZ_SALT=your_salt_key
EASEBUZZ_API_KEY=your_api_key                  # For EBZ-KEY header (TMS APIs)
EASEBUZZ_ENCRYPTION_KEY=your_encryption_key    # Optional (derived from merchant_key)
EASEBUZZ_ENCRYPTION_IV=your_encryption_iv      # Optional (derived from salt)
EASEBUZZ_WEBHOOK_SECRET=your_webhook_secret
```

**Note:** If `EASEBUZZ_ENCRYPTION_KEY` and `EASEBUZZ_ENCRYPTION_IV` are not provided, they are automatically derived from `EASEBUZZ_MERCHANT_KEY` and `EASEBUZZ_SALT` using SHA-256.

---

## Context & Timeouts

| Operation | Timeout | Notes |
|-----------|---------|-------|
| Payment | 30s | Standard transactions |
| Tokenization | 60s | JioPay Vault API, Easebuzz TMS API |
| Cryptogram | 30s | Generated on-demand |
| Bank Verification | 30s | Penny drop |

---

## Amount Handling

- **Internal Storage**: All amounts in paise (smallest unit for INR)
- **Example**: ₹500.00 = 50000 paise
- **JioPay Expects**: "500.00" (rupees with decimals as string)
- **Easebuzz Expects**: "500.00" (rupees with decimals as string)
- **Conversion**: Done automatically in gateway clients

**Conversion Logic:**
```go
// Paise to Rupees (for gateway)
rupees := float64(paise) / 100.0
rupeesString := fmt.Sprintf("%.2f", rupees)

// Rupees to Paise (from gateway)
paise := int64(rupees * 100)
```

---

## Security Best Practices

### PCI DSS Compliance
- **Cards**: Never store in plaintext
- **CVV**: Transient only, never persisted
- **Tokens**: Gateway-provided, secure references
- **Encryption**: All card data encrypted in transit

### Webhook Security
- **Signature Validation**: Always validate webhook signatures
- **Idempotency**: Handle duplicate webhooks gracefully
- **HTTPS Only**: All webhook endpoints use HTTPS
- **IP Whitelisting**: Consider whitelisting gateway IPs

### Error Handling
- **Timeout Retry**: Implement exponential backoff
- **Graceful Degradation**: Fallback to alternative gateway if available
- **Logging**: Log all gateway interactions (excluding sensitive data)
- **Alerting**: Monitor failed transactions and webhook failures

---

## Testing

### Test Cards

**JioPay Test Cards:**
```
Visa Success:       4111111111111111
MasterCard Success: 5500000000000004
RuPay Success:      6073849700004947
```

**Easebuzz Test Cards:**
```
Success: 4111111111111111
Failure: 4000000000000002
```

### Webhook Testing

Use tools like ngrok to expose local endpoints:

```bash
# Expose local port 8080
ngrok http 8080

# Update webhook URLs in gateway dashboard to ngrok URL
# Example: https://abc123.ngrok.io/v1/process_payment/webhooks/payment_status?type=jiopay
```

### Mock Services

For local development without actual gateway calls:

```go
// Enable mock mode
export ENABLE_MOCK_GATEWAY=true

// Mock always returns success
// Useful for frontend development
```

---

## Error Codes

### JioPay Error Codes

| Code | Description |
|------|-------------|
| 0000 | Success |
| 1001 | Invalid merchant ID |
| 1002 | Invalid transaction ID |
| 2001 | Card declined |
| 2002 | Insufficient funds |
| 3001 | 3DS authentication failed |
| 9999 | System error |

### Easebuzz Error Codes

| Code | Description |
|------|-------------|
| 1 | Success |
| 0 | Failure |
| -1 | Pending |

Status values: `success`, `failure`, `pending`, `userCancelled`

---

## Implementation Files

### JioPay Client
```
internal/adapters/external/payment/jiopay/
├── client.go              # Main client implementation
├── tokenization.go        # Tokenization API
├── cryptogram.go          # Cryptogram generation
├── payment.go             # Payment initiation
├── webhook.go             # Webhook handling
└── encryption.go          # AES-128-ECB encryption
```

### Easebuzz Client
```
internal/adapters/external/payment/easebuzz/
├── client.go              # Main client implementation
├── tokenization.go        # Tokenization API (TMS)
├── cryptogram.go          # Cryptogram generation (TMS)
├── payment.go             # Payment initiation
├── webhook.go             # Webhook handling
└── encryption.go          # AES-256-CBC encryption
```

### Payment Orchestrator
```
internal/services/payments/
├── payment_orchestrator.go    # Gateway routing logic
├── gateway_factory.go         # Gateway factory pattern
└── fee_calculator.go          # Fee calculation
```

---

## Adding a New Gateway

Follow these steps to add a new payment gateway:

### 1. Create Gateway Client

```bash
mkdir -p internal/adapters/external/payment/newgateway
cd internal/adapters/external/payment/newgateway
touch client.go tokenization.go payment.go webhook.go
```

### 2. Implement PaymentGateway Interface

```go
type PaymentGateway interface {
    TokenizeCard(ctx context.Context, req TokenizeRequest) (*TokenizeResponse, error)
    GenerateCryptogram(ctx context.Context, req CryptogramRequest) (*CryptogramResponse, error)
    InitiatePayment(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)
    ValidateWebhook(ctx context.Context, payload []byte, signature string) error
}
```

### 3. Update Gateway Factory

**File:** `internal/services/payments/factory/gateway_factory.go`

```go
func GetGateway(gatewayType string) (PaymentGateway, error) {
    switch gatewayType {
    case "jiopay":
        return jiopay.NewClient(config), nil
    case "easebuzz":
        return easebuzz.NewClient(config), nil
    case "newgateway":
        return newgateway.NewClient(config), nil  // Add this
    default:
        return nil, fmt.Errorf("unsupported gateway: %s", gatewayType)
    }
}
```

### 4. Update Payment Orchestrator

**File:** `internal/services/payments/payment_orchestrator.go`

```go
func (o *Orchestrator) SelectGateway(req PaymentRequest) string {
    // Add routing logic
    if req.PaymentMethod == "new_method" {
        return "newgateway"
    }

    // Existing logic...
}
```

### 5. Add Environment Variables

**File:** `.env`

```bash
NEWGATEWAY_BASE_URL=https://api.newgateway.com
NEWGATEWAY_MERCHANT_ID=your_merchant_id
NEWGATEWAY_API_KEY=your_api_key
NEWGATEWAY_WEBHOOK_SECRET=your_webhook_secret
```

### 6. Update Configuration

**File:** `internal/shared/config/config.go`

```go
type Config struct {
    // ... existing config

    NewGateway struct {
        BaseURL       string
        MerchantID    string
        APIKey        string
        WebhookSecret string
    }
}
```

### 7. Add Tests

```go
func TestNewGateway_TokenizeCard(t *testing.T) {
    // Test implementation
}

func TestNewGateway_InitiatePayment(t *testing.T) {
    // Test implementation
}
```

---

## Troubleshooting

### Common Issues

#### 1. JioPay x-app-token Error

**Error:** "Invalid x-app-token"

**Solution:**
- Verify encryption key is exactly 16 bytes
- Check timestamp format: `yyyyMMddHHmmss`
- Ensure trace-id is uppercase UUID without hyphens
- Verify concatenation order: `app-id$trace-id$timestamp`

#### 2. Easebuzz Decryption Failed

**Error:** "Unable to decrypt payload"

**Solution:**
- Verify merchant key and salt are correct
- Check key derivation: `key = SHA256(merchantKey)`
- Check IV derivation: `iv = SHA256(salt)[:16]`
- Ensure padding is PKCS7

#### 3. Webhook Signature Mismatch

**Error:** "Webhook signature validation failed"

**Solution:**
- Verify webhook secret is correct
- Check field concatenation order (alphabetical)
- Ensure hash algorithm matches (HMAC-SHA256 vs SHA-512)
- Log computed hash vs received hash for debugging

#### 4. Settlement Not Processing

**Solution:**
- Check payment status is 'success' before settlement
- Verify settlement_status is 'pending' (not already 'settled')
- For Easebuzz: Ensure form-urlencoded parsing is correct
- Check amount conversion (float to int64)

---

## Support & Documentation

### Official Gateway Documentation

- **JioPay**: Contact JioPay support for API documentation
- **Easebuzz**: https://docs.easebuzz.in/

### Internal Documentation

- **Payment Module**: `docs/payment_module.md`
- **API Reference**: `API_REFERENCE.md`
- **Code Examples**: `EXAMPLES.md`

### Contact

For implementation questions, contact:
- Backend Team: backend@xcentro.in
- DevOps: devops@xcentro.in
