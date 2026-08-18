-- M3-A 回滚
ALTER TABLE connector_bindings
DROP CONSTRAINT IF EXISTS ck_connector_binding_env;
DROP INDEX IF EXISTS idx_connector_bindings_env;
ALTER TABLE connector_bindings
DROP COLUMN IF EXISTS environment,
DROP COLUMN IF EXISTS allowed_capabilities,
DROP COLUMN IF EXISTS connector_version;
DROP INDEX IF EXISTS idx_connector_registry_code;
DROP TABLE IF EXISTS connector_registry;
