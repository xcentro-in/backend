-- Rollback: Remove payee provider mappings
-- Phase 8 rollback

-- Drop indexes
DROP INDEX IF EXISTS xcentro.idx_provider_mappings_pending;
DROP INDEX IF EXISTS xcentro.idx_provider_mappings_provider;
DROP INDEX IF EXISTS xcentro.idx_provider_mappings_payee_account;

-- Drop table
DROP TABLE IF EXISTS xcentro.payee_provider_mappings;

-- Drop ENUMs (in reverse order of creation)
DROP TYPE IF EXISTS xcentro.payout_provider_enum;
DROP TYPE IF EXISTS xcentro.payee_onboarding_status_enum;
