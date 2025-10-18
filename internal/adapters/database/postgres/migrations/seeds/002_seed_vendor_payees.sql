-- #############################################################################
-- # Seed File 002: Vendor Payees Test Data
-- # Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
-- #
-- # PURPOSE: Seeds 3 vendor payees with bank accounts for testing
-- # USAGE: ./apply-seeds.sh apply 002
-- # ROLLBACK: ./apply-seeds.sh rollback 002
-- #############################################################################

\echo '=== Starting Vendor Payees Seed ==='

-- #############################################################################
-- # Vendor 1: TechCorp Solutions Pvt Ltd (IT Services)
-- #############################################################################

\echo 'Creating Vendor 1: TechCorp Solutions...'

-- Insert into payee_master (no display_name - removed in migration 000006)
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c7-1234-7001-a001-111111111111'::UUID, 'vendor', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert vendor details
INSERT INTO xcentro.payee_vendor (
    payee_id,
    gstin,
    legal_name,
    business_type,
    registration_date,
    gst_status,
    building_name,
    street,
    city,
    state,
    pincode,
    gst_verification_status,
    gst_verified_at,
    gst_data
)
VALUES (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '29AABCT1234M1Z5',
    'TechCorp Solutions Private Limited',
    'Private Limited Company',
    '2018-04-15',
    'Active',
    'Tech Park Building',
    '123 MG Road',
    'Bangalore',
    'Karnataka',
    '560001',
    'success',
    NOW() - INTERVAL '30 days',
    '{"directors": [{"name": "Rajesh Kumar", "din": "12345678"}], "turnover": "5CR", "registration_type": "Regular"}'::JSONB
)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert bank account for Vendor 1
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
    bank_verification_data,
    cross_verification_status,
    name_match_score,
    is_active,
    is_primary,
    account_label
)
VALUES (
    '0199f7c7-1234-7001-a002-111111111111'::UUID,
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '50200012345678',  -- Plaintext for now (encryption in Phase 5.5)
    '8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92',  -- SHA-256 hash
    'HDFC0001234',
    'HDFC Bank',
    'MG Road Branch',
    'current',
    'TechCorp Solutions Private Limited',
    TRUE,
    NOW() - INTERVAL '25 days',
    'success',
    '{"verified_name": "TechCorp Solutions Private Limited", "account_status": "Active"}'::JSONB,
    'success',
    0.95,
    TRUE,
    TRUE,
    'Primary Current Account'
)
ON CONFLICT (account_id) DO NOTHING;

-- Insert business-payee relation for Vendor 1
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
    '0199f7c7-1234-7001-a003-111111111111'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7001-a002-111111111111'::UUID,
    'TechCorp IT Services',
    'accounts@techcorp.com',
    '+91-80-12345678',
    'Primary IT vendor for software development and consulting',
    TRUE,
    TRUE,
    ARRAY['IT Services', 'Software Development', 'Consulting'],
    NOW() - INTERVAL '20 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Vendor 2: Global Office Supplies India Pvt Ltd (Office Supplies)
-- #############################################################################

\echo 'Creating Vendor 2: Global Office Supplies...'

-- Insert into payee_master (no display_name - removed in migration 000006)
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c7-1234-7002-a001-222222222222'::UUID, 'vendor', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert vendor details
INSERT INTO xcentro.payee_vendor (
    payee_id,
    gstin,
    legal_name,
    business_type,
    registration_date,
    gst_status,
    building_name,
    street,
    city,
    state,
    pincode,
    gst_verification_status,
    gst_verified_at,
    gst_data
)
VALUES (
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '27AADCG5432N1ZP',
    'Global Office Supplies India Private Limited',
    'Private Limited Company',
    '2015-08-20',
    'Active',
    'Commercial Complex',
    '456 Nehru Place',
    'Mumbai',
    'Maharashtra',
    '400001',
    'success',
    NOW() - INTERVAL '45 days',
    '{"directors": [{"name": "Priya Sharma", "din": "87654321"}], "turnover": "10CR", "registration_type": "Regular"}'::JSONB
)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert bank account for Vendor 2
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
    bank_verification_data,
    cross_verification_status,
    name_match_score,
    is_active,
    is_primary,
    account_label
)
VALUES (
    '0199f7c7-1234-7002-a002-222222222222'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '12345678901234',  -- Plaintext for now
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',  -- SHA-256 hash
    'ICIC0001234',
    'ICICI Bank',
    'Nehru Place Branch',
    'current',
    'Global Office Supplies India Private Limited',
    TRUE,
    NOW() - INTERVAL '40 days',
    'success',
    '{"verified_name": "Global Office Supplies India Pvt Ltd", "account_status": "Active"}'::JSONB,
    'success',
    0.92,
    TRUE,
    TRUE,
    'Primary Account'
)
ON CONFLICT (account_id) DO NOTHING;

-- Insert business-payee relation for Vendor 2
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
    '0199f7c7-1234-7002-a003-222222222222'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7002-a002-222222222222'::UUID,
    'Office Supplies - GOS',
    'billing@officesupplies.com',
    '+91-22-98765432',
    'Monthly office supplies vendor - stationery, furniture, equipment',
    TRUE,
    FALSE,
    ARRAY['Office Supplies', 'Stationery', 'Furniture'],
    NOW() - INTERVAL '35 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Vendor 3: BuildRight Constructions LLP (Contractor)
-- #############################################################################

\echo 'Creating Vendor 3: BuildRight Constructions...'

-- Insert into payee_master (no display_name - removed in migration 000006)
INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active)
VALUES
    ('0199f7c7-1234-7003-a001-333333333333'::UUID, 'vendor', TRUE)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert vendor details
INSERT INTO xcentro.payee_vendor (
    payee_id,
    gstin,
    legal_name,
    business_type,
    registration_date,
    gst_status,
    building_name,
    street,
    city,
    state,
    pincode,
    gst_verification_status,
    gst_verified_at,
    gst_data
)
VALUES (
    '0199f7c7-1234-7003-a001-333333333333'::UUID,
    '06AABFB9876K1ZL',
    'BuildRight Constructions LLP',
    'Limited Liability Partnership',
    '2016-03-10',
    'Active',
    'Construction House',
    '789 Sector 18',
    'Gurgaon',
    'Haryana',
    '122001',
    'success',
    NOW() - INTERVAL '60 days',
    '{"directors": [{"name": "Amit Singh", "din": "11223344"}, {"name": "Vikram Patel", "din": "55667788"}], "turnover": "25CR", "registration_type": "Regular"}'::JSONB
)
ON CONFLICT (payee_id) DO NOTHING;

-- Insert bank account for Vendor 3
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
    bank_verification_data,
    cross_verification_status,
    name_match_score,
    is_active,
    is_primary,
    account_label
)
VALUES (
    '0199f7c7-1234-7003-a002-333333333333'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID,
    '98765432109876',  -- Plaintext for now
    '9b871512327c09ce91dd649b3f96a63b7408ef267c8cc5710114e629730cb544',  -- SHA-256 hash
    'SBIN0012345',
    'State Bank of India',
    'Gurgaon Sector 18 Branch',
    'current',
    'BuildRight Constructions LLP',
    TRUE,
    NOW() - INTERVAL '55 days',
    'success',
    '{"verified_name": "BuildRight Constructions LLP", "account_status": "Active"}'::JSONB,
    'success',
    0.98,
    TRUE,
    TRUE,
    'Primary Business Account'
)
ON CONFLICT (account_id) DO NOTHING;

-- Insert business-payee relation for Vendor 3
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
    '0199f7c7-1234-7003-a003-333333333333'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID,
    '0199f7c7-1234-7003-a002-333333333333'::UUID,
    'BuildRight - Construction Contractor',
    'projects@buildright.com',
    '+91-124-55667788',
    'Primary contractor for office renovation and construction projects',
    TRUE,
    TRUE,
    ARRAY['Construction', 'Contractor', 'Civil Works'],
    NOW() - INTERVAL '50 days'
)
ON CONFLICT (business_id, payee_id, account_id) DO NOTHING;


-- #############################################################################
-- # Verification Output
-- #############################################################################

DO $$
DECLARE
    vendor_count INTEGER;
    relation_count INTEGER;
BEGIN
    -- Count vendors created
    SELECT COUNT(*) INTO vendor_count
    FROM xcentro.payee_master pm
    JOIN xcentro.payee_vendor pv ON pm.payee_id = pv.payee_id
    WHERE pm.payee_id IN (
        '0199f7c7-1234-7001-a001-111111111111'::UUID,
        '0199f7c7-1234-7002-a001-222222222222'::UUID,
        '0199f7c7-1234-7003-a001-333333333333'::UUID
    );

    -- Count business relations
    SELECT COUNT(*) INTO relation_count
    FROM xcentro.business_payee_relation
    WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID;

    RAISE NOTICE '=== Vendor Payees Seed Complete ===';
    RAISE NOTICE 'Vendors created: %', vendor_count;
    RAISE NOTICE 'Business relations: %', relation_count;
    RAISE NOTICE 'Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b';
    RAISE NOTICE '';
    RAISE NOTICE 'Vendor 1: TechCorp Solutions Pvt Ltd (IT Services)';
    RAISE NOTICE '  - GSTIN: 29AABCT1234M1Z5';
    RAISE NOTICE '  - Relation ID: 0199f7c7-1234-7001-a003-111111111111';
    RAISE NOTICE '';
    RAISE NOTICE 'Vendor 2: Global Office Supplies India Pvt Ltd';
    RAISE NOTICE '  - GSTIN: 27AADCG5432N1ZP';
    RAISE NOTICE '  - Relation ID: 0199f7c7-1234-7002-a003-222222222222';
    RAISE NOTICE '';
    RAISE NOTICE 'Vendor 3: BuildRight Constructions LLP';
    RAISE NOTICE '  - GSTIN: 06AABFB9876K1ZL';
    RAISE NOTICE '  - Relation ID: 0199f7c7-1234-7003-a003-333333333333';
END $$;

\echo '=== Seed complete! Use relation_id values in API requests ==='
