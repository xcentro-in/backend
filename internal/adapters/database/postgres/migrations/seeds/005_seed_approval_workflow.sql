-- #############################################################################
-- Test Seed Data for Approval Workflow Testing
-- #############################################################################
-- This script creates vouchers and approval records for testing the approval inbox
--
-- Test Scenarios:
-- 1. Vouchers with NO approval entries
-- 2. Vouchers with PENDING approval
-- 3. Vouchers with REJECTED approval
-- 4. Vouchers with REQUEST_CHANGES approval
-- 5. Vouchers with APPROVED approval (within 7 days)
-- 6. Vouchers with APPROVED approval (exactly 7 days ago - edge case)
-- 7. Vouchers with APPROVED approval (8+ days ago - should not appear)
-- 8. Vouchers with APPROVAL HISTORY (multiple state changes)
-- 9. Some vouchers with payments (using stored cards)
--
-- IDs used:
-- Business: 0199f7c6-39ab-77a1-b611-385a4cb8f24b (IIT Tirupati)
-- Users:
--   - Requester: 0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6 (Shubhankar Bhadra)
--   - Approver: 0199f7d1-7514-7dc7-bdfa-075975b8ac2a (Darahas Koppurappu)
-- Payees (3 different vendors)
-- Stored Cards: 2 cards available
-- #############################################################################

BEGIN;

-- =============================================================================
-- SCENARIO 1: Vouchers with NO approval entries (2 vouchers)
-- =============================================================================

-- Voucher 1: No approval, unpaid, overdue
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01aaaaaa-0001-7000-0000-000000000001',
    'PV-2024-0001',
    CURRENT_DATE - INTERVAL '15 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7001-a001-111111111111',
    '0199f7c7-1234-7001-a002-111111111111',
    'INV-TECH-001',
    CURRENT_DATE - INTERVAL '15 days',
    CURRENT_DATE - INTERVAL '5 days', -- Overdue
    5000000, -- ₹50,000
    5000000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '15 days'
);

-- Voucher 2: No approval, unpaid, future due date
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01aaaaaa-0002-7000-0000-000000000002',
    'PV-2024-0002',
    CURRENT_DATE - INTERVAL '5 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7002-a001-222222222222',
    '0199f7c7-1234-7002-a002-222222222222',
    'INV-OFF-002',
    CURRENT_DATE - INTERVAL '5 days',
    CURRENT_DATE + INTERVAL '10 days', -- Future due date
    2500000, -- ₹25,000
    2500000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '5 days'
);

-- =============================================================================
-- SCENARIO 2: Vouchers with PENDING approval (3 vouchers)
-- =============================================================================

-- Voucher 3: Pending approval
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01bbbbbb-0003-7000-0000-000000000003',
    'PV-2024-0003',
    CURRENT_DATE - INTERVAL '3 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7003-a001-333333333333',
    '0199f7c7-1234-7003-a002-333333333333',
    'INV-BUILD-003',
    CURRENT_DATE - INTERVAL '3 days',
    CURRENT_DATE + INTERVAL '15 days',
    10000000, -- ₹1,00,000
    10000000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '3 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01bbbbbb-a003-7000-0000-000000000003',
    '01bbbbbb-0003-7000-0000-000000000003',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'Please review and approve this construction payment',
    CURRENT_DATE - INTERVAL '2 days'
);

-- Voucher 4: Pending approval (recent)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01bbbbbb-0004-7000-0000-000000000004',
    'PV-2024-0004',
    CURRENT_DATE,
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7001-a001-111111111111',
    '0199f7c7-1234-7001-a002-111111111111',
    'INV-TECH-004',
    CURRENT_DATE,
    CURRENT_DATE + INTERVAL '30 days',
    7500000, -- ₹75,000
    7500000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01bbbbbb-a004-7000-0000-000000000004',
    '01bbbbbb-0004-7000-0000-000000000004',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'Urgent - please approve today',
    CURRENT_DATE - INTERVAL '2 hours'
);

-- Voucher 5: Pending (high amount)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01bbbbbb-0005-7000-0000-000000000005',
    'PV-2024-0005',
    CURRENT_DATE - INTERVAL '1 day',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7002-a001-222222222222',
    '0199f7c7-1234-7002-a002-222222222222',
    'INV-OFF-005',
    CURRENT_DATE - INTERVAL '1 day',
    CURRENT_DATE + INTERVAL '7 days',
    15000000, -- ₹1,50,000
    15000000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '1 day'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01bbbbbb-a005-7000-0000-000000000005',
    '01bbbbbb-0005-7000-0000-000000000005',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'High value payment - needs approval',
    CURRENT_DATE - INTERVAL '1 day'
);

-- =============================================================================
-- SCENARIO 3: Vouchers with REJECTED approval (1 voucher)
-- =============================================================================

-- Voucher 6: Rejected
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01cccccc-0006-7000-0000-000000000006',
    'PV-2024-0006',
    CURRENT_DATE - INTERVAL '4 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7003-a001-333333333333',
    '0199f7c7-1234-7003-a002-333333333333',
    'INV-BUILD-006',
    CURRENT_DATE - INTERVAL '4 days',
    CURRENT_DATE + INTERVAL '20 days',
    3000000, -- ₹30,000
    3000000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '4 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01cccccc-a061-7000-0000-000000000006',
    '01cccccc-0006-7000-0000-000000000006',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'Please approve',
    CURRENT_DATE - INTERVAL '4 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01cccccc-a062-7000-0000-000000000006',
    '01cccccc-0006-7000-0000-000000000006',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'rejected',
    'Invoice amount does not match PO. Please verify and resubmit.',
    CURRENT_DATE - INTERVAL '3 days'
);

-- =============================================================================
-- SCENARIO 4: Vouchers with REQUEST_CHANGES approval (2 vouchers)
-- =============================================================================

-- Voucher 7: Request changes
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01dddddd-0007-7000-0000-000000000007',
    'PV-2024-0007',
    CURRENT_DATE - INTERVAL '6 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7001-a001-111111111111',
    '0199f7c7-1234-7001-a002-111111111111',
    'INV-TECH-007',
    CURRENT_DATE - INTERVAL '6 days',
    CURRENT_DATE + INTERVAL '10 days',
    4500000, -- ₹45,000
    4500000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '6 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01dddddd-a007-7000-0000-000000000007',
    '01dddddd-0007-7000-0000-000000000007',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'request_changes',
    'Please attach the delivery challan and update GST details',
    CURRENT_DATE - INTERVAL '5 days'
);

-- Voucher 8: Request changes (recent)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01dddddd-0008-7000-0000-000000000008',
    'PV-2024-0008',
    CURRENT_DATE - INTERVAL '1 day',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7002-a001-222222222222',
    '0199f7c7-1234-7002-a002-222222222222',
    'INV-OFF-008',
    CURRENT_DATE - INTERVAL '1 day',
    CURRENT_DATE + INTERVAL '14 days',
    6000000, -- ₹60,000
    6000000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '1 day'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01dddddd-a008-7000-0000-000000000008',
    '01dddddd-0008-7000-0000-000000000008',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'request_changes',
    'Expense category is missing. Please categorize before approval.',
    CURRENT_DATE - INTERVAL '5 hours'
);

-- =============================================================================
-- SCENARIO 5: Vouchers with APPROVED (within 7 days) - SHOULD APPEAR (3 vouchers)
-- =============================================================================

-- Voucher 9: Approved today (with payment)
INSERT INTO xcentro.payments (
    payment_id, business_id, payment_method,
    gateway_used, amount, total_amount, currency, status, created_at
) VALUES (
    '01eeeeee-0909-7000-0000-000000000009',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    'card',
    'jiopay',
    5500000, -- ₹55,000
    5500000,
    'INR',
    'success',
    CURRENT_DATE - INTERVAL '1 hour'
);

INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, payment_id, created_by_user_id, created_at
) VALUES (
    '01eeeeee-0009-7000-0000-000000000009',
    'PV-2024-0009',
    CURRENT_DATE,
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7003-a001-333333333333',
    '0199f7c7-1234-7003-a002-333333333333',
    'INV-BUILD-009',
    CURRENT_DATE,
    CURRENT_DATE + INTERVAL '30 days',
    5500000, -- ₹55,000
    5500000,
    'INR',
    'finalized',
    '01eeeeee-0909-7000-0000-000000000009',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a009-7000-0000-000000000009',
    '01eeeeee-0009-7000-0000-000000000009',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'approved',
    'Approved - payment processed',
    CURRENT_DATE - INTERVAL '30 minutes'
);

-- Voucher 10: Approved 3 days ago
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01eeeeee-0010-7000-0000-000000000010',
    'PV-2024-0010',
    CURRENT_DATE - INTERVAL '5 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7001-a001-111111111111',
    '0199f7c7-1234-7001-a002-111111111111',
    'INV-TECH-010',
    CURRENT_DATE - INTERVAL '5 days',
    CURRENT_DATE + INTERVAL '25 days',
    3500000, -- ₹35,000
    3500000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '5 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a010-7000-0000-000000000010',
    '01eeeeee-0010-7000-0000-000000000010',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'approved',
    'Approved for payment',
    CURRENT_DATE - INTERVAL '3 days'
);

-- Voucher 11: Approved 6 days ago (EDGE CASE - within 7 days)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01eeeeee-0011-7000-0000-000000000011',
    'PV-2024-0011',
    CURRENT_DATE - INTERVAL '8 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7002-a001-222222222222',
    '0199f7c7-1234-7002-a002-222222222222',
    'INV-OFF-011',
    CURRENT_DATE - INTERVAL '8 days',
    CURRENT_DATE + INTERVAL '22 days',
    8000000, -- ₹80,000
    8000000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '8 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a011-7000-0000-000000000011',
    '01eeeeee-0011-7000-0000-000000000011',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'approved',
    'Approved',
    CURRENT_DATE - INTERVAL '6 days'
);

-- =============================================================================
-- SCENARIO 6: Vouchers with APPROVED (8+ days ago) - SHOULD NOT APPEAR (2 vouchers)
-- =============================================================================

-- Voucher 12: Approved 8 days ago (EDGE CASE - should NOT appear)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01ffffff-0012-7000-0000-000000000012',
    'PV-2024-0012',
    CURRENT_DATE - INTERVAL '10 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7003-a001-333333333333',
    '0199f7c7-1234-7003-a002-333333333333',
    'INV-BUILD-012',
    CURRENT_DATE - INTERVAL '10 days',
    CURRENT_DATE + INTERVAL '20 days',
    9000000, -- ₹90,000
    9000000,
    'INR',
    'finalized',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '10 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01ffffff-a012-7000-0000-000000000012',
    '01ffffff-0012-7000-0000-000000000012',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'approved',
    'Approved - old payment',
    CURRENT_DATE - INTERVAL '8 days'
);

-- Voucher 13: Approved 15 days ago (definitely should NOT appear)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01ffffff-0013-7000-0000-000000000013',
    'PV-2024-0013',
    CURRENT_DATE - INTERVAL '20 days',
    '2024-25',
    '2024-10',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7001-a001-111111111111',
    '0199f7c7-1234-7001-a002-111111111111',
    'INV-TECH-013',
    CURRENT_DATE - INTERVAL '20 days',
    CURRENT_DATE - INTERVAL '5 days',
    12000000, -- ₹1,20,000
    12000000,
    'INR',
    'posted',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '20 days'
);

INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01ffffff-a013-7000-0000-000000000013',
    '01ffffff-0013-7000-0000-000000000013',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'approved',
    'Approved - archived',
    CURRENT_DATE - INTERVAL '15 days'
);

-- =============================================================================
-- SCENARIO 7: Voucher with APPROVAL HISTORY (multiple state changes)
-- =============================================================================

-- Voucher 14: Complex history (pending -> request_changes -> pending)
INSERT INTO xcentro.payment_vouchers (
    voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period,
    business_id, payee_id, account_id,
    invoice_number, invoice_date, invoice_due_date,
    gross_amount, net_amount, currency,
    status, created_by_user_id, created_at
) VALUES (
    '01eeeeee-0014-7000-0000-000000000014',
    'PV-2024-0014',
    CURRENT_DATE - INTERVAL '7 days',
    '2024-25',
    '2024-11',
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b',
    '0199f7c7-1234-7002-a001-222222222222',
    '0199f7c7-1234-7002-a002-222222222222',
    'INV-OFF-014',
    CURRENT_DATE - INTERVAL '7 days',
    CURRENT_DATE + INTERVAL '18 days',
    9500000, -- ₹95,000
    9500000,
    'INR',
    'draft',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    CURRENT_DATE - INTERVAL '7 days'
);

-- History entry 1: Initial pending
INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a141-7000-0000-000000000014',
    '01eeeeee-0014-7000-0000-000000000014',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'Initial approval request',
    CURRENT_DATE - INTERVAL '7 days'
);

-- History entry 2: Request changes
INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a142-7000-0000-000000000014',
    '01eeeeee-0014-7000-0000-000000000014',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'request_changes',
    'Need updated invoice with correct GST breakdown',
    CURRENT_DATE - INTERVAL '5 days'
);

-- History entry 3: Resubmitted as pending
INSERT INTO xcentro.voucher_approvals (
    approval_id, voucher_id, requester_user_id, approver_user_id,
    action, remarks, created_at
) VALUES (
    '01eeeeee-a143-7000-0000-000000000014',
    '01eeeeee-0014-7000-0000-000000000014',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    '0199f7d1-7514-7dc7-bdfa-075975b8ac2a',
    'pending',
    'Updated invoice attached. Please review again.',
    CURRENT_DATE - INTERVAL '2 days'
);

COMMIT;

-- =============================================================================
-- SUMMARY OF SEED DATA
-- =============================================================================
-- Total vouchers created: 14
--
-- Expected results for GET /vouchers/approval-inbox?status=:
--
-- status=pending -> Should return 4 vouchers (3, 4, 5, 14)
-- status=rejected -> Should return 1 voucher (6)
-- status=request_changes -> Should return 2 vouchers (7, 8)
-- status=approved -> Should return 3 vouchers (9, 10, 11)
--                    [12 & 13 should NOT appear - older than 7 days]
--
-- Vouchers with NO approval entries: 2 (1, 2) - should not appear in any inbox
-- =============================================================================
