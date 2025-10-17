-- #############################################################################
-- # Rollback/Cleanup Script for Wallet Test Data
-- # Purpose: Remove seeded test data for wallet functionality
-- # Run this to clean up test data after testing is complete
-- #############################################################################

-- NOTE: This will DELETE all test data created by 001_seed_wallet_test_data.sql
-- Run this in your development/test environment to clean up
-- DO NOT run this in production unless you're absolutely sure

-- #############################################################################
-- # Delete Test Data (in reverse order due to foreign keys)
-- #############################################################################

-- Delete test bank accounts
DELETE FROM xcentro.bank_accounts
WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;

-- Delete test stored cards
DELETE FROM xcentro.stored_cards
WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;

-- Delete test business
DELETE FROM xcentro.business
WHERE bid = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;

-- #############################################################################
-- # Verification
-- #############################################################################

DO $$
DECLARE
    business_count INT;
    cards_count INT;
    accounts_count INT;
BEGIN
    SELECT COUNT(*) INTO business_count FROM xcentro.business WHERE bid = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;
    SELECT COUNT(*) INTO cards_count FROM xcentro.stored_cards WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;
    SELECT COUNT(*) INTO accounts_count FROM xcentro.bank_accounts WHERE business_id = '01936d7a-7c8e-7890-b123-456789abcdef'::uuid;

    RAISE NOTICE '========================================';
    RAISE NOTICE 'Wallet Test Data Cleanup Complete!';
    RAISE NOTICE '========================================';
    RAISE NOTICE 'Remaining test data:';
    RAISE NOTICE 'Businesses: %', business_count;
    RAISE NOTICE 'Stored Cards: %', cards_count;
    RAISE NOTICE 'Bank Accounts: %', accounts_count;
    RAISE NOTICE '========================================';

    IF business_count = 0 AND cards_count = 0 AND accounts_count = 0 THEN
        RAISE NOTICE '✅ All test data successfully removed!';
    ELSE
        RAISE WARNING '⚠️ Some test data still remains. Check foreign key constraints.';
    END IF;

    RAISE NOTICE '========================================';
END $$;
