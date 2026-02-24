SELECT cron.unschedule('cleanup-abandoned-pending-vouchers');
DROP FUNCTION IF EXISTS xcentro.cleanup_abandoned_pending_vouchers();
