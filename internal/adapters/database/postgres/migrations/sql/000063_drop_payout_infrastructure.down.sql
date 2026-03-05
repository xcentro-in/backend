-- Migration 000063 DOWN: Restore payout infrastructure (rollback split settlement migration)

-- Step 1: Recreate payout_transfer_status_enum
CREATE TYPE xcentro.payout_transfer_status_enum AS ENUM (
    'pending_approval',
    'approved',
    'initiated',
    'processing',
    'completed',
    'failed',
    'cancelled',
    'timeout'
);

-- Step 2: Recreate payout_transfer_requests table
CREATE TABLE xcentro.payout_transfer_requests (
    transfer_request_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payment_id UUID REFERENCES xcentro.payments(payment_id) ON DELETE RESTRICT,
    voucher_id UUID NOT NULL REFERENCES xcentro.payment_vouchers(voucher_id) ON DELETE RESTRICT,
    provider VARCHAR(50) NOT NULL,
    provider_beneficiary_id VARCHAR(255) NOT NULL,
    provider_transfer_id VARCHAR(255),
    amount BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'INR',
    purpose VARCHAR(255),
    reference VARCHAR(255),
    narration TEXT,
    transfer_mode VARCHAR(50),
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    approval_mode VARCHAR(20),
    approved_by UUID REFERENCES xcentro.user_table(id),
    approved_at TIMESTAMPTZ,
    status xcentro.payout_transfer_status_enum NOT NULL DEFAULT 'approved',
    initiated_at TIMESTAMPTZ,
    processing_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    failure_reason TEXT,
    utr VARCHAR(100),
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    next_retry_at TIMESTAMPTZ,
    last_retry_at TIMESTAMPTZ,
    last_webhook_at TIMESTAMPTZ,
    webhook_timeout_at TIMESTAMPTZ,
    webhook_history JSONB[] DEFAULT ARRAY[]::JSONB[],
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    deleted_at TIMESTAMPTZ,
    delete_reason TEXT,
    provider_metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    CONSTRAINT check_amount_positive CHECK (amount > 0)
);

-- Step 3: Restore castler_beneficiary_id column
ALTER TABLE xcentro.payee_bank_accounts
    ADD COLUMN IF NOT EXISTS castler_beneficiary_id VARCHAR(255);

-- Step 4: Restore FK from payment_state_changes to payout_transfer_requests
ALTER TABLE xcentro.payment_state_changes
    ADD CONSTRAINT payment_state_changes_transfer_request_id_fkey
    FOREIGN KEY (transfer_request_id) REFERENCES xcentro.payout_transfer_requests(transfer_request_id);
