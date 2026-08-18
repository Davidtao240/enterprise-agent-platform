-- M3-B: Webhook Inbox
-- 外部系统事件先落库后处理(Inbox 模式):
--   1. 签名校验结果持久化(signature_valid=false 不进入处理)
--   2. (connector_code, external_event_id) 唯一约束实现投递去重
--   3. 乱序与限流由消费端处理(WebhookConsumer 按 occurred_at 判定 stale)
--   4. payload_json 视为不可信数据,消费时做注入防护

CREATE TABLE IF NOT EXISTS webhook_events (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connector_code    VARCHAR(128) NOT NULL,
    external_event_id VARCHAR(255) NOT NULL,
    signature_valid   BOOLEAN      NOT NULL DEFAULT FALSE,
    payload_json      JSONB        NOT NULL,
    received_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    processed_at      TIMESTAMPTZ,
    process_error     TEXT,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_webhook_events_connector_event UNIQUE (connector_code, external_event_id)
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_unprocessed
    ON webhook_events (received_at)
    WHERE processed_at IS NULL;

-- M3-B: ticket_connector 注册(human_approved_write:ticket_create_or_update
-- 为 write 能力,发布阶段门禁要求 registry 达到该阶段才放行)
INSERT INTO connector_registry (connector_code, version, connector_type, capabilities_json, auth_type, release_stage, status)
VALUES (
    'ticket_connector',
    '1.0.0',
    'ticket',
    '[{"name":"ticket_create_or_update","kind":"write","input_schema":{"action":"create|update","ticket_id":"string(update required)","idempotency_key":"string(create dedup)","title":"string","priority":"low|medium|high|urgent","expected_version":"integer(update optimistic lock)"},"output_schema":{"ticket_id":"string","version":"integer","status":"string"}}]'::jsonb,
    'none',
    'human_approved_write',
    'active'
)
ON CONFLICT (connector_code, version) DO NOTHING;
