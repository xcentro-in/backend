-- ############################################################################
-- # Seed Script: Update Draft Voucher with Successful Card Payment
-- # Purpose: Create payment and update existing voucher to finalized status
-- ############################################################################

-- Using existing entities:
-- Voucher ID:   c80a0b34-47e9-4d70-a7d7-a3cb301e84e3 (existing draft)
-- Business ID:  0199f7c6-39ab-77a1-b611-385a4cb8f24b
-- User ID:      0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6
-- Card ID:      01936d7a-8000-7890-c004-444444444444

BEGIN;

-- ============================================================================
-- 1. CREATE SUCCESSFUL PAYMENT TRANSACTION (Card Payment)
-- ============================================================================
INSERT INTO xcentro.payments (
    payment_id,
    user_id,
    business_id,
    amount,
    currency,
    payment_method,
    status,
    description,
    gateway_used,
    gateway_payment_id,
    callback_url,
    redirect_url,
    payment_source_id,
    payment_source_type,
    fee_amount,
    total_amount,
    metadata,
    created_at,
    updated_at
)
SELECT
    uuidv7(), -- Auto-generate payment ID
    '0199f7c4-e8cb-73ad-8d66-3eaacfec9bb6'::UUID,
    '0199f7c6-39ab-77a1-b611-385a4cb8f24b'::UUID,
    net_amount, -- Use the net_amount from existing voucher
    'INR',
    'token_card', -- Card payment using stored card
    'success', -- ✅ SUCCESSFUL PAYMENT
    'Payment for voucher ' || voucher_number,
    'jiopay', -- Gateway used
    'jiopay_' || substring(gen_random_uuid()::text from 1 for 16), -- Unique gateway transaction ID
    'https://xcentro.in/payments/callback',
    'https://xcentro.in/payments/success',
    '01936d7a-8000-7890-c004-444444444444'::UUID, -- Card ID from stored_cards
    'card',
    GREATEST(ROUND(net_amount * 0.02), 20000), -- Gateway fee (2% with minimum ₹200)
    net_amount + GREATEST(ROUND(net_amount * 0.02), 20000), -- Total = amount + fee
    jsonb_build_object(
        'gateway_response', jsonb_build_object(
            'status', 'success',
            'transaction_id', 'jiopay_' || substring(gen_random_uuid()::text from 1 for 16),
            'payment_mode', 'CARD',
            'card_brand', 'VISA',
            'bank_ref_no', 'REF' || floor(random() * 1000000000)::text,
            'card_type', 'CREDIT',
            'response_code', '00',
            'response_message', 'Transaction Successful'
        ),
        'fee_breakdown', jsonb_build_object(
            'base_fee', ROUND(GREATEST(ROUND(net_amount * 0.02), 20000) / 1.18),
            'gst', ROUND(GREATEST(ROUND(net_amount * 0.02), 20000) * 0.18 / 1.18),
            'total', GREATEST(ROUND(net_amount * 0.02), 20000)
        ),
        'payment_timestamp', NOW(),
        'card_details', jsonb_build_object(
            'masked_card', '****-****-****-4242',
            'card_brand', 'Visa'
        )
    ),
    NOW() - INTERVAL '5 minutes', -- Payment created 5 minutes ago
    NOW() - INTERVAL '3 minutes'  -- Payment completed 2 minutes after creation
FROM xcentro.payment_vouchers
WHERE voucher_id = 'c80a0b34-47e9-4d70-a7d7-a3cb301e84e3'::UUID
RETURNING payment_id, gateway_payment_id, amount, fee_amount, total_amount
\gset

-- Display the created payment details
\echo ''
\echo '✅ Payment Created:'
\echo '   Payment ID: ' :payment_id
\echo '   Gateway Payment ID: ' :gateway_payment_id
\echo '   Amount: ₹' :amount ' paise'
\echo '   Fee: ₹' :fee_amount ' paise'
\echo '   Total: ₹' :total_amount ' paise'
\echo ''

-- ============================================================================
-- 2. UPDATE VOUCHER TO FINALIZED STATUS
-- ============================================================================
UPDATE xcentro.payment_vouchers
SET
    payment_id = :'payment_id'::UUID,
    status = 'finalized',
    updated_at = NOW()
WHERE voucher_id = 'c80a0b34-47e9-4d70-a7d7-a3cb301e84e3'::UUID
RETURNING
    voucher_id,
    voucher_number,
    status,
    net_amount,
    payment_id;

\echo ''
\echo '✅ Voucher Updated to Finalized Status'
\echo ''

COMMIT;

-- ============================================================================
-- VERIFICATION QUERY
-- ============================================================================
\echo '=== Final Verification ==='
\echo ''

SELECT
    v.voucher_id,
    v.voucher_number,
    v.status as voucher_status,
    v.net_amount / 100.0 as voucher_amount_inr,
    v.invoice_number,
    v.expense_category,
    p.payment_id,
    p.status as payment_status,
    p.payment_method,
    p.gateway_used,
    p.gateway_payment_id,
    p.amount / 100.0 as payment_amount_inr,
    p.fee_amount / 100.0 as gateway_fee_inr,
    p.total_amount / 100.0 as total_paid_inr,
    p.created_at::text as payment_created_at,
    v.updated_at::text as voucher_finalized_at
FROM xcentro.payment_vouchers v
JOIN xcentro.payments p ON v.payment_id = p.payment_id
WHERE v.voucher_id = 'c80a0b34-47e9-4d70-a7d7-a3cb301e84e3'::UUID;
