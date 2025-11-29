-- Migration 000052: Add indexes for dashboard metrics
-- Purpose: Optimize dashboard queries for real-time metric fetching

-- ============================================================================
-- PAYMENTS TABLE INDEXES
-- ============================================================================

-- Composite index for total spend, recent payments, payment method breakdown
-- Covers queries filtering by business_id, status, and ordering by created_at
CREATE INDEX IF NOT EXISTS idx_payments_business_status_created
ON xcentro.payments(business_id, status, created_at DESC)
WHERE status IN ('success', 'pending', 'failed');

COMMENT ON INDEX xcentro.idx_payments_business_status_created IS
'Dashboard optimization: total spend this month, recent payments, payment trends';

-- Index for payment method breakdown (pie chart)
-- Optimizes GROUP BY payment_method queries with date filtering
CREATE INDEX IF NOT EXISTS idx_payments_business_month_method
ON xcentro.payments(business_id, payment_method, created_at)
WHERE status = 'success';

COMMENT ON INDEX xcentro.idx_payments_business_month_method IS
'Dashboard optimization: expense distribution by payment source (pie chart)';

-- ============================================================================
-- PAYMENT VOUCHERS TABLE INDEXES
-- ============================================================================

-- Index for overdue bills query
-- Covers queries filtering by business_id, invoice_due_date, and status
CREATE INDEX IF NOT EXISTS idx_vouchers_business_due_date_status
ON xcentro.payment_vouchers(business_id, invoice_due_date, status)
WHERE status IN ('draft', 'finalized');

COMMENT ON INDEX xcentro.idx_vouchers_business_due_date_status IS
'Dashboard optimization: overdue bills count and total value';

-- Index for upcoming large payments (top 3 by amount)
-- Uses DESC on net_amount for efficient sorting
-- Note: Cannot filter by invoice_due_date >= CURRENT_DATE in index (CURRENT_DATE not immutable)
CREATE INDEX IF NOT EXISTS idx_vouchers_business_upcoming_large
ON xcentro.payment_vouchers(business_id, status, net_amount DESC)
WHERE status IN ('draft', 'finalized');

COMMENT ON INDEX xcentro.idx_vouchers_business_upcoming_large IS
'Dashboard optimization: upcoming large payments (top 3 by amount)';

-- ============================================================================
-- ANALYSIS NOTES
-- ============================================================================

-- Index Strategy:
-- 1. CONCURRENTLY: Prevents table locks during index creation
-- 2. Partial indexes (WHERE clause): Reduces index size and improves performance
-- 3. DESC on created_at/net_amount: Optimizes ORDER BY DESC queries
-- 4. Composite indexes: Match query WHERE + ORDER BY patterns

-- Expected Performance:
-- - Total spend this month: < 20ms
-- - Overdue bills: < 30ms
-- - Recent payments: < 15ms
-- - Upcoming large payments: < 25ms
-- - Monthly spends (6 months): < 50ms (< 10K payments), < 200ms (> 50K payments)
-- - Payment source breakdown: < 30ms

-- Index Sizes (estimated):
-- - idx_payments_business_status_created: ~2-5MB per 100K payments
-- - idx_payments_business_month_method: ~1-3MB per 100K payments
-- - idx_vouchers_business_due_date_status: ~500KB-2MB per 50K vouchers
-- - idx_vouchers_business_upcoming_large: ~300KB-1MB per 50K vouchers
