SELECT cron.unschedule('cleanup-expired-session-auth-data');
DROP FUNCTION IF EXISTS xcentro.cleanup_expired_session_auth_data();
