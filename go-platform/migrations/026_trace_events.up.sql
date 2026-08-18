-- M5-A: 全链路 Trace 事件存储 (六层: Workflow/Run/Model Turn/Tool Call/Checkpoint/Interrupt)
-- Spec: docs/03_PLATFORM_SPEC/TRACE_AND_EVAL.md §2.2 / DATABASE_SCHEMA.md
-- 附带 trace:read 权限点 (platform_admin + ops_viewer)

CREATE TABLE IF NOT EXISTS trace_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id VARCHAR(128) NOT NULL,
    layer VARCHAR(16) NOT NULL CHECK (layer IN ('L1', 'L2', 'L3', 'L4', 'L5', 'L6')),
    parent_id UUID,
    event_type VARCHAR(64) NOT NULL,
    payload_json JSONB,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INT,
    tenant_id UUID NOT NULL,
    metadata_json JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_trace_events_trace_id ON trace_events (trace_id);
CREATE INDEX IF NOT EXISTS idx_trace_events_layer_timestamp ON trace_events (layer, timestamp);
CREATE INDEX IF NOT EXISTS idx_trace_events_tenant ON trace_events (tenant_id);

-- 权限点: trace:read (Trace 查询面)
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000021', 'trace:read', 'Read Trace Events', 'trace', 'read')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000021'::uuid),
  ('20000000-0000-0000-0000-000000000004'::uuid, '30000000-0000-0000-0000-000000000021'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
