DROP INDEX IF EXISTS idx_webhook_events_unprocessed;
DROP TABLE IF EXISTS webhook_events;
DELETE FROM connector_registry WHERE connector_code = 'ticket_connector' AND version = '1.0.0';
