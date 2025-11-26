# MockPG Settlement Webhook Testing Guide

## Overview

This guide shows you how to test the complete end-to-end flow from payment to settlement to payout using MockPG.

---

## Prerequisites

1. **MockPG Enabled**: Ensure `MOCKPG_ENABLED=true` in your `.env`
2. **Server Running**: Your backend API should be running (default: `http://localhost:8080`)
3. **Database Access**: For verification queries
4. **Payment ID**: You need a successful MockPG payment first

---

## Step-by-Step Testing Flow

### Step 1: Make a Payment via MockPG

First, create a payment using MockPG. After the payment succeeds:
- `payment.status` = `'success'`
- `payment.settlement_status` = `'pending'` (auto-set by payment webhook)

**Verification Query:**
```sql
SELECT
    payment_id,
    status,
    settlement_status,
    total_amount,
    gateway_used
FROM payment_table
WHERE payment_id = 'your_payment_id_here'
  AND gateway_used = 'mockpg';
```

Expected Result:
```
payment_id              | status  | settlement_status | total_amount | gateway_used
your_payment_id_here    | success | pending          | 100000       | mockpg
```

---

### Step 2: Trigger Settlement Webhook

Now trigger the settlement webhook to mark the payment as settled:

```bash
curl -X POST 'http://localhost:8080/v1/process_payment/webhooks/settlement_status?type=mockpg' \
  -H 'Content-Type: application/json' \
  -d '{
    "merchant_id": "mock_merchant_123",
    "transaction_id": "YOUR_PAYMENT_ID_HERE",
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

**Important**: Replace `YOUR_PAYMENT_ID_HERE` with your actual payment ID!

**Expected Response:**
```json
{
  "status": "success",
  "message": "Settlement processed successfully"
}
```

**Verification Query:**
```sql
SELECT
    payment_id,
    status,
    settlement_status,
    settlement_amount,
    mdr_amount,
    net_settlement_amount,
    settled_at,
    settlement_metadata->>'settlement_gateway' as gateway
FROM payment_table
WHERE payment_id = 'your_payment_id_here';
```

Expected Result:
```
payment_id         | status  | settlement_status | settlement_amount | mdr_amount | net_settlement_amount | settled_at              | gateway
your_payment_id    | success | settled          | 100000            | 0          | 100000                | 2025-01-25 10:30:00+00  | mockpg
```

---

### Step 3: Check Vouchers are Ready for Payout

After settlement, vouchers linked to this payment should be ready for payout:

```sql
SELECT
    v.voucher_id,
    v.voucher_number,
    v.payment_id,
    v.payment_status,
    v.payout_status,
    v.net_amount,
    p.settlement_status
FROM payment_vouchers v
JOIN payment_table p ON v.payment_id = p.payment_id
WHERE v.payment_id = 'your_payment_id_here';
```

Expected Result:
```
voucher_id   | voucher_number  | payment_id     | payment_status | payout_status | net_amount | settlement_status
uuid-123     | VOUCH-2025-001  | your_payment_id| paid          | pending       | 98000      | settled
```

---

### Step 4: Wait for Payout Worker (or Trigger Manually)

#### Option A: Automatic (Payout Worker)

The payout worker runs every 5 minutes and will automatically:
1. Find vouchers where `settlement_status='settled'` AND `payout_status='pending'`
2. Create transfer requests
3. Initiate payouts to Castler

**Wait 5 minutes** and check:
```sql
SELECT
    ptr.transfer_request_id,
    ptr.voucher_id,
    ptr.status,
    ptr.amount,
    ptr.initiated_at
FROM payout_transfer_requests ptr
WHERE ptr.payment_id = 'your_payment_id_here';
```

#### Option B: Manual Payout (Immediate Testing)

Trigger payout immediately via API:

```bash
curl -X POST 'http://localhost:8080/v1/payout/dashboard/initiate' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_JWT_TOKEN' \
  -d '{
    "payment_id": "your_payment_id_here",
    "voucher_ids": []
  }'
```

**Expected Response:**
```json
{
  "transfers": [
    {
      "transfer_request_id": "uuid-456",
      "voucher_id": "uuid-123",
      "amount": 98000,
      "status": "initiated",
      "provider": "castler"
    }
  ]
}
```

---

### Step 5: Verify Payout Transfer Created

```sql
SELECT
    transfer_request_id,
    payment_id,
    voucher_id,
    status,
    amount,
    provider,
    initiated_at,
    webhook_timeout_at
FROM payout_transfer_requests
WHERE payment_id = 'your_payment_id_here';
```

Expected Result:
```
transfer_request_id | payment_id     | voucher_id | status    | amount | provider | initiated_at        | webhook_timeout_at
uuid-456           | your_payment_id| uuid-123   | initiated | 98000  | castler  | 2025-01-25 10:35:00 | 2025-01-25 11:35:00
```

---

### Step 6: Simulate Castler Webhook (Completion)

Simulate Castler sending a completion webhook:

```bash
curl -X POST 'http://localhost:8080/v1/payout/webhooks/castler' \
  -H 'Content-Type: application/json' \
  -H 'X-Castler-Signature: mock_signature_123' \
  -d '{
    "event_type": "transfer.completed",
    "transfer_id": "castler_transfer_id_123",
    "status": "success",
    "utr": "HDFC24011512345678",
    "timestamp": "2025-01-25T10:45:00Z"
  }'
```

**Note**: You need to set `provider_transfer_id` in the database first, or adjust the webhook to match your transfer.

---

### Step 7: Verify Payout Completed

```sql
SELECT
    ptr.transfer_request_id,
    ptr.status,
    ptr.utr,
    ptr.completed_at,
    v.payout_status
FROM payout_transfer_requests ptr
JOIN payment_vouchers v ON ptr.voucher_id = v.voucher_id
WHERE ptr.payment_id = 'your_payment_id_here';
```

Expected Result:
```
transfer_request_id | status    | utr                  | completed_at        | payout_status
uuid-456           | completed | HDFC24011512345678   | 2025-01-25 10:45:00 | completed
```

---

## Complete Flow Verification Query

Run this to see the complete status:

```sql
SELECT
    p.payment_id,
    p.status as payment_status,
    p.settlement_status,
    p.settled_at,
    v.voucher_id,
    v.voucher_number,
    v.payout_status,
    ptr.transfer_request_id,
    ptr.status as transfer_status,
    ptr.utr
FROM payment_table p
LEFT JOIN payment_vouchers v ON p.payment_id = v.payment_id
LEFT JOIN payout_transfer_requests ptr ON v.voucher_id = ptr.voucher_id
WHERE p.payment_id = 'your_payment_id_here';
```

Expected Complete Flow:
```
payment_id     | payment_status | settlement_status | settled_at          | voucher_id | payout_status | transfer_request_id | transfer_status | utr
your_payment_id| success       | settled          | 2025-01-25 10:30:00 | uuid-123   | completed     | uuid-456           | completed       | HDFC24011512345678
```

---

## Quick Test Script

Here's a bash script to test the complete flow:

```bash
#!/bin/bash

# Configuration
API_BASE="http://localhost:8080"
PAYMENT_ID="your_payment_id_here"  # Replace with actual payment ID
JWT_TOKEN="your_jwt_token_here"     # Replace with actual JWT

echo "=== Testing MockPG Settlement → Payout Flow ==="
echo ""

# Step 1: Trigger Settlement Webhook
echo "Step 1: Triggering settlement webhook..."
curl -X POST "${API_BASE}/v1/process_payment/webhooks/settlement_status?type=mockpg" \
  -H 'Content-Type: application/json' \
  -d "{
    \"merchant_id\": \"mock_merchant_123\",
    \"transaction_id\": \"${PAYMENT_ID}\",
    \"gateway_payment_id\": \"MOCKPG_xxx\",
    \"settlement_id\": \"SETTLE_BATCH_$(date +%Y%m%d)\",
    \"settlement_date\": \"$(date +%Y-%m-%d)\",
    \"settlement_amount\": 100000,
    \"mdr_amount\": 0,
    \"service_tax\": 0,
    \"net_settlement_amount\": 100000,
    \"payment_method\": \"card\",
    \"settlement_account\": \"9876543210\",
    \"settlement_account_ifsc\": \"MOCK0001234\",
    \"signature\": \"MOCK_SIG_$(date +%s)\"
  }"
echo ""
echo ""

# Step 2: Wait a moment
echo "Step 2: Waiting 2 seconds..."
sleep 2

# Step 3: Initiate Payout
echo "Step 3: Initiating payout..."
curl -X POST "${API_BASE}/v1/payout/dashboard/initiate" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d "{
    \"payment_id\": \"${PAYMENT_ID}\",
    \"voucher_ids\": []
  }"
echo ""
echo ""

echo "=== Test Complete ==="
echo "Check database to verify:"
echo "  - payment.settlement_status = 'settled'"
echo "  - voucher.payout_status = 'pending' or 'initiated'"
echo "  - payout_transfer_requests created"
```

Save as `test_settlement_flow.sh`, make executable with `chmod +x test_settlement_flow.sh`, and run!

---

## Troubleshooting

### Issue: "Payment not found"

**Cause**: Wrong `transaction_id` in webhook payload

**Fix**: Ensure `transaction_id` matches your actual `payment_id`

---

### Issue: "Payment not in success state"

**Cause**: Payment status is not 'success'

**Fix**:
```sql
-- Check payment status
SELECT payment_id, status FROM payment_table WHERE payment_id = 'your_payment_id';

-- If status is 'pending', complete the payment first via payment webhook
```

---

### Issue: "Invalid settlement status"

**Cause**: Settlement status is not 'pending'

**Fix**: Payment might already be settled. Check:
```sql
SELECT payment_id, settlement_status FROM payment_table WHERE payment_id = 'your_payment_id';
```

If already 'settled', the webhook is idempotent and will return success.

---

### Issue: "Mock PG is not enabled"

**Cause**: `MOCKPG_ENABLED=false` in environment

**Fix**: Set `MOCKPG_ENABLED=true` in `.env` and restart server

---

### Issue: Payout not triggering automatically

**Cause**: Payout worker might be disabled or not running

**Check**:
```sql
SELECT worker_enabled FROM payout_configuration;
```

**Fix**: Enable worker or use manual payout API (Step 4, Option B)

---

## Testing Checklist

- [ ] Payment created via MockPG
- [ ] Payment status = 'success'
- [ ] Settlement webhook triggered (curl command)
- [ ] Settlement status = 'settled' in database
- [ ] Settlement amounts populated (settlement_amount, mdr_amount, net_settlement_amount)
- [ ] Voucher payment_status = 'paid'
- [ ] Payout initiated (automatically or manually)
- [ ] Transfer request created with status = 'initiated'
- [ ] Castler webhook simulated (optional)
- [ ] Payout completed (if webhook sent)

---

## Database Verification Queries

### Check Payment Settlement Status
```sql
SELECT
    payment_id,
    status,
    settlement_status,
    settlement_amount,
    mdr_amount,
    net_settlement_amount,
    settled_at
FROM payment_table
WHERE gateway_used = 'mockpg'
  AND status = 'success'
ORDER BY created_at DESC
LIMIT 10;
```

### Check Vouchers Ready for Payout
```sql
SELECT
    v.voucher_id,
    v.payment_id,
    v.payout_status,
    p.settlement_status,
    p.status as payment_status
FROM payment_vouchers v
JOIN payment_table p ON v.payment_id = p.payment_id
WHERE p.settlement_status = 'settled'
  AND v.payout_status = 'pending'
  AND p.gateway_used = 'mockpg'
ORDER BY v.created_at DESC
LIMIT 10;
```

### Check Payout Transfers
```sql
SELECT
    transfer_request_id,
    payment_id,
    voucher_id,
    status,
    amount,
    provider,
    initiated_at,
    completed_at,
    utr
FROM payout_transfer_requests
ORDER BY created_at DESC
LIMIT 10;
```

---

## Expected Console Output

When you trigger the settlement webhook, you should see:

```
=== MOCKPG SETTLEMENT WEBHOOK: Processing settlement notification ===
📥 Webhook payload size: 387 bytes
📋 TransactionID: your_payment_id_here
📋 GatewayPaymentID: MOCKPG_xxx
📋 SettlementID: SETTLE_BATCH_20250125
📋 SettlementDate: 2025-01-25
📋 SettlementAmount: 100000
📋 MDRAmount: 0
📋 ServiceTax: 0
📋 NetSettlementAmount: 100000
✅ Webhook signature verified
✅ Payment found: your_payment_id_here (status: success, settlement_status: pending)
📅 Settlement date: 2025-01-25
💰 Settlement amounts (in paise):
   - Settlement Amount: 100000 (₹1000.00)
   - MDR Amount: 0 (₹0.00) [Charges: 0 + Tax: 0]
   - Net Settlement: 100000 (₹1000.00)
🔄 Updating settlement status to 'settled'...
✅ Settlement status updated to 'settled'
🔄 Updating settlement amounts...
✅ Settlement amounts updated successfully
=== MOCKPG SETTLEMENT WEBHOOK: Processing complete ===
✅ Settlement processed: Payment your_payment_id_here → ₹1000.00 (after ₹0.00 MDR)
```

---

## Summary

This testing guide enables complete end-to-end testing of:
- ✅ Payment processing (MockPG)
- ✅ Settlement tracking (webhook)
- ✅ Payout initiation (worker or manual)
- ✅ Payout completion (Castler webhook)

No manual database updates needed - everything works through APIs!
