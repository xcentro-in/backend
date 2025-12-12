-- ============================================================================
-- Migration: Create pg_cron job for pending payment cleanup
-- Description: Automatically marks pending payments as failed after 2 hours
--              if callback URL is missed or webhook is not received
-- ============================================================================

-- Step 1: Enable pg_cron extension (idempotent)
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Step 2: Create cleanup function
CREATE OR REPLACE FUNCTION xcentro.cleanup_stale_pending_payments()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    updated_payments_count INT;
BEGIN
    -- Update pending payments older than 2 hours to failed status
    -- Excludes tokenization payments (handled by separate cleanup job)
    -- Rationale: If callback URL is missed or webhook not received within 2 hours,
    --            payment should be marked as failed to prevent indefinite pending state
    WITH updated_payments AS (
        UPDATE xcentro.payments
        SET
            status = 'failed',
            updated_at = NOW(),
            metadata = COALESCE(metadata, '{}'::jsonb) ||
                      jsonb_build_object(
                          'auto_failed_at', NOW()::text,
                          'auto_failed_reason', 'Payment timed out after 2 hours in pending state',
                          'original_status', 'pending'
                      )
        WHERE status = 'pending'
          AND payment_type = 'payment'  -- Exclude tokenization and refund payments
          AND created_at < NOW() - INTERVAL '2 hours'
        RETURNING payment_id
    )
    SELECT COUNT(*) INTO updated_payments_count FROM updated_payments;

    -- Log the cleanup results (will appear in PostgreSQL logs)
    IF updated_payments_count > 0 THEN
        RAISE NOTICE 'Pending payment cleanup: marked % payment(s) as failed', updated_payments_count;
    END IF;
END;
$$;

-- Step 3: Schedule the cleanup job to run every 1 hour
-- Cron format: minute hour day month weekday
-- Runs at:every hour
SELECT cron.schedule(
    'cleanup-stale-pending-payments',  -- Job name
    '* 1 * * *',                    -- Every 1 hour
    'SELECT xcentro.cleanup_stale_pending_payments();'
);

-- Step 4: Grant execute permission to app user
-- Note: Default privileges should already grant this, but explicit grant ensures compatibility
GRANT EXECUTE ON FUNCTION xcentro.cleanup_stale_pending_payments() TO xcentro_rw;

-- Verify the job was created
SELECT jobid, schedule, command, active
FROM cron.job
WHERE jobname = 'cleanup-stale-pending-payments';
