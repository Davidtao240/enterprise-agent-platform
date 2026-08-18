-- M3-C: Connector Outbox(Saga/Outbox/Compensation)
-- 跨系统写操作不假设分布式强事务:
--   1. 写请求先落 outbox(pending),由 Dispatcher 周期投递(至少一次);
--   2. 投递失败按 attempts/next_attempt_at 指数退避重试;
--   3. 部分成功可恢复:ToolCall 取消/审批撤回且已发出 → compensate_pending
--      → 补偿撤销外部单据 → compensated;
--   4. 全链路关联:tool_call_id → tool_calls.trace_id/external_request_id
--      → Run/Step/Audit,外部请求可追溯。
--
-- state 语义:
--   pending            待投递(next_attempt_at 到期后可投)
--   sent               已发出,待 Verify 确认
--   confirmed          外部副作用已核验(终态)
--   compensate_pending 待补偿(ToolCall 取消/重试耗尽且有副作用)
--   compensated        补偿完成(终态)
--   failed             不可恢复失败(终态,last_error 留痕)

CREATE TABLE IF NOT EXISTS connector_outbox (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_call_id        UUID NOT NULL REFERENCES tool_calls(id),
    tenant_id           UUID NOT NULL,
    connector_code      VARCHAR(128) NOT NULL,
    operation           VARCHAR(64)  NOT NULL,
    payload_json        JSONB        NOT NULL,
    state               VARCHAR(32)  NOT NULL DEFAULT 'pending',
    external_request_id VARCHAR(255),
    external_object_id  VARCHAR(255),
    attempts            INT          NOT NULL DEFAULT 0,
    next_attempt_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_error          TEXT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_connector_outbox_dispatchable
    ON connector_outbox (next_attempt_at)
    WHERE state = 'pending';
CREATE INDEX IF NOT EXISTS idx_connector_outbox_state_scan
    ON connector_outbox (updated_at)
    WHERE state IN ('sent', 'compensate_pending');
CREATE INDEX IF NOT EXISTS idx_connector_outbox_tool_call
    ON connector_outbox (tool_call_id);

-- M3-C: erp_connector 注册
-- erp_purchase_request_preview 为 read(Sandbox/Dry-run 优先:预检无副作用,任意阶段放行);
-- erp_purchase_request / erp_purchase_cancel 为 write(release_stage=human_approved_write,
-- 实际放行还要求 binding.environment ≥ shadow,由 ConnectorRuntime 双层门禁控制)。
INSERT INTO connector_registry (connector_code, version, connector_type, capabilities_json, auth_type, release_stage, status)
VALUES (
    'erp_connector',
    '1.0.0',
    'erp',
    '[{"name":"erp_purchase_request_preview","kind":"read","input_schema":{"idempotency_key":"string","title":"string","items":"array","total_amount":"number","supplier":"string"},"output_schema":{"would_create":"boolean","estimated_pr":"string","validations":"array"}},{"name":"erp_purchase_request","kind":"write","input_schema":{"idempotency_key":"string(required,dedup)","title":"string","items":"array","total_amount":"number","supplier":"string"},"output_schema":{"purchase_request_id":"string","status":"string","version":"integer"}},{"name":"erp_purchase_cancel","kind":"write","input_schema":{"purchase_request_id":"string(required)","reason":"string"},"output_schema":{"cancelled":"boolean","status":"string"}}]'::jsonb,
    'none',
    'human_approved_write',
    'active'
)
ON CONFLICT (connector_code, version) DO NOTHING;
