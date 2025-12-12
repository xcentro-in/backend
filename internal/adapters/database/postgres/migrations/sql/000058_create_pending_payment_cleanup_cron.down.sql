-- ============================================================================
-- Rollback: Remove pg_cron job for pending payment cleanup
-- ============================================================================

-- Step 1: Unschedule the cron job
SELECT cron.unschedule('cleanup-stale-pending-payments');

-- Step 2: Drop the cleanup function
DROP FUNCTION IF EXISTS xcentro.cleanup_stale_pending_payments();

-- Note: We don't drop pg_cron extension as other jobs might depend on it
