-- 038_fix_permissions.down.sql
-- 撤销权限修复

-- 移除 ops_viewer 的 agent:read 权限
DELETE FROM role_permissions 
WHERE role_id = '20000000-0000-0000-0000-000000000004' 
AND permission_id IN (
    SELECT id FROM permissions WHERE code IN ('agent:read', 'tool:read', 'trace:read', 'observability:read', 'outbox:read')
);

-- 移除 finance_user 的 conversation 和 agent:read 权限
DELETE FROM role_permissions 
WHERE role_id = '20000000-0000-0000-0000-000000000002' 
AND permission_id IN (
    SELECT id FROM permissions WHERE code IN ('conversation:read', 'conversation:write', 'agent:read', 'skill:read')
);

-- 移除 finance_manager 的 conversation 和 skill:read 权限
DELETE FROM role_permissions 
WHERE role_id = '20000000-0000-0000-0000-000000000003' 
AND permission_id IN (
    SELECT id FROM permissions WHERE code IN ('conversation:read', 'conversation:write', 'skill:read')
);
