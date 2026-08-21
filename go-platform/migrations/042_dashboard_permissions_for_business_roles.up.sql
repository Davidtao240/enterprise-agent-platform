-- 042_dashboard_permissions_for_business_roles.up.sql
-- 为 business_user 和 business_reviewer 添加 Dashboard 所需的关键权限
-- 根因：DashboardPage 调用 getApprovalTasks (需要 approval:read) 和 getDashboard (需要 agent:read)
-- 而 business_user 初始种子数据缺少这两个权限，导致每次进入工作台都看到 permission denied

-- 1. 先确保 agent:read 权限码存在（permissions 表中原本没有此权限）
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'agent:read', 'Read Agents', 'agent', 'read')
ON CONFLICT (code) DO NOTHING;

-- 2. 为 business_user (role_id 02) 添加 approval:read
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code = 'approval:read'
AND NOT EXISTS (
  SELECT 1 FROM role_permissions rp
  WHERE rp.role_id = '20000000-0000-0000-0000-000000000002'
  AND rp.permission_id = p.id
);

-- 3. 为 business_user (role_id 02) 添加 agent:read
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code = 'agent:read'
AND NOT EXISTS (
  SELECT 1 FROM role_permissions rp
  WHERE rp.role_id = '20000000-0000-0000-0000-000000000002'
  AND rp.permission_id = p.id
);

-- 4. 为 business_reviewer (role_id 03) 添加 agent:read（如果还没有的话）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code = 'agent:read'
AND NOT EXISTS (
  SELECT 1 FROM role_permissions rp
  WHERE rp.role_id = '20000000-0000-0000-0000-000000000003'
  AND rp.permission_id = p.id
);

-- 5. 为 business_reviewer (role_id 03) 添加 conversation:read/write（如果还没有的话）
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code IN ('conversation:read', 'conversation:write')
AND NOT EXISTS (
  SELECT 1 FROM role_permissions rp
  WHERE rp.role_id = '20000000-0000-0000-0000-000000000003'
  AND rp.permission_id = p.id
);
