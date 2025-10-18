-- #############################################################################
-- # Rollback File 002: Vendor Payees Test Data
-- # Business ID: 0199f7c6-39ab-77a1-b611-385a4cb8f24b
-- #
-- # PURPOSE: Removes all vendor payee test data created by 002_seed_vendor_payees.sql
-- # USAGE: ./apply-seeds.sh rollback 002
-- #############################################################################

\echo '=== Rolling back Vendor Payees Seed ==='

-- Delete in reverse order of foreign key dependencies

-- Step 1: Delete business-payee relations
\echo 'Deleting business-payee relations...'
DELETE FROM xcentro.business_payee_relation
WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID
AND payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Step 2: Delete bank accounts
\echo 'Deleting payee bank accounts...'
DELETE FROM xcentro.payee_bank_accounts
WHERE payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Step 3: Delete vendor details
\echo 'Deleting vendor details...'
DELETE FROM xcentro.payee_vendor
WHERE payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Step 4: Delete from payee_master (CASCADE will handle child records if any remain)
\echo 'Deleting from payee_master...'
DELETE FROM xcentro.payee_master
WHERE payee_id IN (
    '0199f7c7-1234-7001-a001-111111111111'::UUID,
    '0199f7c7-1234-7002-a001-222222222222'::UUID,
    '0199f7c7-1234-7003-a001-333333333333'::UUID
);

-- Verification
DO $$
DECLARE
    remaining_vendors INTEGER;
    remaining_relations INTEGER;
BEGIN
    -- Count any remaining test vendors
    SELECT COUNT(*) INTO remaining_vendors
    FROM xcentro.payee_master
    WHERE payee_id IN (
        '0199f7c7-1234-7001-a001-111111111111'::UUID,
        '0199f7c7-1234-7002-a001-222222222222'::UUID,
        '0199f7c7-1234-7003-a001-333333333333'::UUID
    );

    -- Count any remaining test relations
    SELECT COUNT(*) INTO remaining_relations
    FROM xcentro.business_payee_relation
    WHERE business_id = '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID
    AND payee_id IN (
        '0199f7c7-1234-7001-a001-111111111111'::UUID,
        '0199f7c7-1234-7002-a001-222222222222'::UUID,
        '0199f7c7-1234-7003-a001-333333333333'::UUID
    );

    RAISE NOTICE '=== Vendor Payees Rollback Complete ===';
    RAISE NOTICE 'Remaining test vendors: %', remaining_vendors;
    RAISE NOTICE 'Remaining test relations: %', remaining_relations;

    IF remaining_vendors > 0 OR remaining_relations > 0 THEN
        RAISE WARNING 'Some test data may still exist. Check manually.';
    ELSE
        RAISE NOTICE 'All vendor payee test data successfully removed.';
    END IF;
END $$;

\echo '=== Rollback complete! ==='
