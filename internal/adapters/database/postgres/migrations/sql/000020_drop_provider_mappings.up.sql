-- Migration: Drop payee_provider_mappings table
-- Reason: Over-engineered for single provider (Castler)
-- Solution: Store castler_beneficiary_id directly in payee_bank_accounts

DROP TABLE IF EXISTS xcentro.payee_provider_mappings CASCADE;

DROP TYPE IF EXISTS xcentro.payee_onboarding_status_enum CASCADE;
