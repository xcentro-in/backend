-- ============================================================================
-- Migration: Create pg_cron job for failed card cleanup
-- Description: Automatically deletes failed tokenization cards and their
--              associated payments after 2 days
-- ============================================================================

-- Step 1: Enable pg_cron extension (idempotent)
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Step 2: Create cleanup function
CREATE OR REPLACE FUNCTION xcentro.cleanup_failed_tokenization_cards()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    deleted_cards_count INT;
    deleted_payments_count INT;
BEGIN
    -- Delete ALL tokenization payments (regardless of status) for failed cards
    -- Rationale: If card tokenization failed, payment status is irrelevant
    --            (e.g., payment may succeed but tokenization server fails)
    -- Must be done BEFORE deleting cards to avoid orphaned references
    WITH deleted_payments AS (
        DELETE FROM xcentro.payments
        WHERE payment_type = 'tokenization'
          AND created_at < NOW() - INTERVAL '2 days'
          AND payment_source_type = 'card'
          AND payment_source_id IN (
              SELECT card_id
              FROM xcentro.stored_cards
              WHERE is_failed_tokenization = TRUE
                AND added_at < NOW() - INTERVAL '2 days'
          )
        RETURNING payment_id
    )
    SELECT COUNT(*) INTO deleted_payments_count FROM deleted_payments;

    -- Delete failed tokenization cards older than 2 days
    WITH deleted_cards AS (
        DELETE FROM xcentro.stored_cards
        WHERE is_failed_tokenization = TRUE
          AND added_at < NOW() - INTERVAL '2 days'
        RETURNING card_id
    )
    SELECT COUNT(*) INTO deleted_cards_count FROM deleted_cards;

    -- Log the cleanup results (will appear in PostgreSQL logs)
    RAISE NOTICE 'Failed card cleanup: deleted % cards and % payments',
                 deleted_cards_count, deleted_payments_count;
END;
$$;

-- Step 3: Schedule the cleanup job to run daily at 2:00 AM
-- Cron format: minute hour day month weekday
SELECT cron.schedule(
    'cleanup-failed-tokenization-cards',  -- Job name
    '0 2 * * *',                          -- Every day at 2:00 AM
    'SELECT xcentro.cleanup_failed_tokenization_cards();'
);

-- Step 4: Grant execute permission to app user
GRANT EXECUTE ON FUNCTION xcentro.cleanup_failed_tokenization_cards() TO xcentro_app_user;

-- Verify the job was created
SELECT jobid, schedule, command, active
FROM cron.job
WHERE jobname = 'cleanup-failed-tokenization-cards';
