-- Tear down in reverse dependency order.

-- -------------------------------------------------------
-- 11. Drop templates_history
-- -------------------------------------------------------
DROP TABLE IF EXISTS xcentro.templates_history;

-- -------------------------------------------------------
-- 10. Drop document_corrections
-- -------------------------------------------------------
DROP TABLE IF EXISTS xcentro.document_corrections;

-- -------------------------------------------------------
-- 8. Revert quick vendor creation schema changes (idempotent)
-- -------------------------------------------------------

-- Drop partial indexes (safe regardless of whether section 8 of up ever ran)
DROP INDEX IF EXISTS xcentro.business_payee_relation_bpa_unique;
DROP INDEX IF EXISTS xcentro.business_payee_relation_bp_pending_unique;

-- Remove any pending relations (NULL account_id) before restoring NOT NULL.
-- This is DESTRUCTIVE: pending vendor relations created via the quick-create
-- flow will be lost. Surface a NOTICE with the row count so operators can
-- archive before proceeding.
DO $$
DECLARE
    pending_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO pending_count
    FROM xcentro.business_payee_relation WHERE account_id IS NULL;
    IF pending_count > 0 THEN
        RAISE NOTICE 'Down-migration 000060: deleting % pending business_payee_relation rows (account_id IS NULL). Archive first if needed.', pending_count;
    END IF;
END $$;
DELETE FROM xcentro.business_payee_relation WHERE account_id IS NULL;

-- Restore account_id NOT NULL only if it is currently nullable
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro'
          AND table_name   = 'business_payee_relation'
          AND column_name  = 'account_id'
          AND is_nullable  = 'YES'
    ) THEN
        ALTER TABLE xcentro.business_payee_relation ALTER COLUMN account_id SET NOT NULL;
    END IF;
END $$;

-- Re-add unique constraint only if it doesn't already exist
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'business_payee_relation_business_payee_account_key'
    ) THEN
        ALTER TABLE xcentro.business_payee_relation
            ADD CONSTRAINT business_payee_relation_business_payee_account_key
            UNIQUE (business_id, payee_id, account_id);
    END IF;
END $$;

-- Re-add FK from payment_vouchers only if it doesn't already exist
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'payment_vouchers_business_id_payee_id_account_id_fkey'
    ) THEN
        ALTER TABLE xcentro.payment_vouchers
            ADD CONSTRAINT payment_vouchers_business_id_payee_id_account_id_fkey
            FOREIGN KEY (business_id, payee_id, account_id)
                REFERENCES xcentro.business_payee_relation(business_id, payee_id, account_id)
                ON DELETE RESTRICT;
    END IF;
END $$;

-- Remove any unverified vendor rows before restoring NOT NULL.
-- DESTRUCTIVE: vendors quick-created without GSTIN will be lost.
DO $$
DECLARE
    unverified_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO unverified_count
    FROM xcentro.payee_vendor
    WHERE gstin IS NULL OR legal_name IS NULL OR gst_status IS NULL;
    IF unverified_count > 0 THEN
        RAISE NOTICE 'Down-migration 000060: deleting % unverified payee_vendor rows. Archive first if needed.', unverified_count;
    END IF;
END $$;
DELETE FROM xcentro.payee_vendor WHERE gstin IS NULL OR legal_name IS NULL OR gst_status IS NULL;

-- Restore NOT NULL on payee_vendor columns only if currently nullable
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'xcentro'
          AND table_name   = 'payee_vendor'
          AND column_name  = 'gstin'
          AND is_nullable  = 'YES'
    ) THEN
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN gstin SET NOT NULL;
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN legal_name SET NOT NULL;
        ALTER TABLE xcentro.payee_vendor ALTER COLUMN gst_status SET NOT NULL;
    END IF;
END $$;

-- -------------------------------------------------------
-- 9. Drop templates table
-- -------------------------------------------------------
DROP TABLE IF EXISTS xcentro.templates;

-- -------------------------------------------------------
-- 7–1. Drop extraction tables
-- -------------------------------------------------------
DROP TABLE IF EXISTS xcentro.match_results;
DROP TABLE IF EXISTS xcentro.line_items;

ALTER TABLE xcentro.invoice DROP CONSTRAINT IF EXISTS fk_invoice_po_id;

DROP TABLE IF EXISTS xcentro.purchase_orders;
DROP TABLE IF EXISTS xcentro.invoice;
DROP TABLE IF EXISTS xcentro.voucher_documents;

DROP TYPE IF EXISTS xcentro.invoice_type_enum;
