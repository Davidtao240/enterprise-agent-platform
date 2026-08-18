-- M5-A rollback
DELETE FROM role_permissions
WHERE permission_id = '30000000-0000-0000-0000-000000000021'::uuid;
DELETE FROM permissions WHERE id = '30000000-0000-0000-0000-000000000021'::uuid;
DROP TABLE IF EXISTS trace_events;
