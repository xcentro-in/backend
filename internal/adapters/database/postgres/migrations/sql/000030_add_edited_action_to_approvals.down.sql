-- Rollback: Remove 'edited' action from approval_action_enum
-- Note: PostgreSQL does not support removing enum values directly.
-- To rollback, the enum must be recreated without the 'edited' value:

-- 1. Check if 'edited' value is in use
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM xcentro.voucher_approvals WHERE action = 'edited'
    ) THEN
        RAISE EXCEPTION 'Cannot rollback: approval records with action=edited exist';
    END IF;
END $$;

-- 2. Recreate the enum without 'edited'
ALTER TYPE xcentro.approval_action_enum RENAME TO approval_action_enum_old;

CREATE TYPE xcentro.approval_action_enum AS ENUM (
    'pending',
    'approved',
    'rejected',
    'request_changes'
);

-- Drop the default before altering column type
ALTER TABLE xcentro.voucher_approvals
    ALTER COLUMN action DROP DEFAULT;

ALTER TABLE xcentro.voucher_approvals
    ALTER COLUMN action TYPE xcentro.approval_action_enum
    USING action::text::xcentro.approval_action_enum;

-- Restore the default with the new enum type
ALTER TABLE xcentro.voucher_approvals
    ALTER COLUMN action SET DEFAULT 'pending'::xcentro.approval_action_enum;

DROP TYPE xcentro.approval_action_enum_old;
