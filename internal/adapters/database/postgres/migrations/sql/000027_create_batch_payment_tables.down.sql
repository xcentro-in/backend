-- Drop indexes
DROP INDEX IF EXISTS xcentro.idx_batch_payment_items_voucher_id;
DROP INDEX IF EXISTS xcentro.idx_batch_payment_items_status;
DROP INDEX IF EXISTS xcentro.idx_batch_payment_items_batch_id;

DROP INDEX IF EXISTS xcentro.idx_batch_payments_created_at;
DROP INDEX IF EXISTS xcentro.idx_batch_payments_status;
DROP INDEX IF EXISTS xcentro.idx_batch_payments_business_id;

-- Drop tables
DROP TABLE IF EXISTS xcentro.batch_payment_items;
DROP TABLE IF EXISTS xcentro.batch_payments;
