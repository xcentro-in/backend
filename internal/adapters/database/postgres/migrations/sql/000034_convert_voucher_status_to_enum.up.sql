-- #############################################################################
-- # Convert voucher status from VARCHAR to ENUM for type safety
-- # Also adds payment_status to track payment state independently
-- #############################################################################

-- Step 1: Create the voucher_status enum type
CREATE TYPE xcentro.voucher_status AS ENUM ('draft', 'finalized', 'posted', 'synced');

-- Step 2: Add a temporary column with the enum type
ALTER TABLE xcentro.payment_vouchers
ADD COLUMN status_new xcentro.voucher_status;

-- Step 3: Copy data from old column to new column with type conversion
UPDATE xcentro.payment_vouchers
SET status_new = status::xcentro.voucher_status;

-- Step 4: Drop the old column
ALTER TABLE xcentro.payment_vouchers
DROP COLUMN status;

-- Step 5: Rename the new column to the original name
ALTER TABLE xcentro.payment_vouchers
RENAME COLUMN status_new TO status;

-- Step 6: Set NOT NULL and default value
ALTER TABLE xcentro.payment_vouchers
ALTER COLUMN status SET NOT NULL,
ALTER COLUMN status SET DEFAULT 'draft'::xcentro.voucher_status;

-- Step 7: Update the comment for documentation
COMMENT ON COLUMN xcentro.payment_vouchers.status IS
'Voucher lifecycle status (ENUM): draft (created on Page 1→2 transition) → finalized (after payment) → posted (accounting complete) → synced (ERP synchronized). This is the voucher state, not approval or payment status.';

COMMENT ON TYPE xcentro.voucher_status IS
'Enum type for voucher lifecycle states in the payment_vouchers table';
