-- Migration: Make added_by column NOT NULL in business_payee_relation
-- Purpose: Enforce that every relation must have an added_by user

-- Step 1: For any existing NULL values, set them to the business owner's user ID
-- This handles legacy data where added_by was not captured
UPDATE xcentro.business_payee_relation bpr
SET added_by = (
    SELECT bur.user_id
    FROM xcentro.business_user_relation bur
    WHERE bur.bid = bpr.business_id
      AND bur.role = 'owner'
    LIMIT 1
)
WHERE bpr.added_by IS NULL;

-- Step 2: Make the column NOT NULL
ALTER TABLE xcentro.business_payee_relation
ALTER COLUMN added_by SET NOT NULL;

-- Add comment
COMMENT ON COLUMN xcentro.business_payee_relation.added_by IS 'User who added this payee relation (required - extracted from JWT)';
