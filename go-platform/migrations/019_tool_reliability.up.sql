-- M2-C 收尾: Tool 可靠性(超时/DLQ/熔断) + M2-E trace 串联
-- 领域中立:全部作用于 tool_calls / 熔断状态表,不含业务逻辑。

-- M2-E: 全链 Trace 串联(ToolCall → Policy → Approval → Result → Verify)
ALTER TABLE tool_calls
ADD COLUMN IF NOT EXISTS trace_id VARCHAR(64);
CREATE INDEX IF NOT EXISTS idx_tool_calls_trace
ON tool_calls (trace_id)
WHERE trace_id IS NOT NULL;

-- M2-C.10 DLQ: 超过重试上限/不可恢复的调用进入死信
ALTER TABLE tool_calls
ADD COLUMN IF NOT EXISTS is_dead_letter BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_tool_calls_dead_letter
ON tool_calls (is_dead_letter, updated_at DESC)
WHERE is_dead_letter;

-- M2-C.9 熔断器: 按 tool 维度的外部系统熔断状态
-- state: closed(正常) / open(熔断,拒绝新调用) / half_open(冷却期满,放行探测)
CREATE TABLE IF NOT EXISTS tool_circuit_breakers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tool_id VARCHAR(128) NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'closed',
    consecutive_failures INT NOT NULL DEFAULT 0,
    opened_at TIMESTAMPTZ,
    half_open_probe_at TIMESTAMPTZ,
    last_failure_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_tool_circuit_breakers_tool UNIQUE (tool_id),
    CONSTRAINT ck_circuit_state CHECK (state IN ('closed','open','half_open'))
);
