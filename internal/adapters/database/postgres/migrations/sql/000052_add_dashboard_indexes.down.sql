-- Migration 000052 Rollback: Drop dashboard indexes

DROP INDEX CONCURRENTLY IF EXISTS xcentro.idx_payments_business_status_created;
DROP INDEX CONCURRENTLY IF EXISTS xcentro.idx_payments_business_month_method;
DROP INDEX CONCURRENTLY IF EXISTS xcentro.idx_vouchers_business_due_date_status;
DROP INDEX CONCURRENTLY IF EXISTS xcentro.idx_vouchers_business_upcoming_large;
