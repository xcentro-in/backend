-- #############################################################################
-- # Rollback Seed File 003: Employee & Biller Payees Test Data
-- # Purpose: Remove employee and biller test data
-- #############################################################################

\echo ''
\echo '========================================================================='
\echo '  Rollback Seed 003: Employee & Biller Payees Test Data'
\echo '========================================================================='
\echo ''

-- Delete in reverse order of creation to respect foreign key constraints

-- Delete business-payee relations
DELETE FROM xcentro.business_payee_relation
WHERE relation_id IN (
    '0199f7c8-1234-8001-e003-111111111111'::UUID,  -- Employee 1 relation
    '0199f7c8-1234-8002-e003-222222222222'::UUID,  -- Employee 2 relation
    '0199f7c8-1234-9001-b003-333333333333'::UUID   -- Biller 1 relation
);

-- Delete biller account
DELETE FROM xcentro.payee_biller_accounts
WHERE biller_account_id = '0199f7c8-1234-9001-b002-333333333333'::UUID;

-- Delete employee bank accounts
DELETE FROM xcentro.payee_bank_accounts
WHERE account_id IN (
    '0199f7c8-1234-8001-e002-111111111111'::UUID,  -- Employee 1 bank
    '0199f7c8-1234-8002-e002-222222222222'::UUID   -- Employee 2 bank
);

-- Delete employee details
DELETE FROM xcentro.payee_employee
WHERE payee_id IN (
    '0199f7c8-1234-8001-e001-111111111111'::UUID,  -- Employee 1
    '0199f7c8-1234-8002-e001-222222222222'::UUID   -- Employee 2
);

-- Delete biller details
DELETE FROM xcentro.payee_biller
WHERE payee_id = '0199f7c8-1234-9001-b001-333333333333'::UUID;

-- Delete payee master entries (CASCADE will clean up any remaining references)
DELETE FROM xcentro.payee_master
WHERE payee_id IN (
    '0199f7c8-1234-8001-e001-111111111111'::UUID,  -- Employee 1
    '0199f7c8-1234-8002-e001-222222222222'::UUID,  -- Employee 2
    '0199f7c8-1234-9001-b001-333333333333'::UUID   -- Biller 1
);


-- #############################################################################
-- # Verification Output
-- #############################################################################

DO $$
DECLARE
    remaining_count INT;
BEGIN
    -- Verify deletion
    SELECT COUNT(*) INTO remaining_count
    FROM xcentro.payee_master
    WHERE payee_id IN (
        '0199f7c8-1234-8001-e001-111111111111'::UUID,
        '0199f7c8-1234-8002-e001-222222222222'::UUID,
        '0199f7c8-1234-9001-b001-333333333333'::UUID
    );

    RAISE NOTICE '';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE ' Seed 003 Rolled Back Successfully';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE ' Remaining test payees: % (should be 0)', remaining_count;
    RAISE NOTICE '';
    RAISE NOTICE ' Deleted:';
    RAISE NOTICE '   - Employee 1: Rajesh Kumar';
    RAISE NOTICE '   - Employee 2: Priya Sharma';
    RAISE NOTICE '   - Biller 1: Airtel Mobile Postpaid';
    RAISE NOTICE '========================================================================';
    RAISE NOTICE '';
END $$;
