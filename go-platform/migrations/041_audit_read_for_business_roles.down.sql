-- 041_audit_read_for_business_roles.down.sql
-- 回滚：移除 business_user 和 business_reviewer 的 audit:read 权限

DELETE FROM role_permissions
WHERE role_id IN (
    '20000000-0000-0000-0000-000000000002',
    '20000000-0000-0000-0000-000000000003'
)
AND permission_id = (
    SELECT id FROM permissions WHERE code = 'audit:read'
);
