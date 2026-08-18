-- M5-C down: 实验表回滚(依赖顺序: 权限 → canary → shadow_executions → shadow_rules → replay)
DELETE FROM role_permissions
WHERE permission_id = '30000000-0000-0000-0000-000000000023';

DELETE FROM permissions
WHERE id = '30000000-0000-0000-0000-000000000023';

DROP TABLE IF EXISTS canary_releases;
DROP TABLE IF EXISTS shadow_executions;
DROP TABLE IF EXISTS shadow_rules;
DROP TABLE IF EXISTS replay_sessions;
