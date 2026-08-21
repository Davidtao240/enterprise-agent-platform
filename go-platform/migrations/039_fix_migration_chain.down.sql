-- 039_fix_migration_chain.down.sql
-- 撤销迁移链修复

-- 删除权限修复
DELETE FROM role_permissions 
WHERE role_id IN ('20000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000004')
AND permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'agent:read', 'tool:read', 'trace:read', 'observability:read', 'outbox:read',
        'conversation:read', 'conversation:write', 'skill:read'
    )
);

DELETE FROM permissions WHERE code IN (
    'agent:read', 'tool:read', 'skill:read', 'trace:read', 
    'observability:read', 'outbox:read', 'conversation:read', 'conversation:write'
);

-- 删除添加的列（如果存在）
ALTER TABLE agent_run_logs DROP COLUMN IF EXISTS estimated_cost;
ALTER TABLE agent_run_logs DROP COLUMN IF EXISTS duration_ms;

-- 删除创建的表（如果存在）
DROP TABLE IF EXISTS knowledge_chunks CASCADE;
DROP TABLE IF EXISTS knowledge_documents CASCADE;
DROP TABLE IF EXISTS knowledge_collections CASCADE;
DROP TABLE IF EXISTS sidecar_registrations CASCADE;
DROP TABLE IF EXISTS skill_usage_events CASCADE;
DROP TABLE IF EXISTS skill_installations CASCADE;
DROP TABLE IF EXISTS agent_package_installations CASCADE;
DROP TABLE IF EXISTS agent_package_versions CASCADE;
DROP TABLE IF EXISTS agent_package_registrations CASCADE;

-- 删除创建的角色
DROP ROLE IF EXISTS ops_viewer;
DROP ROLE IF EXISTS finance_user;
DROP ROLE IF EXISTS finance_manager;
DROP ROLE IF EXISTS platform_admin;
