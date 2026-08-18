-- M5-B down: 回滚 Eval 支撑变更(权限授予 → 权限点 → 列)
DELETE FROM role_permissions
WHERE permission_id = '30000000-0000-0000-0000-000000000022';

DELETE FROM permissions
WHERE id = '30000000-0000-0000-0000-000000000022';

ALTER TABLE agent_runs
DROP COLUMN IF EXISTS metadata_json;
