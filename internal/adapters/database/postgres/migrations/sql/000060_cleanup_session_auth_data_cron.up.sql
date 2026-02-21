-- ============================================================================
-- Migration: Create pg_cron job for session and auth data cleanup
-- Description: Purges expired verification codes, JWT refresh tokens, and
--              ops sessions that are no longer valid
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pg_cron;

CREATE OR REPLACE FUNCTION xcentro.cleanup_expired_session_auth_data()
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
    deleted_codes  INT;
    deleted_tokens INT;
    deleted_ops    INT;
BEGIN
    -- Delete expired verification codes (email, OTP, password reset)
    WITH d AS (
        DELETE FROM xcentro.user_verification_codes
        WHERE expires_at < NOW()
        RETURNING id
    ) SELECT COUNT(*) INTO deleted_codes FROM d;

    -- Delete expired JWT refresh tokens (user sessions)
    WITH d AS (
        DELETE FROM xcentro.user_tokens
        WHERE expires_at < NOW()
        RETURNING session_id
    ) SELECT COUNT(*) INTO deleted_tokens FROM d;

    -- Delete expired ops dashboard sessions
    WITH d AS (
        DELETE FROM xcentro.ops_session_table
        WHERE expires_at < NOW()
        RETURNING session_id
    ) SELECT COUNT(*) INTO deleted_ops FROM d;

    RAISE NOTICE 'Session/auth cleanup: deleted % verification codes, % user tokens, % ops sessions',
                 deleted_codes, deleted_tokens, deleted_ops;
END;
$$;

SELECT cron.schedule(
    'cleanup-expired-session-auth-data',
    '0 3 * * *',  -- Daily at 3:00 AM
    'SELECT xcentro.cleanup_expired_session_auth_data();'
);

GRANT EXECUTE ON FUNCTION xcentro.cleanup_expired_session_auth_data() TO xcentro_rw;

SELECT jobid, schedule, command, active
FROM cron.job
WHERE jobname = 'cleanup-expired-session-auth-data';
