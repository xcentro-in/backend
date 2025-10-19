-- #############################################################################
-- # Seed File 003: Employee & Biller Payees Test Data
-- # Purpose: Seed data for payroll employees and BBPS billers
-- # Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b (from seed 002)
-- #############################################################################

\echo ''
\echo '========================================================================='
\echo '  Seed 003: Employee & Biller Payees Test Data'
\echo '========================================================================='
\echo ''

-- Use the same business from seed 002
-- Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b


-- #############################################################################
-- # Employee 1: Rajesh Kumar (Senior Software Engineer)
-- #############################################################################

\echo 'Creating Employee 1: Rajesh Kumar (Senior Software Engineer)...'

-- Insert into payee_master
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c8-1234-8001-e001-111111111111'::UUID, 'employee', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert employee details
INSERT INTO xcentro.payee_employee (
    payee_id,
    employee_code,
    full_name,
    pan,
    email,
    phone,
    designation,
    department,
    date_of_joining,
    employment_type,
    pan_verification_status,
    is_sensitive
)
VALUES (
    '0199f7c8-1234-8001-e001-111111111111'::UUID,
    'EMP001',
    'Rajesh Kumar',
    'ABCDE1234F',
    'rajesh.kumar@company.com',
    '+91-9876543210',
    'Senior Software Engineer',
    'Engineering',
    '2022-03-15',
    'full-time',
    'success',
    TRUE
)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert bank account for Employee 1
INSERT INTO xcentro.payee_bank_accounts (
    account_id,
    payee_id,
    account_number_encrypted,
    account_number_hash,
    ifsc_code,
    bank_name,
    branch_name,
    account_type,
    account_holder_name,
    is_verified,
    verified_at,
    bank_verification_status,
    cross_verification_status,
    name_match_score,
    is_active,
    is_primary,
    account_label
)
VALUES (
    '0199f7c8-1234-8001-e002-111111111111'::UUID,
    '0199f7c8-1234-8001-e001-111111111111'::UUID,
    '9876543210123456',  -- Plaintext for now (Phase 5.5: encryption)
    encode(sha256('9876543210123456'::bytea), 'hex'),
    'HDFC0001234',
    'HDFC Bank',
    'MG Road Branch, Bangalore',
    'savings',
    'Rajesh Kumar',
    TRUE,
    NOW() - INTERVAL '30 days',
    'success',
    'success',
    0.98,
    TRUE,
    TRUE,
    'Primary Salary Account'
)
ON CONFLICT (account_id) DO NOTHING;

-- Insert business-payee relation for Employee 1
INSERT INTO xcentro.business_payee_relation (
    relation_id,
    business_id,
    payee_id,
    account_id,
    display_name,
    contact_email,
    contact_phone,
    notes,
    is_active,
    is_favorite,
    tags,
    added_at
)
VALUES (
    '0199f7c8-1234-8001-e003-111111111111'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c8-1234-8001-e001-111111111111'::UUID,
    '0199f7c8-1234-8001-e002-111111111111'::UUID,
    'Rajesh Kumar - Engineering',
    'rajesh.kumar@company.com',
    '+91-9876543210',
    'Senior software engineer in product team',
    TRUE,
    FALSE,
    ARRAY['Employee', 'Engineering', 'Payroll'],
    NOW() - INTERVAL '30 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Employee 2: Priya Sharma (HR Manager)
-- #############################################################################

\echo 'Creating Employee 2: Priya Sharma (HR Manager)...'

-- Insert into payee_master
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c8-1234-8002-e001-222222222222'::UUID, 'employee', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert employee details
INSERT INTO xcentro.payee_employee (
    payee_id,
    employee_code,
    full_name,
    pan,
    email,
    phone,
    designation,
    department,
    date_of_joining,
    employment_type,
    pan_verification_status,
    is_sensitive
)
VALUES (
    '0199f7c8-1234-8002-e001-222222222222'::UUID,
    'EMP002',
    'Priya Sharma',
    'FGHIJ5678K',
    'priya.sharma@company.com',
    '+91-9123456789',
    'HR Manager',
    'Human Resources',
    '2021-06-01',
    'full-time',
    'success',
    TRUE
)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert bank account for Employee 2
INSERT INTO xcentro.payee_bank_accounts (
    account_id,
    payee_id,
    account_number_encrypted,
    account_number_hash,
    ifsc_code,
    bank_name,
    branch_name,
    account_type,
    account_holder_name,
    is_verified,
    verified_at,
    bank_verification_status,
    cross_verification_status,
    name_match_score,
    is_active,
    is_primary,
    account_label
)
VALUES (
    '0199f7c8-1234-8002-e002-222222222222'::UUID,
    '0199f7c8-1234-8002-e001-222222222222'::UUID,
    '5432109876543210',  -- Plaintext for now (Phase 5.5: encryption)
    encode(sha256('5432109876543210'::bytea), 'hex'),
    'ICIC0001234',
    'ICICI Bank',
    'Koramangala Branch, Bangalore',
    'savings',
    'Priya Sharma',
    TRUE,
    NOW() - INTERVAL '25 days',
    'success',
    'success',
    0.96,
    TRUE,
    TRUE,
    'Primary Salary Account'
)
ON CONFLICT (account_id) DO NOTHING;

-- Insert business-payee relation for Employee 2
INSERT INTO xcentro.business_payee_relation (
    relation_id,
    business_id,
    payee_id,
    account_id,
    display_name,
    contact_email,
    contact_phone,
    notes,
    is_active,
    is_favorite,
    tags,
    added_at
)
VALUES (
    '0199f7c8-1234-8002-e003-222222222222'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c8-1234-8002-e001-222222222222'::UUID,
    '0199f7c8-1234-8002-e002-222222222222'::UUID,
    'Priya Sharma - HR',
    'priya.sharma@company.com',
    '+91-9123456789',
    'HR Manager responsible for employee benefits',
    TRUE,
    TRUE,
    ARRAY['Employee', 'HR', 'Payroll', 'Management'],
    NOW() - INTERVAL '25 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Biller 1: Airtel (Telecom - Mobile Bill)
-- #############################################################################

\echo 'Creating Biller 1: Airtel (Telecom - Mobile Bill)...'

-- Insert into payee_master
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c8-1234-9001-b001-333333333333'::UUID, 'biller', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert biller details
INSERT INTO xcentro.payee_biller (
    payee_id,
    biller_id,
    biller_name,
    biller_category,
    biller_description,
    bbps_biller_code,
    npci_registered,
    supports_adhoc,
    supports_recurring,
    payment_modes,
    avg_processing_time_hours
)
VALUES (
    '0199f7c8-1234-9001-b001-333333333333'::UUID,
    'AIRTEL_MOBILE',
    'Airtel Mobile Postpaid',
    'Telecom',
    'Airtel mobile postpaid bill payments via BBPS',
    'BBPS_AIRTEL_001',
    TRUE,
    TRUE,
    TRUE,
    ARRAY['UPI', 'Net Banking', 'Debit Card', 'Credit Card'],
    2
)
ON CONFLICT (biller_id) DO NOTHING;

-- Insert biller account configuration for the business
INSERT INTO xcentro.payee_biller_accounts (
    biller_account_id,
    payee_id,
    business_id,
    mobile_number,
    customer_params,
    auto_fetch_enabled,
    fetch_schedule,
    preferred_fetch_day,
    last_bill_fetched_at,
    last_bill_amount,
    last_bill_due_date,
    is_active
)
VALUES (
    '0199f7c8-1234-9001-b002-333333333333'::UUID,
    '0199f7c8-1234-9001-b001-333333333333'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '+91-9988776655',
    '{"mobile_number": "+91-9988776655", "registered_name": "XCentro Private Limited"}'::JSONB,
    TRUE,
    'monthly',
    5,
    NOW() - INTERVAL '3 days',
    249900,  -- ₹2499.00 (last bill)
    CURRENT_DATE + INTERVAL '12 days',
    TRUE
)
ON CONFLICT (business_id, payee_id, mobile_number) DO NOTHING;

-- Insert business-payee relation for Biller 1
INSERT INTO xcentro.business_payee_relation (
    relation_id,
    business_id,
    payee_id,
    account_id,
    display_name,
    contact_email,
    contact_phone,
    notes,
    is_active,
    is_favorite,
    tags,
    added_at
)
VALUES (
    '0199f7c8-1234-9001-b003-333333333333'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c8-1234-9001-b001-333333333333'::UUID,
    '0199f7c8-1234-9001-b002-333333333333'::UUID,  -- References biller_account_id
    'Airtel Corporate Mobile',
    'billing@company.com',
    '+91-9988776655',
    'Corporate mobile connection for office communication',
    TRUE,
    TRUE,
    ARRAY['Telecom', 'Utility', 'Monthly Bills'],
    NOW() - INTERVAL '60 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Verification Output
-- #############################################################################

DO $$
DECLARE
    emp_count INT;
    biller_count INT;
    relation_count INT;
BEGIN
    -- Count employees
    SELECT COUNT(*) INTO emp_count
    FROM xcentro.payee_master
    WHERE payee_id IN (
        '0199f7c8-1234-8001-e001-111111111111'::UUID,
        '0199f7c8-1234-8002-e001-222222222222'::UUID
    );

    -- Count billers
    SELECT COUNT(*) INTO biller_count
    FROM xcentro.payee_master
    WHERE payee_id = '0199f7c8-1234-9001-b001-333333333333'::UUID;

    -- Count business relations for this seed
    SELECT COUNT(*) INTO relation_count
    FROM xcentro.business_payee_relation
    WHERE relation_id IN (
        '0199f7c8-1234-8001-e003-111111111111'::UUID,
        '0199f7c8-1234-8002-e003-222222222222'::UUID,
        '0199f7c8-1234-9001-b003-333333333333'::UUID
    );

    RAISE NOTICE '';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE ' Seed 003 Applied Successfully';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE ' Employees Created: %', emp_count;
    RAISE NOTICE ' Billers Created: %', biller_count;
    RAISE NOTICE ' Business Relations Created: %', relation_count;
    RAISE NOTICE '';
    RAISE NOTICE ' Employee 1: Rajesh Kumar (EMP001) - Engineering';
    RAISE NOTICE '   - PAN: ABCDE1234F';
    RAISE NOTICE '   - Bank: HDFC Bank (Savings)';
    RAISE NOTICE '   - Relation ID: 0199f7c8-1234-8001-e003-111111111111';
    RAISE NOTICE '';
    RAISE NOTICE ' Employee 2: Priya Sharma (EMP002) - HR';
    RAISE NOTICE '   - PAN: FGHIJ5678K';
    RAISE NOTICE '   - Bank: ICICI Bank (Savings)';
    RAISE NOTICE '   - Relation ID: 0199f7c8-1234-8002-e003-222222222222';
    RAISE NOTICE '';
    RAISE NOTICE ' Biller 1: Airtel Mobile Postpaid';
    RAISE NOTICE '   - Category: Telecom';
    RAISE NOTICE '   - Mobile: +91-9988776655';
    RAISE NOTICE '   - Relation ID: 0199f7c8-1234-9001-b003-333333333333';
    RAISE NOTICE '';
    RAISE NOTICE ' Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE '';
END $$;
