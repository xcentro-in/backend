-- Migration 000009: Add payout transfer requests and configuration tables
-- Purpose: Track payout transfers with webhook-based status updates and approval workflow

-- ============================================================================
-- ENUMS
-- ============================================================================

-- Payout transfer status (webhook-driven)
CREATE TYPE xcentro.payout_transfer_status_enum AS ENUM (
    'pending_approval',    -- Awaiting manual approval (amount > ₹2L)
    'approved',            -- Approved, ready to initiate
    'initiated',           -- Sent to Castler, awaiting webhook
    'processing',          -- Castler confirmed processing (from webhook)
    'completed',           -- Successfully completed (from webhook)
    'failed',              -- Failed (from webhook) - needs manual retry (MVP)
    'cancelled',           -- Cancelled by ops before initiation
    'timeout'              -- No webhook received after timeout period (needs investigation)
);

-- ============================================================================
-- TABLES
-- ============================================================================

-- Payout transfer requests (audit trail for all payout attempts)
CREATE TABLE xcentro.payout_transfer_requests (
    transfer_request_id UUID PRIMARY KEY DEFAULT uuidv7(),
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE RESTRICT,

    -- Provider details
    provider VARCHAR(50) NOT NULL,
    provider_beneficiary_id VARCHAR(255) NOT NULL,
    provider_transfer_id VARCHAR(255),  -- Castler's transfer ID (set after initiation)

    -- Transfer details
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',
    purpose VARCHAR(255),
    reference VARCHAR(255),          -- voucher_number
    narration TEXT,
    transfer_mode VARCHAR(50),       -- 'NEFT', 'RTGS', 'IMPS'

    -- Approval workflow
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    approval_mode VARCHAR(20),       -- 'auto', 'manual'
    approved_by UUID REFERENCES xcentro.user_table(id),
    approved_at TIMESTAMPTZ,

    -- Status tracking (webhook-driven)
    status xcentro.payout_transfer_status_enum NOT NULL DEFAULT 'approved',
    initiated_at TIMESTAMPTZ,        -- When sent to Castler
    processing_at TIMESTAMPTZ,       -- When Castler started processing (from webhook)
    completed_at TIMESTAMPTZ,        -- When completed (from webhook)
    failed_at TIMESTAMPTZ,           -- When failed (from webhook)
    failure_reason TEXT,             -- From webhook
    utr VARCHAR(100),                -- Bank UTR from webhook

    -- Retry tracking (placeholder for future)
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,
    last_retry_at TIMESTAMPTZ,

    -- Webhook tracking
    last_webhook_at TIMESTAMPTZ,
    webhook_timeout_at TIMESTAMPTZ,  -- initiated_at + timeout_minutes (flag if no webhook)
    webhook_history JSONB[] DEFAULT ARRAY[]::JSONB[],  -- Array of all webhook payloads

    -- Metadata
    provider_metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    CONSTRAINT unique_voucher_transfer UNIQUE(voucher_id),  -- One transfer per voucher
    CONSTRAINT check_amount_positive CHECK (amount > 0)
);

-- Payout configuration (auto-approval limits, retry settings, etc.)
CREATE TABLE xcentro.payout_configuration (
    config_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID REFERENCES xcentro.business(bid) ON DELETE CASCADE,  -- NULL = global, specific UUID = per-business

    -- Auto-approval settings
    auto_approval_enabled BOOLEAN NOT NULL DEFAULT true,
    auto_approval_limit BIGINT NOT NULL DEFAULT 20000000,  -- ₹2,00,000 in paise

    -- Transfer preferences
    preferred_transfer_mode VARCHAR(50) NOT NULL DEFAULT 'NEFT',  -- 'NEFT', 'RTGS', 'IMPS'

    -- Retry settings (placeholder for future)
    max_retry_attempts INT NOT NULL DEFAULT 3,
    retry_delay_minutes INT NOT NULL DEFAULT 15,  -- First retry after 15min
    retry_backoff_multiplier DECIMAL(3,1) NOT NULL DEFAULT 2.0,  -- Exponential: 15, 30, 60 min

    -- Webhook timeout
    webhook_timeout_minutes INT NOT NULL DEFAULT 60,  -- Flag if no webhook after 1 hour

    -- Worker settings
    batch_size INT NOT NULL DEFAULT 100,
    worker_enabled BOOLEAN NOT NULL DEFAULT true,

    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    CONSTRAINT unique_business_config UNIQUE(business_id),
    CONSTRAINT check_auto_approval_limit_positive CHECK (auto_approval_limit >= 0),
    CONSTRAINT check_batch_size_positive CHECK (batch_size > 0),
    CONSTRAINT check_timeout_positive CHECK (webhook_timeout_minutes > 0)
);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Transfer request indexes for worker queries
CREATE INDEX idx_payout_transfers_voucher ON xcentro.payout_transfer_requests(voucher_id);
CREATE INDEX idx_payout_transfers_status ON xcentro.payout_transfer_requests(status);
CREATE INDEX idx_payout_transfers_provider_transfer_id ON xcentro.payout_transfer_requests(provider_transfer_id);

-- Worker-specific indexes
CREATE INDEX idx_payout_pending_approval ON xcentro.payout_transfer_requests(status, amount DESC)
    WHERE status = 'pending_approval';

CREATE INDEX idx_payout_approved ON xcentro.payout_transfer_requests(status, created_at)
    WHERE status = 'approved';

CREATE INDEX idx_payout_initiated ON xcentro.payout_transfer_requests(status, initiated_at)
    WHERE status = 'initiated';

CREATE INDEX idx_payout_timeout ON xcentro.payout_transfer_requests(webhook_timeout_at)
    WHERE status = 'initiated';

CREATE INDEX idx_payout_retry ON xcentro.payout_transfer_requests(next_retry_at, retry_count)
    WHERE status = 'failed' AND retry_count < max_retries;

-- Configuration index
CREATE INDEX idx_payout_config_business ON xcentro.payout_configuration(business_id);

-- ============================================================================
-- SEED DATA
-- ============================================================================

-- Insert global default configuration
INSERT INTO xcentro.payout_configuration (
    config_id,
    business_id,
    auto_approval_enabled,
    auto_approval_limit,
    preferred_transfer_mode,
    max_retry_attempts,
    retry_delay_minutes,
    retry_backoff_multiplier,
    webhook_timeout_minutes,
    batch_size,
    worker_enabled
) VALUES (
    uuidv7(),
    NULL,  -- Global default
    true,
    20000000,  -- ₹2,00,000
    'NEFT',
    3,
    15,
    2.0,
    60,
    100,
    true
);

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE xcentro.payout_transfer_requests IS 'Audit trail for all payout transfer attempts with webhook-driven status updates';
COMMENT ON COLUMN xcentro.payout_transfer_requests.webhook_history IS 'Array of all webhook payloads received from provider';
COMMENT ON COLUMN xcentro.payout_transfer_requests.webhook_timeout_at IS 'Timestamp when transfer will be flagged as timeout if no webhook received';
COMMENT ON COLUMN xcentro.payout_transfer_requests.retry_count IS 'Number of retry attempts (placeholder for future retry strategy)';

COMMENT ON TABLE xcentro.payout_configuration IS 'Payout system configuration including auto-approval limits and retry settings';
COMMENT ON COLUMN xcentro.payout_configuration.business_id IS 'NULL indicates global default configuration';
COMMENT ON COLUMN xcentro.payout_configuration.auto_approval_limit IS 'Amounts <= this value are auto-approved (in paise)';
