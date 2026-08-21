-- 038_fix_permissions.up.sql
-- 修复权限配置：为 ops_viewer 添加缺失的 :read 权限
-- 并确保所有角色拥有必要的查看权限

-- 为 ops_viewer 添加 agent:read 权限（用于查看 Dashboard 和 Run 详情）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'agent:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 ops_viewer 添加 tool:read 权限（用于查看 Tools/Knowledge）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'tool:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 ops_viewer 添加 trace:read 权限（用于查看 Trace）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'trace:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 ops_viewer 添加 observability:read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'observability:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 ops_viewer 添加 outbox:read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000004', p.id
FROM permissions p
WHERE p.code = 'outbox:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000004' 
    AND rp.permission_id = p.id
);

-- 为 finance_user 添加 conversation:read 和 conversation:write 权限（用于对话功能）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code IN ('conversation:read', 'conversation:write')
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000002' 
    AND rp.permission_id = p.id
);

-- 为 finance_user 添加 agent:read 权限（用于查看 Agent Gallery）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code = 'agent:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000002' 
    AND rp.permission_id = p.id
);

-- 为 finance_user 添加 skill:read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code = 'skill:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000002' 
    AND rp.permission_id = p.id
);

-- 为 finance_manager 添加 conversation:read 和 conversation:write 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code IN ('conversation:read', 'conversation:write')
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000003' 
    AND rp.permission_id = p.id
);

-- 为 finance_manager 添加 skill:read 权限
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code = 'skill:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp 
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000003' 
    AND rp.permission_id = p.id
);
