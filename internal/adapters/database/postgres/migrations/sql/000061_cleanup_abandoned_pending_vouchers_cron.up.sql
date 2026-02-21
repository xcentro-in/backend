-- ============================================================================
-- Migration: Create pg_cron job for abandoned pending voucher cleanup
-- Description: Removes orphaned payment_voucher_pending records from failed or
--              abandoned payments. On success these are deleted by the callback
--              handler; on failure/abandonment they are never cleaned up.
-- Note: Uses 3-hour threshold — stale pending payments auto-fail at 2 hours
--       (cleanup-stale-pending-payments job), so 3 hours ensures payment
--       status is settled before removing the pending voucher link.
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pg_cron;

CREATE OR REPLACE FUNCTION xcentro.cleanup_abandoned_pending_vouchers()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    deleted_count INT;
BEGIN
    WITH d AS (
        DELETE FROM xcentro.payment_voucher_pending
        WHERE created_at < NOW() - INTERVAL '3 hours'
        RETURNING payment_id
    ) SELECT COUNT(*) INTO deleted_count FROM d;

    IF deleted_count > 0 THEN
        RAISE NOTICE 'Pending voucher cleanup: deleted % abandoned records', deleted_count;
    END IF;
END;
$$;

SELECT cron.schedule(
    'cleanup-abandoned-pending-vouchers',
    '0 * * * *',  -- Every hour on the hour
    'SELECT xcentro.cleanup_abandoned_pending_vouchers();'
);

GRANT EXECUTE ON FUNCTION xcentro.cleanup_abandoned_pending_vouchers() TO xcentro_rw;

SELECT jobid, schedule, command, active
FROM cron.job
WHERE jobname = 'cleanup-abandoned-pending-vouchers';
