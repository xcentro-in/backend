-- ============================================================================
-- Rollback: Remove pg_cron job for failed card cleanup
-- ============================================================================

-- Step 1: Unschedule the cron job
SELECT cron.unschedule('cleanup-failed-tokenization-cards');

-- Step 2: Drop the cleanup function
DROP FUNCTION IF EXISTS xcentro.cleanup_failed_tokenization_cards();

-- Note: We don't drop pg_cron extension as other jobs might depend on it
