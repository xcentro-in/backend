-- Restore payout_configuration table

CREATE TABLE xcentro.payout_configuration (
    config_id UUID PRIMARY KEY DEFAULT uuidv7(),
    business_id UUID REFERENCES xcentro.business(bid) ON DELETE CASCADE,

    -- Auto-approval settings
    auto_approval_enabled BOOLEAN NOT NULL DEFAULT true,
    auto_approval_limit BIGINT NOT NULL DEFAULT 20000000,

    -- Transfer preferences
    preferred_transfer_mode VARCHAR(50) NOT NULL DEFAULT 'NEFT',

    -- Retry settings
    max_retry_attempts INT NOT NULL DEFAULT 3,
    retry_delay_minutes INT NOT NULL DEFAULT 15,
    retry_backoff_multiplier DECIMAL(3,1) NOT NULL DEFAULT 2.0,

    -- Webhook timeout
    webhook_timeout_minutes INT NOT NULL DEFAULT 60,

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

-- Insert global default
INSERT INTO xcentro.payout_configuration (
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
    NULL,
    true,
    20000000,
    'NEFT',
    3,
    15,
    2.0,
    60,
    100,
    true
);
