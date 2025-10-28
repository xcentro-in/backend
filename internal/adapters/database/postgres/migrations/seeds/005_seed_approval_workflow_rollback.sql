-- #############################################################################
-- Rollback: Approval Workflow Test Seed Data
-- #############################################################################
-- This script removes all test vouchers and approvals created by 005_seed_approval_workflow.sql
-- #############################################################################

BEGIN;

-- Delete all approval entries for test vouchers (cascades will handle this, but explicit for clarity)
DELETE FROM xcentro.voucher_approvals
WHERE voucher_id IN (
    '01aaaaaa-0001-7000-0000-000000000001',
    '01aaaaaa-0002-7000-0000-000000000002',
    '01bbbbbb-0003-7000-0000-000000000003',
    '01bbbbbb-0004-7000-0000-000000000004',
    '01bbbbbb-0005-7000-0000-000000000005',
    '01cccccc-0006-7000-0000-000000000006',
    '01dddddd-0007-7000-0000-000000000007',
    '01dddddd-0008-7000-0000-000000000008',
    '01eeeeee-0009-7000-0000-000000000009',
    '01eeeeee-0010-7000-0000-000000000010',
    '01eeeeee-0011-7000-0000-000000000011',
    '01ffffff-0012-7000-0000-000000000012',
    '01ffffff-0013-7000-0000-000000000013',
    '01eeeeee-0014-7000-0000-000000000014'
);

-- Delete all test vouchers
DELETE FROM xcentro.payment_vouchers
WHERE voucher_id IN (
    '01aaaaaa-0001-7000-0000-000000000001',
    '01aaaaaa-0002-7000-0000-000000000002',
    '01bbbbbb-0003-7000-0000-000000000003',
    '01bbbbbb-0004-7000-0000-000000000004',
    '01bbbbbb-0005-7000-0000-000000000005',
    '01cccccc-0006-7000-0000-000000000006',
    '01dddddd-0007-7000-0000-000000000007',
    '01dddddd-0008-7000-0000-000000000008',
    '01eeeeee-0009-7000-0000-000000000009',
    '01eeeeee-0010-7000-0000-000000000010',
    '01eeeeee-0011-7000-0000-000000000011',
    '01ffffff-0012-7000-0000-000000000012',
    '01ffffff-0013-7000-0000-000000000013',
    '01eeeeee-0014-7000-0000-000000000014'
);

-- Delete test payment (only one payment was created)
DELETE FROM xcentro.payments
WHERE payment_id = '01eeeeee-0909-7000-0000-000000000009';

COMMIT;
