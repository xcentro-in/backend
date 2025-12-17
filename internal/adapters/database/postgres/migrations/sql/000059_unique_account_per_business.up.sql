-- Migration: 000060_unique_account_per_business
-- Purpose: Prevent a business from having the same bank account appear multiple times
--          across different payees (anti-fraud measure)
--
-- Valid use case (allowed):
--   Business A + Vendor XYZ + Account 123
--   Business B + Vendor XYZ + Account 123  (different businesses, same vendor)
--
-- Fraud use case (blocked):
--   Business A + Vendor XYZ + Account 123
--   Business A + Employee John + Account 123  (same business, different payees)
--   Business A + Vendor ABC + Account 123     (same business, different payees)

-- Create unique index on (business_id, account_id)
-- This ensures each business can only have one payee per bank account
CREATE UNIQUE INDEX unique_account_per_business
ON xcentro.business_payee_relation (business_id, account_id);

-- Add comment for documentation
COMMENT ON INDEX xcentro.unique_account_per_business IS
  'Ensures each business can only have one payee per bank account (prevents fraud while allowing cross-business account reuse for Castler efficiency)';
