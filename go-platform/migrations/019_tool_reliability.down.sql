-- M2-C 收尾回滚
DROP TABLE IF EXISTS tool_circuit_breakers;
DROP INDEX IF EXISTS idx_tool_calls_dead_letter;
ALTER TABLE tool_calls DROP COLUMN IF EXISTS is_dead_letter;
DROP INDEX IF EXISTS idx_tool_calls_trace;
ALTER TABLE tool_calls DROP COLUMN IF EXISTS trace_id;
