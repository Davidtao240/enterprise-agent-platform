-- 041_audit_read_for_business_roles.up.sql
-- 业务用户和审批人在查看工作流详情时，需要通过"审计链路"按钮
-- 查看该工作流实例相关的审计日志，因此授予 audit:read 权限。

-- business_user (finance_user): 可查看自己发起的工作流审计链路
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000002', p.id
FROM permissions p
WHERE p.code = 'audit:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000002'
    AND rp.permission_id = p.id
);

-- business_reviewer (finance_manager): 可查看审批相关的审计链路
INSERT INTO role_permissions (role_id, permission_id)
SELECT '20000000-0000-0000-0000-000000000003', p.id
FROM permissions p
WHERE p.code = 'audit:read'
AND NOT EXISTS (
    SELECT 1 FROM role_permissions rp
    WHERE rp.role_id = '20000000-0000-0000-0000-000000000003'
    AND rp.permission_id = p.id
);
