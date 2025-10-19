# Database Seed Files

This directory contains SQL seed files for populating test data in development and testing environments.

## ⚠️ Important Notice

**These seed files are for DEVELOPMENT and TESTING purposes ONLY.**
**DO NOT run these scripts in production environments.**

## Available Seed Files

### 1. Wallet Test Data

**File:** `001_seed_wallet_test_data.sql`
**Purpose:** Seeds test data for wallet/payment functionality testing
**Rollback:** `001_seed_wallet_test_data_rollback.sql`

#### What It Creates

1. **Test Business**
   - Business ID: `01936d7a-7c8e-7890-b123-456789abcdef`
   - Name: Test Business Pvt Ltd
   - GSTIN: 29ABCDE1234F1Z5
   - Status: KYB Completed

2. **Stored Cards (5 total)**
   - ✅ 4 Active Cards:
     - Visa Credit (HDFC) - last 4: 4532 → Routes to JioPay
     - Mastercard Debit (ICICI) - last 4: 8765 → Routes to Easebuzz
     - RuPay Credit (SBI) - last 4: 5678 → Routes to JioPay
     - Amex Credit - last 4: 1234 → Routes to Easebuzz
   - ❌ 1 Inactive Card (for testing filtering)

3. **Bank Accounts (5 total)**
   - ✅ 3 Active & Verified:
     - HDFC Current Account (last 5: 23456)
     - ICICI Savings Account (last 5: 87654)
     - SBI Current Account (last 5: 78888)
   - ⏳ 1 Unverified (for testing filtering)
   - ❌ 1 Inactive (for testing filtering)

### 2. Vendor Payees Test Data

**File:** `002_seed_vendor_payees.sql`
**Purpose:** Seeds vendor payees with bank accounts for voucher/payment testing
**Rollback:** `002_seed_vendor_payees_rollback.sql`
**Business ID:** `0199f7c6-39ab-77a1-b611-385a4cb8f24b`

#### What It Creates

1. **Vendor 1: TechCorp Solutions Pvt Ltd** (IT Services)
   - Payee ID: `0199f7c7-1234-7001-a001-111111111111`
   - Relation ID: `0199f7c7-1234-7001-a003-111111111111`
   - GSTIN: `29AABCT1234M1Z5`
   - Location: Bangalore, Karnataka
   - Bank: HDFC Bank (Current Account)
   - Tags: IT Services, Software Development, Consulting
   - Is Favorite: ✅

2. **Vendor 2: Global Office Supplies India Pvt Ltd** (Office Supplies)
   - Payee ID: `0199f7c7-1234-7002-a001-222222222222`
   - Relation ID: `0199f7c7-1234-7002-a003-222222222222`
   - GSTIN: `27AADCG5432N1ZP`
   - Location: Mumbai, Maharashtra
   - Bank: ICICI Bank (Current Account)
   - Tags: Office Supplies, Stationery, Furniture

3. **Vendor 3: BuildRight Constructions LLP** (Contractor)
   - Payee ID: `0199f7c7-1234-7003-a001-333333333333`
   - Relation ID: `0199f7c7-1234-7003-a003-333333333333`
   - GSTIN: `06AABFB9876K1ZL`
   - Location: Gurgaon, Haryana
   - Bank: State Bank of India (Current Account)
   - Tags: Construction, Contractor, Civil Works
   - Is Favorite: ✅

**All vendors include:**
- ✅ GST verification (status: success)
- ✅ Bank account verification (penny drop: success)
- ✅ Cross-verification (name match: 92-98%)
- ✅ Active status
- ✅ Complete business details (directors, turnover, registration)

### 3. Employee & Biller Payees Test Data

**File:** `003_seed_employee_biller_payees.sql`
**Purpose:** Seeds employee payees (payroll) and biller payees (BBPS) for payee diversity testing
**Rollback:** `003_seed_employee_biller_payees_rollback.sql`
**Business ID:** `0199f7c6-39ab-77a1-b611-385a4cb8f24b` (same as seed 002)

#### What It Creates

1. **Employee 1: Rajesh Kumar** (Senior Software Engineer)
   - Payee ID: `0199f7c8-1234-8001-e001-111111111111`
   - Relation ID: `0199f7c8-1234-8001-e003-111111111111`
   - Employee Code: EMP001
   - PAN: `ABCDE1234F`
   - Department: Engineering
   - Bank: HDFC Bank (Savings Account)
   - Tags: Employee, Engineering, Payroll
   - Is Favorite: ❌

2. **Employee 2: Priya Sharma** (HR Manager)
   - Payee ID: `0199f7c8-1234-8002-e001-222222222222`
   - Relation ID: `0199f7c8-1234-8002-e003-222222222222`
   - Employee Code: EMP002
   - PAN: `FGHIJ5678K`
   - Department: Human Resources
   - Bank: ICICI Bank (Savings Account)
   - Tags: Employee, HR, Payroll, Management
   - Is Favorite: ✅

3. **Biller 1: Airtel Mobile Postpaid** (Telecom)
   - Payee ID: `0199f7c8-1234-9001-b001-333333333333`
   - Relation ID: `0199f7c8-1234-9001-b003-333333333333`
   - Biller ID: `AIRTEL_MOBILE`
   - Category: Telecom
   - Mobile: +91-9988776655
   - Last Bill: ₹2499.00
   - Auto-fetch: ✅ Enabled (monthly, day 5)
   - Tags: Telecom, Utility, Monthly Bills
   - Is Favorite: ✅

**All payees include:**
- ✅ PAN verification for employees (status: success)
- ✅ Bank account verification for employees (penny drop: success)
- ✅ Cross-verification (name match: 96-98%)
- ✅ BBPS registration for biller (NPCI registered)
- ✅ Active status
- ✅ Complete employment/biller details

## How to Use

### Running Seed Scripts

#### Method 1: Using Helper Script (Recommended)

```bash
# Navigate to seeds directory
cd internal/adapters/database/postgres/migrations/seeds

# Apply wallet test data
./apply-seeds.sh apply 001

# Apply vendor payees test data
./apply-seeds.sh apply 002

# Apply employee & biller payees test data
./apply-seeds.sh apply 003

# Rollback wallet test data
./apply-seeds.sh rollback 001

# Rollback vendor payees
./apply-seeds.sh rollback 002

# Rollback employee & biller payees
./apply-seeds.sh rollback 003

# List available seed files
./apply-seeds.sh list

# Apply all seeds
./apply-seeds.sh apply-all
```

#### Method 2: Using Docker Directly

```bash
# Apply seed data
cat internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data.sql | \
  docker exec -i xcentro-postgres psql -U xcentro_admin -d xcentro

# Rollback seed data
cat internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data_rollback.sql | \
  docker exec -i xcentro-postgres psql -U xcentro_admin -d xcentro
```

#### Method 3: Using existing connect-postgres.sh script

```bash
cd deployments/docker

# Apply seed data
./connect-postgres.sh admin -f ../../internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data.sql

# Rollback seed data
./connect-postgres.sh admin -f ../../internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data_rollback.sql
```

#### Method 4: Using DBeaver or other GUI tools

1. Open the seed SQL file
2. Connect to your development database
3. Execute the entire script
4. Check the output messages for confirmation

## Testing with Postman

After seeding the data, use this **business_id** in your Postman requests:

```json
{
  "business_id": "01936d7a-7c8e-7890-b123-456789abcdef",
  "payment_context": "vendor_payment",
  "base_amount": 100000
}
```

### Example API Request

**Endpoint:** `POST /api/v1/process_payment/get_payment_sources_with_fees`

**Headers:**
```
Authorization: Bearer <your-jwt-token>
Content-Type: application/json
```

**Request Body:**
```json
{
  "business_id": "01936d7a-7c8e-7890-b123-456789abcdef",
  "payment_context": "vendor_payment",
  "base_amount": 100000
}
```

**Expected Response:**
```json
{
  "message": "Payment sources with fees retrieved successfully",
  "status": "success",
  "data": {
    "business_id": "01936d7a-7c8e-7890-b123-456789abcdef",
    "payment_context": "vendor_payment",
    "base_amount": 100000,
    "sources": [
      {
        "source_id": "01936d7a-8000-7890-c001-111111111111",
        "source_type": "card",
        "payment_method": "token_card",
        "display_name": "Visa •••• 4532",
        "card_type": "visa",
        "card_category": "credit",
        "last_four_digits": "4532",
        "is_allowed": true,
        "fee_breakdown": {
          "base_amount": 100000,
          "convenience_fee": 2500,
          "convenience_fee_gst": 450,
          "total_fee": 2950,
          "total_amount": 102950,
          "convenience_fee_percent": 2.5,
          "gst_percent": 18.0
        }
      },
      // ... more cards ...
      {
        "source_id": "01936d7a-9000-7890-a001-111111111111",
        "source_type": "bank_account",
        "payment_method": "bank_account",
        "display_name": "HDFC Bank ••• 23456",
        "bank_name": "HDFC Bank",
        "last_five_digits": "23456",
        "ifsc_code": "HDFC0001234",
        "is_allowed": true,
        "fee_breakdown": {
          "base_amount": 100000,
          "convenience_fee": 0,
          "convenience_fee_gst": 0,
          "total_fee": 0,
          "total_amount": 100000,
          "convenience_fee_percent": 0.0,
          "gst_percent": 0.0
        }
      }
      // ... more bank accounts ...
    ]
  }
}
```

## Verification Queries

After seeding, you can verify the data with these SQL queries:

```sql
-- Check business
SELECT bid, name, gstin, kyb_status
FROM xcentro.business
WHERE bid = '01936d7a-7c8e-7890-b123-456789abcdef';

-- Check stored cards (should show 4 active)
SELECT card_id, last_four_digits, card_type, card_category, gateway_used, is_active
FROM xcentro.stored_cards
WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'
ORDER BY added_at DESC;

-- Check bank accounts (should show 3 verified & active)
SELECT account_id, bank_name, account_type, is_verified, is_active,
       metadata->>'last_five_digits' as last_five
FROM xcentro.bank_accounts
WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'
ORDER BY added_at DESC;

-- Summary counts
SELECT
    (SELECT COUNT(*) FROM xcentro.stored_cards
     WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef' AND is_active = TRUE) as active_cards,
    (SELECT COUNT(*) FROM xcentro.bank_accounts
     WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'
     AND is_verified = TRUE AND is_active = TRUE) as verified_accounts;
```

### Testing Vendor Payees (Seed 002)

Use the vendor payees for creating vouchers and testing voucher/payee workflows:

**Business ID:** `0199f7c6-39ab-77a1-b611-385a4cb8f24b`

#### Example: Create Draft Voucher

**Endpoint:** `POST /v1/vouchers/draft`

**Request Body:**
```json
{
  "business_id": "0199f7c6-39ab-77a1-b611-385a4cb8f24b",
  "user_id": "<your-user-id>",
  "relation_id": "0199f7c7-1234-7001-a003-111111111111",
  "invoice_number": "INV-2024-001",
  "invoice_date": "2024-10-01T00:00:00Z",
  "gross_amount": 118000,
  "net_amount": 100000,
  "currency": "INR",
  "gst_amount": 18000,
  "gst_rate": 18.0,
  "tds_amount": 0,
  "expense_category": "IT Services",
  "description": "Software development services for Q4 2024"
}
```

#### Example: List Vendor Payees

**Endpoint:** `GET /v1/payees?bid=0199f7c6-39ab-77a1-b611-385a4cb8f24b`

**Expected Response:**
```json
{
  "status": "success",
  "data": {
    "payees": [
      {
        "relation_id": "0199f7c7-1234-7001-a003-111111111111",
        "display_name": "TechCorp IT Services",
        "gstin": "29AABCT1234M1Z5",
        "tags": ["IT Services", "Software Development", "Consulting"],
        "is_favorite": true
      },
      // ... more vendors
    ]
  }
}
```

### Vendor Payee Verification Queries

```sql
-- Check all 3 vendors were created
SELECT
    pm.payee_id,
    pm.display_name,
    pv.gstin,
    pv.legal_name,
    pv.city,
    pv.state
FROM xcentro.payee_master pm
JOIN xcentro.payee_vendor pv ON pm.payee_id = pv.payee_id
WHERE pm.payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Check business-payee relations
SELECT
    bpr.relation_id,
    bpr.display_name,
    bpr.contact_email,
    bpr.tags,
    bpr.is_favorite,
    pm.display_name as payee_legal_name
FROM xcentro.business_payee_relation bpr
JOIN xcentro.payee_master pm ON bpr.payee_id = pm.payee_id
WHERE bpr.business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID
ORDER BY bpr.added_at;

-- Check bank accounts are verified
SELECT
    pba.account_id,
    pm.display_name,
    pba.bank_name,
    pba.ifsc_code,
    pba.is_verified,
    pba.bank_verification_status,
    pba.cross_verification_status,
    pba.name_match_score
FROM xcentro.payee_bank_accounts pba
JOIN xcentro.payee_master pm ON pba.payee_id = pm.payee_id
WHERE pba.payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Summary: Should show 3 vendors, 3 accounts, 3 relations
SELECT
    (SELECT COUNT(*) FROM xcentro.payee_master
     WHERE payee_id IN (
        '0199f7c7-1234-7001-a001-111111111111'::UUID,
        '0199f7c7-1234-7002-a001-222222222222'::UUID,
        '0199f7c7-1234-7003-a001-333333333333'::UUID
     )) as vendor_count,
    (SELECT COUNT(*) FROM xcentro.payee_bank_accounts
     WHERE payee_id IN (
        '0199f7c7-1234-7001-a001-111111111111'::UUID,
        '0199f7c7-1234-7002-a001-222222222222'::UUID,
        '0199f7c7-1234-7003-a001-333333333333'::UUID
     ) AND is_verified = TRUE) as verified_accounts,
    (SELECT COUNT(*) FROM xcentro.business_payee_relation
     WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID) as business_relations;
```

## Clean Up

When you're done testing, clean up the test data:

```bash
psql -h localhost -U xcentro_admin -d xcentro_db -f internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data_rollback.sql
```

## Adding New Seed Files

When creating new seed files, follow this naming convention:

```
NNN_seed_<feature>_test_data.sql
NNN_seed_<feature>_test_data_rollback.sql
```

Where `NNN` is a sequential number (002, 003, etc.)

### Guidelines for New Seed Files

1. **Always include a rollback script**
2. **Use fixed UUIDs** for easy testing (like `01936d7a-...`)
3. **Use `ON CONFLICT DO NOTHING`** to prevent duplicate insertions
4. **Add verification output** using `DO $$ ... RAISE NOTICE ... $$`
5. **Document the test IDs** in this README
6. **Include realistic test data** that matches production patterns
7. **Add comments** explaining the purpose of each data entry

## Troubleshooting

### Error: Duplicate Key Violation

If you see duplicate key errors, the data already exists. Run the rollback script first:

```bash
psql -h localhost -U xcentro_admin -d xcentro_db -f internal/adapters/database/postgres/migrations/seeds/001_seed_wallet_test_data_rollback.sql
```

### Error: Foreign Key Constraint Violation

Make sure all migrations are up to date:

```bash
go build -o bin/migrate ./cmd/migrate && ./bin/migrate -action=up
```

### Permission Errors

Use the `xcentro_admin` user for seeding (not `xcentro_rw`), as it has DDL permissions.

## Notes

- Seed files use `ON CONFLICT DO NOTHING` to safely re-run without errors
- UUIDs are manually specified for consistency across environments
- Encrypted fields use placeholder values (real encryption in production)
- Timestamps are relative (e.g., `NOW() - INTERVAL '60 days'`)
- Metadata includes `"mock_data": false` to distinguish from in-memory mocks

## Related Documentation

- [CLAUDE.md](../../../../../CLAUDE.md) - Complete API documentation
- [Migration Guide](../../../../docs/migrations.md) - Database migration instructions
- [Payment Module Documentation](../../../../docs/payment_module.md) - Payment system details
