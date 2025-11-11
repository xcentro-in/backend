-- Drop payment_voucher_pending table

DROP INDEX IF EXISTS xcentro.idx_payment_voucher_pending_created_at;
DROP INDEX IF EXISTS xcentro.idx_payment_voucher_pending_voucher_id;
DROP INDEX IF EXISTS xcentro.idx_payment_voucher_pending_payment_id;

DROP TABLE IF EXISTS xcentro.payment_voucher_pending;
