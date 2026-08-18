-- M2-B rollback: drop tool_calls table, remove version column

DROP INDEX IF EXISTS idx_tool_calls_tool;
DROP INDEX IF EXISTS idx_tool_calls_status;
DROP INDEX IF EXISTS idx_tool_calls_run;
DROP TABLE IF EXISTS tool_calls;

DROP INDEX IF EXISTS idx_tool_registry_version;
ALTER TABLE tool_registry DROP COLUMN IF EXISTS version;
