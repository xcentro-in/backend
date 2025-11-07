-- #############################################################################
-- # Rollback: Convert voucher status from ENUM back to VARCHAR
-- #############################################################################

-- Step 1: Add a temporary VARCHAR column
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN status_old VARCHAR(50);

-- Step 2: Copy data from enum column to varchar column
UPDATE xcentro.payment_vouchers
SET status_old = status::TEXT;

-- Step 3: Drop the enum column
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN status;

-- Step 4: Rename the varchar column back to original name
ALTER TABLE xcentro.payment_vouchers
RENAME COLUMN status_old TO status;

-- Step 5: Set NOT NULL and default value
ALTER TABLE xcentro.payment_vouchers
ALTER COLUMN status SET NOT NULL,
ALTER COLUMN status SET DEFAULT 'draft';

-- Step 6: Drop the enum type
DROP TYPE xcentro.voucher_status;

-- Step 7: Restore original comment
COMMENT ON COLUMN xcentro.payment_vouchers.status IS
'Voucher lifecycle: draft (created on Page 1→2 transition) → finalized (after payment) → posted (accounting complete) → synced (ERP synchronized)';
