-- Create enum for payment action types
CREATE TYPE xcentro.payment_action_enum AS ENUM (
    -- Payment lifecycle
    'payment_initiated',      -- User clicked pay, payment request created
    'payment_processing',     -- Gateway processing payment
    'payment_success',        -- Payment succeeded (gateway webhook)
    'payment_failed',         -- Payment failed (gateway webhook)
    'payment_refunded',       -- Payment refunded (admin action or gateway)

    -- Settlement lifecycle (funds to escrow)
    'settlement_pending',     -- Awaiting settlement from gateway
    'settlement_received',    -- Funds settled to escrow account (T+2)
    'settlement_failed',      -- Settlement failed

    -- Payout lifecycle (funds from escrow to payee)
    'payout_initiated',       -- Payout transfer initiated (manual or auto)
    'payout_processing',      -- Payout provider processing transfer
    'payout_completed',       -- Funds settled to payee account (final state)
    'payout_failed',          -- Payout transfer failed
    'payout_timeout',         -- No webhook received (investigation needed)
    'payout_cancelled'        -- Payout cancelled before initiation
);

COMMENT ON TYPE xcentro.payment_action_enum IS 'Payment and payout lifecycle actions for audit trail';

-- Create enum for actor types
CREATE TYPE xcentro.payment_actor_enum AS ENUM (
    'user',                   -- Human user (customer, admin, ops)
    'system',                 -- System/worker (payout_worker, settlement_worker)
    'webhook',                -- External webhook (gateway, payout provider)
    'api'                     -- External API call (manual trigger)
);

COMMENT ON TYPE xcentro.payment_actor_enum IS 'Actor type that triggered the payment state change';

-- Create payment state changes table
CREATE TABLE xcentro.payment_state_changes (
    state_change_id      UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Related entities (payment is primary relationship, no voucher_id to avoid duplicates in multi-bill payments)
    payment_id           UUID NOT NULL REFERENCES xcentro.payments(payment_id) ON DELETE CASCADE,
    transfer_request_id  UUID REFERENCES xcentro.payout_transfer_requests(transfer_request_id) ON DELETE SET NULL,

    -- Action details
    action               xcentro.payment_action_enum NOT NULL,
    action_timestamp     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Actor details
    actor_type           xcentro.payment_actor_enum NOT NULL DEFAULT 'system',
    actor_user_id        UUID REFERENCES xcentro.user_table(id) ON DELETE SET NULL,
    actor_name           VARCHAR(100),  -- Denormalized: 'payout_worker', 'razorpay_webhook', etc.

    -- State context (optional)
    previous_state       VARCHAR(50),   -- Previous status value (if applicable)
    new_state            VARCHAR(50),   -- New status value (if applicable)

    -- Additional information
    amount               BIGINT,        -- Amount involved in this action (in paise)
    currency             VARCHAR(3) DEFAULT 'INR',
    utr                  VARCHAR(100),  -- Bank UTR for completed payouts
    provider             VARCHAR(50),   -- Gateway/provider name (jiopay, easebuzz, castler)

    -- Metadata and remarks
    metadata             JSONB,         -- Additional context (gateway response, error codes, etc.)
    remarks              TEXT,          -- Human-readable description

    -- Audit timestamps
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_payment_state_payment ON xcentro.payment_state_changes(payment_id, action_timestamp DESC);
CREATE INDEX idx_payment_state_transfer ON xcentro.payment_state_changes(transfer_request_id, action_timestamp DESC);
CREATE INDEX idx_payment_state_action ON xcentro.payment_state_changes(action, action_timestamp DESC);
CREATE INDEX idx_payment_state_actor ON xcentro.payment_state_changes(actor_user_id) WHERE actor_user_id IS NOT NULL;
CREATE INDEX idx_payment_state_timestamp ON xcentro.payment_state_changes(action_timestamp DESC);

-- Comments
COMMENT ON TABLE xcentro.payment_state_changes IS 'Audit trail for payment and payout state transitions. Tracks who initiated payments and when funds settled to payee accounts. Uses payment_id as primary relationship to avoid duplicates in multi-bill payments.';
COMMENT ON COLUMN xcentro.payment_state_changes.payment_id IS 'Payment record (multiple vouchers can share same payment in multi-bill scenarios)';
COMMENT ON COLUMN xcentro.payment_state_changes.transfer_request_id IS 'Payout transfer request (nullable for payment-only events)';
COMMENT ON COLUMN xcentro.payment_state_changes.action IS 'Type of state change event';
COMMENT ON COLUMN xcentro.payment_state_changes.actor_type IS 'Who/what triggered this change (user, system, webhook, api)';
COMMENT ON COLUMN xcentro.payment_state_changes.actor_user_id IS 'User who initiated action (if actor_type = user)';
COMMENT ON COLUMN xcentro.payment_state_changes.actor_name IS 'Denormalized actor name for system/webhook actors';
COMMENT ON COLUMN xcentro.payment_state_changes.utr IS 'Bank UTR from payout completion webhook';
COMMENT ON COLUMN xcentro.payment_state_changes.metadata IS 'Additional context: gateway responses, error codes, webhook payloads';
