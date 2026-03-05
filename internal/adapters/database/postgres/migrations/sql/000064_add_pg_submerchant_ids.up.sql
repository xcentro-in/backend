-- Migration: Add payment gateway sub-merchant IDs to payee_bank_accounts
-- These IDs are assigned by the payment gateway when a payee is onboarded
-- as a sub-merchant (required for split settlement).
-- Each payee bank account can be registered with multiple gateways,
-- hence separate columns per gateway.

ALTER TABLE xcentro.payee_bank_accounts
    ADD COLUMN IF NOT EXISTS jiopay_submerchant_id  VARCHAR(255),
    ADD COLUMN IF NOT EXISTS easebuzz_submerchant_id VARCHAR(255);

COMMENT ON COLUMN xcentro.payee_bank_accounts.jiopay_submerchant_id    IS 'JioPay-assigned sub-merchant ID for split settlement; NULL until the payee is onboarded with JioPay';
COMMENT ON COLUMN xcentro.payee_bank_accounts.easebuzz_submerchant_id  IS 'Easebuzz-assigned sub-merchant ID for split settlement; NULL until the payee is onboarded with Easebuzz';
