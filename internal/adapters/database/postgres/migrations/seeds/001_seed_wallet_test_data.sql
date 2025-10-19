-- #############################################################################
-- # Seed Data for Wallet Testing
-- # Purpose: Populate test data for payment wallet functionality testing
-- # Tables: business, stored_cards, bank_accounts
-- #############################################################################

-- NOTE: This seed file is for TESTING purposes only
-- Run this in your development/test environment to test wallet APIs
-- DO NOT run this in production

-- #############################################################################
-- # Test Business Entry
-- #############################################################################

-- Insert a test business for wallet testing
-- Using a fixed UUID for easy reference in API testing
-- INSERT INTO xcentro.business (
--     bid,
--     name,
--     gstin,
--     gst_data,
--     kyb_status,
--     business_type,
--     country_code,
--     currency_code
-- ) VALUES (
--     '01936d7a-7c8e-7890-b123-456789abcdef'::uuid,  -- Fixed UUID for testing
--     'Test Business Pvt Ltd',
--     '29ABCDE1234F1Z5',
--     '{"legal_name": "Test Business Private Limited", "registration_date": "2020-01-15", "registered_address": "123 Test Street, Bangalore, Karnataka 560001"}'::jsonb,
--     'kyc_completed'::xcentro.kyc_status_enum,
--     'private_limited_company'::xcentro.business_type_enum,
--     'IND',
--     'INR'
-- )
-- ON CONFLICT (bid) DO NOTHING;  -- Prevent duplicate insertion if already exists

-- COMMENT ON TABLE xcentro.business IS 'Added test business: Test Business Pvt Ltd for wallet testing';


-- #############################################################################
-- # Test Stored Cards (Tokenized Cards)
-- #############################################################################

-- Card 1: Visa Credit Card (routes to JioPay)
INSERT INTO xcentro.stored_cards (
    card_id,
    business_id,
    token,
    last_four_digits,
    card_type,
    card_category,
    expiry_month,
    expiry_year,
    issuer_bank,
    card_holder_name,
    is_active,
    gateway_used,
    added_at,
    metadata
) VALUES (
    '01936d7a-8000-7890-c001-111111111111'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,  -- References test business
    'jio_tok_visa_test_4532',
    '4532',
    'visa'::xcentro.card_type_enum,
    'credit'::xcentro.card_category_enum,
    '12',
    '2026',
    'HDFC Bank',
    'Test Business Account',
    TRUE,
    'jiopay'::xcentro.gateway_type_enum,
    NOW() - INTERVAL '60 days',  -- Added 2 months ago
    '{"tokenization_date": "2024-08-15", "gateway_used": "jiopay", "tokenization_id": "jio_tokenization_001", "mock_data": false}'::jsonb
)
ON CONFLICT (card_id) DO NOTHING;

-- Card 2: Mastercard Debit Card (routes to Easebuzz for amounts < ₹2000)
INSERT INTO xcentro.stored_cards (
    card_id,
    business_id,
    token,
    last_four_digits,
    card_type,
    card_category,
    expiry_month,
    expiry_year,
    issuer_bank,
    card_holder_name,
    is_active,
    gateway_used,
    added_at,
    last_used_at,
    metadata
) VALUES (
    '01936d7a-8000-7890-c002-222222222222'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    'ebz_tok_master_test_8765',
    '8765',
    'mastercard'::xcentro.card_type_enum,
    'debit'::xcentro.card_category_enum,
    '08',
    '2027',
    'ICICI Bank',
    'Test Business Account',
    TRUE,
    'easebuzz'::xcentro.gateway_type_enum,
    NOW() - INTERVAL '45 days',  -- Added 1.5 months ago
    NOW() - INTERVAL '5 days',    -- Last used 5 days ago
    '{"tokenization_date": "2024-09-01", "gateway_used": "easebuzz", "tokenization_id": "ebz_tokenization_001", "mock_data": false}'::jsonb
)
ON CONFLICT (card_id) DO NOTHING;

-- Card 3: RuPay Credit Card (routes to JioPay)
INSERT INTO xcentro.stored_cards (
    card_id,
    business_id,
    token,
    last_four_digits,
    card_type,
    card_category,
    expiry_month,
    expiry_year,
    issuer_bank,
    card_holder_name,
    is_active,
    gateway_used,
    added_at,
    metadata
) VALUES (
    '01936d7a-8000-7890-c003-333333333333'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    'jio_tok_rupay_test_5678',
    '5678',
    'rupay'::xcentro.card_type_enum,
    'credit'::xcentro.card_category_enum,
    '03',
    '2028',
    'State Bank of India',
    'Test Business Account',
    TRUE,
    'jiopay'::xcentro.gateway_type_enum,
    NOW() - INTERVAL '30 days',  -- Added 1 month ago
    '{"tokenization_date": "2024-09-17", "gateway_used": "jiopay", "tokenization_id": "jio_tokenization_002", "mock_data": false}'::jsonb
)
ON CONFLICT (card_id) DO NOTHING;

-- Card 4: American Express (routes to Easebuzz)
INSERT INTO xcentro.stored_cards (
    card_id,
    business_id,
    token,
    last_four_digits,
    card_type,
    card_category,
    expiry_month,
    expiry_year,
    issuer_bank,
    card_holder_name,
    is_active,
    gateway_used,
    added_at,
    metadata
) VALUES (
    '01936d7a-8000-7890-c004-444444444444'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    'ebz_tok_amex_test_1234',
    '1234',
    'amex'::xcentro.card_type_enum,
    'credit'::xcentro.card_category_enum,
    '11',
    '2025',
    'American Express',
    'Test Business Account',
    TRUE,
    'easebuzz'::xcentro.gateway_type_enum,
    NOW() - INTERVAL '15 days',  -- Added 15 days ago
    '{"tokenization_date": "2024-10-02", "gateway_used": "easebuzz", "tokenization_id": "ebz_tokenization_002", "mock_data": false}'::jsonb
)
ON CONFLICT (card_id) DO NOTHING;

-- Card 5: Inactive Visa Card (for testing filtering)
INSERT INTO xcentro.stored_cards (
    card_id,
    business_id,
    token,
    last_four_digits,
    card_type,
    card_category,
    expiry_month,
    expiry_year,
    issuer_bank,
    card_holder_name,
    is_active,
    gateway_used,
    added_at,
    metadata
) VALUES (
    '01936d7a-8000-7890-c005-555555555555'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    'jio_tok_visa_test_9999',
    '9999',
    'visa'::xcentro.card_type_enum,
    'credit'::xcentro.card_category_enum,
    '06',
    '2025',
    'Axis Bank',
    'Test Business Account',
    FALSE,  -- INACTIVE - should be filtered out in API
    'jiopay'::xcentro.gateway_type_enum,
    NOW() - INTERVAL '90 days',  -- Added 3 months ago
    '{"tokenization_date": "2024-07-17", "gateway_used": "jiopay", "deactivated_reason": "card_expired", "mock_data": false}'::jsonb
)
ON CONFLICT (card_id) DO NOTHING;

COMMENT ON TABLE xcentro.stored_cards IS 'Added 5 test cards: 4 active (Visa, Mastercard, RuPay, Amex) and 1 inactive for testing';


-- #############################################################################
-- # Test Bank Accounts
-- #############################################################################

-- For testing purposes, we'll use simple hashes and placeholder encrypted values
-- In production, proper encryption would be applied

-- Bank Account 1: HDFC Current Account (Verified)
INSERT INTO xcentro.bank_accounts (
    account_id,
    business_id,
    account_number_hash,
    account_number_encrypted,
    ifsc_code,
    account_holder_name,
    bank_name,
    branch_name,
    account_type,
    is_verified,
    is_active,
    added_at,
    metadata
) VALUES (
    '01936d7a-9000-7890-a001-111111111111'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    encode(sha256('1234567890123456'::bytea), 'hex'),  -- SHA-256 hash of account number
    'ENCRYPTED_1234567890123456',  -- Placeholder - in production would be AES-256-GCM encrypted
    'HDFC0001234',
    'Test Business Pvt Ltd',
    'HDFC Bank',
    'Corporate Branch, Bangalore',
    'current',
    TRUE,
    TRUE,
    NOW() - INTERVAL '60 days',
    '{"verification_status": "verified", "verification_date": "2024-08-16", "verification_amount": 100, "transaction_reference": "PENNY_DROP_HDC001", "provider": "mock_bank_verification", "last_five_digits": "23456", "mock_data": false}'::jsonb
)
ON CONFLICT (account_id) DO NOTHING;

-- Bank Account 2: ICICI Savings Account (Verified)
INSERT INTO xcentro.bank_accounts (
    account_id,
    business_id,
    account_number_hash,
    account_number_encrypted,
    ifsc_code,
    account_holder_name,
    bank_name,
    branch_name,
    account_type,
    is_verified,
    is_active,
    added_at,
    metadata
) VALUES (
    '01936d7a-9000-7890-a002-222222222222'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    encode(sha256('9876543210987654'::bytea), 'hex'),
    'ENCRYPTED_9876543210987654',
    'ICIC0001234',
    'Test Business Pvt Ltd',
    'ICICI Bank',
    'Business Banking Branch',
    'savings',
    TRUE,
    TRUE,
    NOW() - INTERVAL '45 days',
    '{"verification_status": "verified", "verification_date": "2024-09-02", "verification_amount": 100, "transaction_reference": "PENNY_DROP_ICI001", "provider": "mock_bank_verification", "last_five_digits": "87654", "mock_data": false}'::jsonb
)
ON CONFLICT (account_id) DO NOTHING;

-- Bank Account 3: State Bank Current Account (Verified)
INSERT INTO xcentro.bank_accounts (
    account_id,
    business_id,
    account_number_hash,
    account_number_encrypted,
    ifsc_code,
    account_holder_name,
    bank_name,
    branch_name,
    account_type,
    is_verified,
    is_active,
    added_at,
    metadata
) VALUES (
    '01936d7a-9000-7890-a003-333333333333'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    encode(sha256('5555666677778888'::bytea), 'hex'),
    'ENCRYPTED_5555666677778888',
    'SBIN0001234',
    'Test Business Pvt Ltd',
    'State Bank of India',
    'Corporate Branch',
    'current',
    TRUE,
    TRUE,
    NOW() - INTERVAL '30 days',
    '{"verification_status": "verified", "verification_date": "2024-09-17", "verification_amount": 100, "transaction_reference": "PENNY_DROP_SBI001", "provider": "mock_bank_verification", "last_five_digits": "78888", "mock_data": false}'::jsonb
)
ON CONFLICT (account_id) DO NOTHING;

-- Bank Account 4: Axis Bank Current Account (Unverified - for testing)
INSERT INTO xcentro.bank_accounts (
    account_id,
    business_id,
    account_number_hash,
    account_number_encrypted,
    ifsc_code,
    account_holder_name,
    bank_name,
    branch_name,
    account_type,
    is_verified,
    is_active,
    added_at,
    metadata
) VALUES (
    '01936d7a-9000-7890-a004-444444444444'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    encode(sha256('1111222233334444'::bytea), 'hex'),
    'ENCRYPTED_1111222233334444',
    'UTIB0001234',
    'Test Business Pvt Ltd',
    'Axis Bank',
    'Business Branch',
    'current',
    FALSE,  -- UNVERIFIED - should be filtered out in API
    TRUE,
    NOW() - INTERVAL '15 days',
    '{"verification_status": "pending", "verification_date": null, "last_five_digits": "34444", "mock_data": false}'::jsonb
)
ON CONFLICT (account_id) DO NOTHING;

-- Bank Account 5: Inactive HDFC Account (for testing filtering)
INSERT INTO xcentro.bank_accounts (
    account_id,
    business_id,
    account_number_hash,
    account_number_encrypted,
    ifsc_code,
    account_holder_name,
    bank_name,
    branch_name,
    account_type,
    is_verified,
    is_active,
    added_at,
    metadata
) VALUES (
    '01936d7a-9000-7890-a005-555555555555'::uuid,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid,
    encode(sha256('7777888899990000'::bytea), 'hex'),
    'ENCRYPTED_7777888899990000',
    'HDFC0005678',
    'Test Business Pvt Ltd',
    'HDFC Bank',
    'Indiranagar Branch',
    'current',
    TRUE,
    FALSE,  -- INACTIVE - should be filtered out in API
    NOW() - INTERVAL '90 days',
    '{"verification_status": "verified", "verification_date": "2024-07-17", "deactivated_reason": "account_closed", "last_five_digits": "90000", "mock_data": false}'::jsonb
)
ON CONFLICT (account_id) DO NOTHING;

COMMENT ON TABLE xcentro.bank_accounts IS 'Added 5 test bank accounts: 3 active verified, 1 unverified, 1 inactive for testing';


-- #############################################################################
-- # Verification and Summary
-- #############################################################################

-- Show summary of seeded data
DO $$
DECLARE
    business_count INT;
    cards_count INT;
    active_cards_count INT;
    accounts_count INT;
    verified_accounts_count INT;
BEGIN
    SELECT COUNT(*) INTO business_count FROM xcentro.business WHERE bid = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid;
    SELECT COUNT(*) INTO cards_count FROM xcentro.stored_cards WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid;
    SELECT COUNT(*) INTO active_cards_count FROM xcentro.stored_cards WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid AND is_active = TRUE;
    SELECT COUNT(*) INTO accounts_count FROM xcentro.bank_accounts WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid;
    SELECT COUNT(*) INTO verified_accounts_count FROM xcentro.bank_accounts WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::uuid AND is_verified = TRUE AND is_active = TRUE;

    RAISE NOTICE '========================================';
    RAISE NOTICE 'Wallet Test Data Seeding Complete!';
    RAISE NOTICE '========================================';
    RAISE NOTICE 'Test Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b';
    RAISE NOTICE 'Businesses: %', business_count;
    RAISE NOTICE 'Total Stored Cards: % (% active)', cards_count, active_cards_count;
    RAISE NOTICE 'Total Bank Accounts: % (% verified & active)', accounts_count, verified_accounts_count;
    RAISE NOTICE '========================================';
    RAISE NOTICE 'Use this business_id for API testing in Postman';
    RAISE NOTICE '========================================';
END $$;
