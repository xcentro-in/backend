-- Rollback: Remove payment gateway sub-merchant ID columns from payee_bank_accounts

ALTER TABLE xcentro.payee_bank_accounts
    DROP COLUMN IF EXISTS jiopay_submerchant_id,
    DROP COLUMN IF EXISTS easebuzz_submerchant_id;
