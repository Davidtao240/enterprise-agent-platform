-- M2-C rollback
DROP INDEX IF EXISTS uq_approval_tasks_tool_call;

ALTER TABLE approval_tasks DROP COLUMN IF EXISTS payload_hash;
ALTER TABLE approval_tasks DROP COLUMN IF EXISTS tool_call_id;

-- 仅当不存在遗留 NULL 行时才能恢复 NOT NULL
ALTER TABLE approval_tasks ALTER COLUMN workflow_instance_id SET NOT NULL;
ALTER TABLE approval_tasks ALTER COLUMN node_instance_id SET NOT NULL;

ALTER TABLE tool_calls DROP COLUMN IF EXISTS retry_count;
ALTER TABLE tool_calls DROP COLUMN IF EXISTS timeout_at;
ALTER TABLE tool_calls DROP COLUMN IF EXISTS business_app_code;
