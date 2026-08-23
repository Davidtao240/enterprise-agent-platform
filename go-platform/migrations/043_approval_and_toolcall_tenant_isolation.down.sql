-- migrations/043_approval_and_toolcall_tenant_isolation.down.sql
DROP INDEX IF EXISTS idx_tool_calls_tenant;
DROP INDEX IF EXISTS idx_approval_tasks_tenant;
ALTER TABLE tool_calls DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE approval_tasks DROP COLUMN IF EXISTS tenant_id;
