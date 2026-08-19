-- M7-A: 回滚 Conversation Engine

DROP TABLE IF EXISTS conversation_messages;
DROP TABLE IF EXISTS conversations;

-- 回滚 conversation 权限
DELETE FROM role_permissions
WHERE permission_id IN ('31000000-0000-0000-0000-000000000030'::uuid, '31000000-0000-0000-0000-000000000031'::uuid);

DELETE FROM permissions
WHERE id IN ('31000000-0000-0000-0000-000000000030'::uuid, '31000000-0000-0000-0000-000000000031'::uuid);