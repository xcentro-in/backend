# Payment-Voucher Integration & Payout Flow

**Created**: 2025-10-03
**Status**: Phase 6 Implementation
**Purpose**: Document payout flow, design decisions, and payment-voucher integration patterns

---

## Business Context: End-to-End Money Flow

```
Customer Payment → Payment Gateway → XCentro Escrow Account → [Operations Team] → Payout API → Payee Bank Account
```

### Key Components

| Stage | Owner | Purpose |
|-------|-------|---------|
| **Payment** | Customer + Gateway | Customer pays to XCentro escrow via payment gateway |
| **Escrow** | XCentro Finance | Funds held in XCentro account (compliance, reconciliation) |
| **Payout** | Operations Team | Initiate bank transfers to payees using payout API |
| **Settlement** | Payee | Funds reach payee bank account |

### Critical Insight

**One payment can trigger multiple payouts**

- Payment = Gateway transaction (customer → escrow)
- Payout = Bank transfer (escrow → payee)
- Multi-bill payment: 1 payment → N vouchers → N payouts

---

## Multi-Payment Scenarios

### Scenario 1: Multi-Bill Payment

**User Flow**:
```
User selects 3 unpaid bills from different vendors:
├── Bill 1: Vendor A, Invoice INV-001, Amount: ₹50,000
├── Bill 2: Vendor B, Invoice INV-002, Amount: ₹30,000
└── Bill 3: Vendor C, Invoice INV-003, Amount: ₹20,000

Total: ₹1,00,000
```

**Payment Flow**:
```
Step 1: Create 3 draft vouchers
POST /v1/vouchers/multi
Response:
  voucher_ids: ["v-uuid-1", "v-uuid-2", "v-uuid-3"]

Step 2: User clicks "Pay All"
POST /v1/process_payment/make_payment
{
  "amount": 100000,
  "voucher_ids": ["v-uuid-1", "v-uuid-2", "v-uuid-3"]
}

Step 3: One payment record created
payments table:
  payment_id: P-123
  amount: 100000
  status: success

Step 4: All 3 vouchers finalized with same payment_id
payment_vouchers table:
  V1: payee_id=A, amount=50000, payment_id=P-123, status=finalized
  V2: payee_id=B, amount=30000, payment_id=P-123, status=finalized
  V3: payee_id=C, amount=20000, payment_id=P-123, status=finalized
```

**Payout Flow (Operations Team)**:
```
Operations query: "Show pending payouts"

Result:
  Payee A: ₹50,000 (1 voucher)
  Payee B: ₹30,000 (1 voucher)
  Payee C: ₹20,000 (1 voucher)

Operations initiate 3 separate payouts:
  Payout 1: ₹50,000 → Vendor A (HDFC Bank, ****3456)
  Payout 2: ₹30,000 → Vendor B (ICICI Bank, ****7890)
  Payout 3: ₹20,000 → Vendor C (Axis Bank, ****1234)

Update voucher payout_status:
  V1: payout_status = 'initiated' → 'completed', payout_utr = 'UTR123456'
  V2: payout_status = 'initiated' → 'completed', payout_utr = 'UTR123457'
  V3: payout_status = 'initiated' → 'completed', payout_utr = 'UTR123458'
```

---

### Scenario 2: Bulk Payroll Payment

**User Flow**:
```
Payroll run for 50 employees:
├── Employee 1: ₹10,000
├── Employee 2: ₹10,000
├── ...
└── Employee 50: ₹10,000

Total: ₹5,00,000
```

**Payment Flow**:
```
Step 1: Create 50 draft vouchers (one per employee)
POST /v1/vouchers/multi
Response: 50 voucher IDs

Step 2: One payment for entire payroll
POST /v1/process_payment/make_payment
{
  "amount": 500000,
  "voucher_ids": [50 voucher IDs]
}

Step 3: One payment record
payments table:
  payment_id: P-456
  amount: 500000

Step 4: All 50 vouchers finalized
payment_vouchers table:
  V1-V50: payment_id = P-456, status = finalized
```

**Payout Flow**:
```
Operations: Initiate 50 payouts (one per employee)
Each voucher tracks its own payout status independently
```

---

## Design Decision: Where Does Payee Tracking Live?

### Problem Statement

**Question**: Should `payments` table have a `payee_id` column for payout tracking?

### ❌ Option A: Add payee_id to payments table

**Problem**: One payment can have multiple payees

```sql
-- DOESN'T WORK FOR MULTI-BILL PAYMENTS
payments table:
payment_id | amount   | payee_id
P-123      | 100000   | ??? (Vendor A, B, or C?)
```

**Analysis**:
- Multi-bill payment has 3 different payees
- Can't store 3 payees in one column
- Would need array or separate junction table
- Duplicates data already in voucher table

### ✅ Option B: Payout tracking in payment_vouchers table

**Solution**: Voucher is the natural breakdown

```sql
payment_vouchers table:
voucher_id | payment_id | payee_id | amount | payout_status
V1         | P-123      | A        | 50000  | pending
V2         | P-123      | B        | 30000  | pending
V3         | P-123      | C        | 20000  | pending
```

**Benefits**:
- ✅ One voucher = one payee = one payout
- ✅ Natural 1:1 mapping
- ✅ No data duplication
- ✅ Payout status per voucher (granular tracking)
- ✅ Supports partial payouts (some vouchers paid, others pending)

### Decision: Use Option B

**Rationale**:
- Voucher is the **unit of payout**
- Payment is the **unit of customer transaction**
- Operations team works with vouchers for payout initiation

---

## Payout Tracking Schema

### Added Columns to `payment_vouchers` Table

```sql
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN payout_status VARCHAR(50) DEFAULT 'pending'
    CHECK (payout_status IN ('pending', 'initiated', 'completed', 'failed')),
ADD COLUMN payout_initiated_at TIMESTAMP,
ADD COLUMN payout_initiated_by UUID REFERENCES xcentro.users(uid),
ADD COLUMN payout_completed_at TIMESTAMP,
ADD COLUMN payout_reference_id VARCHAR(255),
ADD COLUMN payout_utr VARCHAR(100),
ADD COLUMN payout_metadata JSONB;
```

### Field Descriptions

| Field | Type | Purpose |
|-------|------|---------|
| `payout_status` | VARCHAR(50) | Lifecycle: pending → initiated → completed/failed |
| `payout_initiated_at` | TIMESTAMP | When operations team initiated payout |
| `payout_initiated_by` | UUID | Operations user who initiated payout |
| `payout_completed_at` | TIMESTAMP | When payout settled in payee account |
| `payout_reference_id` | VARCHAR(255) | Payout gateway transaction ID (Razorpay, Cashfree) |
| `payout_utr` | VARCHAR(100) | Bank UTR number for reconciliation |
| `payout_metadata` | JSONB | Additional payout details (gateway response, etc.) |

### Payout Lifecycle

```
draft → finalized → [payout: pending] → [payout: initiated] → [payout: completed]
                                                             → [payout: failed]
```

**States**:
1. **pending**: Voucher finalized, ready for payout
2. **initiated**: Payout API called, awaiting settlement
3. **completed**: Funds settled in payee account (UTR received)
4. **failed**: Payout failed (invalid account, insufficient balance, etc.)

---

## Operations Team Queries

### Query 1: Pending Payouts Grouped by Payee

**Use Case**: Operations team sees dashboard of pending payouts

```sql
SELECT
    v.payee_id,
    pb.legal_name AS payee_name,
    pba.account_number_encrypted AS account_number,
    pba.ifsc_code,
    pba.bank_name,
    COUNT(v.voucher_id) AS voucher_count,
    SUM(v.net_amount) AS total_payout_amount,
    ARRAY_AGG(v.voucher_number ORDER BY v.voucher_date) AS voucher_list,
    ARRAY_AGG(v.invoice_number ORDER BY v.voucher_date) AS invoice_list
FROM xcentro.payment_vouchers v
JOIN xcentro.payee_business pb ON v.payee_id = pb.payee_id
JOIN xcentro.payee_bank_accounts pba ON v.account_id = pba.account_id
JOIN xcentro.payments p ON v.payment_id = p.payment_id
WHERE p.status = 'success'
  AND v.status = 'finalized'
  AND v.payout_status = 'pending'
GROUP BY v.payee_id, pb.legal_name, pba.account_number_encrypted, pba.ifsc_code, pba.bank_name
ORDER BY total_payout_amount DESC;
```

**Result**:
```
payee_id | payee_name | account  | ifsc       | bank      | vouchers | amount    | voucher_numbers
---------|------------|----------|------------|-----------|----------|-----------|----------------
payee-A  | Vendor A   | ****3456 | HDFC0001234| HDFC Bank | 5        | ₹2,50,000 | [PV-001, PV-010, ...]
payee-B  | Vendor B   | ****7890 | ICIC0004567| ICICI Bank| 2        | ₹1,20,000 | [PV-005, PV-012]
```

**Operations Action**: Click "Initiate Payout" → Calls payout API for each payee

---

### Query 2: Payment History for Specific Payee

**Use Case**: Customer support queries "Show all payments to Vendor A"

```sql
SELECT
    p.payment_id,
    p.amount AS payment_amount,
    p.status AS payment_status,
    p.created_at AS payment_date,
    v.voucher_id,
    v.voucher_number,
    v.invoice_number,
    v.net_amount AS voucher_amount,
    v.status AS voucher_status,
    v.payout_status,
    v.payout_utr
FROM xcentro.payments p
JOIN xcentro.payment_vouchers v ON p.payment_id = v.payment_id
WHERE v.payee_id = $1
ORDER BY p.created_at DESC
LIMIT 50;
```

**Result**:
```
payment_id | payment_amt | voucher_number | invoice   | voucher_amt | payout_status | payout_utr
-----------|-------------|----------------|-----------|-------------|---------------|------------
P-123      | ₹1,00,000   | PV-2024-00001 | INV-001   | ₹50,000     | completed     | UTR123456
P-456      | ₹2,00,000   | PV-2024-00050 | INV-050   | ₹2,00,000   | pending       | NULL
```

---

### Query 3: Payout Reconciliation Report

**Use Case**: Finance team reconciles bank statements with payouts

```sql
SELECT
    v.payout_utr,
    v.payout_completed_at,
    pb.legal_name AS payee_name,
    pba.account_number_encrypted,
    pba.ifsc_code,
    v.net_amount,
    v.voucher_number,
    v.invoice_number,
    v.payout_reference_id
FROM xcentro.payment_vouchers v
JOIN xcentro.payee_business pb ON v.payee_id = pb.payee_id
JOIN xcentro.payee_bank_accounts pba ON v.account_id = pba.account_id
WHERE v.payout_status = 'completed'
  AND v.payout_completed_at BETWEEN $1 AND $2
ORDER BY v.payout_completed_at DESC;
```

---

## Separation of Concerns

### Table Responsibilities

| Table | Purpose | Stakeholder | Operations |
|-------|---------|-------------|------------|
| `payments` | Gateway transactions, escrow tracking | Finance Team | Payment reconciliation, refunds |
| `payment_vouchers` | Accounting records, payout tracking, ERP sync | Operations + Accounting | Payout initiation, voucher posting |
| `payee_business` | Vendor/employee master data | Vendor Management | Onboarding, GST verification |
| `payee_bank_accounts` | Payee bank account details | Operations | Penny drop, payout destination |

### Why Voucher is the Payout Unit

**Analogy**:
- **Payment** = Shopping cart (one transaction, multiple items)
- **Voucher** = Individual item (one seller, one amount, one delivery)
- **Payout** = Delivery to seller (happens per item, not per cart)

**Example**:
```
Amazon order (₹1000) with 3 items:
├── Item 1: Seller A (₹400) → Payout to Seller A
├── Item 2: Seller B (₹300) → Payout to Seller B
└── Item 3: Seller C (₹300) → Payout to Seller C

Customer makes ONE payment
Amazon initiates THREE payouts (one per seller)
```

---

## Payment-Voucher Integration Flow

### API Flow: Quick Payment

**Step 1: Create Draft Voucher**
```bash
POST /v1/vouchers/draft
{
  "payee_id": "uuid-vendor-a",
  "account_id": "uuid-account-1",
  "invoice_number": "INV-2024-001",
  "gross_amount": 118000,
  "gst_amount": 18000,
  "tds_amount": 1000,
  "net_amount": 117000,
  "description": "Website development services"
}

Response:
{
  "voucher_id": "v-uuid-123",
  "voucher_number": "PV-2024-00001",
  "status": "draft",
  "fiscal_year": "FY2024-25"
}
```

**Step 2: Make Payment (with voucher_ids)**
```bash
POST /v1/process_payment/make_payment
{
  "business_id": "uuid-business",
  "user_id": "uuid-user",
  "amount": 117000,
  "currency": "INR",
  "payment_method": "card",
  "voucher_ids": ["v-uuid-123"],  # Links payment to voucher
  "card_details": {
    "number": "4111111111111111",
    "expiry_month": "12",
    "expiry_year": "2025",
    "cvv": "123",
    "holder_name": "Business Owner",
    "card_type": "visa",
    "card_category": "credit"
  }
}

Response:
{
  "payment_id": "p-uuid-456",
  "status": "pending",
  "amount": 117000,
  "redirect_url": "https://gateway.com/...",
  "voucher_status": "finalized",           # NEW
  "vouchers_finalized": ["v-uuid-123"]     # NEW
}
```

**Backend Flow**:
```
1. Validate voucher exists and is in draft status
2. Create payment record (status: pending)
3. Call payment gateway
4. On gateway response success:
   a. Update payment status
   b. Finalize voucher (status: draft → finalized)
   c. Link voucher to payment (voucher.payment_id = p-uuid-456)
   d. Set payout_status = 'pending'
5. Return response with voucher finalization status
```

---

### API Flow: Multi-Bill Payment

**Step 1: Get Unpaid Bills**
```bash
GET /v1/bills/unpaid?business_id=uuid-business

Response:
{
  "bills": [
    {
      "bill_id": "bill-uuid-1",
      "invoice_number": "INV-001",
      "payee_name": "Vendor A",
      "amount": 50000,
      "due_date": "2024-10-15"
    },
    {
      "bill_id": "bill-uuid-2",
      "invoice_number": "INV-002",
      "payee_name": "Vendor B",
      "amount": 30000,
      "due_date": "2024-10-20"
    }
  ]
}
```

**Step 2: Create Multiple Draft Vouchers**
```bash
POST /v1/vouchers/multi
{
  "business_id": "uuid-business",
  "user_id": "uuid-user",
  "vouchers": [
    {
      "payee_id": "uuid-vendor-a",
      "account_id": "uuid-account-1",
      "invoice_number": "INV-001",
      "net_amount": 50000
    },
    {
      "payee_id": "uuid-vendor-b",
      "account_id": "uuid-account-2",
      "invoice_number": "INV-002",
      "net_amount": 30000
    }
  ]
}

Response:
{
  "vouchers": [
    { "voucher_id": "v-uuid-1", "voucher_number": "PV-2024-00001" },
    { "voucher_id": "v-uuid-2", "voucher_number": "PV-2024-00002" }
  ]
}
```

**Step 3: Make Aggregated Payment**
```bash
POST /v1/process_payment/make_payment
{
  "amount": 80000,  # Sum of all vouchers
  "voucher_ids": ["v-uuid-1", "v-uuid-2"],
  "payment_method": "netbanking",
  "net_banking_details": {
    "bank_code": "HDFC",
    "bank_name": "HDFC Bank"
  }
}

Response:
{
  "payment_id": "p-uuid-789",
  "voucher_status": "finalized",
  "vouchers_finalized": ["v-uuid-1", "v-uuid-2"]
}
```

**Database State After Payment**:
```sql
-- One payment record
payments:
  payment_id: p-uuid-789
  amount: 80000
  status: success

-- Two vouchers, both linked to same payment
payment_vouchers:
  voucher_id: v-uuid-1, payment_id: p-uuid-789, payee_id: vendor-a, amount: 50000
  voucher_id: v-uuid-2, payment_id: p-uuid-789, payee_id: vendor-b, amount: 30000
```

---

## Future Enhancements

### Phase 7: Payout API Integration

**Scope**: Integrate with payout gateway (Razorpay Payouts, Cashfree Payouts)

**Files to Create**:
- `internal/adapters/external/payout/razorpay_payout_client.go`
- `internal/services/payout/payout_service.go`
- `internal/handlers/http/payout/payout_handler.go`

**API Endpoint**:
```bash
POST /v1/payouts/initiate
{
  "voucher_ids": ["v-uuid-1", "v-uuid-2"],
  "payout_mode": "IMPS"  # IMPS, NEFT, RTGS
}
```

### Phase 8: Payout Reconciliation

**Scope**: Automatic reconciliation via webhook + manual upload

**Features**:
- Webhook from payout gateway (status updates)
- Bank statement upload (CSV/Excel parsing)
- UTR matching with vouchers
- Discrepancy alerts

---

## Document Maintenance

**Last Updated**: 2025-10-03
**Updated By**: Claude Code
**Next Review**: After Phase 6 completion

**Change Log**:
- 2025-10-03: Initial document creation (Phase 6 planning)
