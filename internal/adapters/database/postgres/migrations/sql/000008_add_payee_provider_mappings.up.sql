-- #############################################################################
-- # Phase 8: Payee Provider Onboarding
-- # Created: 2025-10-05
-- # Purpose: Track payee registrations with payout providers (Castler, Razorpay, etc.)
-- #############################################################################

-- =============================================================================
-- Create ENUMs for Provider Status
-- =============================================================================

-- Payee onboarding status with provider
CREATE TYPE xcentro.payee_onboarding_status_enum AS ENUM (
    'not_onboarded',  -- Payee not yet registered with provider
    'pending',        -- Onboarding API called, awaiting provider response
    'active',         -- Successfully onboarded, can receive payouts
    'failed',         -- Onboarding failed
    'suspended',      -- Provider suspended this beneficiary
    'deactivated'     -- Manually deactivated
);

-- Payout provider type
CREATE TYPE xcentro.payout_provider_enum AS ENUM (
    'castler',        -- Castler (primary)
    'razorpay',       -- Razorpay (backup)
    'bank_api'        -- Direct bank API (future)
);

-- =============================================================================
-- Create Payee-Provider Mappings Table
-- =============================================================================

CREATE TABLE xcentro.payee_provider_mappings (
    mapping_id UUID PRIMARY KEY DEFAULT uuidv7(),

    -- Payee and account reference
    payee_id UUID NOT NULL REFERENCES xcentro.payee_master(payee_id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES xcentro.payee_bank_accounts(account_id) ON DELETE RESTRICT,

    -- Provider details
    provider xcentro.payout_provider_enum NOT NULL,
    provider_beneficiary_id VARCHAR(255),  -- Castler beneficiary_id, Razorpay contact_id, etc.
    provider_account_id VARCHAR(255),      -- Provider's account reference (if separate from beneficiary)

    -- Onboarding status
    onboarding_status xcentro.payee_onboarding_status_enum NOT NULL DEFAULT 'not_onboarded',
    onboarded_at TIMESTAMPTZ,
    onboarding_error_code VARCHAR(100),
    onboarding_error_message TEXT,

    -- Provider metadata (JSONB for flexibility)
    provider_metadata JSONB,  -- Provider-specific data (Castler beneficiary details, Razorpay contact, etc.)

    -- Audit fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID REFERENCES xcentro.user_table(id),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    last_synced_at TIMESTAMPTZ,  -- Last time we synced with provider

    -- Unique constraint: one mapping per payee-account-provider combination
    UNIQUE (payee_id, account_id, provider)
);

-- =============================================================================
-- Indexes for Provider Mappings
-- =============================================================================

-- Index for lookup by payee+account (used when initiating payout)
CREATE INDEX idx_provider_mappings_payee_account
ON xcentro.payee_provider_mappings(payee_id, account_id);

-- Index for querying by provider (for provider-specific operations)
CREATE INDEX idx_provider_mappings_provider
ON xcentro.payee_provider_mappings(provider, onboarding_status);

-- Index for finding pending onboardings (for worker)
CREATE INDEX idx_provider_mappings_pending
ON xcentro.payee_provider_mappings(onboarding_status, created_at)
WHERE onboarding_status IN ('not_onboarded', 'pending', 'failed');

-- =============================================================================
-- Comments
-- =============================================================================

COMMENT ON TABLE xcentro.payee_provider_mappings IS
'Maps payees to payout providers (Castler, Razorpay, etc.) - supports multi-provider routing';

COMMENT ON COLUMN xcentro.payee_provider_mappings.provider_beneficiary_id IS
'Provider-generated beneficiary ID (e.g., Castler beneficiary_id, Razorpay contact_id)';

COMMENT ON COLUMN xcentro.payee_provider_mappings.provider_metadata IS
'Provider-specific metadata (Castler: beneficiary details, Razorpay: contact info, etc.)';

COMMENT ON COLUMN xcentro.payee_provider_mappings.onboarding_status IS
'Onboarding lifecycle: not_onboarded → pending → active (or failed)';
