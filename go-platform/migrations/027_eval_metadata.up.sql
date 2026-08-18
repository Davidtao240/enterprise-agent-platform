-- M5-B: Eval 评估体系支撑
-- 1) agent_runs.metadata_json: Run 级扩展标记(M5-C Shadow/Replay Run 写入
--    shadow=true / replay=true;Eval 聚合按 TRACE_AND_EVAL.md §3.3 排除规则过滤)
-- 2) eval:read 权限点 (保护 POST /api/v1/eval/reports)

ALTER TABLE agent_runs
ADD COLUMN IF NOT EXISTS metadata_json JSONB NOT NULL DEFAULT '{}';

INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000022', 'eval:read', 'Read Eval Reports', 'eval', 'read')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000022'::uuid),
  ('20000000-0000-0000-0000-000000000004'::uuid, '30000000-0000-0000-0000-000000000022'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
