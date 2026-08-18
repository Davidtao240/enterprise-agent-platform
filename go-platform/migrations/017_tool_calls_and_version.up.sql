-- M2-B: Tool Definition Version + Tool Execution Contract
-- 1) tool_registry 增加 version 列,工具定义自此可追溯版本
-- 2) 创建 tool_calls 表,记录每次工具调用的完整执行契约
--    (identity/tenant/domain/risk 校验、幂等、审批、输出、验证)
-- 3) 唯一约束 (tenant_id, idempotency_key) 保证幂等去重

ALTER TABLE tool_registry
ADD COLUMN IF NOT EXISTS version VARCHAR(32) NOT NULL DEFAULT '1.0';

CREATE INDEX IF NOT EXISTS idx_tool_registry_version
    ON tool_registry (tool_id, version);

CREATE TABLE IF NOT EXISTS tool_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    run_id UUID NOT NULL,
    step_id UUID,
    tool_id VARCHAR(128) NOT NULL,
    tool_version VARCHAR(32) NOT NULL,
    connector_binding_id UUID,
    policy_version VARCHAR(64) NOT NULL,
    risk_level VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    input_hash VARCHAR(128) NOT NULL,
    input_summary_json JSONB,
    approval_task_id UUID,
    external_request_id VARCHAR(255),
    external_object_id VARCHAR(255),
    verification_json JSONB,
    output_summary_json JSONB,
    error_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_run
    ON tool_calls (run_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_status
    ON tool_calls (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_tool_calls_tool
    ON tool_calls (tool_id, tool_version);
