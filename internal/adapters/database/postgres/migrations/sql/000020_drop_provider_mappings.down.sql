-- Rollback: Recreate payee_provider_mappings table
-- Note: Data will be lost. If rolling back with data, manual migration needed.

CREATE TYPE xcentro.payee_onboarding_status_enum AS ENUM ('not_onboarded', 'pending', 'active', 'failed');

CREATE TABLE xcentro.payee_provider_mappings (
    mapping_id UUID PRIMARY KEY DEFAULT uuidv7(),
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES xcentro.payee_bank_accounts(account_id) ON DELETE RESTRICT,
    provider xcentro.payout_provider_enum NOT NULL,
    provider_beneficiary_id VARCHAR(255),
    provider_account_id VARCHAR(255),
    onboarding_status xcentro.payee_onboarding_status_enum NOT NULL DEFAULT 'not_onboarded',
    onboarded_at TIMESTAMPTZ,
    onboarding_error_code VARCHAR(100),
    onboarding_error_message TEXT,
    provider_metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID REFERENCES xcentro.user_table(id),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    last_synced_at TIMESTAMPTZ,
    UNIQUE (payee_id, account_id, provider)
);

CREATE INDEX idx_provider_mappings_payee_account
ON xcentro.payee_provider_mappings(payee_id, account_id);

CREATE INDEX idx_provider_mappings_provider
ON xcentro.payee_provider_mappings(provider, onboarding_status);

CREATE INDEX idx_provider_mappings_pending
ON xcentro.payee_provider_mappings(onboarding_status, created_at)
WHERE onboarding_status IN ('not_onboarded', 'pending', 'failed');
