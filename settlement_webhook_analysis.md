# Settlement Webhook Analysis

## Executive Summary

**Yes, there IS a settlement webhook endpoint** implemented in the XCentro backend. Settlement webhooks are separate from payment status webhooks and handle the T+1 to T+3 day settlement notifications from payment gateways.

## Endpoint Details

### Settlement Webhook Endpoint
- **URL**: `POST /v1/process_payment/webhooks/settlement_status?type=<gateway>`
- **Location**: `cmd/api/routes.go:185`
- **Handler**: `internal/handlers/http/payment/webhook_handler.go:647-1298`
- **Supported Gateways**:
  - `type=jiopay` → JioPay settlement webhook handler
  - `type=easebuzz` → Easebuzz settlement webhook handler
  - `type=mockpg` → MockPG settlement webhook handler (testing only)

### Registration in Routes
```go
// Line 185 in cmd/api/routes.go
webhookGroup.POST("/settlement_status", webhookHandler.HandleSettlementStatusWebhook)
```

## How Different Payment Gateway Settlement Webhooks Are Handled

### Architecture Overview

The settlement webhook system follows a **gateway-specific handler pattern**:

```
POST /v1/process_payment/webhooks/settlement_status?type=<gateway>
  ↓
HandleSettlementStatusWebhook (router)
  ↓
  ├─→ handleJioPaySettlementWebhook (single transaction)
  └─→ handleEasebuzzSettlementWebhook (batch transactions)
```

---

## 1. JioPay Settlement Webhook

### Webhook Characteristics
- **Format**: JSON (single transaction per webhook)
- **Trigger**: Sent T+1 to T+3 days after payment success
- **Security**: HMAC-SHA256 signature validation
- **Amount Format**: Strings in rupees (e.g., "1000.00")
- **Processing**: One transaction per webhook call

### Webhook Payload Structure (`jiopay.SettlementWebhook`)
```json
{
  "aggregatorID": "string",
  "merchantId": "string",
  "merchantTxnNo": "payment_id_123",  // KEY: Used to lookup payment
  "paymentMode": "CARD",
  "paymentSubInstType": "CC",
  "txnAmount": "1000.00",             // Original payment amount
  "txnCharges": "15.00",              // Base MDR charges
  "serviceTax": "2.70",               // GST on MDR
  "settlementID": "A00121-20171101-010",
  "settlementDate": "20171101",        // YYYYMMDD format
  "settlementAccount": "1234567890",
  "settlementAccountIFSC": "HDFC0000123",
  "settledAmount": "982.30",          // Net amount received
  "cardNetwork": "VISA",
  "secureHash": "signature"
}
```

### Handler: `handleJioPaySettlementWebhook` (Lines 667-927)

**Processing Steps:**

1. **Read & Parse**: Read JSON body and unmarshal to `jiopay.SettlementWebhook`

2. **Signature Verification**: Validate HMAC-SHA256 `secureHash` using gateway client

3. **Payment Lookup**: Find payment by `merchantTxnNo` (our payment_id)
   ```go
   pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, webhook.MerchantTxnNo)
   ```

4. **State Validation**:
   - Payment status must be `success`
   - Settlement status must be `pending`
   - If already `settled`, perform idempotency check (verify amounts match)

5. **Amount Parsing**: Convert rupee strings to paise (int64)
   ```go
   settlementAmount := parseAmountToPaise(webhook.TxnAmount)     // "1000.00" → 100000
   txnCharges := parseAmountToPaise(webhook.TxnCharges)          // "15.00" → 1500
   serviceTax := parseAmountToPaise(webhook.ServiceTax)          // "2.70" → 270
   netSettlementAmount := parseAmountToPaise(webhook.SettledAmount) // "982.30" → 98230
   mdrAmount := txnCharges + serviceTax                          // 1770
   ```

6. **Date Parsing**: Parse YYYYMMDD format to `time.Time`
   ```go
   settlementDate := parseSettlementDate(webhook.SettlementDate) // "20171101" → 2017-11-01
   ```

7. **Metadata Preparation**: Store full webhook payload as JSONB
   ```go
   settlementMetadata := map[string]any{
       "settlement_webhook_received_at": time.Now(),
       "settlement_gateway": "jiopay",
       "settlement_batch_id": webhook.SettlementID,
       "webhook_payload": webhookData,
       ...
   }
   ```

8. **Database Updates**:
   - Update settlement status: `pending` → `settled`
   - Update settlement amounts: settlement_amount, mdr_amount, net_settlement_amount
   - Auto-set `settled_at` timestamp

9. **Response**: Return 200 OK (always, even on errors to avoid gateway retries)

### Financial Calculation Formula
```
settlement_amount = txnAmount (e.g., ₹1000.00)
mdr_amount = txnCharges + serviceTax (e.g., ₹15.00 + ₹2.70 = ₹17.70)
net_settlement_amount = settlement_amount - mdr_amount (e.g., ₹1000.00 - ₹17.70 = ₹982.30)
```

---

## 2. Easebuzz Settlement Webhook

### Webhook Characteristics
- **Format**: Form-urlencoded (must parse to JSON)
- **Trigger**: Batch processing (multiple transactions in one webhook)
- **Security**: SHA-512 hash validation (TODO - algorithm TBD)
- **Amount Format**: Numbers (int/float64, never strings)
- **Processing**: Multiple transactions per webhook call

### Webhook Payload Structure (`easebuzz.SettlementWebhookResponse`)
```json
{
  "status": 1,
  "data": {
    "hash": "sha512_signature",
    "paid_out": 1,
    "payout_id": "PT1LCHKBAB",
    "payout_date": "2025-01-30 11:50:21.712996",
    "bank_transaction_id": "YESBN5202501300640000000",
    "bank_name": "Kotak Mahindra Bank",
    "account_number": "000000000000",
    "ifsc_code": "K00000ABC",
    "total_amount": 100.0,
    "payout_amount": 95.0,
    "service_charge_amount": 4.0,
    "service_tax_amount": 1.0,
    "settled_transactions": [
      {
        "txnid": "payment_id_456",        // KEY: Used to lookup payment
        "easepayid": "E2501789CVRAAA",
        "transaction_amount": 5.0,         // Original amount (float)
        "service_charge": 0.2,             // MDR base fee (float)
        "service_tax": 0.036,              // GST on MDR (float)
        "settlement_amount": 4.764,        // Net amount (float)
        "transaction_type": "UPI"
      }
      // ... more transactions
    ],
    "refund_transactions": [],
    "split_payouts": [],
    "submerchant_payouts": []
  }
}
```

### Handler: `handleEasebuzzSettlementWebhook` (Lines 937-1109)

**Processing Steps:**

1. **Parse Form-Urlencoded**: Extract JSON from form field "data"
   ```go
   jsonData := parseFormEncodedWebhook(c)  // Handles form → JSON conversion
   ```

2. **Unmarshal**: Parse to `easebuzz.SettlementWebhookResponse`

3. **Validate Status**: Webhook status must be `1` (success)

4. **Date Parsing**: Parse "YYYY-MM-DD HH:MM:SS.SSSSSS" format
   ```go
   payoutDate := parseEasebuzzSettlementDate(webhook.Data.PayoutDate)
   ```

5. **Batch Processing Loop**: Iterate through `settled_transactions[]` array
   ```go
   for idx, settlement := range webhook.Data.SettledTransactions {
       // Process each transaction independently
   }
   ```

6. **Per-Transaction Processing** (inside loop):

   a. **Payment Lookup**: Find payment by `txnid`
      ```go
      pmt, err := h.paymentRepo.GetPaymentByGatewayPaymentID(ctx, settlement.TxnID)
      ```

   b. **State Validation**:
      - Payment status must be `success`
      - Settlement status must be `pending`
      - If already `settled`, skip (idempotent)

   c. **Amount Conversion**: Convert float64 to paise (int64)
      ```go
      settlementAmount := parseNumberToPaise(settlement.TransactionAmount)  // 5.0 → 500
      serviceCharge := parseNumberToPaise(settlement.ServiceCharge)         // 0.2 → 20
      serviceTax := parseNumberToPaise(settlement.ServiceTax)               // 0.036 → 4
      netAmount := parseNumberToPaise(settlement.SettlementAmount)          // 4.764 → 476
      mdrAmount := serviceCharge + serviceTax                               // 24
      ```

   d. **Metadata Preparation**: Store batch-level and transaction-level data
      ```go
      settlementMetadata := map[string]any{
          "settlement_batch_id": webhook.Data.PayoutID,
          "settlement_bank_transaction_id": webhook.Data.BankTransactionID,
          "batch_total_amount": webhook.Data.TotalAmount,
          "easepayid": settlement.EasepayID,
          ...
      }
      ```

   e. **Database Updates**:
      - Update settlement status: `pending` → `settled`
      - Update settlement amounts

   f. **Counter Tracking**:
      - `successCount++`: Successfully processed
      - `skipCount++`: Already settled or invalid state
      - `errorCount++`: Database errors

7. **Batch Summary Response**: Return 200 OK with processing statistics
   ```json
   {
     "status": "success",
     "message": "Batch settlement processed",
     "summary": {
       "total_transactions": 10,
       "processed": 8,
       "skipped": 1,
       "errors": 1
     }
   }
   ```

### Financial Calculation Formula
```
settlement_amount = transaction_amount (e.g., ₹5.00)
mdr_amount = service_charge + service_tax (e.g., ₹0.20 + ₹0.036 = ₹0.236)
net_settlement_amount = settlement_amount - mdr_amount (e.g., ₹5.00 - ₹0.236 = ₹4.764)
```

---

## 3. MockPG Settlement Webhook (Testing Only)

### Webhook Characteristics
- **Format**: JSON (single transaction per webhook)
- **Trigger**: Manual testing (simulated T+1 to T+3 days after payment success)
- **Security**: Mock signature validation
- **Amount Format**: Int64 in paise (e.g., 100000)
- **Processing**: One transaction per webhook call

### Webhook Payload Structure (`mockpg.MockPGSettlementWebhook`)
```json
{
  "merchant_id": "mock_merchant_123",
  "transaction_id": "payment_id_789",        // KEY: Our payment_id for lookup
  "gateway_payment_id": "MOCKPG_xxx",
  "settlement_id": "SETTLE_BATCH_20250125",
  "settlement_date": "2025-01-25",           // YYYY-MM-DD format
  "settlement_amount": 100000,               // Original amount in paise (₹1000.00)
  "mdr_amount": 0,                           // MDR charges in paise (0 for testing)
  "service_tax": 0,                          // GST on MDR in paise (0 for testing)
  "net_settlement_amount": 100000,           // Net amount in paise (₹1000.00)
  "payment_method": "card",
  "settlement_account": "9876543210",
  "settlement_account_ifsc": "MOCK0001234",
  "signature": "MOCK_SIG_abcd1234"
}
```

### Handler: `handleMockPGSettlementWebhook` (Lines 1111-1298)

**Processing Steps:**

1. **Read & Parse**: Read JSON body and unmarshal to `mockpg.MockPGSettlementWebhook`

2. **Signature Verification**: Validate mock signature using gateway client

3. **Payment Lookup**: Find payment by `transaction_id` (our payment_id)
   ```go
   pmt, err := h.paymentRepo.GetPaymentByID(ctx, webhook.TransactionID)
   ```

4. **State Validation**:
   - Payment status must be `success`
   - Settlement status must be `pending`
   - If already `settled`, perform idempotency check

5. **Date Parsing**: Parse YYYY-MM-DD format to `time.Time`
   ```go
   settlementDate := time.Parse("2006-01-02", webhook.SettlementDate) // "2025-01-25"
   ```

6. **Amount Handling**: Amounts already in paise (int64), no conversion needed
   ```go
   settlementAmount := webhook.SettlementAmount     // Already in paise
   mdrAmount := webhook.MDRAmount + webhook.ServiceTax // Typically 0 for testing
   netSettlementAmount := webhook.NetSettlementAmount  // Already in paise
   ```

7. **Metadata Preparation**: Store full webhook payload as JSONB
   ```go
   settlementMetadata := map[string]any{
       "settlement_webhook_received_at": time.Now(),
       "settlement_gateway": "mockpg",
       "settlement_batch_id": webhook.SettlementID,
       "mock_simulation": true,
       ...
   }
   ```

8. **Database Updates**:
   - Update settlement status: `pending` → `settled`
   - Update settlement amounts
   - Auto-set `settled_at` timestamp

9. **Response**: Return 200 OK

### Financial Calculation Formula
```
settlement_amount = original payment amount (e.g., ₹1000.00 = 100000 paise)
mdr_amount = mdr_charges + service_tax (typically 0 for MockPG testing)
net_settlement_amount = settlement_amount - mdr_amount (e.g., 100000 paise)
```

---

## Key Differences Between JioPay, Easebuzz, and MockPG

| Aspect | JioPay | Easebuzz | MockPG |
|--------|---------|----------|---------|
| **Data Format** | JSON (direct) | Form-urlencoded → JSON | JSON (direct) |
| **Batch Processing** | Single transaction | Multiple transactions | Single transaction |
| **Amount Format** | Strings (e.g., "1000.00") | Numbers (e.g., 1000.0) | Int64 (e.g., 100000) |
| **Date Format** | YYYYMMDD | YYYY-MM-DD HH:MM:SS.SSSSSS | YYYY-MM-DD |
| **Signature** | HMAC-SHA256 ✅ | SHA-512 ⚠️ | Mock validation ✅ |
| **Transaction ID** | `merchantTxnNo` | `txnid` | `transaction_id` |
| **Gateway ID** | `txnID` | `easepayid` | `gateway_payment_id` |
| **MDR Amount** | Non-zero (real fees) | Non-zero (real fees) | Zero (testing) |
| **Response** | Simple success/error | Batch summary | Simple success/error |
| **Purpose** | Production | Production | Testing only |

---

## Database Updates Performed

Both handlers update the `payment_table` with the following changes:

### 1. Settlement Status Update
```sql
UPDATE payment_table
SET settlement_status = 'settled',
    settlement_metadata = <metadata_jsonb>,
    settled_at = NOW()
WHERE payment_id = <payment_id>;
```

### 2. Settlement Amounts Update
```sql
UPDATE payment_table
SET settlement_amount = <amount>,
    mdr_amount = <mdr>,
    net_settlement_amount = <net_amount>
WHERE payment_id = <payment_id>;
```

### Fields Updated
- `settlement_status`: `pending` → `settled`
- `settlement_amount`: Original payment amount eligible for settlement
- `mdr_amount`: Total MDR fees (charges + GST)
- `net_settlement_amount`: Final amount received in merchant account
- `settled_at`: Timestamp when settlement completed
- `settlement_metadata`: Full webhook payload as JSONB (for audit trail)

---

## Security & Validation

### JioPay
✅ **Implemented**: HMAC-SHA256 signature verification via gateway client
```go
if err := h.jioPayGateway.ValidateWebhook(bodyBytes, webhook.SecureHash); err != nil {
    // Return 200 OK to avoid retries, but log failure
}
```

### Easebuzz
⚠️ **TODO**: SHA-512 hash validation algorithm TBD
```go
// TODO: Implement hash validation once settlement webhook hash algorithm confirmed
fmt.Printf("⚠️  Hash validation skipped for settlement webhooks (algorithm TBD)\n")
```

---

## Idempotency Handling

Both handlers implement **idempotency checks** to handle duplicate webhooks:

1. **Check Current Status**: If `settlement_status == 'settled'`, the payment is already processed
2. **Verify Amounts**: Compare stored amounts with webhook amounts
3. **Log Mismatches**: Warn if amounts differ (but still return success)
4. **Return Success**: Always return 200 OK to prevent gateway retries

---

## Error Handling Strategy

**Philosophy**: Always return 200 OK to avoid infinite gateway retries

### Error Response Pattern
```go
return c.JSON(http.StatusOK, map[string]string{
    "status":  "error",
    "message": "Payment not found",
})
```

### Errors That Return 200 OK
- Signature verification failure
- Payment not found
- Payment not in success state
- Settlement status not pending
- Database update failures

### Only 400 Bad Request
- Invalid request body (cannot read/parse)
- Invalid JSON structure
- Missing gateway type parameter

---

## Settlement Service Integration

Location: `internal/services/settlement/settlement_service.go`

### Current Status: MVP (Placeholder Implementation)

The settlement service is currently **ultra-simple** and always passes verification:

```go
// MVP: Always pass - ops team is verification layer
return true, "", nil
```

### Planned Enhancements (Phase 2+)

1. **VerifySettlement**: Compare expected vs actual settlement amounts
2. **CalculateExpectedNetSettlement**: Gateway-specific MDR calculation
3. **VerifySettlementWebhook**: Validate webhook amounts against payment records

---

## Related Files & Components

### Webhook Handler
- `internal/handlers/http/payment/webhook_handler.go` (lines 647-1109)

### Gateway Type Definitions
- `internal/adapters/external/payment/jiopay/types.go` (lines 286-321)
- `internal/adapters/external/payment/easebuzz/types.go` (lines 291-382)

### Routes Registration
- `cmd/api/routes.go` (line 185)

### Payment Repository
- Updates settlement status and amounts via repository methods:
  - `UpdateSettlementStatus(ctx, paymentID, status, metadata)`
  - `UpdateSettlementAmounts(ctx, paymentID, settlementAmt, mdrAmt, netAmt)`

### Documentation
- `GATEWAY_INTEGRATION.md` - Webhook payload examples and signature algorithms
- `docs/payment_module.md` - Payment system architecture

---

## Usage Examples

### JioPay Settlement Webhook (Single Transaction)
```bash
curl -X POST 'http://localhost:8080/v1/process_payment/webhooks/settlement_status?type=jiopay' \
  -H 'Content-Type: application/json' \
  -d '{
    "merchantTxnNo": "pmt_01HK...",
    "txnAmount": "1000.00",
    "txnCharges": "15.00",
    "serviceTax": "2.70",
    "settledAmount": "982.30",
    "settlementID": "A00121-20250125-010",
    "settlementDate": "20250125",
    "secureHash": "abcd1234..."
  }'
```

### Easebuzz Settlement Webhook (Batch)
```bash
curl -X POST 'http://localhost:8080/v1/process_payment/webhooks/settlement_status?type=easebuzz' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'data={"status":1,"data":{"payout_id":"PT123","settled_transactions":[...]}}'
```

### MockPG Settlement Webhook (Testing)
```bash
curl -X POST 'http://localhost:8080/v1/process_payment/webhooks/settlement_status?type=mockpg' \
  -H 'Content-Type: application/json' \
  -d '{
    "merchant_id": "mock_merchant_123",
    "transaction_id": "your_payment_id_here",
    "gateway_payment_id": "MOCKPG_xxx",
    "settlement_id": "SETTLE_BATCH_20250125",
    "settlement_date": "2025-01-25",
    "settlement_amount": 100000,
    "mdr_amount": 0,
    "service_tax": 0,
    "net_settlement_amount": 100000,
    "payment_method": "card",
    "settlement_account": "9876543210",
    "settlement_account_ifsc": "MOCK0001234",
    "signature": "MOCK_SIG_abcd1234"
  }'
```

---

## Outstanding TODOs

1. ✅ JioPay signature verification - **IMPLEMENTED**
2. ⚠️ Easebuzz hash validation - **TODO** (algorithm TBD for settlement webhooks)
3. ⚠️ Settlement service verification logic - **MVP placeholder** (always passes)
4. ⚠️ Webhook logging/history table - **TODO** (for debugging webhook attempts)
5. ⚠️ Gateway-specific MDR calculators - **Planned for Phase 2+**

---

## Conclusion

The XCentro backend has **fully implemented settlement webhook endpoints** for JioPay, Easebuzz, and MockPG payment gateways. The system handles:

- ✅ Gateway-specific payload parsing (JSON vs form-urlencoded)
- ✅ Amount format differences (strings vs numbers vs int64)
- ✅ Single vs batch transaction processing
- ✅ Financial tracking (settlement amount, MDR, net amount)
- ✅ Signature verification (JioPay: HMAC-SHA256, MockPG: Mock validation)
- ✅ Idempotency handling
- ✅ Database updates with full audit trail
- ✅ **MockPG settlement webhook for end-to-end testing**
- ⚠️ Easebuzz signature validation pending (algorithm TBD)

### Testing Capability

With the new MockPG settlement webhook:
- ✅ Complete end-to-end testing (payment → settlement → payout)
- ✅ Payout worker can auto-trigger for MockPG payments
- ✅ No manual database updates needed
- ✅ Full flow validation in test environments

The implementation follows a **defensive error handling strategy** where all errors return 200 OK to prevent infinite gateway retries while logging failures for investigation.
