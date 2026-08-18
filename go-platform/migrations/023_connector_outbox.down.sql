DELETE FROM connector_registry WHERE connector_code = 'erp_connector' AND version = '1.0.0';
DROP INDEX IF EXISTS idx_connector_outbox_tool_call;
DROP INDEX IF EXISTS idx_connector_outbox_state_scan;
DROP INDEX IF EXISTS idx_connector_outbox_dispatchable;
DROP TABLE IF EXISTS connector_outbox;
