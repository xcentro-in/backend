-- #############################################################################
-- Seed Data for Payment State Changes (Audit Trail)
-- #############################################################################
-- This script creates a complete payment lifecycle for testing the
-- payment state changes audit trail and voucher activity feed.
--
-- Payment ID: 01eeeeee-0909-7000-0000-000000000009
-- Voucher ID: 01eeeeee-0009-7000-0000-000000000009
-- Business: 0199f7c6-39ab-77a1-b611-385a4cb8f24b (IIT Tirupati)
-- User: 0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6 (Shubhankar Bhadra)
--
-- Timeline:
-- 1. User initiates payment (card)
-- 2. Payment gateway processes and succeeds
-- 3. Settlement received in escrow (T+2) - HIDDEN from user activity
-- 4. Payout automatically initiated by system
-- 5. Payout completed - funds settled to payee account
-- #############################################################################

BEGIN;

-- =============================================================================
-- Payment Lifecycle: Complete flow from initiation to payout completion
-- =============================================================================

-- Step 1: Payment Initiated (User clicks pay)
-- Time: Current date - 2 days, 10:00 AM
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0001-7000-0000-000000000001',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL, -- No transfer yet
    'payment_initiated',
    CURRENT_DATE - INTERVAL '2 days' + INTERVAL '10 hours',
    'user',
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6',
    NULL, -- Not needed for user type
    NULL,
    'initiated',
    5500000, -- ₹55,000 in paise
    'INR',
    NULL, -- No UTR yet
    'jiopay',
    '{"card_last_four": "4242", "card_type": "visa", "card_category": "credit"}'::jsonb,
    'Payment initiated via jiopay gateway',
    CURRENT_DATE - INTERVAL '2 days' + INTERVAL '10 hours'
);

-- Step 2: Payment Success (Gateway webhook)
-- Time: Current date - 2 days, 10:02 AM
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0002-7000-0000-000000000002',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL,
    'payment_success',
    CURRENT_DATE - INTERVAL '2 days' + INTERVAL '10 hours 2 minutes',
    'webhook',
    NULL,
    'jiopay_webhook',
    'initiated',
    'success',
    5500000,
    'INR',
    NULL, -- Payment UTR (not settlement UTR)
    'jiopay',
    '{"gateway_payment_id": "pay_JioPay123456", "gateway_order_id": "order_JioPay789", "payment_method": "card", "card_network": "visa"}'::jsonb,
    'Payment successful. Gateway Payment ID: pay_JioPay123456',
    CURRENT_DATE - INTERVAL '2 days' + INTERVAL '10 hours 2 minutes'
);

-- Step 3: Settlement Received (T+2 - funds in escrow)
-- Time: Current date, 12:00 PM
-- NOTE: This will be HIDDEN from user-facing activity feed
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0003-7000-0000-000000000003',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL,
    'settlement_received',
    CURRENT_DATE + INTERVAL '12 hours',
    'webhook',
    NULL,
    'settlement_webhook',
    'success',
    'settled',
    5390000, -- ₹53,900 (after MDR deduction of 2%)
    'INR',
    'JIOPAY240101123456', -- Settlement UTR
    'jiopay',
    '{"settlement_batch_id": "BATCH20241101", "settlement_date": "2024-11-04", "mdr_rate": "2.00", "mdr_amount": 110000, "net_settlement_amount": 5390000}'::jsonb,
    'Settlement received in escrow. UTR: JIOPAY240101123456',
    CURRENT_DATE + INTERVAL '12 hours'
);

-- Step 4: Payout Initiated (Automatic - by payout worker)
-- Time: Current date, 14:00 PM
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0004-7000-0000-000000000004',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL, -- No transfer request record in seed data
    'payout_initiated',
    CURRENT_DATE + INTERVAL '14 hours',
    'system',
    NULL,
    'payout_worker',
    'settled',
    'initiated',
    5500000, -- Full voucher amount to payee
    'INR',
    NULL, -- No UTR yet
    'castler',
    '{"transfer_type": "IMPS", "beneficiary_account": "****7890", "beneficiary_ifsc": "HDFC0001234", "auto_initiated": true}'::jsonb,
    'Payout initiated automatically by payout worker',
    CURRENT_DATE + INTERVAL '14 hours'
);

-- Step 5: Payout Processing (Provider processing)
-- Time: Current date, 14:01 PM
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0005-7000-0000-000000000005',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL, -- No transfer request record in seed data
    'payout_processing',
    CURRENT_DATE + INTERVAL '14 hours 1 minute',
    'webhook',
    NULL,
    'castler_webhook',
    'initiated',
    'processing',
    5500000,
    'INR',
    NULL,
    'castler',
    '{"provider_transfer_id": "CASTLER_TX_123456", "processing_status": "in_progress"}'::jsonb,
    'Payout transfer is being processed by Castler',
    CURRENT_DATE + INTERVAL '14 hours 1 minute'
);

-- Step 6: Payout Completed (Final state - funds in payee account)
-- Time: Current date, 14:30 PM
INSERT INTO xcentro.payment_state_changes (
    state_change_id,
    payment_id,
    transfer_request_id,
    action,
    action_timestamp,
    actor_type,
    actor_user_id,
    actor_name,
    previous_state,
    new_state,
    amount,
    currency,
    utr,
    provider,
    metadata,
    remarks,
    created_at
) VALUES (
    '01ffffff-0006-7000-0000-000000000006',
    '01eeeeee-0909-7000-0000-000000000009',
    NULL, -- No transfer request record in seed data
    'payout_completed',
    CURRENT_DATE + INTERVAL '14 hours 30 minutes',
    'webhook',
    NULL,
    'castler_webhook',
    'processing',
    'completed',
    5500000,
    'INR',
    'HDFC2024110456789', -- Bank UTR for payout
    'castler',
    '{"provider_transfer_id": "CASTLER_TX_123456", "bank_reference": "HDFC2024110456789", "completion_time": "2024-11-04T14:30:00Z", "beneficiary_name": "Tech Solutions Pvt Ltd"}'::jsonb,
    'Payout completed. Funds settled to payee account. UTR: HDFC2024110456789',
    CURRENT_DATE + INTERVAL '14 hours 30 minutes'
);

COMMIT;

-- #############################################################################
-- Summary of State Changes
-- #############################################################################
-- User-Facing Activity (settlement_received is HIDDEN):
-- 1. [2 days ago, 10:00] Payment Initiated (by Shubhankar Bhadra)
-- 2. [Today, 14:00] Payment Processing (by payout_worker)
-- 3. [Today, 14:30] Payment Completed to Payee (by castler_webhook, UTR: HDFC2024110456789)
--
-- Internal Audit Trail (all 6 entries visible for debugging):
-- 1. payment_initiated
-- 2. payment_success
-- 3. settlement_received (hidden from users)
-- 4. payout_initiated
-- 5. payout_processing
-- 6. payout_completed
-- #############################################################################
