-- 040_runtime_tool_artifacts.up.sql
-- M1-B: 为 agent runtime 增加 step.updated / tool.called / artifact.ready 事件
-- 对应的持久化表,供工具面板实时展示使用。

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id              VARCHAR(128) PRIMARY KEY,        -- 事件级唯一键(event_id)
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    run_id          VARCHAR(128) NOT NULL,
    tool_call_id    VARCHAR(128) NOT NULL,          -- LangChain run id
    tool_name       VARCHAR(128) NOT NULL,
    status          VARCHAR(16)  NOT NULL,          -- run / ok / err
    input_json      JSONB,
    output_json     JSONB,
    error_json      JSONB,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_atc_run_id   ON agent_tool_calls(tenant_id, run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_atc_tool_call ON agent_tool_calls(tool_call_id);

CREATE TABLE IF NOT EXISTS agent_artifacts (
    id              VARCHAR(256) PRIMARY KEY,        -- run_id:artifact_id
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    run_id          VARCHAR(128) NOT NULL,
    artifact_id     VARCHAR(128) NOT NULL,
    name            VARCHAR(255),
    sources_json    JSONB,
    preview         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_aa_run_id   ON agent_artifacts(tenant_id, run_id, created_at);

-- 便于面板侧按 run 批量读取
ALTER TABLE agent_tool_calls ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_artifacts ENABLE ROW LEVEL SECURITY;

-- 允许在 applyRuntimeControlMetadata 的 ON CONFLICT 中安全 Upsert
ALTER TABLE agent_tool_calls ADD CONSTRAINT agent_tool_calls_tool_call_id_key UNIQUE NULLS NOT DIFFERENT;
