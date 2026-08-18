-- M5-C: Replay / Shadow / Canary 实验机制存储 (business-domain neutral)
-- Spec: docs/03_PLATFORM_SPEC/TRACE_AND_EVAL.md §4.4 / DATABASE_SCHEMA.md
-- 附带 experiment:manage 权限点 (platform_admin)

CREATE TABLE IF NOT EXISTS replay_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    source_run_id VARCHAR(128) NOT NULL,
    replay_run_id VARCHAR(128),
    graph_key VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    diff_json JSONB,
    created_by VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_replay_sessions_tenant ON replay_sessions (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS shadow_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    business_app_code VARCHAR(64) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    shadow_graph_key VARCHAR(128) NOT NULL,
    traffic_percent INT NOT NULL CHECK (traffic_percent BETWEEN 0 AND 100),
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'stopped')),
    created_by VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_shadow_rules_match ON shadow_rules (tenant_id, business_app_code, graph_key, status);

CREATE TABLE IF NOT EXISTS shadow_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    rule_id UUID NOT NULL REFERENCES shadow_rules(id),
    primary_run_id VARCHAR(128) NOT NULL,
    shadow_run_id VARCHAR(128) NOT NULL,
    comparison_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_shadow_executions_pending ON shadow_executions (created_at)
WHERE comparison_json IS NULL;

CREATE TABLE IF NOT EXISTS canary_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    business_app_code VARCHAR(64) NOT NULL,
    graph_key VARCHAR(128) NOT NULL,
    candidate_graph_key VARCHAR(128) NOT NULL,
    stages JSONB NOT NULL,
    current_stage_index INT NOT NULL DEFAULT 0,
    max_error_rate DOUBLE PRECISION NOT NULL,
    min_sample_size INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'promoted', 'rolled_back')),
    created_by VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_canary_releases_match ON canary_releases (tenant_id, business_app_code, graph_key, status);

-- 权限点: experiment:manage (Replay/Shadow/Canary 管理面)
INSERT INTO permissions (id, code, name, resource, action) VALUES
  ('30000000-0000-0000-0000-000000000023', 'experiment:manage', 'Manage Experiments', 'experiment', 'manage')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT role_id, permission_id
FROM (VALUES
  ('20000000-0000-0000-0000-000000000001'::uuid, '30000000-0000-0000-0000-000000000023'::uuid)
) AS grants(role_id, permission_id)
ON CONFLICT DO NOTHING;
