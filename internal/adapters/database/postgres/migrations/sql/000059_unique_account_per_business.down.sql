-- Rollback migration: 000060_unique_account_per_business
-- Removes the unique constraint on (business_id, account_id)

DROP INDEX IF EXISTS xcentro.unique_account_per_business;
