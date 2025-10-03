-- #############################################################################
-- # Rollback Payment Voucher System
-- # Phase 1: Drop all voucher tables
-- #############################################################################

-- Drop tables in reverse dependency order

DROP TABLE IF EXISTS xcentro.erp_sync_queue CASCADE;

DROP TABLE IF EXISTS xcentro.voucher_approvals CASCADE;

DROP TABLE IF EXISTS xcentro.payment_vouchers CASCADE;
