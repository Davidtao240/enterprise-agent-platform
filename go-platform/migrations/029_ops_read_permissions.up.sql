-- M6-B: 可靠性运维只读权限 (business-domain neutral)
-- Spec: docs/06_FRONTEND/WORKBENCH_DESIGN.md §6.2
-- tool:read   → Tool Call 探索器 / DLQ
-- outbox:read → Outbox 监控 / 人工补偿(平台管理员限定,审计日志兜底)

INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000024', 'tool:read', 'Read Tool Calls', 'tool', 'read'),
  ('30000000-0000-0000-0000-000000000025', 'outbox:read', 'Read Outbox Governance', 'outbox', 'read')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000024'::uuid),
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000025'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
