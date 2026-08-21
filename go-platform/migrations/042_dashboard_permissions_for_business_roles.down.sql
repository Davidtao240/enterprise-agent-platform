-- 042_dashboard_permissions_for_business_roles.down.sql
-- 回滚：移除 042 添加的权限

-- 移除 business_user 的 approval:read
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code = 'approval:read'
  AND rp.role_id = '20000000-0000-0000-0000-000000000002';

-- 移除 business_user 的 agent:read
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code = 'agent:read'
  AND rp.role_id = '20000000-0000-0000-0000-000000000002';

-- 移除 business_reviewer 的 agent:read
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code = 'agent:read'
  AND rp.role_id = '20000000-0000-0000-0000-000000000003';

-- 移除 business_reviewer 的 conversation:read/write
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code IN ('conversation:read', 'conversation:write')
  AND rp.role_id = '20000000-0000-0000-0000-000000000003';

-- 删除 agent:read 权限码
DELETE FROM permissions WHERE code = 'agent:read';
